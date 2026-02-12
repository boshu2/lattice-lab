# Handoff: lattice-lab Phase 1 Complete

**Date:** 2026-02-12
**Session:** Built Phase 1 (Proto + Entity Store) of lattice-lab from scratch
**Status:** Between tasks — Phase 1 done, Phase 2 (Sensor Simulator) ready to start

---

## What We Accomplished This Session

### 1. Project scaffolding

Created `~/gt/lattice-lab/` as standalone Go module (`github.com/boshu2/lattice-lab`). Initialized git repo, Go module, directory structure.

### 2. Protobuf schemas

Designed Entity-Component data model mirroring Lattice's architecture:

- `proto/entity/v1/entity.proto` — `Entity` message with `EntityType` enum (ASSET, TRACK, GEO), composable components via `map<string, google.protobuf.Any>`, 5 component types (Position, Velocity, Classification, TaskCatalog, Threat)
- `proto/store/v1/store.proto` — `EntityStoreService` with full CRUD + `WatchEntities` server streaming RPC

### 3. Code generation

Configured `buf` (v1.65.0) with `buf.yaml` + `buf.gen.yaml`. Generated Go code into `gen/` via `buf generate`.

### 4. In-memory entity store (`internal/store/`)

- `store.go` — Thread-safe store using `sync.RWMutex`, CRUD operations, watcher pattern with type filtering, `proto.Clone` for immutability
- `store_test.go` — 10 tests covering create, get, duplicate, not-found, list+filter, update, delete, watch, filtered watch

### 5. gRPC server (`internal/server/`)

- `grpc.go` — Implements `EntityStoreServiceServer` interface. Proper gRPC status codes (InvalidArgument, NotFound, AlreadyExists). WatchEntities uses `grpc.ServerStreamingServer[EntityEvent]` generic.
- `grpc_test.go` — 6 integration tests spinning up real gRPC server on random port, testing full round-trip

### 6. Entry point + build system

- `cmd/entity-store/main.go` — gRPC server with reflection enabled, graceful shutdown on SIGINT/SIGTERM, configurable port via `PORT` env
- `Makefile` — targets: `proto`, `build`, `test`, `run`, `clean`
- `README.md` — Quick start with grpcurl examples

### 7. Verification

- **16 tests pass** (10 store + 6 gRPC integration)
- **Binary builds clean** to `bin/entity-store`
- **grpcurl smoke test passed:** Create, Get, List (unfiltered + type-filtered), Update, Delete all working
- **gRPC reflection working** — `grpcurl list` shows `store.v1.EntityStoreService`

---

## Where We Paused

**Last action:** Completed full Phase 1 verification (tests + build + grpcurl smoke test)
**Next action:** Initial git commit, then start Phase 2 (Sensor Simulator)

### Remaining setup before Phase 2:
1. Create GitHub repo `boshu2/lattice-lab` (or local-only is fine)
2. Initial commit of all Phase 1 files
3. Begin Phase 2: sensor-sim service that generates Track entities and streams them to entity-store

---

## Phase 2 Plan (Sensor Simulator)

From the master plan, Phase 2 adds:

```
cmd/sensor-sim/main.go           — gRPC client, connects to entity-store
internal/sensor/simulator.go     — Generates random Track entities (position, velocity)
internal/sensor/simulator_test.go
```

Key concepts to implement:
- gRPC **client** (Phase 1 was server-only)
- Configurable data generation rate
- Streams Track entities to entity-store via `CreateEntity` RPC
- Simulates radar returns / camera detections

Success criteria: Start sensor-sim → Tracks appear in entity-store → visible via grpcurl or watch stream.

---

## Full Build Plan (Phases 3-7)

| Phase | Service | Status |
|-------|---------|--------|
| 1 | entity-store (Proto + gRPC + Entity-Component) | **DONE** |
| 2 | sensor-sim (gRPC client, streaming data gen) | Next |
| 3 | classifier (watch streams, threat classification) | Planned |
| 4 | task-manager (orchestration, state machines) | Planned |
| 5 | lattice-cli (Cobra CLI, operator interface) | Planned |
| 6 | mesh-relay (P2P, partition tolerance) | Planned |
| 7 | K8s deployment (Dockerfile, manifests) | Planned |

---

## Tools Installed This Session

Via Homebrew:
- `buf` v1.65.0 — Protobuf generation
- `grpcurl` — gRPC CLI testing
- `protoc-gen-go` — Go Protobuf plugin
- `protoc-gen-go-grpc` — Go gRPC plugin

---

## Files to Read

```
# Proto schemas (understand the data model)
proto/entity/v1/entity.proto
proto/store/v1/store.proto

# Core implementation
internal/store/store.go
internal/server/grpc.go

# Entry point + build
cmd/entity-store/main.go
Makefile

# Generated code (reference for client usage in Phase 2)
gen/store/v1/store_grpc.pb.go

# Tests (patterns to follow)
internal/store/store_test.go
internal/server/grpc_test.go
```
