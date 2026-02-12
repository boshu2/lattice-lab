# Phase 1 Summary: Research

**Goal:** Enhance lattice-lab with production-grade distributed systems patterns for Anduril Lattice interview prep. Test-driven. All Go, no Rust.

**Research Artifacts:**
- `.agents/research/2026-02-12-codebase-walkthrough.md` — Full pseudocode walkthrough of all Go files with K8s analogies
- `.agents/council/2026-02-12-brainstorm-lattice-enhancements.md` — Council brainstorm (2 judges, strong consensus)

**Current System:** ~1,500 lines of Go. Entity store + gRPC watch + sensor-sim + classifier + task-manager + mesh relay + CLI. ECS architecture with protobuf Any components.

**Council Consensus — 6 Enhancements (priority order):**

1. **HLC Timestamps** (M) — Hybrid Logical Clocks for causal ordering. Replace `timestamppb.Now()` with HLC. New `internal/hlc/` package. Prerequisite for CRDTs.
2. **CRDT Component Merge** (L) — Per-component merge semantics in mesh relay. LWW-Register for position/velocity (keyed on HLC), max-wins for threat, OR-set for task catalog. New `internal/crdt/` package.
3. **Partition Test Harness** (M) — Jepsen-style tests with programmable TCP proxy. Spin up 3 stores, partition, diverge, heal, verify convergence.
4. **Human-on-the-Loop Approval Gate** (S) — `pending_approval` state for HIGH threat. Approve/deny RPCs. Timeout auto-deny. CLI commands.
5. **Edge Bandwidth Budgeting** (M) — Priority queue in mesh relay. Token bucket rate limiter. HIGH threat always gets through. Event coalescing.
6. **Sensor Fusion / Track Correlation** (L) — Second sensor type, fusion service, nearest-neighbor gating, fused track entities.

**Approach:** Test-driven development. Write tests first for each enhancement.
