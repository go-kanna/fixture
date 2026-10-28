package gen

import (
	"errors"
	"go/constant"
	"go/token"
	"go/types"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-kanna/internal/gofile"
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

// Fake reads tag, a struct tag as written between backquotes, for a field of
// type t. It spells what the fake tag asks for, such as "skip",
// "fixed=\"123-4567\"", "Number(18, 65)", or "template={firstname}", or "" for
// no fake tag, and the error, with its hint on a line of its own.
func Fake(tag string, t types.Type) (string, string) {
	v, ok, err := fakeOf(reflect.StructTag(tag), t)
	if err != nil {
		msg := err.Error()
		if e, ok := errors.AsType[*tagError](err); ok && e.hint != "" {
			msg += "\n  " + e.hint
		}
		return "", msg
	}
	switch {
	case !ok:
		return "", ""
	case v.skip:
		return "skip", ""
	case v.fixed != "":
		return "fixed=" + v.fixed, ""
	case v.method != "":
		return v.method + "(" + strings.Join(v.args, ", ") + ")", ""
	default:
		return "template=" + v.template, ""
	}
}

// Fill finds the targets in pkgs and works out the fixture of each, spelling
// it, by the name of its struct, as a line per field it sets, such as
// "Name: fmt.Sprintf(\"name-%d\", n)", then "Confirm = Password" for each copy,
// then "uses n" and "uses f" when it does. Diagnostics are spelled as
// diag.Diag.String does.
func Fill(fset *token.FileSet, pkgs []*packages.Package) (map[string][]string, []string) {
	found, diags := targets(fset, scan.Structs(pkgs), nil)
	fl := &filler{
		imports:  gofile.New("kanna-fixture", "example.com/fixture", "fixture"),
		targets:  map[*types.TypeName]bool{},
		generate: "mustGenerate",
		orZero:   "orZero",
		n:        "n",
		f:        "f",
	}
	for _, s := range found {
		fl.targets[s.Named.Obj()] = true
	}

	spelled := make(map[string][]string, len(found))
	for _, s := range found {
		filled, ds := fl.fill(s)
		diags = append(diags, ds...)
		lines := []string{}
		for _, a := range filled.fields {
			lines = append(lines, a.field+": "+a.value)
		}
		for _, a := range filled.copies {
			lines = append(lines, a.field+" = "+a.value)
		}
		if filled.usesN {
			lines = append(lines, "uses n")
		}
		if filled.usesF {
			lines = append(lines, "uses f")
		}
		spelled[s.Name] = lines
	}
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.String()
	}
	return spelled, msgs
}

// FillField works out the value of a field of type t named name, with struct
// tag tag, in a struct of its own.
func FillField(name string, t types.Type, tag string) string {
	owner := types.NewNamed(types.NewTypeName(0, types.NewPackage("example.com/model", "model"), "Owner", nil),
		types.NewStruct(nil, nil), nil)
	s := scan.Struct{
		Name:   "Owner",
		Named:  owner,
		Fields: []scan.Field{{Name: name, Type: t, Tag: reflect.StructTag(tag), Exported: true}},
	}
	fl := &filler{
		imports:  gofile.New("kanna-fixture", "example.com/fixture", "fixture"),
		generate: "mustGenerate",
		orZero:   "orZero",
		n:        "n",
		f:        "f",
	}
	filled, _ := fl.fill(s)
	if len(filled.fields) == 0 {
		return ""
	}
	return filled.fields[0].value
}

// Emit finds the targets in pkgs and renders their fixtures into a file of the
// package destName at destPath, whose other files declare declared. It spells
// each diagnostic as diag.Diag.String does.
func Emit(fset *token.FileSet, pkgs []*packages.Package, destPath, destName string,
	declared map[string]token.Position,
) ([]byte, []string) {
	found, diags := targets(fset, scan.Structs(pkgs), nil)
	f, ds := emit(found, destPath, destName, declared)

	var msgs []string
	for _, d := range append(diags, ds...) {
		msgs = append(msgs, d.String())
	}
	src, err := f.Bytes()
	if err != nil {
		msgs = append(msgs, err.Error())
	}
	return src, msgs
}

// ImportPath is importPath.
var ImportPath = importPath
