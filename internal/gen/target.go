package gen

import (
	"cmp"
	"fmt"
	"go/token"
	"go/types"
	"slices"
	"strconv"

	"github.com/go-kanna/internal/diag"
	"github.com/go-kanna/internal/directive"
	"github.com/go-kanna/internal/scan"
	"github.com/go-kanna/internal/suggest"
)

// directiveKey spells the directive //kanna:fixture, whose argument says what
// to do with the struct.
const directiveKey = "fixture"

// skip is the argument of //kanna:fixture that leaves a struct out.
const skip = "skip"

// valueKey spells //kanna:value, which marks a value type of kanna-value. Its
// one field is unexported, so a fixture could not set it, and what holds it is
// made through its constructor rather than field by field.
const valueKey = "value"

// skipHint tells how to leave a struct out without a warning.
const skipHint = "mark it //kanna:" + directiveKey + " " + skip + " to leave it out without a warning"

// targets returns the structs that get a fixture, sorted by name, and says why
// the others a reader would expect one for do not. exclude names structs to
// leave out, as -exclude lists them.
//
// Every exported struct is a target unless something rules it out, so that a
// struct added to the model has a fixture without anyone asking for one. Sorting
// by name keeps the output still when declarations move between files.
func targets(fset *token.FileSet, structs []scan.Struct, exclude []string) ([]scan.Struct, []diag.Diag) {
	var (
		found    []scan.Struct
		diags    []diag.Diag
		exported []string
	)
	for _, s := range structs {
		// The fixtures live in another package, which cannot name the type.
		if !token.IsExported(s.Name) {
			continue
		}
		exported = append(exported, s.Name)

		// kanna-value reports what is wrong with the line itself.
		if v, _ := directive.Find(fset, s.Doc, valueKey); v.Found {
			continue
		}

		d, ds := directive.Find(fset, s.Doc, directiveKey)
		diags = append(diags, ds...)
		switch {
		case d.Found:
			if d.Args != skip {
				diags = append(diags, badArgs(d))
			}
		case slices.Contains(exclude, s.Name):
		case s.Named.TypeParams().Len() > 0:
			diags = append(diags, diag.Warningf(s.Pos,
				"no fixture for generic struct %s, whose type arguments a fixture cannot pick", s.Name).
				WithHints(skipHint))
		default:
			if f, ok := lockedField(s); ok {
				diags = append(diags, diag.Warningf(s.Pos,
					"no fixture for %s, whose field %s (%s) must not be copied, as a fixture returns a copy",
					s.Name, f.Name, types.TypeString(f.Type, qualifierFor(s.Named.Obj().Pkg()))).
					WithHints(skipHint))
				continue
			}
			found = append(found, s)
		}
	}

	slices.SortFunc(found, func(a, b scan.Struct) int { return cmp.Compare(a.Name, b.Name) })
	return found, append(diags, unknownExcludes(exclude, exported)...)
}

// badArgs reports d, a //kanna:fixture whose arguments are not skip, the one
// argument it takes.
func badArgs(d directive.Directive) diag.Diag {
	got := "nothing"
	if d.Args != "" {
		got = strconv.Quote(d.Args)
	}
	e := diag.Errorf(d.Pos, "//kanna:%s takes %s, got %s", directiveKey, skip, got)
	if want, ok := suggest.Closest(d.Args, []string{skip}); ok {
		e = e.WithHints(fmt.Sprintf("did you mean %q?", "//kanna:"+directiveKey+" "+want))
	}
	return e
}

// unknownExcludes warns about each name in exclude that is not among exported,
// which would leave in place the fixture it was meant to drop.
func unknownExcludes(exclude, exported []string) []diag.Diag {
	var diags []diag.Diag
	for _, name := range exclude {
		if slices.Contains(exported, name) {
			continue
		}
		d := diag.Warningf(token.Position{}, "-exclude %s names no exported struct of the package", name)
		if want, ok := suggest.Closest(name, exported); ok {
			d = d.WithHints(fmt.Sprintf("did you mean %q?", want))
		}
		diags = append(diags, d)
	}
	return diags
}

// locker is sync.Locker, which marks the types go vet's copylocks check forbids
// copying.
var locker = types.NewInterfaceType([]*types.Func{
	types.NewFunc(token.NoPos, nil, "Lock", types.NewSignatureType(nil, nil, nil, nil, nil, false)),
	types.NewFunc(token.NoPos, nil, "Unlock", types.NewSignatureType(nil, nil, nil, nil, nil, false)),
}, nil).Complete()

// lockedField returns the first field of s that go vet's copylocks check
// forbids copying.
func lockedField(s scan.Struct) (scan.Field, bool) {
	for _, f := range s.Fields {
		if holdsLock(f.Type) {
			return f, true
		}
	}
	return scan.Field{}, false
}

// holdsLock reports whether t holds, by value, a type that must not be copied.
// As in go vet, that is a type whose pointer is a locker while the type itself
// is not: sync.Mutex, or a noCopy marker such as the one in atomic.Int64.
func holdsLock(t types.Type) bool {
	if types.Implements(types.NewPointer(t), locker) && !types.Implements(t, locker) {
		return true
	}
	switch u := t.Underlying().(type) {
	case *types.Struct:
		for f := range u.Fields() {
			if holdsLock(f.Type()) {
				return true
			}
		}
	case *types.Array:
		return holdsLock(u.Elem())
	}
	return false
}

// qualifierFor names the types of other packages by package name, as Go source
// in pkg would, and those of pkg by their name alone.
func qualifierFor(pkg *types.Package) types.Qualifier {
	return func(other *types.Package) string {
		if other == pkg {
			return ""
		}
		return other.Name()
	}
}
