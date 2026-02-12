package fusion

import (
	"math"
	"testing"

	entityv1 "github.com/boshu2/lattice-lab/gen/entity/v1"
	"google.golang.org/protobuf/types/known/anypb"
)

// makeTrackEntity builds a test entity with position and source components.
func makeTrackEntity(id string, lat, lon float64, sensorID, sensorType string) *entityv1.Entity {
	pos, _ := anypb.New(&entityv1.PositionComponent{Lat: lat, Lon: lon, Alt: 3000})
	src, _ := anypb.New(&entityv1.SourceComponent{SensorId: sensorID, SensorType: sensorType})
	return &entityv1.Entity{
		Id:   id,
		Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
		Components: map[string]*anypb.Any{
			"position": pos,
			"source":   src,
		},
	}
}

func TestCorrelate_WithinThreshold(t *testing.T) {
	f := New(Config{DistThreshold: 0.01})

	// Two tracks from different sensors, within 0.005 degrees apart.
	f.UpdateTrack(makeTrackEntity("track-0", 38.9000, -77.0000, "eo-1", "eo"))
	f.UpdateTrack(makeTrackEntity("radar-track-0", 38.9040, -77.0030, "radar-1", "radar"))

	corrs := f.Correlations()
	if len(corrs) != 1 {
		t.Fatalf("expected 1 correlation, got %d", len(corrs))
	}

	c := corrs[0]
	if (c.TrackA != "track-0" || c.TrackB != "radar-track-0") &&
		(c.TrackA != "radar-track-0" || c.TrackB != "track-0") {
		t.Fatalf("unexpected correlation pair: %s, %s", c.TrackA, c.TrackB)
	}
}

func TestCorrelate_BeyondThreshold(t *testing.T) {
	f := New(Config{DistThreshold: 0.01})

	// Two tracks from different sensors, far apart (> 0.01 degrees).
	f.UpdateTrack(makeTrackEntity("track-0", 38.9000, -77.0000, "eo-1", "eo"))
	f.UpdateTrack(makeTrackEntity("radar-track-0", 39.0000, -76.8000, "radar-1", "radar"))

	corrs := f.Correlations()
	if len(corrs) != 0 {
		t.Fatalf("expected 0 correlations, got %d", len(corrs))
	}
}

func TestCorrelate_SameSensorIgnored(t *testing.T) {
	f := New(Config{DistThreshold: 0.01})

	// Two tracks from the SAME sensor, very close together.
	f.UpdateTrack(makeTrackEntity("track-0", 38.9000, -77.0000, "eo-1", "eo"))
	f.UpdateTrack(makeTrackEntity("track-1", 38.9001, -77.0001, "eo-1", "eo"))

	corrs := f.Correlations()
	if len(corrs) != 0 {
		t.Fatalf("expected 0 correlations from same sensor, got %d", len(corrs))
	}
}

func TestFusedPosition_WeightedAverage(t *testing.T) {
	a := &trackInfo{entityID: "a", lat: 38.9000, lon: -77.0000, sensorID: "eo-1"}
	b := &trackInfo{entityID: "b", lat: 38.9100, lon: -77.0100, sensorID: "radar-1"}

	lat, lon := FusedPosition(a, b)

	wantLat := (38.9000 + 38.9100) / 2.0
	wantLon := (-77.0000 + -77.0100) / 2.0

	if math.Abs(lat-wantLat) > 1e-9 {
		t.Fatalf("fused lat: got %f, want %f", lat, wantLat)
	}
	if math.Abs(lon-wantLon) > 1e-9 {
		t.Fatalf("fused lon: got %f, want %f", lon, wantLon)
	}
}

func TestDecorrelate(t *testing.T) {
	f := New(Config{DistThreshold: 0.01})

	// Start correlated.
	f.UpdateTrack(makeTrackEntity("track-0", 38.9000, -77.0000, "eo-1", "eo"))
	f.UpdateTrack(makeTrackEntity("radar-track-0", 38.9040, -77.0030, "radar-1", "radar"))

	corrs := f.Correlations()
	if len(corrs) != 1 {
		t.Fatalf("setup: expected 1 correlation, got %d", len(corrs))
	}

	// Move radar track far away — should de-correlate.
	f.UpdateTrack(makeTrackEntity("radar-track-0", 39.5000, -76.0000, "radar-1", "radar"))

	corrs = f.Correlations()
	if len(corrs) != 0 {
		t.Fatalf("expected 0 correlations after divergence, got %d", len(corrs))
	}
}

func TestFusionComponent(t *testing.T) {
	f := New(Config{DistThreshold: 0.01})

	f.UpdateTrack(makeTrackEntity("track-0", 38.9000, -77.0000, "eo-1", "eo"))
	f.UpdateTrack(makeTrackEntity("radar-track-0", 38.9040, -77.0030, "radar-1", "radar"))

	fused := f.BuildFusedEntities()
	if len(fused) != 1 {
		t.Fatalf("expected 1 fused entity, got %d", len(fused))
	}

	ent := fused[0]
	fusionAny, ok := ent.Components["fusion"]
	if !ok {
		t.Fatal("fused entity missing fusion component")
	}

	fc := &entityv1.FusionComponent{}
	if err := fusionAny.UnmarshalTo(fc); err != nil {
		t.Fatalf("unmarshal fusion component: %v", err)
	}

	if len(fc.SourceIds) != 2 {
		t.Fatalf("expected 2 source IDs, got %d", len(fc.SourceIds))
	}

	// Verify fused position is the average.
	wantLat := (38.9000 + 38.9040) / 2.0
	wantLon := (-77.0000 + -77.0030) / 2.0

	if math.Abs(fc.FusedLat-wantLat) > 1e-9 {
		t.Fatalf("fused lat: got %f, want %f", fc.FusedLat, wantLat)
	}
	if math.Abs(fc.FusedLon-wantLon) > 1e-9 {
		t.Fatalf("fused lon: got %f, want %f", fc.FusedLon, wantLon)
	}

	if fc.Confidence <= 0 || fc.Confidence > 1.0 {
		t.Fatalf("confidence out of range: %f", fc.Confidence)
	}
}

func TestRemoveTrack(t *testing.T) {
	f := New(Config{DistThreshold: 0.01})

	f.UpdateTrack(makeTrackEntity("track-0", 38.9000, -77.0000, "eo-1", "eo"))
	f.UpdateTrack(makeTrackEntity("radar-track-0", 38.9040, -77.0030, "radar-1", "radar"))

	corrs := f.Correlations()
	if len(corrs) != 1 {
		t.Fatalf("setup: expected 1 correlation, got %d", len(corrs))
	}

	// Remove one track — correlation should disappear.
	f.RemoveTrack("track-0")

	corrs = f.Correlations()
	if len(corrs) != 0 {
		t.Fatalf("expected 0 correlations after removal, got %d", len(corrs))
	}
}

func TestDistance(t *testing.T) {
	// Same point.
	d := Distance(38.9, -77.0, 38.9, -77.0)
	if d != 0 {
		t.Fatalf("same point distance should be 0, got %f", d)
	}

	// Known offset.
	d = Distance(0, 0, 0.003, 0.004)
	want := 0.005
	if math.Abs(d-want) > 1e-9 {
		t.Fatalf("distance: got %f, want %f", d, want)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.StoreAddr != "localhost:50051" {
		t.Fatalf("expected localhost:50051, got %s", cfg.StoreAddr)
	}
	if cfg.DistThreshold != 0.01 {
		t.Fatalf("expected 0.01, got %f", cfg.DistThreshold)
	}
}
