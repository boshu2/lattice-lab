# Council Report: lattice-lab Enhancements for Anduril Interview Prep

**Date:** 2026-02-12
**Mode:** brainstorm (2 judges, runtime-native)
**Target:** Enhance lattice-lab with production-grade distributed systems patterns

## Council Consensus

**Strong agreement** across both judges. The enhancement list and ordering converged independently.

### Shared Findings

Both judges independently identified the same core insight: **the mesh relay's last-write-wins semantics is the single biggest gap**, and replacing it with HLC + CRDTs transforms this from a CRUD demo into a genuine distributed systems portfolio piece.

### Consolidated Enhancement List

| # | Enhancement | Complexity | Priority | Judge 1 | Judge 2 |
|---|------------|-----------|----------|---------|---------|
| 1 | HLC Timestamps | M | 1 | Yes (separate) | Yes (combined with CRDTs) |
| 2 | CRDT Component Merge | L | 1 | Yes | Yes |
| 3 | Partition Tolerance Test Harness | M | 1 | Folded into CRDTs | Yes (separate, high priority) |
| 4 | Human-on-the-Loop Approval Gate | S | 2 | Yes | Yes |
| 5 | Sensor Fusion / Track Correlation | L | 2 | Yes | Yes |
| 6 | Edge Bandwidth Budgeting | M | 3 | Stretch | Yes (Priority 3) |
| 7 | OpenTelemetry Tracing | S-M | 3 | Yes | Yes |
| 8 | Behavior Tree Engine | L | 4 | Yes | No |
| 9 | Gossip + Anti-Entropy | L | 4 | WAL-based | Merkle-based |

### Disagreements

| Topic | Judge 1 | Judge 2 | Resolution |
|-------|---------|---------|------------|
| CRDTs scope | Separate HLC then CRDT phases | Combined into one phase | **Split**: HLC is small enough to be its own deliverable, validates the foundation before building merge on top |
| Partition tests | Part of CRDT enhancement | Standalone enhancement | **Standalone**: Judge 2 is right — partition tests are their own interview talking point and prove the system works |
| Bandwidth budgeting | Stretch goal | Phase 2 priority | **Include**: Small-medium effort, very Anduril-specific (tactical links), strong interview signal |
| Behavior trees | Phase 7 stretch | Not proposed | **Defer**: Both judges agree core distributed systems > fancy decision engines |

### Recommended Build Order

```
Phase 1: HLC Timestamps                    (M, ~150 lines)  — foundation for everything
Phase 2: CRDT Component Merge              (L, ~300 lines)  — the crown jewel
Phase 3: Partition Test Harness            (M, ~200 lines)  — proves it works
Phase 4: Human-on-the-Loop Approval Gate   (S, ~100 lines)  — quick Anduril-specific win
Phase 5: Edge Bandwidth Budgeting          (M, ~200 lines)  — tactical edge signal
Phase 6: Sensor Fusion / Track Correlation (L, ~400 lines)  — deep domain knowledge
Phase 7: OpenTelemetry Tracing             (S, sprinkle)    — polish
Phase 8: Gossip + Anti-Entropy             (L, if time)     — stretch
```

**Minimum viable portfolio: Phases 1-4.** This gives you HLC + CRDTs + partition proof + human-on-the-loop, which covers the top interview topics with ~750 lines of new code.

**Full portfolio: Phases 1-6.** Adding bandwidth budgeting and sensor fusion makes this a standout project that demonstrates both distributed systems depth and C2 domain knowledge.

### Interview Questions Each Phase Answers

| Phase | Interview Question |
|-------|-------------------|
| 1 (HLC) | "Why can't you use wall clocks in a distributed system?" |
| 2 (CRDT) | "How do you merge state from two partitioned nodes without coordination?" |
| 3 (Partition Tests) | "How do you test distributed system correctness under failure?" |
| 4 (Approval Gate) | "How do you keep a human in the loop for high-consequence autonomous decisions?" |
| 5 (Bandwidth) | "You have a 64kbps satellite link. How do you prioritize what gets through?" |
| 6 (Fusion) | "Multiple sensors see the same object. How do you maintain one fused track?" |
