// Command kanna-fixture generates a fixture for every exported struct of a
// package: a function returning the struct with every field it can fill set.
package main

import (
	"os"

	"github.com/go-kanna/fixture/internal/gen"
)

func main() {
	os.Exit(gen.Command().Main(os.Args[1:]))
}
