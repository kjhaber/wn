package wn

import "testing"

func TestResolveItemID(t *testing.T) {
	tests := []struct {
		current, explicit string
		wantID            string
		wantErr           bool
	}{
		{"cur", "", "cur", false},
		{"cur", "abc", "abc", false},
		{"", "abc", "abc", false},
		{"", "", "", true},
	}
	for _, tt := range tests {
		got, err := ResolveItemID(tt.current, tt.explicit)
		if (err != nil) != tt.wantErr {
			t.Errorf("ResolveItemID(%q, %q) err = %v, wantErr %v", tt.current, tt.explicit, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.wantID {
			t.Errorf("ResolveItemID(%q, %q) = %q, want %q", tt.current, tt.explicit, got, tt.wantID)
		}
	}
}
