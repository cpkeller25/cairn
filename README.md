# Cairn

A miniature service catalog and maturity scorecard — Go, Postgres, Docker, CI.

**Status:** In development. Phase 0 (skeleton) complete.

## Quickstart

```bash
make run
curl localhost:8081/healthz
```

## Development

| Command      | Does                        |
|--------------|-----------------------------|
| `make run`   | Run the server locally      |
| `make test`  | Run all tests               |
| `make lint`  | Static analysis (`go vet`)  |
| `make fmt`   | Format with `gofmt`         |
| `make build` | Build binary to `bin/cairn` |
