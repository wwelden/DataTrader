# DataTrader

DataTrader is a web app designed to help users manage their trades and analyze their trading habbits.
It supports stocks and options.

A design goal of this project was to not sure any javascript, and have the site feel fast and responsive.

Stack:
- Go
- HTMX
- Templ
- SQLite


Features:
- [x] Stock position tracking
- [x] Option position tracking
- [x] User registration and login
- [x] CSV trade import
- [x] CSV trade processing
- [x] Update/Delete/Close positions
- [x] Public landing page with a one-click demo account (no sign-up)

## Running tests

```sh
go test -race ./...
```

Tests use a temporary SQLite database, so they need cgo (a C compiler). The same suite, plus `gofmt`, `go vet`, and a check that the generated templ files are current, runs on every push and pull request via GitHub Actions (`.github/workflows/test.yml`).
