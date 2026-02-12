.PHONY: proto build test run run-sim run-classifier run-task-manager clean

proto:
	buf generate

build:
	go build -o bin/entity-store ./cmd/entity-store
	go build -o bin/sensor-sim ./cmd/sensor-sim
	go build -o bin/classifier ./cmd/classifier
	go build -o bin/task-manager ./cmd/task-manager
	go build -o bin/lattice-cli ./cmd/lattice-cli

test:
	go test ./...

run: build
	./bin/entity-store

run-sim: build
	./bin/sensor-sim

run-classifier: build
	./bin/classifier

run-task-manager: build
	./bin/task-manager

clean:
	rm -rf bin/
