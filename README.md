# lattice-lab

A mini Lattice built in Go to learn distributed systems architecture — Entity-Component model, gRPC services, sensor fusion, task orchestration, mesh networking.

## Phase 1: Entity Store

In-memory gRPC entity store implementing the Entity-Component data model.

### Quick Start

```bash
# Generate proto code
make proto

# Run tests
make test

# Start the server
make run

# In another terminal — CRUD via grpcurl
grpcurl -plaintext -d '{"entity":{"id":"drone-1","type":"ENTITY_TYPE_ASSET"}}' \
  localhost:50051 store.v1.EntityStoreService/CreateEntity

grpcurl -plaintext -d '{"id":"drone-1"}' \
  localhost:50051 store.v1.EntityStoreService/GetEntity

grpcurl -plaintext localhost:50051 store.v1.EntityStoreService/ListEntities

# Watch for changes (streams)
grpcurl -plaintext localhost:50051 store.v1.EntityStoreService/WatchEntities
```

### Architecture

```
entity-store (gRPC server, port 50051)
├── proto/entity/v1/entity.proto   — Entity, Components (Position, Velocity, Threat, etc.)
├── proto/store/v1/store.proto     — EntityStoreService (CRUD + Watch)
├── internal/store/store.go        — Thread-safe in-memory store with watchers
└── internal/server/grpc.go        — gRPC handler implementing EntityStoreService
```

### Entity-Component Model

Entities are typed containers (Asset, Track, Geo) with composable components attached via `google.protobuf.Any`:

- **PositionComponent** — lat/lon/alt
- **VelocityComponent** — speed/heading
- **ClassificationComponent** — label + confidence
- **TaskCatalogComponent** — list of available tasks
- **ThreatComponent** — threat level enum
