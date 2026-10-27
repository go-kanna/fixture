package gen

import (
	"errors"
	"fmt"
	"go/constant"
	"go/types"
	"math"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

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
		if !field.Exported {
			continue
		}
		if other, ok := copied(s, field); ok {
			out.copies = append(out.copies, assignment{field: field.Name, value: other})
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
		switch {
		case faked && fv.skip:
		case faked:
			e, ok = fl.fake(fv, deref(field.Type)), true
			if _, isPtr := field.Type.Underlying().(*types.Pointer); isPtr {
				e = fl.pointer(e, deref(field.Type))
			}
		default:
			wants := wantsOf(field.Tag, field.Type)
			diags = append(diags, dropUnmatched(field, wants)...)
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

// dropUnmatched clears from wants, the rules of field, each pattern the faker
// makes values that do not match, so that the field is numbered instead, and
// warns about it. One that does not compile is cleared without a word: that is
// for the validator to report.
func dropUnmatched(field scan.Field, wants []want) []diag.Diag {
	var diags []diag.Diag
	for i, w := range wants {
		if w.pattern == "" {
			continue
		}
		re, err := regexp.Compile(w.pattern)
		if err == nil && fakerMatches(re, w.pattern) {
			continue
		}
		if err == nil {
			diags = append(diags, diag.Warningf(field.Pos,
				"values generated from the pattern do not match it; give the field a fake tag (field %s)", field.Name))
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
			return expr{text: w.exact.String()}, true
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
// faker makes from the pattern, a format, or the name of the field numbered.
func (fl *filler) str(t types.Type, w want, name string) expr {
	k := kebab(name)
	switch format := formatOf(w.format, name); {
	case len(w.choices) > 0:
		return expr{text: strconv.Quote(w.choices[0])}
	case w.pattern != "":
		lit := strconv.Quote(w.pattern)
		if strconv.CanBackquote(w.pattern) {
			lit = "`" + w.pattern + "`"
		}
		return fl.convert(expr{text: fl.f + ".Regex(" + lit + ")", typ: types.Typ[types.String], usesF: true}, t)
	case format == "email":
		return fl.convert(fl.sprintf(k+"-%d@example.com", fl.n), t)
	case format == "url":
		return fl.convert(fl.sprintf("https://example.com/"+k+"-%d", fl.n), t)
	case format == "uuid":
		return fl.convert(fl.sprintf("00000000-0000-4000-8000-%012x", fl.n), t)
	case w.maxLen == 0:
		return expr{text: `""`}
	}

	format, mod := numbered(k, w.minLen, w.maxLen)
	arg := fl.n
	if mod > 0 {
		arg += "%" + strconv.FormatInt(mod, 10)
	}
	return fl.convert(fl.sprintf(format, arg), t)
}

// formatOf returns the format a string takes: the one its rules name, or the
// one the name of its field ends in.
func formatOf(rule, name string) string {
	switch {
	case rule != "":
		return rule
	case strings.HasSuffix(name, "Email"):
		return "email"
	case strings.HasSuffix(name, "URL"):
		return "url"
	case strings.HasSuffix(name, "UUID"):
		return "uuid"
	}
	return ""
}

// fakerMatches reports whether the values the faker makes from pattern match
// re, its compiled form, as far as a few tries tell. gofakeit does not make
// every expression: it gives up on a long repeat such as a{1000}.
func fakerMatches(re *regexp.Regexp, pattern string) bool {
	for seed := range uint64(8) {
		if !re.MatchString(gofakeit.New(seed + 1).Regex(pattern)) {
			return false
		}
	}
	return true
}

// numbered returns the format of a string numbered by the fixture, such as
// "slug-%d", of a length between minLen and maxLen, with -1 for no maximum. The
// name gives way to the number where both do not fit, and the number is to be
// taken modulo mod, when mod is not 0, where its digits would not fit.
func numbered(name string, minLen, maxLen int) (string, int64) {
	base := name + "-"
	if maxLen >= 0 && len(base)+4 > maxLen {
		base = ""
		if maxLen >= 6 {
			base = name[:maxLen-5] + "-"
		}
	}

	var mod int64
	if room := maxLen - len(base); maxLen >= 0 && room < 10 {
		mod = int64(math.Pow10(room))
	}
	if width := minLen - len(base); width > 1 {
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
		low, _ = constant.Int64Val(w.low)
	}
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
