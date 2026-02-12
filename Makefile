.PHONY: proto build test run clean

proto:
	buf generate

build:
	go build -o bin/entity-store ./cmd/entity-store

test:
	go test ./...

run: build
	./bin/entity-store

clean:
	rm -rf bin/
