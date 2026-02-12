# lattice-lab

A mini Lattice built in Go to learn distributed systems architecture — Entity-Component model, gRPC services, sensor fusion, task orchestration, mesh networking.

## Architecture

```
sensor-sim ──Create/Update──▶ entity-store ◀──Watch── classifier
                                   ▲                      │
                                   │                 Update (classification
                                   │                  + threat components)
                                   │
                         task-manager ──Watch──▶ entity-store
                              │                      ▲
                              └──Update (tasks)──────┘

mesh-relay: replicates entities between peer stores
lattice-cli: operator CLI (list, get, watch)
```

## Quick Start

```bash
# Build everything
make build

# Run tests (34 tests across 6 packages)
make test

# Start the full pipeline
make run              # Terminal 1: entity-store on :50051
make run-sim          # Terminal 2: sensor-sim (5 tracks, 1s updates)
make run-classifier   # Terminal 3: classifier (adds threat levels)
make run-task-manager # Terminal 4: task-manager (assigns tasks)

# Query with grpcurl
grpcurl -plaintext localhost:50051 store.v1.EntityStoreService/ListEntities
grpcurl -plaintext -d '{"type_filter": 2}' localhost:50051 store.v1.EntityStoreService/WatchEntities

# Or use the CLI
./bin/lattice-cli list
./bin/lattice-cli list -t track
./bin/lattice-cli get track-0
./bin/lattice-cli watch
```

## Services

| Service | Binary | Purpose |
|---------|--------|---------|
| **entity-store** | `bin/entity-store` | gRPC server with in-memory Entity-Component store |
| **sensor-sim** | `bin/sensor-sim` | Generates Track entities with dead-reckoning position updates |
| **classifier** | `bin/classifier` | Watches tracks, classifies by speed, adds threat levels |
| **task-manager** | `bin/task-manager` | Watches threat levels, assigns tasks via state machine |
| **lattice-cli** | `bin/lattice-cli` | Operator interface (list, get, watch) |
| **mesh-relay** | (library) | P2P entity replication between peer stores |

## Entity-Component Model

Entities are typed containers (`Asset`, `Track`, `Geo`) with composable components attached via `google.protobuf.Any`:

- **PositionComponent** — lat/lon/alt
- **VelocityComponent** — speed/heading
- **ClassificationComponent** — label + confidence
- **TaskCatalogComponent** — list of available tasks
- **ThreatComponent** — threat level enum (NONE, LOW, MEDIUM, HIGH)

## Configuration

All services use environment variables:

| Variable | Default | Used By |
|----------|---------|---------|
| `PORT` | `50051` | entity-store |
| `STORE_ADDR` | `localhost:50051` | sensor-sim, classifier, task-manager |
| `INTERVAL` | `1s` | sensor-sim |
| `NUM_TRACKS` | `5` | sensor-sim |

## Build Targets

```bash
make proto              # Regenerate proto code (requires buf)
make build              # Build all binaries to bin/
make test               # Run all tests
make run                # Start entity-store
make run-sim            # Start sensor-sim
make run-classifier     # Start classifier
make run-task-manager   # Start task-manager
make clean              # Remove bin/
```

## Deployment

Multi-stage Dockerfile builds all services into a single image. Kubernetes manifests in `deploy/k8s/` deploy entity-store, sensor-sim, classifier, and task-manager with gRPC health probes.

```bash
docker build -f deploy/Dockerfile -t lattice-lab .
kubectl apply -f deploy/k8s/
```

## Project Phases

| Phase | Component | Status |
|-------|-----------|--------|
| 1 | Entity Store (Proto + gRPC + CRUD + Watch) | Done |
| 2 | Sensor Simulator (gRPC client, track generation) | Done |
| 3 | Classifier (watch streams, threat classification) | Done |
| 4 | Task Manager (state machines, task orchestration) | Done |
| 5 | Lattice CLI (Cobra, operator interface) | Done |
| 6 | Mesh Relay (P2P entity replication) | Done |
| 7 | K8s Deployment (Dockerfile, manifests) | Done |
