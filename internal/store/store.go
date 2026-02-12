package store

import (
	"fmt"
	"sync"

	entityv1 "github.com/boshu2/lattice-lab/gen/entity/v1"
	storev1 "github.com/boshu2/lattice-lab/gen/store/v1"
	"google.golang.org/protobuf/proto"
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

	watchMu  sync.RWMutex
	watchers []*Watcher
}

// New creates an empty entity store.
func New() *Store {
	return &Store{
		entities: make(map[string]*entityv1.Entity),
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
	stored := proto.Clone(e).(*entityv1.Entity)
	stored.CreatedAt = now
	stored.UpdatedAt = now
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

	stored := proto.Clone(e).(*entityv1.Entity)
	stored.CreatedAt = existing.CreatedAt
	stored.UpdatedAt = timestamppb.Now()
	s.entities[stored.Id] = stored

	s.notify(&storev1.EntityEvent{
		Type:   storev1.EventType_EVENT_TYPE_UPDATED,
		Entity: proto.Clone(stored).(*entityv1.Entity),
	})
	return proto.Clone(stored).(*entityv1.Entity), nil
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
