package wn

import (
	"os"
	"testing"
)

func TestRunEditorOnFile_Unset(t *testing.T) {
	orig := os.Getenv("EDITOR")
	os.Unsetenv("EDITOR")
	t.Cleanup(func() { os.Setenv("EDITOR", orig) })
	err := RunEditorOnFile("/tmp/any")
	if err != ErrEditorUnset {
		t.Errorf("err = %v, want ErrEditorUnset", err)
	}
}
