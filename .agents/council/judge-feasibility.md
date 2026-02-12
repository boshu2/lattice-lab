# Feasibility Pre-Mortem: lattice-lab Enhancement Plan

**Verdict: WARN** | **Confidence: HIGH**

**Key insight:** HLC stale-write rejection will silently break classifier and task-manager update loops, and the ORSet + partition proxy are each underscoped by at least 2x.

---

## Critical Findings

### 1. HLC Stale-Write Rejection Breaks Existing Callers (Issue 3)

This is the single biggest risk in the plan.

The classifier and task-manager both follow the same pattern:

```
Watch event arrives -> mutate entity components in-place -> call UpdateEntity
```

Look at `classifier.go:104-136`:
```go
func (c *Classifier) classifyEntity(ctx context.Context, client storev1.EntityStoreServiceClient, entity *entityv1.Entity) error {
    // entity is a snapshot from the watch event
    speed, err := extractSpeed(entity)
    cl := Classify(speed)
    entity.Components["classification"] = clComp   // mutate snapshot
    entity.Components["threat"] = threatComp
    client.UpdateEntity(ctx, &storev1.UpdateEntityRequest{Entity: entity})  // send stale snapshot
}
```

Between the watch event delivery and the UpdateEntity call, the sensor-sim may have already updated the same entity with a newer HLC timestamp. The classifier's update carries the old HLC. If Issue 3 rejects stale writes, the classification is silently dropped. The task-manager has the identical problem.

This is not a bug to fix in the caller -- it reveals that whole-entity replacement with HLC gating is architecturally incompatible with the existing component-per-service design. Each service owns different component keys but overwrites the entire entity.

**Recommendation:** Either implement HLC per-component-key (not per-entity), add a compare-and-swap retry loop, or scope HLC rejection to mesh relay only.

### 2. ORSet is the Wrong CRDT for This Domain (Issue 4)

A proper ORSet requires:
- Unique tags per add operation
- A causal context (vector clock or dotted version vector)
- Merge of causal contexts on sync
- Serialization of the tag set per element

This is 500+ lines for correctness, not 300. More importantly, the entity store's `map[string]*anypb.Any` component map is not a set -- it is a map of named registers. The actual merge semantics needed are "keep the component with the newer timestamp per key," which is an LWW-Element-Map, not an ORSet.

**Recommendation:** Replace ORSet with LWW-Element-Map (~100 lines). One HLC timestamp per component key. This solves the real problem and is implementable in the budget.

---

## Significant Findings

### 3. Mesh CRDT Merge is Non-Atomic with Echo Amplification (Issue 6)

The proposed GET+merge+PUT on the peer store has a race window. More critically, bidirectional relay (already tested in `TestRelayBidirectional`) will create infinite echo loops: relay1 forwards to store2, relay2 sees the update event and forwards back to store1, ad infinitum. The current code survives this by luck (stable data, no actual mutations during relay). With CRDT merge producing new update events on every merge, the echo becomes infinite.

**Recommendation:** Add origin-ID or source-node metadata to suppress relay echo. This is mandatory before any mesh CRDT work.

### 4. Approval Gate Timer Races (Issue 5)

Three race conditions with per-entity timer goroutines:
1. Timer fires concurrently with explicit approve -- double state transition
2. De-escalation cancels pending approval while timer callback executes
3. Entity deleted while timer is pending -- update to non-existent entity

**Recommendation:** Use a single-goroutine event loop with select (approve/deny/timer/cancel channels) instead of per-entity goroutines. Inject a clock interface for deterministic tests.

### 5. TCP Proxy for Partition Tests is Harder Than It Looks (Issue 9)

A raw TCP proxy that drops bytes mid-stream will cause gRPC HTTP/2 protocol errors, not clean partition behavior. gRPC will report stream resets and protocol violations rather than the Unavailable errors that partition-tolerant code expects. Getting clean partition semantics requires understanding gRPC's HTTP/2 framing.

**Recommendation:** Use listener-level interception (refuse new connections, close existing ones) for ~50 lines instead of a full TCP proxy at ~200+ lines. Or test at the mock-client level with injected Unavailable errors.

### 6. Sensor Fusion Distance Threshold (Issue 7)

Static distance threshold fails across altitude bands and speed regimes. Two tracks 5km apart at 30,000ft may be the same aircraft; two tracks 1km apart at ground level are different vehicles.

**Recommendation:** For a learning lab: simple 2D lat/lon Euclidean distance with a configurable threshold (default ~0.01 degrees), require matching EntityType. Document as simplification. Do not attempt adaptive thresholds.

### 7. Proto Batch is a Single Point of Failure (Issue 2)

All proto changes in one issue means one mistake blocks all 4 waves. Proto iteration is slow (edit .proto, buf generate, fix Go, re-test). Getting the HLC field placement wrong (entity-level vs component-level) forces rework across every downstream issue.

**Recommendation:** Split into Issue 2a (HLC + CRDT fields for Wave 1) and Issue 2b (approval gate + fusion + budget fields for Waves 2-3).

---

## Minor Findings

### 8. Bandwidth Budget Couples Relay to Domain Logic (Issue 8)

The relay is currently domain-agnostic. Priority-based throttling requires inspecting threat components, breaking this clean separation.

**Recommendation:** Use a generic `priority` metadata field on Entity rather than inspecting threat components in the relay.

### 9. False Dependency in Wave Structure

Issue 6 (Mesh CRDT) depends on Issue 4 (ORSet). If ORSet is replaced with LWW-Map, Issue 6 only needs Issue 3 (HLC). This removes one link from the critical path and allows Issues 4 and 6 to parallelize.

---

## Bottom Line

Three changes before starting implementation:

1. **Resolve the HLC + multi-service-update conflict** -- this is a design decision that changes the Store API contract and affects Issues 3, 6, and every existing caller.
2. **Replace ORSet with LWW-Element-Map** -- right tool for the actual problem, 3x less code.
3. **Split proto Issue 2 into two batches** -- reduces blast radius of getting proto wrong.

These changes reduce plan risk from "likely to stall in Wave 1" to "achievable with expected friction."
