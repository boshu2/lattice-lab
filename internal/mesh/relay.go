package mesh

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	entityv1 "github.com/boshu2/lattice-lab/gen/entity/v1"
	storev1 "github.com/boshu2/lattice-lab/gen/store/v1"
	"github.com/boshu2/lattice-lab/internal/crdt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// Config controls the mesh relay.
type Config struct {
	LocalAddr    string   // address of the local entity-store
	Peers        []string // addresses of peer entity-stores
	NodeID       string   // for echo suppression — skip events originating from this node
	BandwidthBPS float64  // bytes per second budget; 0 = unlimited (default)
	BurstBytes   float64  // burst capacity; 0 = use BandwidthBPS as burst
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
	cfg    Config
	mu     sync.RWMutex
	stats  Stats
	bucket *TokenBucket // nil when BandwidthBPS == 0 (unlimited)
}

// Stats tracks relay activity.
type Stats struct {
	Forwarded int
	Errors    int
	Merged    int // entities that required CRDT merge
	Dropped   int // events dropped by bandwidth budget
}

// New creates a relay with the given config.
func New(cfg Config) *Relay {
	r := &Relay{cfg: cfg}
	if cfg.BandwidthBPS > 0 {
		burst := cfg.BurstBytes
		if burst == 0 {
			burst = cfg.BandwidthBPS
		}
		r.bucket = NewTokenBucket(cfg.BandwidthBPS, burst)
	}
	return r
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

	slog.Info("mesh-relay started", "local", r.cfg.LocalAddr, "peers", r.cfg.Peers)

	for {
		event, err := stream.Recv()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("recv: %w", err)
		}

		// Echo suppression: skip events that originated from this node.
		if r.cfg.NodeID != "" && event.OriginNode == r.cfg.NodeID {
			continue
		}

		r.forwardToPeers(ctx, peerClients, event)
	}
}

func (r *Relay) forwardToPeers(ctx context.Context, peers []storev1.EntityStoreServiceClient, event *storev1.EntityEvent) {
	// Echo suppression: skip events that originated from this node.
	if r.cfg.NodeID != "" && event.OriginNode == r.cfg.NodeID {
		return
	}

	// Budget check: if a token bucket is configured, check the budget.
	if r.bucket != nil {
		size := 0
		if event.Entity != nil {
			size = proto.Size(event.Entity)
		}
		priority := EventPriority(event)
		if !r.bucket.Allow(size, priority) {
			r.mu.Lock()
			r.stats.Dropped++
			r.mu.Unlock()
			slog.Debug("mesh-relay budget drop", "entity", event.Entity.GetId(), "priority", priority, "size", size)
			return
		}
	}

	for i, peer := range peers {
		if err := r.forwardEvent(ctx, peer, event); err != nil {
			slog.Error("mesh-relay forward failed", "peer_index", i, "error", err)
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
	entity := event.Entity

	switch event.Type {
	case storev1.EventType_EVENT_TYPE_CREATED:
		// Try create first.
		_, err := peer.CreateEntity(ctx, &storev1.CreateEntityRequest{Entity: entity})
		if err != nil {
			if status.Code(err) == codes.AlreadyExists {
				// Entity exists on peer — merge.
				return r.mergeAndUpdate(ctx, peer, entity)
			}
			return err
		}
		return nil

	case storev1.EventType_EVENT_TYPE_UPDATED:
		// Always merge for updates.
		return r.mergeAndUpdate(ctx, peer, entity)

	case storev1.EventType_EVENT_TYPE_DELETED:
		// Delete, ignore NotFound.
		_, err := peer.DeleteEntity(ctx, &storev1.DeleteEntityRequest{Id: entity.Id})
		if err != nil && status.Code(err) != codes.NotFound {
			return err
		}
		return nil

	default:
		return nil
	}
}

// mergeAndUpdate fetches the existing entity from the peer, merges it with the
// incoming entity using CRDT strategies, and writes the merged result back.
func (r *Relay) mergeAndUpdate(ctx context.Context, peer storev1.EntityStoreServiceClient, incoming *entityv1.Entity) error {
	// GET current from peer.
	existing, err := peer.GetEntity(ctx, &storev1.GetEntityRequest{Id: incoming.Id})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			// Peer doesn't have it — create.
			_, createErr := peer.CreateEntity(ctx, &storev1.CreateEntityRequest{Entity: incoming})
			return createErr
		}
		return err
	}

	// MERGE using CRDT strategies (LWW per-component, max-wins for threat).
	merged := crdt.MergeEntity(existing, incoming)
	merged.Id = incoming.Id
	merged.Type = incoming.Type
	merged.CreatedAt = existing.CreatedAt

	// PUT merged result.
	_, err = peer.UpdateEntity(ctx, &storev1.UpdateEntityRequest{Entity: merged})
	if err != nil {
		return err
	}

	r.mu.Lock()
	r.stats.Merged++
	r.mu.Unlock()

	return nil
}
