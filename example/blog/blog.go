// Package blog holds the structs the example makes fixtures of. It knows
// nothing of kanna-fixture but the go:generate line and the occasional tag.
package blog

import (
	"time"

	"github.com/google/uuid"
)

//go:generate go tool kanna-fixture
//go:generate go tool kanna-validate

// Status is a named string with constants beside it, the first of which that
// is not the zero value a fixture takes.
type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
)

// Author shows what a field gets from its type and its name.
type Author struct {
	ID    int64
	Name  string `fake:"{firstname} {lastname}"` // a template, for a value that reads like one
	Email string // numbered, so that no two authors share one

	// A pointer stays nil: it says the author has written no bio, which a
	// fixture inventing one would hide.
	Bio *string

	CreatedAt time.Time
}

// Article calls the fixture of Author for its author, and carries a UUID.
type Article struct {
	ID          uuid.UUID
	Slug        string
	Title       string `fake:"{sentence}"`
	Status      Status
	Author      Author
	PublishedAt time.Time
	DeletedAt   time.Time `fake:"skip"` // set, it would say the article is gone
}

// SignUp is a request kanna-validate checks. Its fixture passes as made.
//
//kanna:validate
type SignUp struct {
	Email    string   `json:"email"    validate:"required,email"`
	Password string   `json:"password" validate:"required,min=12"`
	Confirm  string   `json:"confirm"  validate:"eqfield=Password"`
	Role     string   `json:"role"     validate:"required,oneof=admin member guest"`
	Age      int      `json:"age"      validate:"gte=18,lte=130"`
	Zip      string   `json:"zip"      validate:"pattern=^\\d{3}-\\d{4}$"`
	Tags     []string `json:"tags"     validate:"min=1,dive,max=20"`
}

// Draft gets no fixture.
//
//kanna:fixture skip
type Draft struct {
	Body string
}
