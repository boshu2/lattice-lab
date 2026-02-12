# Phase 4 Summary: Crank (Implementation)

**Epic:** ll-4ii (CLOSED)
**Issues:** 9/9 closed
**Test packages:** 9 passing, 0 failing

## Wave Execution

| Wave | Issues | Status |
|------|--------|--------|
| Wave 1 | HLC Package, Proto Schema | Done |
| Wave 2 | Store HLC, CRDT Merge, Approval Gate | Done |
| Wave 3 | Mesh Relay CRDT, Sensor Fusion | Done |
| Wave 4 | Bandwidth Budgeting, Partition Tests | Done |

## New Packages
- `internal/hlc/` — Hybrid Logical Clocks (9 tests)
- `internal/crdt/` — LWW-Element-Map + MaxWins merge (8 tests)
- `internal/fusion/` — Sensor fusion with cascade delete (9 tests)
- `internal/mesh/budget.go` — Token bucket + coalescer (9 tests)
- `internal/mesh/partition_test.go` — Jepsen-style partition tests (3 tests)

## Key Design Decisions (from pre-mortem)
- Per-component-key HLC (not per-entity) — supports multi-service concurrent writes
- LWW-Element-Map (not ORSet) — correct for named register map, 3x less code
- Origin-node echo suppression — prevents infinite relay loops
- Functional options for Store.New() — backward-compatible API change
- Listener-level partition — ~50 lines vs 200+ TCP proxy
