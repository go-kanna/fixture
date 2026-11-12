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
		"Status: model.Status(n)",
		`Code: model.Code(fmt.Sprintf("code-%d", n))`,
		"Half: model.Ratio(n)",
		"CreatedAt: time.Date(2000, time.January, 1, 0, 0, int(n), 0, time.UTC)",
		"Month: time.Month(n)",
		"Timeout: time.Duration(n)",
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
	Pi       float64           `+"`"+`validate:"eq=3.14159265"`+"`"+`
	Small    float64           `+"`"+`validate:"lte=0.1234567"`+"`"+`
	Tight    float64           `+"`"+`validate:"gt=1,lt=1.00001"`+"`"+`
	Big      uint64            `+"`"+`validate:"gte=9223372036854775808"`+"`"+`
	Ärger    string            `+"`"+`validate:"max=6"`+"`"+`
	Word     string            `+"`"+`validate:"min=3,pattern=^[a-z]+$"`+"`"+`
	Mail     string            `+"`"+`validate:"email,len=19"`+"`"+`
	Site     string            `+"`"+`validate:"url,min=30"`+"`"+`
	Contact  string            `+"`"+`validate:"email,max=10"`+"`"+`
	Ref      string            `+"`"+`validate:"uuid,max=20"`+"`"+`
	HomeURL  string            `+"`"+`validate:"max=10"`+"`"+`
	Bigger   uint64            `+"`"+`validate:"gte=9223372036854775808,lte=9223372036854775810"`+"`"+`
	Biggest  uint64            `+"`"+`validate:"gte=18446744073709551615"`+"`"+`
	Floor    int               `+"`"+`validate:"gte=1,lte=20,ne=13,ne=4"`+"`"+`
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
		"State: new(model.Status(n))",
		"Ratio: new(float64(1))",
		"Other: new(struct{A int})",
		`Tags: []string{fmt.Sprintf("%d", n%100000), fmt.Sprintf("%d", n%100000)}`,
		"Items: []model.Item{Item()}",
		"Anys: make([]any, 1)",
		"Meta: map[string]string{}",
		`Again: fmt.Sprintf("again-%d", n)`,
		"Hits: int(n)",
		"Cost: 1",
		"Pi: 3.14159265",
		"Small: 0.1234567",
		"Tight: 1.000005",
		"Big: 9223372036854775808 + (uint64(n)-1)%9223372036854775808",
		`Ärger: fmt.Sprintf("ä-%d", n%10000)`,
		`Word: fmt.Sprintf("word-%d", n)`,
		`Mail: fmt.Sprintf("ma-%04d@example.com", n%10000)`,
		`Site: fmt.Sprintf("https://example.com/site-%05d", n)`,
		`Contact: fmt.Sprintf("conta-%d", n%10000)`,
		`Ref: fmt.Sprintf("ref-%d", n)`,
		`HomeURL: fmt.Sprintf("home-%d", n%100000)`,
		"Bigger: 9223372036854775808 + (uint64(n)-1)%3",
		"Biggest: 18446744073709551615",
		"Floor: int(skipping(1 + (n-1)%18, 4, 13))",
		"Confirm = Password",
		"uses n",
		"uses f",
	})

	want := []string{
		"test.go:34:2: warning: values generated from the pattern do not match it; give the field a fake tag (field Code)",
		"test.go:54:2: warning: values generated from the pattern do not fit within the length the rules set; " +
			"give the field a fake tag (field Word)",
		"test.go:57:2: warning: a numbered email address does not fit within the length the rules set; " +
			"give the field a fake tag (field Contact)",
		"test.go:58:2: warning: a UUID does not fit within the length the rules set; give the field a fake tag (field Ref)",
	}
	if !slices.Equal(diags, want) {
		t.Errorf("diagnostics = %q, want %q", diags, want)
	}
}

// A value the rules fix is written as the constant of its type that holds it,
// the first exported one declared, and as a literal where none does.
func TestFill_ConstantsOfFixedValues(t *testing.T) {
	t.Parallel()

	got, diags := fill(t, `package model

type Status string

const (
	StatusOpen   Status = "open"
	StatusClosed Status = "closed"
	statusGone   Status = "gone"
)

type Level int

const (
	LevelLow Level = iota + 1
	LevelHigh
	LevelDefault = LevelHigh
)

type Ratio float64

const Half Ratio = 0.5

type Ticket struct {
	State   Status `+"`"+`validate:"oneof=closed open"`+"`"+`
	Gone    Status `+"`"+`validate:"oneof=gone"`+"`"+`
	Unknown Status `+"`"+`validate:"eq=draft"`+"`"+`
	Level   Level  `+"`"+`validate:"oneof=2"`+"`"+`
	Same    Level  `+"`"+`validate:"gte=1,lte=1"`+"`"+`
	Range   Level  `+"`"+`validate:"gte=1,lte=2"`+"`"+`
	Ratio   Ratio  `+"`"+`validate:"eq=0.5"`+"`"+`
	Plain   string `+"`"+`validate:"oneof=open"`+"`"+`
}
`)

	checkFill(t, got, "Ticket", []string{
		"State: model.StatusClosed",
		`Gone: "gone"`,
		`Unknown: "draft"`,
		"Level: model.LevelHigh",
		"Same: model.LevelLow",
		"Range: model.Level(1 + (n-1)%2)",
		"Ratio: model.Half",
		`Plain: "open"`,
		"uses n",
	})
	if len(diags) > 0 {
		t.Errorf("diagnostics = %q, want none", diags)
	}
}

// A float keeps within its bounds as its type holds it, compared both with the
// bound converted to that type and with the bound as written, and the fields
// whose type holds no value within them are warned about.
func TestFill_FloatPrecision(t *testing.T) {
	t.Parallel()

	got, diags := fill(t, `package model

type Ratio float32

type Reading struct {
	Big    float32 `+"`"+`validate:"gt=16777216"`+"`"+`
	Under  float32 `+"`"+`validate:"lt=16777216"`+"`"+`
	Tenth  float32 `+"`"+`validate:"lte=0.1"`+"`"+`
	Half   float32 `+"`"+`validate:"gte=0.5"`+"`"+`
	Ratio  Ratio   `+"`"+`validate:"gt=16777216"`+"`"+`
	Wide   float64 `+"`"+`validate:"gt=9007199254740992"`+"`"+`
	Narrow float32 `+"`"+`validate:"gt=16777216,lt=16777217"`+"`"+`
}
`)

	checkFill(t, got, "Reading", []string{
		"Big: 1.6777218e+07",
		"Under: 1.6777215e+07",
		"Tenth: 0.099999994",
		"Half: 0.5",
		"Ratio: 1.6777218e+07",
		"Wide: 9.007199254740994e+15",
		"Narrow: float32(n)",
		"uses n",
	})
	want := []string{
		"test.go:12:2: warning: no float32 keeps within the bounds the rules set; give the field a fake tag (field Narrow)",
	}
	if !slices.Equal(diags, want) {
		t.Errorf("diagnostics = %q, want %q", diags, want)
	}
}

// The rules of a time.Duration are read as durations, as kanna-validator reads
// them, and rules no duration passes, or whose value cannot be read, are warned
// about, as is a number rule that cannot be read on another type.
func TestFill_Durations(t *testing.T) {
	t.Parallel()

	got, diags := fill(t, `package model

import "time"

type Job struct {
	Timeout time.Duration   `+"`"+`validate:"gte=1s,lt=1h"`+"`"+`
	Backoff *time.Duration  `+"`"+`validate:"required,oneof=0 1500ms"`+"`"+`
	Second  time.Duration   `+"`"+`validate:"eq=1s"`+"`"+`
	Steps   []time.Duration `+"`"+`validate:"min=1,dive,gt=1m"`+"`"+`
	Never   time.Duration   `+"`"+`validate:"gte=1h,lte=1m"`+"`"+`
	Typo    time.Duration   `+"`"+`validate:"gte=1sec"`+"`"+`
	Count   int             `+"`"+`validate:"lte=ten"`+"`"+`
}
`)

	checkFill(t, got, "Job", []string{
		"Timeout: time.Duration(1000000000 + (n-1)%3599000000000)",
		"Backoff: new(time.Duration(1500000000))",
		"Second: time.Second",
		"Steps: []time.Duration{time.Duration(n + 60000000000)}",
		"Never: time.Duration(n)",
		"Typo: time.Duration(n)",
		"Count: int(n)",
		"uses n",
	})
	want := []string{
		"test.go:10:2: warning: no time.Duration passes the rules; give the field a fake tag (field Never)",
		"test.go:11:2: warning: kanna-fixture cannot read gte=1sec as a duration, so the value it fills may break it; " +
			"give the field a fake tag (field Typo)",
		"test.go:12:2: warning: kanna-fixture cannot read lte=ten as a number, so the value it fills may break it; " +
			"give the field a fake tag (field Count)",
	}
	if !slices.Equal(diags, want) {
		t.Errorf("diagnostics =\n\t%s\nwant\n\t%s", strings.Join(diags, "\n\t"), strings.Join(want, "\n\t"))
	}
}

// Rules no integer of its type passes, a bound beyond the type or bounds the
// wrong way round, are warned about, and the field is numbered as before.
func TestFill_UnmetIntegers(t *testing.T) {
	t.Parallel()

	got, diags := fill(t, `package model

type Level uint8

type Gauge struct {
	Small int8  `+"`"+`validate:"gte=200"`+"`"+`
	Wide  int   `+"`"+`validate:"gte=10,lte=5"`+"`"+`
	Pct   int   `+"`"+`validate:"gt=0,lt=1"`+"`"+`
	Floor Level `+"`"+`validate:"lte=0,ne=0"`+"`"+`
	Low   int8  `+"`"+`validate:"gte=-200,lte=5"`+"`"+`
}
`)

	checkFill(t, got, "Gauge", []string{
		"Small: int8(1 + (n-1)%127)",
		"Wide: int(n)",
		"Pct: int(n)",
		"Floor: model.Level(1 + (n-1)%255)",
		"Low: int8(1 + (n-1)%5)",
		"uses n",
	})
	want := []string{
		"test.go:6:2: warning: no int8 passes the rules; give the field a fake tag (field Small)",
		"test.go:7:2: warning: no int passes the rules; give the field a fake tag (field Wide)",
		"test.go:8:2: warning: no int passes the rules; give the field a fake tag (field Pct)",
		"test.go:9:2: warning: no model.Level passes the rules; give the field a fake tag (field Floor)",
	}
	if !slices.Equal(diags, want) {
		t.Errorf("diagnostics =\n\t%s\nwant\n\t%s", strings.Join(diags, "\n\t"), strings.Join(want, "\n\t"))
	}
}

// A copy of a field that copies another comes after that copy, whatever the
// order of the fields, so that the chain ends at the same value. A cycle has
// nothing to copy, and leaves its fields zero.
func TestFill_CopiesInOrder(t *testing.T) {
	t.Parallel()

	got, diags := fill(t, `package model

type Change struct {
	Third  string `+"`"+`validate:"eqfield=Second"`+"`"+`
	Second string `+"`"+`validate:"eqfield=First"`+"`"+`
	First  string
	A      string `+"`"+`validate:"eqfield=B"`+"`"+`
	B      string `+"`"+`validate:"eqfield=A"`+"`"+`
}
`)

	checkFill(t, got, "Change", []string{
		`First: fmt.Sprintf("first-%d", n)`,
		"Second = First",
		"Third = Second",
		"B = A",
		"A = B",
		"uses n",
	})
	if len(diags) > 0 {
		t.Errorf("diagnostics = %q, want none", diags)
	}
}

// A rule kanna-fixture does not know, and a length a map it fills empty is to
// have, are warned about, unless a fake tag says what the field holds. Of
// alternatives, the one it knows is filled.
func TestFill_WarnsOfRulesNotKept(t *testing.T) {
	t.Parallel()

	got, diags := fill(t, `package model

type Item struct {
	Slug   string            `+"`"+`validate:"required,alphanum"`+"`"+`
	Token  string            `+"`"+`validate:"uuid4|uuid"`+"`"+`
	When   string            `+"`"+`validate:"required_if=Slug x"`+"`"+`
	Labels map[string]string `+"`"+`validate:"min=1"`+"`"+`
	Codes  []string          `+"`"+`validate:"min=1,dive,numeric"`+"`"+`
	Faked  string            `+"`"+`fake:"abc" validate:"alphanum"`+"`"+`
	Plain  string            `+"`"+`validate:"omitempty,omitnil,omitzero,max=10,"`+"`"+`
	Typo   string            `+"`"+`validate:"requird"`+"`"+`
}
`)

	checkFill(t, got, "Item", []string{
		`Slug: fmt.Sprintf("slug-%d", n)`,
		`Token: fmt.Sprintf("00000000-0000-4000-8000-%012x", n)`,
		`When: fmt.Sprintf("when-%d", n)`,
		`Codes: []string{fmt.Sprintf("codes-%d", n)}`,
		`Faked: "abc"`,
		`Plain: fmt.Sprintf("plain-%d", n%10000)`,
		`Typo: fmt.Sprintf("typo-%d", n)`,
		"uses n",
	})
	want := []string{
		"test.go:4:2: warning: kanna-fixture does not know alphanum, so the value it fills may break it; " +
			"give the field a fake tag (field Slug)",
		"test.go:6:2: warning: kanna-fixture does not know required_if=Slug x, so the value it fills may break it; " +
			"give the field a fake tag (field When)",
		"test.go:7:2: warning: an empty map does not fit within the length the rules set; " +
			"give the field a fake tag (field Labels)",
		"test.go:8:2: warning: kanna-fixture does not know numeric, so the value it fills may break it; " +
			"give the field a fake tag (field Codes)",
		"test.go:11:2: warning: kanna-fixture does not know requird, so the value it fills may break it (field Typo)\n" +
			"  did you mean \"required\"?\n" +
			"  give the field a fake tag if it is a rule of the validator",
	}
	if !slices.Equal(diags, want) {
		t.Errorf("diagnostics =\n\t%s\nwant\n\t%s", strings.Join(diags, "\n\t"), strings.Join(want, "\n\t"))
	}
}

// rule= of kanna-validator holds a field to the rule of a type, named as the
// struct's file imports it: an enum gives its first constant, and a value type
// what a field of it holds, read through its accessor, so that its constructor
// rejects a guess its rule breaks. A type whose rule the fixture cannot tell is
// warned about.
func TestFill_Rule(t *testing.T) {
	t.Parallel()

	got, diags := fill(t, `package model

import (
	"time"

	_ "net/url"
)

type Status string

const (
	StatusNone   Status = ""
	StatusActive Status = "active"
)

func NewStatus(s string) (Status, error) { return Status(s), nil }

type Deadline struct{ t time.Time }

func (d Deadline) Time() time.Time { return d.t }

func NewDeadline(t time.Time) (Deadline, error) { return Deadline{t: t}, nil }

type Slug struct{ s string }

func (s Slug) String() string { return s.s }

func NewSlug(s string) (Slug, error) { return Slug{s: s}, nil }

type Past struct{}

func (Past) Validate(time.Time) error { return nil }

type Task struct {
	State  string     `+"`"+`validate:"rule=Status"`+"`"+`
	States []string   `+"`"+`validate:"min=1,dive,rule=Status"`+"`"+`
	Due    time.Time  `+"`"+`validate:"rule=Deadline"`+"`"+`
	Done   *time.Time `+"`"+`validate:"required,rule=Deadline"`+"`"+`
	Slug   string     `+"`"+`validate:"required,max=20,rule=Slug"`+"`"+`
	When   time.Time  `+"`"+`validate:"rule=Past"`+"`"+`
	Link   string     `+"`"+`validate:"rule=url.URL"`+"`"+`
	Other  string     `+"`"+`validate:"rule=model.Status"`+"`"+`
}
`)

	checkFill(t, got, "Task", []string{
		`State: string(model.StatusActive)`,
		`States: []string{string(model.StatusActive)}`,
		"Due: orPanic(model.NewDeadline(time.Date(2000, time.January, 1, 0, 0, int(n), 0, time.UTC))).Time()",
		"Done: new(orPanic(model.NewDeadline(time.Date(2000, time.January, 1, 0, 0, int(n), 0, time.UTC))).Time())",
		`Slug: orPanic(model.NewSlug(fmt.Sprintf("slug-%d", n))).String()`,
		"When: time.Date(2000, time.January, 1, 0, 0, int(n), 0, time.UTC)",
		`Link: fmt.Sprintf("link-%d", n)`,
		`Other: fmt.Sprintf("other-%d", n)`,
		"uses n",
	})
	want := []string{
		"test.go:40:2: warning: kanna-fixture cannot tell what passes rule=Past, which is neither an enum nor " +
			"a value type; give the field a fake tag (field When)",
		"test.go:41:2: warning: kanna-fixture cannot tell what passes rule=url.URL, which is neither an enum nor " +
			"a value type; give the field a fake tag (field Link)",
		"test.go:42:2: warning: kanna-fixture cannot read rule=model.Status: the file imports no package named model; " +
			"give the field a fake tag (field Other)",
	}
	if !slices.Equal(diags, want) {
		t.Errorf("diagnostics =\n\t%s\nwant\n\t%s", strings.Join(diags, "\n\t"), strings.Join(want, "\n\t"))
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
	Again    string    `+"`"+`fake:"{lastname}" validate:"eqfield=Name"`+"`"+`
	Twice    string    `+"`"+`fake:"{lastname" validate:"eqfield=Name"`+"`"+`
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
		`Again: mustGenerate(f, "{lastname}")`,
		"uses f",
	})

	want := []string{
		`test.go:23:2: error: unknown function "zzzzzzzzzz" in fake tag "{zzzzzzzzzz}" (field Ignored)`,
		"test.go:24:2: error: fake tag \"{email}\" cannot fill int (field Bad)\n" +
			"  a number takes {number}, {intrange}, {uintrange}, {float32range}, {float64range}, {price}, " +
			"or a number as it stands",
		`test.go:26:2: error: fake tag "{lastname": no } closes the { of {lastname (field Twice)`,
	}
	if !slices.Equal(diags, want) {
		t.Errorf("diagnostics = %q, want %q", diags, want)
	}
}

// A fake tag on a type no template fills says how to fill the field instead: by
// the fixture of a struct that gets one, or by one written by hand, with the
// faker for a value type, and without for another struct.
func TestFill_FakeOnAType(t *testing.T) {
	t.Parallel()

	_, diags := fill(t, `package model

//kanna:value
type Email struct{ s string }

func (e Email) String() string { return e.s }

func NewEmail(s string) (Email, error) { return Email{s: s}, nil }

type Inner struct{ A int }

//kanna:fixture skip
type Skipped struct{ A int }

type Person struct {
	Mail  Email   `+"`"+`fake:"{email}"`+"`"+`
	PMail *Email  `+"`"+`fake:"{email}"`+"`"+`
	In    Inner   `+"`"+`fake:"{number}"`+"`"+`
	Skip  Skipped `+"`"+`fake:"{number}"`+"`"+`
}
`)

	want := []string{
		"test.go:16:2: error: fake tag \"{email}\" cannot fill model.Email (field Mail)\n" +
			"  write the fixture of Email by hand, func Email(f *gofakeit.Faker) model.Email in package fixture, " +
			"and drop the fake tag",
		"test.go:17:2: error: fake tag \"{email}\" cannot fill model.Email (field PMail)\n" +
			"  write the fixture of Email by hand, func Email(f *gofakeit.Faker) model.Email in package fixture, " +
			"and drop the fake tag",
		"test.go:18:2: error: fake tag \"{number}\" cannot fill model.Inner (field In)\n" +
			"  drop the fake tag: the fixture of Inner fills the field",
		"test.go:19:2: error: fake tag \"{number}\" cannot fill model.Skipped (field Skip)\n" +
			"  write the fixture of Skipped by hand, func Skipped() model.Skipped in package fixture, and drop the fake tag",
	}
	if !slices.Equal(diags, want) {
		t.Errorf("diagnostics =\n\t%s\nwant\n\t%s", strings.Join(diags, "\n\t"), strings.Join(want, "\n\t"))
	}
}

// A fake tag that makes a value its rules reject is warned about, as far as
// what it makes is known before it runs: a range of numbers, or a value as it
// stands. fake:"skip" is the way to leave a field zero, rules or not.
func TestFill_FakeBreakingRules(t *testing.T) {
	t.Parallel()

	_, diags := fill(t, `package model

type Person struct {
	Age    int     `+"`"+`fake:"{number:1,10}" validate:"gte=18"`+"`"+`
	Rank   int     `+"`"+`fake:"{number:1,10}" validate:"gte=5"`+"`"+`
	Grade  int     `+"`"+`fake:"{number:1,10}" validate:"gt=0,lte=10"`+"`"+`
	Code   int     `+"`"+`fake:"42" validate:"lte=10"`+"`"+`
	Slug   string  `+"`"+`fake:"abcdef" validate:"max=5"`+"`"+`
	Kind   string  `+"`"+`fake:"c" validate:"oneof=a b"`+"`"+`
	Level  int     `+"`"+`fake:"{number:1,3}" validate:"oneof=5 6"`+"`"+`
	Pick   int     `+"`"+`fake:"{number:1,3}" validate:"oneof=2 6"`+"`"+`
	Odd    int     `+"`"+`fake:"{number:0,3}" validate:"required"`+"`"+`
	Ok     int     `+"`"+`fake:"{number:0,3}" validate:"omitempty,gte=1"`+"`"+`
	Few    int     `+"`"+`fake:"{number:0,3}" validate:"omitempty,gte=2"`+"`"+`
	Except int     `+"`"+`fake:"{number:1,3}" validate:"ne=2"`+"`"+`
	Rate   float64 `+"`"+`fake:"{float64range:0.5,1.5}" validate:"lt=1"`+"`"+`
	Name   string  `+"`"+`fake:"{firstname}" validate:"max=3"`+"`"+`
	Gone   string  `+"`"+`fake:"skip" validate:"required"`+"`"+`
	Zip    string  `+"`"+`fake:"zz" validate:"pattern=^a"`+"`"+`
	Match  string  `+"`"+`fake:"ab" validate:"pattern=^a"`+"`"+`
}
`)

	want := []string{
		`test.go:4:2: warning: fake tag "{number:1,10}" makes no value that validate:"gte=18" lets through (field Age)`,
		`test.go:5:2: warning: fake tag "{number:1,10}" makes some values that validate:"gte=5" rejects (field Rank)`,
		`test.go:7:2: warning: fake tag "42" is a value that validate:"lte=10" rejects (field Code)`,
		`test.go:8:2: warning: fake tag "abcdef" is a value that validate:"max=5" rejects (field Slug)`,
		`test.go:9:2: warning: fake tag "c" is a value that validate:"oneof=a b" rejects (field Kind)`,
		`test.go:10:2: warning: fake tag "{number:1,3}" makes no value that validate:"oneof=5 6" lets through (field Level)`,
		`test.go:11:2: warning: fake tag "{number:1,3}" makes some values that validate:"oneof=2 6" rejects (field Pick)`,
		`test.go:12:2: warning: fake tag "{number:0,3}" makes some values that validate:"required" rejects (field Odd)`,
		`test.go:14:2: warning: fake tag "{number:0,3}" makes some values that validate:"omitempty,gte=2" ` +
			`rejects (field Few)`,
		`test.go:15:2: warning: fake tag "{number:1,3}" makes some values that validate:"ne=2" rejects (field Except)`,
		`test.go:16:2: warning: fake tag "{float64range:0.5,1.5}" makes some values that validate:"lt=1" ` +
			`rejects (field Rate)`,
		`test.go:19:2: warning: fake tag "zz" is a value that validate:"pattern=^a" rejects (field Zip)`,
	}
	if !slices.Equal(diags, want) {
		t.Errorf("diagnostics =\n\t%s\nwant\n\t%s", strings.Join(diags, "\n\t"), strings.Join(want, "\n\t"))
	}
}

func TestFill_Fixtures(t *testing.T) {
	t.Parallel()

	got, diags := fill(t, `package model

import "time"

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

//kanna:fixture skip
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

//kanna:value
type Email struct{ s string }

func (e Email) String() string { return e.s }

func NewEmail(s string) (Email, error) { return Email{s: s}, nil }

//kanna:value
type Since struct{ t time.Time }

func (s Since) Time() time.Time { return s.t }

func NewSince(t time.Time) (Since, error) { return Since{t: t}, nil }

// Lookalike has the shape of a value type but no constructor, so it is a struct
// like any other, with a fixture of its own.
type Lookalike struct{ s string }

func (l Lookalike) String() string { return l.s }

// NoAccessor, Unnamed, and WrongCtor fall short of a value type each, so each
// is a struct like any other too.
type NoAccessor struct{ s string }

type Unnamed struct{ s []string }

type WrongCtor struct{ s string }

func (w WrongCtor) String() string { return w.s }

func NewWrongCtor(s string) WrongCtor { return WrongCtor{s: s} }

// Bag is a value type holding a map, which an optional field leaves nil, and
// so a Bag is left zero.
type Bag struct{ m Labels }

type Labels map[string]string

func (b Bag) Labels() Labels { return b.m }

func NewBag(m Labels) (Bag, error) { return Bag{m: m}, nil }

type Contact struct {
	Email      Email
	Backup     *Email
	Primary    *Email `+"`"+`validate:"required"`+"`"+`
	Since      Since
	Lookalike  Lookalike
	NoAccessor NoAccessor
	Unnamed    Unnamed
	WrongCtor  WrongCtor
	Bag        Bag
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
	// A value type of kanna-value has no fixture of its own: its constructor is
	// called with what a field named after the type would hold, and an optional
	// pointer to one stays nil.
	checkFill(t, got, "Contact", []string{
		`Email: orPanic(model.NewEmail(fmt.Sprintf("email-%d@example.com", n)))`,
		`Primary: new(orPanic(model.NewEmail(fmt.Sprintf("email-%d@example.com", n))))`,
		"Since: orPanic(model.NewSince(time.Date(2000, time.January, 1, 0, 0, int(n), 0, time.UTC)))",
		"Lookalike: Lookalike()",
		"NoAccessor: NoAccessor()",
		"Unnamed: Unnamed()",
		"WrongCtor: WrongCtor()",
		"uses n",
	})
	// Each value left zero for the cycle is warned about where it goes.
	want := []string{
		"test.go:15:2: warning: the fixture of User is not called here, as it would call that of Address back, " +
			"so a zero User breaks its rules; set the field, or give it a fake tag (field Owner)",
		"test.go:31:2: warning: the fixture of Tree is not called here, as it would call that of Tree back, " +
			"so a zero Tree breaks its rules; set the field, or give it a fake tag (field Children)",
		"test.go:7:2: warning: the fixture of Address is not called here, as it would call that of User back, " +
			"so a zero Address breaks its rules; set the field, or give it a fake tag (field Home)",
		"test.go:8:2: warning: the fixture of User is not called here, as it would call that of User back, " +
			"so a zero User breaks its rules; set the field, or give it a fake tag (field Manager)",
		"test.go:10:2: warning: the fixture of User is not called here, as it would call that of User back, " +
			"so a zero User breaks its rules; set the field, or give it a fake tag (field Friends)",
	}
	if !slices.Equal(diags, want) {
		t.Errorf("diagnostics =\n\t%s\nwant\n\t%s", strings.Join(diags, "\n\t"), strings.Join(want, "\n\t"))
	}
}

// A slice that has to be there and may hold nothing is empty, and not nil. What
// would otherwise fill it is not called, so it closes no cycle of fixtures.
func TestFill_SlicesWithNoRoom(t *testing.T) {
	t.Parallel()

	got, diags := fill(t, `package model

type IDs []int

type Node struct {
	Name string
}

type Team struct {
	Lead *Lead `+"`"+`validate:"required"`+"`"+`
}

type Lead struct {
	Teams []Team `+"`"+`validate:"required,max=0"`+"`"+`
}

type Slices struct {
	Max      []int     `+"`"+`validate:"required,max=0"`+"`"+`
	Len      []string  `+"`"+`validate:"required,len=0"`+"`"+`
	Lt       []int     `+"`"+`validate:"required,lt=1"`+"`"+`
	Named    IDs       `+"`"+`validate:"required,max=0"`+"`"+`
	Pointer  *[]int    `+"`"+`validate:"required,max=0"`+"`"+`
	Dive     []Node    `+"`"+`validate:"required,max=0,dive,required"`+"`"+`
	Optional []int     `+"`"+`validate:"max=0"`+"`"+`
	Room     []int     `+"`"+`validate:"required,max=1"`+"`"+`
	Bounded  []int     `+"`"+`validate:"required,min=2,max=3"`+"`"+`
}
`)

	checkFill(t, got, "Slices", []string{
		"Max: []int{}",
		"Len: []string{}",
		"Lt: []int{}",
		"Named: model.IDs{}",
		"Pointer: new([]int{})",
		"Dive: []model.Node{}",
		"Room: []int{int(n)}",
		"Bounded: []int{int(n), int(n)}",
		"uses n",
	})
	checkFill(t, got, "Team", []string{"Lead: new(Lead())"})
	checkFill(t, got, "Lead", []string{"Teams: []model.Team{}"})
	if len(diags) > 0 {
		t.Errorf("diagnostics = %q, want none", diags)
	}
}

// A type the fixtures cannot name from their own package leaves its field
// zero, as an unexported field is, whatever the tags ask, with a warning where
// the rules reject that zero value, unless the field is tagged fake:"skip".
func TestFill_Unnameable(t *testing.T) {
	t.Parallel()

	got, diags := fill(t, `package model

type level int

type lvl = level

type home struct {
	Street string
}

type hider interface {
	hide()
}

//kanna:fixture skip
type Box[T any] struct {
	Value T
}

type Thing struct {
	Name   string
	Level  level
	Alias  lvl
	Faked  level                 `+"`"+`fake:"42"`+"`"+`
	Home   *home                 `+"`"+`validate:"required"`+"`"+`
	Homes  []home                `+"`"+`validate:"min=1"`+"`"+`
	Places map[string]home       `+"`"+`validate:"required"`+"`"+`
	Hooks  []func(chan level)    `+"`"+`validate:"min=1"`+"`"+`
	Hidden []interface{ hide() } `+"`"+`validate:"min=1"`+"`"+`
	Hiders []interface{ hider }  `+"`"+`validate:"min=1"`+"`"+`
	Boxed  []Box[level]          `+"`"+`validate:"min=1"`+"`"+`
	Spot   *struct{ x int }      `+"`"+`validate:"required"`+"`"+`
	Place  *struct{ X int }      `+"`"+`validate:"required"`+"`"+`
	Boxes  []Box[int]            `+"`"+`validate:"min=1"`+"`"+`
	Kept   *home                 `+"`"+`validate:"required" fake:"skip"`+"`"+`
	Free   level                 `+"`"+`validate:"gte=0"`+"`"+`
	name   string                `+"`"+`validate:"required"`+"`"+`
	nick   string
	tag    string                `+"`"+`validate:"omitempty,min=2"`+"`"+`
	code   string                `+"`"+`validate:"required" fake:"skip"`+"`"+`
}
`)

	checkFill(t, got, "Thing", []string{
		`Name: fmt.Sprintf("name-%d", n)`,
		"Place: new(struct{X int})",
		"Boxes: make([]model.Box[int], 1)",
		"uses n",
	})
	const hints = "\n  write the fixture of Thing by hand, func Thing() model.Thing in package fixture, in place of " +
		"the generated one\n  tag the field fake:\"skip\" to leave it zero without a warning, and set it where it matters"
	warn := func(line, what, field string) string {
		return "test.go:" + line + ":2: warning: kanna-fixture cannot " + what +
			", and the zero value it leaves breaks the rules (field " + field + ")" + hints
	}
	unnamed := func(line, typ, field string) string {
		return warn(line, "name "+typ+" in package fixture", field)
	}
	want := []string{
		unnamed("25", "*model.home", "Home"),
		unnamed("26", "[]model.home", "Homes"),
		unnamed("27", "map[string]model.home", "Places"),
		unnamed("28", "[]func(chan model.level)", "Hooks"),
		unnamed("29", "[]interface{hide()}", "Hidden"),
		unnamed("30", "[]interface{model.hider}", "Hiders"),
		unnamed("31", "[]model.Box[model.level]", "Boxed"),
		unnamed("32", "*struct{x int}", "Spot"),
		warn("37", "set unexported field name", "name"),
	}
	if !slices.Equal(diags, want) {
		t.Errorf("diagnostics =\n\t%s\nwant\n\t%s", strings.Join(diags, "\n\t"), strings.Join(want, "\n\t"))
	}
}

// The fixtures cannot call the constructor of an unexported value type, so a
// field held to its rule by rule= gets a value made in its place, with a
// warning that the rules of the type may reject it.
func TestFill_UnexportedRule(t *testing.T) {
	t.Parallel()

	got, diags := fill(t, `package model

type email struct{ s string }

func (e email) String() string { return e.s }

func newEmail(s string) (email, error) { return email{s: s}, nil }

type User struct {
	Email   string   `+"`"+`validate:"rule=email"`+"`"+`
	Emails  []string `+"`"+`validate:"min=1,dive,rule=email"`+"`"+`
	Backup  *string  `+"`"+`validate:"required,rule=email"`+"`"+`
	Skipped string   `+"`"+`validate:"rule=email" fake:"skip"`+"`"+`
	Faked   string   `+"`"+`validate:"rule=email" fake:"{email}"`+"`"+`
}
`)

	checkFill(t, got, "User", []string{
		`Email: fmt.Sprintf("email-%d", n)`,
		`Emails: []string{fmt.Sprintf("email-%d", n)}`,
		`Backup: new(fmt.Sprintf("email-%d", n))`,
		`Faked: mustGenerate(f, "{email}")`,
		"uses n",
		"uses f",
	})
	warn := func(line, field string) string {
		return "test.go:" + line + ":2: warning: kanna-fixture cannot call the constructor of unexported " +
			"model.email, and the value it makes in its place may break the rules (field " + field + ")" +
			"\n  write the fixture of User by hand, func User() model.User in package fixture, in place of the " +
			"generated one\n  tag the field fake:\"skip\" to leave it zero without a warning, and set it where it matters"
	}
	want := []string{warn("10", "Email"), warn("11", "Emails"), warn("12", "Backup")}
	if !slices.Equal(diags, want) {
		t.Errorf("diagnostics =\n\t%s\nwant\n\t%s", strings.Join(diags, "\n\t"), strings.Join(want, "\n\t"))
	}
}

func TestFillField_UUID(t *testing.T) {
	t.Parallel()

	uuidOf := func(path string) types.Type {
		pkg := types.NewPackage(path, "uuid")
		return types.NewNamed(types.NewTypeName(0, pkg, "UUID", nil), types.NewArray(types.Typ[types.Byte], 16), nil)
	}

	const numbered = `uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012x", n))`
	orderID := func(u types.Type) types.Type {
		pkg := types.NewPackage("example.com/model", "model")
		return types.NewNamed(types.NewTypeName(0, pkg, "OrderID", nil), u, nil)
	}
	const shaped = "model.OrderID{0, 0, 0, 0, 0, 0, 0x40, 0, 0x80, 0, byte(n >> 40), byte(n >> 32), " +
		"byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}"

	tests := []struct {
		name string
		typ  types.Type
		tag  string
		want string
	}{
		{name: "google", typ: uuidOf("github.com/google/uuid"), want: numbered},
		{name: "the standard library's", typ: uuidOf("uuid"), want: numbered},
		{name: "faked", typ: uuidOf("github.com/google/uuid"), tag: `fake:"{uuid}"`, want: "uuid.MustParse(f.UUID())"},
		{name: "a named [16]byte", typ: orderID(types.NewArray(types.Typ[types.Byte], 16)), want: shaped},
		{name: "a named UUID", typ: orderID(uuidOf("github.com/google/uuid").Underlying()), want: shaped},
		{
			name: "a named [16]byte faked",
			typ:  orderID(types.NewArray(types.Typ[types.Byte], 16)),
			tag:  `fake:"{uuid}"`,
			want: "model.OrderID(uuid.MustParse(f.UUID()))",
		},
		{name: "a named [8]byte", typ: orderID(types.NewArray(types.Typ[types.Byte], 8)), want: ""},
		{name: "a named [16]int8", typ: orderID(types.NewArray(types.Typ[types.Int8], 16)), want: ""},
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

// A value the fixture cannot fill is left zero, and where its rules reject
// that, the field is warned about, with how to fill it instead.
func TestFill_ZeroBreakingRules(t *testing.T) {
	t.Parallel()

	_, diags := fill(t, `package model

//kanna:fixture skip
type Inner struct {
	Zip string `+"`"+`validate:"required,len=7"`+"`"+`
}

//kanna:fixture skip
type Loose struct {
	Note string
	Opt  string `+"`"+`validate:"omitempty,len=7"`+"`"+`
	Tags []string
}

//kanna:fixture skip
type Box[T any] struct{ V T }

type Outer struct {
	In     Inner      `+"`"+`validate:"required"`+"`"+`
	PIn    *Inner     `+"`"+`validate:"required"`+"`"+`
	Ins    []Inner    `+"`"+`validate:"required"`+"`"+`
	Plain  Inner
	Arr    [3]string  `+"`"+`validate:"dive,required"`+"`"+`
	Any    any        `+"`"+`validate:"required"`+"`"+`
	Fn     func()     `+"`"+`validate:"required"`+"`"+`
	Gen    Box[int]   `+"`"+`validate:"required"`+"`"+`
	Skip   Inner      `+"`"+`fake:"skip"`+"`"+`
	Opt    *Inner
	OptIn  Inner      `+"`"+`validate:"omitempty"`+"`"+`
	Free   Loose
	PFree  *Loose     `+"`"+`validate:"required"`+"`"+`
	Ar     [2]int
	Ch     chan int
}
`)

	hand := func(name, typ string) string {
		return "\n  write the fixture of " + name + " by hand, func " + name + "() " + typ +
			" in package fixture, which the fixtures then call"
	}
	const skip = "\n  tag the field fake:\"skip\" to leave it zero without a warning, and set it where it matters"
	warn := func(line, typ, field string) string {
		return "test.go:" + line + ":2: warning: kanna-fixture cannot fill " + typ +
			", and the zero value it leaves breaks the rules (field " + field + ")"
	}
	want := []string{
		warn("19", "model.Inner", "In") + hand("Inner", "model.Inner") + skip,
		warn("20", "model.Inner", "PIn") + hand("Inner", "model.Inner") + skip,
		warn("21", "model.Inner", "Ins") + hand("Inner", "model.Inner") + skip,
		warn("22", "model.Inner", "Plain") + hand("Inner", "model.Inner") + skip,
		warn("23", "[3]string", "Arr") + skip,
		warn("24", "any", "Any") + skip,
		warn("25", "func()", "Fn") + skip,
		warn("26", "model.Box[int]", "Gen") + hand("Box", "model.Box[int]") + skip,
	}
	if !slices.Equal(diags, want) {
		t.Errorf("diagnostics =\n\t%s\nwant\n\t%s", strings.Join(diags, "\n\t"), strings.Join(want, "\n\t"))
	}
}
