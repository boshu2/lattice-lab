package classifier

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

func TestClassifyCivilian(t *testing.T) {
	cl := Classify(100) // 100 knots
	if cl.Label != "civilian" {
		t.Fatalf("expected civilian, got %s", cl.Label)
	}
	if cl.Threat != entityv1.ThreatLevel_THREAT_LEVEL_NONE {
		t.Fatalf("expected NONE threat, got %v", cl.Threat)
	}
}

func TestClassifyAircraft(t *testing.T) {
	cl := Classify(250) // 250 knots
	if cl.Label != "aircraft" {
		t.Fatalf("expected aircraft, got %s", cl.Label)
	}
	if cl.Threat != entityv1.ThreatLevel_THREAT_LEVEL_LOW {
		t.Fatalf("expected LOW threat, got %v", cl.Threat)
	}
}

func TestClassifyMilitary(t *testing.T) {
	cl := Classify(500) // 500 knots
	if cl.Label != "military" {
		t.Fatalf("expected military, got %s", cl.Label)
	}
	if cl.Threat != entityv1.ThreatLevel_THREAT_LEVEL_HIGH {
		t.Fatalf("expected HIGH threat, got %v", cl.Threat)
	}
}

func TestClassifyBoundaries(t *testing.T) {
	// Exactly 150 should be aircraft (not civilian)
	cl := Classify(150)
	if cl.Label != "aircraft" {
		t.Fatalf("at 150 kts expected aircraft, got %s", cl.Label)
	}

	// Exactly 350 should be aircraft (not military)
	cl = Classify(350)
	if cl.Label != "aircraft" {
		t.Fatalf("at 350 kts expected aircraft, got %s", cl.Label)
	}

	// Just above 350 should be military
	cl = Classify(351)
	if cl.Label != "military" {
		t.Fatalf("at 351 kts expected military, got %s", cl.Label)
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

func TestClassifierIntegration(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	// Start classifier in background.
	cl := New(Config{StoreAddr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go cl.Run(ctx)

	// Give classifier time to connect and start watching.
	time.Sleep(100 * time.Millisecond)

	// Create a track with velocity component.
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := storev1.NewEntityStoreServiceClient(conn)

	vel, _ := anypb.New(&entityv1.VelocityComponent{Speed: 400, Heading: 90})
	pos, _ := anypb.New(&entityv1.PositionComponent{Lat: 38.9, Lon: -77.0, Alt: 3000})

	_, err = client.CreateEntity(ctx, &storev1.CreateEntityRequest{
		Entity: &entityv1.Entity{
			Id:   "track-test",
			Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
			Components: map[string]*anypb.Any{
				"velocity": vel,
				"position": pos,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}

	// Wait for classifier to process the event and update the entity.
	time.Sleep(500 * time.Millisecond)

	// Verify classification was added.
	got, err := client.GetEntity(ctx, &storev1.GetEntityRequest{Id: "track-test"})
	if err != nil {
		t.Fatalf("GetEntity: %v", err)
	}

	clAny, ok := got.Components["classification"]
	if !ok {
		t.Fatal("missing classification component after classifier ran")
	}

	clComp := &entityv1.ClassificationComponent{}
	if err := clAny.UnmarshalTo(clComp); err != nil {
		t.Fatalf("unmarshal classification: %v", err)
	}

	if clComp.Label != "military" {
		t.Fatalf("expected military for 400 kts, got %s", clComp.Label)
	}

	threatAny, ok := got.Components["threat"]
	if !ok {
		t.Fatal("missing threat component after classifier ran")
	}

	threatComp := &entityv1.ThreatComponent{}
	if err := threatAny.UnmarshalTo(threatComp); err != nil {
		t.Fatalf("unmarshal threat: %v", err)
	}

	if threatComp.Level != entityv1.ThreatLevel_THREAT_LEVEL_HIGH {
		t.Fatalf("expected HIGH threat, got %v", threatComp.Level)
	}
}
