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

// orderVal returns a pointer to int for Item.Order.
func orderVal(n int) *int { return &n }

func TestTopoOrder_OrderTiebreaker(t *testing.T) {
	// When dependencies don't define order, optional Order field breaks ties (lower = earlier).
	now := time.Now().UTC()
	items := []*Item{
		{ID: "a", DependsOn: nil, Order: orderVal(2), Created: now, Updated: now},
		{ID: "b", DependsOn: nil, Order: orderVal(1), Created: now, Updated: now},
	}
	ordered, acyclic := TopoOrder(items)
	if !acyclic {
		t.Fatal("expected acyclic")
	}
	if ordered[0].ID != "b" || ordered[1].ID != "a" {
		t.Errorf("order = %v (expected b then a)", ordered)
	}
}

func TestTopoOrder_OrderWithDeps(t *testing.T) {
	// Dependencies still take precedence; Order only breaks ties among ready items.
	// a and c are ready (no deps); b depends on a. Order: c=1, a=2 -> c, a, then b.
	now := time.Now().UTC()
	items := []*Item{
		{ID: "a", DependsOn: nil, Order: orderVal(2), Created: now, Updated: now},
		{ID: "b", DependsOn: []string{"a"}, Order: nil, Created: now, Updated: now},
		{ID: "c", DependsOn: nil, Order: orderVal(1), Created: now, Updated: now},
	}
	ordered, acyclic := TopoOrder(items)
	if !acyclic {
		t.Fatal("expected acyclic")
	}
	if ordered[0].ID != "c" || ordered[1].ID != "a" || ordered[2].ID != "b" {
		t.Errorf("order = %v (expected c, a, b)", ordered)
	}
}

func TestTopoOrder_OrderNilLast(t *testing.T) {
	// Items with no Order (nil) use DefaultOrder (99), so they appear after low explicit values (1, 2).
	now := time.Now().UTC()
	items := []*Item{
		{ID: "a", DependsOn: nil, Order: orderVal(1), Created: now, Updated: now},
		{ID: "b", DependsOn: nil, Order: nil, Created: now, Updated: now},
		{ID: "c", DependsOn: nil, Order: orderVal(2), Created: now, Updated: now},
	}
	ordered, acyclic := TopoOrder(items)
	if !acyclic {
		t.Fatal("expected acyclic")
	}
	if ordered[0].ID != "a" || ordered[1].ID != "c" || ordered[2].ID != "b" {
		t.Errorf("order = %v (expected a, c, b)", ordered)
	}
}

func TestTopoOrder_OrderAboveDefault(t *testing.T) {
	// Order 100 is below default (99), so item with 100 appears after item with nil.
	now := time.Now().UTC()
	items := []*Item{
		{ID: "default", DependsOn: nil, Order: nil, Created: now, Updated: now},
		{ID: "low", DependsOn: nil, Order: orderVal(100), Created: now, Updated: now},
	}
	ordered, acyclic := TopoOrder(items)
	if !acyclic {
		t.Fatal("expected acyclic")
	}
	if ordered[0].ID != "default" || ordered[1].ID != "low" {
		t.Errorf("order = %v (expected default then low)", ordered)
	}
}

func TestValidOrder(t *testing.T) {
	for _, n := range []int{0, 1, 99, 255} {
		if !ValidOrder(n) {
			t.Errorf("ValidOrder(%d) = false, want true", n)
		}
	}
	for _, n := range []int{-1, 256, 1000} {
		if ValidOrder(n) {
			t.Errorf("ValidOrder(%d) = true, want false", n)
		}
	}
}
