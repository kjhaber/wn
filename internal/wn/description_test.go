package wn

import "testing"

func TestFirstLine(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"one line", "one line"},
		{"first\nsecond", "first"},
		{"  trimmed  \n", "trimmed"},
		{"", ""},
		{"\nonly second", ""},
	}
	for _, tt := range tests {
		got := FirstLine(tt.in)
		if got != tt.want {
			t.Errorf("FirstLine(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
