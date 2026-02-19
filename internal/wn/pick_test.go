package wn

import (
	"os"
	"testing"
	"time"
)

func TestPickInteractive_EmptyList(t *testing.T) {
	id, err := PickInteractive(nil)
	if err != nil {
		t.Errorf("PickInteractive(nil) err = %v", err)
	}
	if id != "" {
		t.Errorf("PickInteractive(nil) = %q, want \"\"", id)
	}
	id, err = PickInteractive([]*Item{})
	if err != nil {
		t.Errorf("PickInteractive([]) err = %v", err)
	}
	if id != "" {
		t.Errorf("PickInteractive([]) = %q, want \"\"", id)
	}
}

func TestPickInteractive_NumberedChoice(t *testing.T) {
	// Force numbered path by making fzf unavailable
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	t.Cleanup(func() { os.Setenv("PATH", origPath) })

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })

	// Feed "1" then newline so pickNumbered selects the first item
	if _, err := w.WriteString("1\n"); err != nil {
		t.Fatal(err)
	}
	w.Close()

	items := []*Item{
		{ID: "only1", Description: "only item", Created: time.Now().UTC(), Updated: time.Now().UTC()},
	}
	id, err := PickInteractive(items)
	if err != nil {
		t.Errorf("PickInteractive(...) err = %v", err)
	}
	if id != "only1" {
		t.Errorf("PickInteractive(...) = %q, want \"only1\"", id)
	}
}
