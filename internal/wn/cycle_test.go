package wn

import (
	"testing"
	"time"
)

func TestWouldCreateCycle(t *testing.T) {
	now := time.Now().UTC()
	mk := func(id string, deps ...string) *Item {
		return &Item{ID: id, DependsOn: deps, Created: now, Updated: now}
	}
	// Graph: a depends on b, b depends on c. So a->b->c. Adding c->a gives cycle a->b->c->a.
	items := []*Item{
		mk("a", "b"),
		mk("b", "c"),
		mk("c"),
	}
	if !WouldCreateCycle(items, "c", "a") {
		t.Error("c -> a should create cycle")
	}
	// Adding a -> c is fine (no cycle: a->b->c, then a->c still no path c->a)
	if WouldCreateCycle(items, "a", "c") {
		t.Error("a -> c should not create cycle")
	}
	// Self-loop
	if !WouldCreateCycle(items, "a", "a") {
		t.Error("a -> a should create cycle")
	}
}
