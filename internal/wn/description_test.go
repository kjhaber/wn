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

func TestPromptBody(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"only one line", "only one line"},
		{"Title\n\nAdd support for X.  This should be smart\nenough to collapse nested.", "Add support for X.  This should be smart\nenough to collapse nested."},
		{"Job exec expand/collapse\n\nAdd support for Expand/collapse tool calls.", "Add support for Expand/collapse tool calls."},
	}
	for _, tt := range tests {
		got := PromptBody(tt.in)
		if got != tt.want {
			t.Errorf("PromptBody(...) =\n%q\nwant\n%q", got, tt.want)
		}
	}
}
