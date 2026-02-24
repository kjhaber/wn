package wn

import "testing"

func TestHasDescriptionBody(t *testing.T) {
	tests := []struct {
		desc string
		want bool
	}{
		{"one line", false},
		{"Title only", false},
		{"first\nsecond", true},
		{"Title\n\nBody here.", true},
		{"", false},
		{"\nonly second", true},
	}
	for _, tt := range tests {
		got := HasDescriptionBody(tt.desc)
		if got != tt.want {
			t.Errorf("HasDescriptionBody(%q) = %v, want %v", tt.desc, got, tt.want)
		}
	}
}

func TestFormatPrompt(t *testing.T) {
	tests := []struct {
		tpl     string
		content string
		want    string
	}{
		{"Please implement: {}", "add a feature", "Please implement: add a feature"},
		{"Work item: {}", "one line", "Work item: one line"},
		{"Please implement the following:\n\n{}", "Title\n\nBody.", "Please implement the following:\n\nTitle\n\nBody."},
		{"{}", "only content", "only content"},
		{"Prefix {} suffix", "x", "Prefix x suffix"},
	}
	for _, tt := range tests {
		got := FormatPrompt(tt.tpl, tt.content)
		if got != tt.want {
			t.Errorf("FormatPrompt(%q, %q) = %q, want %q", tt.tpl, tt.content, got, tt.want)
		}
	}
}

func TestPromptContent(t *testing.T) {
	tests := []struct {
		desc string
		want string
	}{
		{"one line only", "one line only"},
		{"Title\n\nBody paragraph.", "Title\n\nBody paragraph."},
		{"First\nSecond", "First\nSecond"},
	}
	for _, tt := range tests {
		got := PromptContent(tt.desc)
		if got != tt.want {
			t.Errorf("PromptContent(...) =\n%q\nwant\n%q", got, tt.want)
		}
	}
}
