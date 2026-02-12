# Judge 2 Brainstorm: lattice-lab Enhancements for Anduril Interview Prep

## Key Insight

The current mesh-relay's last-write-wins semantics is the single biggest gap. Replacing it with CRDTs and causal ordering transforms this from a CRUD demo into a genuine distributed systems portfolio piece that maps directly to Lattice's core replication problem.

---

## Enhancement Proposals (Priority Order)

### 1. Hybrid Logical Clocks + Component-Level CRDTs
**Complexity: M | Priority: 1**

**What it teaches:** Conflict-free merge after network partitions, causal ordering, why wall clocks fail in distributed systems.

**Description:** Replace the last-write-wins `Store.Update()` with per-component versioning using Hybrid Logical Clocks. Each component gets an HLC timestamp; on merge, the store keeps the component with the higher HLC. This is ~150 lines of new code but demonstrates the single most important concept for Lattice engineering.

**Where:** `internal/store/store.go` (Update method), `internal/mesh/relay.go`, `proto/entity/v1/entity.proto`, new `internal/hlc/` package.

**What to build:**
- `internal/hlc` package: HLC struct with `Now()`, `Update(remote)`, `Compare()` methods
- Add `map<string, uint64> version_vector` to Entity proto (key = node ID)
- Modify `Store.Update` to compare HLC per component key and keep the winner
- Modify mesh relay to propagate version vectors
- Tests: simulate concurrent updates from two stores, verify deterministic merge

**K8s analogy:** Like ResourceVersion + conflict detection on kube-apiserver, except here you must MERGE rather than just reject with 409 Conflict.

**Interview question it answers:** *"How would you handle two sensors updating the same entity while network-partitioned, then reconciling when connectivity returns?"*

---

### 2. Partition Tolerance Integration Test Harness
**Complexity: M | Priority: 2**

**What it teaches:** Testing distributed systems under failure, convergence verification, Jepsen-style thinking.

**Description:** Build a test harness that programmatically introduces network partitions between mesh-relay peers, runs both sides independently, heals the partition, and verifies convergence. More valuable than any feature because it proves your system actually works under failure. The current test suite is entirely happy-path.

**Where:** New `test/partition_test.go` or `internal/mesh/relay_partition_test.go`.

**What to build:**
- Controllable TCP proxy (Go `net.Conn` wrapper) that can drop/delay packets on command
- Test: spin up 3 entity-stores with mesh-relays
- Create entities on store-A, partition store-B from A and C
- Create conflicting updates on both sides of the partition
- Heal partition, assert all three stores converge within timeout
- Check invariant: no entity lost, all components present, deterministic winner for conflicts

**K8s analogy:** Like Jepsen tests against etcd, or K8s integration tests with fake clocks and injected failures.

**Interview question it answers:** *"How would you test that your distributed system converges after a network partition? What invariants would you check?"*

---

### 3. Human-on-the-Loop Approval Gate
**Complexity: S | Priority: 3**

**What it teaches:** Autonomy levels, human-machine teaming, safety constraints in C2 systems.

**Description:** When threat escalates to HIGH, instead of immediately assigning intercept tasks, transition to `pending_approval` and expose `ApproveAction` / `DenyAction` RPCs. Include a configurable timeout that auto-denies if no human responds. Small code change, enormous Anduril-specific interview signal.

**Where:** `internal/task/manager.go`, `proto/store/v1/store.proto` (new RPCs), `cmd/lattice-cli/`.

**What to build:**
- `ApprovalState` enum: `auto_approved`, `pending`, `approved`, `denied`, `timed_out`
- For `THREAT_LEVEL_HIGH`: set state to pending, start countdown timer
- New RPCs: `ApproveAction(entity_id)`, `DenyAction(entity_id)`
- CLI: `lattice approve <entity-id>` command
- Tests: timeout path, race between approval and threat de-escalation

**K8s analogy:** Like an admission webhook that can block a resource before persistence, or a PodDisruptionBudget gating automated eviction.

**Interview question it answers:** *"How do you design an autonomous system that keeps a human in the loop for high-consequence decisions, especially under time pressure?"*

---

### 4. Edge-Degraded Mode with Bandwidth Budgeting
**Complexity: M | Priority: 3**

**What it teaches:** Resource-constrained edge computing, priority queuing, graceful degradation.

**Description:** Add a configurable bandwidth budget to the mesh relay. When over budget, implement priority-based queuing: HIGH threat updates go first, position updates are decimated, classification updates are batched. Demonstrates understanding of real tactical edge constraints (satellite links at 64kbps).

**Where:** `internal/mesh/relay.go`, new `internal/mesh/budget.go`.

**What to build:**
- Priority queue between local event receipt and peer forwarding
- Priority assignment: threat level + event type (deletes > creates > position updates)
- Token bucket rate limiter parameterized by bytes/sec
- Event coalescing: if same entity has 3 pending position updates, send only the latest
- Metrics: queue depth, drop rate, bytes sent per priority level
- Test: set 1KB/s budget, verify HIGH threat entities always get through

**K8s analogy:** Like resource quotas + priority classes for pod scheduling, applied to network bandwidth.

**Interview question it answers:** *"You have a 64kbps satellite link connecting edge nodes. How do you ensure the most critical data gets through first?"*

---

### 5. Multi-Sensor Track Correlation / Fusion
**Complexity: L | Priority: 2**

**What it teaches:** Sensor fusion, track association, observation-vs-identity modeling.

**Description:** Add a second sensor type (radar-sim with noisy position-only data) and build a fusion service that correlates observations from multiple sensors into single fused track entities. Use nearest-neighbor gating for association and weighted averaging for state estimation. Hardest thing to learn on the job.

**Where:** New `cmd/radar-sim/`, new `internal/fusion/`, touches `internal/store/` for correlation index.

**What to build:**
- `cmd/radar-sim`: produces noisier position-only observations with a `source` component
- `internal/fusion`: watches all tracks, maintains correlation matrix (track-pair distances)
- Association gate: if position distance < threshold within time window, same physical object
- Merge correlated tracks into single fused entity with `FusionComponent` (source_ids, fused_position, confidence)
- Handle de-correlation when tracks diverge

**K8s analogy:** Like having multiple controllers that can create the same resource, needing owner-references to deduplicate.

**Interview question it answers:** *"How would you design a system where multiple sensors produce observations of the same physical object, and you need to maintain a single fused track?"*

---

### 6. Distributed Tracing with OpenTelemetry
**Complexity: S | Priority: 4**

**What it teaches:** Observability in distributed pipelines, context propagation via gRPC metadata, correlation IDs.

**Description:** Instrument the full entity lifecycle (sensor -> store -> classifier -> task-manager) with OpenTelemetry trace context propagation through gRPC metadata. Not just observability theater -- the trace_id propagation pattern is directly applicable to Lattice.

**Where:** All `cmd/` binaries, `internal/server/`, gRPC interceptors, new `internal/telemetry/` package.

**What to build:**
- `internal/telemetry`: tracer initialization (OTLP exporter to stdout or Jaeger)
- gRPC unary and stream interceptors for automatic trace propagation
- Instrument `Store.Create`/`Update` to start spans
- Opt-in via `OTEL_ENABLED` env var
- Sample trace screenshot in README

**K8s analogy:** Tracing a request through API server -> etcd -> controller -> scheduler -> kubelet.

**Interview question it answers:** *"How do you debug latency or correctness issues in a distributed pipeline where data flows through multiple services asynchronously?"*

---

### 7. Gossip-Based Peer Discovery + Anti-Entropy
**Complexity: L | Priority: 4**

**What it teaches:** Gossip protocols, Merkle trees for set reconciliation, eventual consistency repair.

**Description:** Replace the static peer list with gossip-based discovery (SWIM-lite), and add anti-entropy background sync using Merkle tree comparison to detect and repair divergence without retransmitting everything. Fixes the fundamental flaw that the current relay only forwards new events -- missed events create permanent inconsistency.

**Where:** `internal/mesh/relay.go`, new `internal/mesh/gossip.go`, new `internal/mesh/antientropy.go`.

**What to build:**
- Merkle tree over entity IDs + versions
- Periodic tree root exchange between peers
- Walk tree to find divergent subtrees, sync only those entities
- UDP gossip for peer list exchange
- Pairs perfectly with HLC since version comparison becomes trivial

**K8s analogy:** Like etcd member discovery + the watch-cache's list-then-watch pattern for catching up on missed events.

**Interview question it answers:** *"Your mesh relay missed some events during a network blip. How do you detect and repair the inconsistency without re-transmitting everything?"*

---

## Recommended Build Order

```
Phase 1 (Foundation):   HLC + CRDTs  -->  Partition Tests
Phase 2 (Anduril-specific signal):   Approval Gate  -->  Bandwidth Budgeting
Phase 3 (Deep domain):   Sensor Fusion  (skip if time-constrained)
Sprinkle throughout:   OTel Tracing
Phase 4 (If time allows):   Gossip + Anti-Entropy
```

**The first four enhancements alone (HLC, partition tests, approval gate, bandwidth budgeting) would make a very strong portfolio piece.** They demonstrate distributed systems depth (not just breadth), practical tradeoff reasoning, and Anduril-specific domain awareness -- all in Go, all testable, all buildable incrementally on the existing ~1,500 line codebase.

## What Makes This Stand Out vs. Generic Distributed Systems Projects

Most candidates would build a Raft implementation or a key-value store. This project stands out because:

1. **Domain-specific**: It speaks the language of C2 (tracks, threats, tasks, sensors) rather than generic KV operations
2. **ECS architecture**: Shows understanding of Lattice's actual data model (entities as bags of typed components)
3. **Merge semantics over consensus**: Lattice likely favors CRDTs and merge over strong consensus because edge nodes cannot afford synchronous coordination -- demonstrating this understanding is a strong signal
4. **The full pipeline**: sensor -> store -> classify -> decide -> act is the actual Lattice data flow; most portfolio projects only implement storage
5. **Testing under failure**: Partition tests are what separate "I read the CRDT paper" from "I can ship distributed systems"
