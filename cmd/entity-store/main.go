package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	storev1 "github.com/boshu2/lattice-lab/gen/store/v1"
	"github.com/boshu2/lattice-lab/internal/server"
	"github.com/boshu2/lattice-lab/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "50051"
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := store.New()
	grpcServer := grpc.NewServer()
	storev1.RegisterEntityStoreServiceServer(grpcServer, server.New(s))
	reflection.Register(grpcServer)

	// Graceful shutdown on SIGINT/SIGTERM.
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down...")
		grpcServer.GracefulStop()
	}()

	log.Printf("entity-store listening on :%s", port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
