package gen

import (
	"fmt"
	"go/types"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/go-kanna/internal/suggest"
)

// fakeKey is the struct tag holding a gofakeit template.
const fakeKey = "fake"

// fakeValue is what a field's fake tag asks it to be filled with.
type fakeValue struct {
	// skip leaves the field at its zero value, as fake:"skip" and fake:"-" ask.
	skip bool

	// fixed is a Go literal, for a tag that generates nothing, such as
	// fake:"123-4567": the value as it stands.
	fixed string

	// method and args call a Faker method that returns the kind of the field,
	// such as Number(18, 65). result is the kind a numeric one returns.
	method string
	args   []string
	result types.BasicKind

	// template is passed to Faker.Generate, for a string.
	template string
}

// tagError is a mistake in a tag, with a hint on what to write instead.
type tagError struct {
	msg, hint string
}

func (e *tagError) Error() string { return e.msg }

// numericCall is a template a number takes: a Faker method returning result,
// whose arguments are numbers of that kind.
type numericCall struct {
	method string
	result types.BasicKind
}

// numericCalls are the templates a number takes. Any other generates text,
// which a number cannot hold without being parsed at run time.
var numericCalls = map[string]numericCall{
	"number":       {"Number", types.Int},
	"intrange":     {"IntRange", types.Int},
	"uintrange":    {"UintRange", types.Uint},
	"float32range": {"Float32Range", types.Float32},
	"float64range": {"Float64Range", types.Float64},
	"price":        {"Price", types.Float64},
}

// fakeOf reads the fake tag in tag, of a field of type t, and reports false for
// a field without one. An empty tag is no tag, as gofakeit has it.
//
// The template is checked against gofakeit as generation runs, since gofakeit
// itself does not complain: it gives back a function it does not know, braces
// and all, as the value, and passes over parameters it has no use for.
func fakeOf(tag reflect.StructTag, t types.Type) (fakeValue, bool, error) {
	s, ok := tag.Lookup(fakeKey)
	if !ok || s == "" {
		return fakeValue{}, false, nil
	}
	if s == "skip" || s == "-" {
		return fakeValue{skip: true}, true, nil
	}

	calls, err := parseTemplate(s)
	if err != nil {
		return fakeValue{}, true, err
	}
	v, err := fakeFor(s, calls, deref(t))
	return v, true, err
}

// fakeFor resolves the template s, which calls calls, for a value of type t.
func fakeFor(s string, calls []call, t types.Type) (fakeValue, error) {
	only := func(name string) bool { return len(calls) == 1 && calls[0].whole == s && calls[0].name == name }
	switch {
	case isNamed(t, "time", "Time"):
		if only("date") && calls[0].params == "" {
			return fakeValue{method: "Date"}, nil
		}
		return fakeValue{}, cannotFill(s, t, "a time.Time takes {date}")
	case isUUID(t):
		if only("uuid") && calls[0].params == "" {
			return fakeValue{method: "UUID"}, nil
		}
		return fakeValue{}, cannotFill(s, t, "a UUID takes {uuid}")
	}

	b, ok := t.Underlying().(*types.Basic)
	generates := len(calls) > 0 || strings.ContainsAny(s, "#?")
	switch {
	case !ok:
	case b.Info()&types.IsString != 0:
		if !generates {
			return fakeValue{fixed: strconv.Quote(s)}, nil
		}
		if _, err := gofakeit.New(1).Generate(s); err != nil {
			return fakeValue{}, &tagError{msg: fmt.Sprintf("fake tag %q: %v", s, err)}
		}
		return fakeValue{template: s}, nil
	case b.Info()&(types.IsInteger|types.IsFloat) != 0:
		if !generates {
			if v := parseNumber(s); v != nil && fits(v, b) && (b.Info()&types.IsFloat != 0 || isInteger(v)) {
				return fakeValue{fixed: s}, nil
			}
			return fakeValue{}, &tagError{msg: fmt.Sprintf("fake tag %q is not a number %s can hold", s, b)}
		}
		if len(calls) == 1 {
			if nc, ok := numericCalls[calls[0].name]; ok && only(calls[0].name) {
				return numeric(s, calls[0], nc, b)
			}
		}
		return fakeValue{}, cannotFill(s, t, "a number takes {number}, {intrange}, {uintrange}, {float32range}, "+
			"{float64range}, {price}, or a number as it stands")
	case b.Kind() == types.Bool:
		if s == "true" || s == "false" {
			return fakeValue{fixed: s}, nil
		}
		return fakeValue{}, cannotFill(s, t, "a bool takes true or false")
	}
	return fakeValue{}, cannotFill(s, t, "")
}

// cannotFill says that the template s makes no value of type t.
func cannotFill(s string, t types.Type, hint string) error {
	name := types.TypeString(t, (*types.Package).Name)
	return &tagError{msg: fmt.Sprintf("fake tag %q cannot fill %s", s, name), hint: hint}
}

// numeric resolves c, the call that makes up the template s, to a call of the
// Faker method nc names, for a number of type b. Parameters left out take
// gofakeit's defaults, and each has to fit both the method and b, so that the
// generated code neither fails to compile nor wraps around.
func numeric(s string, c call, nc numericCall, b *types.Basic) (fakeValue, error) {
	info := gofakeit.GetFuncLookup(c.name)
	args := c.args(info)
	for i := len(args); i < len(info.Params); i++ {
		args = append(args, info.Params[i].Default)
	}

	result := types.Typ[nc.result]
	for _, arg := range args {
		v := parseNumber(arg)
		switch {
		case v == nil:
			return fakeValue{}, &tagError{msg: fmt.Sprintf("fake tag %q: %s takes numbers, not %q", s, c.name, arg)}
		case result.Info()&types.IsInteger != 0 && !isInteger(v):
			return fakeValue{}, &tagError{msg: fmt.Sprintf("fake tag %q: %s takes integers, not %s", s, c.name, arg)}
		case !fits(v, result) || !fits(v, b):
			return fakeValue{}, &tagError{msg: fmt.Sprintf("fake tag %q: %s does not fit %s", s, arg, b)}
		}
	}
	return fakeValue{method: nc.method, args: args, result: nc.result}, nil
}

// call is a function a template calls, such as {number:18,65}.
type call struct {
	whole  string // as written, braces and all
	name   string
	params string // what follows the colon
}

// args splits the parameters of c as gofakeit does: a function taking one
// string takes them whole, and otherwise they are separated by commas, except
// within brackets.
func (c call) args(info *gofakeit.Info) []string {
	if c.params == "" {
		return nil
	}
	if len(info.Params) == 1 && info.Params[0].Type == "string" {
		return []string{c.params}
	}

	var args []string
	for rest := c.params; rest != ""; {
		var arg string
		if strings.HasPrefix(rest, "[") {
			if end := strings.IndexByte(rest, ']'); end >= 0 {
				arg, rest = rest[:end+1], strings.TrimPrefix(rest[end+1:], ",")
				args = append(args, strings.TrimSpace(arg))
				continue
			}
		}
		arg, rest, _ = strings.Cut(rest, ",")
		args = append(args, strings.TrimSpace(arg))
	}
	return args
}

// parseTemplate finds the functions s calls, as gofakeit's Generate does, and
// checks that gofakeit has each and that none is given more parameters than it
// takes.
func parseTemplate(s string) ([]call, error) {
	var calls []call
	for rest := s; ; {
		start := strings.IndexByte(rest, '{')
		if start < 0 {
			return calls, nil
		}
		end, depth := -1, 0
		for i := start; i < len(rest) && end < 0; i++ {
			switch rest[i] {
			case '{':
				depth++
			case '}':
				if depth--; depth == 0 {
					end = i
				}
			}
		}
		if end < 0 {
			return nil, &tagError{msg: fmt.Sprintf("fake tag %q: no } closes the { of %s", s, rest[start:])}
		}

		c := call{whole: rest[start : end+1]}
		c.name, c.params, _ = strings.Cut(rest[start+1:end], ":")
		info := gofakeit.GetFuncLookup(c.name)
		if info == nil {
			e := &tagError{msg: fmt.Sprintf("unknown function %q in fake tag %q", c.name, s)}
			if name, ok := suggest.Closest(c.name, slices.Sorted(maps.Keys(gofakeit.FuncLookups))); ok {
				e.hint = fmt.Sprintf("did you mean %q?", name)
			}
			return nil, e
		}
		if n := len(c.args(info)); n > len(info.Params) {
			return nil, &tagError{
				msg: fmt.Sprintf("fake tag %q: %s takes %d parameters, got %d", s, c.name, len(info.Params), n),
			}
		}
		calls = append(calls, c)
		rest = rest[end+1:]
	}
}

// isNamed reports whether t is the type name declared in the package at path.
func isNamed(t types.Type, path, name string) bool {
	n, ok := types.Unalias(t).(*types.Named)
	return ok && n.Obj().Name() == name && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == path
}

// isUUID reports whether t is the UUID of the standard library or of
// github.com/google/uuid, which both parse with MustParse.
func isUUID(t types.Type) bool {
	return isNamed(t, "uuid", "UUID") || isNamed(t, "github.com/google/uuid", "UUID")
}
