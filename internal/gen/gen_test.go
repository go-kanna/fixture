package gen_test

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Before any test runs: t.Setenv would rule out t.Parallel. The go command
	// that loads packages must not resolve them through a workspace, which a
	// Makefile or mise may point GOWORK at.
	if err := os.Setenv("GOWORK", "off"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
