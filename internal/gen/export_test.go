package gen

import (
	"go/constant"
	"go/token"
	"go/types"
	"reflect"
	"strconv"
	"strings"

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

// Wants reads tag, a struct tag as written between backquotes, for a field of
// type t, and spells each level as a line, such as "required len=..50" or
// "low=18 high=130".
func Wants(tag string, t types.Type) []string {
	wants := wantsOf(reflect.StructTag(tag), t)
	if wants == nil {
		return nil
	}

	lines := make([]string, len(wants))
	for i, w := range wants {
		var parts []string
		if w.required {
			parts = append(parts, "required")
		}
		if w.minLen > 0 || w.maxLen >= 0 {
			hi := ""
			if w.maxLen >= 0 {
				hi = strconv.Itoa(w.maxLen)
			}
			parts = append(parts, "len="+strconv.Itoa(w.minLen)+".."+hi)
		}
		if len(w.choices) > 0 {
			parts = append(parts, "oneof="+strings.Join(w.choices, "|"))
		}
		for _, n := range []struct {
			name string
			v    constant.Value
		}{{"low", w.low}, {"high", w.high}, {"exact", w.exact}} {
			if n.v != nil {
				parts = append(parts, n.name+"="+n.v.String())
			}
		}
		if w.field != "" {
			parts = append(parts, "eqfield="+w.field)
		}
		if w.format != "" {
			parts = append(parts, w.format)
		}
		if w.pattern != "" {
			parts = append(parts, "pattern="+w.pattern)
		}
		lines[i] = strings.Join(parts, " ")
	}
	return lines
}
