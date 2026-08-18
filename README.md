# Cairn

A miniature service catalog and maturity scorecard — Go, Postgres, Docker, CI.

Cairn answers one question for an engineering org: **how healthy is each of our
services, and who owns it?** Register a service, point it at a repository, and
Cairn scores it against a weighted set of maturity checks.

**Status:** In development. Phases 0–4 complete (skeleton, scorecard engine,
REST API, Postgres, GitHub ingestion).

**Requires:** Go 1.25+, Docker, and the Compose plugin (`docker compose`).

## Quickstart

```bash
make db-up      # start Postgres in Docker
make run        # migrations run automatically at startup
curl localhost:8081/healthz
```

Register a service and score it:

```bash
BASE=http://localhost:8081/api/v1

ID=$(curl -s -X POST $BASE/services \
  -H 'Content-Type: application/json' \
  -d '{"name":"go","owner_team":"golang","repo_url":"https://github.com/golang/go","tier":1}' \
  | jq -r .id)

curl -s -X POST $BASE/services/$ID/evaluate | jq
```

## Configuration

| Variable       | Default    | Purpose                                                |
|----------------|------------|--------------------------------------------------------|
| `PORT`         | `8080`     | Listen port (`make run` overrides it to `8081`)        |
| `DATABASE_URL` | *required* | Postgres connection string; the service exits without it |
| `GITHUB_TOKEN` | *optional* | Raises the GitHub API rate limit from 60 to 5,000 requests/hour, and is required to read private repositories |

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

## Where the facts come from

`POST /api/v1/services/{id}/evaluate` reads the repository through the GitHub
REST API in two requests: repository metadata, then the recursive file tree.
Every file-based fact is derived from that single file list rather than one
request per check.

Known limitations, stated plainly:

- **`has_tests` is a heuristic** over file paths. It recognises common
  conventions (`*_test.go`, `test_*.py`, `*.spec.ts`, `spec/`, `tests/`) but
  will miss unusual layouts. Determining this exactly would require the code
  search API, which is heavily rate-limited.
- **Private repositories return `404`** from the GitHub API when
  unauthenticated — GitHub deliberately does not distinguish "forbidden" from
  "not found" — so evaluating one requires `GITHUB_TOKEN`.
- **Only `github.com` is supported.** Other hosts are rejected with `422`.
  Adding GitLab means one new implementation of the `FactFetcher` interface and
  no change to the API or domain layers.

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
Use `go test ./... -short` to skip the integration tests. The GitHub adapter is
tested against an `httptest` server serving canned API responses, so no test
makes a live network call.

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

Data is persisted in Postgres; repository facts come from the GitHub REST API.
