// Package gen generates a fixture for every exported struct of a package: a
// function returning the struct with every field it can fill set. cmd/kanna-fixture
// runs it.
//
// A field is filled from a number counted per type, within the rules its
// validate tag declares, or from the gofakeit template of its fake tag. The
// fixtures are written into a package of their own, since they import the one
// they are made for.
package gen
