package gen

import (
	"fmt"
	"go/token"
	"go/types"
	"hash/fnv"
	"maps"
	"slices"

	"github.com/go-kanna/internal/diag"
	"github.com/go-kanna/internal/gofile"
	"github.com/go-kanna/internal/scan"
)

// generator names the tool in the header of the file it writes.
const generator = "kanna-fixture"

// gofakeitPath is the import path of the faker the generated code calls for a
// fake tag or a pattern.
const gofakeitPath = "github.com/brianvoe/gofakeit/v7"

// emit renders the fixtures of targets, the structs of one package that get
// one, into a file of the package destName at destPath. declared are the
// top-level names that package declares besides, with where.
//
// A fixture is named after its struct, so a name the package declares already
// is an error; the other names the file introduces keep clear of them.
func emit(
	targets []scan.Struct, destPath, destName string, declared map[string]token.Position,
) (*gofile.File, []diag.Diag) {
	var diags []diag.Diag
	for _, s := range targets {
		if pos, ok := declared[s.Name]; ok {
			diags = append(diags, diag.Errorf(pos, "the fixture of %s would declare %s again", s.Name, s.Name).
				WithHints("rename this declaration, or generate the fixtures into a directory of their own"))
		}
	}

	f := gofile.New(generator, destPath, destName)
	f.Reserve(slices.Sorted(maps.Keys(declared))...)
	fl := &filler{imports: f, targets: make(map[*types.TypeName]bool, len(targets))}
	for _, s := range targets {
		f.Reserve(s.Name)
		fl.targets[s.Named.Obj()] = true
	}

	// gofile hands out the rest of the top-level names before any import or
	// local, from a scope of their own.
	top := f.Scope()
	fl.generate, fl.orZero = top.Name("mustGenerate"), top.Name("orZero")
	counters := make([]string, len(targets))
	for i, s := range targets {
		counters[i] = top.Name("seq" + s.Name)
	}

	var generates, usesOrZero bool
	for i, s := range targets {
		filled, ds := fl.fixture(f, s, counters[i])
		diags = append(diags, ds...)
		generates = generates || filled.generates
		usesOrZero = usesOrZero || filled.usesOrZero
	}
	if generates {
		writeGenerate(f, fl.generate)
	}
	if usesOrZero {
		writeOrZero(f, fl.orZero)
	}
	return f, diags
}

// fixture writes the fixture of s, numbered by the counter of that name.
func (fl *filler) fixture(f *gofile.File, s scan.Struct, counter string) (filled, []diag.Diag) {
	scope := f.Scope()
	x, setters, set := scope.Name("x"), scope.Name("setters"), scope.Name("set")
	fl.n, fl.f = scope.Name("n"), scope.Name("f")

	filled, diags := fl.fill(s)
	typ := types.TypeString(s.Named, f.Qualifier)

	// The faker is seeded with the number, too.
	numbered := filled.usesN || filled.usesF
	if numbered {
		fmt.Fprintf(f, "var %s %s.Int64\n\n", counter, f.Import("sync/atomic", "atomic"))
	}
	fmt.Fprintf(f, "// %s returns a new %s with every field it can fill set, then\n", s.Name, typ)
	fmt.Fprintf(f, "// applies setters in order.\n")
	fmt.Fprintf(f, "func %s(%s ...func(*%s)) %s {\n", s.Name, setters, typ, typ)
	if numbered {
		fmt.Fprintf(f, "%s := %s.Add(1)\n", fl.n, counter)
	}
	if filled.usesF {
		fmt.Fprintf(f, "%s := %s.NewFaker(%s.NewPCG(uint64(%s), 0x%016x), false)\n",
			fl.f, f.Import(gofakeitPath, "gofakeit"), f.Import("math/rand/v2", "rand"), fl.n, seed(s))
	}
	if len(filled.fields) == 0 {
		fmt.Fprintf(f, "%s := %s{}\n", x, typ)
	} else {
		fmt.Fprintf(f, "%s := %s{\n", x, typ)
		for _, a := range filled.fields {
			fmt.Fprintf(f, "%s: %s,\n", a.field, a.value)
		}
		fmt.Fprint(f, "}\n")
	}
	for _, a := range filled.copies {
		fmt.Fprintf(f, "%[1]s.%[2]s = %[1]s.%[3]s\n", x, a.field, a.value)
	}
	fmt.Fprintf(f, "for _, %[1]s := range %[2]s {\n%[1]s(&%[3]s)\n}\n", set, setters, x)
	fmt.Fprintf(f, "return %s\n}\n\n", x)
	return filled, diags
}

// seed returns what tells the faker of the fixture of s apart from those of
// other structs: a hash of the import path and name of s, so that it stays the
// same from one run of the generator to the next.
func seed(s scan.Struct) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s.PkgPath + "." + s.Name)) // a hash takes every write
	return h.Sum64()
}

// writeOrZero writes the helper named name, which keeps what the constructor
// of a value type returns where only one value can go, such as in a composite
// literal, or the zero value when the constructor rejects what it was given.
func writeOrZero(f *gofile.File, name string) {
	scope := f.Scope()
	v, err := scope.Name("v"), scope.Name("err")
	fmt.Fprintf(f, "func %s[T any](%s T, %s error) T {\n", name, v, err)
	fmt.Fprintf(f, "if %s != nil {\nvar zero T\nreturn zero\n}\n", err)
	fmt.Fprintf(f, "return %s\n}\n", v)
}

// writeGenerate writes the helper named name, which calls Faker.Generate where
// only one value can go, such as in a composite literal. Generation has checked
// every template already, so it does not fail.
func writeGenerate(f *gofile.File, name string) {
	scope := f.Scope()
	faker, template, s, err := scope.Name("f"), scope.Name("template"), scope.Name("s"), scope.Name("err")
	gofakeit := f.Import(gofakeitPath, "gofakeit")
	fmt.Fprintf(f, "func %s(%s *%s.Faker, %s string) string {\n", name, faker, gofakeit, template)
	fmt.Fprintf(f, "%s, %s := %s.Generate(%s)\n", s, err, faker, template)
	fmt.Fprintf(f, "if %s != nil {\npanic(%[1]s)\n}\n", err)
	fmt.Fprintf(f, "return %s\n}\n", s)
}
