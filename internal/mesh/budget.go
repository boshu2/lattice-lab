package mesh

import (
	"sort"
	"sync"
	"time"

	entityv1 "github.com/boshu2/lattice-lab/gen/entity/v1"
	storev1 "github.com/boshu2/lattice-lab/gen/store/v1"
)

// Priority constants for event ordering. Higher value = higher priority.
const (
	PriorityNone   = 0
	PriorityLow    = 1
	PriorityMedium = 2
	PriorityHigh   = 3
	PriorityDelete = 4
)

// TokenBucket implements a token-bucket rate limiter measured in bytes.
type TokenBucket struct {
	mu        sync.Mutex
	tokens    float64
	maxTokens float64
	rate      float64   // bytes per second
	lastTime  time.Time
}

// NewTokenBucket creates a token bucket with the given fill rate and burst capacity.
func NewTokenBucket(bytesPerSec, burstBytes float64) *TokenBucket {
	return &TokenBucket{
		tokens:    burstBytes,
		maxTokens: burstBytes,
		rate:      bytesPerSec,
		lastTime:  time.Now(),
	}
}

// Allow checks whether the given number of bytes can be consumed.
// Events with priority >= PriorityHigh always bypass the budget check.
func (tb *TokenBucket) Allow(bytes int, priority int) bool {
	if priority >= PriorityHigh {
		return true
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastTime).Seconds()
	tb.tokens += elapsed * tb.rate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastTime = now

	cost := float64(bytes)
	if cost > tb.tokens {
		return false
	}
	tb.tokens -= cost
	return true
}

// EventPriority returns the priority of an entity event based on its type
// and threat component. DELETE events get the highest priority.
func EventPriority(event *storev1.EntityEvent) int {
	if event.Type == storev1.EventType_EVENT_TYPE_DELETED {
		return PriorityDelete
	}

	entity := event.Entity
	if entity == nil || entity.Components == nil {
		return PriorityNone
	}

	threatAny, ok := entity.Components["threat"]
	if !ok {
		return PriorityNone
	}

	threat := &entityv1.ThreatComponent{}
	if err := threatAny.UnmarshalTo(threat); err != nil {
		return PriorityNone
	}

	switch threat.Level {
	case entityv1.ThreatLevel_THREAT_LEVEL_HIGH:
		return PriorityHigh
	case entityv1.ThreatLevel_THREAT_LEVEL_MEDIUM:
		return PriorityMedium
	case entityv1.ThreatLevel_THREAT_LEVEL_LOW:
		return PriorityLow
	default:
		return PriorityNone
	}
}

// Coalescer deduplicates entity events, keeping only the latest event per entity.
// DELETE events are never coalesced away.
type Coalescer struct {
	mu     sync.Mutex
	events map[string]*storev1.EntityEvent // entityID -> latest non-delete event
	deletes []*storev1.EntityEvent          // delete events (never coalesced)
	order  []string                         // insertion order for fairness
}

// NewCoalescer creates an empty event coalescer.
func NewCoalescer() *Coalescer {
	return &Coalescer{
		events: make(map[string]*storev1.EntityEvent),
	}
}

// Add queues an event. If the same entityID already exists and the event
// is not a DELETE, the older event is replaced with the latest.
// DELETE events are always preserved (never coalesced).
func (c *Coalescer) Add(event *storev1.EntityEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if event.Type == storev1.EventType_EVENT_TYPE_DELETED {
		c.deletes = append(c.deletes, event)
		return
	}

	id := event.Entity.Id
	if _, exists := c.events[id]; !exists {
		c.order = append(c.order, id)
	}
	c.events[id] = event
}

// Drain returns all queued events sorted by priority (highest first) and clears the queue.
func (c *Coalescer) Drain() []*storev1.EntityEvent {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := make([]*storev1.EntityEvent, 0, len(c.events)+len(c.deletes))

	// Collect non-delete events in insertion order.
	for _, id := range c.order {
		if ev, ok := c.events[id]; ok {
			result = append(result, ev)
		}
	}

	// Append delete events.
	result = append(result, c.deletes...)

	// Sort by priority, highest first.
	sort.Slice(result, func(i, j int) bool {
		return EventPriority(result[i]) > EventPriority(result[j])
	})

	// Clear the queue.
	c.events = make(map[string]*storev1.EntityEvent)
	c.deletes = nil
	c.order = nil

	return result
}
