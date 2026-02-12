# Pre-Mortem: Missing Requirements Judge

**Verdict: WARN** | Confidence: HIGH

**Key Insight:** Plan omits proto schema changes, Store.New() migration strategy, CRDT Any-type dispatch, replication loop prevention, and approval gate goroutine lifecycle management.

---

## Critical Findings

### 1. No proto schema evolution issue

HLC timestamps and CRDT merge require new fields on `entity.proto` (HLC timestamp fields, merge metadata). There is no issue covering:

- What fields get added to `Entity` message
- Running `buf generate` and updating `gen/`
- Verifying all 5 existing services still compile
- Wire-format backward compatibility (old clients reading new entities)

**Impact:** Every service imports `entityv1`. Proto changes ripple everywhere. Without planning this upfront, Wave 1 will stall on unexpected compile failures across the codebase.

**Fix:** Add explicit issue "Proto schema evolution for HLC/CRDT" as a Wave 1 prerequisite.

### 2. Store.New() breaking change unaddressed

`Store.New()` currently takes zero arguments. It is called in:

- `internal/store/store_test.go` (7 direct calls)
- `internal/server/grpc_test.go` (`startTestServer` helper)
- `cmd/entity-store/main.go`

Adding a required node ID parameter breaks all of these. The plan does not describe a migration path.

**Fix:** Use functional options pattern `New(opts ...Option)` so zero-arg calls still work with an auto-generated node ID. Or enumerate every call site in the issue scope.

### 3. CRDT merge needs type-aware component dispatch

Components are `map[string]*anypb.Any`. To merge two entities, you must:

1. Unmarshal each `Any` to its concrete type
2. Apply type-specific merge logic
3. Re-marshal back to `Any`

The plan does not define merge semantics per component type:

| Component | Merge Strategy |
|-----------|---------------|
| Position | Last-writer-wins (by HLC) |
| Velocity | Last-writer-wins (by HLC) |
| Classification | Higher confidence wins |
| Threat | Max threat level |
| TaskCatalog | Union of task lists |

Without this, the implementer will default to full-entity LWW, which defeats the purpose of component-level CRDT.

**Fix:** Define a `ComponentMerger` interface and document per-type strategies in the issue.

### 4. Replication loop prevention missing

Current mesh relay: Node A watches local store, forwards events to Node B. With CRDT merge, Node B's store emits an update event when it merges, which Node B's relay forwards back to Node A, which merges again, creating an infinite loop.

This is not a test edge case -- it will happen on every single entity update in a 2-node mesh.

**Fix:** Add origin-node tagging to replicated events. Relay skips events that originated from a remote peer. This is an architectural prerequisite for CRDT in the mesh.

---

## Significant Findings

### 5. Approval gate timer goroutine leak

The approval gate starts a timer goroutine per HIGH-threat entity. If the entity is deleted before the timer fires, the goroutine leaks. No cleanup mechanism is described.

**Fix:** Maintain a `map[string]context.CancelFunc` for pending approvals. Cancel on entity delete events and on shutdown.

### 6. Fused entity orphan lifecycle

Sensor fusion creates fused entities from source tracks. When sources are deleted (TTL expiry, manual delete), fused entities become orphans with stale data. No cleanup path is defined.

**Fix:** Fusion service must watch delete events and cascade-delete fused entities. Or wire fused entities into TTL with auto-refresh on each fusion cycle.

### 7. Bandwidth budget: size estimation and drop policy undefined

Token bucket needs byte sizes. Plan does not specify:
- Measurement approach (`proto.Size()` vs estimation)
- Behavior when budget exhausted (drop? queue? prioritize HIGH threats?)

**Fix:** Specify `proto.Size()` for measurement. Define drop-with-metric policy. Add priority exemption for HIGH threat entities.

### 8. Makefile and build targets for new binaries

`radar-sim` binary has no Makefile target. Current Makefile builds exactly 5 binaries. No issue covers this.

**Fix:** Include Makefile update in sensor fusion issue scope. Add `make build` as acceptance criteria.

### 9. CLI approve/deny commands unscoped

Approval gate requires new CLI commands. This needs either a new gRPC RPC or a component-based convention, plus Cobra subcommands. Estimated ~100+ lines. Not clearly scoped in any issue.

**Fix:** Explicitly scope within approval gate issue or create separate CLI issue.

### 10. Partition injection mechanism undefined

Partition test needs to block/unblock connections between specific peers dynamically. No mechanism exists in the codebase. The plan does not specify how partitions are simulated.

**Fix:** Specify approach: controllable net.Listener wrapper or gRPC interceptor that can be toggled in tests.

---

## Minor Findings

### 11. HLC node ID uniqueness

No strategy for ensuring unique node IDs. Duplicate IDs silently break HLC ordering.

**Fix:** Default to hostname + random suffix. Document uniqueness requirement.

### 12. GOALS.yaml not updated

No issue covers adding quality gates for the 6 new features to GOALS.yaml.

### 13. HLC must be built from scratch

The "no external dependencies" constraint means ~100-150 lines of hand-rolled HLC. The issue should acknowledge this and provide pseudocode or paper reference.

---

## Recommendation

**Before implementation, add 3 blocking prerequisite issues:**

1. **Proto schema evolution** -- Define new fields for HLC/CRDT, run buf generate, verify all services compile
2. **Replication loop prevention** -- Design origin tagging for mesh relay events
3. **Component merge strategy registry** -- Define per-type CRDT merge semantics with unmarshal/dispatch pipeline

These are architectural foundations. Deferring them will cause cascading rework across all 4 waves.
