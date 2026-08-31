package pages

import (
	"time"
)

// Standard 12-hour datetime layouts with PM/AM.
const (
	LayoutDateTime12    = "2006-01-02 03:04 PM"
	LayoutDateTimeSec12 = "2006-01-02 03:04:05 PM"
	LayoutTime12        = "03:04 PM"
	LayoutTimeSec12     = "03:04:05 PM"
	LayoutDateOnly      = "2006-01-02"
)

// FormatDateTime formats a timestamp in 12-hour format: YYYY-MM-DD hh:mm PM.
func FormatDateTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(LayoutDateTime12)
}

// FormatDateTimePtr formats a pointer timestamp in 12-hour format: YYYY-MM-DD hh:mm PM.
func FormatDateTimePtr(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format(LayoutDateTime12)
}

// FormatDateTimeSec formats a timestamp in 12-hour format with seconds: YYYY-MM-DD hh:mm:ss PM.
func FormatDateTimeSec(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(LayoutDateTimeSec12)
}

// FormatTime12 formats time-only in 12-hour format: hh:mm PM.
func FormatTime12(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(LayoutTime12)
}

// FormatTimeSec12 formats time-only with seconds in 12-hour format: hh:mm:ss PM.
func FormatTimeSec12(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(LayoutTimeSec12)
}
