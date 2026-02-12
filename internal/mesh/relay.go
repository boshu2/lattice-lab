package mesh

import (
	"context"
	"fmt"
	"log"
	"sync"

	entityv1 "github.com/boshu2/lattice-lab/gen/entity/v1"
	storev1 "github.com/boshu2/lattice-lab/gen/store/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// Config controls the mesh relay.
type Config struct {
	LocalAddr string   // address of the local entity-store
	Peers     []string // addresses of peer entity-stores
}

// DefaultConfig returns mesh relay defaults.
func DefaultConfig() Config {
	return Config{
		LocalAddr: "localhost:50051",
	}
}

// Relay replicates entities between peer entity-stores.
// It watches the local store and forwards events to all peers.
type Relay struct {
	cfg   Config
	mu    sync.RWMutex
	stats Stats
}

// Stats tracks relay activity.
type Stats struct {
	Forwarded int
	Errors    int
}

// New creates a relay with the given config.
func New(cfg Config) *Relay {
	return &Relay{cfg: cfg}
}

// GetStats returns current relay statistics.
func (r *Relay) GetStats() Stats {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.stats
}

// Run watches the local store and replicates events to peers until ctx is cancelled.
func (r *Relay) Run(ctx context.Context) error {
	if len(r.cfg.Peers) == 0 {
		return fmt.Errorf("no peers configured")
	}

	// Connect to local store.
	localConn, err := grpc.NewClient(r.cfg.LocalAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connect to local store: %w", err)
	}
	defer localConn.Close()

	localClient := storev1.NewEntityStoreServiceClient(localConn)

	// Connect to all peers.
	peerClients := make([]storev1.EntityStoreServiceClient, 0, len(r.cfg.Peers))
	var peerConns []*grpc.ClientConn
	for _, addr := range r.cfg.Peers {
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			for _, c := range peerConns {
				c.Close()
			}
			return fmt.Errorf("connect to peer %s: %w", addr, err)
		}
		peerConns = append(peerConns, conn)
		peerClients = append(peerClients, storev1.NewEntityStoreServiceClient(conn))
	}
	defer func() {
		for _, c := range peerConns {
			c.Close()
		}
	}()

	// Watch local store for all entity events.
	stream, err := localClient.WatchEntities(ctx, &storev1.WatchEntitiesRequest{})
	if err != nil {
		return fmt.Errorf("watch local store: %w", err)
	}

	log.Printf("mesh-relay: local=%s, peers=%v", r.cfg.LocalAddr, r.cfg.Peers)

	for {
		event, err := stream.Recv()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("recv: %w", err)
		}

		r.forwardToPeers(ctx, peerClients, event)
	}
}

func (r *Relay) forwardToPeers(ctx context.Context, peers []storev1.EntityStoreServiceClient, event *storev1.EntityEvent) {
	for i, peer := range peers {
		if err := r.forwardEvent(ctx, peer, event); err != nil {
			log.Printf("mesh-relay: forward to peer %d: %v", i, err)
			r.mu.Lock()
			r.stats.Errors++
			r.mu.Unlock()
		} else {
			r.mu.Lock()
			r.stats.Forwarded++
			r.mu.Unlock()
		}
	}
}

func (r *Relay) forwardEvent(ctx context.Context, peer storev1.EntityStoreServiceClient, event *storev1.EntityEvent) error {
	switch event.Type {
	case storev1.EventType_EVENT_TYPE_CREATED:
		return r.forwardCreate(ctx, peer, event.Entity)
	case storev1.EventType_EVENT_TYPE_UPDATED:
		return r.forwardUpdate(ctx, peer, event.Entity)
	case storev1.EventType_EVENT_TYPE_DELETED:
		return r.forwardDelete(ctx, peer, event.Entity.Id)
	default:
		return nil
	}
}

func (r *Relay) forwardCreate(ctx context.Context, peer storev1.EntityStoreServiceClient, entity *entityv1.Entity) error {
	_, err := peer.CreateEntity(ctx, &storev1.CreateEntityRequest{Entity: entity})
	if err != nil {
		// If already exists, try update instead (idempotent replication).
		if status.Code(err) == codes.AlreadyExists {
			_, err = peer.UpdateEntity(ctx, &storev1.UpdateEntityRequest{Entity: entity})
		}
	}
	return err
}

func (r *Relay) forwardUpdate(ctx context.Context, peer storev1.EntityStoreServiceClient, entity *entityv1.Entity) error {
	_, err := peer.UpdateEntity(ctx, &storev1.UpdateEntityRequest{Entity: entity})
	if err != nil {
		// If not found, create instead (peer might not have it yet).
		if status.Code(err) == codes.NotFound {
			_, err = peer.CreateEntity(ctx, &storev1.CreateEntityRequest{Entity: entity})
		}
	}
	return err
}

func (r *Relay) forwardDelete(ctx context.Context, peer storev1.EntityStoreServiceClient, id string) error {
	_, err := peer.DeleteEntity(ctx, &storev1.DeleteEntityRequest{Id: id})
	if err != nil {
		// Not found is fine — already deleted on peer.
		if status.Code(err) == codes.NotFound {
			return nil
		}
	}
	return err
}
