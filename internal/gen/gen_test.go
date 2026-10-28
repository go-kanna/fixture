package gen_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-kanna/internal/cli"
	"github.com/go-kanna/internal/packages"

	"github.com/go-kanna/fixture/internal/gen"
)

func TestMain(m *testing.M) {
	// Before any test runs: t.Setenv would rule out t.Parallel. The go command
	// that loads packages must not resolve them through a workspace, which a
	// Makefile or mise may point GOWORK at.
	if err := os.Setenv("GOWORK", "off"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// writeModule writes files into a fresh module, example.com/app unless files
// hold a go.mod of their own, and returns its directory. The module requires
// gofakeit, which the generated code calls, and borrows the go.sum of this
// checkout so that nothing is fetched.
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()

	sum, err := os.ReadFile(filepath.Join("..", "..", "go.sum"))
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if _, ok := files["go.mod"]; !ok {
		files["go.mod"] = "module example.com/app\n\ngo 1.26.0\n\nrequire github.com/brianvoe/gofakeit/v7 v7.18.0\n"
	}
	files["go.sum"] = string(sum)
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		writeFile(t, path, content)
	}
	return dir
}

// run runs kanna-fixture in dir and returns its exit code, stdout, and stderr.
func run(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	code := gen.Command().Exec(cli.Env{Out: &stdout, Err: &stderr, Dir: dir}, args)
	return code, stdout.String(), stderr.String()
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path) //nolint:gosec // a file of the test's own module
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

const userSrc = `package model

type User struct {
	ID   int64
	Name string ` + "`fake:\"{firstname}\"`" + `
}
`

func TestCommand_Generates(t *testing.T) {
	t.Parallel()

	dir := writeModule(t, map[string]string{"model/model.go": userSrc})
	model := filepath.Join(dir, "model")

	code, stdout, stderr := run(t, model)
	if code != cli.ExitOK || stdout != "" || stderr != "" {
		t.Fatalf("run = %d, stdout %q, stderr %q; want success in silence", code, stdout, stderr)
	}
	got := readFile(t, filepath.Join(model, "fixture", "fixture_gen.go"))
	if !strings.Contains(got, "package fixture\n") ||
		!strings.Contains(got, "func User(setters ...func(*model.User)) model.User {") {
		t.Errorf("fixture/fixture_gen.go does not declare the fixture of User in package fixture:\n%s", got)
	}

	// The output compiles against the model and gofakeit.
	set, err := packages.Load([]string{"./..."}, packages.Config{Dir: dir})
	if err != nil || len(set.Diags) > 0 {
		t.Errorf("the generated package does not load: %v %v", err, set.Diags)
	}

	code, stdout, stderr = run(t, model, "-check")
	if code != cli.ExitOK || stdout != "" || stderr != "" {
		t.Errorf("-check right after generating = %d, stdout %q, stderr %q; want success in silence", code, stdout, stderr)
	}
	if code, stdout, _ := run(t, model, "-v"); code != cli.ExitOK || stdout != "unchanged fixture/fixture_gen.go\n" {
		t.Errorf("-v again = %d, stdout %q; want the file listed as unchanged", code, stdout)
	}
}

func TestCommand_Flags(t *testing.T) {
	t.Parallel()

	dir := writeModule(t, map[string]string{
		"model/model.go": userSrc + "\ntype Internal struct{ Token string }\n",
	})

	code, _, stderr := run(t, dir, "-destination", "testing/fx", "-package", "fx", "-exclude", "Internal", "./model")
	if code != cli.ExitOK || stderr != "" {
		t.Fatalf("run = %d, stderr %q", code, stderr)
	}
	got := readFile(t, filepath.Join(dir, "testing", "fx", "fixture_gen.go"))
	if !strings.Contains(got, "package fx\n") || strings.Contains(got, "func Internal(") {
		t.Errorf("testing/fx/fixture_gen.go is not package fx without Internal:\n%s", got)
	}

	// The destination's own Go files name its package, whatever -package says.
	writeFile(t, filepath.Join(dir, "testing", "fx", "doc.go"), "package fx\n")
	code, _, stderr = run(t, dir, "-destination", "testing/fx", "-package", "other", "./model")
	if code != cli.ExitFailure || !strings.Contains(stderr, "testing/fx: error:") {
		t.Errorf("a -package the destination contradicts = %d, stderr %q; want an error at the destination", code, stderr)
	}
}

func TestCommand_MissingRequires(t *testing.T) {
	t.Parallel()

	dir := writeModule(t, map[string]string{
		"go.mod":         "module example.com/app\n\ngo 1.26.0\n",
		"model/model.go": userSrc,
	})

	code, _, stderr := run(t, filepath.Join(dir, "model"))
	want := "fixture/fixture_gen.go: warning: imports github.com/brianvoe/gofakeit/v7, which go.mod does not require"
	if code != cli.ExitOK || !strings.Contains(stderr, want) {
		t.Errorf("run = %d, stderr %q; want success with %q", code, stderr, want)
	}
}

func TestCommand_Errors(t *testing.T) {
	t.Parallel()

	dir := writeModule(t, map[string]string{
		"model/model.go":  userSrc,
		"other/other.go":  "package other\n\ntype Order struct{ ID int64 }\n",
		"cmd/app/main.go": "package main\n\ntype Config struct{ Port int }\n\nfunc main() {}\n",
		"tagged/model.go": "package tagged\n\ntype User struct {\n\tAge int `fake:\"{numbr:1,10}\"`\n}\n",
		"tests/x_test.go": "package tests\n",
	})

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "two packages", args: []string{"./model", "./other"}, want: "kanna-fixture reads one package, got 2"},
		{name: "several packages", args: []string{"./..."}, want: "./... matches 5 packages; kanna-fixture reads one"},
		{name: "files", args: []string{"model/model.go"}, want: "name a package, not the files of one"},
		{
			name: "a command",
			args: []string{"./cmd/app"},
			want: "example.com/app/cmd/app is a command, which the fixtures cannot import",
		},
		{
			name: "the package itself",
			args: []string{"-destination", "model", "./model"},
			want: "model: error: the destination is example.com/app/model itself, which the fixtures import\n" +
				"  name a directory of their own with -destination",
		},
		{name: "a package that does not exist", args: []string{"./missing"}, want: "error:"},
		{name: "a misspelled template", args: []string{"./tagged"}, want: `did you mean "number"?`},
		{name: "only tests", args: []string{"./tests"}, want: "example.com/app/tests has no Go files to generate from"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			code, _, stderr := run(t, dir, tt.args...)
			if code != cli.ExitFailure || !strings.Contains(stderr, tt.want) {
				t.Errorf("run %q = %d, stderr %q; want a failure with %q", tt.args, code, stderr, tt.want)
			}
		})
	}
}

// A fixture is named after its struct, so it cannot keep clear of a
// declaration of that name in the destination.
func TestCommand_Clash(t *testing.T) {
	t.Parallel()

	dir := writeModule(t, map[string]string{
		"model/model.go":           userSrc,
		"model/fixture/helpers.go": "package fixture\n\nfunc User() {}\n",
	})

	code, _, stderr := run(t, filepath.Join(dir, "model"))
	if want := "fixture/helpers.go:3:6: error: the fixture of User would declare User again"; code != cli.ExitFailure ||
		!strings.Contains(stderr, want) {
		t.Errorf("run = %d, stderr %q; want an error with %q", code, stderr, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "model", "fixture", "fixture_gen.go")); !os.IsNotExist(err) {
		t.Errorf("fixture_gen.go was written: %v", err)
	}
}

func TestCommand_NoStruct(t *testing.T) {
	t.Parallel()

	dir := writeModule(t, map[string]string{"model/model.go": userSrc})
	model := filepath.Join(dir, "model")
	if code, _, stderr := run(t, model); code != cli.ExitOK {
		t.Fatalf("first run = %d, stderr %q", code, stderr)
	}

	// The last struct goes, and the fixtures written before go with it.
	writeFile(t, filepath.Join(model, "model.go"), "package model\n\ntype user struct{}\n")
	code, stdout, stderr := run(t, model, "-v")
	if want := "warning: no struct of example.com/app/model gets a fixture"; code != cli.ExitOK ||
		!strings.Contains(stderr, want) {
		t.Errorf("run = %d, stderr %q; want success with %q", code, stderr, want)
	}
	if stdout != "removed fixture/fixture_gen.go\n" {
		t.Errorf("stdout = %q, want the file listed as removed", stdout)
	}

	// A file of that name without the marker is someone's own.
	path := filepath.Join(model, "fixture", "fixture_gen.go")
	writeFile(t, path, "package fixture\n")
	if code, stdout, _ := run(t, model, "-v"); code != cli.ExitOK || stdout != "" {
		t.Errorf("run = %d, stdout %q; want the hand-written file left alone", code, stdout)
	}
	if got := readFile(t, path); got != "package fixture\n" {
		t.Errorf("fixture_gen.go = %q, want it untouched", got)
	}
}

// sqlc writes its models with the generated-code marker, which does not keep
// them from getting fixtures.
func TestCommand_GeneratedModels(t *testing.T) {
	t.Parallel()

	dir := writeModule(t, map[string]string{
		"db/models.go": "// Code generated by sqlc. DO NOT EDIT.\n\npackage db\n\ntype User struct {\n\tID int64\n}\n",
		"db/doc.go":    "package db\n",
	})

	code, _, stderr := run(t, filepath.Join(dir, "db"))
	if code != cli.ExitOK || stderr != "" {
		t.Fatalf("run = %d, stderr %q", code, stderr)
	}
	if got := readFile(t, filepath.Join(dir, "db", "fixture", "fixture_gen.go")); !strings.Contains(got, "func User(") {
		t.Errorf("fixture_gen.go has no fixture of User:\n%s", got)
	}
}

func TestCommand_AbsoluteDestination(t *testing.T) {
	t.Parallel()

	dir := writeModule(t, map[string]string{"model/model.go": userSrc})
	dest := filepath.Join(dir, "fixtures")

	code, _, stderr := run(t, filepath.Join(dir, "model"), "-destination", dest)
	if code != cli.ExitOK || stderr != "" {
		t.Fatalf("run = %d, stderr %q", code, stderr)
	}
	if got := readFile(t, filepath.Join(dest, "fixture_gen.go")); !strings.Contains(got, "package fixtures\n") {
		t.Errorf("fixtures/fixture_gen.go is not package fixtures:\n%s", got)
	}
}

// The go command cannot run where there is no directory.
func TestCommand_NoDirectory(t *testing.T) {
	t.Parallel()

	code, _, stderr := run(t, filepath.Join(t.TempDir(), "missing"))
	if code != cli.ExitFailure || !strings.Contains(stderr, "error:") {
		t.Errorf("run = %d, stderr %q; want a failure", code, stderr)
	}
}

func TestImportPath(t *testing.T) {
	t.Parallel()

	dir := writeModule(t, map[string]string{"model/model.go": userSrc, "odd/go.mod": "go 1.26.0\n"})
	outside := t.TempDir()

	for in, want := range map[string]string{
		dir:                                    "example.com/app",
		filepath.Join(dir, "model", "fixture"): "example.com/app/model/fixture",
		filepath.Join(dir, "odd", "fixture"):   "",
		outside:                                "",
	} {
		if got := gen.ImportPath(in); got != want {
			t.Errorf("ImportPath(%s) = %q, want %q", in, got, want)
		}
	}
}
