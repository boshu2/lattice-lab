package main

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"os"
	"os/signal"
	"strconv"
	"syscall"
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
	jitterDeg          = 0.002 // ±0.002 degrees per update
)

type config struct {
	storeAddr string
	interval  time.Duration
	numTracks int
	sensorID  string
	bbox      bbox
}

type bbox struct {
	minLat, maxLat float64
	minLon, maxLon float64
}

type track struct {
	id      string
	lat     float64
	lon     float64
	alt     float64
	speed   float64 // m/s
	heading float64 // degrees
	created bool
}

func defaultConfig() config {
	return config{
		storeAddr: "localhost:50051",
		interval:  2 * time.Second,
		numTracks: 3,
		sensorID:  "radar-1",
		bbox: bbox{
			minLat: 38.8, maxLat: 39.0,
			minLon: -77.2, maxLon: -76.9,
		},
	}
}

func main() {
	cfg := defaultConfig()

	if v := os.Getenv("STORE_ADDR"); v != "" {
		cfg.storeAddr = v
	}
	if v := os.Getenv("INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			slog.Error("invalid INTERVAL", "value", v, "error", err)
			os.Exit(1)
		}
		cfg.interval = d
	}
	if v := os.Getenv("NUM_TRACKS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			slog.Error("invalid NUM_TRACKS", "value", v, "error", err)
			os.Exit(1)
		}
		cfg.numTracks = n
	}
	if v := os.Getenv("SENSOR_ID"); v != "" {
		cfg.sensorID = v
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("shutting down")
		cancel()
	}()

	if err := run(ctx, cfg); err != nil {
		slog.Error("radar-sim failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config) error {
	conn, err := grpc.NewClient(cfg.storeAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connect to store: %w", err)
	}
	defer conn.Close()

	client := storev1.NewEntityStoreServiceClient(conn)

	tracks := make([]*track, cfg.numTracks)
	for i := range tracks {
		tracks[i] = newTrack(i, cfg.bbox)
	}

	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()

	slog.Info("radar-sim started",
		"num_tracks", cfg.numTracks,
		"interval", cfg.interval,
		"store_addr", cfg.storeAddr,
		"sensor_id", cfg.sensorID,
	)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			for _, t := range tracks {
				if err := tick(ctx, client, t, cfg.sensorID); err != nil {
					slog.Error("tick failed", "track_id", t.id, "error", err)
				}
			}
		}
	}
}

func newTrack(n int, bb bbox) *track {
	return &track{
		id:      fmt.Sprintf("radar-track-%d", n),
		lat:     bb.minLat + rand.Float64()*(bb.maxLat-bb.minLat),
		lon:     bb.minLon + rand.Float64()*(bb.maxLon-bb.minLon),
		alt:     rand.Float64()*5000 + 1000,
		speed:   (rand.Float64()*400 + 100) * knotsToMps,
		heading: rand.Float64() * 360,
	}
}

func tick(ctx context.Context, client storev1.EntityStoreServiceClient, t *track, sensorID string) error {
	if !t.created {
		return createTrack(ctx, client, t, sensorID)
	}
	advanceTrack(t)
	addJitter(t)
	return updateTrack(ctx, client, t, sensorID)
}

func createTrack(ctx context.Context, client storev1.EntityStoreServiceClient, t *track, sensorID string) error {
	entity, err := buildEntity(t, sensorID)
	if err != nil {
		return err
	}
	if _, err := client.CreateEntity(ctx, &storev1.CreateEntityRequest{Entity: entity}); err != nil {
		return fmt.Errorf("create %s: %w", t.id, err)
	}
	t.created = true
	slog.Info("created radar track", "track_id", t.id, "lat", t.lat, "lon", t.lon)
	return nil
}

func updateTrack(ctx context.Context, client storev1.EntityStoreServiceClient, t *track, sensorID string) error {
	entity, err := buildEntity(t, sensorID)
	if err != nil {
		return err
	}
	if _, err := client.UpdateEntity(ctx, &storev1.UpdateEntityRequest{Entity: entity}); err != nil {
		return fmt.Errorf("update %s: %w", t.id, err)
	}
	slog.Info("updated radar track", "track_id", t.id, "lat", t.lat, "lon", t.lon)
	return nil
}

func buildEntity(t *track, sensorID string) (*entityv1.Entity, error) {
	pos, err := anypb.New(&entityv1.PositionComponent{
		Lat: t.lat,
		Lon: t.lon,
		Alt: t.alt,
	})
	if err != nil {
		return nil, fmt.Errorf("pack position: %w", err)
	}

	src, err := anypb.New(&entityv1.SourceComponent{
		SensorId:   sensorID,
		SensorType: "radar",
	})
	if err != nil {
		return nil, fmt.Errorf("pack source: %w", err)
	}

	return &entityv1.Entity{
		Id:   t.id,
		Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
		Components: map[string]*anypb.Any{
			"position": pos,
			"source":   src,
		},
	}, nil
}

func advanceTrack(t *track) {
	dt := 2 * time.Second // matches default interval
	hdgRad := t.heading * math.Pi / 180
	ds := t.speed * dt.Seconds()

	t.lat += (ds * math.Cos(hdgRad)) / metersPerDegreeLat
	t.lon += (ds * math.Sin(hdgRad)) / (metersPerDegreeLat * math.Cos(t.lat*math.Pi/180))
}

func addJitter(t *track) {
	t.lat += (rand.Float64()*2 - 1) * jitterDeg
	t.lon += (rand.Float64()*2 - 1) * jitterDeg
}
