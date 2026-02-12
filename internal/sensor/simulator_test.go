package sensor

import (
	"context"
	"math"
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

func TestNewTrack(t *testing.T) {
	bbox := BBox{MinLat: 38.8, MaxLat: 39.0, MinLon: -77.2, MaxLon: -76.9}
	tr := newTrack(0, bbox)

	if tr.id != "track-0" {
		t.Fatalf("expected track-0, got %s", tr.id)
	}
	if tr.lat < bbox.MinLat || tr.lat > bbox.MaxLat {
		t.Fatalf("lat %.4f outside bbox", tr.lat)
	}
	if tr.lon < bbox.MinLon || tr.lon > bbox.MaxLon {
		t.Fatalf("lon %.4f outside bbox", tr.lon)
	}
	// Speed should be 100-500 knots in m/s
	minSpeed := 100 * knotsToMps
	maxSpeed := 500 * knotsToMps
	if tr.speed < minSpeed || tr.speed > maxSpeed {
		t.Fatalf("speed %.2f m/s outside expected range", tr.speed)
	}
	if tr.heading < 0 || tr.heading >= 360 {
		t.Fatalf("heading %.1f outside [0,360)", tr.heading)
	}
}

func TestAdvanceTrack(t *testing.T) {
	tr := &track{
		lat:     39.0,
		lon:     -77.0,
		speed:   100 * knotsToMps, // 100 knots
		heading: 90,               // due east
	}

	origLat := tr.lat
	origLon := tr.lon

	advanceTrack(tr, time.Second)

	// Moving due east: lat should barely change, lon should increase
	latDelta := math.Abs(tr.lat - origLat)
	lonDelta := tr.lon - origLon

	if latDelta > 0.0001 {
		t.Fatalf("expected minimal lat change for due-east heading, got delta=%.6f", latDelta)
	}
	if lonDelta <= 0 {
		t.Fatalf("expected lon to increase for due-east heading, got delta=%.6f", lonDelta)
	}
}

func TestAdvanceTrackNorth(t *testing.T) {
	tr := &track{
		lat:     39.0,
		lon:     -77.0,
		speed:   200 * knotsToMps,
		heading: 0, // due north
	}

	origLat := tr.lat
	origLon := tr.lon

	advanceTrack(tr, time.Second)

	latDelta := tr.lat - origLat
	lonDelta := math.Abs(tr.lon - origLon)

	if latDelta <= 0 {
		t.Fatalf("expected lat to increase for due-north heading, got delta=%.6f", latDelta)
	}
	if lonDelta > 0.0001 {
		t.Fatalf("expected minimal lon change for due-north heading, got delta=%.6f", lonDelta)
	}
}

func TestBuildEntity(t *testing.T) {
	tr := &track{
		id:      "track-0",
		lat:     38.9,
		lon:     -77.0,
		alt:     3000,
		speed:   150 * knotsToMps,
		heading: 45,
	}

	entity, err := buildEntity(tr)
	if err != nil {
		t.Fatalf("buildEntity: %v", err)
	}
	if entity.Id != "track-0" {
		t.Fatalf("expected track-0, got %s", entity.Id)
	}
	if entity.Type != entityv1.EntityType_ENTITY_TYPE_TRACK {
		t.Fatalf("expected TRACK type, got %v", entity.Type)
	}
	if _, ok := entity.Components["position"]; !ok {
		t.Fatal("missing position component")
	}
	if _, ok := entity.Components["velocity"]; !ok {
		t.Fatal("missing velocity component")
	}
}

// startTestServer spins up entity-store on a random port for integration testing.
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

	cleanup := func() {
		srv.Stop()
	}
	return lis.Addr().String(), cleanup
}

func TestSimulatorIntegration(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	cfg := Config{
		StoreAddr: addr,
		Interval:  100 * time.Millisecond,
		NumTracks: 2,
		BBox:      BBox{MinLat: 38.8, MaxLat: 39.0, MinLon: -77.2, MaxLon: -76.9},
	}

	sim := New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer cancel()

	// Run simulator — it should create 2 tracks, then update them at least once.
	_ = sim.Run(ctx)

	// Verify tracks were created in the store.
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := storev1.NewEntityStoreServiceClient(conn)
	resp, err := client.ListEntities(context.Background(), &storev1.ListEntitiesRequest{
		TypeFilter: entityv1.EntityType_ENTITY_TYPE_TRACK,
	})
	if err != nil {
		t.Fatalf("ListEntities: %v", err)
	}
	if len(resp.Entities) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(resp.Entities))
	}

	// Verify entities have components.
	for _, e := range resp.Entities {
		if _, ok := e.Components["position"]; !ok {
			t.Fatalf("entity %s missing position component", e.Id)
		}
		if _, ok := e.Components["velocity"]; !ok {
			t.Fatalf("entity %s missing velocity component", e.Id)
		}
	}
}
