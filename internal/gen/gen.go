package gen

import (
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/go-kanna/internal/cli"
	"github.com/go-kanna/internal/diag"
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
	pkg         string
	exclude     []string
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
			fs.StringVar(&cfg.pkg, "package", "",
				"call the package `name` when the destination has no Go files yet (default: the directory's name)")
			fs.Func("exclude", "skip the structs `names` lists, separated by commas", func(s string) error {
				for name := range strings.SplitSeq(s, ",") {
					if name = strings.TrimSpace(name); name != "" {
						cfg.exclude = append(cfg.exclude, name)
					}
				}
				return nil
			})
		},
		Run: run,
	}
}

func run(dir string, cfg config, patterns []string) ([]output.Change, []diag.Diag) {
	if len(patterns) > 1 {
		return nil, []diag.Diag{diag.Errorf(token.Position{}, "%s reads one package, got %d", generator, len(patterns))}
	}
	dest := filepath.Join(dir, cfg.destination)
	if filepath.IsAbs(cfg.destination) {
		dest = filepath.Clean(cfg.destination)
	}

	// Code in the destination calls the fixtures, so it is loaded too, from
	// the same load as the source, for its types to be the source's.
	load := patterns
	if hasCode(dest) {
		load = []string{".", dest}
		if len(patterns) > 0 {
			load[0] = patterns[0]
		}
	}
	set, err := packages.Load(load, packages.Config{Dir: dir, Regenerated: []string{outputFile}})
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
	destName, err := output.PackageName(dest, outputFile, cfg.pkg)
	if err != nil {
		return nil, []diag.Diag{diag.Errorf(token.Position{Filename: dest}, "%v", err)}
	}

	path := filepath.Join(dest, outputFile)
	found, diags := targets(set.Fset, scan.AllStructs([]*packages.Package{pkg}), cfg.exclude)
	if diag.HasErrors(diags) {
		return nil, packages.Stopped(deferred, diags, set.Pending(fixtures(dest, found)))
	}
	var changes []output.Change
	if len(found) == 0 {
		// Nothing to stop for: a go:generate line left behind does no harm.
		diags = append(diags, diag.Warningf(token.Position{}, "no struct of %s gets a fixture", pkg.PkgPath))
		if generated(path) {
			changes = []output.Change{{Path: path, Remove: true}}
		}
	} else {
		f, ds := emit(found, importPath(dest), destName, output.DeclaredNames(dest, outputFile, destName))
		if diags = append(diags, ds...); diag.HasErrors(diags) {
			return nil, packages.Stopped(deferred, diags, set.Pending(fixtures(dest, found)))
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

// hasCode reports whether dir holds Go files other than the output and tests.
func hasCode(dir string) bool {
	names, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	return slices.ContainsFunc(names, func(name string) bool {
		return filepath.Base(name) != outputFile && !strings.HasSuffix(name, "_test.go")
	})
}

// fixtures returns the full names of the fixtures of found, which the run
// writes into the package in dest.
func fixtures(dest string, found []scan.Struct) map[string]bool {
	names := make(map[string]bool, len(found))
	for _, s := range found {
		names[importPath(dest)+"."+s.Name] = true
	}
	return names
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
