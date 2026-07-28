# Cairn

A miniature service catalog and maturity scorecard — Go, Postgres, Docker, CI.

Cairn answers one question for an engineering org: **how healthy is each of our
services, and who owns it?** Register a service, point it at a repository, and
Cairn scores it against a weighted set of maturity checks.

**Status:** In development. Phases 0–1 complete (skeleton, scorecard engine).

## Quickstart

```bash
make run
curl localhost:8081/healthz
```

## The scorecard model

Each check is a pure function over a set of `Facts` gathered about a repository.
Checks are weighted; the overall score is the sum of passed weights divided by
the total, expressed as 0–100.

| Check key         | Verifies                            | Weight |
|-------------------|-------------------------------------|-------:|
| `has_readme`      | Repository has a README             |     10 |
| `has_ci`          | CI pipeline configured              |     20 |
| `has_tests`       | Test files detected                 |     20 |
| `has_dockerfile`  | Dockerfile present                  |     10 |
| `has_license`     | LICENSE present                     |     10 |
| `has_owner`       | Catalog entry names an owning team  |     15 |
| `recent_activity` | Committed to within 90 days         |     15 |

Scores map to a level:

| Level  | Score  |
|--------|--------|
| Bronze | 0–59   |
| Silver | 60–84  |
| Gold   | 85–100 |

The engine (`internal/scorecard`) performs no I/O — it takes `Facts` in and
returns a `Report`. That keeps it fully unit-testable and independent of where
the facts came from.

## Development

| Command      | Does                                  |
|--------------|---------------------------------------|
| `make run`   | Run the server locally on port 8081   |
| `make test`  | Run all tests with the race detector  |
| `make cover` | Run tests and report total coverage   |
| `make lint`  | Static analysis (`go vet`)            |
| `make fmt`   | Format with `gofmt`                   |
| `make build` | Build binary to `bin/cairn`           |
| `make tidy`  | Sync `go.mod` / `go.sum`              |
| `make clean` | Remove build artifacts                |
