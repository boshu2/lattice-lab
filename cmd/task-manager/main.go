package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/boshu2/lattice-lab/internal/task"
)

func main() {
	cfg := task.DefaultConfig()

	if v := os.Getenv("STORE_ADDR"); v != "" {
		cfg.StoreAddr = v
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

	mgr := task.New(cfg)
	if err := mgr.Run(ctx); err != nil {
		log.Fatalf("task-manager: %v", err)
	}
}
