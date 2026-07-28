.PHONY: run build test lint tidy fmt clean

run:
	PORT=8081 go run ./cmd/cairn

build:
	go build -o bin/cairn ./cmd/cairn

test:
	go test ./...

lint:
	go vet ./...

tidy:
	go mod tidy

fmt:
	go fmt ./...

clean:
	rm -rf bin/
