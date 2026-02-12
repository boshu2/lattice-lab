package hlc

import (
	"sync"
	"testing"
)

func TestNow_Monotonic(t *testing.T) {
	c := NewClock("node-1")
	prev := c.Now()
	for i := 0; i < 1000; i++ {
		cur := c.Now()
		if Compare(prev, cur) != -1 {
			t.Fatalf("iteration %d: prev %+v not less than cur %+v", i, prev, cur)
		}
		prev = cur
	}
}

func TestNow_AdvancesLogical(t *testing.T) {
	c := NewClock("node-1")
	// Call Now() twice in rapid succession; physical clock likely hasn't advanced.
	a := c.Now()
	b := c.Now()
	// b must be greater than a
	if Compare(a, b) != -1 {
		t.Fatalf("expected a < b, got a=%+v b=%+v", a, b)
	}
	// If physical is the same, logical must have incremented.
	if a.Physical == b.Physical && b.Logical != a.Logical+1 {
		t.Fatalf("expected logical to increment when physical unchanged: a=%+v b=%+v", a, b)
	}
}

func TestUpdate_AdvancesPastRemote(t *testing.T) {
	c := NewClock("node-1")
	remote := Timestamp{Physical: uint64(1e18), Logical: 5, Node: "node-2"} // far future
	result := c.Update(remote)
	if Compare(result, remote) != 1 {
		t.Fatalf("expected result > remote, got result=%+v remote=%+v", result, remote)
	}
}

func TestUpdate_AdvancesPastLocal(t *testing.T) {
	c := NewClock("node-1")
	// Advance local clock well ahead.
	for i := 0; i < 100; i++ {
		c.Now()
	}
	local := c.Now()
	// Remote is in the past.
	remote := Timestamp{Physical: 1, Logical: 0, Node: "node-2"}
	result := c.Update(remote)
	if Compare(result, local) != 1 {
		t.Fatalf("expected result > local, got result=%+v local=%+v", result, local)
	}
}

func TestCompare(t *testing.T) {
	tests := []struct {
		name string
		a, b Timestamp
		want int
	}{
		{
			name: "less by physical",
			a:    Timestamp{Physical: 1, Logical: 0, Node: "a"},
			b:    Timestamp{Physical: 2, Logical: 0, Node: "a"},
			want: -1,
		},
		{
			name: "greater by physical",
			a:    Timestamp{Physical: 3, Logical: 0, Node: "a"},
			b:    Timestamp{Physical: 2, Logical: 0, Node: "a"},
			want: 1,
		},
		{
			name: "less by logical",
			a:    Timestamp{Physical: 1, Logical: 0, Node: "a"},
			b:    Timestamp{Physical: 1, Logical: 1, Node: "a"},
			want: -1,
		},
		{
			name: "greater by logical",
			a:    Timestamp{Physical: 1, Logical: 2, Node: "a"},
			b:    Timestamp{Physical: 1, Logical: 1, Node: "a"},
			want: 1,
		},
		{
			name: "less by node",
			a:    Timestamp{Physical: 1, Logical: 0, Node: "a"},
			b:    Timestamp{Physical: 1, Logical: 0, Node: "b"},
			want: -1,
		},
		{
			name: "greater by node",
			a:    Timestamp{Physical: 1, Logical: 0, Node: "b"},
			b:    Timestamp{Physical: 1, Logical: 0, Node: "a"},
			want: 1,
		},
		{
			name: "equal",
			a:    Timestamp{Physical: 1, Logical: 0, Node: "a"},
			b:    Timestamp{Physical: 1, Logical: 0, Node: "a"},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Compare(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("Compare(%+v, %+v) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestConcurrentSafety(t *testing.T) {
	c := NewClock("node-1")
	const goroutines = 100
	const iterations = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			prev := c.Now()
			for i := 0; i < iterations; i++ {
				var cur Timestamp
				if i%2 == 0 {
					cur = c.Now()
				} else {
					cur = c.Update(Timestamp{Physical: prev.Physical, Logical: prev.Logical, Node: "remote"})
				}
				if Compare(prev, cur) != -1 {
					t.Errorf("not monotonic: prev=%+v cur=%+v", prev, cur)
					return
				}
				prev = cur
			}
		}()
	}
	wg.Wait()
}

func TestZeroValue(t *testing.T) {
	c := NewClock("node-1")
	zero := Timestamp{}
	now := c.Now()
	if Compare(zero, now) != -1 {
		t.Fatalf("expected zero < now, got zero=%+v now=%+v", zero, now)
	}
}

func TestBefore(t *testing.T) {
	a := Timestamp{Physical: 1, Logical: 0, Node: "a"}
	b := Timestamp{Physical: 2, Logical: 0, Node: "a"}
	if !a.Before(b) {
		t.Error("expected a.Before(b)")
	}
	if b.Before(a) {
		t.Error("expected !b.Before(a)")
	}
}

func TestAfter(t *testing.T) {
	a := Timestamp{Physical: 1, Logical: 0, Node: "a"}
	b := Timestamp{Physical: 2, Logical: 0, Node: "a"}
	if !b.After(a) {
		t.Error("expected b.After(a)")
	}
	if a.After(b) {
		t.Error("expected !a.After(b)")
	}
}
