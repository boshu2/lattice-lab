package task

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

func TestRulesNone(t *testing.T) {
	state, tasks := Rules(entityv1.ThreatLevel_THREAT_LEVEL_NONE)
	if state != StateIdle {
		t.Fatalf("expected idle, got %s", state)
	}
	if tasks != nil {
		t.Fatalf("expected no tasks, got %v", tasks)
	}
}

func TestRulesLow(t *testing.T) {
	state, tasks := Rules(entityv1.ThreatLevel_THREAT_LEVEL_LOW)
	if state != StateInvestigate {
		t.Fatalf("expected investigate, got %s", state)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestRulesMedium(t *testing.T) {
	state, tasks := Rules(entityv1.ThreatLevel_THREAT_LEVEL_MEDIUM)
	if state != StateTrack {
		t.Fatalf("expected track, got %s", state)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
}

func TestRulesHigh(t *testing.T) {
	state, tasks := Rules(entityv1.ThreatLevel_THREAT_LEVEL_HIGH)
	if state != StateIntercept {
		t.Fatalf("expected intercept, got %s", state)
	}
	if len(tasks) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(tasks))
	}
}

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

func TestManagerIntegration(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	mgr := New(Config{StoreAddr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go mgr.Run(ctx)

	time.Sleep(100 * time.Millisecond)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := storev1.NewEntityStoreServiceClient(conn)

	// Create a track with threat component (HIGH).
	threat, _ := anypb.New(&entityv1.ThreatComponent{Level: entityv1.ThreatLevel_THREAT_LEVEL_HIGH})
	vel, _ := anypb.New(&entityv1.VelocityComponent{Speed: 400, Heading: 90})

	_, err = client.CreateEntity(ctx, &storev1.CreateEntityRequest{
		Entity: &entityv1.Entity{
			Id:   "track-mgr-test",
			Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
			Components: map[string]*anypb.Any{
				"threat":   threat,
				"velocity": vel,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}

	// Wait for manager to process.
	time.Sleep(500 * time.Millisecond)

	// Check assignment was created.
	a, ok := mgr.GetAssignment("track-mgr-test")
	if !ok {
		t.Fatal("expected assignment for track-mgr-test")
	}
	if a.State != StateIntercept {
		t.Fatalf("expected intercept, got %s", a.State)
	}

	// Check entity was updated with task catalog.
	got, err := client.GetEntity(ctx, &storev1.GetEntityRequest{Id: "track-mgr-test"})
	if err != nil {
		t.Fatalf("GetEntity: %v", err)
	}

	catalogAny, ok := got.Components["task_catalog"]
	if !ok {
		t.Fatal("missing task_catalog component")
	}

	catalog := &entityv1.TaskCatalogComponent{}
	if err := catalogAny.UnmarshalTo(catalog); err != nil {
		t.Fatalf("unmarshal task catalog: %v", err)
	}

	if len(catalog.AvailableTasks) != 4 {
		t.Fatalf("expected 4 tasks, got %d: %v", len(catalog.AvailableTasks), catalog.AvailableTasks)
	}
}
