package task

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	entityv1 "github.com/boshu2/lattice-lab/gen/entity/v1"
	storev1 "github.com/boshu2/lattice-lab/gen/store/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/anypb"
)

// State represents the current task state for an entity.
type State string

const (
	StateIdle            State = "idle"
	StateInvestigate     State = "investigate"
	StateTrack           State = "track"
	StateIntercept       State = "intercept"
	StatePendingApproval State = "pending_approval"
)

// Assignment holds the current task assignment for an entity.
type Assignment struct {
	EntityID       string
	State          State
	Tasks          []string
	catalogWritten bool // tracks whether the task catalog was pushed to the store
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

// pendingApproval tracks an entity awaiting operator approval.
type pendingApproval struct {
	entityID string
	cancel   context.CancelFunc
	state    State
	tasks    []string
}

// Config controls the task manager.
type Config struct {
	StoreAddr       string
	ApprovalTimeout time.Duration
}

// DefaultConfig returns task manager defaults.
func DefaultConfig() Config {
	return Config{
		StoreAddr:       "localhost:50051",
		ApprovalTimeout: 30 * time.Second,
	}
}

// Manager watches classified entities and assigns tasks based on threat level.
type Manager struct {
	cfg         Config
	mu          sync.RWMutex
	assignments map[string]*Assignment
	pending     map[string]*pendingApproval

	// Set during Run() for use by Approve to push catalog updates.
	runCtx context.Context
	client storev1.EntityStoreServiceClient
}

// New creates a task manager.
func New(cfg Config) *Manager {
	if cfg.ApprovalTimeout == 0 {
		cfg.ApprovalTimeout = 30 * time.Second
	}
	return &Manager{
		cfg:         cfg,
		assignments: make(map[string]*Assignment),
		pending:     make(map[string]*pendingApproval),
	}
}

// GetAssignment returns the current assignment for an entity.
func (m *Manager) GetAssignment(entityID string) (*Assignment, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.assignments[entityID]
	return a, ok
}

// Approve transitions a pending entity to its approved state with tasks.
// It also pushes the task catalog to the entity store if the manager is running.
func (m *Manager) Approve(entityID string) (*Assignment, error) {
	m.mu.Lock()

	p, ok := m.pending[entityID]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("no pending approval for %s", entityID)
	}

	p.cancel() // stop timeout
	delete(m.pending, entityID)

	a := &Assignment{EntityID: entityID, State: p.state, Tasks: p.tasks, catalogWritten: true}
	m.assignments[entityID] = a

	// Capture client/ctx for catalog write outside lock.
	client := m.client
	ctx := m.runCtx
	m.mu.Unlock()

	slog.Info("task-manager approved", "entity_id", entityID, "state", p.state)

	// Push task catalog to the entity store.
	if client != nil && ctx != nil && len(p.tasks) > 0 {
		go m.pushCatalogForEntity(ctx, client, entityID, p.tasks)
	}

	return a, nil
}

// Deny rejects a pending approval, returning the entity to idle with no tasks.
func (m *Manager) Deny(entityID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.pending[entityID]
	if !ok {
		return fmt.Errorf("no pending approval for %s", entityID)
	}

	p.cancel()
	delete(m.pending, entityID)
	m.assignments[entityID] = &Assignment{EntityID: entityID, State: StateIdle}
	slog.Info("task-manager denied", "entity_id", entityID)
	return nil
}

// Run connects to the store, watches all entities, and manages task assignments.
func (m *Manager) Run(ctx context.Context) error {
	conn, err := grpc.NewClient(m.cfg.StoreAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connect to store: %w", err)
	}
	defer conn.Close()

	client := storev1.NewEntityStoreServiceClient(conn)

	m.mu.Lock()
	m.runCtx = ctx
	m.client = client
	m.mu.Unlock()

	stream, err := client.WatchEntities(ctx, &storev1.WatchEntitiesRequest{
		TypeFilter: entityv1.EntityType_ENTITY_TYPE_TRACK,
	})
	if err != nil {
		return fmt.Errorf("watch entities: %w", err)
	}

	slog.Info("task-manager watching tracks", "store_addr", m.cfg.StoreAddr)

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

	// HIGH threat requires approval gate.
	if state == StateIntercept {
		m.mu.Lock()
		prev, ok := m.assignments[entity.Id]

		// If already approved and assigned intercept, check if we need to
		// push the task catalog to the store (happens on first event after approval).
		if ok && prev.State == StateIntercept {
			needsCatalog := !prev.catalogWritten
			if needsCatalog {
				prev.catalogWritten = true
			}
			m.mu.Unlock()
			if needsCatalog {
				m.writeTaskCatalog(ctx, client, entity, tasks)
			}
			return
		}

		// If already pending, skip (don't re-enter pending on re-watch).
		if _, pending := m.pending[entity.Id]; pending {
			m.mu.Unlock()
			return
		}

		// Set pending approval.
		m.assignments[entity.Id] = &Assignment{
			EntityID: entity.Id,
			State:    StatePendingApproval,
			Tasks:    nil,
		}

		// Start timeout.
		timerCtx, cancel := context.WithCancel(context.Background())
		m.pending[entity.Id] = &pendingApproval{
			entityID: entity.Id,
			cancel:   cancel,
			state:    state,
			tasks:    tasks,
		}
		m.mu.Unlock()

		go m.approvalTimer(timerCtx, entity.Id)

		slog.Info("task-manager pending approval", "entity_id", entity.Id, "state", state)
		return
	}

	// Non-HIGH threats: assign directly (unchanged behavior).
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

	m.writeTaskCatalog(ctx, client, entity, tasks)
}

// pushCatalogForEntity fetches the entity from the store and writes the task catalog.
func (m *Manager) pushCatalogForEntity(ctx context.Context, client storev1.EntityStoreServiceClient, entityID string, tasks []string) {
	entity, err := client.GetEntity(ctx, &storev1.GetEntityRequest{Id: entityID})
	if err != nil {
		slog.Error("fetch entity for catalog push failed", "entity_id", entityID, "error", err)
		return
	}
	m.writeTaskCatalog(ctx, client, entity, tasks)
}

func (m *Manager) writeTaskCatalog(ctx context.Context, client storev1.EntityStoreServiceClient, entity *entityv1.Entity, tasks []string) {
	if len(tasks) == 0 {
		return
	}

	catalog, err := anypb.New(&entityv1.TaskCatalogComponent{
		AvailableTasks: tasks,
	})
	if err != nil {
		slog.Error("pack task catalog failed", "entity_id", entity.Id, "error", err)
		return
	}
	entity.Components["task_catalog"] = catalog

	if _, err := client.UpdateEntity(ctx, &storev1.UpdateEntityRequest{Entity: entity}); err != nil {
		slog.Error("update task catalog failed", "entity_id", entity.Id, "error", err)
		return
	}

	slog.Info("task-manager assigned tasks", "entity_id", entity.Id, "tasks", tasks)
}

func (m *Manager) approvalTimer(ctx context.Context, entityID string) {
	select {
	case <-ctx.Done():
		return // cancelled by approve/deny/delete
	case <-time.After(m.cfg.ApprovalTimeout):
		m.mu.Lock()
		if _, ok := m.pending[entityID]; ok {
			delete(m.pending, entityID)
			m.assignments[entityID] = &Assignment{EntityID: entityID, State: StateIdle}
			slog.Info("approval timed out, auto-denied", "entity_id", entityID)
		}
		m.mu.Unlock()
	}
}

func (m *Manager) removeAssignment(entityID string) {
	m.mu.Lock()
	if p, ok := m.pending[entityID]; ok {
		p.cancel()
		delete(m.pending, entityID)
	}
	delete(m.assignments, entityID)
	m.mu.Unlock()
	slog.Info("task-manager removed assignment", "entity_id", entityID)
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
