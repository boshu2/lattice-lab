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

// --- Approval gate tests ---

func TestRules_HighThreatPendingApproval(t *testing.T) {
	// Rules() itself is unchanged — it still returns intercept for HIGH.
	state, tasks := Rules(entityv1.ThreatLevel_THREAT_LEVEL_HIGH)
	if state != StateIntercept {
		t.Fatalf("expected intercept, got %s", state)
	}
	if len(tasks) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(tasks))
	}
}

func TestManager_HighThreat_PendingApproval(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	mgr := New(Config{StoreAddr: addr, ApprovalTimeout: 5 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go mgr.Run(ctx) //nolint:errcheck
	time.Sleep(100 * time.Millisecond)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	client := storev1.NewEntityStoreServiceClient(conn)

	// Create HIGH threat track.
	threat, _ := anypb.New(&entityv1.ThreatComponent{Level: entityv1.ThreatLevel_THREAT_LEVEL_HIGH})
	_, err = client.CreateEntity(ctx, &storev1.CreateEntityRequest{
		Entity: &entityv1.Entity{
			Id:         "track-pending",
			Type:       entityv1.EntityType_ENTITY_TYPE_TRACK,
			Components: map[string]*anypb.Any{"threat": threat},
		},
	})
	if err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	a, ok := mgr.GetAssignment("track-pending")
	if !ok {
		t.Fatal("expected assignment for track-pending")
	}
	if a.State != StatePendingApproval {
		t.Fatalf("expected pending_approval, got %s", a.State)
	}
	if len(a.Tasks) != 0 {
		t.Fatalf("expected no tasks while pending, got %v", a.Tasks)
	}
}

func TestManager_ApproveAction(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	mgr := New(Config{StoreAddr: addr, ApprovalTimeout: 5 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go mgr.Run(ctx) //nolint:errcheck
	time.Sleep(100 * time.Millisecond)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	client := storev1.NewEntityStoreServiceClient(conn)

	threat, _ := anypb.New(&entityv1.ThreatComponent{Level: entityv1.ThreatLevel_THREAT_LEVEL_HIGH})
	_, err = client.CreateEntity(ctx, &storev1.CreateEntityRequest{
		Entity: &entityv1.Entity{
			Id:         "track-approve",
			Type:       entityv1.EntityType_ENTITY_TYPE_TRACK,
			Components: map[string]*anypb.Any{"threat": threat},
		},
	})
	if err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Approve it.
	a, err := mgr.Approve("track-approve")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if a.State != StateIntercept {
		t.Fatalf("expected intercept after approve, got %s", a.State)
	}
	if len(a.Tasks) != 4 {
		t.Fatalf("expected 4 tasks after approve, got %d", len(a.Tasks))
	}

	// Check assignment is now intercept.
	got, ok := mgr.GetAssignment("track-approve")
	if !ok {
		t.Fatal("expected assignment")
	}
	if got.State != StateIntercept {
		t.Fatalf("expected intercept, got %s", got.State)
	}
}

func TestManager_DenyAction(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	mgr := New(Config{StoreAddr: addr, ApprovalTimeout: 5 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go mgr.Run(ctx) //nolint:errcheck
	time.Sleep(100 * time.Millisecond)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	client := storev1.NewEntityStoreServiceClient(conn)

	threat, _ := anypb.New(&entityv1.ThreatComponent{Level: entityv1.ThreatLevel_THREAT_LEVEL_HIGH})
	_, err = client.CreateEntity(ctx, &storev1.CreateEntityRequest{
		Entity: &entityv1.Entity{
			Id:         "track-deny",
			Type:       entityv1.EntityType_ENTITY_TYPE_TRACK,
			Components: map[string]*anypb.Any{"threat": threat},
		},
	})
	if err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Deny it.
	err = mgr.Deny("track-deny")
	if err != nil {
		t.Fatalf("Deny: %v", err)
	}

	got, ok := mgr.GetAssignment("track-deny")
	if !ok {
		t.Fatal("expected assignment after deny")
	}
	if got.State != StateIdle {
		t.Fatalf("expected idle after deny, got %s", got.State)
	}
	if len(got.Tasks) != 0 {
		t.Fatalf("expected no tasks after deny, got %v", got.Tasks)
	}
}

func TestManager_ApprovalTimeout(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	// Use very short timeout for testing.
	mgr := New(Config{StoreAddr: addr, ApprovalTimeout: 10 * time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go mgr.Run(ctx) //nolint:errcheck
	time.Sleep(100 * time.Millisecond)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	client := storev1.NewEntityStoreServiceClient(conn)

	threat, _ := anypb.New(&entityv1.ThreatComponent{Level: entityv1.ThreatLevel_THREAT_LEVEL_HIGH})
	_, err = client.CreateEntity(ctx, &storev1.CreateEntityRequest{
		Entity: &entityv1.Entity{
			Id:         "track-timeout",
			Type:       entityv1.EntityType_ENTITY_TYPE_TRACK,
			Components: map[string]*anypb.Any{"threat": threat},
		},
	})
	if err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}

	// Wait for pending + timeout + processing.
	time.Sleep(500 * time.Millisecond)

	got, ok := mgr.GetAssignment("track-timeout")
	if !ok {
		t.Fatal("expected assignment after timeout")
	}
	if got.State != StateIdle {
		t.Fatalf("expected idle after timeout auto-deny, got %s", got.State)
	}
}

func TestManager_EntityDeleteCancelsPending(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	mgr := New(Config{StoreAddr: addr, ApprovalTimeout: 5 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go mgr.Run(ctx) //nolint:errcheck
	time.Sleep(100 * time.Millisecond)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	client := storev1.NewEntityStoreServiceClient(conn)

	threat, _ := anypb.New(&entityv1.ThreatComponent{Level: entityv1.ThreatLevel_THREAT_LEVEL_HIGH})
	_, err = client.CreateEntity(ctx, &storev1.CreateEntityRequest{
		Entity: &entityv1.Entity{
			Id:         "track-del-pending",
			Type:       entityv1.EntityType_ENTITY_TYPE_TRACK,
			Components: map[string]*anypb.Any{"threat": threat},
		},
	})
	if err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Verify pending.
	a, ok := mgr.GetAssignment("track-del-pending")
	if !ok || a.State != StatePendingApproval {
		t.Fatalf("expected pending_approval, got %v %v", ok, a)
	}

	// Delete the entity.
	_, err = client.DeleteEntity(ctx, &storev1.DeleteEntityRequest{Id: "track-del-pending"})
	if err != nil {
		t.Fatalf("DeleteEntity: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	// Assignment and pending should be cleaned up.
	if _, ok := mgr.GetAssignment("track-del-pending"); ok {
		t.Fatal("expected assignment removed after delete")
	}

	// Verify no pending entry (no timer leak).
	mgr.mu.RLock()
	_, hasPending := mgr.pending["track-del-pending"]
	mgr.mu.RUnlock()
	if hasPending {
		t.Fatal("expected pending entry removed after delete")
	}
}

func TestManager_LowThreatNoApproval(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	mgr := New(Config{StoreAddr: addr, ApprovalTimeout: 5 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go mgr.Run(ctx) //nolint:errcheck
	time.Sleep(100 * time.Millisecond)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	client := storev1.NewEntityStoreServiceClient(conn)

	threat, _ := anypb.New(&entityv1.ThreatComponent{Level: entityv1.ThreatLevel_THREAT_LEVEL_LOW})
	_, err = client.CreateEntity(ctx, &storev1.CreateEntityRequest{
		Entity: &entityv1.Entity{
			Id:         "track-low",
			Type:       entityv1.EntityType_ENTITY_TYPE_TRACK,
			Components: map[string]*anypb.Any{"threat": threat},
		},
	})
	if err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	a, ok := mgr.GetAssignment("track-low")
	if !ok {
		t.Fatal("expected assignment for low threat")
	}
	if a.State != StateInvestigate {
		t.Fatalf("expected investigate (no approval gate), got %s", a.State)
	}
	if len(a.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(a.Tasks))
	}
}

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

	go srv.Serve(lis) //nolint:errcheck

	return lis.Addr().String(), func() { srv.Stop() }
}

func TestManagerIntegration(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	mgr := New(Config{StoreAddr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go mgr.Run(ctx) //nolint:errcheck

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

	// HIGH threat now enters pending_approval state first.
	a, ok := mgr.GetAssignment("track-mgr-test")
	if !ok {
		t.Fatal("expected assignment for track-mgr-test")
	}
	if a.State != StatePendingApproval {
		t.Fatalf("expected pending_approval, got %s", a.State)
	}

	// Approve to transition to intercept.
	approved, err := mgr.Approve("track-mgr-test")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if approved.State != StateIntercept {
		t.Fatalf("expected intercept after approve, got %s", approved.State)
	}

	// Wait for store update to propagate (the processEntity re-fires on update).
	time.Sleep(500 * time.Millisecond)

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

func TestManagerDeleteRemovesAssignment(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	mgr := New(Config{StoreAddr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go mgr.Run(ctx) //nolint:errcheck
	time.Sleep(100 * time.Millisecond)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	client := storev1.NewEntityStoreServiceClient(conn)

	threat, _ := anypb.New(&entityv1.ThreatComponent{Level: entityv1.ThreatLevel_THREAT_LEVEL_LOW})
	_, _ = client.CreateEntity(ctx, &storev1.CreateEntityRequest{
		Entity: &entityv1.Entity{
			Id:         "track-del-test",
			Type:       entityv1.EntityType_ENTITY_TYPE_TRACK,
			Components: map[string]*anypb.Any{"threat": threat},
		},
	})
	time.Sleep(300 * time.Millisecond)

	if _, ok := mgr.GetAssignment("track-del-test"); !ok {
		t.Fatal("expected assignment before delete")
	}

	_, _ = client.DeleteEntity(ctx, &storev1.DeleteEntityRequest{Id: "track-del-test"})
	time.Sleep(300 * time.Millisecond)

	if _, ok := mgr.GetAssignment("track-del-test"); ok {
		t.Fatal("expected assignment removed after delete")
	}
}

func TestManagerNoThreatSkips(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	mgr := New(Config{StoreAddr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go mgr.Run(ctx) //nolint:errcheck
	time.Sleep(100 * time.Millisecond)

	conn, _ := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer conn.Close()
	client := storev1.NewEntityStoreServiceClient(conn)

	// Create track without threat component — manager should skip it.
	_, _ = client.CreateEntity(ctx, &storev1.CreateEntityRequest{
		Entity: &entityv1.Entity{
			Id:   "track-no-threat",
			Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
		},
	})
	time.Sleep(300 * time.Millisecond)

	if _, ok := mgr.GetAssignment("track-no-threat"); ok {
		t.Fatal("expected no assignment for entity without threat component")
	}
}

func TestRulesUnspecified(t *testing.T) {
	state, tasks := Rules(entityv1.ThreatLevel_THREAT_LEVEL_UNSPECIFIED)
	if state != StateIdle {
		t.Fatalf("expected idle, got %s", state)
	}
	if tasks != nil {
		t.Fatalf("expected no tasks, got %v", tasks)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.StoreAddr != "localhost:50051" {
		t.Fatalf("expected localhost:50051, got %s", cfg.StoreAddr)
	}
	if cfg.ApprovalTimeout != 30*time.Second {
		t.Fatalf("expected 30s approval timeout, got %s", cfg.ApprovalTimeout)
	}
}
