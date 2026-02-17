package wn

import (
	"os"
	"testing"
)

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
