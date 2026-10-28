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
	"github.com/go-kanna/internal/scan"
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
	// the fixture, its faker, and the helper calling Faker.Generate.
	usesN, usesF, generates bool
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
	// number of the fixture, its faker, and the helper calling Faker.Generate.
	usesN, usesF, generates bool
}

// filler works out the values of the fixtures of one generated file.
type filler struct {
	imports imports

	// targets are the structs that get a fixture, by the name of their type.
	targets map[*types.TypeName]bool

	// generate names the helper that calls Faker.Generate.
	generate string

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
			e, ok = fl.value(s, field.Type, wants, field.Name)
		}
		if !ok {
			continue
		}
		out.fields = append(out.fields, assignment{field: field.Name, value: e.text})
		out.usesN = out.usesN || e.usesN
		out.usesF = out.usesF || e.usesF
		out.generates = out.generates || e.generates
	}
	return out, diags
}

// unkept warns about the rules of field, wants, that the fixture cannot keep:
// a format no value of which fits the length the rules set, which str passes
// over, and a pattern the faker makes values from that do not match it or do
// not fit that length, which is cleared so that the field is numbered instead.
// A pattern that does not compile is cleared without a word: that is for the
// validator to report, as is a pattern or a format on what is not a string.
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
		if shapeOf(t) != shapeString {
			continue
		}
		if format, fits := formatOf(w, field.Name); !fits {
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
		k := w.minLen
		if w.required {
			k = max(k, 1)
		}
		if k == 0 {
			return expr{}, false
		}
		typ := types.TypeString(t, fl.imports.Qualifier)
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

// named works out the value of a named type that has one of its own: a struct
// with a fixture, time.Time, a UUID, or a type with constants declared beside
// it. It reports false for any other.
func (fl *filler) named(s scan.Struct, t *types.Named, w want) (expr, bool) {
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
	if c := firstConstant(t); c != nil {
		return expr{text: fl.imports.Qualifier(c.Pkg()) + "." + c.Name(), typ: t}, true
	}
	return expr{}, false
}

// firstConstant returns the first constant of type t declared in its package
// that is not the zero value, so that a sentinel such as StatusUnspecified is
// passed over. It has to be exported, since the fixtures live in another
// package.
func firstConstant(t *types.Named) *types.Const {
	pkg := t.Obj().Pkg()
	if pkg == nil {
		return nil
	}
	var consts []*types.Const
	for _, name := range pkg.Scope().Names() {
		c, ok := pkg.Scope().Lookup(name).(*types.Const)
		if ok && c.Exported() && types.Identical(c.Type(), t) && !isZero(c.Val()) {
			consts = append(consts, c)
		}
	}
	if len(consts) == 0 {
		return nil
	}
	return slices.MinFunc(consts, func(a, b *types.Const) int { return int(a.Pos() - b.Pos()) })
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
			base = string([]rune(name)[:maxLen-5]) + "-"
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
		text = fmt.Sprintf("%d + (%s-1)%%%d", low, fl.n, high-low+1)
	}
	return fl.convert(expr{text: text, typ: types.Typ[types.Int64], usesN: true}, t)
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
		if seen[n] {
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
		if w.required || w.minLen > 0 {
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
