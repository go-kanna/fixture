package gen_test

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-kanna/internal/deps"
)

func TestDeps(t *testing.T) {
	t.Parallel()

	dir := writeModule(t, map[string]string{
		"p/p.go": "package p\n\ntype User struct{ Name string }\n",
	})

	code, stdout, stderr := run(t, filepath.Join(dir, "p"), "-deps", "-destination", "../q/fx")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var r deps.Report
	if err := json.Unmarshal([]byte(stdout), &r); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(r.Packages, []string{"example.com/app/p"}) {
		t.Errorf("Packages = %q", r.Packages)
	}
	if len(r.Writes) != 1 || !strings.HasSuffix(r.Writes[0], filepath.Join("q", "fx", "fixture_gen.go")) {
		t.Errorf("Writes = %q", r.Writes)
	}
	if !slices.Equal(r.Declares, []string{"example.com/app/q/fx.User"}) {
		t.Errorf("Declares = %q", r.Declares)
	}
}

// Code in the destination is loaded by a run, and so is what it imports.
func TestDeps_CodeInTheDestination(t *testing.T) {
	t.Parallel()

	dir := writeModule(t, map[string]string{
		"p/p.go":           "package p\n\ntype User struct{ Name string }\n",
		"p/fixture/own.go": "package fixture\n",
	})

	code, stdout, stderr := run(t, filepath.Join(dir, "p"), "-deps")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var r deps.Report
	if err := json.Unmarshal([]byte(stdout), &r); err != nil {
		t.Fatal(err)
	}

	if want := []string{"example.com/app/p", "example.com/app/p/fixture"}; !slices.Equal(r.Packages, want) {
		t.Errorf("Packages = %q, want %q", r.Packages, want)
	}
}
