# Cairn

A miniature service catalog and maturity scorecard — Go, Postgres, Docker, CI.

Cairn answers one question for an engineering org: **how healthy is each of our
services, and who owns it?** Register a service, point it at a repository, and
Cairn scores it against a weighted set of maturity checks.

**Status:** In development. Phases 0–3 complete (skeleton, scorecard engine, REST API, Postgres).

**Requires:** Go 1.22+, Docker, and the Compose plugin (`docker compose`).

## Quickstart

```bash
make db-up      # start Postgres in Docker
make run        # migrations run automatically at startup
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

| Command         | Does                                                        |
|-----------------|-------------------------------------------------------------|
| `make run`      | Run the server locally on port 8081                         |
| `make test`     | Run all tests with the race detector                        |
| `make cover`    | Run tests and report total coverage                         |
| `make lint`     | Static analysis (`go vet`)                                  |
| `make fmt`      | Format with `gofmt`                                         |
| `make build`    | Build binary to `bin/cairn`                                 |
| `make tidy`     | Sync `go.mod` / `go.sum`                                    |
| `make clean`    | Remove build artifacts                                      |
| `make db-up`    | Start Postgres in Docker                                    |
| `make db-down`  | Stop Postgres in Docker                                     |
| `make db-reset` | **Destroy** the database and start fresh (deletes all data) |
| `make psql`     | Access Postgres tables in Docker                            |

`make test` starts throwaway Postgres containers via
[testcontainers](https://testcontainers.com), so Docker must be running.
Use `go test ./... -short` to skip the integration tests.

## API
| Method & path                         | Purpose                                  |
|---------------------------------------|------------------------------------------|
| `POST /api/v1/services`               | Register a service                       |
| `GET /api/v1/services`                | List the catalog with latest scores      |
| `GET /api/v1/services/{id}`           | Service detail + scorecard breakdown     |
| `POST /api/v1/services/{id}/evaluate` | Gather facts, run the scorecard, persist |
| `DELETE /api/v1/services/{id}`        | Remove a service                         |
| `GET /api/v1/checks`                  | Check definitions and weights            |
| `GET /healthz`                        | Liveness probe                           |

Data is persisted in Postgres. Repository facts currently come from a
deterministic stub — Phase 4 adds the GitHub adapter.