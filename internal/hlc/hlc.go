package hlc

import (
	"strings"
	"sync"
	"time"
)

// Timestamp is a hybrid logical clock timestamp providing total ordering
// across distributed nodes.
type Timestamp struct {
	Physical uint64 // Unix nanoseconds
	Logical  uint32 // Logical counter for sub-nanosecond ordering
	Node     string // Node ID for tie-breaking
}

// Before returns true if t is ordered before other.
func (t Timestamp) Before(other Timestamp) bool {
	return Compare(t, other) == -1
}

// After returns true if t is ordered after other.
func (t Timestamp) After(other Timestamp) bool {
	return Compare(t, other) == 1
}

// Compare returns -1 if a < b, 0 if a == b, 1 if a > b.
// Ordering: Physical first, then Logical, then Node (lexicographic) for total ordering.
func Compare(a, b Timestamp) int {
	if a.Physical < b.Physical {
		return -1
	}
	if a.Physical > b.Physical {
		return 1
	}
	if a.Logical < b.Logical {
		return -1
	}
	if a.Logical > b.Logical {
		return 1
	}
	return strings.Compare(a.Node, b.Node)
}

// Clock is a hybrid logical clock bound to a specific node.
type Clock struct {
	mu           sync.Mutex
	node         string
	lastPhysical uint64
	lastLogical  uint32
}

// NewClock creates a new HLC for the given node ID.
func NewClock(nodeID string) *Clock {
	return &Clock{node: nodeID}
}

// Now generates a new timestamp that is guaranteed to be greater than
// any previously generated timestamp from this clock.
func (c *Clock) Now() Timestamp {
	c.mu.Lock()
	defer c.mu.Unlock()

	wall := uint64(time.Now().UnixNano())

	if wall > c.lastPhysical {
		c.lastPhysical = wall
		c.lastLogical = 0
	} else {
		c.lastLogical++
	}

	return Timestamp{
		Physical: c.lastPhysical,
		Logical:  c.lastLogical,
		Node:     c.node,
	}
}

// Update merges a remote timestamp with the local clock state, producing
// a new timestamp that is greater than both the local state and the remote timestamp.
func (c *Clock) Update(remote Timestamp) Timestamp {
	c.mu.Lock()
	defer c.mu.Unlock()

	wall := uint64(time.Now().UnixNano())

	// Determine the maximum physical time among wall, local last, and remote.
	maxPhys := wall
	if c.lastPhysical > maxPhys {
		maxPhys = c.lastPhysical
	}
	if remote.Physical > maxPhys {
		maxPhys = remote.Physical
	}

	switch {
	case maxPhys == c.lastPhysical && maxPhys == remote.Physical:
		// All three tied — advance logical past the max of local and remote.
		if c.lastLogical > remote.Logical {
			c.lastLogical++
		} else {
			c.lastLogical = remote.Logical + 1
		}
	case maxPhys == c.lastPhysical:
		// Local physical wins — just increment local logical.
		c.lastLogical++
	case maxPhys == remote.Physical:
		// Remote physical wins — adopt remote logical + 1.
		c.lastLogical = remote.Logical + 1
	default:
		// Wall clock wins — reset logical.
		c.lastLogical = 0
	}

	c.lastPhysical = maxPhys

	return Timestamp{
		Physical: c.lastPhysical,
		Logical:  c.lastLogical,
		Node:     c.node,
	}
}
