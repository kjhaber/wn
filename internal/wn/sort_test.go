package wn

import (
	"testing"
	"time"
)

func TestTopoOrder(t *testing.T) {
	now := time.Now().UTC()
	mk := func(id string, deps ...string) *Item {
		return &Item{ID: id, DependsOn: deps, Created: now, Updated: now}
	}
	items := []*Item{
		mk("a"),
		mk("b", "a"),
		mk("c", "b"),
	}
	ordered, acyclic := TopoOrder(items)
	if !acyclic {
		t.Fatal("expected acyclic")
	}
	if len(ordered) != 3 {
		t.Fatalf("len = %d", len(ordered))
	}
	if ordered[0].ID != "a" || ordered[1].ID != "b" || ordered[2].ID != "c" {
		t.Errorf("order = %v", ordered)
	}
}

func TestTopoOrder_Cycle(t *testing.T) {
	now := time.Now().UTC()
	mk := func(id string, deps ...string) *Item {
		return &Item{ID: id, DependsOn: deps, Created: now, Updated: now}
	}
	items := []*Item{
		mk("a", "c"),
		mk("b", "a"),
		mk("c", "b"),
	}
	_, acyclic := TopoOrder(items)
	if acyclic {
		t.Error("expected cycle to be detected")
	}
}
