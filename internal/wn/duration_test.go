package wn

import "testing"

func TestParseDurationWithDays_rejectsEmpty(t *testing.T) {
	if _, err := ParseDurationWithDays(""); err == nil {
		t.Fatal("ParseDurationWithDays(\"\") = nil error, want non-nil")
	}
}

func TestParseDurationWithDays_supportsHoursAndMinutes(t *testing.T) {
	d, err := ParseDurationWithDays("2h30m")
	if err != nil {
		t.Fatalf("ParseDurationWithDays(2h30m) err = %v", err)
	}
	// 2h30m = 150 minutes
	if got, want := d.Minutes(), float64(150); got != want {
		t.Errorf("Minutes = %v, want %v", got, want)
	}
}

func TestParseDurationWithDays_supportsDaysSuffix(t *testing.T) {
	d, err := ParseDurationWithDays("2d")
	if err != nil {
		t.Fatalf("ParseDurationWithDays(2d) err = %v", err)
	}
	// 2d = 48h
	if got, want := d.Hours(), float64(48); got != want {
		t.Errorf("Hours = %v, want %v", got, want)
	}
}
