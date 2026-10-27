// Command example builds values from the fixtures kanna-fixture generated for
// package blog, which are plain Go to call, and checks with kanna-validate that
// a request made by one is valid as it stands.
//
// Regenerate with:
//
//	go generate ./...
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/go-kanna/fixture/example/blog"
	"github.com/go-kanna/fixture/example/blog/fixture"
)

func main() {
	// Every field arrives filled. What identifies an article is numbered, so
	// two of them never collide.
	for range 2 {
		a := fixture.Article()
		fmt.Fprintf(os.Stdout, "%s %s %q by %s <%s>\n", a.ID, a.Slug, a.Title, a.Author.Name, a.Author.Email)
	}

	// A test states only what it cares about. What it leaves alone is either
	// numbered or, like the status, the same however many fixtures came first.
	published := fixture.Article(func(a *blog.Article) { a.Status = blog.StatusPublished })
	draft := fixture.Article()
	fmt.Fprintf(os.Stdout, "statuses: %s, %s\n", published.Status, draft.Status)

	// What is absent stays absent: no bio, not deleted.
	fmt.Fprintf(os.Stdout, "bio is nil: %t, deleted: %t\n", draft.Author.Bio == nil, !draft.DeletedAt.IsZero())

	// A fixture keeps to the rules of its validate tags, so the request is
	// valid as made, and breaking one rule breaks only that one.
	if err := fixture.SignUp().Validate(); err != nil {
		panic(fmt.Sprintf("the fixture of SignUp is not valid: %v", err))
	}
	err := fixture.SignUp(func(s *blog.SignUp) { s.Email = "not an address" }).Validate()
	out, jerr := json.Marshal(err)
	if jerr != nil {
		panic(jerr)
	}
	fmt.Fprintf(os.Stdout, "with a bad email: %s\n", out)
}
