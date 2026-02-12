package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/boshu2/lattice-lab/internal/sensor"
)

func main() {
	cfg := sensor.DefaultConfig()

	if v := os.Getenv("STORE_ADDR"); v != "" {
		cfg.StoreAddr = v
	}
	if v := os.Getenv("INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			slog.Error("invalid INTERVAL", "value", v, "error", err)
			os.Exit(1)
		}
		cfg.Interval = d
	}
	if v := os.Getenv("NUM_TRACKS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			slog.Error("invalid NUM_TRACKS", "value", v, "error", err)
			os.Exit(1)
		}
		cfg.NumTracks = n
	}
	if v := os.Getenv("BBOX_MIN_LAT"); v != "" {
		cfg.BBox.MinLat, _ = strconv.ParseFloat(v, 64)
	}
	if v := os.Getenv("BBOX_MAX_LAT"); v != "" {
		cfg.BBox.MaxLat, _ = strconv.ParseFloat(v, 64)
	}
	if v := os.Getenv("BBOX_MIN_LON"); v != "" {
		cfg.BBox.MinLon, _ = strconv.ParseFloat(v, 64)
	}
	if v := os.Getenv("BBOX_MAX_LON"); v != "" {
		cfg.BBox.MaxLon, _ = strconv.ParseFloat(v, 64)
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

	sim := sensor.New(cfg)
	if err := sim.Run(ctx); err != nil {
		slog.Error("sensor-sim failed", "error", err)
		os.Exit(1)
	}
}
