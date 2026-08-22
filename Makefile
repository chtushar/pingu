BINARY := bin/pingu

.PHONY: build test test-race vet fmt fmt-check check clean

build:
	go build -o $(BINARY) ./cmd/pingu

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "files need gofmt:"; gofmt -l .; exit 1)

check: fmt-check vet test test-race build

clean:
	rm -rf bin
