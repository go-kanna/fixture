package fixture

import (
	"strings"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/go-kanna/fixture/internal/gen/testdata/emit/model"
)

// Handle is the fixture of model.Handle, written by hand since the numbered
// value a fixture would guess has a digit, which its rule rejects. It takes the
// faker of the fixture calling it.
func Handle(f *gofakeit.Faker) model.Handle {
	h, err := model.NewHandle(strings.ToLower(f.FirstName()))
	if err != nil {
		panic(err)
	}
	return h
}

// Owner is the fixture of model.Owner, written by hand, which the generated
// fixtures call.
func Owner(setters ...func(*model.Owner)) model.Owner {
	o := model.Owner{Name: "kanna"}
	for _, set := range setters {
		set(&o)
	}
	return o
}

// Status is the fixture of model.Status, an enum of int, whose constants a
// fixture does not pick from as it does those of an enum of string.
func Status() model.Status {
	return model.StatusActive
}
