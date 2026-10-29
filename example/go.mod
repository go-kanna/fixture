module github.com/go-kanna/fixture/example

go 1.27.0

toolchain go1.27.3

tool (
	github.com/go-kanna/fixture/cmd/kanna-fixture
	github.com/go-kanna/validate/cmd/kanna-validate
)

require (
	github.com/brianvoe/gofakeit/v7 v7.18.0
	github.com/go-kanna/validate v0.0.6
	github.com/go-kanna/value v0.0.3
	github.com/google/uuid v1.6.0
)

require (
	github.com/go-kanna/fixture v0.0.6 // indirect
	github.com/go-kanna/internal v0.0.9 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
)
