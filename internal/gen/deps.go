package gen

import (
	"maps"
	"path/filepath"
	"slices"

	"github.com/go-kanna/internal/deps"
	"github.com/go-kanna/internal/packages"
	"github.com/go-kanna/internal/scan"
)

// reportDeps tells what a run in dir reads and writes. A field whose type does
// not resolve, as before the generated code declaring it is written, does not
// keep its struct from the answer, since the run makes a fixture for it once
// that code is in place.
func reportDeps(dir string, cfg config, patterns []string) (deps.Report, error) {
	dest := destination(dir, cfg)
	set, err := packages.Load(toLoad(dest, patterns),
		packages.Config{Dir: dir, Tags: cfg.tags, Regenerated: []string{outputFile}})
	if err != nil {
		return deps.Report{}, err //nolint:wrapcheck // it says what could not be loaded
	}

	r := deps.Report{Packages: set.Paths()}
	pkg, _, ok := source(set, patterns, dest)
	if !ok {
		return r, nil // which the run reports
	}
	structs := scan.AllStructs([]*packages.Package{pkg})
	for i, s := range structs {
		structs[i].Fields = slices.DeleteFunc(slices.Clone(s.Fields), func(f scan.Field) bool {
			return packages.Invalid(f.Type)
		})
	}
	found, _ := targets(set.Fset, structs, cfg.exclude)
	if len(found) > 0 {
		r.Writes = []string{filepath.Join(dest, outputFile)}
		r.Declares = slices.Collect(maps.Keys(fixtures(dest, found, handWritten(set, dest))))
	}
	return r, nil
}
