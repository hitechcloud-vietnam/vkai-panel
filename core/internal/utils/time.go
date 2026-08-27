package utils

import (
	"time"
)

// Time formats
const (
	TimeFormatISO8601  = "2006-01-02T15:04:05Z07:00"
	TimeFormatRFC3339  = "2006-01-02T15:04:05Z07:00"
	TimeFormatDate     = "2006-01-02"
	TimeFormatTime     = "15:04:05"
	TimeFormatDateTime = "2006-01-02 15:04:05"
	TimeFormatHuman    = "January 2, 2006 3:04 PM"
)

// Now returns current time in UTC
func Now() time.Time {
	return time.Now().UTC()
}

// FormatISO8601 formats time in ISO8601 format
func FormatISO8601(t time.Time) string {
	return t.UTC().Format(TimeFormatISO8601)
}

// FormatDate formats time as date only
func FormatDate(t time.Time) string {
	return t.UTC().Format(TimeFormatDate)
}

// FormatTime formats time as time only
func FormatTime(t time.Time) string {
	return t.UTC().Format(TimeFormatTime)
}

// FormatDateTime formats time as date and time
func FormatDateTime(t time.Time) string {
	return t.UTC().Format(TimeFormatDateTime)
}

// FormatHuman formats time in human-readable format
func FormatHuman(t time.Time) string {
	return t.UTC().Format(TimeFormatHuman)
}

// ParseISO8601 parses ISO8601 time string
func ParseISO8601(s string) (time.Time, error) {
	return time.Parse(TimeFormatISO8601, s)
}

// ParseDate parses date string
func ParseDate(s string) (time.Time, error) {
	return time.Parse(TimeFormatDate, s)
}

// ParseDateTime parses date-time string
func ParseDateTime(s string) (time.Time, error) {
	return time.Parse(TimeFormatDateTime, s)
}

// StartOfDay returns start of day (00:00:00)
func StartOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// EndOfDay returns end of day (23:59:59)
func EndOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, t.Location())
}

// StartOfWeek returns start of week (Monday)
func StartOfWeek(t time.Time) time.Time {
	weekday := t.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	return StartOfDay(t.AddDate(0, 0, -int(weekday-time.Monday)))
}

// EndOfWeek returns end of week (Sunday)
func EndOfWeek(t time.Time) time.Time {
	return EndOfDay(StartOfWeek(t).AddDate(0, 0, 6))
}

// StartOfMonth returns start of month
func StartOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

// EndOfMonth returns end of month
func EndOfMonth(t time.Time) time.Time {
	return StartOfMonth(t).AddDate(0, 1, 0).Add(-time.Nanosecond)
}

// StartOfYear returns start of year
func StartOfYear(t time.Time) time.Time {
	return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location())
}

// EndOfYear returns end of year
func EndOfYear(t time.Time) time.Time {
	return time.Date(t.Year(), 12, 31, 23, 59, 59, 999999999, t.Location())
}

// DaysBetween calculates days between two dates
func DaysBetween(t1, t2 time.Time) int {
	return int(t2.Sub(t1).Hours() / 24)
}

// HoursBetween calculates hours between two times
func HoursBetween(t1, t2 time.Time) int {
	return int(t2.Sub(t1).Hours())
}

// MinutesBetween calculates minutes between two times
func MinutesBetween(t1, t2 time.Time) int {
	return int(t2.Sub(t1).Minutes())
}

// IsExpired checks if a time is in the past
func IsExpired(t time.Time) bool {
	return t.Before(Now())
}

// IsFuture checks if a time is in the future
func IsFuture(t time.Time) bool {
	return t.After(Now())
}

// IsToday checks if a time is today
func IsToday(t time.Time) bool {
	now := Now()
	return t.Year() == now.Year() && t.Month() == now.Month() && t.Day() == now.Day()
}

// IsYesterday checks if a time is yesterday
func IsYesterday(t time.Time) bool {
	yesterday := Now().AddDate(0, 0, -1)
	return t.Year() == yesterday.Year() && t.Month() == yesterday.Month() && t.Day() == yesterday.Day()
}

// IsTomorrow checks if a time is tomorrow
func IsTomorrow(t time.Time) bool {
	tomorrow := Now().AddDate(0, 0, 1)
	return t.Year() == tomorrow.Year() && t.Month() == tomorrow.Month() && t.Day() == tomorrow.Day()
}

// RelativeTime returns a human-readable relative time string
func RelativeTime(t time.Time) string {
	now := Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		minutes := int(diff.Minutes())
		if minutes == 1 {
			return "1 minute ago"
		}
		return string(rune(minutes+'0')) + " minutes ago"
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return string(rune(hours+'0')) + " hours ago"
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return string(rune(days+'0')) + " days ago"
	case diff < 30*24*time.Hour:
		weeks := int(diff.Hours() / 24 / 7)
		if weeks == 1 {
			return "1 week ago"
		}
		return string(rune(weeks+'0')) + " weeks ago"
	case diff < 365*24*time.Hour:
		months := int(diff.Hours() / 24 / 30)
		if months == 1 {
			return "1 month ago"
		}
		return string(rune(months+'0')) + " months ago"
	default:
		years := int(diff.Hours() / 24 / 365)
		if years == 1 {
			return "1 year ago"
		}
		return string(rune(years+'0')) + " years ago"
	}
}
