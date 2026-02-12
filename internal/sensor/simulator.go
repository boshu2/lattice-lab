package sensor

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"time"

	entityv1 "github.com/boshu2/lattice-lab/gen/entity/v1"
	storev1 "github.com/boshu2/lattice-lab/gen/store/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	metersPerDegreeLat = 111_320.0
	knotsToMps         = 0.514444
)

// BBox defines a geographic bounding box.
type BBox struct {
	MinLat, MaxLat float64
	MinLon, MaxLon float64
}

// Config controls the sensor simulator.
type Config struct {
	StoreAddr string
	Interval  time.Duration
	NumTracks int
	BBox      BBox
}

// DefaultConfig returns a config with DC metro area defaults.
func DefaultConfig() Config {
	return Config{
		StoreAddr: "localhost:50051",
		Interval:  time.Second,
		NumTracks: 5,
		BBox: BBox{
			MinLat: 38.8, MaxLat: 39.0,
			MinLon: -77.2, MaxLon: -76.9,
		},
	}
}

type track struct {
	id      string
	lat     float64
	lon     float64
	alt     float64
	speed   float64 // m/s
	heading float64 // degrees, 0=north, clockwise
	created bool
}

// Simulator generates Track entities and streams them to an entity store.
type Simulator struct {
	cfg    Config
	tracks []*track
}

// New creates a simulator with the given config.
func New(cfg Config) *Simulator {
	tracks := make([]*track, cfg.NumTracks)
	for i := range tracks {
		tracks[i] = newTrack(i, cfg.BBox)
	}
	return &Simulator{cfg: cfg, tracks: tracks}
}

func newTrack(n int, bbox BBox) *track {
	return &track{
		id:      fmt.Sprintf("track-%d", n),
		lat:     bbox.MinLat + rand.Float64()*(bbox.MaxLat-bbox.MinLat),
		lon:     bbox.MinLon + rand.Float64()*(bbox.MaxLon-bbox.MinLon),
		alt:     rand.Float64()*5000 + 1000, // 1000-6000m
		speed:   (rand.Float64()*400 + 100) * knotsToMps,
		heading: rand.Float64() * 360,
	}
}

// Run connects to the entity store and streams track updates until ctx is cancelled.
func (s *Simulator) Run(ctx context.Context) error {
	conn, err := grpc.NewClient(s.cfg.StoreAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connect to store: %w", err)
	}
	defer conn.Close()

	client := storev1.NewEntityStoreServiceClient(conn)
	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()

	slog.Info("sensor-sim started", "num_tracks", s.cfg.NumTracks, "interval", s.cfg.Interval, "store_addr", s.cfg.StoreAddr)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			for _, t := range s.tracks {
				if err := s.tick(ctx, client, t); err != nil {
					slog.Error("tick failed", "track_id", t.id, "error", err)
				}
			}
		}
	}
}

func (s *Simulator) tick(ctx context.Context, client storev1.EntityStoreServiceClient, t *track) error {
	if !t.created {
		return s.createTrack(ctx, client, t)
	}
	advanceTrack(t, s.cfg.Interval)
	return s.updateTrack(ctx, client, t)
}

func (s *Simulator) createTrack(ctx context.Context, client storev1.EntityStoreServiceClient, t *track) error {
	entity, err := buildEntity(t)
	if err != nil {
		return err
	}
	if _, err := client.CreateEntity(ctx, &storev1.CreateEntityRequest{Entity: entity}); err != nil {
		return fmt.Errorf("create %s: %w", t.id, err)
	}
	t.created = true
	slog.Info("created track", "track_id", t.id, "lat", t.lat, "lon", t.lon, "speed_kts", t.speed/knotsToMps, "heading_deg", t.heading)
	return nil
}

func (s *Simulator) updateTrack(ctx context.Context, client storev1.EntityStoreServiceClient, t *track) error {
	entity, err := buildEntity(t)
	if err != nil {
		return err
	}
	if _, err := client.UpdateEntity(ctx, &storev1.UpdateEntityRequest{Entity: entity}); err != nil {
		return fmt.Errorf("update %s: %w", t.id, err)
	}
	slog.Info("updated track", "track_id", t.id, "lat", t.lat, "lon", t.lon, "speed_kts", t.speed/knotsToMps, "heading_deg", t.heading)
	return nil
}

func buildEntity(t *track) (*entityv1.Entity, error) {
	pos, err := anypb.New(&entityv1.PositionComponent{
		Lat: t.lat,
		Lon: t.lon,
		Alt: t.alt,
	})
	if err != nil {
		return nil, fmt.Errorf("pack position: %w", err)
	}

	vel, err := anypb.New(&entityv1.VelocityComponent{
		Speed:   t.speed / knotsToMps, // store as knots
		Heading: t.heading,
	})
	if err != nil {
		return nil, fmt.Errorf("pack velocity: %w", err)
	}

	src, err := anypb.New(&entityv1.SourceComponent{
		SensorId:   "eo-1",
		SensorType: "eo",
	})
	if err != nil {
		return nil, fmt.Errorf("pack source: %w", err)
	}

	return &entityv1.Entity{
		Id:   t.id,
		Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
		Components: map[string]*anypb.Any{
			"position": pos,
			"velocity": vel,
			"source":   src,
		},
	}, nil
}

// advanceTrack updates position using dead-reckoning (flat-earth approximation).
func advanceTrack(t *track, dt time.Duration) {
	hdgRad := t.heading * math.Pi / 180
	ds := t.speed * dt.Seconds()

	t.lat += (ds * math.Cos(hdgRad)) / metersPerDegreeLat
	t.lon += (ds * math.Sin(hdgRad)) / (metersPerDegreeLat * math.Cos(t.lat*math.Pi/180))
}
