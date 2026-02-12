package store

import (
	"context"
	"testing"
	"time"

	entityv1 "github.com/boshu2/lattice-lab/gen/entity/v1"
	storev1 "github.com/boshu2/lattice-lab/gen/store/v1"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestCreateAndGet(t *testing.T) {
	s := New()

	e := &entityv1.Entity{
		Id:   "asset-1",
		Type: entityv1.EntityType_ENTITY_TYPE_ASSET,
	}

	created, err := s.Create(e)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Id != "asset-1" {
		t.Fatalf("expected id asset-1, got %s", created.Id)
	}
	if created.CreatedAt == nil {
		t.Fatal("expected CreatedAt to be set")
	}

	got, err := s.Get("asset-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Id != "asset-1" {
		t.Fatalf("expected id asset-1, got %s", got.Id)
	}
}

func TestCreateDuplicate(t *testing.T) {
	s := New()

	e := &entityv1.Entity{Id: "dup-1", Type: entityv1.EntityType_ENTITY_TYPE_TRACK}
	if _, err := s.Create(e); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if _, err := s.Create(e); err == nil {
		t.Fatal("expected error on duplicate create")
	}
}

func TestGetNotFound(t *testing.T) {
	s := New()
	if _, err := s.Get("nope"); err == nil {
		t.Fatal("expected error for missing entity")
	}
}

func TestListWithFilter(t *testing.T) {
	s := New()

	_, _ = s.Create(&entityv1.Entity{Id: "a1", Type: entityv1.EntityType_ENTITY_TYPE_ASSET})
	_, _ = s.Create(&entityv1.Entity{Id: "t1", Type: entityv1.EntityType_ENTITY_TYPE_TRACK})
	_, _ = s.Create(&entityv1.Entity{Id: "t2", Type: entityv1.EntityType_ENTITY_TYPE_TRACK})

	all := s.List(entityv1.EntityType_ENTITY_TYPE_UNSPECIFIED)
	if len(all) != 3 {
		t.Fatalf("expected 3 entities, got %d", len(all))
	}

	tracks := s.List(entityv1.EntityType_ENTITY_TYPE_TRACK)
	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(tracks))
	}
}

func TestUpdate(t *testing.T) {
	s := New()

	_, _ = s.Create(&entityv1.Entity{Id: "u1", Type: entityv1.EntityType_ENTITY_TYPE_GEO})

	updated, err := s.Update(&entityv1.Entity{
		Id:   "u1",
		Type: entityv1.EntityType_ENTITY_TYPE_GEO,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.CreatedAt == nil {
		t.Fatal("expected CreatedAt preserved")
	}
}

func TestUpdateNotFound(t *testing.T) {
	s := New()
	if _, err := s.Update(&entityv1.Entity{Id: "nope"}); err == nil {
		t.Fatal("expected error for missing entity")
	}
}

func TestDelete(t *testing.T) {
	s := New()
	_, _ = s.Create(&entityv1.Entity{Id: "d1", Type: entityv1.EntityType_ENTITY_TYPE_ASSET})

	if err := s.Delete("d1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get("d1"); err == nil {
		t.Fatal("expected entity to be deleted")
	}
}

func TestDeleteNotFound(t *testing.T) {
	s := New()
	if err := s.Delete("nope"); err == nil {
		t.Fatal("expected error for missing entity")
	}
}

func TestWatch(t *testing.T) {
	s := New()

	w := s.Watch(entityv1.EntityType_ENTITY_TYPE_UNSPECIFIED)
	defer s.Unwatch(w)

	_, _ = s.Create(&entityv1.Entity{Id: "w1", Type: entityv1.EntityType_ENTITY_TYPE_TRACK})

	select {
	case event := <-w.Events:
		if event.Type != storev1.EventType_EVENT_TYPE_CREATED {
			t.Fatalf("expected CREATED, got %v", event.Type)
		}
		if event.Entity.Id != "w1" {
			t.Fatalf("expected entity w1, got %s", event.Entity.Id)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestWatchWithFilter(t *testing.T) {
	s := New()

	w := s.Watch(entityv1.EntityType_ENTITY_TYPE_ASSET)
	defer s.Unwatch(w)

	// This should NOT trigger the watcher (wrong type).
	_, _ = s.Create(&entityv1.Entity{Id: "f1", Type: entityv1.EntityType_ENTITY_TYPE_TRACK})

	select {
	case event := <-w.Events:
		t.Fatalf("expected no event, got %v", event)
	case <-time.After(100 * time.Millisecond):
		// Good — no event received.
	}

	// This should trigger the watcher.
	_, _ = s.Create(&entityv1.Entity{Id: "f2", Type: entityv1.EntityType_ENTITY_TYPE_ASSET})

	select {
	case event := <-w.Events:
		if event.Entity.Id != "f2" {
			t.Fatalf("expected entity f2, got %s", event.Entity.Id)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for filtered event")
	}
}

func TestTTLExpiration(t *testing.T) {
	s := New()

	_, _ = s.Create(&entityv1.Entity{Id: "ttl-1", Type: entityv1.EntityType_ENTITY_TYPE_TRACK})
	_, _ = s.Create(&entityv1.Entity{Id: "ttl-2", Type: entityv1.EntityType_ENTITY_TYPE_TRACK})

	// Set a very short TTL on ttl-1.
	s.SetTTL("ttl-1", 50*time.Millisecond)

	// Start reaper.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.StartReaper(ctx, 25*time.Millisecond)

	// Wait for TTL to expire and reaper to run.
	time.Sleep(200 * time.Millisecond)

	// ttl-1 should be gone.
	if _, err := s.Get("ttl-1"); err == nil {
		t.Fatal("expected ttl-1 to be expired")
	}

	// ttl-2 should still exist (no TTL set).
	if _, err := s.Get("ttl-2"); err != nil {
		t.Fatalf("ttl-2 should still exist: %v", err)
	}
}

// --- HLC Integration Tests ---

func TestNew_DefaultNodeID(t *testing.T) {
	s := New()
	if s.clock == nil {
		t.Fatal("expected clock to be initialized")
	}
	// Generate a timestamp to verify the node ID is non-empty.
	ts := s.clock.Now()
	if ts.Node == "" {
		t.Fatal("expected non-empty default node ID")
	}
}

func TestNew_WithNodeID(t *testing.T) {
	s := New(WithNodeID("test-node"))
	ts := s.clock.Now()
	if ts.Node != "test-node" {
		t.Fatalf("expected node ID 'test-node', got %q", ts.Node)
	}
}

func TestCreate_StampsHLC(t *testing.T) {
	s := New(WithNodeID("store-1"))

	created, err := s.Create(&entityv1.Entity{
		Id:   "hlc-1",
		Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.HlcPhysical == 0 {
		t.Fatal("expected non-zero HlcPhysical after Create")
	}
	if created.HlcNode != "store-1" {
		t.Fatalf("expected HlcNode 'store-1', got %q", created.HlcNode)
	}
}

func TestUpdate_AdvancesHLC(t *testing.T) {
	s := New(WithNodeID("store-2"))

	created, err := s.Create(&entityv1.Entity{
		Id:   "hlc-2",
		Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := s.Update(&entityv1.Entity{
		Id:   "hlc-2",
		Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Updated HLC should be >= created HLC.
	if updated.HlcPhysical < created.HlcPhysical {
		t.Fatalf("expected HlcPhysical to advance: created=%d updated=%d",
			created.HlcPhysical, updated.HlcPhysical)
	}
	if updated.HlcPhysical == created.HlcPhysical && updated.HlcLogical <= created.HlcLogical {
		t.Fatalf("expected HLC to advance: created=(%d,%d) updated=(%d,%d)",
			created.HlcPhysical, created.HlcLogical,
			updated.HlcPhysical, updated.HlcLogical)
	}
}

func makeAnyString(t *testing.T, val string) *anypb.Any {
	t.Helper()
	a, err := anypb.New(wrapperspb.String(val))
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

func TestUpdate_MergesComponents(t *testing.T) {
	s := New(WithNodeID("merge-node"))

	// Create entity with position and velocity components.
	created, err := s.Create(&entityv1.Entity{
		Id:   "merge-1",
		Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
		Components: map[string]*anypb.Any{
			"position": makeAnyString(t, "pos-data"),
			"velocity": makeAnyString(t, "vel-data"),
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Update with classification and threat (different keys).
	// Use an HLC >= the created entity's HLC so the update is not stale.
	updated, err := s.Update(&entityv1.Entity{
		Id:   "merge-1",
		Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
		Components: map[string]*anypb.Any{
			"classification": makeAnyString(t, "class-data"),
			"threat":         makeAnyString(t, "threat-data"),
		},
		HlcPhysical: created.HlcPhysical,
		HlcLogical:  created.HlcLogical,
		HlcNode:     created.HlcNode,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Result should have all 4 components.
	for _, key := range []string{"position", "velocity", "classification", "threat"} {
		if _, ok := updated.Components[key]; !ok {
			t.Fatalf("expected component %q to be present after merge", key)
		}
	}
}

func TestUpdate_SameKeyHigherHLCWins(t *testing.T) {
	s := New(WithNodeID("hlc-win"))

	created, err := s.Create(&entityv1.Entity{
		Id:   "hlc-win-1",
		Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
		Components: map[string]*anypb.Any{
			"position": makeAnyString(t, "old-pos"),
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Update same key with a higher HLC (physical+1).
	updated, err := s.Update(&entityv1.Entity{
		Id:   "hlc-win-1",
		Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
		Components: map[string]*anypb.Any{
			"position": makeAnyString(t, "new-pos"),
		},
		HlcPhysical: created.HlcPhysical + 1,
		HlcLogical:  0,
		HlcNode:     "remote-node",
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	// The incoming (higher HLC) should win for "position".
	posAny := updated.Components["position"]
	var sv wrapperspb.StringValue
	if err := posAny.UnmarshalTo(&sv); err != nil {
		t.Fatalf("unmarshal position: %v", err)
	}
	if sv.Value != "new-pos" {
		t.Fatalf("expected position='new-pos', got %q", sv.Value)
	}
}

func TestUpdate_SameKeyStaleHLCKept(t *testing.T) {
	s := New(WithNodeID("hlc-stale"))

	// Create entity — store will stamp it with current HLC.
	created, err := s.Create(&entityv1.Entity{
		Id:   "hlc-stale-1",
		Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
		Components: map[string]*anypb.Any{
			"position": makeAnyString(t, "existing-pos"),
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Update with a STALE HLC (physical=5, which is much less than the stored one)
	// for the same key. Also include a new key that should be accepted.
	updated, err := s.Update(&entityv1.Entity{
		Id:   "hlc-stale-1",
		Type: entityv1.EntityType_ENTITY_TYPE_TRACK,
		Components: map[string]*anypb.Any{
			"position": makeAnyString(t, "stale-pos"),
			"threat":   makeAnyString(t, "new-threat"),
		},
		HlcPhysical: 5,
		HlcLogical:  0,
		HlcNode:     "stale-node",
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	// "position" should keep the existing value (store had higher HLC).
	posAny := updated.Components["position"]
	var sv wrapperspb.StringValue
	if err := posAny.UnmarshalTo(&sv); err != nil {
		t.Fatalf("unmarshal position: %v", err)
	}
	if sv.Value != "existing-pos" {
		t.Fatalf("expected position='existing-pos' (kept from store), got %q", sv.Value)
	}

	// "threat" should be accepted (new key, not in existing entity).
	if _, ok := updated.Components["threat"]; !ok {
		t.Fatal("expected 'threat' component to be accepted (new key)")
	}

	// Verify the created HLC was preserved.
	if updated.HlcPhysical < created.HlcPhysical {
		t.Fatalf("expected store to advance HLC, not go backward: created=%d updated=%d",
			created.HlcPhysical, updated.HlcPhysical)
	}
}
