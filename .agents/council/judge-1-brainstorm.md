# Judge 1 Brainstorm: lattice-lab Enhancements for Anduril Interview Prep

## Key Insight

The highest-leverage enhancements add causal ordering, conflict-free merge, and sensor fusion -- these are the three hardest distributed systems problems Anduril faces at the edge, and the current codebase has clean seams to slot them in.

---

## Enhancements (Ordered by Priority)

### 1. Hybrid Logical Clocks (HLC)
- **Teaches:** Causal ordering, vector clocks, Lamport timestamps
- **Complexity:** M
- **Priority:** 1

Add HLC timestamps to every entity mutation so causally-ordered merge is possible across partitioned mesh peers. Each store node maintains its own HLC; on mesh relay forward, the receiver advances its clock to `max(local, remote)+1`. This replaces the current `timestamppb.Now()` wall-clock stamps with causally meaningful ones, enabling last-writer-wins merge and conflict detection.

**Where:** `internal/store/store.go` (Entity timestamps), `internal/mesh/relay.go` (forwarding), `proto/entity/v1/entity.proto` (HLC field)

**K8s analogy:** ResourceVersion on every K8s object -- the optimistic concurrency field that etcd uses to reject stale writes.

**Why first:** It is a prerequisite for CRDT merge and partition tolerance. Small scope, foundational impact.

---

### 2. CRDT Component Merge
- **Teaches:** CRDTs, eventual consistency, conflict resolution strategies
- **Complexity:** L
- **Priority:** 1

Implement conflict-free merge for entity components using operation-based CRDTs. Position/velocity use last-writer-wins register (keyed on HLC), classification uses max-threat-wins, task catalog uses OR-set (add wins over remove). The mesh relay becomes a CRDT replication layer: on receive, it merges component-by-component rather than wholesale overwrite.

Add a partition simulation mode that splits peers, lets them diverge, then reconnects and verifies convergence. Property-based tests should prove that merge is commutative, associative, and idempotent.

**Where:** `internal/mesh/relay.go` (merge logic), new `internal/crdt/` package, `proto/entity/v1/entity.proto` (version vectors per component)

**K8s analogy:** Server-side apply's field-manager merge -- each component owner merges independently, conflicts resolved by policy.

**Why critical:** This is the single most interview-relevant enhancement. Anduril's Lattice must merge entity state from disconnected edge nodes without coordination. Being able to whiteboard CRDT merge semantics with working code behind it is a massive differentiator.

---

### 3. Sensor Fusion / Track Correlation
- **Teaches:** Track correlation, sensor fusion, measurement uncertainty, data association
- **Complexity:** L
- **Priority:** 1

Add a second sensor-sim instance producing overlapping tracks with different IDs, noise, and update rates. Build a fusion service that correlates tracks from multiple sensors into a single fused entity using nearest-neighbor gating on position/velocity. Fused tracks get a new `FusionComponent` listing contributing sensor track IDs and a covariance estimate.

Start simple (Euclidean distance gating on lat/lon), then optionally add a Kalman filter for position prediction between updates.

**Where:** New `internal/fusion/` package, new `cmd/fusion/` binary, extends `proto/entity/v1/entity.proto`

**K8s analogy:** Endpoint slices merging pod IPs from multiple node sources into a single Service abstraction.

**Why critical:** Sensor fusion is core Lattice functionality. Demonstrating that you understand the data association problem (which sensor reports map to which real-world object?) is extremely high signal.

---

### 4. Human-on-the-Loop Approval Gate
- **Teaches:** Human-in-the-loop control systems, state machine design, approval workflows
- **Complexity:** M
- **Priority:** 2

When threat escalates to HIGH and task state transitions to "intercept", the task enters a "pending_approval" state. An operator must approve via `lattice-cli` before the intercept tasks activate. Implement a timeout-based auto-escalation if no human responds within N seconds.

**Where:** `internal/task/manager.go` (state machine), `cmd/lattice-cli/` (approve/reject commands), `proto/entity/v1/entity.proto` (ApprovalComponent)

**K8s analogy:** Admission webhooks -- a validating webhook that gates resource creation on external approval.

**Why important:** Directly maps to Anduril's autonomy philosophy. Their public messaging emphasizes "human-on-the-loop" -- autonomous systems that act but require human authorization for lethal decisions.

---

### 5. Partition Tolerance with WAL and Anti-Entropy
- **Teaches:** Write-ahead logging, anti-entropy, Merkle trees, partition recovery
- **Complexity:** L
- **Priority:** 2

Add a local write-ahead log (WAL) to each store node so that when mesh peers are unreachable, mutations queue locally and replay on reconnect. Implement anti-entropy: on reconnection, peers exchange entity version summaries (Merkle tree or version vector digest) and sync only the delta. Add a chaos mode to mesh-relay that randomly drops connections.

**Where:** `internal/mesh/relay.go`, new `internal/wal/` and `internal/antientropy/` packages

**K8s analogy:** Kubelet's standalone mode -- when the node loses API server connectivity, it keeps running pods from its local checkpoint and reconciles when connectivity returns.

---

### 6. Distributed Tracing and Observability
- **Teaches:** Distributed tracing, context propagation, metrics, SLI/SLO thinking
- **Complexity:** M
- **Priority:** 3

Instrument the full entity lifecycle with OpenTelemetry traces: sensor-sim creates a span on entity creation, classifier adds a child span, task-manager adds a child span. Propagate trace context through gRPC metadata. Add Prometheus metrics: entity count by type, classification latency p99, mesh replication lag.

**Where:** All `cmd/` binaries, new `internal/observability/` package

**K8s analogy:** kube-apiserver audit logging and the tracing KEP -- every request gets a trace ID flowing through etcd, admission, and controllers.

---

### 7. Behavior Tree Engine (Stretch)
- **Teaches:** Behavior trees, declarative policy engines, composable decision logic
- **Complexity:** L
- **Priority:** 3

Replace the static `Rules()` function in task-manager with a behavior tree evaluator. Define trees in YAML: sequence nodes, selector nodes, condition checks (threat level, speed, proximity to geo-fence), action leaves (assign task, escalate, notify).

**Where:** `internal/task/manager.go`, new `internal/behaviortree/` package

**K8s analogy:** OPA/Gatekeeper -- declarative rules evaluated against resource state, composable and hot-reloadable.

---

### 8. Resource-Constrained Edge Mode (Stretch)
- **Teaches:** Resource-constrained systems, eviction policies, delta compression, bandwidth optimization
- **Complexity:** M
- **Priority:** 3

Add a memory budget to the entity store with priority-based eviction (NONE threat first, then by staleness). Add bandwidth throttling to mesh-relay: batch updates into periodic sync windows. Add entity delta compression -- only send changed components.

**Where:** `internal/store/store.go`, `internal/mesh/relay.go`, `proto/store/v1/store.proto`

**K8s analogy:** Kubelet eviction manager -- when node resources are exhausted, pods evicted by priority class.

---

## Recommended Build Order

```
Phase 1 (Foundation):  HLC Timestamps
Phase 2 (Crown Jewel): CRDT Component Merge
Phase 3 (Domain):      Sensor Fusion / Track Correlation
Phase 4 (Quick Win):   Human-on-the-Loop Approval Gate
Phase 5 (Resilience):  Partition Tolerance with WAL
Phase 6 (Polish):      Distributed Tracing
Phase 7-8 (Stretch):   Behavior Trees, Edge Mode
```

The first three alone would make an extremely strong interview artifact. They demonstrate:
- Deep understanding of distributed systems theory (HLC, CRDTs)
- Domain-specific knowledge of the sensor fusion problem
- Ability to build working systems, not just talk about concepts

Each enhancement builds on the previous: HLC enables CRDT merge, CRDT merge enables partition-tolerant fusion, and the approval gate shows you understand the human factors that constrain autonomous systems.
