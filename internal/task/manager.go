package task

import (
	"context"
	"fmt"
	"log"
	"sync"

	entityv1 "github.com/boshu2/lattice-lab/gen/entity/v1"
	storev1 "github.com/boshu2/lattice-lab/gen/store/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/anypb"
)

// State represents the current task state for an entity.
type State string

const (
	StateIdle         State = "idle"
	StateInvestigate  State = "investigate"
	StateTrack        State = "track"
	StateIntercept    State = "intercept"
)

// Assignment holds the current task assignment for an entity.
type Assignment struct {
	EntityID string
	State    State
	Tasks    []string
}

// Rules maps threat levels to task assignments.
func Rules(threat entityv1.ThreatLevel) (State, []string) {
	switch threat {
	case entityv1.ThreatLevel_THREAT_LEVEL_NONE:
		return StateIdle, nil
	case entityv1.ThreatLevel_THREAT_LEVEL_LOW:
		return StateInvestigate, []string{"monitor", "identify"}
	case entityv1.ThreatLevel_THREAT_LEVEL_MEDIUM:
		return StateTrack, []string{"monitor", "identify", "track"}
	case entityv1.ThreatLevel_THREAT_LEVEL_HIGH:
		return StateIntercept, []string{"monitor", "identify", "track", "intercept"}
	default:
		return StateIdle, nil
	}
}

// Config controls the task manager.
type Config struct {
	StoreAddr string
}

// DefaultConfig returns task manager defaults.
func DefaultConfig() Config {
	return Config{StoreAddr: "localhost:50051"}
}

// Manager watches classified entities and assigns tasks based on threat level.
type Manager struct {
	cfg         Config
	mu          sync.RWMutex
	assignments map[string]*Assignment
}

// New creates a task manager.
func New(cfg Config) *Manager {
	return &Manager{
		cfg:         cfg,
		assignments: make(map[string]*Assignment),
	}
}

// GetAssignment returns the current assignment for an entity.
func (m *Manager) GetAssignment(entityID string) (*Assignment, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.assignments[entityID]
	return a, ok
}

// Run connects to the store, watches all entities, and manages task assignments.
func (m *Manager) Run(ctx context.Context) error {
	conn, err := grpc.NewClient(m.cfg.StoreAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connect to store: %w", err)
	}
	defer conn.Close()

	client := storev1.NewEntityStoreServiceClient(conn)

	stream, err := client.WatchEntities(ctx, &storev1.WatchEntitiesRequest{
		TypeFilter: entityv1.EntityType_ENTITY_TYPE_TRACK,
	})
	if err != nil {
		return fmt.Errorf("watch entities: %w", err)
	}

	log.Printf("task-manager: watching tracks on %s", m.cfg.StoreAddr)

	for {
		event, err := stream.Recv()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("recv: %w", err)
		}

		switch event.Type {
		case storev1.EventType_EVENT_TYPE_DELETED:
			m.removeAssignment(event.Entity.Id)
		default:
			m.processEntity(ctx, client, event.Entity)
		}
	}
}

func (m *Manager) processEntity(ctx context.Context, client storev1.EntityStoreServiceClient, entity *entityv1.Entity) {
	threat, err := extractThreat(entity)
	if err != nil {
		return // no threat component yet, skip
	}

	state, tasks := Rules(threat)

	m.mu.Lock()
	prev, existed := m.assignments[entity.Id]
	changed := !existed || prev.State != state
	m.assignments[entity.Id] = &Assignment{
		EntityID: entity.Id,
		State:    state,
		Tasks:    tasks,
	}
	m.mu.Unlock()

	if !changed {
		return
	}

	// Update entity with task catalog.
	if len(tasks) > 0 {
		catalog, err := anypb.New(&entityv1.TaskCatalogComponent{
			AvailableTasks: tasks,
		})
		if err != nil {
			log.Printf("pack task catalog for %s: %v", entity.Id, err)
			return
		}
		entity.Components["task_catalog"] = catalog

		if _, err := client.UpdateEntity(ctx, &storev1.UpdateEntityRequest{Entity: entity}); err != nil {
			log.Printf("update task catalog for %s: %v", entity.Id, err)
			return
		}
	}

	log.Printf("task-manager: %s → %s (tasks: %v)", entity.Id, state, tasks)
}

func (m *Manager) removeAssignment(entityID string) {
	m.mu.Lock()
	delete(m.assignments, entityID)
	m.mu.Unlock()
	log.Printf("task-manager: removed assignment for %s", entityID)
}

func extractThreat(entity *entityv1.Entity) (entityv1.ThreatLevel, error) {
	threatAny, ok := entity.Components["threat"]
	if !ok {
		return entityv1.ThreatLevel_THREAT_LEVEL_UNSPECIFIED, fmt.Errorf("no threat component")
	}

	threat := &entityv1.ThreatComponent{}
	if err := threatAny.UnmarshalTo(threat); err != nil {
		return entityv1.ThreatLevel_THREAT_LEVEL_UNSPECIFIED, fmt.Errorf("unmarshal threat: %w", err)
	}

	return threat.Level, nil
}
