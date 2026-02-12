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

	go srv.Serve(lis)

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

	go relay.Run(ctx)
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
		client.CreateEntity(context.Background(), &storev1.CreateEntityRequest{
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

	go relay.Run(ctx)
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
