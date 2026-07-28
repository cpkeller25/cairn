.PHONY: run build test lint tidy fmt clean cover

run:
	PORT=8081 go run ./cmd/cairn

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
	