package gen

import (
	"flag"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/go-kanna/internal/cli"
	"github.com/go-kanna/internal/diag"
	"github.com/go-kanna/internal/directive"
	"github.com/go-kanna/internal/output"
	"github.com/go-kanna/internal/packages"
	"github.com/go-kanna/internal/scan"
	"golang.org/x/mod/modfile"
)

// outputFile is the file kanna-fixture writes into the destination.
const outputFile = "fixture_gen.go"

// config holds what kanna-fixture's own flags set.
type config struct {
	destination string
	exclude     []string
	tags        string
}

// Command returns kanna-fixture.
func Command() cli.Command[config] {
	return cli.Command[config]{
		Name:    generator,
		Summary: "generate a fixture for every exported struct of a package",
		Args:    "[package]",
		Help: "The package defaults to the one in the current directory, so that\n" +
			"//go:generate go tool " + generator + " needs no argument. The fixtures import\n" +
			"it, so they go into a package of their own: fixture, in the current directory,\n" +
			"unless -destination names another.",
		Flags: func(fs *flag.FlagSet, cfg *config) {
			fs.StringVar(&cfg.destination, "destination", "fixture", "write "+outputFile+" into `dir`")
			cli.Tags(fs, &cfg.tags)
			fs.Func("exclude", "skip the structs `names` lists, separated by commas", func(s string) error {
				for name := range strings.SplitSeq(s, ",") {
					if name = strings.TrimSpace(name); name != "" {
						cfg.exclude = append(cfg.exclude, name)
					}
				}
				return nil
			})
		},
		Run:  run,
		Deps: reportDeps,
	}
}

func run(dir string, cfg config, patterns []string) ([]output.Change, []diag.Diag) {
	if len(patterns) > 1 {
		return nil, []diag.Diag{diag.Errorf(token.Position{}, "%s reads one package, got %d", generator, len(patterns))}
	}
	dest := destination(dir, cfg)
	set, err := packages.Load(toLoad(dest, patterns),
		packages.Config{Dir: dir, Tags: cfg.tags, Regenerated: []string{outputFile}})
	if err != nil {
		return nil, []diag.Diag{diag.Errorf(token.Position{}, "%v", err)}
	}
	// Code calling a fixture does not compile before the run writes it, nor
	// does code calling it as the run is about to change it. So a type error
	// does not stop the run here: packages.Recheck tells, once the output is in
	// place, whether it remains.
	blocking, deferred := set.Split()
	if diag.HasErrors(blocking) {
		return nil, append(blocking, set.SetAsideWarnings()...)
	}
	pkg, d, ok := source(set, patterns, dest)
	if !ok {
		return nil, []diag.Diag{d}
	}

	if output.SameDir(dest, filepath.Dir(pkg.GoFiles[0])) {
		return nil, []diag.Diag{diag.Errorf(token.Position{Filename: dest},
			"the destination is %s itself, which the fixtures import", pkg.PkgPath).
			WithHints("name a directory of their own with -destination")}
	}
	destName, err := output.PackageName(dest, outputFile)
	if err != nil {
		return nil, []diag.Diag{diag.Errorf(token.Position{Filename: dest}, "%v", err)}
	}

	path := filepath.Join(dest, outputFile)
	if d, ok := foreign(path, pkg); ok {
		return nil, []diag.Diag{d}
	}
	hand := handWritten(set, dest)
	found, diags := targets(set.Fset, scan.AllStructs([]*packages.Package{pkg}), cfg.exclude)
	diags = append(diags, unplaced(set.Fset, scan.Types([]*packages.Package{pkg}))...)
	diags = append(diags, directive.Loose(directive.Lines(set.Fset, pkg.Syntax), directiveKey)...)
	if diag.HasErrors(diags) {
		return nil, packages.Stopped(deferred, diags, set.Pending(fixtures(dest, found, hand)))
	}
	var changes []output.Change
	if len(found) == 0 {
		// Nothing to stop for: a go:generate line left behind does no harm.
		diags = append(diags, diag.Warningf(token.Position{}, "no struct of %s gets a fixture", pkg.PkgPath))
		if generated(path) {
			changes = []output.Change{{Path: path, Remove: true}}
		}
	} else {
		f, ds := emit(set.Fset, found, importPath(dest), destName, output.DeclaredNames(dest, outputFile, destName), hand)
		if diags = append(diags, ds...); diag.HasErrors(diags) {
			return nil, packages.Stopped(deferred, diags, set.Pending(fixtures(dest, found, hand)))
		}
		src, err := f.Bytes()
		if err != nil {
			return nil, append(diags, diag.Errorf(token.Position{Filename: path}, "generated code does not parse: %v", err))
		}
		changes = []output.Change{{Path: path, Data: src}}
		diags = append(diags, output.MissingRequires(path, f.ModuleImports())...)
	}
	if rds := packages.Recheck(set, changes); len(rds) > 0 {
		return nil, slices.Concat(diags, rds, set.SetAsideWarnings())
	}
	return changes, diags
}

// destination returns the directory a run in dir writes into.
func destination(dir string, cfg config) string {
	if filepath.IsAbs(cfg.destination) {
		return filepath.Clean(cfg.destination)
	}
	return filepath.Join(dir, cfg.destination)
}

// toLoad returns what a run writing into dest loads. Code in the destination
// calls the fixtures, so it is loaded too, from the same load as the source,
// for its types to be the source's.
func toLoad(dest string, patterns []string) []string {
	if !hasCode(dest) {
		return patterns
	}
	if len(patterns) > 0 {
		return []string{patterns[0], dest}
	}
	return []string{".", dest}
}

// hasCode reports whether dir holds Go files other than the output and tests.
func hasCode(dir string) bool {
	names, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	return slices.ContainsFunc(names, func(name string) bool {
		return filepath.Base(name) != outputFile && !strings.HasSuffix(name, "_test.go")
	})
}

// fixtures returns the full names of the fixtures of found, which the run
// writes into the package in dest, but for those hand declares there.
func fixtures(dest string, found []scan.Struct, hand map[string]*types.Func) map[string]bool {
	names := make(map[string]bool, len(found))
	for _, s := range found {
		if hand[s.Name] == nil {
			names[importPath(dest)+"."+s.Name] = true
		}
	}
	return names
}

// handWritten returns the functions the package of set in dest declares by
// hand, by name, among which a fixture written by hand is: those of a file
// carrying the generated-code header, the output of an earlier run among them,
// are not.
func handWritten(set packages.Set, dest string) map[string]*types.Func {
	hand := make(map[string]*types.Func)
	for _, p := range set.Packages {
		if len(p.GoFiles) == 0 || p.Types == nil || !output.SameDir(filepath.Dir(p.GoFiles[0]), dest) {
			continue
		}
		generated := packages.GeneratedFiles(p)
		for _, name := range p.Types.Scope().Names() {
			fn, ok := p.Types.Scope().Lookup(name).(*types.Func)
			if ok && !generated[p.Fset.Position(fn.Pos()).Filename] {
				hand[name] = fn
			}
		}
	}
	return hand
}

// source returns the one package of set that the fixtures are made from, or
// says why there is none. The package of set in dest, loaded for the code
// there, is not it.
func source(set packages.Set, patterns []string, dest string) (*packages.Package, diag.Diag, bool) {
	pkgs := set.Packages
	if len(pkgs) > 1 {
		pkgs = slices.DeleteFunc(slices.Clone(pkgs), func(p *packages.Package) bool {
			return len(p.GoFiles) > 0 && output.SameDir(dest, filepath.Dir(p.GoFiles[0]))
		})
	}
	if len(pkgs) != 1 {
		pattern := "."
		if len(patterns) > 0 {
			pattern = patterns[0]
		}
		return nil, diag.Errorf(token.Position{}, "%s matches %d packages; %s reads one",
			pattern, len(pkgs), generator), false
	}
	switch pkg := pkgs[0]; {
	case pkg.PkgPath == "command-line-arguments":
		// The go command names a package made of the files it is given so,
		// which nothing can import.
		return nil, diag.Errorf(token.Position{}, "name a package, not the files of one"), false
	case pkg.Name == "main":
		return nil, diag.Errorf(token.Position{}, "%s is a command, which the fixtures cannot import", pkg.PkgPath), false
	case len(pkg.GoFiles) == 0:
		// Only tests, or files the build tags exclude: nothing to read, and no
		// directory to keep the destination out of.
		return nil, diag.Errorf(token.Position{}, "%s has no Go files to generate from", pkg.PkgPath), false
	default:
		return pkg, diag.Diag{}, true
	}
}

// generated reports whether path is a file carrying the generated-code marker,
// which a run that has nothing to write removes. A hand-written file of the
// same name is left alone.
func generated(path string) bool {
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.PackageClauseOnly|parser.ParseComments)
	return err == nil && ast.IsGenerated(f)
}

// foreign tells whether path is a file generated with the fixtures of another
// package than pkg, which a run for pkg would overwrite. A fixture is told by
// its signature: it is named after the struct it returns.
func foreign(path string, pkg *packages.Package) (diag.Diag, bool) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil || !ast.IsGenerated(f) {
		return diag.Diag{}, false
	}
	seen := make(map[string]bool)
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
			continue
		}
		sel, ok := fn.Type.Results.List[0].Type.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != fn.Name.Name {
			continue
		}
		q, ok := sel.X.(*ast.Ident)
		if !ok || seen[q.Name] {
			continue
		}
		seen[q.Name] = true
		if !names(f, filepath.Dir(path), q.Name, pkg) {
			return diag.Errorf(fset.Position(fn.Name.Pos()),
				"%s is the fixture of %s.%s, of another package, which a run for %s would overwrite",
				fn.Name.Name, q.Name, sel.Sel.Name, pkg.PkgPath).
				WithHints("give each package a -destination of its own",
					"remove the file if that package no longer generates here"), true
		}
	}
	return diag.Diag{}, false
}

// names reports whether q, a qualifier in f, the file in dir, names pkg. An
// import without a name of its own goes by the name its package had when f was
// written, which pkg may have changed since: q names pkg then unless another
// such import goes by q.
func names(f *ast.File, dir, q string, pkg *packages.Package) bool {
	var (
		imported bool
		others   []string
	)
	for _, spec := range f.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		switch {
		case spec.Name != nil:
			if spec.Name.Name == q {
				return path == pkg.PkgPath
			}
		case path == pkg.PkgPath:
			imported = true
		default:
			others = append(others, path)
		}
	}
	if !imported || q == pkg.Name {
		return imported
	}
	ctxt := build.Default
	ctxt.Dir = dir
	return !slices.ContainsFunc(others, func(path string) bool {
		p, err := ctxt.Import(path, dir, 0)
		return err == nil && p.Name == q
	})
}

// importPath returns the import path of the package in dir, from the module
// that the go.mod above it declares, or "" when it finds none. dir need not
// exist yet.
func importPath(dir string) string {
	for mod := dir; ; mod = filepath.Dir(mod) {
		data, err := os.ReadFile(filepath.Join(mod, "go.mod")) //nolint:gosec // the go.mod of the destination
		if err == nil {
			rel, err := filepath.Rel(mod, dir)
			if path := modfile.ModulePath(data); path != "" && err == nil {
				return strings.TrimSuffix(path+"/"+filepath.ToSlash(rel), "/.")
			}
			return ""
		}
		if filepath.Dir(mod) == mod {
			return ""
		}
	}
}
