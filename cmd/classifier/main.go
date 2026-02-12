package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/boshu2/lattice-lab/internal/classifier"
)

func main() {
	cfg := classifier.DefaultConfig()

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

	cl := classifier.New(cfg)
	if err := cl.Run(ctx); err != nil {
		log.Fatalf("classifier: %v", err)
	}
}
