package gen_test

import (
	"go/types"
	"slices"
	"strings"
	"testing"

	"github.com/go-kanna/internal/packages"
	"github.com/go-kanna/internal/pkgtest"

	"github.com/go-kanna/fixture/internal/gen"
)

// fill loads src as one package and works out its fixtures, as gen.Fill spells
// them.
func fill(t *testing.T, src string) (map[string][]string, []string) {
	t.Helper()

	pkg := pkgtest.Load(t, map[string]string{"test.go": src})
	return gen.Fill(pkg.Fset, []*packages.Package{pkg})
}

func checkFill(t *testing.T, got map[string][]string, name string, want []string) {
	t.Helper()

	if !slices.Equal(got[name], want) {
		t.Errorf("%s =\n\t%s\nwant\n\t%s", name, strings.Join(got[name], "\n\t"), strings.Join(want, "\n\t"))
	}
}

func TestFill_Types(t *testing.T) {
	t.Parallel()

	got, diags := fill(t, `package model

import "time"

type Role string

const (
	RoleNone   Role = ""
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

type Status int

const (
	StatusUnspecified Status = iota
	StatusActive
)

type UserID int64

type Code string

type Flag bool

const (
	FlagOff Flag = false
	FlagOn  Flag = true
)

type Ratio float64

const Half Ratio = 0.5

type User struct {
	ID           int64
	UserID       UserID
	Name         string
	FirstName    string
	ContactEmail string
	HomeURL      string
	TraceUUID    string
	Age          int8
	Byte         uint8
	Small        int16
	Medium       int32
	Port         uint16
	Wide         uint32
	Count        int
	Score        float64
	Ratio        float32
	Admin        bool
	Role         Role
	Status       Status
	Code         Code
	Flag         Flag
	Half         Ratio
	Err          error
	Nickname     *string
	Tags         []string
	Meta         map[string]string
	Grid         [2]int
	Any          any
	CreatedAt    time.Time
	Month        time.Month
	Timeout      time.Duration
	secret       string
	_            int
}
`)

	checkFill(t, got, "User", []string{
		"ID: n",
		"UserID: model.UserID(n)",
		`Name: fmt.Sprintf("name-%d", n)`,
		`FirstName: fmt.Sprintf("first-name-%d", n)`,
		`ContactEmail: fmt.Sprintf("contact-email-%d@example.com", n)`,
		`HomeURL: fmt.Sprintf("https://example.com/home-url-%d", n)`,
		`TraceUUID: fmt.Sprintf("00000000-0000-4000-8000-%012x", n)`,
		"Age: int8(1 + (n-1)%127)",
		"Byte: uint8(1 + (n-1)%255)",
		"Small: int16(1 + (n-1)%32767)",
		"Medium: int32(1 + (n-1)%2147483647)",
		"Port: uint16(1 + (n-1)%65535)",
		"Wide: uint32(1 + (n-1)%4294967295)",
		"Count: int(n)",
		"Score: float64(n)",
		"Ratio: float32(n)",
		"Role: model.RoleAdmin",
		"Status: model.StatusActive",
		`Code: model.Code(fmt.Sprintf("code-%d", n))`,
		"Flag: model.FlagOn",
		"Half: model.Half",
		"CreatedAt: time.Date(2000, time.January, 1, 0, 0, int(n), 0, time.UTC)",
		"Month: time.January",
		"Timeout: time.Nanosecond",
		"uses n",
	})
	if len(diags) > 0 {
		t.Errorf("diagnostics = %q, want none", diags)
	}
}

func TestFill_Rules(t *testing.T) {
	t.Parallel()

	got, diags := fill(t, `package model

type Status int

const StatusActive Status = 1

type Role string

const RoleAdmin Role = "admin"

type Item struct {
	SKU string
}

type SignUp struct {
	Email    string            `+"`"+`validate:"required,email"`+"`"+`
	Password string            `+"`"+`validate:"required,min=12"`+"`"+`
	Confirm  string            `+"`"+`validate:"eqfield=Password"`+"`"+`
	Role     string            `+"`"+`validate:"required,oneof=admin member guest"`+"`"+`
	Kind     Role              `+"`"+`validate:"oneof=member"`+"`"+`
	Age      int               `+"`"+`validate:"gte=18,lte=130"`+"`"+`
	Count    int               `+"`"+`validate:"gt=0"`+"`"+`
	Adult    int               `+"`"+`validate:"gte=18"`+"`"+`
	Offset   int               `+"`"+`validate:"gte=-5"`+"`"+`
	Exact    int               `+"`"+`validate:"eq=7"`+"`"+`
	Same     int               `+"`"+`validate:"gte=3,lte=3"`+"`"+`
	Huge     uint64            `+"`"+`validate:"lte=18446744073709551615"`+"`"+`
	Level    Status            `+"`"+`validate:"gte=2,lte=4"`+"`"+`
	Price    float64           `+"`"+`validate:"gt=0"`+"`"+`
	Short    string            `+"`"+`validate:"max=5"`+"`"+`
	Slug     string            `+"`"+`validate:"min=3,max=8"`+"`"+`
	Empty    string            `+"`"+`validate:"max=0"`+"`"+`
	Zip      string            `+"`"+`validate:"pattern=^\\d{3}-\\d{4}$"`+"`"+`
	Code     string            `+"`"+`validate:"pattern=^a{1000}$"`+"`"+`
	Broken   string            `+"`"+`validate:"pattern=^(x$"`+"`"+`
	Agree    bool              `+"`"+`validate:"required"`+"`"+`
	Opt      *bool             `+"`"+`validate:"required"`+"`"+`
	Nick     *string           `+"`"+`validate:"required,max=20"`+"`"+`
	State    *Status           `+"`"+`validate:"required"`+"`"+`
	Ratio    *float64          `+"`"+`validate:"required,gt=0"`+"`"+`
	Other    *struct{ A int }  `+"`"+`validate:"required"`+"`"+`
	Tags     []string          `+"`"+`validate:"min=2,dive,max=5"`+"`"+`
	Items    []Item            `+"`"+`validate:"required"`+"`"+`
	Anys     []any             `+"`"+`validate:"required"`+"`"+`
	Meta     map[string]string `+"`"+`validate:"required"`+"`"+`
	Again    string            `+"`"+`validate:"eqfield=Missing"`+"`"+`
	Hits     int               `+"`"+`validate:"required,gte=0"`+"`"+`
	Cost     float64           `+"`"+`validate:"required,gte=0"`+"`"+`
}
`)

	checkFill(t, got, "SignUp", []string{
		`Email: fmt.Sprintf("email-%d@example.com", n)`,
		`Password: fmt.Sprintf("password-%03d", n)`,
		`Role: "admin"`,
		`Kind: "member"`,
		"Age: int(18 + (n-1)%113)",
		"Count: int(n)",
		"Adult: int(n + 17)",
		"Offset: int(n - 6)",
		"Exact: 7",
		"Same: 3",
		"Huge: uint64(n)",
		"Level: model.Status(2 + (n-1)%3)",
		"Price: 1",
		`Short: fmt.Sprintf("%d", n%100000)`,
		`Slug: fmt.Sprintf("slu-%d", n%10000)`,
		`Empty: ""`,
		"Zip: f.Regex(`^\\d{3}-\\d{4}$`)",
		`Code: fmt.Sprintf("code-%d", n)`,
		`Broken: fmt.Sprintf("broken-%d", n)`,
		"Agree: true",
		"Opt: new(true)",
		`Nick: new(fmt.Sprintf("nick-%d", n))`,
		"State: new(model.StatusActive)",
		"Ratio: new(float64(1))",
		"Other: new(struct{A int})",
		`Tags: []string{fmt.Sprintf("%d", n%100000), fmt.Sprintf("%d", n%100000)}`,
		"Items: []model.Item{Item()}",
		"Anys: make([]any, 1)",
		"Meta: map[string]string{}",
		`Again: fmt.Sprintf("again-%d", n)`,
		"Hits: int(n)",
		"Cost: 1",
		"Confirm = Password",
		"uses n",
		"uses f",
	})

	want := []string{
		"test.go:34:2: warning: values generated from the pattern do not match it; give the field a fake tag (field Code)",
	}
	if !slices.Equal(diags, want) {
		t.Errorf("diagnostics = %q, want %q", diags, want)
	}
}

func TestFill_Fake(t *testing.T) {
	t.Parallel()

	got, diags := fill(t, `package model

import "time"

type Code string

type Status int

type Person struct {
	Name     string    `+"`"+`fake:"{firstname} {lastname}"`+"`"+`
	Code     Code      `+"`"+`fake:"???-####"`+"`"+`
	Age      int8      `+"`"+`fake:"{number:18,65}"`+"`"+`
	Count    int       `+"`"+`fake:"{number:1,9}"`+"`"+`
	Level    Status    `+"`"+`fake:"{number:1,5}"`+"`"+`
	Born     time.Time `+"`"+`fake:"{date}"`+"`"+`
	Deleted  time.Time `+"`"+`fake:"skip"`+"`"+`
	Nick     *string   `+"`"+`fake:"{firstname}"`+"`"+`
	NickCode *Code     `+"`"+`fake:"A1"`+"`"+`
	Fixed    string    `+"`"+`fake:"123-4567"`+"`"+`
	Answer   *int      `+"`"+`fake:"42"`+"`"+`
	Label    *string   `+"`"+`fake:"x"`+"`"+`
	Rate     *float64  `+"`"+`fake:"1.5"`+"`"+`
	Ignored  string    `+"`"+`fake:"{zzzzzzzzzz}" validate:"required"`+"`"+`
	Bad      int       `+"`"+`fake:"{email}"`+"`"+`
}
`)

	checkFill(t, got, "Person", []string{
		`Name: mustGenerate(f, "{firstname} {lastname}")`,
		`Code: model.Code(mustGenerate(f, "???-####"))`,
		"Age: int8(f.Number(18, 65))",
		"Count: f.Number(1, 9)",
		"Level: model.Status(f.Number(1, 5))",
		"Born: f.Date()",
		`Nick: new(mustGenerate(f, "{firstname}"))`,
		`NickCode: new(model.Code("A1"))`,
		`Fixed: "123-4567"`,
		"Answer: new(42)",
		`Label: new("x")`,
		"Rate: new(1.5)",
		"uses f",
	})

	want := []string{
		`test.go:23:2: error: unknown function "zzzzzzzzzz" in fake tag "{zzzzzzzzzz}" (field Ignored)`,
		"test.go:24:2: error: fake tag \"{email}\" cannot fill int (field Bad)\n" +
			"  a number takes {number}, {intrange}, {uintrange}, {float32range}, {float64range}, {price}, " +
			"or a number as it stands",
	}
	if !slices.Equal(diags, want) {
		t.Errorf("diagnostics = %q, want %q", diags, want)
	}
}

func TestFill_Fixtures(t *testing.T) {
	t.Parallel()

	got, diags := fill(t, `package model

type User struct {
	Name    string
	Home    *Address `+"`"+`validate:"required"`+"`"+`
	Manager *User    `+"`"+`validate:"required"`+"`"+`
	Card    *Secret  `+"`"+`validate:"required"`+"`"+`
	Friends []User   `+"`"+`validate:"min=1"`+"`"+`
}

type Address struct {
	Street string
	Owner  *User `+"`"+`validate:"required"`+"`"+`
	backup *User
	Shadow *User `+"`"+`fake:"-" validate:"required"`+"`"+`
}

//kanna:ignore
type Secret struct{}

type Article struct {
	Author   User
	Editor   *User `+"`"+`validate:"required"`+"`"+`
	Reviewer *User
	Readers  []User `+"`"+`validate:"min=2"`+"`"+`
}

type Tree struct {
	Children []Tree `+"`"+`validate:"min=1"`+"`"+`
}

type Base struct {
	ID int64
}

type Post struct {
	Base
	Title string `+"`"+`fake:"-"`+"`"+`
}
`)

	// No fixture calls one that would call it back: User's calling Address's,
	// which calls User's, or User's own Manager. What the rules require is there
	// all the same, holding the zero value.
	checkFill(t, got, "User", []string{
		`Name: fmt.Sprintf("name-%d", n)`,
		"Home: new(model.Address)",
		"Manager: new(model.User)",
		"Card: new(model.Secret)",
		"Friends: make([]model.User, 1)",
		"uses n",
	})
	checkFill(t, got, "Address", []string{
		`Street: fmt.Sprintf("street-%d", n)`,
		"Owner: new(model.User)",
		"uses n",
	})
	checkFill(t, got, "Tree", []string{"Children: make([]model.Tree, 1)"})
	checkFill(t, got, "Article", []string{
		"Author: User()",
		"Editor: new(User())",
		"Readers: []model.User{User(), User()}",
	})
	checkFill(t, got, "Post", []string{"Base: Base()"})
	if len(diags) > 0 {
		t.Errorf("diagnostics = %q, want none", diags)
	}
}

func TestFillField_UUID(t *testing.T) {
	t.Parallel()

	uuidOf := func(path string) types.Type {
		pkg := types.NewPackage(path, "uuid")
		return types.NewNamed(types.NewTypeName(0, pkg, "UUID", nil), types.NewArray(types.Typ[types.Byte], 16), nil)
	}

	const numbered = `uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012x", n))`

	tests := []struct {
		name string
		typ  types.Type
		tag  string
		want string
	}{
		{name: "google", typ: uuidOf("github.com/google/uuid"), want: numbered},
		{name: "the standard library's", typ: uuidOf("uuid"), want: numbered},
		{name: "faked", typ: uuidOf("github.com/google/uuid"), tag: `fake:"{uuid}"`, want: "uuid.MustParse(f.UUID())"},
		{name: "another array", typ: types.NewArray(types.Typ[types.Byte], 16), want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := gen.FillField("ID", tt.typ, tt.tag); got != tt.want {
				t.Errorf("FillField(ID, %s, %s) = %q, want %q", tt.typ, tt.tag, got, tt.want)
			}
		})
	}
}

func TestKebab(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]string{
		"Name":       "name",
		"FirstName":  "first-name",
		"UserID":     "user-id",
		"ID":         "id",
		"HTTPServer": "http-server",
		"Address2":   "address2",
		"V2Name":     "v2-name",
	} {
		if got := gen.FillField(name, types.Typ[types.String], ""); got != `fmt.Sprintf("`+want+`-%d", n)` {
			t.Errorf("%s fills as %s, want it named %s", name, got, want)
		}
	}
}
