package gen_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/go-kanna/internal/packages"
	"github.com/go-kanna/internal/pkgtest"

	"github.com/go-kanna/fixture/internal/gen"
)

// targets loads src as one package and finds its targets, as gen.Targets spells
// them.
func targets(t *testing.T, src string, exclude ...string) ([]string, []string) {
	t.Helper()

	pkg := pkgtest.Load(t, map[string]string{"test.go": src})
	return gen.Targets(pkg.Fset, []*packages.Package{pkg}, exclude)
}

func TestTargets(t *testing.T) {
	t.Parallel()

	// Declared in an order that is neither alphabetical nor reversed, so that
	// sorting cannot pass by accident.
	names, diags := targets(t, `package test

type User struct {
	Name string
}

type Article struct {
	Author User
}

type session struct{}

type Session struct {
	ID int64
}

//kanna:fixture skip
type Internal struct{}

type Status string
`)

	if want := []string{"Article", "Session", "User"}; !slices.Equal(names, want) {
		t.Errorf("targets = %q, want %q", names, want)
	}
	if len(diags) > 0 {
		t.Errorf("diagnostics = %q, want none", diags)
	}
}

func TestTargets_Exclude(t *testing.T) {
	t.Parallel()

	src := `package test

type User struct{}

type Article struct{}

type Querier struct{}

type queries struct{}
`

	tests := []struct {
		name    string
		exclude []string
		want    []string
		diags   []string
	}{
		{
			name:    "names left out",
			exclude: []string{"Querier", "User"},
			want:    []string{"Article"},
		},
		{
			name:    "a misspelled name",
			exclude: []string{"Querer"},
			want:    []string{"Article", "Querier", "User"},
			diags:   []string{"warning: -exclude Querer names no exported struct of the package\n  did you mean \"Querier\"?"},
		},
		{
			name:    "an unexported struct, which gets no fixture anyway",
			exclude: []string{"queries"},
			want:    []string{"Article", "Querier", "User"},
			diags:   []string{"warning: -exclude queries names no exported struct of the package\n  did you mean \"Querier\"?"},
		},
		{
			name:    "a name nothing is close to",
			exclude: []string{"Invoice"},
			want:    []string{"Article", "Querier", "User"},
			diags:   []string{"warning: -exclude Invoice names no exported struct of the package"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			names, diags := targets(t, src, tt.exclude...)
			if !slices.Equal(names, tt.want) {
				t.Errorf("targets = %q, want %q", names, tt.want)
			}
			if !slices.Equal(diags, tt.diags) {
				t.Errorf("diagnostics = %q, want %q", diags, tt.diags)
			}
		})
	}
}

func TestTargets_Skipped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "generic",
			src: `package test

type Page[T any] struct {
	Items []T
}
`,
			want: "test.go:3:6: warning: no fixture for generic struct Page, whose type arguments a fixture cannot pick\n" +
				"  mark it //kanna:fixture skip to leave it out without a warning",
		},
		{
			name: "a mutex",
			src: `package test

import "sync"

type Cache struct {
	mu    sync.Mutex
	Items map[string]string
}
`,
			want: "test.go:5:6: warning: no fixture for Cache, whose field mu (sync.Mutex) must not be copied, " +
				"as a fixture returns a copy\n" +
				"  mark it //kanna:fixture skip to leave it out without a warning",
		},
		{
			name: "an embedded mutex",
			src: `package test

import "sync"

type Counter struct {
	sync.RWMutex
	N int
}
`,
			want: "field RWMutex (sync.RWMutex) must not be copied",
		},
		{
			name: "a mutex inside another struct",
			src: `package test

import "sync"

type guard struct {
	mu sync.Mutex
}

type Account struct {
	Balance int64
	g       guard
}
`,
			want: "field g (guard) must not be copied",
		},
		{
			name: "a mutex in an array",
			src: `package test

import "sync"

type Shards struct {
	locks [4]sync.Mutex
}
`,
			want: "field locks ([4]sync.Mutex) must not be copied",
		},
		{
			name: "a noCopy marker",
			src: `package test

import "sync/atomic"

type Hits struct {
	N atomic.Int64
}
`,
			want: "field N (atomic.Int64) must not be copied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			names, diags := targets(t, tt.src)
			if len(names) > 0 {
				t.Errorf("targets = %q, want none", names)
			}
			if len(diags) != 1 || !strings.Contains(diags[0], tt.want) {
				t.Errorf("diagnostics = %q, want one containing %q", diags, tt.want)
			}
		})
	}
}

// What only points at a lock copies the pointer, which go vet allows.
func TestTargets_PointerToLock(t *testing.T) {
	t.Parallel()

	names, diags := targets(t, `package test

import "sync"

type Shared struct {
	mu *sync.Mutex
	Locks []sync.Mutex
}
`)

	if want := []string{"Shared"}; !slices.Equal(names, want) {
		t.Errorf("targets = %q, want %q", names, want)
	}
	if len(diags) > 0 {
		t.Errorf("diagnostics = %q, want none", diags)
	}
}

// A struct left out on purpose is not warned about for what would have kept it
// out anyway.
func TestTargets_LeftOutSilently(t *testing.T) {
	t.Parallel()

	names, diags := targets(t, `package test

import "sync"

//kanna:fixture skip
type Page[T any] struct{}

type Cache struct {
	mu sync.Mutex
}
`, "Cache")

	if len(names) > 0 {
		t.Errorf("targets = %q, want none", names)
	}
	if len(diags) > 0 {
		t.Errorf("diagnostics = %q, want none", diags)
	}
}

func TestTargets_Directive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		src   string
		names []string
		diags []string
	}{
		{
			name: "misspelled",
			src: `package test

//kanna:fixtur skip
type Internal struct{}
`,
			names: []string{"Internal"},
			diags: []string{`test.go:3:1: warning: "//kanna:fixtur skip" is not recognized as a directive
  did you mean "//kanna:fixture skip"?`},
		},
		{
			name: "no argument",
			src: `package test

//kanna:fixture
type Internal struct{}
`,
			diags: []string{"test.go:3:1: error: //kanna:fixture takes skip, got nothing"},
		},
		{
			name: "an unknown argument",
			src: `package test

//kanna:fixture please
type Internal struct{}
`,
			diags: []string{`test.go:3:1: error: //kanna:fixture takes skip, got "please"`},
		},
		{
			name: "a misspelled argument",
			src: `package test

//kanna:fixture skp
type Internal struct{}
`,
			diags: []string{`test.go:3:1: error: //kanna:fixture takes skip, got "skp"
  did you mean "//kanna:fixture skip"?`},
		},
		{
			name: "the old spelling, which is not read",
			src: `package test

//kanna:ignore
type Internal struct{}
`,
			names: []string{"Internal"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			names, diags := targets(t, tt.src)
			if !slices.Equal(names, tt.names) {
				t.Errorf("targets = %q, want %q", names, tt.names)
			}
			if !slices.Equal(diags, tt.diags) {
				t.Errorf("diagnostics = %q, want %q", diags, tt.diags)
			}
		})
	}
}
