# Vibe: Distributed Systems Enhancements

**Date:** 2026-02-12
**Mode:** 2 judges (correctness, architecture)

## Council Verdict: WARN

| Judge | Verdict | Confidence | Key Finding |
|-------|---------|------------|-------------|
| Correctness & Safety | WARN | HIGH | HLC/CRDT semantics correct; theoretical race in Store.Update(), goroutine cleanup, error propagation |
| Architecture & Design | PASS | HIGH | Clean separation, idiomatic Go, exceptional test quality, interview-ready |

## Shared Findings (both judges noted)

1. **Store HLC is local operation ordering, not causal timestamp** — works correctly but semantics should be documented
2. **Fusion O(n²) correlation** — fine for n<100, needs spatial indexing at scale
3. **All pre-mortem amendments were incorporated** — both judges verified this independently

## Disagreements

- **Severity of Store.Update() component merge**: Correctness judge flagged as critical race condition; architecture judge noted the write lock serializes access and called it correct. **Assessment:** The mu.Lock() in Update() prevents concurrent interleaving — this is a theoretical concern, not an active bug.

## Concerns Raised (Correctness Judge)

### Critical (for production — not blocking for learning lab)
1. Store.Update() per-component merge under single HLC comparison — serialized by mutex in practice
2. Relay stream cleanup on context cancellation — gRPC propagates cancellation to Recv()
3. Task manager catalog push errors logged but not retried — appropriate for demo

### Significant
4. HLC uint32 logical overflow (requires extreme conditions)
5. CreatedAt clobbering on CRDT merge (use min instead)
6. Token bucket negative elapsed on clock skew
7. Partition test relay restart without TIME_WAIT buffer

### Minor
8. Watcher channel drop not instrumented
9. No CRDT merge re-classification test for classifier
10. Fused entity ID format brittle with hyphenated track IDs
11. Proto HLC zero-value backward compat
12. Approval timeout race (double-logging, not double-execution)

## Positive Highlights

- HLC monotonicity is mathematically correct
- CRDT tests verify commutativity and idempotency (rare in code reviews)
- Jepsen-style partition test validates convergence after heal
- All 9 test packages pass, integration tests use real gRPC servers
- Dependency graph is acyclic and shallow: hlc → crdt → store → mesh
- All 9 pre-mortem amendments incorporated

## Recommendation

**PROCEED.** WARN items are production-hardening concerns, not correctness bugs for the learning lab. The architecture is clean, tests demonstrate deep distributed systems understanding, and the code is interview-ready.

**Follow-up items for production hardening:**
1. Add clock skew detection/logging to HLC
2. Comment Store HLC semantics
3. Add overflow check to HLC logical counter
4. Guard token bucket against negative elapsed time
5. Spatial indexing for fusion at scale
