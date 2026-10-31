package gen

import (
	"maps"
	"path/filepath"
	"slices"

	"github.com/go-kanna/internal/deps"
	"github.com/go-kanna/internal/packages"
	"github.com/go-kanna/internal/scan"
)

// reportDeps tells what a run in dir reads and writes.
func reportDeps(dir string, cfg config, patterns []string) (deps.Report, error) {
	dest := destination(dir, cfg)
	set, err := packages.Load(toLoad(dest, patterns), packages.Config{Dir: dir, Regenerated: []string{outputFile}})
	if err != nil {
		return deps.Report{}, err //nolint:wrapcheck // it says what could not be loaded
	}

	r := deps.Report{Packages: set.Paths()}
	pkg, _, ok := source(set, patterns, dest)
	if !ok {
		return r, nil // which the run reports
	}
	found, _ := targets(set.Fset, scan.AllStructs([]*packages.Package{pkg}), cfg.exclude)
	if len(found) > 0 {
		r.Writes = []string{filepath.Join(dest, outputFile)}
		r.Declares = slices.Collect(maps.Keys(fixtures(dest, found)))
	}
	return r, nil
}
