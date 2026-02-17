package wn

import "testing"

func TestValidateTag(t *testing.T) {
	tests := []struct {
		tag string
		ok  bool
	}{
		{"a", true},
		{"a1", true},
		{"a-b", true},
		{"a_b", true},
		{"", false},
		{"a b", false},
		{"a.b", false},
		{"x", true},
	}
	for _, tt := range tests {
		err := ValidateTag(tt.tag)
		if tt.ok && err != nil {
			t.Errorf("ValidateTag(%q) = %v", tt.tag, err)
		}
		if !tt.ok && err == nil {
			t.Errorf("ValidateTag(%q) expected error", tt.tag)
		}
	}
}

func TestValidateTag_TooLong(t *testing.T) {
	b := make([]byte, 33)
	for i := range b {
		b[i] = 'a'
	}
	if err := ValidateTag(string(b)); err != ErrTagTooLong {
		t.Errorf("ValidateTag(33 chars) = %v", err)
	}
}
