# Plan: Phase 2 — Sensor Simulator

**Epic:** Sensor Simulator
**Date:** 2026-02-12

## Files

### 1. `internal/sensor/simulator.go` (new)

**Config struct:**
- `StoreAddr string` (default `localhost:50051`)
- `Interval time.Duration` (default `1s`)
- `NumTracks int` (default `5`)
- Bounding box: DC metro (38.8-39.0 lat, -77.2 to -76.9 lon)

**Simulator struct:**
- Holds config, gRPC client conn, active tracks slice
- `New(cfg Config) *Simulator`
- `Run(ctx context.Context) error` — connect, init tracks, tick loop
- `advanceTrack(t *track, dt time.Duration)` — dead-reckoning position update

**Track lifecycle per tick:**
- First tick: `CreateEntity` with random position in bbox, random speed (100-500 kts), random heading (0-360)
- Subsequent ticks: advance position via dead-reckoning, `UpdateEntity` with new Position + Velocity
- Pack components: `anypb.New(&entityv1.PositionComponent{...})` into `entity.Components["position"]`

**Dead-reckoning math (flat earth, sufficient for small bbox):**
- `dlat = speedMps * cos(heading) * dt.Seconds() / 111_320`
- `dlon = speedMps * sin(heading) * dt.Seconds() / (111_320 * cos(lat))`

### 2. `internal/sensor/simulator_test.go` (new)

- `TestAdvanceTrack` — verify position changes
- `TestGenerateTrack` — verify initial track within bbox, valid speed/heading
- `TestSimulatorIntegration` — real gRPC server, create + update round-trip

### 3. `cmd/sensor-sim/main.go` (new)

- Parse env: `STORE_ADDR`, `INTERVAL`, `NUM_TRACKS`
- Create config + simulator
- `sim.Run(ctx)` with SIGINT/SIGTERM cancellation
- Log each create/update

### 4. `Makefile` (modify)

- Add `build-sim` target
- Update `build` to build both binaries
- Add `run-sim` target

## Verification

1. `go test ./...` — all tests pass
2. Terminal 1: `make run` → entity-store on :50051
3. Terminal 2: `make run-sim` → tracks stream in
4. Terminal 3: `grpcurl -plaintext localhost:50051 store.v1.EntityStoreService/ListEntities` → shows tracks
