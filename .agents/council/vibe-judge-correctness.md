# Vibe Judge: Correctness & Safety

**Verdict:** WARN
**Confidence:** HIGH
**Key Insight:** HLC monotonicity is correct and CRDT semantics are sound, but found critical race condition in store.Update(), goroutine leak in relay, and missing error propagation in task manager.

## Findings

### Critical

1. **Store.Update() Component Merge Race Condition** (`internal/store/store.go:157-201`)
   - Lines 172-187 perform component-by-component merge under write lock
   - The incoming HLC comparison `hlc.Compare(incomingHLC, existingHLC) >= 0` is checked ONCE but used for ALL components
   - If two concurrent Update() calls arrive with different HLCs, the per-component merge can produce inconsistent state
   - Example: Thread A (HLC=100) and Thread B (HLC=200) both update entity simultaneously
     - Thread A gets lock first, merges with HLC=100 baseline
     - Thread B gets lock second, but `existingHLC` is now stale from Thread A's partial merge
   - **Fix:** Move HLC comparison inside the per-component loop or use store-level CRDT merge via `crdt.MergeEntity()`
   - **Impact:** Can violate CRDT monotonicity under concurrent updates from multiple relays

2. **Relay Goroutine Leak on Context Cancellation** (`internal/mesh/relay.go:73-131`)
   - `Run()` spawns a goroutine for the watch stream (line 108)
   - On `ctx.Done()`, the function returns (line 119) but the stream.Recv() goroutine may still be blocked
   - gRPC stream cleanup via `defer conn.Close()` (line 83) happens AFTER the leak
   - **Fix:** Add `defer stream.CloseSend()` or wrap stream.Recv() in a goroutine with proper cancellation
   - **Impact:** Memory/goroutine leak on relay shutdown

3. **Task Manager: Silent Failure on Catalog Push** (`internal/task/manager.go:271-279`)
   - `pushCatalogForEntity()` is called in a goroutine (line 132)
   - Errors are only logged (line 275), never returned or retried
   - If the catalog write fails, the entity remains in StateIntercept but has NO task catalog in the store
   - **Fix:** Either block on catalog write in `Approve()` or add retry logic with failure callback
   - **Impact:** Operator approves HIGH threat but tasks never propagate to operators

### Significant

4. **HLC Logical Counter Overflow** (`internal/hlc/hlc.go:60-78`)
   - `lastLogical` is `uint32`, incremented on every same-physical-clock call
   - At 1M calls/sec with same physical time, overflows in ~4000 seconds
   - On overflow, wraps to 0 (line 70) and breaks monotonicity
   - **Mitigation:** Current code resets logical to 0 when physical advances (line 68), so overflow requires sustained nanosecond-level clock stalling
   - **Fix:** Add overflow check: `if c.lastLogical == math.MaxUint32 { panic("HLC logical overflow") }`
   - **Impact:** LOW (requires extreme conditions) but violates total ordering guarantee

5. **Relay CRDT Merge: CreatedAt Clobbering** (`internal/mesh/relay.go:204-233`)
   - Line 220: `merged.CreatedAt = existing.CreatedAt`
   - If `existing` entity is older than `incoming` (by creation time), this is correct
   - But if entities were created on different nodes with different CreatedAt timestamps, the merge picks existing's timestamp arbitrarily
   - **Impact:** CreatedAt no longer reflects true creation time across partitions
   - **Fix:** Use min(existing.CreatedAt, incoming.CreatedAt) to preserve earliest creation time

6. **Token Bucket Refill Race** (`internal/mesh/budget.go:42-64`)
   - Lines 50-55: Token refill calculation uses `time.Now()` inside mutex
   - If system clock jumps backward (NTP correction), `elapsed` becomes negative
   - Tokens can underflow (line 52: `tb.tokens += negative`) and become huge after uint->float conversion
   - **Fix:** Add `if elapsed < 0 { elapsed = 0 }` guard on line 51
   - **Impact:** Clock skew can grant infinite bandwidth during backward jumps

7. **Partition Test: Relay Restart Without Connection Draining** (`internal/mesh/partition_test.go:416-455`)
   - Lines 435-455: After healing partition, ALL relays are restarted (including non-partitioned nodes)
   - Old relay contexts are cancelled (line 437) but new relays immediately dial (line 446)
   - No wait for old TCP connections to fully close (no TIME_WAIT buffer)
   - **Impact:** Flaky test — can bind to same port before old connection releases it
   - **Fix:** Add `time.Sleep(100*time.Millisecond)` after cancel loop (line 439) before restart

### Minor

8. **Store Watcher Channel Drop on Slow Consumer** (`internal/store/store.go:250-264`)
   - Line 259-262: Events are dropped silently on full channel (default case)
   - No metric/log for dropped events
   - **Fix:** Add `slog.Warn("dropped event", "entity_id", event.Entity.Id)` in default case
   - **Impact:** Silent data loss for slow watchers (e.g., task-manager, classifier)

9. **Classifier Missing ComponentUpdates Test** (`internal/classifier/classifier_test.go` not shown)
   - CRDT merge can update classification component from remote node
   - No test verifying classifier doesn't re-classify on CRDT merge (would cause classification ping-pong)
   - **Fix:** Add test: create entity on node A, classify, replicate to node B, CRDT merge back to A, verify no re-classification
   - **Impact:** Potential classification thrashing in mesh

10. **Fusion: Fused Entity ID Collision** (`internal/fusion/fusion.go:105-108`)
    - Fused ID format: `fused-{trackA}-{trackB}` (line 108)
    - If trackA or trackB contains hyphens, ID parsing becomes ambiguous
    - **Fix:** Use deterministic hash instead: `fused-{sha256(trackA+trackB)[:16]}`
    - **Impact:** LOW (track IDs are controlled), but brittle

11. **Proto Backward Compatibility: Missing Field Annotations**
    - `entity.proto` line 6-8: Added HLC fields (hlc_physical, hlc_logical, hlc_node) without `reserved` tags for old field numbers
    - If old clients don't have these fields, they send zero values
    - Zero HLC (physical=0, logical=0, node="") is a valid timestamp and will be treated as "oldest"
    - **Impact:** Old clients can't participate in CRDT merge correctly (their updates appear ancient)
    - **Mitigation:** HLC zero-check in `crdt.MergeEntity()` to treat zero HLC as "use entity-level timestamp fallback"

12. **Task Manager: Approval Timeout Race** (`internal/task/manager.go:303-316`)
    - Line 307: Timer fires, checks `m.pending[entityID]` under lock
    - If operator calls `Approve()` concurrently, `p.cancel()` (line 117) stops the timer
    - But if timer fires RIGHT before cancel, both goroutines compete for the lock
    - **Impact:** LOW (timeout is 30s, approval is milliseconds) but can cause double-logging
    - **Fix:** Use `select` with `ctx.Done()` in timer goroutine (line 305) instead of `time.After`

## Recommendation

**Immediate Actions:**
1. Fix Store.Update() CRDT race by replacing component-level merge with `crdt.MergeEntity()` call
2. Add relay stream cleanup: `defer func() { stream.CloseSend() }()` after line 108 in relay.go
3. Make task manager catalog push synchronous OR add explicit failure state for entities with missing catalogs

**Before Merge:**
4. Add HLC logical overflow check (panic or saturate at MaxUint32)
5. Fix token bucket negative elapsed guard
6. Add test coverage for classifier CRDT merge behavior

**Follow-up:**
7. Instrument watcher drop events with metrics
8. Add proto migration guide for HLC field backward compatibility
9. Consider replacing fused entity ID format with content-addressed hash

**Positive Notes:**
- HLC monotonicity logic is mathematically correct (lines 59-123 in hlc.go)
- CRDT commutativity/idempotency tests are excellent (merge_test.go)
- Partition tests are comprehensive (Jepsen-style validation)
- Concurrency test coverage is strong (hlc_test.go:118-144)
- Error handling in relay is defensive (codes.AlreadyExists, codes.NotFound checks)
