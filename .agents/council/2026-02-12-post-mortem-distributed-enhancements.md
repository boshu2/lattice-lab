# Post-Mortem: Distributed Systems Enhancements

**Date:** 2026-02-12
**Epic:** ll-4ii (CLOSED)
**Goal:** Enhance lattice-lab with 6 production-grade distributed systems features for Anduril Lattice interview prep

## Results

| Metric | Value |
|--------|-------|
| Issues | 9/9 closed |
| Test packages | 9 passing |
| New code | ~2,400 lines (30 files) |
| New packages | 4 (hlc, crdt, fusion, mesh/budget) |
| Pre-mortem | WARN → all amendments incorporated |
| Vibe | WARN (architecture PASS, correctness WARN) |

## What Went Well

1. **Pre-mortem caught 3 design flaws before implementation** — per-component-key HLC (not per-entity), LWW-Element-Map (not ORSet), origin-node echo suppression. Without pre-mortem, Wave 1 would have stalled on the HLC stale-write problem and Wave 2 would have been 500+ lines of unnecessary ORSet code.

2. **4-wave parallelism worked** — each wave ran 2-3 agents in parallel. Total implementation was faster than serial execution would have been.

3. **Test-driven approach produced high confidence** — 9 test packages, integration tests with real gRPC servers, CRDT property verification (commutativity, idempotency), Jepsen-style partition tests.

4. **Functional options pattern avoided breaking changes** — Store.New() migration was seamless, no call site changes needed.

5. **Plan decomposition matched implementation** — issues were right-sized, dependencies were accurate, waves executed cleanly.

## What Could Improve

1. **Vibe found production-hardening gaps** — HLC overflow, token bucket clock skew, watcher drop instrumentation. These should have been caught in pre-mortem or implementation.

2. **No end-to-end integration test** — each package is tested in isolation, but no test runs the full pipeline (sensor-sim → store → classifier → task-manager → mesh relay → fusion).

3. **Proto schema changes ripple widely** — `make proto` regenerates all gen/ files. A proto change in Wave 1 forced recompilation across all packages for subsequent waves.

## Learnings

### Process
- Pre-mortem with 2 judges (feasibility + missing-requirements) is the right level for implementation plans
- WARN verdict with specific amendments is the ideal pre-mortem outcome — forces design decisions upfront
- Parallel wave execution with per-wave agents maximizes throughput
- Beads dependency direction matters: `bd dep add <dependent> <dependency>` (not the reverse)

### Technical
- Per-component-key HLC is the correct granularity for ECS architectures with multiple writers
- LWW-Element-Map is almost always better than ORSet for named-key maps
- Echo suppression is mandatory in bidirectional mesh topologies — not an edge case
- Functional options in Go prevent breaking API changes without sacrificing explicitness
- Listener-level partition (refuse connections) is simpler and more reliable than TCP proxy injection
- CRDT merge must be a pure function for testability — pass entities in, get merged entity out

### Interview Talking Points
- HLC provides total ordering without synchronized clocks
- CRDT merge enables convergence without coordination (AP in CAP)
- Per-component-key merge prevents false conflicts from independent services
- Max-wins for safety-critical fields (threat level) is a deliberate design choice
- Jepsen-style testing validates convergence after network partition heal
- Human-on-the-loop approval gate balances autonomy with oversight

## Follow-Up Work

| # | Title | Type | Severity |
|---|-------|------|----------|
| 1 | Add end-to-end integration test | tech-debt | high |
| 2 | HLC clock skew detection and logging | tech-debt | medium |
| 3 | Token bucket negative elapsed guard | tech-debt | medium |
| 4 | Watcher drop event instrumentation | tech-debt | low |
| 5 | Fusion spatial indexing for scale | tech-debt | low |
| 6 | CLAUDE.md update with new packages/features | process | medium |
