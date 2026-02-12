# Continuation Prompt for lattice-lab

Copy/paste this to start the next session:

---

## Context

I'm building `lattice-lab` — a mini Lattice in Go to learn distributed systems architecture (gRPC, Protobuf, Entity-Component model, microservices). Phase 1 (entity-store) is complete with 16 passing tests and verified grpcurl smoke tests. No commits yet — needs initial commit then Phase 2.

## Read First

1. Handoff: `.agents/handoff/2026-02-12-lattice-lab-phase1.md`
2. Proto schemas: `proto/entity/v1/entity.proto` and `proto/store/v1/store.proto`
3. Store implementation: `internal/store/store.go`
4. gRPC server: `internal/server/grpc.go`

## What I Need Help With

1. **Initial commit** — all Phase 1 files need to be committed (nothing is committed yet)
2. **Phase 2: Sensor Simulator** — build `cmd/sensor-sim/` and `internal/sensor/` that generates random Track entities and streams them to entity-store via gRPC client

## Key Files

```
proto/entity/v1/entity.proto      # Entity + Component types
proto/store/v1/store.proto         # EntityStoreService (CRUD + Watch)
internal/store/store.go            # In-memory store with watchers
internal/server/grpc.go            # gRPC handler
cmd/entity-store/main.go           # Server entry point
gen/store/v1/store_grpc.pb.go      # Generated client stubs (use for sensor-sim)
Makefile                           # Build targets
```

## Open Questions

1. Should we create the GitHub repo now or keep it local-only for now?
2. For sensor-sim data generation — what geographic area should tracks spawn in? (suggestion: use a bounding box around a test area)

---

Start with: commit Phase 1, then `/implement` Phase 2 (Sensor Simulator)
