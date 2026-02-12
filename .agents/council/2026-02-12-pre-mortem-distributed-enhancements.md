# Pre-Mortem: Distributed Systems Enhancements

**Date:** 2026-02-12
**Plan:** `.agents/plans/2026-02-12-distributed-systems-enhancements.md`
**Mode:** 2 judges (feasibility + missing-requirements)

## Council Verdict: WARN

Both judges independently reached WARN with HIGH confidence.

| Judge | Verdict | Key Finding |
|-------|---------|-------------|
| Feasibility | WARN | HLC stale-write rejection breaks classifier/task-manager update pattern |
| Missing-Requirements | WARN | Replication loop prevention, Store.New() breaking change, proto evolution unscoped |

## Shared Findings (both judges flagged)

1. **Replication echo loop** — CRDT merge on peer emits update event, relay forwards back, infinite loop. Origin-ID suppression mandatory.
2. **CRDT type dispatch on anypb.Any** — Must unmarshal each component to apply type-specific merge. Plan underspecified.
3. **ORSet is wrong** — Components are a map of named registers, not a set. LWW-Element-Map is correct and 3x less code.

## Critical Design Decisions Required

### 1. HLC Scope: Per-Entity vs Per-Component-Key
**Problem:** Classifier and task-manager do watch→mutate→update. If sensor-sim updates the entity between watch delivery and the update call, the classifier's HLC is stale. Rejecting stale writes silently drops classifications.
**Decision:** Use per-component-key HLC. Each component carries its own HLC. Store.Update() merges per-key — only rejects if the *same component key* has a newer HLC. Different services writing different keys never conflict.

### 2. Store.New() API Change
**Problem:** Adding node ID breaks 9+ call sites.
**Decision:** Use functional options: `New(opts ...Option)`. Default auto-generates node ID from hostname+random. Tests use `WithNodeID("test-node")`.

### 3. Relay Echo Suppression
**Problem:** Bidirectional relay loops infinitely with CRDT merge.
**Decision:** Add `origin_node` field to EntityEvent. Relay tags outgoing events with local node ID. Receiving relay skips events where origin_node == self.

## Plan Amendments

Based on pre-mortem findings, the following changes to the plan:

1. **Issue 4: Replace ORSet with LWW-Element-Map** — Simpler, correct for the domain, ~100 lines instead of 500+
2. **Issue 3: Per-component-key HLC** — Not per-entity. Store.Update() merges component maps by key, each key carrying its own HLC
3. **Issue 3: Store.New() uses functional options** — Backward-compatible, no call-site breaks
4. **Issue 6: Add origin-node echo suppression** — Prerequisite for mesh CRDT, add origin_node to EntityEvent proto
5. **Issue 5: Single-goroutine event loop** — Replace per-entity timer goroutines with select-based loop + cancel map
6. **Issue 7: Add fused entity cleanup** — Watch source deletes, cascade-delete fused entities
7. **Issue 8: Use proto.Size() for byte estimation** — Specify in description
8. **Issue 9: Use listener-level partition** — Refuse connections instead of TCP proxy (~50 lines vs 200+)
9. **Proto Issue 2: Add origin_node to EntityEvent**

## Recommendation

PROCEED with amendments. The findings are design clarifications, not fundamental flaws. Incorporating them into issue descriptions before /crank prevents rework during implementation.
