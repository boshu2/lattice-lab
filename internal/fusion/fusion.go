package fusion

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"sync"

	entityv1 "github.com/boshu2/lattice-lab/gen/entity/v1"
	storev1 "github.com/boshu2/lattice-lab/gen/store/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/anypb"
)

// Config controls the fusion service.
type Config struct {
	StoreAddr     string
	DistThreshold float64 // degrees, default 0.01 (~1.1km)
}

// DefaultConfig returns fusion defaults.
func DefaultConfig() Config {
	return Config{
		StoreAddr:     "localhost:50051",
		DistThreshold: 0.01,
	}
}

// trackInfo holds extracted position and sensor data for a track entity.
type trackInfo struct {
	entityID string
	lat, lon float64
	sensorID string
}

// Correlation represents a pair of tracks from different sensors that are
// close enough to be considered the same real-world object.
type Correlation struct {
	TrackA  string
	TrackB  string
	FusedID string // ID of the fused entity in the store
}

// Fusioner watches tracks from multiple sensors, correlates by distance, and
// creates fused entities.
type Fusioner struct {
	cfg    Config
	mu     sync.RWMutex
	tracks map[string]*trackInfo // entityID -> trackInfo
}

// New creates a Fusioner with the given config.
func New(cfg Config) *Fusioner {
	return &Fusioner{
		cfg:    cfg,
		tracks: make(map[string]*trackInfo),
	}
}

// UpdateTrack extracts position and source from an entity and updates the
// internal tracks map. Returns true if the entity had valid position+source.
func (f *Fusioner) UpdateTrack(entity *entityv1.Entity) bool {
	ti, err := extractTrackInfo(entity)
	if err != nil {
		return false
	}
	f.mu.Lock()
	f.tracks[ti.entityID] = ti
	f.mu.Unlock()
	return true
}

// RemoveTrack removes a track from the internal map.
func (f *Fusioner) RemoveTrack(entityID string) {
	f.mu.Lock()
	delete(f.tracks, entityID)
	f.mu.Unlock()
}

// Correlations returns all current correlations between tracks from different
// sensors that are within the distance threshold. This is the pure, testable
// core of the fusion logic.
func (f *Fusioner) Correlations() []Correlation {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Collect tracks into a slice for pairwise comparison.
	all := make([]*trackInfo, 0, len(f.tracks))
	for _, ti := range f.tracks {
		all = append(all, ti)
	}

	var corrs []Correlation
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			a, b := all[i], all[j]
			// Skip same-sensor pairs.
			if a.sensorID == b.sensorID {
				continue
			}
			if Distance(a.lat, a.lon, b.lat, b.lon) < f.cfg.DistThreshold {
				// Deterministic fused ID from sorted track IDs.
				ids := []string{a.entityID, b.entityID}
				sort.Strings(ids)
				fusedID := fmt.Sprintf("fused-%s-%s", ids[0], ids[1])
				corrs = append(corrs, Correlation{
					TrackA:  a.entityID,
					TrackB:  b.entityID,
					FusedID: fusedID,
				})
			}
		}
	}
	return corrs
}

// BuildFusedEntities constructs Entity protos for all current correlations.
func (f *Fusioner) BuildFusedEntities() []*entityv1.Entity {
	f.mu.RLock()
	defer f.mu.RUnlock()

	corrs := f.correlationsLocked()
	entities := make([]*entityv1.Entity, 0, len(corrs))

	for _, c := range corrs {
		a, okA := f.tracks[c.TrackA]
		b, okB := f.tracks[c.TrackB]
		if !okA || !okB {
			continue
		}

		lat, lon := FusedPosition(a, b)
		dist := Distance(a.lat, a.lon, b.lat, b.lon)
		// Confidence: inversely proportional to distance, capped at 1.0.
		confidence := float32(1.0 - (dist / f.cfg.DistThreshold))
		if confidence < 0.1 {
			confidence = 0.1
		}

		fc, err := anypb.New(&entityv1.FusionComponent{
			SourceIds: []string{c.TrackA, c.TrackB},
			FusedLat:  lat,
			FusedLon:  lon,
			Confidence: confidence,
		})
		if err != nil {
			continue
		}

		pos, err := anypb.New(&entityv1.PositionComponent{
			Lat: lat,
			Lon: lon,
		})
		if err != nil {
			continue
		}

		entities = append(entities, &entityv1.Entity{
			Id:   c.FusedID,
			Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
			Components: map[string]*anypb.Any{
				"fusion":   fc,
				"position": pos,
			},
		})
	}
	return entities
}

// correlationsLocked is the internal version that assumes the read lock is held.
func (f *Fusioner) correlationsLocked() []Correlation {
	all := make([]*trackInfo, 0, len(f.tracks))
	for _, ti := range f.tracks {
		all = append(all, ti)
	}

	var corrs []Correlation
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			a, b := all[i], all[j]
			if a.sensorID == b.sensorID {
				continue
			}
			if Distance(a.lat, a.lon, b.lat, b.lon) < f.cfg.DistThreshold {
				ids := []string{a.entityID, b.entityID}
				sort.Strings(ids)
				fusedID := fmt.Sprintf("fused-%s-%s", ids[0], ids[1])
				corrs = append(corrs, Correlation{
					TrackA:  a.entityID,
					TrackB:  b.entityID,
					FusedID: fusedID,
				})
			}
		}
	}
	return corrs
}

// FusedPosition returns the average position of two tracks.
func FusedPosition(a, b *trackInfo) (lat, lon float64) {
	return (a.lat + b.lat) / 2.0, (a.lon + b.lon) / 2.0
}

// Distance returns the Euclidean distance in degrees between two points
// (flat-earth approximation, fine for the learning lab).
func Distance(lat1, lon1, lat2, lon2 float64) float64 {
	dlat := lat2 - lat1
	dlon := lon2 - lon1
	return math.Sqrt(dlat*dlat + dlon*dlon)
}

// extractTrackInfo extracts position and source data from an entity.
func extractTrackInfo(entity *entityv1.Entity) (*trackInfo, error) {
	posAny, ok := entity.Components["position"]
	if !ok {
		return nil, fmt.Errorf("no position component on %s", entity.Id)
	}
	pos := &entityv1.PositionComponent{}
	if err := posAny.UnmarshalTo(pos); err != nil {
		return nil, fmt.Errorf("unmarshal position on %s: %w", entity.Id, err)
	}

	srcAny, ok := entity.Components["source"]
	if !ok {
		return nil, fmt.Errorf("no source component on %s", entity.Id)
	}
	src := &entityv1.SourceComponent{}
	if err := srcAny.UnmarshalTo(src); err != nil {
		return nil, fmt.Errorf("unmarshal source on %s: %w", entity.Id, err)
	}

	return &trackInfo{
		entityID: entity.Id,
		lat:      pos.Lat,
		lon:      pos.Lon,
		sensorID: src.SensorId,
	}, nil
}

// Run connects to the store, watches all TRACK entities, and manages fused
// entities until ctx is cancelled.
func (f *Fusioner) Run(ctx context.Context) error {
	conn, err := grpc.NewClient(f.cfg.StoreAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connect to store: %w", err)
	}
	defer conn.Close()

	client := storev1.NewEntityStoreServiceClient(conn)

	stream, err := client.WatchEntities(ctx, &storev1.WatchEntitiesRequest{
		TypeFilter: entityv1.EntityType_ENTITY_TYPE_TRACK,
	})
	if err != nil {
		return fmt.Errorf("watch entities: %w", err)
	}

	slog.Info("fusion service watching tracks", "store_addr", f.cfg.StoreAddr, "dist_threshold", f.cfg.DistThreshold)

	// Track which fused entities currently exist in the store.
	activeFused := make(map[string]bool)

	for {
		event, err := stream.Recv()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("recv: %w", err)
		}

		switch event.Type {
		case storev1.EventType_EVENT_TYPE_DELETED:
			f.RemoveTrack(event.Entity.Id)
		default:
			f.UpdateTrack(event.Entity)
		}

		// Recompute correlations.
		fused := f.BuildFusedEntities()
		newFused := make(map[string]bool)

		for _, ent := range fused {
			newFused[ent.Id] = true
			if activeFused[ent.Id] {
				// Update existing fused entity.
				if _, err := client.UpdateEntity(ctx, &storev1.UpdateEntityRequest{Entity: ent}); err != nil {
					slog.Error("update fused entity", "id", ent.Id, "error", err)
				} else {
					slog.Info("updated fused entity", "id", ent.Id)
				}
			} else {
				// Create new fused entity.
				if _, err := client.CreateEntity(ctx, &storev1.CreateEntityRequest{Entity: ent}); err != nil {
					slog.Error("create fused entity", "id", ent.Id, "error", err)
				} else {
					slog.Info("created fused entity", "id", ent.Id)
				}
			}
		}

		// Delete fused entities that are no longer correlated.
		for id := range activeFused {
			if !newFused[id] {
				if _, err := client.DeleteEntity(ctx, &storev1.DeleteEntityRequest{Id: id}); err != nil {
					slog.Error("delete fused entity", "id", id, "error", err)
				} else {
					slog.Info("deleted fused entity", "id", id)
				}
			}
		}

		activeFused = newFused
	}
}
