package crdt

import (
	"testing"

	entityv1 "github.com/boshu2/lattice-lab/gen/entity/v1"
	"github.com/boshu2/lattice-lab/internal/hlc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// makeEntity creates a test entity with the given HLC and component map.
func makeEntity(id string, ts hlc.Timestamp, comps map[string]proto.Message) *entityv1.Entity {
	e := &entityv1.Entity{
		Id:          id,
		Type:        entityv1.EntityType_ENTITY_TYPE_TRACK,
		Components:  make(map[string]*anypb.Any),
		HlcPhysical: ts.Physical,
		HlcLogical:  ts.Logical,
		HlcNode:     ts.Node,
	}
	for k, v := range comps {
		a, err := anypb.New(v)
		if err != nil {
			panic(err)
		}
		e.Components[k] = a
	}
	return e
}

func hlcTS(physical uint64, logical uint32, node string) hlc.Timestamp {
	return hlc.Timestamp{Physical: physical, Logical: logical, Node: node}
}

// componentKeys returns sorted keys from a component map.
func componentKeys(e *entityv1.Entity) map[string]bool {
	out := make(map[string]bool, len(e.Components))
	for k := range e.Components {
		out[k] = true
	}
	return out
}

func TestMergeEntity_Commutativity(t *testing.T) {
	tsA := hlcTS(100, 0, "nodeA")
	tsB := hlcTS(200, 0, "nodeB")

	a := makeEntity("e1", tsA, map[string]proto.Message{
		"position": &entityv1.PositionComponent{Lat: 1.0, Lon: 2.0, Alt: 3.0},
	})
	b := makeEntity("e1", tsB, map[string]proto.Message{
		"velocity": &entityv1.VelocityComponent{Speed: 100, Heading: 90},
	})

	ab := MergeEntity(a, b)
	ba := MergeEntity(b, a)

	// Both merges should have the same component keys.
	abKeys := componentKeys(ab)
	baKeys := componentKeys(ba)

	if len(abKeys) != len(baKeys) {
		t.Fatalf("commutativity violated: merge(a,b) has %d keys, merge(b,a) has %d keys", len(abKeys), len(baKeys))
	}
	for k := range abKeys {
		if !baKeys[k] {
			t.Errorf("commutativity violated: key %q in merge(a,b) but not merge(b,a)", k)
		}
	}

	// Both should have same HLC.
	if ab.HlcPhysical != ba.HlcPhysical || ab.HlcLogical != ba.HlcLogical {
		t.Errorf("commutativity violated: HLC differs: (%d,%d) vs (%d,%d)",
			ab.HlcPhysical, ab.HlcLogical, ba.HlcPhysical, ba.HlcLogical)
	}
}

func TestMergeEntity_Idempotency(t *testing.T) {
	ts := hlcTS(100, 5, "node1")
	a := makeEntity("e1", ts, map[string]proto.Message{
		"position": &entityv1.PositionComponent{Lat: 1.0, Lon: 2.0, Alt: 3.0},
		"velocity": &entityv1.VelocityComponent{Speed: 50, Heading: 180},
	})

	result := MergeEntity(a, a)

	if len(result.Components) != len(a.Components) {
		t.Fatalf("idempotency violated: expected %d components, got %d", len(a.Components), len(result.Components))
	}

	// Verify position is unchanged.
	var pos entityv1.PositionComponent
	if err := result.Components["position"].UnmarshalTo(&pos); err != nil {
		t.Fatal(err)
	}
	if pos.Lat != 1.0 || pos.Lon != 2.0 || pos.Alt != 3.0 {
		t.Errorf("idempotency violated: position changed: %v", &pos)
	}

	if result.HlcPhysical != ts.Physical || result.HlcLogical != ts.Logical {
		t.Errorf("idempotency violated: HLC changed")
	}
}

func TestMergeEntity_LWW_HigherHLCWins(t *testing.T) {
	tsA := hlcTS(10, 0, "node1")
	tsB := hlcTS(20, 0, "node1")

	a := makeEntity("e1", tsA, map[string]proto.Message{
		"position": &entityv1.PositionComponent{Lat: 1.0, Lon: 1.0},
	})
	b := makeEntity("e1", tsB, map[string]proto.Message{
		"position": &entityv1.PositionComponent{Lat: 2.0, Lon: 2.0},
	})

	result := MergeEntity(a, b)

	var pos entityv1.PositionComponent
	if err := result.Components["position"].UnmarshalTo(&pos); err != nil {
		t.Fatal(err)
	}
	if pos.Lat != 2.0 || pos.Lon != 2.0 {
		t.Errorf("LWW failed: expected B's position (2,2), got (%v,%v)", pos.Lat, pos.Lon)
	}
}

func TestMergeEntity_LWW_LowerHLCLoses(t *testing.T) {
	tsA := hlcTS(10, 0, "node1")
	tsB := hlcTS(20, 0, "node1")

	a := makeEntity("e1", tsA, map[string]proto.Message{
		"position": &entityv1.PositionComponent{Lat: 1.0, Lon: 1.0},
	})
	b := makeEntity("e1", tsB, map[string]proto.Message{
		"position": &entityv1.PositionComponent{Lat: 2.0, Lon: 2.0},
	})

	result := MergeEntity(a, b)

	var pos entityv1.PositionComponent
	if err := result.Components["position"].UnmarshalTo(&pos); err != nil {
		t.Fatal(err)
	}
	// A's position (1.0, 1.0) should NOT be present.
	if pos.Lat == 1.0 && pos.Lon == 1.0 {
		t.Error("LWW failed: lower HLC's position should have been overwritten")
	}
}

func TestMergeEntity_DisjointComponents(t *testing.T) {
	tsA := hlcTS(100, 0, "nodeA")
	tsB := hlcTS(200, 0, "nodeB")

	a := makeEntity("e1", tsA, map[string]proto.Message{
		"position": &entityv1.PositionComponent{Lat: 1.0, Lon: 2.0},
		"velocity": &entityv1.VelocityComponent{Speed: 100, Heading: 90},
	})
	b := makeEntity("e1", tsB, map[string]proto.Message{
		"classification": &entityv1.ClassificationComponent{Label: "military", Confidence: 0.9},
		"threat":         &entityv1.ThreatComponent{Level: entityv1.ThreatLevel_THREAT_LEVEL_HIGH},
	})

	result := MergeEntity(a, b)

	expected := []string{"position", "velocity", "classification", "threat"}
	if len(result.Components) != len(expected) {
		t.Fatalf("disjoint merge: expected %d components, got %d", len(expected), len(result.Components))
	}
	for _, key := range expected {
		if _, ok := result.Components[key]; !ok {
			t.Errorf("disjoint merge: missing component %q", key)
		}
	}
}

func TestMergeEntity_ThreatMaxWins(t *testing.T) {
	// A has LOW threat with HIGHER HLC — but B's HIGH should still win.
	tsA := hlcTS(200, 0, "node1")
	tsB := hlcTS(100, 0, "node1")

	a := makeEntity("e1", tsA, map[string]proto.Message{
		"threat": &entityv1.ThreatComponent{Level: entityv1.ThreatLevel_THREAT_LEVEL_LOW},
	})
	b := makeEntity("e1", tsB, map[string]proto.Message{
		"threat": &entityv1.ThreatComponent{Level: entityv1.ThreatLevel_THREAT_LEVEL_HIGH},
	})

	result := MergeEntity(a, b)

	var threat entityv1.ThreatComponent
	if err := result.Components["threat"].UnmarshalTo(&threat); err != nil {
		t.Fatal(err)
	}
	if threat.Level != entityv1.ThreatLevel_THREAT_LEVEL_HIGH {
		t.Errorf("threat max-wins failed: expected HIGH, got %v", threat.Level)
	}
}

func TestMergeEntity_ThreatSameLevelUsesHLC(t *testing.T) {
	// Both have LOW, but B has higher HLC so B wins.
	tsA := hlcTS(100, 0, "node1")
	tsB := hlcTS(200, 0, "node1")

	a := makeEntity("e1", tsA, map[string]proto.Message{
		"threat": &entityv1.ThreatComponent{Level: entityv1.ThreatLevel_THREAT_LEVEL_LOW},
	})
	b := makeEntity("e1", tsB, map[string]proto.Message{
		"threat": &entityv1.ThreatComponent{Level: entityv1.ThreatLevel_THREAT_LEVEL_LOW},
	})

	result := MergeEntity(a, b)

	// Both are LOW, so result should be LOW — but from B (higher HLC).
	var threat entityv1.ThreatComponent
	if err := result.Components["threat"].UnmarshalTo(&threat); err != nil {
		t.Fatal(err)
	}
	if threat.Level != entityv1.ThreatLevel_THREAT_LEVEL_LOW {
		t.Errorf("threat same-level HLC fallback: expected LOW, got %v", threat.Level)
	}
}

func TestMergeEntity_ResultHLC(t *testing.T) {
	tsA := hlcTS(100, 5, "nodeA")
	tsB := hlcTS(200, 3, "nodeB")

	a := makeEntity("e1", tsA, map[string]proto.Message{
		"position": &entityv1.PositionComponent{Lat: 1.0},
	})
	b := makeEntity("e1", tsB, map[string]proto.Message{
		"velocity": &entityv1.VelocityComponent{Speed: 50},
	})

	result := MergeEntity(a, b)

	// B has the higher HLC (physical 200 > 100).
	if result.HlcPhysical != 200 {
		t.Errorf("result HLC physical: expected 200, got %d", result.HlcPhysical)
	}
	if result.HlcLogical != 3 {
		t.Errorf("result HLC logical: expected 3, got %d", result.HlcLogical)
	}
	if result.HlcNode != "nodeB" {
		t.Errorf("result HLC node: expected nodeB, got %s", result.HlcNode)
	}
}
