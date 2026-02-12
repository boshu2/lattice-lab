.PHONY: proto build test run run-sim run-radar-sim run-classifier run-task-manager run-fusion clean

proto:
	buf generate

build:
	go build -o bin/entity-store ./cmd/entity-store
	go build -o bin/sensor-sim ./cmd/sensor-sim
	go build -o bin/radar-sim ./cmd/radar-sim
	go build -o bin/classifier ./cmd/classifier
	go build -o bin/task-manager ./cmd/task-manager
	go build -o bin/fusion ./cmd/fusion
	go build -o bin/lattice-cli ./cmd/lattice-cli

test:
	go test ./...

run: build
	./bin/entity-store

run-sim: build
	./bin/sensor-sim

run-radar-sim: build
	./bin/radar-sim

run-classifier: build
	./bin/classifier

run-task-manager: build
	./bin/task-manager

run-fusion: build
	./bin/fusion

clean:
	rm -rf bin/
