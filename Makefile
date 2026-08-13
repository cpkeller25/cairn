.PHONY: db-up db-down db-reset psql run build test lint tidy fmt clean cover

DB_URL ?= postgres://cairn:cairn@localhost:5432/cairn?sslmode=disable

db-up:
	docker compose -f deploy/docker-compose.yml up -d
	@echo "waiting for postgres..."
	@until docker exec cairn-postgres pg_isready -U cairn -d cairn >/dev/null 2>&1; do sleep 1; done
	@echo "postgres ready"

db-down:
	docker compose -f deploy/docker-compose.yml down

db-reset:
	docker compose -f deploy/docker-compose.yml down -v
	$(MAKE) db-up

psql:
	docker exec -it cairn-postgres psql -U cairn -d cairn

run:
	DATABASE_URL="$(DB_URL)" PORT=8081 go run ./cmd/cairn

build:
	go build -o bin/cairn ./cmd/cairn

test:
	go test ./... -race

lint:
	go vet ./...

tidy:
	go mod tidy

fmt:
	go fmt ./...

clean:
	rm -rf bin/

cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1




