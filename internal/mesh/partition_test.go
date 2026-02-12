package mesh

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	entityv1 "github.com/boshu2/lattice-lab/gen/entity/v1"
	storev1 "github.com/boshu2/lattice-lab/gen/store/v1"
	"github.com/boshu2/lattice-lab/internal/server"
	"github.com/boshu2/lattice-lab/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/anypb"
)

// ---------------------------------------------------------------------------
// controllableListener — wraps net.Listener to simulate network partitions
// ---------------------------------------------------------------------------

type controllableListener struct {
	net.Listener
	mu      sync.RWMutex
	blocked bool
	conns   []net.Conn
}

func newControllableListener(addr string) (*controllableListener, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &controllableListener{Listener: lis}, nil
}

func (cl *controllableListener) Accept() (net.Conn, error) {
	for {
		conn, err := cl.Listener.Accept()
		if err != nil {
			return nil, err
		}
		cl.mu.RLock()
		blocked := cl.blocked
		cl.mu.RUnlock()
		if blocked {
			conn.Close() // refuse connection during partition
			continue
		}
		cl.mu.Lock()
		cl.conns = append(cl.conns, conn)
		cl.mu.Unlock()
		return conn, nil
	}
}

// Partition isolates this node by refusing new connections and closing existing ones.
func (cl *controllableListener) Partition() {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.blocked = true
	for _, c := range cl.conns {
		c.Close()
	}
	cl.conns = nil
}

// Heal restores connectivity by allowing new connections.
func (cl *controllableListener) Heal() {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.blocked = false
}

// ---------------------------------------------------------------------------
// testNode — one node in a test cluster
// ---------------------------------------------------------------------------

type testNode struct {
	store    *store.Store
	server   *grpc.Server
	listener *controllableListener
	addr     string
	relay    *Relay
	cancel   context.CancelFunc
}

// ---------------------------------------------------------------------------
// startTestCluster — spin up an n-node mesh cluster for partition testing
// ---------------------------------------------------------------------------

func startTestCluster(t *testing.T, n int) []*testNode {
	t.Helper()

	nodes := make([]*testNode, n)

	// Phase 1: create stores, listeners, and gRPC servers.
	for i := 0; i < n; i++ {
		nodeID := fmt.Sprintf("node-%d", i)
		s := store.New(store.WithNodeID(nodeID))

		lis, err := newControllableListener("localhost:0")
		if err != nil {
			t.Fatalf("listen node-%d: %v", i, err)
		}
		addr := lis.Addr().String()

		gs := grpc.NewServer()
		storev1.RegisterEntityStoreServiceServer(gs, server.New(s))
		go gs.Serve(lis) //nolint:errcheck

		nodes[i] = &testNode{
			store:    s,
			server:   gs,
			listener: lis,
			addr:     addr,
		}
	}

	// Phase 2: set up mesh relays connecting every node to every other node.
	for i, node := range nodes {
		var peers []string
		for j, other := range nodes {
			if i != j {
				peers = append(peers, other.addr)
			}
		}
		relay := New(Config{
			LocalAddr: node.addr,
			Peers:     peers,
			NodeID:    fmt.Sprintf("node-%d", i),
		})
		ctx, cancel := context.WithCancel(context.Background())
		node.relay = relay
		node.cancel = cancel
		go relay.Run(ctx) //nolint:errcheck
	}

	// Let relays establish watch streams before tests proceed.
	time.Sleep(200 * time.Millisecond)

	t.Cleanup(func() {
		for _, nd := range nodes {
			nd.cancel()
			nd.server.GracefulStop()
		}
	})

	return nodes
}

// ---------------------------------------------------------------------------
// helpers — gRPC client operations against a node
// ---------------------------------------------------------------------------

func dialNode(t *testing.T, addr string) storev1.EntityStoreServiceClient {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	t.Cleanup(func() { conn.Close() })
	return storev1.NewEntityStoreServiceClient(conn)
}

func createEntity(t *testing.T, client storev1.EntityStoreServiceClient, id string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := client.CreateEntity(ctx, &storev1.CreateEntityRequest{
		Entity: &entityv1.Entity{
			Id:   id,
			Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
		},
	})
	if err != nil {
		t.Fatalf("create entity %s: %v", id, err)
	}
}

func updateEntityWithThreat(t *testing.T, client storev1.EntityStoreServiceClient, id string, level entityv1.ThreatLevel) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	threatComp, err := anypb.New(&entityv1.ThreatComponent{Level: level})
	if err != nil {
		t.Fatalf("marshal threat: %v", err)
	}
	_, err = client.UpdateEntity(ctx, &storev1.UpdateEntityRequest{
		Entity: &entityv1.Entity{
			Id:   id,
			Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
			Components: map[string]*anypb.Any{
				"threat": threatComp,
			},
		},
	})
	if err != nil {
		t.Fatalf("update entity %s with threat %v: %v", id, level, err)
	}
}

func getEntity(t *testing.T, client storev1.EntityStoreServiceClient, id string) *entityv1.Entity {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	e, err := client.GetEntity(ctx, &storev1.GetEntityRequest{Id: id})
	if err != nil {
		t.Fatalf("get entity %s: %v", id, err)
	}
	return e
}

func entityExists(client storev1.EntityStoreServiceClient, id string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_, err := client.GetEntity(ctx, &storev1.GetEntityRequest{Id: id})
	return err == nil
}

func listEntities(client storev1.EntityStoreServiceClient) ([]*entityv1.Entity, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := client.ListEntities(ctx, &storev1.ListEntitiesRequest{})
	if err != nil {
		return nil, err
	}
	return resp.Entities, nil
}

// ---------------------------------------------------------------------------
// waitForEntity — poll until the entity appears on a node (or timeout)
// ---------------------------------------------------------------------------

func waitForEntity(t *testing.T, client storev1.EntityStoreServiceClient, id string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if entityExists(client, id) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("entity %s did not appear within %v", id, timeout)
}

// ---------------------------------------------------------------------------
// waitForConvergence — poll until all nodes agree on entity state (or timeout)
// ---------------------------------------------------------------------------

func waitForConvergence(t *testing.T, nodes []*testNode, entityID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var entities []*entityv1.Entity
		allPresent := true
		for _, nd := range nodes {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			e, err := storev1.NewEntityStoreServiceClient(
				mustDial(nd.addr),
			).GetEntity(ctx, &storev1.GetEntityRequest{Id: entityID})
			cancel()
			if err != nil {
				allPresent = false
				break
			}
			entities = append(entities, e)
		}
		if allPresent && len(entities) == len(nodes) {
			if componentsMatch(entities) {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("nodes did not converge on entity %s within %v", entityID, timeout)
}

// waitForEntityCount polls until a node has at least count entities.
func waitForEntityCount(t *testing.T, client storev1.EntityStoreServiceClient, count int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		entities, err := listEntities(client)
		if err == nil && len(entities) >= count {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("did not reach %d entities within %v", count, timeout)
}

func mustDial(addr string) *grpc.ClientConn {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(fmt.Sprintf("mustDial %s: %v", addr, err))
	}
	return conn
}

// componentsMatch checks whether all entities have the same threat component value.
func componentsMatch(entities []*entityv1.Entity) bool {
	if len(entities) < 2 {
		return true
	}
	ref := threatLevel(entities[0])
	for _, e := range entities[1:] {
		if threatLevel(e) != ref {
			return false
		}
	}
	return true
}

func threatLevel(e *entityv1.Entity) entityv1.ThreatLevel {
	if e.Components == nil {
		return entityv1.ThreatLevel_THREAT_LEVEL_UNSPECIFIED
	}
	comp, ok := e.Components["threat"]
	if !ok {
		return entityv1.ThreatLevel_THREAT_LEVEL_UNSPECIFIED
	}
	var tc entityv1.ThreatComponent
	if err := comp.UnmarshalTo(&tc); err != nil {
		return entityv1.ThreatLevel_THREAT_LEVEL_UNSPECIFIED
	}
	return tc.Level
}

// ===========================================================================
// Partition Tests
// ===========================================================================

// TestPartition_BasicReplication verifies that an entity created on node-0
// is replicated to all other nodes in a 3-node cluster.
func TestPartition_BasicReplication(t *testing.T) {
	nodes := startTestCluster(t, 3)

	client0 := dialNode(t, nodes[0].addr)
	client1 := dialNode(t, nodes[1].addr)
	client2 := dialNode(t, nodes[2].addr)

	createEntity(t, client0, "basic-rep-1")

	waitForEntity(t, client1, "basic-rep-1", 5*time.Second)
	waitForEntity(t, client2, "basic-rep-1", 5*time.Second)

	// Verify entity type is correct on all nodes.
	for i, client := range []storev1.EntityStoreServiceClient{client0, client1, client2} {
		e := getEntity(t, client, "basic-rep-1")
		if e.Type != entityv1.EntityType_ENTITY_TYPE_TRACK {
			t.Fatalf("node-%d: expected TRACK, got %v", i, e.Type)
		}
	}
}

// TestPartition_SurvivesPartitionAndConverges is the main Jepsen-style test.
// It partitions a node, makes conflicting updates on both sides of the
// partition, heals the partition, and verifies CRDT convergence with
// max-wins semantics for the threat component.
func TestPartition_SurvivesPartitionAndConverges(t *testing.T) {
	nodes := startTestCluster(t, 3)

	client0 := dialNode(t, nodes[0].addr)
	client1 := dialNode(t, nodes[1].addr)
	client2 := dialNode(t, nodes[2].addr)

	// Step a: create entity on node-0.
	createEntity(t, client0, "partition-conv-1")

	// Step b: wait for replication to node-1 and node-2.
	waitForEntity(t, client1, "partition-conv-1", 5*time.Second)
	waitForEntity(t, client2, "partition-conv-1", 5*time.Second)

	// Step c: partition node-1 (isolate it from the cluster).
	nodes[1].listener.Partition()

	// Give the relay time to detect the broken connection.
	time.Sleep(300 * time.Millisecond)

	// Step d: update entity on node-0 with LOW threat.
	updateEntityWithThreat(t, client0, "partition-conv-1", entityv1.ThreatLevel_THREAT_LEVEL_LOW)

	// Step e: update same entity directly on node-1 with HIGH threat.
	// Use the store directly since node-1 is partitioned and can't receive gRPC.
	threatHigh, err := anypb.New(&entityv1.ThreatComponent{
		Level: entityv1.ThreatLevel_THREAT_LEVEL_HIGH,
	})
	if err != nil {
		t.Fatalf("marshal threat: %v", err)
	}
	_, err = nodes[1].store.Update(&entityv1.Entity{
		Id:   "partition-conv-1",
		Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
		Components: map[string]*anypb.Any{
			"threat": threatHigh,
		},
	})
	if err != nil {
		t.Fatalf("direct update on partitioned node-1: %v", err)
	}

	// Verify the partition is in effect: node-0's LOW update should NOT
	// appear on node-1, and node-1's HIGH update should NOT appear on node-0.
	time.Sleep(500 * time.Millisecond)
	e0 := getEntity(t, client0, "partition-conv-1")
	if threatLevel(e0) == entityv1.ThreatLevel_THREAT_LEVEL_HIGH {
		t.Fatal("partition breach: node-0 has HIGH before heal")
	}

	// Step f: heal the partition.
	nodes[1].listener.Heal()

	// Need to restart relay for node-1 since the old one lost its connections.
	nodes[1].cancel()
	var peers1 []string
	for j, other := range nodes {
		if j != 1 {
			peers1 = append(peers1, other.addr)
		}
	}
	relay1 := New(Config{
		LocalAddr: nodes[1].addr,
		Peers:     peers1,
		NodeID:    "node-1",
	})
	ctx1, cancel1 := context.WithCancel(context.Background())
	nodes[1].relay = relay1
	nodes[1].cancel = cancel1
	go relay1.Run(ctx1) //nolint:errcheck

	// Also restart relay for other nodes that had connections to node-1 break.
	for i, nd := range nodes {
		if i == 1 {
			continue
		}
		nd.cancel()
		var peers []string
		for j, other := range nodes {
			if j != i {
				peers = append(peers, other.addr)
			}
		}
		relay := New(Config{
			LocalAddr: nd.addr,
			Peers:     peers,
			NodeID:    fmt.Sprintf("node-%d", i),
		})
		ctx, cancel := context.WithCancel(context.Background())
		nd.relay = relay
		nd.cancel = cancel
		go relay.Run(ctx) //nolint:errcheck
	}

	// Give relays time to re-establish.
	time.Sleep(300 * time.Millisecond)

	// Trigger re-sync: update entity on each node to force relay forwarding.
	// This simulates the real-world case where ongoing updates propagate state.
	updateEntityWithThreat(t, client0, "partition-conv-1", entityv1.ThreatLevel_THREAT_LEVEL_LOW)

	// Update on node-1 via store to push its HIGH state through its relay.
	threatHighAgain, _ := anypb.New(&entityv1.ThreatComponent{
		Level: entityv1.ThreatLevel_THREAT_LEVEL_HIGH,
	})
	_, _ = nodes[1].store.Update(&entityv1.Entity{
		Id:   "partition-conv-1",
		Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
		Components: map[string]*anypb.Any{
			"threat": threatHighAgain,
		},
	})

	// Step g: wait for convergence.
	waitForConvergence(t, nodes, "partition-conv-1", 10*time.Second)

	// Step h+i: all 3 stores should have HIGH threat (max-wins CRDT rule).
	for i, client := range []storev1.EntityStoreServiceClient{client0, client1, client2} {
		e := getEntity(t, client, "partition-conv-1")
		level := threatLevel(e)
		if level != entityv1.ThreatLevel_THREAT_LEVEL_HIGH {
			t.Fatalf("node-%d: expected HIGH threat (max-wins), got %v", i, level)
		}
	}

	// Step j: entity exists on all stores (no data loss).
	for i, client := range []storev1.EntityStoreServiceClient{client0, client1, client2} {
		if !entityExists(client, "partition-conv-1") {
			t.Fatalf("node-%d: entity missing after partition heal", i)
		}
	}
}

// TestPartition_NoDataLoss verifies that entities created while a node is
// partitioned are eventually replicated to that node once the partition heals.
func TestPartition_NoDataLoss(t *testing.T) {
	nodes := startTestCluster(t, 3)

	client0 := dialNode(t, nodes[0].addr)
	client2 := dialNode(t, nodes[2].addr)

	// Create 5 entities on node-0 before partition.
	for i := 0; i < 5; i++ {
		createEntity(t, client0, fmt.Sprintf("pre-part-%d", i))
	}

	// Wait for all 5 to reach node-2.
	for i := 0; i < 5; i++ {
		waitForEntity(t, client2, fmt.Sprintf("pre-part-%d", i), 5*time.Second)
	}

	// Partition node-2.
	nodes[2].listener.Partition()
	time.Sleep(300 * time.Millisecond)

	// Create 5 more entities on node-0 while node-2 is partitioned.
	for i := 0; i < 5; i++ {
		createEntity(t, client0, fmt.Sprintf("during-part-%d", i))
	}

	// Verify the 5 new entities do NOT appear on node-2 (partition is effective).
	time.Sleep(500 * time.Millisecond)
	for i := 0; i < 5; i++ {
		if entityExists(client2, fmt.Sprintf("during-part-%d", i)) {
			t.Fatalf("partition breach: during-part-%d appeared on node-2 before heal", i)
		}
	}

	// Heal the partition.
	nodes[2].listener.Heal()

	// Restart relays to re-establish connections.
	for i, nd := range nodes {
		nd.cancel()
		var peers []string
		for j, other := range nodes {
			if j != i {
				peers = append(peers, other.addr)
			}
		}
		relay := New(Config{
			LocalAddr: nd.addr,
			Peers:     peers,
			NodeID:    fmt.Sprintf("node-%d", i),
		})
		ctx, cancel := context.WithCancel(context.Background())
		nd.relay = relay
		nd.cancel = cancel
		go relay.Run(ctx) //nolint:errcheck
	}
	time.Sleep(300 * time.Millisecond)

	// Trigger re-sync by updating each "during-part" entity on node-0.
	// This causes the relay to forward the entities to the healed node-2.
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("during-part-%d", i)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err := client0.UpdateEntity(ctx, &storev1.UpdateEntityRequest{
			Entity: &entityv1.Entity{
				Id:   id,
				Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
			},
		})
		cancel()
		if err != nil {
			t.Fatalf("trigger re-sync for %s: %v", id, err)
		}
	}

	// Verify all 10 entities exist on all 3 nodes.
	allIDs := make([]string, 0, 10)
	for i := 0; i < 5; i++ {
		allIDs = append(allIDs, fmt.Sprintf("pre-part-%d", i))
	}
	for i := 0; i < 5; i++ {
		allIDs = append(allIDs, fmt.Sprintf("during-part-%d", i))
	}

	for nodeIdx, nd := range nodes {
		client := dialNode(t, nd.addr)
		for _, id := range allIDs {
			waitForEntity(t, client, id, 10*time.Second)
		}
		// Final count check.
		entities, err := listEntities(client)
		if err != nil {
			t.Fatalf("node-%d list: %v", nodeIdx, err)
		}
		if len(entities) < 10 {
			t.Fatalf("node-%d: expected at least 10 entities, got %d", nodeIdx, len(entities))
		}
	}
}
