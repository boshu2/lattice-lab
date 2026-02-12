package mesh

import (
	"context"
	"net"
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

func startTestServer(t *testing.T) (string, func()) {
	t.Helper()

	s := store.New()
	srv := grpc.NewServer()
	storev1.RegisterEntityStoreServiceServer(srv, server.New(s))

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	go srv.Serve(lis) //nolint:errcheck

	return lis.Addr().String(), func() { srv.Stop() }
}

func TestRelayNoPeers(t *testing.T) {
	relay := New(Config{LocalAddr: "localhost:50051"})
	err := relay.Run(context.Background())
	if err == nil {
		t.Fatal("expected error with no peers")
	}
}

func TestRelayForwardCreate(t *testing.T) {
	// Start two stores: local + peer.
	localAddr, localCleanup := startTestServer(t)
	defer localCleanup()

	peerAddr, peerCleanup := startTestServer(t)
	defer peerCleanup()

	relay := New(Config{
		LocalAddr: localAddr,
		Peers:     []string{peerAddr},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go relay.Run(ctx) //nolint:errcheck
	time.Sleep(100 * time.Millisecond)

	// Create entity on local store.
	localConn, err := grpc.NewClient(localAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial local: %v", err)
	}
	defer localConn.Close()

	localClient := storev1.NewEntityStoreServiceClient(localConn)
	_, err = localClient.CreateEntity(ctx, &storev1.CreateEntityRequest{
		Entity: &entityv1.Entity{
			Id:   "mesh-test-1",
			Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
		},
	})
	if err != nil {
		t.Fatalf("create on local: %v", err)
	}

	// Wait for relay to forward.
	time.Sleep(500 * time.Millisecond)

	// Verify entity exists on peer.
	peerConn, err := grpc.NewClient(peerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial peer: %v", err)
	}
	defer peerConn.Close()

	peerClient := storev1.NewEntityStoreServiceClient(peerConn)
	got, err := peerClient.GetEntity(ctx, &storev1.GetEntityRequest{Id: "mesh-test-1"})
	if err != nil {
		t.Fatalf("get on peer: %v", err)
	}
	if got.Id != "mesh-test-1" {
		t.Fatalf("expected mesh-test-1, got %s", got.Id)
	}
	if got.Type != entityv1.EntityType_ENTITY_TYPE_TRACK {
		t.Fatalf("expected TRACK, got %v", got.Type)
	}

	stats := relay.GetStats()
	if stats.Forwarded < 1 {
		t.Fatalf("expected at least 1 forwarded, got %d", stats.Forwarded)
	}
}

func TestRelayForwardDelete(t *testing.T) {
	localAddr, localCleanup := startTestServer(t)
	defer localCleanup()

	peerAddr, peerCleanup := startTestServer(t)
	defer peerCleanup()

	// Pre-create entity on both stores.
	for _, addr := range []string{localAddr, peerAddr} {
		conn, _ := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		client := storev1.NewEntityStoreServiceClient(conn)
		_, _ = client.CreateEntity(context.Background(), &storev1.CreateEntityRequest{
			Entity: &entityv1.Entity{
				Id:   "mesh-del-1",
				Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
			},
		})
		conn.Close()
	}

	relay := New(Config{
		LocalAddr: localAddr,
		Peers:     []string{peerAddr},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go relay.Run(ctx) //nolint:errcheck
	time.Sleep(100 * time.Millisecond)

	// Delete entity on local.
	localConn, _ := grpc.NewClient(localAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer localConn.Close()
	localClient := storev1.NewEntityStoreServiceClient(localConn)

	_, err := localClient.DeleteEntity(ctx, &storev1.DeleteEntityRequest{Id: "mesh-del-1"})
	if err != nil {
		t.Fatalf("delete on local: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Verify entity is deleted on peer.
	peerConn, _ := grpc.NewClient(peerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer peerConn.Close()
	peerClient := storev1.NewEntityStoreServiceClient(peerConn)

	_, err = peerClient.GetEntity(ctx, &storev1.GetEntityRequest{Id: "mesh-del-1"})
	if err == nil {
		t.Fatal("expected entity to be deleted on peer")
	}
}

func TestRelayForwardUpdate(t *testing.T) {
	localAddr, localCleanup := startTestServer(t)
	defer localCleanup()

	peerAddr, peerCleanup := startTestServer(t)
	defer peerCleanup()

	relay := New(Config{
		LocalAddr: localAddr,
		Peers:     []string{peerAddr},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go relay.Run(ctx) //nolint:errcheck
	time.Sleep(100 * time.Millisecond)

	localConn, err := grpc.NewClient(localAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial local: %v", err)
	}
	defer localConn.Close()
	localClient := storev1.NewEntityStoreServiceClient(localConn)

	// Create then update on local.
	_, err = localClient.CreateEntity(ctx, &storev1.CreateEntityRequest{
		Entity: &entityv1.Entity{
			Id:   "mesh-upd-1",
			Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	_, err = localClient.UpdateEntity(ctx, &storev1.UpdateEntityRequest{
		Entity: &entityv1.Entity{
			Id:   "mesh-upd-1",
			Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
		},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Verify entity exists on peer (was forwarded via update path).
	peerConn, err := grpc.NewClient(peerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial peer: %v", err)
	}
	defer peerConn.Close()

	peerClient := storev1.NewEntityStoreServiceClient(peerConn)
	got, err := peerClient.GetEntity(ctx, &storev1.GetEntityRequest{Id: "mesh-upd-1"})
	if err != nil {
		t.Fatalf("get on peer after update: %v", err)
	}
	if got.Id != "mesh-upd-1" {
		t.Fatalf("expected mesh-upd-1, got %s", got.Id)
	}
}

func TestRelayBidirectional(t *testing.T) {
	// Two stores, each relaying to the other.
	addr1, cleanup1 := startTestServer(t)
	defer cleanup1()

	addr2, cleanup2 := startTestServer(t)
	defer cleanup2()

	relay1 := New(Config{LocalAddr: addr1, Peers: []string{addr2}, NodeID: "node-1"})
	relay2 := New(Config{LocalAddr: addr2, Peers: []string{addr1}, NodeID: "node-2"})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go relay1.Run(ctx) //nolint:errcheck
	go relay2.Run(ctx) //nolint:errcheck
	time.Sleep(100 * time.Millisecond)

	// Create entity on store 1.
	conn1, _ := grpc.NewClient(addr1, grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer conn1.Close()
	client1 := storev1.NewEntityStoreServiceClient(conn1)

	_, err := client1.CreateEntity(ctx, &storev1.CreateEntityRequest{
		Entity: &entityv1.Entity{Id: "bidir-1", Type: entityv1.EntityType_ENTITY_TYPE_ASSET},
	})
	if err != nil {
		t.Fatalf("create on store1: %v", err)
	}

	// Create different entity on store 2.
	conn2, _ := grpc.NewClient(addr2, grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer conn2.Close()
	client2 := storev1.NewEntityStoreServiceClient(conn2)

	_, err = client2.CreateEntity(ctx, &storev1.CreateEntityRequest{
		Entity: &entityv1.Entity{Id: "bidir-2", Type: entityv1.EntityType_ENTITY_TYPE_TRACK},
	})
	if err != nil {
		t.Fatalf("create on store2: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// bidir-1 should exist on store 2 (relayed from store 1).
	got, err := client2.GetEntity(ctx, &storev1.GetEntityRequest{Id: "bidir-1"})
	if err != nil {
		t.Fatalf("bidir-1 not found on store2: %v", err)
	}
	if got.Type != entityv1.EntityType_ENTITY_TYPE_ASSET {
		t.Fatalf("expected ASSET, got %v", got.Type)
	}

	// bidir-2 should exist on store 1 (relayed from store 2).
	got, err = client1.GetEntity(ctx, &storev1.GetEntityRequest{Id: "bidir-2"})
	if err != nil {
		t.Fatalf("bidir-2 not found on store1: %v", err)
	}
	if got.Type != entityv1.EntityType_ENTITY_TYPE_TRACK {
		t.Fatalf("expected TRACK, got %v", got.Type)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.LocalAddr != "localhost:50051" {
		t.Fatalf("expected localhost:50051, got %s", cfg.LocalAddr)
	}
}

func TestRelay_EchoSuppression(t *testing.T) {
	// Relay with node ID "node-A" should skip events with origin_node="node-A".
	localAddr, localCleanup := startTestServer(t)
	defer localCleanup()

	peerAddr, peerCleanup := startTestServer(t)
	defer peerCleanup()

	relay := New(Config{
		LocalAddr: localAddr,
		Peers:     []string{peerAddr},
		NodeID:    "node-A",
	})

	// Directly test forwardToPeers with an event that has matching origin_node.
	// The relay should suppress it (not forward to peer).
	peerConn, err := grpc.NewClient(peerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial peer: %v", err)
	}
	defer peerConn.Close()
	peerClient := storev1.NewEntityStoreServiceClient(peerConn)

	event := &storev1.EntityEvent{
		Type: storev1.EventType_EVENT_TYPE_CREATED,
		Entity: &entityv1.Entity{
			Id:   "echo-test-1",
			Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
		},
		OriginNode: "node-A", // Same as relay's NodeID — should be suppressed
	}

	relay.forwardToPeers(context.Background(), []storev1.EntityStoreServiceClient{peerClient}, event)

	// Entity should NOT exist on peer because it was suppressed.
	_, err = peerClient.GetEntity(context.Background(), &storev1.GetEntityRequest{Id: "echo-test-1"})
	if err == nil {
		t.Fatal("expected entity to NOT exist on peer (echo suppression failed)")
	}

	stats := relay.GetStats()
	if stats.Forwarded != 0 {
		t.Fatalf("expected 0 forwarded (suppressed), got %d", stats.Forwarded)
	}
}

func TestRelay_ForwardsNonLocalEvents(t *testing.T) {
	// Relay "node-A" should forward events with origin_node="node-B".
	localAddr, localCleanup := startTestServer(t)
	defer localCleanup()

	peerAddr, peerCleanup := startTestServer(t)
	defer peerCleanup()

	relay := New(Config{
		LocalAddr: localAddr,
		Peers:     []string{peerAddr},
		NodeID:    "node-A",
	})

	peerConn, err := grpc.NewClient(peerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial peer: %v", err)
	}
	defer peerConn.Close()
	peerClient := storev1.NewEntityStoreServiceClient(peerConn)

	event := &storev1.EntityEvent{
		Type: storev1.EventType_EVENT_TYPE_CREATED,
		Entity: &entityv1.Entity{
			Id:   "nonlocal-test-1",
			Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
		},
		OriginNode: "node-B", // Different from relay's NodeID — should forward
	}

	relay.forwardToPeers(context.Background(), []storev1.EntityStoreServiceClient{peerClient}, event)

	// Entity should exist on peer because it was forwarded.
	got, err := peerClient.GetEntity(context.Background(), &storev1.GetEntityRequest{Id: "nonlocal-test-1"})
	if err != nil {
		t.Fatalf("expected entity to exist on peer: %v", err)
	}
	if got.Id != "nonlocal-test-1" {
		t.Fatalf("expected nonlocal-test-1, got %s", got.Id)
	}

	stats := relay.GetStats()
	if stats.Forwarded < 1 {
		t.Fatalf("expected at least 1 forwarded, got %d", stats.Forwarded)
	}
}

func TestRelay_MergeOnConflict(t *testing.T) {
	// Create entity on peer with HIGH threat and older HLC.
	// Forward entity from local with LOW threat and newer HLC.
	// After merge, peer should have HIGH threat (max-wins for threat component).
	localAddr, localCleanup := startTestServer(t)
	defer localCleanup()

	peerAddr, peerCleanup := startTestServer(t)
	defer peerCleanup()

	ctx := context.Background()

	// Create entity on peer with HIGH threat (older HLC).
	peerConn, err := grpc.NewClient(peerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial peer: %v", err)
	}
	defer peerConn.Close()
	peerClient := storev1.NewEntityStoreServiceClient(peerConn)

	threatHigh, _ := anypb.New(&entityv1.ThreatComponent{
		Level: entityv1.ThreatLevel_THREAT_LEVEL_HIGH,
	})
	_, err = peerClient.CreateEntity(ctx, &storev1.CreateEntityRequest{
		Entity: &entityv1.Entity{
			Id:   "merge-test-1",
			Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
			Components: map[string]*anypb.Any{
				"threat": threatHigh,
			},
		},
	})
	if err != nil {
		t.Fatalf("create on peer: %v", err)
	}

	// Build incoming entity with LOW threat and a position component (newer HLC).
	threatLow, _ := anypb.New(&entityv1.ThreatComponent{
		Level: entityv1.ThreatLevel_THREAT_LEVEL_LOW,
	})
	posComp, _ := anypb.New(&entityv1.PositionComponent{
		Lat: 10.0,
		Lon: 20.0,
	})

	relay := New(Config{
		LocalAddr: localAddr,
		Peers:     []string{peerAddr},
		NodeID:    "node-A",
	})

	// Forward an update event for merge-test-1 with LOW threat but newer HLC + position.
	incomingEntity := &entityv1.Entity{
		Id:   "merge-test-1",
		Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
		Components: map[string]*anypb.Any{
			"threat":   threatLow,
			"position": posComp,
		},
		HlcPhysical: uint64(time.Now().UnixNano()) + 1000000000, // Future timestamp — newer
		HlcLogical:  1,
		HlcNode:     "node-A",
	}

	event := &storev1.EntityEvent{
		Type:       storev1.EventType_EVENT_TYPE_UPDATED,
		Entity:     incomingEntity,
		OriginNode: "node-B",
	}

	relay.forwardToPeers(ctx, []storev1.EntityStoreServiceClient{peerClient}, event)

	// Verify merged result on peer.
	got, err := peerClient.GetEntity(ctx, &storev1.GetEntityRequest{Id: "merge-test-1"})
	if err != nil {
		t.Fatalf("get merged entity: %v", err)
	}

	// Threat should be HIGH (max-wins) even though incoming had LOW.
	if got.Components["threat"] == nil {
		t.Fatal("expected threat component on merged entity")
	}
	var gotThreat entityv1.ThreatComponent
	if err := got.Components["threat"].UnmarshalTo(&gotThreat); err != nil {
		t.Fatalf("unmarshal threat: %v", err)
	}
	if gotThreat.Level != entityv1.ThreatLevel_THREAT_LEVEL_HIGH {
		t.Fatalf("expected HIGH threat (max-wins), got %v", gotThreat.Level)
	}

	// Position should be present (from incoming, only source).
	if got.Components["position"] == nil {
		t.Fatal("expected position component on merged entity")
	}

	stats := relay.GetStats()
	if stats.Merged < 1 {
		t.Fatalf("expected at least 1 merge, got %d", stats.Merged)
	}
}

func TestRelay_MergeStats(t *testing.T) {
	// After forwarding an update to a peer that already has the entity,
	// the Merged counter should increment.
	localAddr, localCleanup := startTestServer(t)
	defer localCleanup()

	peerAddr, peerCleanup := startTestServer(t)
	defer peerCleanup()

	ctx := context.Background()

	// Pre-create entity on peer.
	peerConn, err := grpc.NewClient(peerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial peer: %v", err)
	}
	defer peerConn.Close()
	peerClient := storev1.NewEntityStoreServiceClient(peerConn)

	_, err = peerClient.CreateEntity(ctx, &storev1.CreateEntityRequest{
		Entity: &entityv1.Entity{
			Id:   "stats-test-1",
			Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
		},
	})
	if err != nil {
		t.Fatalf("create on peer: %v", err)
	}

	relay := New(Config{
		LocalAddr: localAddr,
		Peers:     []string{peerAddr},
		NodeID:    "node-A",
	})

	// Forward an update — should trigger merge since entity exists on peer.
	event := &storev1.EntityEvent{
		Type: storev1.EventType_EVENT_TYPE_UPDATED,
		Entity: &entityv1.Entity{
			Id:          "stats-test-1",
			Type:        entityv1.EntityType_ENTITY_TYPE_TRACK,
			HlcPhysical: uint64(time.Now().UnixNano()),
			HlcLogical:  1,
			HlcNode:     "node-B",
		},
		OriginNode: "node-B",
	}

	relay.forwardToPeers(ctx, []storev1.EntityStoreServiceClient{peerClient}, event)

	stats := relay.GetStats()
	if stats.Forwarded != 1 {
		t.Fatalf("expected 1 forwarded, got %d", stats.Forwarded)
	}
	if stats.Merged != 1 {
		t.Fatalf("expected 1 merged, got %d", stats.Merged)
	}
}
