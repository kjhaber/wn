package wn

import "testing"

func TestValidNoteName_AllowsWnPrefix(t *testing.T) {
	valid := []string{"wn:branch", "wn:pr", "wn:reviewer"}
	for _, name := range valid {
		if !ValidNoteName(name) {
			t.Errorf("ValidNoteName(%q) = false, want true", name)
		}
	}
	invalid := []string{
		"wn:",                                    // empty suffix
		"foo:bar",                                // not the wn: prefix
		"wn:bad name",                            // space in suffix
		"wn:UPPER",                               // uppercase not allowed in suffix
		"wn:" + "toolong12345678901234567890123", // over 32 total chars (32 chars total is max)
	}
	for _, name := range invalid {
		if ValidNoteName(name) {
			t.Errorf("ValidNoteName(%q) = true, want false", name)
		}
	}
}

func TestValidateSpecialNote_KnownName(t *testing.T) {
	for _, name := range []string{"wn:branch", "wn:commit"} {
		if err := ValidateSpecialNote(name); err != nil {
			t.Errorf("ValidateSpecialNote(%q) = %v, want nil", name, err)
		}
	}
}

func TestValidateSpecialNote_UnknownName(t *testing.T) {
	if err := ValidateSpecialNote("wn:unknown"); err == nil {
		t.Error("ValidateSpecialNote(wn:unknown) = nil, want error")
	}
}

func TestValidateSpecialNote_NotSpecial(t *testing.T) {
	// Non-wn: names are not special and should not be rejected
	if err := ValidateSpecialNote("pr-url"); err != nil {
		t.Errorf("ValidateSpecialNote(pr-url) = %v, want nil", err)
	}
}
