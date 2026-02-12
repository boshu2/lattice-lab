package mesh

import (
	"sort"
	"testing"
	"time"

	entityv1 "github.com/boshu2/lattice-lab/gen/entity/v1"
	storev1 "github.com/boshu2/lattice-lab/gen/store/v1"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestTokenBucket_AllowsWithinRate(t *testing.T) {
	tb := NewTokenBucket(1000, 1000) // 1000 bytes/sec, 1000 burst
	if !tb.Allow(500, PriorityNone) {
		t.Fatal("expected 500 bytes to be allowed within 1000 burst")
	}
}

func TestTokenBucket_RejectsOverBudget(t *testing.T) {
	tb := NewTokenBucket(100, 100) // 100 bytes/sec, 100 burst
	if tb.Allow(101, PriorityNone) {
		t.Fatal("expected 101 bytes to be rejected with 100 burst")
	}
}

func TestTokenBucket_RefillsOverTime(t *testing.T) {
	tb := NewTokenBucket(1000, 1000) // 1000 bytes/sec, 1000 burst

	// Consume full burst.
	if !tb.Allow(1000, PriorityNone) {
		t.Fatal("expected full burst to be allowed")
	}

	// Should be empty now.
	if tb.Allow(1, PriorityNone) {
		t.Fatal("expected bucket to be empty after full consume")
	}

	// Wait for partial refill (100ms = ~100 bytes at 1000/sec).
	time.Sleep(150 * time.Millisecond)

	if !tb.Allow(100, PriorityNone) {
		t.Fatal("expected ~100 bytes to be available after 150ms refill")
	}
}

func TestTokenBucket_HighPriorityBypass(t *testing.T) {
	tb := NewTokenBucket(100, 100)

	// Exhaust the bucket.
	tb.Allow(100, PriorityNone)

	// At zero budget, HIGH priority should still be allowed.
	if !tb.Allow(200, PriorityHigh) {
		t.Fatal("expected HIGH priority to bypass empty bucket")
	}

	// DELETE priority should also bypass.
	if !tb.Allow(200, PriorityDelete) {
		t.Fatal("expected DELETE priority to bypass empty bucket")
	}
}

func TestPriority_Ordering(t *testing.T) {
	// DELETE > HIGH > MEDIUM > LOW > NONE
	priorities := []int{PriorityNone, PriorityLow, PriorityMedium, PriorityHigh, PriorityDelete}
	for i := 1; i < len(priorities); i++ {
		if priorities[i] <= priorities[i-1] {
			t.Fatalf("expected priority %d > %d", priorities[i], priorities[i-1])
		}
	}

	// Test EventPriority function returns correct values.
	tests := []struct {
		name     string
		event    *storev1.EntityEvent
		expected int
	}{
		{
			name: "delete event",
			event: &storev1.EntityEvent{
				Type:   storev1.EventType_EVENT_TYPE_DELETED,
				Entity: &entityv1.Entity{Id: "t-1"},
			},
			expected: PriorityDelete,
		},
		{
			name: "no entity",
			event: &storev1.EntityEvent{
				Type: storev1.EventType_EVENT_TYPE_UPDATED,
			},
			expected: PriorityNone,
		},
		{
			name: "no threat component",
			event: &storev1.EntityEvent{
				Type:   storev1.EventType_EVENT_TYPE_UPDATED,
				Entity: &entityv1.Entity{Id: "t-1", Components: map[string]*anypb.Any{}},
			},
			expected: PriorityNone,
		},
		{
			name:     "high threat",
			event:    makeEventWithThreat(entityv1.ThreatLevel_THREAT_LEVEL_HIGH),
			expected: PriorityHigh,
		},
		{
			name:     "medium threat",
			event:    makeEventWithThreat(entityv1.ThreatLevel_THREAT_LEVEL_MEDIUM),
			expected: PriorityMedium,
		},
		{
			name:     "low threat",
			event:    makeEventWithThreat(entityv1.ThreatLevel_THREAT_LEVEL_LOW),
			expected: PriorityLow,
		},
		{
			name:     "none threat",
			event:    makeEventWithThreat(entityv1.ThreatLevel_THREAT_LEVEL_NONE),
			expected: PriorityNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EventPriority(tt.event)
			if got != tt.expected {
				t.Fatalf("expected priority %d, got %d", tt.expected, got)
			}
		})
	}
}

func TestCoalescer_DeduplicatesPositionUpdates(t *testing.T) {
	c := NewCoalescer()

	// Queue 3 position updates for entity "track-0".
	for i := 0; i < 3; i++ {
		pos, _ := anypb.New(&entityv1.PositionComponent{
			Lat: float64(i),
			Lon: float64(i * 10),
		})
		c.Add(&storev1.EntityEvent{
			Type: storev1.EventType_EVENT_TYPE_UPDATED,
			Entity: &entityv1.Entity{
				Id:         "track-0",
				Type:       entityv1.EntityType_ENTITY_TYPE_TRACK,
				Components: map[string]*anypb.Any{"position": pos},
			},
		})
	}

	events := c.Drain()
	if len(events) != 1 {
		t.Fatalf("expected 1 coalesced event, got %d", len(events))
	}
	if events[0].Entity.Id != "track-0" {
		t.Fatalf("expected track-0, got %s", events[0].Entity.Id)
	}

	// Verify it kept the latest (lat=2, lon=20).
	var pos entityv1.PositionComponent
	if err := events[0].Entity.Components["position"].UnmarshalTo(&pos); err != nil {
		t.Fatalf("unmarshal position: %v", err)
	}
	if pos.Lat != 2.0 || pos.Lon != 20.0 {
		t.Fatalf("expected latest position (2, 20), got (%f, %f)", pos.Lat, pos.Lon)
	}
}

func TestCoalescer_PreservesDeleteEvents(t *testing.T) {
	c := NewCoalescer()

	// Add an update then a delete for same entity.
	c.Add(&storev1.EntityEvent{
		Type:   storev1.EventType_EVENT_TYPE_UPDATED,
		Entity: &entityv1.Entity{Id: "track-0", Type: entityv1.EntityType_ENTITY_TYPE_TRACK},
	})
	c.Add(&storev1.EntityEvent{
		Type:   storev1.EventType_EVENT_TYPE_DELETED,
		Entity: &entityv1.Entity{Id: "track-0", Type: entityv1.EntityType_ENTITY_TYPE_TRACK},
	})

	events := c.Drain()
	// Delete events are never coalesced away — both should be present.
	if len(events) < 1 {
		t.Fatal("expected at least 1 event")
	}

	hasDelete := false
	for _, e := range events {
		if e.Type == storev1.EventType_EVENT_TYPE_DELETED {
			hasDelete = true
		}
	}
	if !hasDelete {
		t.Fatal("expected delete event to be preserved")
	}
}

func TestCoalescer_DifferentEntitiesKept(t *testing.T) {
	c := NewCoalescer()

	c.Add(&storev1.EntityEvent{
		Type:   storev1.EventType_EVENT_TYPE_UPDATED,
		Entity: &entityv1.Entity{Id: "track-0", Type: entityv1.EntityType_ENTITY_TYPE_TRACK},
	})
	c.Add(&storev1.EntityEvent{
		Type:   storev1.EventType_EVENT_TYPE_UPDATED,
		Entity: &entityv1.Entity{Id: "track-1", Type: entityv1.EntityType_ENTITY_TYPE_TRACK},
	})

	events := c.Drain()
	if len(events) != 2 {
		t.Fatalf("expected 2 events for different entities, got %d", len(events))
	}

	ids := make(map[string]bool)
	for _, e := range events {
		ids[e.Entity.Id] = true
	}
	if !ids["track-0"] || !ids["track-1"] {
		t.Fatal("expected both track-0 and track-1 to be present")
	}
}

func TestCoalescer_DrainSortsByPriority(t *testing.T) {
	c := NewCoalescer()

	// Add low-priority event first.
	c.Add(&storev1.EntityEvent{
		Type:   storev1.EventType_EVENT_TYPE_UPDATED,
		Entity: &entityv1.Entity{Id: "track-low", Type: entityv1.EntityType_ENTITY_TYPE_TRACK},
	})

	// Add high-priority event.
	threatHigh, _ := anypb.New(&entityv1.ThreatComponent{Level: entityv1.ThreatLevel_THREAT_LEVEL_HIGH})
	c.Add(&storev1.EntityEvent{
		Type: storev1.EventType_EVENT_TYPE_UPDATED,
		Entity: &entityv1.Entity{
			Id:         "track-high",
			Type:       entityv1.EntityType_ENTITY_TYPE_TRACK,
			Components: map[string]*anypb.Any{"threat": threatHigh},
		},
	})

	events := c.Drain()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// High priority should come first.
	if events[0].Entity.Id != "track-high" {
		t.Fatalf("expected track-high first (highest priority), got %s", events[0].Entity.Id)
	}
}

// makeEventWithThreat creates an update event with the given threat level.
func makeEventWithThreat(level entityv1.ThreatLevel) *storev1.EntityEvent {
	threatAny, _ := anypb.New(&entityv1.ThreatComponent{Level: level})
	return &storev1.EntityEvent{
		Type: storev1.EventType_EVENT_TYPE_UPDATED,
		Entity: &entityv1.Entity{
			Id:         "t-1",
			Type:       entityv1.EntityType_ENTITY_TYPE_TRACK,
			Components: map[string]*anypb.Any{"threat": threatAny},
		},
	}
}

// Verify sort.Interface is not needed — just check the sort package is used.
var _ = sort.Slice
