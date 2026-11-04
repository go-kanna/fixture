package gen

import (
	"errors"
	"fmt"
	"go/constant"
	"go/token"
	"go/types"
	"math"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/go-kanna/internal/diag"
	"github.com/go-kanna/internal/enumtype"
	"github.com/go-kanna/internal/qualifier"
	"github.com/go-kanna/internal/scan"
	"github.com/go-kanna/internal/valuetype"
)

// imports names the packages the expressions use, as the generated file
// imports them. *gofile.File is one.
type imports interface {
	Import(path, name string) string
	Qualifier(p *types.Package) string
}

// expr is an expression a fixture sets a field to.
type expr struct {
	text string

	// typ is the type of the expression, or nil for an untyped constant, which
	// any field of its kind takes as it is.
	typ types.Type

	// usesN, usesF, and generates report whether it calls for the number of
	// the fixture, its faker, and the helper calling Faker.Generate; usesOrPanic,
	// for the helper keeping what a value type's constructor returns; skips, for
	// the one counting past what ne leaves out.
	usesN, usesF, generates, usesOrPanic, skips bool
}

// assignment sets field to value.
type assignment struct {
	field, value string
}

// filled is what a fixture sets the fields of its struct to.
type filled struct {
	// fields go in the composite literal, in the order they are declared.
	fields []assignment

	// copies follow it, for an eqfield rule: x.Confirm = x.Password.
	copies []assignment

	// usesN, usesF, and generates report whether the values call for the
	// number of the fixture, its faker, and the helper calling Faker.Generate;
	// usesOrPanic, for the helper keeping what a value type's constructor
	// returns; skips, for the one counting past what ne leaves out.
	usesN, usesF, generates, usesOrPanic, skips bool
}

// filler works out the values of the fixtures of one generated file.
type filler struct {
	imports imports

	// fset is what the types were loaded into, which orders the constants of
	// a type.
	fset *token.FileSet

	// targets are the structs that get a fixture, by the name of their type.
	targets map[*types.TypeName]bool

	// generate names the helper that calls Faker.Generate, orPanic the one
	// that keeps what a value type's constructor returns, and skipping the one
	// counting past what ne leaves out.
	generate, orPanic, skipping string

	// hand holds the functions written by hand in the destination, by name,
	// among which written finds the fixture of a type.
	hand map[string]*types.Func

	// n and f are the names the fixture being worked out gives its number and
	// its faker.
	n, f string
}

// fill works out the value of each field of s, and says what is wrong with
// their tags.
func (fl *filler) fill(s scan.Struct) (filled, []diag.Diag) {
	var (
		out   filled
		diags []diag.Diag
	)
	for _, field := range s.Fields {
		if !field.Exported || !nameable(field.Type) {
			continue
		}

		fv, faked, err := fakeOf(field.Tag, field.Type)
		if err != nil {
			d := diag.Errorf(field.Pos, "%s (field %s)", err, field.Name)
			if te, ok := errors.AsType[*tagError](err); ok && te.hint != "" {
				d = d.WithHints(te.hint)
			}
			diags = append(diags, d)
			continue
		}

		if faked {
			diags = append(diags, broken(field, fv)...)
		}

		var (
			e  expr
			ok bool
		)
		switch other, copies := copied(s, field); {
		case faked && fv.skip:
		case faked:
			e, ok = fl.fake(fv, deref(field.Type)), true
			if _, isPtr := field.Type.Underlying().(*types.Pointer); isPtr {
				e = fl.pointer(e, deref(field.Type))
			}
		case copies:
			out.copies = append(out.copies, assignment{field: field.Name, value: other})
			continue
		default:
			wants := wantsOf(field.Tag, field.Type)
			diags = append(diags, unkept(field, wants)...)
			diags = append(diags, fl.unkeptRules(s, field, wants)...)
			diags = append(diags, fl.recurring(s, field, wants)...)
			e, ok = fl.value(s, field.Type, wants, field.Name)
		}
		if !ok {
			continue
		}
		out.fields = append(out.fields, assignment{field: field.Name, value: e.text})
		out.usesN = out.usesN || e.usesN
		out.usesF = out.usesF || e.usesF
		out.generates = out.generates || e.generates
		out.usesOrPanic = out.usesOrPanic || e.usesOrPanic
		out.skips = out.skips || e.skips
	}
	out.copies = inOrder(out.copies)
	return out, diags
}

// unkept warns about the rules of field, wants, that the fixture cannot keep:
// a rule it does not know, a length a map is to have, which it fills empty, a
// format no value of which fits the length the rules set, which str passes
// over, and a pattern the faker makes values from that do not match it or do
// not fit that length, which is cleared so that the field is numbered instead.
// A pattern that does not compile is cleared without a word: that is for the
// validator to report, as is a pattern or a format on what is not a string.
// broken warns about the fake tag of field, which made fv, where the rules of
// its validate tag reject what it makes: always, or for some of the numbers a
// range makes. What a template of text makes is not known before it runs, and
// fake:"skip" asks for the zero value knowing the rules may reject it, as the
// way to leave a field the fixture cannot fill.
func broken(field scan.Field, fv fakeValue) []diag.Diag {
	wants := wantsOf(field.Tag, field.Type)
	if len(wants) == 0 {
		return nil
	}
	r := wants[0].rules
	warn := func(format string) []diag.Diag {
		return []diag.Diag{diag.Warningf(field.Pos, format+" (field %s)",
			field.Tag.Get(fakeKey), field.Tag.Get(tagKey), field.Name)}
	}
	switch {
	case fv.fixed != "":
		if v := fixedValue(fv.fixed); v != nil && !r.admits(v) {
			return warn("fake tag %q is a value that validate:%q rejects")
		}
	case fv.result != types.Invalid && len(fv.args) == 2:
		low, high := parseNumber(fv.args[0]), parseNumber(fv.args[1])
		if low == nil || high == nil {
			return nil
		}
		switch all, some := r.admitsRange(low, high, types.Typ[fv.result].Info()&types.IsInteger != 0); {
		case !some:
			return warn("fake tag %q makes no value that validate:%q lets through")
		case !all:
			return warn("fake tag %q makes some values that validate:%q rejects")
		}
	}
	return nil
}

// fixedValue reads fixed, the Go literal a fake tag stands for as it is.
func fixedValue(fixed string) constant.Value {
	switch fixed {
	case "true", "false":
		return constant.MakeBool(fixed == "true")
	}
	if s, err := strconv.Unquote(fixed); err == nil {
		return constant.MakeString(s)
	}
	return parseNumber(fixed)
}

func unkept(field scan.Field, wants []want) []diag.Diag {
	var diags []diag.Diag
	warn := func(msg string) {
		diags = append(diags, diag.Warningf(field.Pos, "%s; give the field a fake tag (field %s)", msg, field.Name))
	}

	t := field.Type
	for i, w := range wants {
		if i > 0 {
			t, _ = elemOf(t)
		}
		for _, rule := range w.unknown {
			warn(fmt.Sprintf("kanna-fixture does not know %s, so the value it fills may break it", rule))
		}
		if _, isMap := deref(t).Underlying().(*types.Map); isMap && w.minLen > 0 {
			warn("an empty map does not fit within the length the rules set")
		}
		if shapeOf(t) != shapeString {
			continue
		}
		// A format the name alone asks for gives way to the length, which a
		// value numbered without it meets.
		if format, fits := formatOf(w, field.Name); !fits && w.format != "" {
			warn(formats[format].what + " does not fit within the length the rules set")
		}
		if w.pattern == "" {
			continue
		}
		re, err := regexp.Compile(w.pattern)
		if err == nil {
			msg := unmatched(re, w)
			if msg == "" {
				continue
			}
			warn(msg)
		}
		wants[i].pattern = ""
	}
	return diags
}

// ruled works out a value of type t that passes the rule of the type w.rule
// names: the first constant of an enum, or what a field named after a value
// type would hold, converted to t. It reports false for a rule it cannot keep,
// which unkeptRules warns about.
func (fl *filler) ruled(s scan.Struct, t types.Type, w want) (expr, bool) {
	named, err := ruleType(s, w.rule)
	if err != nil {
		return expr{}, false
	}
	if v, ok := valuetype.Of(named); ok && v.Constructor != nil {
		if e, ok := fl.written(named); ok {
			e.text += "." + v.Accessor + "()"
			e.typ = v.Held
			return fl.convert(e, t), true
		}
		w.rule = ""
		e, ok := fl.value(s, v.Held, []want{w}, named.Obj().Name())
		return fl.convert(e, t), ok
	}
	if _, ok := enumtype.Of(named); ok {
		if c := firstConstant(fl.fset, named); c != nil {
			return fl.convert(expr{text: fl.imports.Qualifier(c.Pkg()) + "." + c.Name(), typ: named}, t), true
		}
	}
	return expr{}, false
}

// ruleType returns the type ref names, as rule=ref of kanna-validate on a field
// of s does: a type of the package of s, or pkg.Type of the package the file of
// s imports as pkg.
func ruleType(s scan.Struct, ref string) (*types.Named, error) {
	own := s.Named.Obj().Pkg()
	pkg, name := own, ref
	if qual, n, ok := strings.Cut(ref, "."); ok {
		p, err := qualifier.Of(s.File, own).Lookup(qual)
		if err != nil {
			return nil, err //nolint:wrapcheck // it says what is wrong with the qualifier
		}
		pkg, name = p, n
	}
	tn, ok := pkg.Scope().Lookup(name).(*types.TypeName)
	if !ok {
		return nil, fmt.Errorf("%s declares no type %s", pkg.Name(), name)
	}
	named, ok := types.Unalias(tn.Type()).(*types.Named)
	if !ok {
		return nil, fmt.Errorf("%s is not a declared type", ref)
	}
	return named, nil
}

// unkeptRules warns about each rule= of wants, the rules of field of s, that the
// fixture cannot keep: one naming no type it can read, or a type that is
// neither an enum with a constant to take nor a value type of kanna-value,
// which kanna-validate refuses too.
func (fl *filler) unkeptRules(s scan.Struct, field scan.Field, wants []want) []diag.Diag {
	var diags []diag.Diag
	for _, w := range wants {
		if w.rule == "" {
			continue
		}
		var msg string
		named, err := ruleType(s, w.rule)
		switch {
		case err != nil:
			msg = fmt.Sprintf("kanna-fixture cannot read rule=%s: %v", w.rule, err)
		case !fl.ruleKept(named):
			msg = fmt.Sprintf("kanna-fixture cannot tell what passes rule=%s, which is neither an enum nor a value type",
				w.rule)
		default:
			continue
		}
		diags = append(diags, diag.Warningf(field.Pos, "%s; give the field a fake tag (field %s)", msg, field.Name))
	}
	return diags
}

// ruleKept reports whether ruled fills a value for the rule of named.
func (fl *filler) ruleKept(named *types.Named) bool {
	if v, ok := valuetype.Of(named); ok {
		return v.Constructor != nil
	}
	_, enum := enumtype.Of(named)
	return enum && firstConstant(fl.fset, named) != nil
}

// written returns the call of the fixture of t written by hand in the
// destination: a function named after t that returns it, callable as the
// generated code calls it. It takes nothing, or, as a generated fixture does,
// setters for a struct, or, for any other type, a value type included, the
// faker of the fixture calling it, seeded as for a field tagged fake.
func (fl *filler) written(t *types.Named) (expr, bool) {
	fn, ok := fl.hand[t.Obj().Name()]
	if !ok {
		return expr{}, false
	}
	sig := fn.Signature()
	if sig.Recv() != nil || sig.TypeParams().Len() > 0 || sig.Results().Len() != 1 ||
		!types.Identical(sig.Results().At(0).Type(), t) {
		return expr{}, false
	}
	call := expr{text: fn.Name() + "()", typ: t}
	_, isValue := valuetype.Of(t)
	_, isStruct := t.Underlying().(*types.Struct)
	setters := isStruct && !isValue
	switch params := sig.Params(); {
	case params.Len() == 0:
		return call, true
	case params.Len() != 1:
	case sig.Variadic() && setters && isSetters(params.At(0).Type(), t):
		return call, true
	case !sig.Variadic() && !setters && isFaker(params.At(0).Type()):
		return expr{text: fn.Name() + "(" + fl.f + ")", typ: t, usesF: true}, true
	}
	return expr{}, false
}

// isFaker reports whether p is the *gofakeit.Faker a fixture gives a field.
func isFaker(p types.Type) bool {
	ptr, ok := p.(*types.Pointer)
	return ok && isNamed(ptr.Elem(), gofakeitPath, "Faker")
}

// hasWritten reports whether t has a fixture written by hand.
func (fl *filler) hasWritten(t *types.Named) bool {
	_, ok := fl.written(t)
	return ok
}

// isSetters reports whether p, the variadic parameter of a function, takes
// what a generated fixture of t does: func(*t) setters.
func isSetters(p types.Type, t *types.Named) bool {
	sl, ok := p.(*types.Slice)
	if !ok {
		return false
	}
	fn, ok := sl.Elem().Underlying().(*types.Signature)
	return ok && fn.Params().Len() == 1 && fn.Results().Len() == 0 &&
		types.Identical(fn.Params().At(0).Type(), types.NewPointer(t))
}

// recurring warns about the fixtures field of s would call, as the rules of
// wants ask for a value there, but does not, since they would call the fixture
// of s back. The value left zero in their place breaks a rule on the way back
// to s, as a required pointer or a slice of a length to have is what leads
// there.
func (fl *filler) recurring(s scan.Struct, field scan.Field, wants []want) []diag.Diag {
	var diags []diag.Diag
	for _, n := range fl.calls(field.Type, wants) {
		if fl.reaches(n, s.Named.Obj()) {
			diags = append(diags, diag.Warningf(field.Pos,
				"the fixture of %[1]s is not called here, as it would call that of %[2]s back, so a zero %[1]s "+
					"breaks its rules; set the field, or give it a fake tag (field %[3]s)", n.Name(), s.Name, field.Name))
		}
	}
	return diags
}

// copied reports the field whose value field copies, for an eqfield rule that
// names another exported field of s of the same type.
func copied(s scan.Struct, field scan.Field) (string, bool) {
	wants := wantsOf(field.Tag, field.Type)
	if len(wants) == 0 || wants[0].field == "" {
		return "", false
	}
	for _, other := range s.Fields {
		if other.Name == wants[0].field && other.Name != field.Name && other.Exported &&
			types.Identical(other.Type, field.Type) {
			return other.Name, true
		}
	}
	return "", false
}

// inOrder orders copies so that a copy of a field that copies another comes
// after that copy. A cycle, which nothing fills, leaves its fields zero
// whichever way it goes.
func inOrder(copies []assignment) []assignment {
	byField := make(map[string]assignment, len(copies))
	for _, a := range copies {
		byField[a.field] = a
	}
	var (
		ordered []assignment
		seen    = make(map[string]bool, len(copies))
		visit   func(a assignment)
	)
	visit = func(a assignment) {
		if seen[a.field] {
			return
		}
		seen[a.field] = true
		if from, ok := byField[a.value]; ok {
			visit(from)
		}
		ordered = append(ordered, a)
	}
	for _, a := range copies {
		visit(a)
	}
	return ordered
}

// fake turns what a fake tag asks for into an expression of type t.
func (fl *filler) fake(fv fakeValue, t types.Type) expr {
	switch {
	case fv.fixed != "":
		return expr{text: fv.fixed}
	case fv.method == "UUID":
		pkg := fl.imports.Import(namedPath(t), "uuid")
		return expr{text: pkg + ".MustParse(" + fl.f + ".UUID())", typ: t, usesF: true}
	case fv.method == "Date":
		return expr{text: fl.f + ".Date()", typ: t, usesF: true}
	case fv.method != "":
		e := expr{
			text:  fl.f + "." + fv.method + "(" + strings.Join(fv.args, ", ") + ")",
			typ:   types.Typ[fv.result],
			usesF: true,
		}
		return fl.convert(e, t)
	default:
		e := expr{
			text:      fl.generate + "(" + fl.f + ", " + strconv.Quote(fv.template) + ")",
			typ:       types.Typ[types.String],
			usesF:     true,
			generates: true,
		}
		return fl.convert(e, t)
	}
}

// value works out the value of type t that wants, the rules of each level, ask
// for, in a field named name of s. It reports false for the zero value.
func (fl *filler) value(s scan.Struct, t types.Type, wants []want, name string) (expr, bool) {
	w := want{maxLen: -1}
	if len(wants) > 0 {
		w = wants[0]
	}

	switch u := t.Underlying().(type) {
	case *types.Pointer:
		// What is absent stays absent, unless the rules say it is there.
		if !w.required {
			return expr{}, false
		}
		if e, ok := fl.value(s, u.Elem(), wants, name); ok {
			return fl.pointer(e, u.Elem()), true
		}
		return expr{text: "new(" + types.TypeString(u.Elem(), fl.imports.Qualifier) + ")", typ: t}, true
	case *types.Slice:
		k := w.elements()
		if k == 0 && !w.required {
			return expr{}, false
		}
		typ := types.TypeString(t, fl.imports.Qualifier)
		if k == 0 {
			return expr{text: typ + "{}", typ: t}, true
		}
		e, ok := fl.value(s, u.Elem(), rest(wants), name)
		if !ok {
			return expr{text: "make(" + typ + ", " + strconv.Itoa(k) + ")", typ: t}, true
		}
		e = fl.convert(e, u.Elem())
		e.text = typ + "{" + strings.Repeat(e.text+", ", k-1) + e.text + "}"
		e.typ = t
		return e, true
	case *types.Map:
		if !w.required {
			return expr{}, false
		}
		return expr{text: types.TypeString(t, fl.imports.Qualifier) + "{}", typ: t}, true
	}

	if w.rule != "" {
		if e, ok := fl.ruled(s, t, w); ok {
			return e, true
		}
	}
	if named, ok := types.Unalias(t).(*types.Named); ok {
		if e, ok := fl.named(s, named, w); ok {
			return e, true
		}
	}

	b, ok := t.Underlying().(*types.Basic)
	switch {
	case !ok:
		return expr{}, false
	case b.Info()&types.IsString != 0:
		return fl.str(t, w, name), true
	case b.Info()&types.IsInteger != 0:
		return fl.integer(t, b, w), true
	case b.Info()&types.IsFloat != 0:
		if w.exact != nil {
			return expr{text: floatLiteral(w.exact)}, true
		}
		return fl.convert(expr{text: fl.n, typ: types.Typ[types.Int64], usesN: true}, t), true
	case b.Kind() == types.Bool && w.required:
		return expr{text: "true"}, true
	}
	return expr{}, false
}

// rest returns the rules of the level after a dive.
func rest(wants []want) []want {
	if len(wants) > 1 {
		return wants[1:]
	}
	return nil
}

// named works out the value of a named type that has one of its own: one with
// a fixture written by hand, a struct with a fixture, time.Time, a UUID, a
// value type of kanna-value, or an enum. It reports false for any other.
func (fl *filler) named(s scan.Struct, t *types.Named, w want) (expr, bool) {
	if e, ok := fl.written(t); ok {
		return e, true
	}
	// The fixtures live in another package, so only an exported value type,
	// and so an exported constructor, can be made there.
	if v, ok := valuetype.Of(t); ok && t.Obj().Exported() && v.Constructor != nil {
		return fl.valueType(s, v, w)
	}

	switch {
	case fl.targets[t.Obj()]:
		if fl.reaches(t.Obj(), s.Named.Obj()) {
			return expr{}, false
		}
		return expr{text: t.Obj().Name() + "()", typ: t}, true
	case isNamed(t, "time", "Time"):
		tm := fl.imports.Import("time", "time")
		return expr{
			text:  fmt.Sprintf("%s.Date(2000, %[1]s.January, 1, 0, 0, int(%s), 0, %[1]s.UTC)", tm, fl.n),
			typ:   t,
			usesN: true,
		}, true
	case isUUID(t):
		pkg := fl.imports.Import(namedPath(t), "uuid")
		e := fl.sprintf("00000000-0000-4000-8000-%012x", fl.n)
		e.text = pkg + ".MustParse(" + e.text + ")"
		e.typ = t
		return e, true
	case len(w.choices) > 0 || w.exact != nil || w.low != nil:
		// The rules of the field say which values it takes.
		return expr{}, false
	}
	// Constants of any other type, such as time.Nanosecond or a MaxRetries,
	// are no choices to pick from.
	if _, ok := enumtype.Of(t); ok {
		if c := firstConstant(fl.fset, t); c != nil {
			return expr{text: fl.imports.Qualifier(c.Pkg()) + "." + c.Name(), typ: t}, true
		}
	}
	return expr{}, false
}

// valueType works out a value of v, a value type of kanna-value: its
// constructor called with what a field named after the type would hold. A rule
// the guess breaks makes the fixture panic, saying to write the fixture of the
// type by hand, which named calls instead of guessing.
func (fl *filler) valueType(s scan.Struct, v valuetype.Type, w want) (expr, bool) {
	e, ok := fl.value(s, v.Held, []want{w}, v.Named.Obj().Name())
	if !ok {
		return expr{}, false
	}
	e = fl.convert(e, v.Held)
	ctor := fl.imports.Qualifier(v.Constructor.Pkg()) + "." + v.Constructor.Name()
	e.text = fl.orPanic + "(" + ctor + "(" + e.text + "))"
	e.typ = v.Named
	e.usesOrPanic = true
	return e, true
}

// firstConstant returns the first constant of type t declared in its package
// that is not the zero value, so that a sentinel such as StatusUnspecified is
// passed over. It has to be exported, since the fixtures live in another
// package.
func firstConstant(fset *token.FileSet, t *types.Named) *types.Const {
	if t.Obj().Pkg() == nil {
		return nil
	}
	for _, c := range enumtype.Constants(t, fset) {
		if c.Exported() && !isZero(c.Val()) {
			return c
		}
	}
	return nil
}

func isZero(v constant.Value) bool {
	switch v.Kind() {
	case constant.String:
		return constant.StringVal(v) == ""
	case constant.Bool:
		return !constant.BoolVal(v)
	case constant.Int, constant.Float, constant.Complex, constant.Unknown:
	}
	// An unknown value has the sign 1.
	return constant.Sign(v) == 0
}

// str works out the value of a string of type t: the first choice, a value the
// faker makes from the pattern, or the name of the field numbered, in the
// format its rules or its name ask for when a value of it fits.
func (fl *filler) str(t types.Type, w want, name string) expr {
	format, fits := formatOf(w, name)
	switch {
	case len(w.choices) > 0:
		return expr{text: strconv.Quote(w.choices[0])}
	case w.pattern != "":
		lit := strconv.Quote(w.pattern)
		if strconv.CanBackquote(w.pattern) {
			lit = "`" + w.pattern + "`"
		}
		return fl.convert(expr{text: fl.f + ".Regex(" + lit + ")", typ: types.Typ[types.String], usesF: true}, t)
	case format == "uuid" && fits:
		return fl.convert(fl.sprintf("00000000-0000-4000-8000-%012x", fl.n), t)
	case w.maxLen == 0:
		return expr{text: `""`}
	case !fits:
		// Numbered within the length alone; unkept has warned.
		format = ""
	}

	f := formats[format]
	fixed := len(f.prefix + f.suffix)
	maxLen := w.maxLen
	if maxLen >= 0 {
		maxLen -= fixed
	}
	verb, mod := numbered(kebab(name), w.minLen-fixed, maxLen)
	arg := fl.n
	if mod > 0 {
		arg += "%" + strconv.FormatInt(mod, 10)
	}
	return fl.convert(fl.sprintf(f.prefix+verb+f.suffix, arg), t)
}

// formats are the strings a rule or the name of a field asks for, by the name
// of the rule: what to call one, and what stands around the numbered name in
// one. A UUID has nothing around its number, which is written in full.
var formats = map[string]struct{ what, prefix, suffix string }{
	"email": {what: "a numbered email address", suffix: "@example.com"},
	"url":   {what: "a numbered URL", prefix: "https://example.com/"},
	"uuid":  {what: "a UUID"},
}

// formatOf returns the format a string takes: the one its rules name, or the
// one the name of its field ends in; and whether a value of it fits the length
// the rules set, which a UUID, 36 characters long, or an address or a URL,
// needing a digit beside its fixed part, may not.
func formatOf(w want, name string) (string, bool) {
	format := w.format
	switch {
	case format != "":
	case strings.HasSuffix(name, "Email"):
		format = "email"
	case strings.HasSuffix(name, "URL"):
		format = "url"
	case strings.HasSuffix(name, "UUID"):
		format = "uuid"
	}
	switch f := formats[format]; format {
	case "":
		return "", true
	case "uuid":
		return format, w.minLen <= 36 && (w.maxLen < 0 || w.maxLen >= 36)
	default:
		return format, w.maxLen < 0 || w.maxLen > len(f.prefix+f.suffix)
	}
}

// unmatched says what is wrong with the values the faker makes from the
// pattern of w, as far as a few tries tell: one does not match it, or does not
// fit the length the other rules of w set. It is "" when nothing is. gofakeit
// does not make every expression: it gives up on a long repeat such as
// a{1000}, and repeats + and * ten times at most.
func unmatched(re *regexp.Regexp, w want) string {
	for seed := range uint64(8) {
		s := gofakeit.New(seed + 1).Regex(w.pattern)
		switch n := utf8.RuneCountInString(s); {
		case !re.MatchString(s):
			return "values generated from the pattern do not match it"
		case n < w.minLen, w.maxLen >= 0 && n > w.maxLen:
			return "values generated from the pattern do not fit within the length the rules set"
		}
	}
	return ""
}

// numbered returns the format of a string numbered by the fixture, such as
// "slug-%d", of a length between minLen and maxLen, with -1 for no maximum. The
// name gives way to the number where both do not fit, and the number is to be
// taken modulo mod, when mod is not 0, where its digits would not fit.
func numbered(name string, minLen, maxLen int) (string, int64) {
	base := name + "-"
	if maxLen >= 0 && utf8.RuneCountInString(base)+4 > maxLen {
		base = ""
		if maxLen >= 6 {
			base = strings.TrimRight(string([]rune(name)[:maxLen-5]), "-") + "-"
		}
	}

	var mod int64
	if room := maxLen - utf8.RuneCountInString(base); maxLen >= 0 && room < 10 {
		mod = int64(math.Pow10(room))
	}
	if width := minLen - utf8.RuneCountInString(base); width > 1 {
		return base + "%0" + strconv.Itoa(width) + "d", mod
	}
	return base + "%d", mod
}

// sprintf formats arg with format, as a string the fixture numbers.
func (fl *filler) sprintf(format, arg string) expr {
	return expr{
		text:  fl.imports.Import("fmt", "fmt") + ".Sprintf(" + strconv.Quote(format) + ", " + arg + ")",
		typ:   types.Typ[types.String],
		usesN: true,
	}
}

// integer works out the value of an integer of type t and kind b: the exact
// value its rules give, or the number of the fixture, counted from their lower
// bound, 1 when they set none, and wrapped within their upper bound or what
// the type holds.
func (fl *filler) integer(t types.Type, b *types.Basic, w want) expr {
	if w.exact != nil {
		return expr{text: w.exact.String()}
	}

	low, high := int64(1), maxOf(b)
	if w.low != nil {
		l, exact := constant.Int64Val(w.low)
		if !exact {
			return fl.beyondInt64(t, w)
		}
		low = l
	}
	// An upper bound beyond what the int64 number of a fixture holds is passed
	// over: the number never reaches it.
	if w.high != nil {
		if h, exact := constant.Int64Val(w.high); exact {
			high = h
		}
	}

	var text string
	switch {
	case low == high:
		return expr{text: strconv.FormatInt(low, 10)}
	case high == math.MaxInt64 && low > 1:
		text = fl.n + " + " + strconv.FormatInt(low-1, 10)
	case high == math.MaxInt64 && low < 1:
		text = fl.n + " - " + strconv.FormatInt(1-low, 10)
	case high == math.MaxInt64:
		text = fl.n
	default:
		text = fmt.Sprintf("%d + (%s-1)%%%d", low, fl.n, high-low+1-int64(len(w.skip)))
	}
	e := expr{text: text, typ: types.Typ[types.Int64], usesN: true}
	if len(w.skip) > 0 {
		args := []string{text}
		for _, v := range w.skip {
			args = append(args, strconv.FormatInt(v, 10))
		}
		e.text, e.skips = fl.skipping+"("+strings.Join(args, ", ")+")", true
	}
	return fl.convert(e, t)
}

// beyondInt64 works out the value of an integer whose lower bound lies beyond
// what the int64 number of a fixture holds, which only an unsigned type of 64
// bits can have: the number counted from that bound in uint64, and wrapped
// within the upper bound, or what the type holds, so that it never overflows.
func (fl *filler) beyondInt64(t types.Type, w want) expr {
	high := w.high
	if high == nil {
		high = constant.MakeUint64(math.MaxUint64)
	}
	if constant.Compare(high, token.EQL, w.low) {
		return expr{text: w.low.String()}
	}
	one := constant.MakeInt64(1)
	span := constant.BinaryOp(constant.BinaryOp(high, token.SUB, w.low), token.ADD, one)
	n := fl.convert(expr{text: fl.n, typ: types.Typ[types.Int64]}, types.Typ[types.Uint64]).text
	text := fmt.Sprintf("%s + (%s-1)%%%s", w.low, n, span)
	return fl.convert(expr{text: text, typ: types.Typ[types.Uint64], usesN: true}, t)
}

// narrowMax holds the largest value of each integer kind narrower than the
// int64 number of a fixture.
var narrowMax = map[types.BasicKind]int64{
	types.Int8:   math.MaxInt8,
	types.Int16:  math.MaxInt16,
	types.Int32:  math.MaxInt32,
	types.Uint8:  math.MaxUint8,
	types.Uint16: math.MaxUint16,
	types.Uint32: math.MaxUint32,
}

// maxOf returns the largest value a fixture counts to in an integer of kind b:
// what it holds, or what the int64 number of the fixture holds.
func maxOf(b *types.Basic) int64 {
	if m, ok := narrowMax[b.Kind()]; ok {
		return m
	}
	return math.MaxInt64
}

// floatLiteral spells v as the generated code writes a float: in full, where
// constant.Value.String rounds to six digits, which could cross a bound.
func floatLiteral(v constant.Value) string {
	f, _ := constant.Float64Val(v)
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// nameable reports whether a package other than the one declaring t can spell
// it, as the fixtures spell a type they convert to, point to, or make. An
// unexported type is known by its name only in its own package, and so is a
// struct type written out with an unexported field.
func nameable(t types.Type) bool {
	switch u := t.(type) {
	case *types.Named:
		return nameableName(u.Obj(), u.TypeArgs())
	case *types.Alias:
		return nameableName(u.Obj(), u.TypeArgs())
	case *types.Map:
		return nameable(u.Key()) && nameable(u.Elem())
	case interface{ Elem() types.Type }:
		// A pointer, slice, array, or channel.
		return nameable(u.Elem())
	case *types.Struct:
		for f := range u.Fields() {
			if !f.Exported() || !nameable(f.Type()) {
				return false
			}
		}
	case *types.Signature:
		return nameable(u.Params()) && nameable(u.Results())
	case *types.Tuple:
		for v := range u.Variables() {
			if !nameable(v.Type()) {
				return false
			}
		}
	case *types.Interface:
		for m := range u.ExplicitMethods() {
			if !m.Exported() || !nameable(m.Type()) {
				return false
			}
		}
		for e := range u.EmbeddedTypes() {
			if !nameable(e) {
				return false
			}
		}
	}
	return true
}

// nameableName reports whether other packages can spell the type obj names,
// with the arguments args of an instance: it is exported, or declared in the
// universe as error is, and so are they.
func nameableName(obj *types.TypeName, args *types.TypeList) bool {
	if obj.Pkg() != nil && !obj.Exported() {
		return false
	}
	for a := range args.Types() {
		if !nameable(a) {
			return false
		}
	}
	return true
}

// convert converts e to t where their types differ. An untyped constant needs
// no conversion.
func (fl *filler) convert(e expr, t types.Type) expr {
	if e.typ == nil || types.Identical(e.typ, t) {
		return e
	}
	e.text = types.TypeString(t, fl.imports.Qualifier) + "(" + e.text + ")"
	e.typ = t
	return e
}

// pointer points to e, a value of type t. An untyped constant is converted to t
// first, unless t is the type new would give it anyway: new(1) makes an *int.
func (fl *filler) pointer(e expr, t types.Type) expr {
	if e.typ == nil {
		e.typ = defaultOf(e.text)
		e = fl.convert(e, t)
	}
	e.text = "new(" + e.text + ")"
	e.typ = types.NewPointer(t)
	return e
}

// defaultOf returns the type of lit, an untyped constant as the generated code
// writes it, where nothing else decides it.
func defaultOf(lit string) types.Type {
	switch {
	case strings.HasPrefix(lit, `"`):
		return types.Typ[types.String]
	case lit == "true" || lit == "false":
		return types.Typ[types.Bool]
	case strings.ContainsAny(lit, ".eE"):
		return types.Typ[types.Float64]
	default:
		return types.Typ[types.Int]
	}
}

// reaches reports whether the fixture of from calls, however indirectly, the
// fixture of to, which would then never return if it called the fixture of
// from in turn.
func (fl *filler) reaches(from, to *types.TypeName) bool {
	seen := map[*types.TypeName]bool{}
	var walk func(*types.TypeName) bool
	walk = func(n *types.TypeName) bool {
		if n == to {
			return true
		}
		// A fixture written by hand calls what it calls, which is for its
		// author to keep from coming back.
		if named, ok := n.Type().(*types.Named); seen[n] || ok && fl.hasWritten(named) {
			return false
		}
		seen[n] = true
		st, _ := n.Type().Underlying().(*types.Struct)
		for i := range st.NumFields() {
			f, tag := st.Field(i), reflect.StructTag(st.Tag(i))
			if !f.Exported() {
				continue
			}
			if _, faked, _ := fakeOf(tag, f.Type()); faked {
				continue
			}
			if slices.ContainsFunc(fl.calls(f.Type(), wantsOf(tag, f.Type())), walk) {
				return true
			}
		}
		return false
	}
	return walk(from)
}

// calls returns the targets whose fixtures fill a value of type t, as the rules
// of wants have it.
func (fl *filler) calls(t types.Type, wants []want) []*types.TypeName {
	w := want{maxLen: -1}
	if len(wants) > 0 {
		w = wants[0]
	}
	switch u := t.Underlying().(type) {
	case *types.Pointer:
		if w.required {
			return fl.calls(u.Elem(), wants)
		}
	case *types.Slice:
		if w.elements() > 0 {
			return fl.calls(u.Elem(), rest(wants))
		}
	case *types.Struct:
		if n, ok := types.Unalias(t).(*types.Named); ok && fl.targets[n.Obj()] {
			return []*types.TypeName{n.Obj()}
		}
	}
	return nil
}

// namedPath returns the import path of the package that declares t, which has
// to be a named type of a package.
func namedPath(t types.Type) string {
	n, _ := types.Unalias(t).(*types.Named)
	return n.Obj().Pkg().Path()
}

// kebab spells the name of a field in kebab case, as it heads a string the
// fixture numbers: FirstName as first-name, UserID as user-id.
func kebab(name string) string {
	var b strings.Builder
	runes := []rune(name)
	for i, r := range runes {
		if unicode.IsUpper(r) {
			beforeLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if i > 0 && (!unicode.IsUpper(runes[i-1]) || beforeLower) {
				b.WriteByte('-')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}
