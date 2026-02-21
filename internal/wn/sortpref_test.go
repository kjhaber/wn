package wn

import (
	"testing"
	"time"
)

func TestParseSortSpec(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		want    []SortOption
		wantErr bool
	}{
		{"empty", "", nil, false},
		{"single key default asc", "created", []SortOption{{Key: "created", Desc: false}}, false},
		{"explicit asc", "updated:asc", []SortOption{{Key: "updated", Desc: false}}, false},
		{"desc", "updated:desc", []SortOption{{Key: "updated", Desc: true}}, false},
		{"multiple", "updated:desc,priority,tags", []SortOption{
			{Key: "updated", Desc: true},
			{Key: "priority", Desc: false},
			{Key: "tags", Desc: false},
		}, false},
		{"alpha", "alpha", []SortOption{{Key: "alpha", Desc: false}}, false},
		{"invalid key", "invalid", nil, true},
		{"invalid direction", "created:invalid", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSortSpec(tt.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSortSpec(%q) error = %v, wantErr %v", tt.s, err, tt.wantErr)
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("ParseSortSpec(%q) len = %d, want %d", tt.s, len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i].Key != tt.want[i].Key || got[i].Desc != tt.want[i].Desc {
					t.Errorf("ParseSortSpec(%q)[%d] = %+v, want %+v", tt.s, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func sortprefOrderVal(n int) *int { return &n }

func TestApplySort(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	items := []*Item{
		{ID: "c", Description: "zeta", Created: now, Updated: now.Add(2 * time.Hour), Order: sortprefOrderVal(2), Tags: []string{"x"}},
		{ID: "a", Description: "alpha", Created: now.Add(1 * time.Hour), Updated: now, Order: sortprefOrderVal(1), Tags: []string{"y"}},
		{ID: "b", Description: "beta", Created: now, Updated: now.Add(1 * time.Hour), Order: nil, Tags: nil},
	}
	spec, _ := ParseSortSpec("alpha")
	got := ApplySort(items, spec)
	if len(got) != 3 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].ID != "a" || got[1].ID != "b" || got[2].ID != "c" {
		t.Errorf("by alpha: got %s, %s, %s want a, b, c", got[0].ID, got[1].ID, got[2].ID)
	}
}

func TestApplySort_created_asc(t *testing.T) {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	items := []*Item{
		{ID: "late", Created: base.Add(2 * time.Hour), Updated: base},
		{ID: "early", Created: base, Updated: base},
	}
	spec, _ := ParseSortSpec("created")
	got := ApplySort(items, spec)
	if got[0].ID != "early" || got[1].ID != "late" {
		t.Errorf("created asc: got %s, %s", got[0].ID, got[1].ID)
	}
}

func TestApplySort_updated_desc(t *testing.T) {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	items := []*Item{
		{ID: "old", Created: base, Updated: base},
		{ID: "new", Created: base, Updated: base.Add(1 * time.Hour)},
	}
	spec, _ := ParseSortSpec("updated:desc")
	got := ApplySort(items, spec)
	if got[0].ID != "new" || got[1].ID != "old" {
		t.Errorf("updated desc: got %s, %s", got[0].ID, got[1].ID)
	}
}

func TestApplySort_priority(t *testing.T) {
	now := time.Now().UTC()
	items := []*Item{
		{ID: "high", Order: sortprefOrderVal(1), Created: now, Updated: now},
		{ID: "low", Order: sortprefOrderVal(10), Created: now, Updated: now},
		{ID: "none", Order: nil, Created: now, Updated: now},
	}
	spec, _ := ParseSortSpec("priority")
	got := ApplySort(items, spec)
	if got[0].ID != "high" || got[1].ID != "low" || got[2].ID != "none" {
		t.Errorf("priority asc: got %v", ids(got))
	}
}

func TestApplySort_empty_spec(t *testing.T) {
	items := []*Item{{ID: "a"}, {ID: "b"}}
	got := ApplySort(items, nil)
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Errorf("empty spec should preserve order: %v", ids(got))
	}
}

func ids(items []*Item) []string {
	var s []string
	for _, it := range items {
		s = append(s, it.ID)
	}
	return s
}
