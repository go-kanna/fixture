package gen

import (
	"cmp"
	"fmt"
	"go/token"
	"go/types"
	"slices"

	"github.com/go-kanna/internal/diag"
	"github.com/go-kanna/internal/directive"
	"github.com/go-kanna/internal/scan"
	"github.com/go-kanna/internal/suggest"
)

// directiveKey spells the directive //kanna:ignore.
const directiveKey = "ignore"

// ignoreHint tells how to leave a struct out without a warning.
const ignoreHint = "mark it //kanna:" + directiveKey + " to leave it out without a warning"

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

		d, ds := directive.Find(fset, s.Doc, directiveKey)
		diags = append(diags, ds...)
		switch {
		case d.Found:
			if d.Args != "" {
				diags = append(diags, diag.Errorf(d.Pos, "//kanna:%s takes no arguments, got %q", directiveKey, d.Args))
			}
		case slices.Contains(exclude, s.Name):
		case s.Named.TypeParams().Len() > 0:
			diags = append(diags, diag.Warningf(s.Pos,
				"no fixture for generic struct %s, whose type arguments a fixture cannot pick", s.Name).
				WithHints(ignoreHint))
		default:
			if f, ok := lockedField(s); ok {
				diags = append(diags, diag.Warningf(s.Pos,
					"no fixture for %s, whose field %s (%s) must not be copied, as a fixture returns a copy",
					s.Name, f.Name, types.TypeString(f.Type, qualifierFor(s.Named.Obj().Pkg()))).
					WithHints(ignoreHint))
				continue
			}
			found = append(found, s)
		}
	}

	slices.SortFunc(found, func(a, b scan.Struct) int { return cmp.Compare(a.Name, b.Name) })
	return found, append(diags, unknownExcludes(exclude, exported)...)
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
