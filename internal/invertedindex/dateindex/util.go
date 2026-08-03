package dateindex

import "time"

// ParseDate parses "YYYY-MM-DD" into time.Time (UTC, midnight).
func ParseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

// DayFromTime converts a time.Time to int32 days since Unix epoch.
func DayFromTime(t time.Time) int32 {
	return int32(t.Unix() / 86400)
}

// DayFromString is a convenience: ParseDate + DayFromTime.
func DayFromString(s string) (int32, error) {
	t, err := ParseDate(s)
	if err != nil {
		return 0, err
	}
	return DayFromTime(t), nil
}
