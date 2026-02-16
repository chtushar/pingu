build:
	go build -o bin/pingu ./cmd/pingu

run:
	go run ./cmd/pingu

test:
	go test ./...

lint:
	golangci-lint run

.PHONY: build run test lint
