# Development

## Requirements

Go 1.26 or later; the go command is the task runner. The library and its tests use only the standard library, so
nothing is downloaded.

## Test

| Command | Runs |
|---|---|
| `go test ./...` | The tests against a local siteverify server, including every case of `__fixtures__/siteverify-cases.json`. Needs no network. CI runs it on Go 1.26 and 1.27 for every push to `main` and every pull request. |
| `go test -tags live -run Live ./...` | The test against `https://api.captchafox.com/siteverify` with the public test keys. Needs the network. |

## Checks

| Command | Runs |
|---|---|
| `go vet ./...` | Static analysis. CI runs it with the tests. |
| `gofmt -l .` | Lists the files whose formatting differs from `gofmt`. |
