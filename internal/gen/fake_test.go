package gen_test

import (
	"go/types"
	"strings"
	"testing"

	"github.com/go-kanna/fixture/internal/gen"
)

func TestFake(t *testing.T) {
	t.Parallel()

	var (
		str   = types.Typ[types.String]
		i     = types.Typ[types.Int]
		i8    = types.Typ[types.Int8]
		u     = types.Typ[types.Uint]
		u8    = types.Typ[types.Uint8]
		f32   = types.Typ[types.Float32]
		f64   = types.Typ[types.Float64]
		b     = types.Typ[types.Bool]
		named = func(path, name string, u types.Type) types.Type {
			pkg := types.NewPackage(path, path[strings.LastIndex(path, "/")+1:])
			return types.NewNamed(types.NewTypeName(0, pkg, name, nil), u, nil)
		}
		level = named("example.com/model", "Level", i)
		code  = named("example.com/model", "Code", str)
		tm    = named("time", "Time", types.NewStruct(nil, nil))
		bytes = types.NewArray(types.Typ[types.Byte], 16)
		gUUID = named("github.com/google/uuid", "UUID", bytes)
		stdID = named("uuid", "UUID", bytes)
		nopkg = named("example.com/model", "Address", types.NewStruct(nil, nil))
	)

	tests := []struct {
		name    string
		tag     string
		typ     types.Type
		want    string
		wantErr string
	}{
		{name: "no fake tag", tag: `json:"name"`, typ: str},
		{name: "an empty fake tag", tag: `fake:""`, typ: str},
		{name: "skip", tag: `fake:"skip"`, typ: str, want: "skip"},
		{name: "dash", tag: `fake:"-"`, typ: tm, want: "skip"},

		{name: "a fixed string", tag: `fake:"123-4567"`, typ: str, want: `fixed="123-4567"`},
		{name: "a fixed named string", tag: `fake:"A1"`, typ: code, want: `fixed="A1"`},
		{name: "functions", tag: `fake:"{firstname} {lastname}"`, typ: str, want: "template={firstname} {lastname}"},
		{name: "letters and digits", tag: `fake:"???-####"`, typ: str, want: "template=???-####"},
		{name: "braces inside a parameter", tag: `fake:"{regex:[a-z]{3}}"`, typ: str, want: "template={regex:[a-z]{3}}"},
		{name: "through a pointer", tag: `fake:"{email}"`, typ: types.NewPointer(str), want: "template={email}"},
		{
			name:    "a misspelled function",
			tag:     `fake:"{fristname}"`,
			typ:     str,
			wantErr: "unknown function \"fristname\" in fake tag \"{fristname}\"\n  did you mean \"firstname\"?",
		},
		{
			name:    "a function nothing is close to",
			tag:     `fake:"{zzzzzzzzzz}"`,
			typ:     str,
			wantErr: `unknown function "zzzzzzzzzz" in fake tag "{zzzzzzzzzz}"`,
		},
		{
			name:    "an unclosed brace",
			tag:     `fake:"Hi {firstname"`,
			typ:     str,
			wantErr: `fake tag "Hi {firstname": no } closes the { of {firstname`,
		},
		{
			name:    "parameters to a function that takes none",
			tag:     `fake:"{sentence:5}"`,
			typ:     str,
			wantErr: `fake tag "{sentence:5}": sentence takes no parameters, got 1`,
		},
		{
			name:    "too many parameters",
			tag:     `fake:"{number:1,2,3}"`,
			typ:     str,
			wantErr: `fake tag "{number:1,2,3}": number takes 2 parameters, got 3`,
		},
		{
			name:    "a parameter gofakeit cannot read",
			tag:     `fake:"{number:a,b}"`,
			typ:     str,
			wantErr: `fake tag "{number:a,b}": `,
		},
		{
			name: "brackets keep commas",
			tag:  `fake:"{randomstring:[a,b,c]}"`,
			typ:  str,
			want: "template={randomstring:[a,b,c]}",
		},

		{name: "number", tag: `fake:"{number:18,65}"`, typ: i, want: "Number(18, 65)"},
		{name: "number with spaces", tag: `fake:"{number: 1, 10}"`, typ: i, want: "Number(1, 10)"},
		{name: "number on a named int", tag: `fake:"{number:1,5}"`, typ: level, want: "Number(1, 5)"},
		{name: "number on a float", tag: `fake:"{number:1,5}"`, typ: f64, want: "Number(1, 5)"},
		{name: "number's defaults", tag: `fake:"{number}"`, typ: i, want: "Number(-2147483648, 2147483647)"},
		{name: "defaults too wide for the field", tag: `fake:"{number}"`, typ: i8, wantErr: "-2147483648 does not fit int8"},
		{
			name:    "a range too wide for the field",
			tag:     `fake:"{number:1,300}"`,
			typ:     i8,
			wantErr: `fake tag "{number:1,300}": 300 does not fit int8`,
		},
		{name: "a negative range on unsigned", tag: `fake:"{intrange:-5,5}"`, typ: u8, wantErr: "-5 does not fit uint8"},
		{name: "a negative uintrange", tag: `fake:"{uintrange:-1,10}"`, typ: u, wantErr: "-1 does not fit uint"},
		{name: "uintrange", tag: `fake:"{uintrange:1,10}"`, typ: u, want: "UintRange(1, 10)"},
		{
			name:    "fractions to an integer function",
			tag:     `fake:"{number:1.5,2}"`,
			typ:     i,
			wantErr: "number takes integers, not 1.5",
		},
		{name: "words to a number function", tag: `fake:"{price:a,b}"`, typ: f64, wantErr: `price takes numbers, not "a"`},
		{name: "price", tag: `fake:"{price:1.00,10.00}"`, typ: f64, want: "Price(1.00, 10.00)"},
		{name: "a float range on an int", tag: `fake:"{price:1.5,2.5}"`, typ: i, wantErr: "1.5 does not fit int"},
		{name: "float32range", tag: `fake:"{float32range:0.5,2.5}"`, typ: f32, want: "Float32Range(0.5, 2.5)"},
		{
			name:    "float32range beyond float32",
			tag:     `fake:"{float32range:0,1e39}"`,
			typ:     f32,
			wantErr: "1e39 does not fit float32",
		},
		{name: "a fixed integer", tag: `fake:"42"`, typ: i, want: "fixed=42"},
		{name: "a fixed float", tag: `fake:"-1.5"`, typ: f64, want: "fixed=-1.5"},
		{
			name:    "a fixed integer too big",
			tag:     `fake:"300"`,
			typ:     u8,
			wantErr: `fake tag "300" is not a number uint8 can hold`,
		},
		{
			name:    "a fixed fraction on an integer",
			tag:     `fake:"1.5"`,
			typ:     i,
			wantErr: `fake tag "1.5" is not a number int can hold`,
		},
		{
			name: "text on a number",
			tag:  `fake:"{email}"`,
			typ:  i,
			wantErr: "fake tag \"{email}\" cannot fill int\n" +
				"  a number takes {number}, {intrange}, {uintrange}, {float32range}, {float64range}, {price}, " +
				"or a number as it stands",
		},
		{name: "digits on a number", tag: `fake:"###"`, typ: i, wantErr: `fake tag "###" cannot fill int`},
		{name: "a number function amid text", tag: `fake:"{number:1,5}0"`, typ: i, wantErr: `cannot fill int`},

		{name: "a fixed bool", tag: `fake:"true"`, typ: b, want: "fixed=true"},
		{
			name:    "a template on a bool",
			tag:     `fake:"{bool}"`,
			typ:     b,
			wantErr: "cannot fill bool\n  a bool takes true or false",
		},

		{name: "date", tag: `fake:"{date}"`, typ: tm, want: "Date()"},
		{
			name:    "date with a format",
			tag:     `fake:"{date:2006-01-02}"`,
			typ:     tm,
			wantErr: "cannot fill time.Time\n  a time.Time takes {date}",
		},
		{name: "uuid", tag: `fake:"{uuid}"`, typ: gUUID, want: "UUID()"},
		{name: "the standard library's uuid", tag: `fake:"{uuid}"`, typ: stdID, want: "UUID()"},
		{
			name:    "a fixed uuid",
			tag:     `fake:"00000000-0000-0000-0000-000000000000"`,
			typ:     gUUID,
			wantErr: "a UUID takes {uuid}",
		},

		{name: "a struct", tag: `fake:"{street}"`, typ: nopkg, wantErr: `fake tag "{street}" cannot fill model.Address`},
		{name: "a slice", tag: `fake:"{word}"`, typ: types.NewSlice(str), wantErr: `fake tag "{word}" cannot fill []string`},
		{name: "a complex number", tag: `fake:"1"`, typ: types.Typ[types.Complex128], wantErr: "cannot fill complex128"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := gen.Fake(tt.tag, tt.typ)
			if got != tt.want {
				t.Errorf("Fake(%s, %s) = %q, want %q", tt.tag, tt.typ, got, tt.want)
			}
			switch {
			case tt.wantErr == "" && err != "":
				t.Errorf("Fake(%s, %s) error = %q, want none", tt.tag, tt.typ, err)
			case !strings.Contains(err, tt.wantErr):
				t.Errorf("Fake(%s, %s) error = %q, want one containing %q", tt.tag, tt.typ, err, tt.wantErr)
			}
		})
	}
}
