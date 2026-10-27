package gen

import (
	"go/token"

	"github.com/go-kanna/internal/packages"
	"github.com/go-kanna/internal/scan"
)

// Targets finds the targets in pkgs, leaving out the structs exclude names, and
// spells each diagnostic as diag.Diag.String does.
func Targets(fset *token.FileSet, pkgs []*packages.Package, exclude []string) ([]string, []string) {
	found, diags := targets(fset, scan.Structs(pkgs), exclude)

	names := make([]string, len(found))
	for i, s := range found {
		names[i] = s.Name
	}
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.String()
	}
	return names, msgs
}
