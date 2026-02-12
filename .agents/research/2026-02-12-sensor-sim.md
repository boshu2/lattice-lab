# Research: Phase 2 — Sensor Simulator

**Date:** 2026-02-12
**Goal:** Build cmd/sensor-sim/ and internal/sensor/ that generates random Track entities and streams them to entity-store via gRPC client.

## Existing Architecture

- **Proto schemas**: `entity.v1.Entity` with composable components via `map<string, google.protobuf.Any>`. Five component types defined: Position, Velocity, Classification, TaskCatalog, Threat. `EntityType` enum has TRACK (value 2).
- **Store service**: `store.v1.EntityStoreService` with CreateEntity, GetEntity, ListEntities, UpdateEntity, DeleteEntity, WatchEntities (server streaming).
- **Generated client**: `gen/store/v1/store_grpc.pb.go` provides `EntityStoreServiceClient` interface and `NewEntityStoreServiceClient(cc grpc.ClientConnInterface)`.
- **gRPC client pattern** (from tests): `grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))` then `storev1.NewEntityStoreServiceClient(conn)`.
- **Component packing**: Use `anypb.New()` to pack typed components into `entity.Components` map.
- **Entry point pattern**: env-var config, graceful shutdown via signal notify, simple `log.Printf` for observability.
- **Test pattern**: `startTestServer(t)` spins up real gRPC server on random port, returns client + cleanup func.

## Key Decisions

- Tracks use dead-reckoning (advance lat/lon from speed + heading + elapsed time)
- DC metro bounding box for geographic realism (~38.8-39.0 lat, -77.2 to -76.9 lon)
- Entity IDs: `track-{n}` format
- Speed range: 100-500 knots (converted to m/s internally)
- Default 5 tracks, 1s interval, configurable via env vars
