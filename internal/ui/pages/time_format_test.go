package pages

import (
	"testing"
	"time"
)

func TestTimeFormat12Hour(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	// 2026-08-31 22:45:30 UTC -> 10:45 PM
	sampleNight := time.Date(2026, 8, 31, 22, 45, 30, 0, loc)
	// 2026-08-31 09:15:05 UTC -> 09:15 AM
	sampleMorning := time.Date(2026, 8, 31, 9, 15, 5, 0, loc)

	if got := FormatDateTime(sampleNight); got != "2026-08-31 10:45 PM" {
		t.Errorf("FormatDateTime(sampleNight) = %q, want '2026-08-31 10:45 PM'", got)
	}
	if got := FormatDateTime(sampleMorning); got != "2026-08-31 09:15 AM" {
		t.Errorf("FormatDateTime(sampleMorning) = %q, want '2026-08-31 09:15 AM'", got)
	}

	if got := FormatDateTimeSec(sampleNight); got != "2026-08-31 10:45:30 PM" {
		t.Errorf("FormatDateTimeSec(sampleNight) = %q, want '2026-08-31 10:45:30 PM'", got)
	}
	if got := FormatDateTimeSec(sampleMorning); got != "2026-08-31 09:15:05 AM" {
		t.Errorf("FormatDateTimeSec(sampleMorning) = %q, want '2026-08-31 09:15:05 AM'", got)
	}

	if got := FormatTime12(sampleNight); got != "10:45 PM" {
		t.Errorf("FormatTime12(sampleNight) = %q, want '10:45 PM'", got)
	}
	if got := FormatTime12(sampleMorning); got != "09:15 AM" {
		t.Errorf("FormatTime12(sampleMorning) = %q, want '09:15 AM'", got)
	}

	if got := FormatTimeSec12(sampleNight); got != "10:45:30 PM" {
		t.Errorf("FormatTimeSec12(sampleNight) = %q, want '10:45:30 PM'", got)
	}

	// Nullable tests
	var nilTime *time.Time
	if got := FormatDateTimePtr(nilTime); got != "" {
		t.Errorf("FormatDateTimePtr(nil) = %q, want ''", got)
	}
	if got := FormatDateTimePtr(&sampleNight); got != "2026-08-31 10:45 PM" {
		t.Errorf("FormatDateTimePtr(&sampleNight) = %q, want '2026-08-31 10:45 PM'", got)
	}
}

func TestFormatRelativeResetTime_12Hour(t *testing.T) {
	// Parsing 12-hour formatted string
	futureTime := time.Now().Add(2 * time.Hour).Format("2006-01-02 03:04 PM")
	res := FormatRelativeResetTime(futureTime)
	if res == "" {
		t.Errorf("FormatRelativeResetTime failed for 12-hour format string: %q", futureTime)
	}
}
