# lattice-lab

Mini Lattice in Go — distributed systems learning lab.

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

## Project Structure

```
cmd/                    # 5 binaries
  entity-store/         # gRPC server (port 50051)
  sensor-sim/           # Track generator (gRPC client)
  classifier/           # Speed-based threat classification
  task-manager/         # Threat-to-task state machine
  lattice-cli/          # Cobra CLI

internal/               # Core packages
  store/                # Thread-safe in-memory entity store + watchers
  server/               # gRPC handler (EntityStoreService)
  sensor/               # Dead-reckoning track simulator
  classifier/           # Classify() by speed → label + threat level
  task/                 # Rules() by threat → state + task list
  mesh/                 # P2P entity replication relay

proto/                  # Protobuf schemas
  entity/v1/            # Entity, Components, EntityType, ThreatLevel
  store/v1/             # EntityStoreService (CRUD + WatchEntities)

gen/                    # buf-generated Go code (do not edit)
deploy/                 # Dockerfile + K8s manifests
```

## Build & Test

```bash
make build              # Build all 5 binaries to bin/
make test               # go test ./...
make proto              # buf generate (regenerate proto code)
make run                # Start entity-store on :50051
make run-sim            # Start sensor-sim (needs entity-store running)
make run-classifier     # Start classifier (needs entity-store running)
make run-task-manager   # Start task-manager (needs entity-store running)
```

## Conventions

- **Go module**: `github.com/boshu2/lattice-lab`
- **Proto packages**: `entity.v1`, `store.v1` — generated to `gen/`
- **Components**: Packed via `anypb.New()` into `entity.Components` map with string keys (`position`, `velocity`, `classification`, `threat`, `task_catalog`)
- **gRPC clients**: Use `grpc.NewClient()` + `insecure.NewCredentials()`
- **Config**: Env vars (`STORE_ADDR`, `PORT`, `INTERVAL`, `NUM_TRACKS`)
- **Tests**: Co-located `_test.go` files. Integration tests spin up real gRPC server on random port via `startTestServer(t)` helper
- **Entity IDs**: Format `track-{n}` for simulator, free-form for manual creation

## Key Types

| Type | Package | Purpose |
|------|---------|---------|
| `store.Store` | internal/store | Thread-safe entity map + watcher notifications |
| `store.Watcher` | internal/store | Channel-based event subscription |
| `server.Server` | internal/server | gRPC handler wrapping Store |
| `sensor.Simulator` | internal/sensor | Generates tracks with position/velocity |
| `sensor.Config` | internal/sensor | StoreAddr, Interval, NumTracks, BBox |
| `classifier.Classifier` | internal/classifier | Watches tracks, adds classification+threat |
| `classifier.Classification` | internal/classifier | Label, Confidence, ThreatLevel |
| `task.Manager` | internal/task | Watches threats, assigns task catalogs |
| `task.Assignment` | internal/task | EntityID, State, Tasks |
| `mesh.Relay` | internal/mesh | Replicates entities between peer stores |

## Classification Rules

| Speed (kts) | Label | Threat |
|-------------|-------|--------|
| < 150 | civilian | NONE |
| 150-350 | aircraft | LOW |
| > 350 | military | HIGH |

## Task Assignment Rules

| Threat | State | Tasks |
|--------|-------|-------|
| NONE | idle | — |
| LOW | investigate | monitor, identify |
| MEDIUM | track | monitor, identify, track |
| HIGH | intercept | monitor, identify, track, intercept |
