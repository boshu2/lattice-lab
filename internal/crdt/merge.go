// Package crdt provides CRDT merge strategies for lattice-lab entities.
// It implements an LWW-Element-Map where each component key is a register
// with per-key merge strategies (LWW by default, max-wins for threat).
package crdt

import (
	entityv1 "github.com/boshu2/lattice-lab/gen/entity/v1"
	"github.com/boshu2/lattice-lab/internal/hlc"
	"google.golang.org/protobuf/types/known/anypb"
)

// MergeEntity merges two entities into one using LWW-Element-Map semantics.
// The result gets the higher entity-level HLC. For each component key present
// in either entity, a per-key merge strategy is applied.
func MergeEntity(a, b *entityv1.Entity) *entityv1.Entity {
	hlcA := entityHLC(a)
	hlcB := entityHLC(b)

	// Determine the winning entity-level HLC.
	winHLC := hlcA
	if hlcB.After(hlcA) {
		winHLC = hlcB
	}

	result := &entityv1.Entity{
		Id:          a.Id,
		Type:        a.Type,
		Components:  make(map[string]*anypb.Any),
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
		HlcPhysical: winHLC.Physical,
		HlcLogical:  winHLC.Logical,
		HlcNode:     winHLC.Node,
	}

	// Collect all component keys from both entities.
	keys := make(map[string]struct{})
	for k := range a.Components {
		keys[k] = struct{}{}
	}
	for k := range b.Components {
		keys[k] = struct{}{}
	}

	for key := range keys {
		compA, inA := a.Components[key]
		compB, inB := b.Components[key]

		switch {
		case inA && !inB:
			result.Components[key] = compA
		case !inA && inB:
			result.Components[key] = compB
		default:
			result.Components[key] = mergeComponent(key, compA, compB, hlcA, hlcB)
		}
	}

	return result
}

// mergeComponent dispatches to the appropriate merge strategy based on key.
func mergeComponent(key string, compA, compB *anypb.Any, hlcA, hlcB hlc.Timestamp) *anypb.Any {
	switch key {
	case "threat":
		return mergeThreat(compA, compB, hlcA, hlcB)
	default:
		// LWW: higher HLC wins. On tie, b wins (arbitrary but deterministic
		// since HLC includes node for total ordering).
		if hlcA.After(hlcB) {
			return compA
		}
		return compB
	}
}

// mergeThreat implements max-wins semantics for threat components.
// The higher threat level always wins. If levels are equal, the component
// with the higher HLC wins.
func mergeThreat(a, b *anypb.Any, hlcA, hlcB hlc.Timestamp) *anypb.Any {
	var threatA, threatB entityv1.ThreatComponent
	if err := a.UnmarshalTo(&threatA); err != nil {
		return b
	}
	if err := b.UnmarshalTo(&threatB); err != nil {
		return a
	}

	if threatA.Level > threatB.Level {
		return a
	}
	if threatB.Level > threatA.Level {
		return b
	}

	// Same level: fall back to HLC.
	if hlcA.After(hlcB) {
		return a
	}
	return b
}

// entityHLC extracts the HLC timestamp from an entity's fields.
func entityHLC(e *entityv1.Entity) hlc.Timestamp {
	return hlc.Timestamp{
		Physical: e.HlcPhysical,
		Logical:  e.HlcLogical,
		Node:     e.HlcNode,
	}
}
