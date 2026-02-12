package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/boshu2/lattice-lab/internal/fusion"
)

func main() {
	cfg := fusion.DefaultConfig()

	if v := os.Getenv("STORE_ADDR"); v != "" {
		cfg.StoreAddr = v
	}
	if v := os.Getenv("DIST_THRESHOLD"); v != "" {
		d, err := strconv.ParseFloat(v, 64)
		if err != nil {
			slog.Error("invalid DIST_THRESHOLD", "value", v, "error", err)
			os.Exit(1)
		}
		cfg.DistThreshold = d
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

	f := fusion.New(cfg)
	if err := f.Run(ctx); err != nil {
		slog.Error("fusion service failed", "error", err)
		os.Exit(1)
	}
}
