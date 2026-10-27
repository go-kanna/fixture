// Package model is what the fixtures in testdata/emit/fixture are generated
// from: a field of each kind the generator fills.
package model

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
	StatusArchived
)

// User shows the values a field gets by its type and name.
type User struct {
	ID        int64
	Name      string `fake:"{firstname} {lastname}"`
	Email     string `validate:"required,email"`
	Age       int    `validate:"gte=18,lte=130"`
	Role      Role
	Status    Status
	Nickname  *string
	Admin     bool
	CreatedAt time.Time
	DeletedAt time.Time `fake:"skip"`
	secret    string
}

// Article calls the fixture of User for its fields of that type.
type Article struct {
	Slug   string
	Title  string `validate:"max=40"`
	Author User
	Editor *User    `validate:"required"`
	Tags   []string `validate:"min=2,dive,max=10"`
	Rating float64
	Code   string `fake:"???-####"`
}

// SignUp is valid as its fixture makes it.
type SignUp struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required,min=12"`
	Confirm  string `json:"confirm"  validate:"eqfield=Password"`
	Role     string `json:"role"     validate:"required,oneof=admin member guest"`
	Age      int    `json:"age"      validate:"gte=18,lte=130"`
	Zip      string `json:"zip"      validate:"pattern=^\\d{3}-\\d{4}$"`
	Agree    bool   `json:"agree"    validate:"required"`
}

// Faked has only fields a fake tag fills, whose faker is seeded with the
// number all the same.
type Faked struct {
	Level int8      `fake:"{number:1,5}"`
	Born  time.Time `fake:"{date}"`
}

// Empty has nothing a fixture can set.
type Empty struct {
	hidden int
}

// Internal gets no fixture.
//
//kanna:ignore
type Internal struct {
	Token string
}
