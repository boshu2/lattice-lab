package server

import (
	"context"
	"net"
	"testing"
	"time"

	entityv1 "github.com/boshu2/lattice-lab/gen/entity/v1"
	storev1 "github.com/boshu2/lattice-lab/gen/store/v1"
	"github.com/boshu2/lattice-lab/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// startTestServer spins up a gRPC server on a random port and returns the client + cleanup.
func startTestServer(t *testing.T) (storev1.EntityStoreServiceClient, func()) {
	t.Helper()

	s := store.New()
	srv := grpc.NewServer()
	storev1.RegisterEntityStoreServiceServer(srv, New(s))

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	go srv.Serve(lis) //nolint:errcheck

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		srv.Stop()
		t.Fatalf("dial: %v", err)
	}

	client := storev1.NewEntityStoreServiceClient(conn)
	cleanup := func() {
		conn.Close()
		srv.Stop()
	}
	return client, cleanup
}

func TestGRPCCreateAndGet(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx := context.Background()

	created, err := client.CreateEntity(ctx, &storev1.CreateEntityRequest{
		Entity: &entityv1.Entity{
			Id:   "asset-1",
			Type: entityv1.EntityType_ENTITY_TYPE_ASSET,
		},
	})
	if err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}
	if created.Id != "asset-1" {
		t.Fatalf("expected asset-1, got %s", created.Id)
	}

	got, err := client.GetEntity(ctx, &storev1.GetEntityRequest{Id: "asset-1"})
	if err != nil {
		t.Fatalf("GetEntity: %v", err)
	}
	if got.Type != entityv1.EntityType_ENTITY_TYPE_ASSET {
		t.Fatalf("expected ASSET type, got %v", got.Type)
	}
}

func TestGRPCGetNotFound(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	_, err := client.GetEntity(context.Background(), &storev1.GetEntityRequest{Id: "nope"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
}

func TestGRPCListEntities(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx := context.Background()
	_, _ = client.CreateEntity(ctx, &storev1.CreateEntityRequest{
		Entity: &entityv1.Entity{Id: "a1", Type: entityv1.EntityType_ENTITY_TYPE_ASSET},
	})
	_, _ = client.CreateEntity(ctx, &storev1.CreateEntityRequest{
		Entity: &entityv1.Entity{Id: "t1", Type: entityv1.EntityType_ENTITY_TYPE_TRACK},
	})

	resp, err := client.ListEntities(ctx, &storev1.ListEntitiesRequest{})
	if err != nil {
		t.Fatalf("ListEntities: %v", err)
	}
	if len(resp.Entities) != 2 {
		t.Fatalf("expected 2, got %d", len(resp.Entities))
	}

	resp, err = client.ListEntities(ctx, &storev1.ListEntitiesRequest{
		TypeFilter: entityv1.EntityType_ENTITY_TYPE_TRACK,
	})
	if err != nil {
		t.Fatalf("ListEntities filtered: %v", err)
	}
	if len(resp.Entities) != 1 {
		t.Fatalf("expected 1 track, got %d", len(resp.Entities))
	}
}

func TestGRPCUpdateAndDelete(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx := context.Background()
	_, _ = client.CreateEntity(ctx, &storev1.CreateEntityRequest{
		Entity: &entityv1.Entity{Id: "u1", Type: entityv1.EntityType_ENTITY_TYPE_GEO},
	})

	updated, err := client.UpdateEntity(ctx, &storev1.UpdateEntityRequest{
		Entity: &entityv1.Entity{Id: "u1", Type: entityv1.EntityType_ENTITY_TYPE_GEO},
	})
	if err != nil {
		t.Fatalf("UpdateEntity: %v", err)
	}
	if updated.Id != "u1" {
		t.Fatalf("expected u1, got %s", updated.Id)
	}

	_, err = client.DeleteEntity(ctx, &storev1.DeleteEntityRequest{Id: "u1"})
	if err != nil {
		t.Fatalf("DeleteEntity: %v", err)
	}

	_, err = client.GetEntity(ctx, &storev1.GetEntityRequest{Id: "u1"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound after delete, got %v", err)
	}
}

func TestGRPCWatchEntities(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.WatchEntities(ctx, &storev1.WatchEntitiesRequest{})
	if err != nil {
		t.Fatalf("WatchEntities: %v", err)
	}

	// Create an entity in a goroutine so the watch can pick it up.
	go func() {
		time.Sleep(100 * time.Millisecond)
		_, _ = client.CreateEntity(context.Background(), &storev1.CreateEntityRequest{
			Entity: &entityv1.Entity{Id: "w1", Type: entityv1.EntityType_ENTITY_TYPE_TRACK},
		})
	}()

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if event.Type != storev1.EventType_EVENT_TYPE_CREATED {
		t.Fatalf("expected CREATED, got %v", event.Type)
	}
	if event.Entity.Id != "w1" {
		t.Fatalf("expected w1, got %s", event.Entity.Id)
	}
}

func TestGRPCValidation(t *testing.T) {
	client, cleanup := startTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Missing entity.
	_, err := client.CreateEntity(ctx, &storev1.CreateEntityRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for nil entity, got %v", err)
	}

	// Empty ID.
	_, err = client.CreateEntity(ctx, &storev1.CreateEntityRequest{
		Entity: &entityv1.Entity{},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for empty id, got %v", err)
	}
}
