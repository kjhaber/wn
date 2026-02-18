package wn

import (
	"os"
	"testing"
)

func TestSplitEditorArgs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"single", "vim", []string{"vim"}},
		{"two args", "vim -f", []string{"vim", "-f"}},
		{"multiple args", "code --wait", []string{"code", "--wait"}},
		{"double-quoted path", `"gvim -f"`, []string{"gvim -f"}},
		{"single-quoted path", `'code --wait'`, []string{"code --wait"}},
		{"multiple spaces", "vim   -f", []string{"vim", "-f"}},
		{"empty", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitEditorArgs(tt.in)
			if len(got) != len(tt.want) {
				t.Errorf("splitEditorArgs(%q) = %v, want %v", tt.in, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitEditorArgs(%q)[%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestEditWithEditor_Unset(t *testing.T) {
	orig := os.Getenv("EDITOR")
	os.Unsetenv("EDITOR")
	t.Cleanup(func() { os.Setenv("EDITOR", orig) })
	_, err := EditWithEditor("hello")
	if err == nil {
		t.Fatal("expected error when EDITOR unset")
	}
	if err != ErrEditorUnset {
		t.Errorf("err = %v, want ErrEditorUnset", err)
	}
}
