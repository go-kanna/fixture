package gen_test

import (
	"go/types"
	"slices"
	"testing"

	"github.com/go-kanna/fixture/internal/gen"
)

func TestWants(t *testing.T) {
	t.Parallel()

	var (
		str   = types.Typ[types.String]
		i     = types.Typ[types.Int]
		i64   = types.Typ[types.Int64]
		u8    = types.Typ[types.Uint8]
		f64   = types.Typ[types.Float64]
		f32   = types.Typ[types.Float32]
		b     = types.Typ[types.Bool]
		ptr   = types.NewPointer(str)
		pkg   = types.NewPackage("example.com/model", "model")
		level = types.NewNamed(types.NewTypeName(0, pkg, "Level", nil), u8, nil)
	)

	tests := []struct {
		name string
		tag  string
		typ  types.Type
		want []string
	}{
		{name: "no validate tag", tag: `json:"name"`, typ: str, want: nil},
		{name: "a tag that does not unquote", tag: `validate:"pattern=^\d$"`, typ: str, want: nil},
		{name: "empty", tag: `validate:""`, typ: str, want: []string{""}},

		{name: "required", tag: `validate:"required"`, typ: b, want: []string{"required"}},
		{name: "max on a string", tag: `validate:"required,max=50"`, typ: str, want: []string{"required len=0..50"}},
		{name: "min on a string", tag: `validate:"min=12"`, typ: str, want: []string{"len=12.."}},
		{name: "len on a string", tag: `validate:"len=3"`, typ: str, want: []string{"len=3..3"}},
		{name: "go-playground's gt and lt on a string", tag: `validate:"gt=2,lt=10"`, typ: str, want: []string{"len=3..9"}},
		{name: "eq on a string is its value", tag: `validate:"eq=fixed"`, typ: str, want: []string{"oneof=fixed"}},
		{name: "ne on a string", tag: `validate:"ne=x"`, typ: str, want: []string{""}},
		{name: "oneof", tag: `validate:"oneof=admin member guest"`, typ: str, want: []string{"oneof=admin|member|guest"}},
		{
			name: "ne passes over a choice",
			tag:  `validate:"oneof=admin member,ne=admin"`,
			typ:  str,
			want: []string{"oneof=member"},
		},
		{name: "ne before oneof", tag: `validate:"ne=admin,oneof=admin member"`, typ: str, want: []string{"oneof=member"}},
		{name: "quoted oneof", tag: `validate:"oneof='red green' blue"`, typ: str, want: []string{"oneof=red green|blue"}},
		{name: "formats", tag: `validate:"omitempty,url"`, typ: str, want: []string{"url"}},
		{name: "alternatives take the first", tag: `validate:"email|url"`, typ: str, want: []string{"email"}},
		{name: "eqfield", tag: `validate:"eqfield=Password"`, typ: str, want: []string{"eqfield=Password"}},
		{
			name: "pattern takes the rest of the tag",
			tag:  `validate:"required,pattern=^\\d{1,3}|x$"`,
			typ:  str,
			want: []string{`required pattern=^\d{1,3}|x$`},
		},
		{
			name: "rules it does not know, or cannot read",
			tag:  `validate:"alphanum,required_if=Kind a,max=abc,min=-1,min=2"`,
			typ:  str,
			want: []string{"len=2.."},
		},
		{name: "a pointer", tag: `validate:"required,max=20"`, typ: ptr, want: []string{"required len=0..20"}},

		{name: "a range", tag: `validate:"gte=18,lte=130"`, typ: i, want: []string{"low=18 high=130"}},
		{name: "min and max on a number", tag: `validate:"min=1,max=5"`, typ: i, want: []string{"low=1 high=5"}},
		{name: "a lower bound alone", tag: `validate:"required,gt=0"`, typ: i64, want: []string{"required low=1"}},
		{name: "an upper bound alone counts from 1", tag: `validate:"lt=10"`, typ: i, want: []string{"low=1 high=9"}},
		{name: "an upper bound below 1", tag: `validate:"lte=-5"`, typ: i, want: []string{"exact=-5"}},
		{name: "a bound between integers", tag: `validate:"gt=1.5,lt=4.5"`, typ: i, want: []string{"low=2 high=4"}},
		{name: "eq", tag: `validate:"eq=7"`, typ: i, want: []string{"exact=7"}},
		{name: "eq written as a float", tag: `validate:"eq=1234567.0"`, typ: i, want: []string{"exact=1234567"}},
		{name: "oneof on a number", tag: `validate:"oneof=3 5"`, typ: i, want: []string{"exact=3"}},
		{name: "oneof with ne", tag: `validate:"oneof=3 5,ne=3"`, typ: i, want: []string{"exact=5"}},
		{name: "ne below where it counts from", tag: `validate:"ne=0"`, typ: i, want: []string{""}},
		{name: "ne in the range", tag: `validate:"gte=1,lte=3,ne=1,ne=2"`, typ: i, want: []string{"exact=3"}},
		{name: "ne with nothing left", tag: `validate:"gte=1,lte=1,ne=1"`, typ: i, want: []string{""}},
		{name: "a range the wrong way round", tag: `validate:"gte=5,lte=1"`, typ: i, want: []string{""}},
		{name: "a bound the type cannot hold", tag: `validate:"gte=300"`, typ: u8, want: []string{""}},
		{name: "an upper bound the type cannot hold", tag: `validate:"lte=300"`, typ: u8, want: []string{""}},
		{name: "a bound that is not a number", tag: `validate:"gte=abc"`, typ: i, want: []string{""}},
		{name: "a named integer", tag: `validate:"lte=200"`, typ: level, want: []string{"low=1 high=200"}},
		{name: "eq that is not an integer", tag: `validate:"eq=1.5"`, typ: i, want: []string{""}},
		{name: "required counts from 1", tag: `validate:"required,gte=0"`, typ: i, want: []string{"required low=1"}},
		{
			name: "required below 0",
			tag:  `validate:"required,gte=-5,lte=0"`,
			typ:  i,
			want: []string{"required low=-5 high=-1"},
		},
		{name: "required bounded above by 0", tag: `validate:"required,lte=0"`, typ: i, want: []string{"required exact=-1"}},
		{name: "required below 0 on unsigned", tag: `validate:"required,lte=0"`, typ: u8, want: []string{"required"}},
		{
			name: "required passes over a choice",
			tag:  `validate:"required,oneof=0 1"`,
			typ:  i,
			want: []string{"required exact=1"},
		},

		{name: "a lower bound on a float", tag: `validate:"gt=0"`, typ: f64, want: []string{"exact=1"}},
		{name: "a narrow range on a float", tag: `validate:"gt=0,lt=0.5"`, typ: f64, want: []string{"exact=0.25"}},
		{name: "a narrow closed range on a float", tag: `validate:"gt=0,lte=0.5"`, typ: f64, want: []string{"exact=0.25"}},
		{name: "an upper bound on a float", tag: `validate:"lt=1"`, typ: f64, want: []string{"exact=0"}},
		{name: "eq on a float", tag: `validate:"eq=2.5"`, typ: f64, want: []string{"exact=2.5"}},
		{name: "an unbounded float", tag: `validate:"required"`, typ: f64, want: []string{"required"}},
		{name: "required on a float from 0", tag: `validate:"required,gte=0"`, typ: f64, want: []string{"required exact=1"}},
		{
			name: "required on a float within a narrow range",
			tag:  `validate:"required,gte=0,lte=0.5"`,
			typ:  f64,
			want: []string{"required exact=0.25"},
		},
		{
			name: "required on a float with no room",
			tag:  `validate:"required,gte=0,lte=0"`,
			typ:  f64,
			want: []string{"required exact=0"},
		},
		{name: "required on a float up to 1", tag: `validate:"required,lt=1"`, typ: f64, want: []string{"required exact=-1"}},
		{name: "ne on a float bounded below", tag: `validate:"gt=0,ne=1"`, typ: f64, want: []string{"exact=2"}},
		{name: "ne on a float bounded above", tag: `validate:"lte=5,ne=5"`, typ: f64, want: []string{"exact=4"}},
		{name: "ne within a narrow range", tag: `validate:"gt=0,lt=1,ne=0.5"`, typ: f64, want: []string{"exact=0.75"}},
		{name: "ne where a float counts", tag: `validate:"ne=1"`, typ: f64, want: []string{"exact=2"}},
		{name: "ne below where a float counts from", tag: `validate:"ne=0"`, typ: f64, want: []string{""}},
		{name: "a float32", tag: `validate:"gte=1.5"`, typ: f32, want: []string{"exact=1.5"}},
		{name: "a bound a float32 cannot hold", tag: `validate:"gte=1e39"`, typ: f32, want: []string{""}},

		{name: "length of a slice", tag: `validate:"min=2"`, typ: types.NewSlice(str), want: []string{"len=2.."}},
		{name: "eq on a slice is its length", tag: `validate:"eq=3"`, typ: types.NewSlice(str), want: []string{"len=3..3"}},
		{
			name: "dive",
			tag:  `validate:"required,dive,email"`,
			typ:  types.NewSlice(str),
			want: []string{"required", "email"},
		},
		{
			name: "dive twice, through pointers",
			tag:  `validate:"dive,dive,gte=1"`,
			typ:  types.NewSlice(types.NewPointer(types.NewArray(i, 2))),
			want: []string{"", "", "low=1"},
		},
		{
			name: "the keys of a map are passed over",
			tag:  `validate:"dive,keys,min=1,endkeys,max=3"`,
			typ:  types.NewMap(str, str),
			want: []string{"", "len=0..3"},
		},
		{name: "dive into what has no elements", tag: `validate:"dive,max=3"`, typ: str, want: []string{""}},
		{name: "length rules on a bool", tag: `validate:"min=1"`, typ: b, want: []string{""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := gen.Wants(tt.tag, tt.typ); !slices.Equal(got, tt.want) {
				t.Errorf("Wants(%s, %s) = %q, want %q", tt.tag, tt.typ, got, tt.want)
			}
		})
	}
}
