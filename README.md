# Cairn

[![CI](https://github.com/cpkeller25/cairn/actions/workflows/ci.yml/badge.svg)](https://github.com/cpkeller25/cairn/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

**A miniature service catalog and maturity scorecard — Go, Postgres, Docker, CI.**

Cairn answers one question for an engineering org: **how healthy is each of our
services, and who owns it?**

Register a service, point it at a GitHub repository, and Cairn gathers facts
about that repo — README, CI pipeline, tests, Dockerfile, license, recent
activity — then runs them through a weighted scorecard that produces a score
from 0–100 and a level: **Bronze**, **Silver**, or **Gold**.

Here it is scoring its own repository:

```console
$ curl -s localhost:8081/api/v1/services/$ID | jq
{
  "id": "5056424c-cd12-4cb9-8fce-f3c0e3158ab5",
  "name": "cairn",
  "owner_team": "platform",
  "repo_url": "https://github.com/cpkeller25/cairn",
  "tier": 1,
  "scorecard": {
    "overall_score": 100,
    "level": "gold",
    "checks": [
      { "key": "has_readme",      "passed": true, "weight": 10, "detail": "README found" },
      { "key": "has_ci",          "passed": true, "weight": 20, "detail": "CI configuration found" },
      { "key": "has_tests",       "passed": true, "weight": 20, "detail": "test files detected" },
      { "key": "has_dockerfile",  "passed": true, "weight": 10, "detail": "Dockerfile found" },
      { "key": "has_license",     "passed": true, "weight": 10, "detail": "LICENSE found" },
      { "key": "has_owner",       "passed": true, "weight": 15, "detail": "owned by platform" },
      { "key": "recent_activity", "passed": true, "weight": 15, "detail": "last commit 0 days ago" }
    ]
  }
}
```

---

## Quickstart

**Requires:** Go 1.25+, Docker, and the Compose plugin.

```bash
git clone https://github.com/cpkeller25/cairn.git && cd cairn
make db-up      # start Postgres in Docker
make seed       # register and score five well-known repositories
make run        # migrations apply automatically at startup
```

```bash
curl -s localhost:8081/api/v1/services | jq -r \
  '.services[] | "\(.name)\t\(.scorecard.overall_score)\t\(.scorecard.level)"'
```

Or run the whole stack in containers:

```bash
make up && make logs
```

---

## Architecture

```
                 ┌────────────────────────────────────────────────┐
   HTTP client   │                    Cairn                       │
  (curl / UI) ──▶│  ┌──────────┐   ┌───────────┐   ┌──────────┐   │──▶ GitHub
                 │  │   api    │──▶│ scorecard │◀──│  ingest  │   │    REST API
                 │  │ net/http │   │  (rules   │   │ (fetch   │   │
                 │  │ handlers │   │  engine)  │   │  facts)  │   │
                 │  └────┬─────┘   └───────────┘   └──────────┘   │
                 │       │               ▲                        │
                 │       │         ┌─────┴─────┐                  │
                 │       │         │  catalog  │                  │
                 │       │         │  (domain) │                  │
                 │       │         └───────────┘                  │
                 │  ┌────▼─────┐                                  │
                 │  │  store   │──────────────────────────────────┼──▶ Postgres
                 │  │  (pgx)   │                                  │
                 │  └──────────┘                                  │
                 │  slog logs · /healthz · /readyz · /metrics      │
                 └────────────────────────────────────────────────┘
```

A light **hexagonal / ports-and-adapters** design. The domain — `catalog` and
`scorecard` — knows nothing about HTTP, Postgres, or GitHub. Those live behind
two interfaces declared by their *consumer*:

```go
// internal/api — the API declares what it needs, not what exists.
type Store interface       { CreateService(...); GetService(...); /* … */ }
type FactFetcher interface { Fetch(ctx, repoURL) (scorecard.Facts, error) }
```

| Package | Responsibility | Depends on |
|---|---|---|
| `internal/catalog` | `Service` entity, validation, domain errors | stdlib |
| `internal/scorecard` | Weighted rules engine. **No I/O.** | stdlib |
| `internal/ingest` | GitHub adapter behind `FactFetcher` | stdlib |
| `internal/store` | Postgres and in-memory adapters behind `Store` | pgx |
| `internal/api` | Handlers, routing, JSON, middleware | the above |
| `internal/config` | Environment configuration | stdlib |

`cmd/cairn/main.go` is the composition root — the only file that names a
concrete adapter. Swapping the in-memory store for Postgres, and the fact stub
for GitHub, were each a **one-line change** there, with no handler or domain
code touched and every handler test still passing.

`cmd/seed` reuses the same domain packages with no HTTP server at all, which is
the practical proof the layering is real rather than decorative.

---

## The scorecard model

Each check is a pure function `func(Facts) (bool, string)`. Checks are
weighted; the score is the sum of passed weights over the total, as 0–100.

| Check key | Verifies | Weight |
|---|---|---:|
| `has_readme` | Repository has a README | 10 |
| `has_ci` | CI pipeline configured | 20 |
| `has_tests` | Test files detected | 20 |
| `has_dockerfile` | Dockerfile present | 10 |
| `has_license` | LICENSE present | 10 |
| `has_owner` | Catalog entry names an owning team | 15 |
| `recent_activity` | Committed to within 90 days | 15 |

| Level | Score |
|---|---|
| Bronze | 0–59 |
| Silver | 60–84 |
| Gold | 85–100 |

Because the engine performs no I/O, it is exhaustively unit-tested with no
mocks and no infrastructure — **100% statement coverage**, including the
59/60 and 84/85 level boundaries.

---

## API

Full contract: [`openapi.yaml`](openapi.yaml).

| Method & path | Purpose |
|---|---|
| `POST /api/v1/services` | Register a service |
| `GET /api/v1/services` | List the catalog with latest scores |
| `GET /api/v1/services/{id}` | Service detail + scorecard breakdown |
| `POST /api/v1/services/{id}/evaluate` | Gather facts, score, persist |
| `DELETE /api/v1/services/{id}` | Remove a service |
| `GET /api/v1/checks` | Check definitions and weights |
| `GET /healthz` · `/readyz` | Liveness · readiness |
| `GET /metrics` | Prometheus metrics |

### Register and evaluate

```bash
BASE=http://localhost:8081/api/v1

ID=$(curl -s -X POST $BASE/services \
  -H 'Content-Type: application/json' \
  -d '{
        "name": "payments-api",
        "description": "Handles payment processing",
        "owner_team": "checkout",
        "repo_url": "github.com/acme/payments-api",
        "tier": 1
      }' | jq -r .id)

curl -s -X POST $BASE/services/$ID/evaluate | jq
```

### Validation errors report every problem at once

```console
$ curl -s -X POST $BASE/services -H 'Content-Type: application/json' \
    -d '{"name":"Bad Name","tier":9}' | jq
{
  "error": "validation failed",
  "fields": [
    { "field": "name",       "message": "must be lowercase alphanumeric, optionally hyphen-separated (e.g. payments-api)" },
    { "field": "owner_team", "message": "is required" },
    { "field": "repo_url",   "message": "is required" },
    { "field": "tier",       "message": "must be between 1 and 3" }
  ]
}
```

### Status codes

| Code | When |
|---|---|
| `400` | Malformed JSON, unknown field, or a non-UUID id |
| `404` | No such service |
| `409` | Service name already taken |
| `422` | Valid JSON, invalid contents — or a repository that cannot be read |
| `502` | GitHub unreachable or rate limited |

---

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | Listen port (`make run` overrides to `8081`) |
| `DATABASE_URL` | *required* | Postgres connection string; the service refuses to start without it |
| `GITHUB_TOKEN` | *optional* | Raises the GitHub rate limit from 60 to 5,000 requests/hour; required for private repositories |
| `LOG_LEVEL` | `info` | `debug` · `info` · `warn` · `error` |
| `LOG_FORMAT` | `json` | `json` for production, `text` for local reading |

---

## Operations

**Structured logging** via `log/slog`. Every request gets an ID — generated, or
taken from an inbound `X-Request-ID` so a trace survives across services — which
is echoed in the response header and attached to every log line for that
request.

```json
{"time":"2026-08-19T13:22:25Z","level":"INFO","msg":"request","request_id":"9f3c…","method":"POST","path":"/api/v1/services","status":201,"duration_ms":12}
```

**Liveness vs readiness are genuinely different.** `/healthz` touches no
dependencies, so a database blip cannot trigger a restart loop across every
instance. `/readyz` pings Postgres, so an unhealthy instance is pulled from the
load balancer while it recovers.

**Metrics** are labelled by *route pattern*, not raw path — `/api/v1/services/{id}`
rather than a distinct series per UUID. Unbounded label cardinality is the
classic way to take down a Prometheus server.

**Graceful shutdown**: `SIGTERM` stops accepting connections and drains
in-flight requests for up to 15 seconds before exiting.

---

## Development

| Command | Does |
|---|---|
| `make run` | Run the server locally on port 8081 |
| `make test` | All tests with the race detector |
| `make cover` | Tests plus a coverage summary |
| `make lint` | `go vet` + `golangci-lint` |
| `make fmt` | Format with `gofmt` |
| `make build` | Build binary to `bin/cairn` |
| `make seed` | Register and score five demo repositories |
| `make tidy` | Sync `go.mod` / `go.sum` |
| `make clean` | Remove build artifacts |
| `make db-up` | Start Postgres in Docker |
| `make db-down` | Stop Postgres |
| `make db-reset` | **Destroy** the database and start fresh (deletes all data) |
| `make psql` | Open a psql shell against the dev database |
| `make docker-build` | Build the container image as `cairn:dev` |
| `make up` / `make down` | Start / stop the full Docker stack |
| `make logs` | Follow the app container's logs |

`make lint` needs [golangci-lint](https://golangci-lint.run/welcome/install/):

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```

### Testing

| Package | Coverage | Approach |
|---|---:|---|
| `scorecard` | 100.0% | Pure functions, exhaustive table-driven tests, zero mocks |
| `store` | 85.7% | One contract suite run against **both** the in-memory and Postgres stores, on a real Postgres via testcontainers |
| `api` | 82.8% | `httptest` — real requests, no server process |
| `ingest` | 81.8% | `httptest` server returning canned GitHub payloads; no live calls |
| `catalog` | 77.8% | Table-driven validation tests |

`make test` starts throwaway Postgres containers, so Docker must be running.
`go test ./... -short` skips them for a fast inner loop.

The store **contract test** is written once against the `Store` interface and
run against every implementation, which is what makes swapping them safe:
in-memory deletes a scorecard with two map deletes, Postgres does it with
`ON DELETE CASCADE`, and one assertion covers both.

---

## Design decisions & tradeoffs

**Hexagonal architecture, with only two interfaces.** `Store` and
`FactFetcher`, each declared in the package that *consumes* them and each with a
second implementation that earns its keep in tests. Go's advice is not to create
an interface until you have a second implementation; three more would have been
ceremony. The payoff was concrete: Phase 3 and Phase 4 each swapped an adapter
with a one-line change to `main.go`.

**Standard library router over a framework.** Go 1.22's `ServeMux` handles
method-aware patterns (`POST /api/v1/services/{id}/evaluate`) and path
wildcards, which is everything this API needs. `chi` would be a reasonable
choice on a larger surface; here it would add a dependency to save nothing.

**Standard library HTTP over `google/go-github`.** The client library is ~100k
lines wrapping the whole GitHub API, of which this uses a fraction. Writing it
directly keeps the integration legible and makes the `httptest` fixtures double
as documentation of the real payloads. For a product integrating deeply with
GitHub, the library's pagination and rate-limit handling would win.

**Two GitHub calls per evaluation, not one per check.** Repository metadata,
then the recursive file tree — every file-based fact is derived from that single
listing. One request per check would be seven round trips and would exhaust an
unauthenticated rate limit in nine evaluations.

**Separate JSON types from domain types.** `catalog.Service` carries no `json`
tags. The API layer owns its own request and response structs and converts.
That costs a mapping function and buys three things: the wire contract stops
being hostage to the schema, nothing leaks by default, and clients structurally
cannot set server-owned fields like `id` or `created_at`.

**Domain owns the error vocabulary.** `catalog.ErrNotFound`,
`ErrNameTaken`, and `ErrRepoUnreadable` live in the domain, not in the adapters
that produce them. That is what lets the API map a failure to 404, 409, or 422
without importing Postgres or GitHub.

**testcontainers over a CI service container.** Tests start their own Postgres,
so the local and CI code paths are identical — no `if CI` branching, no second
connection-string mechanism. The cost is that `make test` requires Docker and
takes ~30s; `-short` exists for that reason.

**Migrations run at startup, embedded in the binary.** `go:embed` puts the SQL
inside the executable, so the image and the cloud deploy carry their own schema.
`golang-migrate` takes an advisory lock, so concurrent instances are safe. A
larger team would run migrations as a separate deploy step instead — here, a bad
migration failing startup is an acceptable tradeoff for a single-instance
service, and it keeps the quickstart to two commands.

**Injected clocks, everywhere.** `catalog.New(input, now)`,
`Facts.FetchedAt`, `Server.now`. Nothing reads `time.Now()` inside logic under
test, which is what makes assertions like "exactly 90 days is still recent"
possible at all.

### Known limitations

- **`has_tests` is a heuristic** over file paths. It recognises common
  conventions (`*_test.go`, `test_*.py`, `*.spec.ts`, `spec/`, `tests/`) and
  will miss unusual layouts. Exactness would need the code search API, which is
  heavily rate-limited.
- **Private repositories return `404`** from GitHub when unauthenticated —
  GitHub deliberately does not distinguish "forbidden" from "not found" — so
  evaluating one requires `GITHUB_TOKEN`.
- **Only `github.com` is supported.** Other hosts are rejected with `422`.
  Adding GitLab means one new `FactFetcher` implementation and no change to the
  API or domain.
- **No authentication.** Out of scope deliberately; this is a demonstration of
  service structure, not a product.
- **The app container has no healthcheck.** `distroless` has no shell, so the
  usual `curl`-based check cannot run. The real answers are a `-healthcheck`
  flag on the binary, or orchestrator-native HTTP probes.

### How it would scale

Evaluations are the expensive path — two GitHub calls, subject to rate limits.
The next steps in order would be: cache repository facts with a short TTL;
move evaluation onto a queue with a worker pool so `POST /evaluate` returns
`202 Accepted`; and add a scheduled re-evaluation sweep so scores stay fresh
without anyone asking. The `FactFetcher` interface is the seam all of that
hangs off.

---

## Container

```bash
make docker-build
docker images cairn:dev
```

A multi-stage build: a **364 MB** Go toolchain compiles the binary, and the
final image is **21.7 MB** on `distroless/static` — the binary, CA certificates
(needed for HTTPS to GitHub), and a non-root user. No shell, no package
manager.

## Dependencies

Four at runtime, two test-only:

| Module | Why |
|---|---|
| `jackc/pgx/v5` | Postgres driver and pool |
| `golang-migrate/migrate/v4` | Schema migrations |
| `google/uuid` | Identifiers |
| `prometheus/client_golang` | Metrics |
| `testcontainers-go` *(test)* | Real Postgres in integration tests |

---

## Status

Phases 0–6 complete: skeleton, scorecard engine, REST API, Postgres, GitHub
ingestion, Docker + CI, production polish. Remaining: a public cloud deploy.

MIT licensed.
