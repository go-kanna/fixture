package gen_test

import (
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-kanna/internal/output"
	"github.com/go-kanna/internal/packages"
	"github.com/go-kanna/internal/pkgtest"

	"github.com/go-kanna/fixture/internal/gen"
)

// TestEmit compares the fixtures of testdata/emit/model with the golden file,
// which is part of the package testdata/emit/fixture that TestGenerated calls.
// To accept a change in the output:
//
//	go test ./internal/gen -run TestEmit -update
func TestEmit(t *testing.T) {
	t.Parallel()

	// The fixtures written by hand are loaded too, without the output of the
	// last run, as a run loads them.
	set, err := packages.Load([]string{"./testdata/emit/model", "./testdata/emit/fixture"},
		packages.Config{Regenerated: []string{"fixture_gen.go"}})
	if err != nil || len(set.Diags) > 0 {
		t.Fatalf("load: %v %v", err, set.Diags)
	}
	dest, err := filepath.Abs("testdata/emit/fixture")
	if err != nil {
		t.Fatal(err)
	}

	byName := make(map[string]*packages.Package, len(set.Packages))
	for _, p := range set.Packages {
		byName[p.Name] = p
	}
	got, diags := gen.Emit(set.Fset, []*packages.Package{byName["model"]},
		"github.com/go-kanna/fixture/internal/gen/testdata/emit/fixture", "fixture",
		output.DeclaredNames(dest, "fixture_gen.go", "fixture"), byName["fixture"])
	if len(diags) > 0 {
		t.Fatalf("diagnostics = %q, want none\n%s", diags, got)
	}
	pkgtest.Golden(t, "testdata/emit/fixture/fixture_gen.go", got)
}

func TestEmit_DeclaredNames(t *testing.T) {
	t.Parallel()

	pkg := pkgtest.Load(t, map[string]string{"test.go": `package model

type User struct {
	Name string ` + "`fake:\"{firstname}\"`" + `
	Slug string
}
`})
	declared := map[string]token.Position{
		"mustGenerate": {Filename: "/dest/helpers.go", Line: 3, Column: 6},
		"seqUser":      {Filename: "/dest/helpers.go", Line: 5, Column: 5},
		"fmt":          {Filename: "/dest/helpers.go", Line: 7, Column: 5},
		"x":            {Filename: "/dest/helpers.go", Line: 9, Column: 5},
	}

	got, diags := gen.Emit(pkg.Fset, []*packages.Package{pkg}, "example.com/fixture", "fixture", declared, nil)
	if len(diags) > 0 {
		t.Fatalf("diagnostics = %q, want none\n%s", diags, got)
	}
	for _, want := range []string{
		`fmt2 "fmt"`,
		"var seqUser2 atomic.Int64",
		"x2 := model.User{",
		`Name: mustGenerate2(f, "{firstname}")`,
		`Slug: fmt2.Sprintf("slug-%d", n)`,
		"func mustGenerate2(f *gofakeit.Faker, template string) string {",
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("output does not contain %q:\n%s", want, got)
		}
	}
}

// A fixture is named after its struct, so a name the destination declares, but
// not as a fixture of the struct, is not one it can keep clear of.
func TestEmit_Clash(t *testing.T) {
	t.Parallel()

	pkg := pkgtest.Load(t, map[string]string{"test.go": `package model

type User struct{}

type Article struct{}
`})
	declared := map[string]token.Position{"User": {Filename: "/dest/user.go", Line: 3, Column: 6}}

	_, diags := gen.Emit(pkg.Fset, []*packages.Package{pkg}, "example.com/fixture", "fixture", declared, nil)
	want := []string{"/dest/user.go:3:6: error: the fixture of User would declare User again\n" +
		"  declare it as func User(setters ...func(*model.User)) model.User for it to be the fixture, " +
		"rename it, or generate the fixtures into a directory of their own"}
	if !slices.Equal(diags, want) {
		t.Errorf("diagnostics = %q, want %q", diags, want)
	}
}
