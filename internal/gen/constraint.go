package gen

import (
	"go/constant"
	"go/token"
	"go/types"
	"iter"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// tagKey is the struct tag whose rules a fixture is filled to pass.
const tagKey = "validate"

// want is what the rules of one level of a validate tag ask of a value: the
// field itself, or after each dive, the elements one level down. Only the rules
// that decide the value a fixture fills are kept; whether a rule is right is
// for the validator to say.
type want struct {
	// required rules out the zero value, which a fixture would otherwise leave
	// in a bool, a pointer, or a slice.
	required bool

	// minLen and maxLen bound the length of a string, in runes, or the number of
	// elements of a slice or map. maxLen is -1 when nothing bounds it.
	minLen, maxLen int

	// choices are the strings a string may be, from oneof or eq. A fixture takes
	// the first.
	choices []string

	// low and high bound an integer, both inclusive, and are nil where the rules
	// set no bound; a fixture counts up from low, or from 1. exact is the one
	// number to fill instead: the one eq or oneof gives, the bound of a float, or
	// one that ne leaves when it cuts into the range.
	low, high, exact constant.Value

	field   string // eqfield: the field whose value it copies
	format  string // email, url, or uuid
	pattern string
}

// wantsOf reads the validate tag in tag, of a field of type t: what it asks of
// the field, then of the elements after each dive. It returns nil for a field
// without one.
//
// The tag is read as go-playground/validator and kanna-validate read it, and as
// leniently as possible. A rule kanna-fixture does not know is passed over, and
// so is one it cannot make sense of, such as max=abc: either is the validator's
// to report. Of alternatives such as email|url, the first is taken, which
// satisfies them all.
func wantsOf(tag reflect.StructTag, t types.Type) []want {
	s, ok := tag.Lookup(tagKey)
	if !ok {
		return nil
	}

	var (
		wants []want
		r     = newReader(t)
		keys  bool
	)
	for item := range items(s) {
		if alt, _, ok := strings.Cut(item, "|"); ok && !strings.HasPrefix(item, "pattern=") {
			item = alt
		}
		name, param, _ := strings.Cut(item, "=")
		switch {
		case name == "keys" || name == "endkeys":
			keys = name == "keys"
		case keys:
			// The rules for the keys of a map, which a fixture leaves nil.
		case name == "dive":
			wants = append(wants, r.want())
			elem, ok := elemOf(r.typ)
			if !ok {
				return wants
			}
			r = newReader(elem)
		default:
			r.add(name, param)
		}
	}
	return append(wants, r.want())
}

// items splits a validate tag into its rules. They are separated by commas,
// except that pattern, which only kanna-validate has, takes the rest of the tag
// as its expression, commas and all.
func items(s string) iter.Seq[string] {
	return func(yield func(string) bool) {
		for rest := s; rest != ""; {
			var item string
			if strings.HasPrefix(rest, "pattern=") {
				item, rest = rest, ""
			} else {
				item, rest, _ = strings.Cut(rest, ",")
			}
			if !yield(item) {
				return
			}
		}
	}
}

// shape is what the rules of a value mean by min and max.
type shape int

const (
	shapeOther shape = iota
	shapeString
	shapeInteger
	shapeFloat
	shapeCollection
)

func shapeOf(t types.Type) shape {
	switch u := deref(t).Underlying().(type) {
	case *types.Basic:
		switch info := u.Info(); {
		case info&types.IsString != 0:
			return shapeString
		case info&types.IsInteger != 0:
			return shapeInteger
		case info&types.IsFloat != 0:
			return shapeFloat
		}
	case *types.Slice, *types.Array, *types.Map:
		return shapeCollection
	}
	return shapeOther
}

// deref strips the pointers from t: the rules of a pointer are for what it
// points to, except required, which is for the pointer.
func deref(t types.Type) types.Type {
	for {
		p, ok := t.Underlying().(*types.Pointer)
		if !ok {
			return t
		}
		t = p.Elem()
	}
}

// elemOf returns the type the rules after a dive are for: the elements of a
// slice or array, or the values of a map.
func elemOf(t types.Type) (types.Type, bool) {
	switch u := deref(t).Underlying().(type) {
	case *types.Slice:
		return u.Elem(), true
	case *types.Array:
		return u.Elem(), true
	case *types.Map:
		return u.Elem(), true
	}
	return nil, false
}

// bound is a lower or upper bound on a number, as a rule writes it.
type bound struct {
	value constant.Value // nil when no rule sets it
	open  bool           // gt or lt, which exclude the bound itself
}

// under reports whether v keeps within b, taken as an upper bound.
func (b bound) under(v constant.Value) bool {
	switch {
	case b.value == nil:
		return true
	case b.open:
		return constant.Compare(v, token.LSS, b.value)
	default:
		return constant.Compare(v, token.LEQ, b.value)
	}
}

// reader collects the rules of one level, for a value of type typ.
type reader struct {
	typ   types.Type
	shape shape
	w     want

	// For a number: its bounds, the values eq and oneof give it, and those ne
	// rules out.
	lower, upper bound
	equal        []string
	not          []constant.Value
}

func newReader(t types.Type) *reader {
	return &reader{typ: t, shape: shapeOf(t), w: want{maxLen: -1}}
}

// add takes in one rule.
func (r *reader) add(name, param string) {
	switch name {
	case "required":
		r.w.required = true
	case "email", "url", "uuid":
		r.w.format = name
	case "pattern":
		r.w.pattern = param
	case "eqfield":
		r.w.field = param
	case "oneof":
		if r.shape == shapeString {
			r.w.choices = choices(param)
		} else {
			r.equal = choices(param)
		}
	case "min", "max", "len", "eq", "ne", "gt", "gte", "lt", "lte":
		switch r.shape {
		case shapeString, shapeCollection:
			r.length(name, param)
		case shapeInteger, shapeFloat:
			r.number(name, param)
		case shapeOther:
		}
	}
}

// length takes in a rule on the length of a string or a collection. On a
// string, eq and ne compare the value instead, as go-playground has it.
func (r *reader) length(name, param string) {
	if r.shape == shapeString && (name == "eq" || name == "ne") {
		if name == "eq" {
			r.w.choices = []string{param}
		}
		return
	}
	n, err := strconv.Atoi(param)
	if err != nil || n < 0 {
		return
	}
	switch name {
	case "min", "gte":
		r.w.minLen = n
	case "gt":
		r.w.minLen = n + 1
	case "max", "lte":
		r.w.maxLen = n
	case "lt":
		r.w.maxLen = max(n-1, 0)
	case "len", "eq":
		r.w.minLen, r.w.maxLen = n, n
	}
}

// number takes in a rule on the value of a number.
func (r *reader) number(name, param string) {
	v := parseNumber(param)
	if v == nil {
		return
	}
	switch name {
	case "min", "gte":
		r.lower = bound{value: v}
	case "gt":
		r.lower = bound{value: v, open: true}
	case "max", "lte":
		r.upper = bound{value: v}
	case "lt":
		r.upper = bound{value: v, open: true}
	case "len", "eq":
		r.equal = []string{param}
	case "ne":
		r.not = append(r.not, v)
	}
}

// want finishes the level.
func (r *reader) want() want {
	b, ok := deref(r.typ).Underlying().(*types.Basic)
	switch {
	case !ok:
	case r.shape == shapeInteger:
		r.integer(b)
	case r.shape == shapeFloat:
		r.float(b)
	}
	return r.w
}

// integer works out the range of an integer of type b, or the one value to
// fill it with.
//
// A fixture counts up from low, wrapping within high, so that the numbers stay
// unique as far as the range lets them: gt=0 on an ID gives 1, 2, 3.
func (r *reader) integer(b *types.Basic) {
	if len(r.equal) > 0 {
		if v := parseNumber(r.equal[0]); v != nil && isInteger(v) && fits(v, b) {
			r.w.exact = v
		}
		return
	}

	var low, high constant.Value
	if r.lower.value != nil {
		low = ceil(r.lower.value, r.lower.open)
	}
	if r.upper.value != nil {
		high = floor(r.upper.value, r.upper.open)
	}
	if low != nil && !fits(low, b) {
		low = nil
	}
	if high != nil && !fits(high, b) {
		high = nil
	}
	if low != nil && high != nil && constant.Compare(low, token.GTR, high) {
		return
	}

	one := constant.MakeInt64(1)
	if low == nil && high != nil {
		if constant.Compare(high, token.LSS, one) {
			r.w.exact = high
			return
		}
		low = one
	}

	start := low
	if start == nil {
		start = one
	}
	cut := slices.ContainsFunc(r.not, func(v constant.Value) bool {
		return constant.Compare(v, token.GEQ, start) && (high == nil || constant.Compare(v, token.LEQ, high))
	})
	if cut {
		for v := start; high == nil || constant.Compare(v, token.LEQ, high); v = constant.BinaryOp(v, token.ADD, one) {
			if !slices.ContainsFunc(r.not, func(n constant.Value) bool { return constant.Compare(n, token.EQL, v) }) {
				r.w.exact = v
				break
			}
		}
		return
	}
	r.w.low, r.w.high = low, high
}

// float works out the one value to fill a float of type b with, when its rules
// bound it: the lowest value they allow, or the highest when they set no lower
// bound. A float is seldom an identity, so it is not counted up.
func (r *reader) float(b *types.Basic) {
	var v constant.Value
	switch {
	case len(r.equal) > 0:
		v = parseNumber(r.equal[0])
	case r.lower.value != nil:
		v = r.lower.value
		if r.lower.open {
			v = constant.BinaryOp(v, token.ADD, constant.MakeInt64(1))
			if !r.upper.under(v) {
				v = constant.BinaryOp(constant.BinaryOp(r.lower.value, token.ADD, r.upper.value), token.QUO, constant.MakeInt64(2))
			}
		}
	case r.upper.value != nil:
		v = r.upper.value
		if r.upper.open {
			v = constant.BinaryOp(v, token.SUB, constant.MakeInt64(1))
		}
	}
	if v != nil && fits(v, b) {
		r.w.exact = v
	}
}

// choices splits the options of oneof, which are separated by spaces, unless
// an option is quoted in single quotes.
func choices(param string) []string {
	var out []string
	for rest := strings.TrimSpace(param); rest != ""; rest = strings.TrimSpace(rest) {
		if quoted, after, ok := strings.Cut(rest[1:], "'"); rest[0] == '\'' && ok {
			out = append(out, quoted)
			rest = after
			continue
		}
		var option string
		option, rest, _ = strings.Cut(rest, " ")
		out = append(out, option)
	}
	return out
}

// parseNumber reads a number as a rule writes it, such as -5 or 0.5, or returns
// nil.
func parseNumber(s string) constant.Value {
	digits := strings.TrimPrefix(s, "-")
	for _, tok := range []token.Token{token.INT, token.FLOAT} {
		if v := constant.MakeFromLiteral(digits, tok, 0); v.Kind() != constant.Unknown {
			if digits != s {
				v = constant.UnaryOp(token.SUB, v, 0)
			}
			return v
		}
	}
	return nil
}

func isInteger(v constant.Value) bool {
	return constant.ToInt(v).Kind() == constant.Int
}

// ceil returns the least integer at or above v, or above it when open.
func ceil(v constant.Value, open bool) constant.Value {
	if i := constant.ToInt(v); i.Kind() == constant.Int {
		if open {
			return constant.BinaryOp(i, token.ADD, constant.MakeInt64(1))
		}
		return i
	}
	f, _ := constant.Float64Val(v)
	return constant.ToInt(constant.MakeFloat64(math.Ceil(f)))
}

// floor returns the greatest integer at or below v, or below it when open.
func floor(v constant.Value, open bool) constant.Value {
	if i := constant.ToInt(v); i.Kind() == constant.Int {
		if open {
			return constant.BinaryOp(i, token.SUB, constant.MakeInt64(1))
		}
		return i
	}
	f, _ := constant.Float64Val(v)
	return constant.ToInt(constant.MakeFloat64(math.Floor(f)))
}

// fits reports whether a value of type b can hold v. int, uint, and uintptr
// are taken to be 64 bits wide, as kanna-validate takes them.
func fits(v constant.Value, b *types.Basic) bool {
	if b.Info()&types.IsFloat != 0 {
		f, _ := constant.Float64Val(constant.ToFloat(v))
		if b.Kind() == types.Float32 {
			return math.Abs(f) <= math.MaxFloat32
		}
		return !math.IsInf(f, 0)
	}

	// A value that is not an integer converts to Unknown, which no Int64Val or
	// Uint64Val gives exactly.
	n := constant.ToInt(v)
	bits := 8 * types.SizesFor("gc", "amd64").Sizeof(b)
	if b.Info()&types.IsUnsigned != 0 {
		u, exact := constant.Uint64Val(n)
		return exact && u <= math.MaxUint64>>(64-bits)
	}
	i, exact := constant.Int64Val(n)
	lo := int64(-1) << (bits - 1)
	return exact && lo <= i && i <= -(lo+1)
}
