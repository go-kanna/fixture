package gen_test

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/go-kanna/fixture/internal/gen/testdata/emit/fixture"
	"github.com/go-kanna/fixture/internal/gen/testdata/emit/model"
)

// The tests call the fixtures generated into testdata/emit/fixture. Tests run
// in parallel share the numbers, so they check what holds whichever number a
// fixture gets.

var email = regexp.MustCompile(`^email-\d+@example\.com$`)

func TestGenerated_Numbered(t *testing.T) {
	t.Parallel()

	a, b := fixture.User(), fixture.User()

	if b.ID <= a.ID {
		t.Errorf("IDs = %d, %d, want them counting up", a.ID, b.ID)
	}
	if !email.MatchString(a.Email) || a.Email == b.Email {
		t.Errorf("emails = %q, %q, want two different addresses numbered", a.Email, b.Email)
	}
	if !b.CreatedAt.After(a.CreatedAt) {
		t.Errorf("CreatedAt = %v, %v, want the later fixture made later", a.CreatedAt, b.CreatedAt)
	}
	if a.Age < 18 || a.Age > 130 {
		t.Errorf("Age = %d, want it within 18..130", a.Age)
	}
}

func TestGenerated_Fixed(t *testing.T) {
	t.Parallel()

	u := fixture.User()

	if u.Role != model.RoleAdmin || u.Status != model.StatusActive {
		t.Errorf("Role, Status = %q, %d, want the first constants that are not zero", u.Role, u.Status)
	}
	if u.Nickname != nil || u.Admin || !u.DeletedAt.IsZero() {
		t.Errorf("Nickname, Admin, DeletedAt = %v, %t, %v, want them zero", u.Nickname, u.Admin, u.DeletedAt)
	}
	if u.Name == "" || strings.ContainsAny(u.Name, "{}") || !strings.Contains(u.Name, " ") {
		t.Errorf("Name = %q, want a first and a last name", u.Name)
	}
}

func TestGenerated_References(t *testing.T) {
	t.Parallel()

	a := fixture.Article()

	if a.Author.ID == 0 || a.Editor == nil || a.Editor.ID == a.Author.ID {
		t.Errorf("Author, Editor = %+v, %+v, want two users of their own", a.Author, a.Editor)
	}
	if len(a.Tags) != 2 {
		t.Errorf("Tags = %q, want two", a.Tags)
	}
	for _, tag := range a.Tags {
		if utf8.RuneCountInString(tag) > 10 {
			t.Errorf("tag %q is longer than 10", tag)
		}
	}
	if !regexp.MustCompile(`^[A-Za-z]{3}-\d{4}$`).MatchString(a.Code) {
		t.Errorf("Code = %q, want ???-####", a.Code)
	}
}

func TestGenerated_Valid(t *testing.T) {
	t.Parallel()

	s := fixture.SignUp()

	switch {
	case !email.MatchString(s.Email):
		t.Errorf("Email = %q", s.Email)
	case utf8.RuneCountInString(s.Password) < 12:
		t.Errorf("Password = %q, want 12 characters or more", s.Password)
	case s.Confirm != s.Password:
		t.Errorf("Confirm = %q, want the password, %q", s.Confirm, s.Password)
	case s.Role != "admin":
		t.Errorf("Role = %q, want admin", s.Role)
	case s.Age < 18 || s.Age > 130:
		t.Errorf("Age = %d", s.Age)
	case !regexp.MustCompile(`^\d{3}-\d{4}$`).MatchString(s.Zip):
		t.Errorf("Zip = %q", s.Zip)
	case !s.Agree:
		t.Error("Agree = false, want true")
	}
}

// A value type is made by its constructor, from what a field named after the
// type would hold, or by the function the fixture package writes for it.
func TestGenerated_ValueTypes(t *testing.T) {
	t.Parallel()

	c := fixture.Contact()
	if !email.MatchString(c.Email.String()) {
		t.Errorf("Email = %q, want an address its constructor accepted", c.Email)
	}
	if h := c.Handle.String(); h == "" || strings.ContainsAny(h, "0123456789") {
		t.Errorf("Handle = %q, want what the function written by hand made", h)
	}
	// The faker it is given is the fixture's, seeded with its number.
	if a, b := fixture.Contact().Handle, fixture.Contact().Handle; a == b {
		t.Errorf("two contacts have the handle %q, want the faker of each", a)
	}
}

// A value type whose constructor rejects what the fixture guessed makes the
// fixture panic, saying what to write instead.
func TestGenerated_ValueTypeRejected(t *testing.T) {
	t.Parallel()

	defer func() {
		want := `kanna-fixture: model.Since rejects the value guessed for it: since: rejected; ` +
			`write func Since() model.Since in package fixture for its fixtures, or tag the field fake:"skip"`
		if got := recover(); got != want {
			t.Errorf("panic = %v, want %q", got, want)
		}
	}()
	fixture.Tenure()
}

// A fixture written by hand is the one the generated fixtures call.
func TestGenerated_FixtureWrittenByHand(t *testing.T) {
	t.Parallel()

	if got := fixture.Shop().Owner.Name; got != "kanna" {
		t.Errorf("Shop().Owner.Name = %q, want the name Owner, written by hand, gives", got)
	}
}

func TestGenerated_Faked(t *testing.T) {
	t.Parallel()

	v := fixture.Faked()

	if v.Level < 1 || v.Level > 5 || v.Born.IsZero() {
		t.Errorf("Faked() = %+v, want a level within 1..5 and a date", v)
	}
	if e := fixture.Empty(); e != (model.Empty{}) {
		t.Errorf("Empty() = %+v, want the zero value", e)
	}
}

func TestGenerated_Setters(t *testing.T) {
	t.Parallel()

	u := fixture.User(
		func(u *model.User) { u.Name = "Alice" },
		func(u *model.User) { u.Name += "!" },
	)

	if u.Name != "Alice!" {
		t.Errorf("Name = %q, want the setters applied in order", u.Name)
	}
}
