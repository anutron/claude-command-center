package automation

import "time"

// isDue determines whether an automation should run based on its schedule,
// the time it last ran, and the current time.
func isDue(schedule string, lastRun time.Time, now time.Time) bool {
	switch schedule {
	case "every_refresh":
		return true

	case "daily":
		// Due if lastRun is before today's midnight (start of day).
		todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return lastRun.Before(todayStart)

	case "daily_9am":
		// Due if lastRun is before today's 9 AM AND now is at or after 9 AM.
		today9am := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, now.Location())
		return lastRun.Before(today9am) && !now.Before(today9am)

	case "hourly":
		// Due if lastRun is more than 1 hour ago.
		return now.Sub(lastRun) >= time.Hour

	case "weekly_monday":
		return weeklyDue(lastRun, now, time.Monday)

	case "weekly_friday":
		return weeklyDue(lastRun, now, time.Friday)

	default:
		// Unknown schedule — skip.
		return false
	}
}

// weeklyDue returns true if today is the target weekday and no run has occurred
// since the most recent occurrence of that weekday (start of day).
func weeklyDue(lastRun, now time.Time, target time.Weekday) bool {
	if now.Weekday() != target {
		return false
	}
	// Start of today (which is the target weekday).
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return lastRun.Before(dayStart)
}
