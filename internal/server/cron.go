// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule is a minimal 5-field cron expression: minute hour day-of-month
// month day-of-week. Implemented in-package (STACK.md lists no cron
// library): fields support "*", lists ("a,b"), ranges ("a-b") and steps
// ("*/5", "1-30/10"). Day-of-week accepts 0-7 with both 0 and 7 meaning
// Sunday. If both day-of-month and day-of-week are restricted, either
// matching fires the job (the classic vixie-cron rule).
type Schedule struct {
	minute [60]bool
	hour   [24]bool
	dom    [32]bool // index 0 unused
	month  [13]bool // index 0 unused
	dow    [8]bool  // 0..7, 0 and 7 both Sunday

	domRestricted bool
	dowRestricted bool
}

// ParseSchedule parses a 5-field cron expression. An empty expression is an
// error; callers treat an empty library schedule as "no schedule".
func ParseSchedule(expr string) (*Schedule, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("schedule %q: want 5 fields (minute hour dom mon dow)", expr)
	}
	s := &Schedule{}
	parsers := []struct {
		field      string
		lo, hi     int
		restricted *bool
		store      func(v int) bool
	}{
		{fields[0], 0, 59, nil, func(v int) bool { s.minute[v] = true; return true }},
		{fields[1], 0, 23, nil, func(v int) bool { s.hour[v] = true; return true }},
		{fields[2], 1, 31, &s.domRestricted, func(v int) bool { s.dom[v] = true; return true }},
		{fields[3], 1, 12, nil, func(v int) bool { s.month[v] = true; return true }},
		{fields[4], 0, 7, &s.dowRestricted, func(v int) bool {
			s.dow[v%7] = true // 7 == 0, both Sunday
			return true
		}},
	}
	for _, p := range parsers {
		if err := parseField(p.field, p.lo, p.hi, p.store); err != nil {
			return nil, fmt.Errorf("schedule %q field %q: %w", expr, p.field, err)
		}
		if p.field != "*" && p.restricted != nil {
			*p.restricted = true
		}
	}
	return s, nil
}

// parseField parses one cron field into the setter.
func parseField(field string, lo, hi int, set func(int) bool) error {
	for _, part := range strings.Split(field, ",") {
		rangePart, stepPart, hasStep := strings.Cut(part, "/")
		step := 1
		if hasStep {
			n, err := strconv.Atoi(stepPart)
			if err != nil || n < 1 {
				return fmt.Errorf("invalid step %q", stepPart)
			}
			step = n
		}
		first, last := lo, hi
		if rangePart != "*" {
			a, b, ok := strings.Cut(rangePart, "-")
			x, err := strconv.Atoi(a)
			if err != nil {
				return fmt.Errorf("invalid value %q", a)
			}
			if ok {
				y, err := strconv.Atoi(b)
				if err != nil {
					return fmt.Errorf("invalid range end %q", b)
				}
				first, last = x, y
			} else {
				if hasStep {
					return fmt.Errorf("step on a single value %q", part)
				}
				first, last = x, x
			}
		}
		if first < lo || last > hi || first > last {
			return fmt.Errorf("value out of range %q: want %d-%d", part, lo, hi)
		}
		for v := first; v <= last; v += step {
			set(v)
		}
	}
	return nil
}

// Match reports whether the schedule fires at time t (minute resolution).
func (s *Schedule) Match(t time.Time) bool {
	if !s.minute[t.Minute()] || !s.hour[t.Hour()] || !s.month[int(t.Month())] {
		return false
	}
	domOK := s.dom[t.Day()]
	dowOK := s.dow[int(t.Weekday())%7]
	switch {
	case s.domRestricted && s.dowRestricted:
		return domOK || dowOK // vixie-cron rule
	case s.domRestricted:
		return domOK
	case s.dowRestricted:
		return dowOK
	default:
		return true
	}
}

// Next returns the first time at or after t (exclusive of t itself) that the
// schedule fires, minute resolution, local time. The search is capped at
// one year; an unsatisfiable schedule returns the cap with an error.
func (s *Schedule) Next(after time.Time) (time.Time, error) {
	t := after.Truncate(time.Minute).Add(time.Minute)
	cap := after.AddDate(1, 0, 0)
	for ; t.Before(cap); t = t.Add(time.Minute) {
		if s.Match(t) {
			return t, nil
		}
	}
	return cap, fmt.Errorf("schedule fires less than once a year")
}
