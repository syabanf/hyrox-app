package domain

import (
	"fmt"
	"time"
)

// Date is a calendar date with no timezone, serialized as "YYYY-MM-DD".
//
// Attendance, leave and holidays are reasoned about in calendar days, not
// instants: "17 August is a holiday" is true regardless of what time it is.
// Keeping that distinct from time.Time is what stops a date drifting a day
// when it crosses a timezone boundary on its way to the database.
type Date string

const dateLayout = "2006-01-02"

// ParseDate validates and normalizes a calendar date.
func ParseDate(value string) (Date, error) {
	parsed, err := time.Parse(dateLayout, value)
	if err != nil {
		return "", fmt.Errorf("date must be YYYY-MM-DD, got %q", value)
	}
	return Date(parsed.Format(dateLayout)), nil
}

// DateOf is the calendar date an instant falls on, in the given location.
func DateOf(t time.Time, loc *time.Location) Date {
	if loc == nil {
		loc = time.UTC
	}
	return Date(t.In(loc).Format(dateLayout))
}

// IsValid reports whether the date parses.
func (d Date) IsValid() bool {
	_, err := time.Parse(dateLayout, string(d))
	return err == nil
}

// utc is the date at midnight UTC, which is where all date arithmetic happens
// so that adding a day never lands on a daylight-saving seam.
func (d Date) utc() time.Time {
	parsed, err := time.Parse(dateLayout, string(d))
	if err != nil {
		return time.Time{}
	}
	return parsed
}

// AddDays moves the date, accepting negative values.
func (d Date) AddDays(days int) Date {
	return Date(d.utc().AddDate(0, 0, days).Format(dateLayout))
}

// Before reports whether d falls before other.
func (d Date) Before(other Date) bool { return string(d) < string(other) }

// After reports whether d falls after other.
func (d Date) After(other Date) bool { return string(d) > string(other) }

// ISODayOfWeek returns 1 for Monday through 7 for Sunday, which is how shift
// patterns are keyed.
// Year is the calendar year, which is what a leave allowance is keyed by.
func (d Date) Year() int {
	t, err := time.Parse(dateLayout, string(d))
	if err != nil {
		return 0
	}
	return t.Year()
}

func (d Date) ISODayOfWeek() int {
	day := int(d.utc().Weekday())
	if day == 0 {
		return 7
	}
	return day
}

// IsWeekend reports Saturday or Sunday.
//
// This is deliberately the studio's whole definition of a non-working day.
// Moving to a six-day week is a policy decision, not something that should
// change as a side effect of touching this function.
func (d Date) IsWeekend() bool {
	day := d.utc().Weekday()
	return day == time.Saturday || day == time.Sunday
}

// At combines the date with a wall-clock time in a location, which is how a
// shift's "08:00" becomes an instant.
func (d Date) At(clock TimeOfDay, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	base := d.utc()
	return time.Date(base.Year(), base.Month(), base.Day(),
		clock.Hour, clock.Minute, clock.Second, 0, loc)
}

// DatesBetween lists every date in an inclusive range. An end before the start
// yields nothing rather than an error: an empty range is a legitimate answer.
func DatesBetween(start, end Date) []Date {
	if start.After(end) {
		return nil
	}
	var out []Date
	for cursor := start; !cursor.After(end); cursor = cursor.AddDays(1) {
		out = append(out, cursor)
	}
	return out
}

// TimeOfDay is a wall-clock time with no date, serialized as "HH:MM" or
// "HH:MM:SS". Shift boundaries are wall-clock: an 08:00 shift starts at eight
// in the morning whatever the date.
type TimeOfDay struct {
	Hour   int
	Minute int
	Second int
}

// ParseTimeOfDay accepts "HH:MM" and "HH:MM:SS".
func ParseTimeOfDay(value string) (TimeOfDay, error) {
	for _, layout := range []string{"15:04:05", "15:04"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return TimeOfDay{Hour: parsed.Hour(), Minute: parsed.Minute(), Second: parsed.Second()}, nil
		}
	}
	return TimeOfDay{}, fmt.Errorf("time must be HH:MM or HH:MM:SS, got %q", value)
}

// String renders "HH:MM:SS", the form PostgreSQL's time column round-trips.
func (t TimeOfDay) String() string {
	return fmt.Sprintf("%02d:%02d:%02d", t.Hour, t.Minute, t.Second)
}

// MarshalJSON keeps the wire format a plain string.
func (t TimeOfDay) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.String() + `"`), nil
}

// UnmarshalJSON accepts either wall-clock form.
func (t *TimeOfDay) UnmarshalJSON(data []byte) error {
	raw := string(data)
	if len(raw) < 2 || raw[0] != '"' {
		return fmt.Errorf("time must be a JSON string, got %s", raw)
	}
	parsed, err := ParseTimeOfDay(raw[1 : len(raw)-1])
	if err != nil {
		return err
	}
	*t = parsed
	return nil
}

// DaysBetween is how many whole days separate two dates, positive when `to`
// falls after `from`.
//
// The arithmetic happens at midnight UTC, so a span never gains or loses a day
// crossing a daylight-saving seam — which is why an expiry counted in days is
// the same number wherever the server happens to be running.
func DaysBetween(from, to Date) int {
	return int(to.utc().Sub(from.utc()).Hours() / 24)
}
