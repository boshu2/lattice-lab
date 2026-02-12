package main

import (
	"context"
	"log"
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
			log.Fatalf("invalid INTERVAL %q: %v", v, err)
		}
		cfg.Interval = d
	}
	if v := os.Getenv("NUM_TRACKS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			log.Fatalf("invalid NUM_TRACKS %q: %v", v, err)
		}
		cfg.NumTracks = n
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down...")
		cancel()
	}()

	sim := sensor.New(cfg)
	if err := sim.Run(ctx); err != nil {
		log.Fatalf("sensor-sim: %v", err)
	}
}
