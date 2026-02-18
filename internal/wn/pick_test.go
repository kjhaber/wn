package wn

import (
	"testing"
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
