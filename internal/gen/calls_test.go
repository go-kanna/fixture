package gen_test

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-kanna/internal/cli"
	"github.com/go-kanna/internal/packages"
)

// articleSrc is the model whose fixtures the code below calls.
const articleSrc = "package model\n\ntype Article struct {\n\tID    int64\n\tSlug  string\n\tTitle string\n}\n"

// compiles reports whether the packages of dir load without errors.
func compiles(t *testing.T, dir string) {
	t.Helper()

	set, err := packages.Load([]string{"./..."}, packages.Config{Dir: dir})
	if err != nil || len(set.Diags) > 0 {
		t.Errorf("the module does not compile: %v %v", err, set.Diags)
	}
}

// module writes the model and files into a module and returns the directory
// of the model, where kanna-fixture runs.
func module(t *testing.T, files map[string]string) string {
	t.Helper()

	all := map[string]string{"model/model.go": articleSrc}
	maps.Copy(all, files)
	return filepath.Join(writeModule(t, all), "model")
}

func TestCommand_GeneratesWhatCodeAlreadyCalls(t *testing.T) {
	t.Parallel()

	model := module(t, map[string]string{
		"model/fixture/helpers.go": "package fixture\n\nimport \"example.com/app/model\"\n\n" +
			"func Admin() model.Article { return Article(func(a *model.Article) { a.Title = \"admin\" }) }\n",
	})
	if code, stdout, stderr := run(t, model); code != cli.ExitOK || stdout != "" || stderr != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q; want success", code, stdout, stderr)
	}
	compiles(t, filepath.Dir(model))
}

func TestCommand_Mistakes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			// Before Article is written, go/types checks no further than
			// undefined: Article.
			name: "mistakes behind calls to a fixture, and one behind none",
			files: map[string]string{"model/fixture/helpers.go": `package fixture

import "example.com/app/model"

var chained = Article().Bogus

func assigned() string {
	a := Article()
	return a.Bogus
}

func take(a model.Article) {}

func passed() {
	take(Article(), 1)
}

var plain int = "one"
`},
			want: "fixture/helpers.go:5:25: error: Article().Bogus undefined " +
				"(type model.Article has no field or method Bogus)\n" +
				"fixture/helpers.go:9:11: error: a.Bogus undefined (type model.Article has no field or method Bogus)\n" +
				"fixture/helpers.go:15:18: error: too many arguments in call to take\n\thave (model.Article, number)\n" +
				"\twant (model.Article)\n" +
				"fixture/helpers.go:18:17: error: cannot use \"one\" (untyped string constant) as int value " +
				"in variable declaration\n",
		},
		{
			// Only the output imports model.
			name:  "a mistake in code importing nothing of the model",
			files: map[string]string{"model/fixture/helpers.go": "package fixture\n\nvar _ = Article().Bogus\n"},
			want: "fixture/helpers.go:3:19: error: Article().Bogus undefined " +
				"(type model.Article has no field or method Bogus)\n",
		},
		{
			// Article gets no fixture, which the type error says why.
			name: "a field that does not type-check",
			files: map[string]string{
				"model/model.go":           "package model\n\ntype Article struct {\n\tID    int64\n\tSlug  strng\n}\n",
				"model/fixture/helpers.go": "package fixture\n\nfunc Some() { _ = Article() }\n",
			},
			want: "./model.go:5:8: error: undefined: strng\nfixture/helpers.go:3:19: error: undefined: Article\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model := module(t, tt.files)
			if code, _, stderr := run(t, model); code != cli.ExitFailure || stderr != tt.want {
				t.Errorf("exit %d, stderr:\n%s\nwant %d and\n%s", code, stderr, cli.ExitFailure, tt.want)
			}
			if _, err := os.Stat(filepath.Join(model, "fixture", "fixture_gen.go")); err == nil {
				t.Error("fixture_gen.go was written")
			}
		})
	}
}

func TestCommand_KeepsWhatCodeStillCalls(t *testing.T) {
	t.Parallel()

	const authors = "package model\n\ntype Author struct {\n\tID   int64\n\tName string\n}\n\n" +
		"type Article struct {\n\tID     int64\n\tTitle  string\n\tAuthor Author\n}\n"
	tests := []struct {
		name, model, helpers, skipped, want string
	}{
		{
			name: "no fixture left", model: articleSrc,
			helpers: "package fixture\n\nimport \"example.com/app/model\"\n\n" +
				"func Published() model.Article {\n\treturn Article(func(a *model.Article) { a.Title = \"published\" })\n}\n",
			skipped: "type Article",
			want: "warning: no struct of example.com/app/model gets a fixture\n" +
				"fixture/helpers.go:6:9: error: undefined: Article\n",
		},
		{
			name: "one fixture of two gone", model: authors,
			helpers: "package fixture\n\nfunc Some() {\n\t_ = Article()\n\t_ = Author()\n}\n",
			skipped: "type Author",
			want:    "fixture/helpers.go:5:6: error: undefined: Author\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model := module(t, map[string]string{"model/model.go": tt.model, "model/fixture/helpers.go": tt.helpers})
			if code, _, stderr := run(t, model); code != cli.ExitOK {
				t.Fatalf("first run: exit %d, stderr %q", code, stderr)
			}
			output := filepath.Join(model, "fixture", "fixture_gen.go")
			generated := readFile(t, output)

			writeFile(t, filepath.Join(model, "model.go"),
				strings.Replace(tt.model, tt.skipped, "//kanna:fixture skip\n"+tt.skipped, 1))
			if code, _, stderr := run(t, model); code != cli.ExitFailure || stderr != tt.want {
				t.Errorf("exit %d, stderr:\n%s\nwant %d and\n%s", code, stderr, cli.ExitFailure, tt.want)
			}
			if got := readFile(t, output); got != generated {
				t.Errorf("fixture_gen.go was rewritten:\n%s", got)
			}
		})
	}
}

func TestCommand_RemovesWhatNothingCalls(t *testing.T) {
	t.Parallel()

	// The destination has code of its own, which calls no fixture.
	model := module(t, map[string]string{
		"model/fixture/doc.go": "// Package fixture makes models for tests.\npackage fixture\n",
	})
	if code, _, stderr := run(t, model); code != cli.ExitOK {
		t.Fatalf("first run: exit %d, stderr %q", code, stderr)
	}
	writeFile(t, filepath.Join(model, "model.go"),
		strings.Replace(articleSrc, "type Article", "//kanna:fixture skip\ntype Article", 1))

	if code, stdout, stderr := run(t, model, "-v"); code != cli.ExitOK || stdout != "removed fixture/fixture_gen.go\n" ||
		stderr != "warning: no struct of example.com/app/model gets a fixture\n" {
		t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestCommand_RegeneratesStaleOutput(t *testing.T) {
	t.Parallel()

	// A field gone breaks the bodies of the last run's output alone, and a
	// package renamed its declarations too, which sets it aside whole.
	tests := []struct {
		name, old, new string
	}{
		{name: "a field gone", old: "\tSlug  string\n", new: ""},
		{name: "the package renamed", old: "package model", new: "package models"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model := module(t, map[string]string{
				"model/fixture/helpers.go": "package fixture\n\nfunc Some() { _ = Article() }\n",
			})
			if code, _, stderr := run(t, model); code != cli.ExitOK {
				t.Fatalf("first run: exit %d, stderr %q", code, stderr)
			}
			writeFile(t, filepath.Join(model, "model.go"), strings.Replace(articleSrc, tt.old, tt.new, 1))

			if code, _, stderr := run(t, model); code != cli.ExitOK || stderr != "" {
				t.Fatalf("exit %d, stderr %q; want success", code, stderr)
			}
			compiles(t, filepath.Dir(model))
		})
	}
}

func TestCommand_OtherGeneratorsStaleOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, file, src, want string
	}{
		{
			// kanna-validate writes it again in the same go generate.
			name: "a body", file: "model/validate_gen.go",
			src: "// Code generated by kanna-validate v0.0.4. DO NOT EDIT.\n\npackage model\n\n" +
				"func (x Article) Validate() error {\n\t_ = x.Confirm\n\treturn nil\n}\n",
		},
		{
			// The fixture of Row cannot be written.
			name: "a declaration", file: "model/models.go",
			src:  "// Code generated by sqlc. DO NOT EDIT.\n\npackage model\n\ntype Row struct{ X Missing }\n",
			want: "./models.go:5:20: error: undefined: Missing\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model := module(t, map[string]string{tt.file: tt.src})
			code, _, stderr := run(t, model)
			if tt.want == "" {
				if code != cli.ExitOK || stderr != "" {
					t.Errorf("exit %d, stderr %q; want success", code, stderr)
				}
				return
			}
			if code != cli.ExitFailure || stderr != tt.want {
				t.Errorf("exit %d, stderr:\n%s\nwant %d and\n%s", code, stderr, cli.ExitFailure, tt.want)
			}
		})
	}
}
