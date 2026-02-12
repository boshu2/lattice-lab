package store

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	entityv1 "github.com/boshu2/lattice-lab/gen/entity/v1"
	storev1 "github.com/boshu2/lattice-lab/gen/store/v1"
	"github.com/boshu2/lattice-lab/internal/hlc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Watcher receives entity events via a channel.
type Watcher struct {
	Filter entityv1.EntityType
	Events chan *storev1.EntityEvent
}

// Store is a thread-safe in-memory entity store.
type Store struct {
	mu       sync.RWMutex
	entities map[string]*entityv1.Entity
	ttls     map[string]time.Time // entity ID → expiry time
	clock    *hlc.Clock

	watchMu  sync.RWMutex
	watchers []*Watcher
}

// Option configures a Store.
type Option func(*Store)

// WithNodeID sets the HLC node identifier for this store instance.
func WithNodeID(id string) Option {
	return func(s *Store) { s.clock = hlc.NewClock(id) }
}

// New creates an empty entity store. Options can configure the HLC node ID;
// if none is provided a random node ID is generated.
func New(opts ...Option) *Store {
	s := &Store{
		entities: make(map[string]*entityv1.Entity),
		ttls:     make(map[string]time.Time),
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.clock == nil {
		s.clock = hlc.NewClock(fmt.Sprintf("node-%d", rand.Int63()))
	}
	return s
}

// SetTTL sets a time-to-live for an entity. The entity will be automatically
// deleted after the TTL expires (requires StartReaper to be running).
func (s *Store) SetTTL(id string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ttls[id] = time.Now().Add(ttl)
}

// StartReaper runs a background goroutine that deletes expired entities.
// It stops when ctx is cancelled.
func (s *Store) StartReaper(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reap()
		}
	}
}

func (s *Store) reap() {
	now := time.Now()

	s.mu.Lock()
	var expired []string
	for id, expiry := range s.ttls {
		if now.After(expiry) {
			expired = append(expired, id)
		}
	}
	s.mu.Unlock()

	for _, id := range expired {
		s.Delete(id) //nolint:errcheck
		s.mu.Lock()
		delete(s.ttls, id)
		s.mu.Unlock()
	}
}

// Create adds a new entity. Returns an error if the ID already exists.
func (s *Store) Create(e *entityv1.Entity) (*entityv1.Entity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.entities[e.Id]; exists {
		return nil, fmt.Errorf("entity %q already exists", e.Id)
	}

	now := timestamppb.Now()
	ts := s.clock.Now()
	stored := proto.Clone(e).(*entityv1.Entity)
	stored.CreatedAt = now
	stored.UpdatedAt = now
	stored.HlcPhysical = ts.Physical
	stored.HlcLogical = ts.Logical
	stored.HlcNode = ts.Node
	s.entities[stored.Id] = stored

	s.notify(&storev1.EntityEvent{
		Type:   storev1.EventType_EVENT_TYPE_CREATED,
		Entity: proto.Clone(stored).(*entityv1.Entity),
	})
	return proto.Clone(stored).(*entityv1.Entity), nil
}

// Get returns an entity by ID.
func (s *Store) Get(id string) (*entityv1.Entity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	e, ok := s.entities[id]
	if !ok {
		return nil, fmt.Errorf("entity %q not found", id)
	}
	return proto.Clone(e).(*entityv1.Entity), nil
}

// List returns all entities, optionally filtered by type.
func (s *Store) List(typeFilter entityv1.EntityType) []*entityv1.Entity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*entityv1.Entity
	for _, e := range s.entities {
		if typeFilter != entityv1.EntityType_ENTITY_TYPE_UNSPECIFIED && e.Type != typeFilter {
			continue
		}
		result = append(result, proto.Clone(e).(*entityv1.Entity))
	}
	return result
}

// Update replaces an existing entity. Returns error if not found.
func (s *Store) Update(e *entityv1.Entity) (*entityv1.Entity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.entities[e.Id]
	if !ok {
		return nil, fmt.Errorf("entity %q not found", e.Id)
	}

	// Advance the store's HLC.
	ts := s.clock.Now()

	// Component-key merge: start from existing entity, merge incoming components.
	merged := proto.Clone(existing).(*entityv1.Entity)

	incomingHLC := hlc.Timestamp{Physical: e.HlcPhysical, Logical: e.HlcLogical, Node: e.HlcNode}
	existingHLC := hlc.Timestamp{Physical: existing.HlcPhysical, Logical: existing.HlcLogical, Node: existing.HlcNode}

	if merged.Components == nil {
		merged.Components = make(map[string]*anypb.Any)
	}
	for key, comp := range e.Components {
		if _, exists := merged.Components[key]; !exists {
			// New key from incoming — always accept.
			merged.Components[key] = comp
		} else if hlc.Compare(incomingHLC, existingHLC) >= 0 {
			// Same key, incoming is newer or equal — accept.
			merged.Components[key] = comp
		}
		// Else: same key, incoming is stale — keep existing.
	}

	// Copy non-component fields from incoming where appropriate.
	merged.Type = e.Type
	merged.UpdatedAt = timestamppb.Now()
	merged.HlcPhysical = ts.Physical
	merged.HlcLogical = ts.Logical
	merged.HlcNode = ts.Node
	s.entities[merged.Id] = merged

	s.notify(&storev1.EntityEvent{
		Type:   storev1.EventType_EVENT_TYPE_UPDATED,
		Entity: proto.Clone(merged).(*entityv1.Entity),
	})
	return proto.Clone(merged).(*entityv1.Entity), nil
}

// Delete removes an entity by ID. Returns error if not found.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.entities[id]
	if !ok {
		return fmt.Errorf("entity %q not found", id)
	}

	delete(s.entities, id)

	s.notify(&storev1.EntityEvent{
		Type:   storev1.EventType_EVENT_TYPE_DELETED,
		Entity: proto.Clone(e).(*entityv1.Entity),
	})
	return nil
}

// Watch registers a watcher that receives entity events.
// Close the returned channel when done watching.
func (s *Store) Watch(typeFilter entityv1.EntityType) *Watcher {
	w := &Watcher{
		Filter: typeFilter,
		Events: make(chan *storev1.EntityEvent, 64),
	}
	s.watchMu.Lock()
	s.watchers = append(s.watchers, w)
	s.watchMu.Unlock()
	return w
}

// Unwatch removes a watcher and closes its channel.
func (s *Store) Unwatch(w *Watcher) {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()

	for i, existing := range s.watchers {
		if existing == w {
			s.watchers = append(s.watchers[:i], s.watchers[i+1:]...)
			close(w.Events)
			return
		}
	}
}

// notify sends an event to all matching watchers. Must NOT hold watchMu.
func (s *Store) notify(event *storev1.EntityEvent) {
	s.watchMu.RLock()
	defer s.watchMu.RUnlock()

	for _, w := range s.watchers {
		if w.Filter != entityv1.EntityType_ENTITY_TYPE_UNSPECIFIED && w.Filter != event.Entity.Type {
			continue
		}
		select {
		case w.Events <- event:
		default:
			// Drop if watcher is slow — prevent blocking the store.
		}
	}
}
