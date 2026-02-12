# Plan: Distributed Systems Enhancements for Anduril Interview Prep

**Date:** 2026-02-12
**Source:** `.agents/council/2026-02-12-brainstorm-lattice-enhancements.md`

## Overview

Add 6 production-grade distributed systems features to lattice-lab: HLC timestamps, CRDT component merge, partition test harness, human-on-the-loop approval gate, edge bandwidth budgeting, and sensor fusion. Test-driven — tests written before implementation for each feature.

## Boundaries

**Always:**
- Test-driven: write `_test.go` files first, then implementation
- All Go — no new languages
- Existing tests must continue passing (`make test`)
- New packages get their own `_test.go` files
- Proto changes regenerated via `make proto`
- Keep existing API backward-compatible

**Ask First:**
- (auto mode — logged only) Any changes to existing proto field numbers

**Never:**
- Break existing sensor-sim / classifier / task-manager / mesh-relay behavior
- Add external dependencies beyond Go stdlib + existing deps (grpc, protobuf, cobra)
- Modify generated code in `gen/`

## Conformance Checks

| Issue | Check Type | Check |
|-------|-----------|-------|
| 1 (HLC) | tests | `go test ./internal/hlc/...` |
| 1 (HLC) | content_check | `{file: "internal/hlc/hlc.go", pattern: "func.*Now"}` |
| 2 (Proto) | command | `make proto && go build ./...` |
| 2 (Proto) | content_check | `{file: "proto/entity/v1/entity.proto", pattern: "hlc_timestamp"}` |
| 3 (Store HLC) | tests | `go test ./internal/store/...` |
| 3 (Store HLC) | content_check | `{file: "internal/store/store.go", pattern: "hlc"}` |
| 4 (CRDT) | tests | `go test ./internal/crdt/...` |
| 4 (CRDT) | content_check | `{file: "internal/crdt/merge.go", pattern: "func Merge"}` |
| 5 (Approval) | tests | `go test ./internal/task/...` |
| 5 (Approval) | content_check | `{file: "internal/task/manager.go", pattern: "pending_approval"}` |
| 6 (Mesh CRDT) | tests | `go test ./internal/mesh/...` |
| 7 (Fusion) | tests | `go test ./internal/fusion/...` |
| 7 (Fusion) | content_check | `{file: "internal/fusion/fusion.go", pattern: "func.*Correlate"}` |
| 8 (Bandwidth) | tests | `go test ./internal/mesh/...` |
| 8 (Bandwidth) | content_check | `{file: "internal/mesh/budget.go", pattern: "TokenBucket"}` |
| 9 (Partition) | tests | `go test ./internal/mesh/ -run Partition -count=1 -timeout=60s` |
| All | tests | `make test` |

## Issues

### Issue 1: HLC Package (internal/hlc/)
**Dependencies:** None
**Wave:** 1
**Acceptance:** `go test ./internal/hlc/...` passes; HLC.Now(), HLC.Update(remote), HLC.Compare() all work; concurrent access safe
**Description:**
Create `internal/hlc/` package implementing Hybrid Logical Clocks. TDD approach:
1. Write tests first: monotonicity, causality (Update advances past remote), concurrent safety, comparison
2. Implement: `Clock` struct with physical time + logical counter + node ID. `Now()` returns new HLC. `Update(remote HLC)` advances to max(local, remote)+1. `Compare(a, b)` returns ordering.
3. HLC struct should be protobuf-serializable (uint64 physical + uint32 logical + string node_id)

### Issue 2: Proto Schema Updates
**Dependencies:** None
**Wave:** 1
**Acceptance:** `make proto` succeeds; `go build ./...` succeeds; new fields visible in generated code
**Description:**
Add all new proto fields/messages needed by later phases in one batch:
- `entity.proto`: Add `uint64 hlc_physical = 6`, `uint32 hlc_logical = 7`, `string hlc_node = 8` to Entity. Add `ApprovalState` enum (AUTO_APPROVED, PENDING, APPROVED, DENIED, TIMED_OUT). Add `ApprovalComponent` message. Add `FusionComponent` message (repeated string source_ids, double fused_lat, double fused_lon, float confidence). Add `SourceComponent` message (string sensor_id, string sensor_type).
- `store.proto`: Add `ApproveAction` and `DenyAction` RPCs.
Run `make proto` to regenerate.

### Issue 3: Store HLC Integration
**Dependencies:** Issue 1, Issue 2
**Wave:** 2
**Acceptance:** `go test ./internal/store/...` passes; entities have HLC timestamps; Update rejects stale writes (HLC comparison)
**Description:**
TDD: Write tests first verifying that Create stamps HLC, Update advances HLC, and stale updates (lower HLC) are rejected with a conflict error. Then:
1. Add `hlc.Clock` to `Store` struct, initialized with a node ID
2. In `Create()`: stamp entity's HLC fields from `clock.Now()`
3. In `Update()`: compare incoming HLC with stored; if incoming is older, return conflict error; otherwise advance clock and stamp
4. Propagate HLC in watch events so downstream consumers see causal ordering

### Issue 4: CRDT Merge Package (internal/crdt/)
**Dependencies:** Issue 1
**Wave:** 2
**Acceptance:** `go test ./internal/crdt/...` passes; LWW, MaxWins, ORSet strategies all work with property-based invariants
**Description:**
TDD: Write tests first for each merge strategy verifying commutativity (merge(a,b) == merge(b,a)), associativity, and idempotency. Then implement:
1. `LWWRegister` — last-writer-wins keyed on HLC. For position/velocity components.
2. `MaxWins` — keep the higher value. For threat level (NONE < LOW < MEDIUM < HIGH).
3. `ORSet` — observed-remove set, add wins over remove. For task catalog.
4. `MergeEntity(local, remote) → merged` — applies per-component strategy based on component key name:
   - "position", "velocity" → LWW
   - "threat" → MaxWins
   - "classification" → LWW (higher confidence wins as tiebreaker)
   - "task_catalog" → ORSet
   - unknown → LWW (default)

### Issue 5: Human-on-the-Loop Approval Gate
**Dependencies:** Issue 2
**Wave:** 2
**Acceptance:** `go test ./internal/task/...` passes; HIGH threat → pending_approval state; approve/deny work via gRPC; timeout auto-denies; CLI commands work
**Description:**
TDD: Write tests for state machine transitions: HIGH → pending_approval, approve → intercept, deny → idle, timeout → timed_out (auto-deny). Then:
1. Add `ApprovalState` tracking to Manager (map[entityID]ApprovalState)
2. When threat = HIGH: set state to pending_approval instead of immediate intercept. Start a goroutine timer (configurable, default 30s).
3. Implement `ApproveAction(entity_id)` and `DenyAction(entity_id)` in server/grpc.go
4. On approve: transition to intercept, assign tasks
5. On deny or timeout: transition to idle
6. Add `lattice-cli approve <id>` and `lattice-cli deny <id>` commands

### Issue 6: Mesh Relay CRDT Merge
**Dependencies:** Issue 3, Issue 4
**Wave:** 3
**Acceptance:** `go test ./internal/mesh/...` passes; relay uses component-level merge; concurrent updates from different peers converge deterministically
**Description:**
TDD: Write tests verifying that two relays forwarding conflicting updates to the same entity produce identical merged state on both peers. Then:
1. Replace `forwardCreate`/`forwardUpdate` with merge-based replication
2. On receive: fetch current entity from peer, merge with incoming using `crdt.MergeEntity()`, write merged result
3. Track merge stats (merges, conflicts resolved, components kept from local vs remote)
4. Handle new entity case (no existing → just create)

### Issue 7: Sensor Fusion Service
**Dependencies:** Issue 2, Issue 3
**Wave:** 3
**Acceptance:** `go test ./internal/fusion/...` passes; `cmd/radar-sim` builds; overlapping tracks from two sensors merge into single fused entity
**Description:**
TDD: Write tests for track correlation (two tracks within distance threshold → correlated), de-correlation (tracks diverge → split), and fused position calculation. Then:
1. `cmd/radar-sim/`: like sensor-sim but noisier position-only data with `SourceComponent{sensor_id, sensor_type="radar"}`
2. Modify sensor-sim to add `SourceComponent{sensor_type="eo"}` to its tracks
3. `internal/fusion/fusion.go`: Watch all tracks. Maintain correlation matrix (pairwise distances). When two tracks from different sensors are within threshold → create fused entity with `FusionComponent`. Update fused position as weighted average.
4. Handle de-correlation when tracks diverge beyond threshold.

### Issue 8: Edge Bandwidth Budgeting
**Dependencies:** Issue 6
**Wave:** 4
**Acceptance:** `go test ./internal/mesh/...` passes; budget enforced; HIGH threat events always get through; coalescing reduces duplicate position updates
**Description:**
TDD: Write tests verifying: HIGH threat always forwarded even at 0 budget; LOW threat dropped when over budget; coalescing keeps only latest position update per entity. Then:
1. `internal/mesh/budget.go`: `TokenBucket` struct with configurable bytes/sec rate and burst
2. `PriorityQueue` that orders events: delete > HIGH > MEDIUM > LOW > NONE > position-only
3. `Coalescer` that deduplicates: if same entity has multiple pending position updates, keep only latest
4. Integrate into relay: events flow through prioritize → coalesce → budget gate → forward
5. Config via env vars: `MESH_BANDWIDTH_BPS`, `MESH_BURST_BYTES`

### Issue 9: Partition Test Harness
**Dependencies:** Issue 6
**Wave:** 4
**Acceptance:** Partition tests pass; 3-store mesh survives partition + conflicting updates + heal → all stores converge
**Description:**
TDD (this IS the test):
1. `internal/mesh/partition_test.go`: Programmable TCP proxy that wraps `net.Listener` and can drop/delay/reorder packets on command
2. Test scenario: spin up 3 entity-stores + mesh-relays in a triangle topology
3. Create entity on store-A, verify replicated to B and C
4. Partition: isolate store-B from A and C
5. Update entity on A (speed=100kts), update same entity on B (speed=400kts)
6. Heal partition
7. Assert: all 3 stores converge to same state (speed=400kts wins via HLC or CRDT rule)
8. Assert: no entity lost, all components present

## Execution Order

**Wave 1** (parallel): Issue 1 (HLC), Issue 2 (Proto)
**Wave 2** (after Wave 1, parallel): Issue 3 (Store HLC), Issue 4 (CRDT), Issue 5 (Approval Gate)
**Wave 3** (after Wave 2, parallel): Issue 6 (Mesh CRDT), Issue 7 (Sensor Fusion)
**Wave 4** (after Wave 3, parallel): Issue 8 (Bandwidth), Issue 9 (Partition Tests)

## Estimated New Code
- ~1,200-1,500 lines of implementation
- ~800-1,000 lines of tests
- Total: ~2,000-2,500 lines added to the existing ~1,500 line codebase

## Next Steps
- Run `/pre-mortem` for failure simulation
- Then `/crank` for autonomous execution
