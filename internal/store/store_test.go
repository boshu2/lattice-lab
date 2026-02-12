package store

import (
	"testing"
	"time"

	entityv1 "github.com/boshu2/lattice-lab/gen/entity/v1"
	storev1 "github.com/boshu2/lattice-lab/gen/store/v1"
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

	s.Create(&entityv1.Entity{Id: "a1", Type: entityv1.EntityType_ENTITY_TYPE_ASSET})
	s.Create(&entityv1.Entity{Id: "t1", Type: entityv1.EntityType_ENTITY_TYPE_TRACK})
	s.Create(&entityv1.Entity{Id: "t2", Type: entityv1.EntityType_ENTITY_TYPE_TRACK})

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

	s.Create(&entityv1.Entity{Id: "u1", Type: entityv1.EntityType_ENTITY_TYPE_GEO})

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
	s.Create(&entityv1.Entity{Id: "d1", Type: entityv1.EntityType_ENTITY_TYPE_ASSET})

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

	s.Create(&entityv1.Entity{Id: "w1", Type: entityv1.EntityType_ENTITY_TYPE_TRACK})

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
	s.Create(&entityv1.Entity{Id: "f1", Type: entityv1.EntityType_ENTITY_TYPE_TRACK})

	select {
	case event := <-w.Events:
		t.Fatalf("expected no event, got %v", event)
	case <-time.After(100 * time.Millisecond):
		// Good — no event received.
	}

	// This should trigger the watcher.
	s.Create(&entityv1.Entity{Id: "f2", Type: entityv1.EntityType_ENTITY_TYPE_ASSET})

	select {
	case event := <-w.Events:
		if event.Entity.Id != "f2" {
			t.Fatalf("expected entity f2, got %s", event.Entity.Id)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for filtered event")
	}
}
