package pages

import (
	"fmt"
	"time"
)

// RelTime renders an Arabic relative time for notification timestamps.
func RelTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "الآن"
	case d < time.Hour:
		return fmt.Sprintf("منذ %d دقيقة", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("منذ %d ساعة", int(d.Hours()))
	default:
		return fmt.Sprintf("منذ %d يوم", int(d.Hours()/24))
	}
}
