// Package cron reads the five-field schedules of cron(8) and says when one
// next comes due. It is what gnosh's scheduled prompts (CronCreate, /loop
// with an interval) run on, as Claude Code's do: minute, hour, day of the
// month, month and day of the week, in the local time of the time asked
// about.
//
// Each field is a list of items parted by commas. An item is * (every
// value), a value, a range a-b, or either of those with a step: */15,
// 10-50/20, 5/15 (from 5 to the field's end). The day of the week runs
// from 0 to 7, both Sunday. As in Vixie cron, when both days are limited
// (neither is *), a date matches when either does. Names (MON, JAN) and
// the extensions L, W, ? and # are not read.
package cron

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule is a five-field cron schedule, read.
type Schedule struct {
	minute, hour, dom, month, dow uint64 // one bit per value that matches
	// domAny and dowAny say the field was *, alone: then the other day
	// field alone decides.
	domAny, dowAny bool
	text           string
}

// field is the range of one of the five fields.
type field struct {
	name     string
	min, max int
}

var fields = [5]field{
	{"minute", 0, 59},
	{"hour", 0, 23},
	{"day of the month", 1, 31},
	{"month", 1, 12},
	{"day of the week", 0, 7},
}

// Parse reads a schedule of five fields parted by blanks. The error says
// what is wrong with it, in words for a person.
func Parse(expr string) (*Schedule, error) {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return nil, fmt.Errorf("a schedule is five fields (minute, hour, day of the month, month, day of the week), and %q has %d", expr, len(parts))
	}
	s := &Schedule{text: strings.Join(parts, " ")}
	sets := [5]*uint64{&s.minute, &s.hour, &s.dom, &s.month, &s.dow}
	for i, p := range parts {
		bits, err := parseField(p, fields[i])
		if err != nil {
			return nil, err
		}
		*sets[i] = bits
	}
	s.domAny, s.dowAny = parts[2] == "*", parts[4] == "*"
	if s.dow&(1<<7) != 0 {
		s.dow |= 1 // 7 is Sunday, as 0 is
	}
	return s, nil
}

// parseField reads one field of a schedule into its bits.
func parseField(text string, f field) (uint64, error) {
	var bits uint64
	for _, item := range strings.Split(text, ",") {
		if item == "" {
			return 0, fmt.Errorf("the %s field %q has an empty item", f.name, text)
		}
		rng, stepText, stepped := strings.Cut(item, "/")
		step := 1
		if stepped {
			n, err := strconv.Atoi(stepText)
			if err != nil || n < 1 || n > f.max-f.min+1 {
				return 0, fmt.Errorf("the %s field %q has the step %q, which is no number from 1 to %d", f.name, text, stepText, f.max-f.min+1)
			}
			step = n
		}
		lo, hi := f.min, f.max
		switch {
		case rng == "*":
		default:
			a, b, isRange := strings.Cut(rng, "-")
			var err error
			if lo, err = value(a, f, text); err != nil {
				return 0, err
			}
			switch {
			case isRange:
				if hi, err = value(b, f, text); err != nil {
					return 0, err
				}
				if hi < lo {
					return 0, fmt.Errorf("the %s field %q has the range %s, which runs backwards", f.name, text, rng)
				}
			case !stepped:
				hi = lo
			}
		}
		for v := lo; v <= hi; v += step {
			bits |= 1 << v
		}
	}
	return bits, nil
}

// value reads one number of a field.
func value(text string, f field, whole string) (int, error) {
	n, err := strconv.Atoi(text)
	if err != nil || n < f.min || n > f.max {
		return 0, fmt.Errorf("the %s field %q has %q, which is no number from %d to %d", f.name, whole, text, f.min, f.max)
	}
	return n, nil
}

// String is the schedule as read, its fields parted by single blanks.
func (s *Schedule) String() string { return s.text }

// matchesDay reports whether t's date is one of the schedule's.
func (s *Schedule) matchesDay(t time.Time) bool {
	if s.month&(1<<uint(t.Month())) == 0 {
		return false
	}
	dom := s.dom&(1<<uint(t.Day())) != 0
	dow := s.dow&(1<<uint(t.Weekday())) != 0
	switch {
	case s.domAny && s.dowAny:
		return true
	case s.domAny:
		return dow
	case s.dowAny:
		return dom
	}
	return dom || dow
}

// ErrNever says a schedule never comes due: "0 0 30 2 *" asks for the
// thirtieth of February.
var ErrNever = errors.New("the schedule never comes due")

// horizon is how far Next looks: past the next leap day, whatever the
// date asked about.
const horizon = 5 * 366 * 24 * time.Hour

// Next is the first minute after t, in t's time zone, that the schedule
// matches: whole minutes, so a time just past one is not that minute
// again. It returns ErrNever for a schedule no date within five years
// matches.
func (s *Schedule) Next(t time.Time) (time.Time, error) {
	loc := t.Location()
	end := t.Add(horizon)
	c := t.Truncate(time.Minute).Add(time.Minute)
	for c.Before(end) {
		if !s.matchesDay(c) {
			// The next day, at its first minute.
			y, m, d := c.Date()
			c = time.Date(y, m, d+1, 0, 0, 0, 0, loc)
			continue
		}
		if s.hour&(1<<uint(c.Hour())) == 0 {
			y, m, d := c.Date()
			next := time.Date(y, m, d, c.Hour()+1, 0, 0, 0, loc)
			if !next.After(c) {
				// (A clock set back: the same hour again.)
				next = c.Truncate(time.Hour).Add(time.Hour)
			}
			c = next
			continue
		}
		if s.minute&(1<<uint(c.Minute())) == 0 {
			c = c.Add(time.Minute)
			continue
		}
		return c, nil
	}
	return time.Time{}, ErrNever
}

// Every is the schedule that comes due every d, d rounded to a step cron
// can take, and that step in words, with whether d had to be rounded:
// minutes that divide an hour (*/5), hours that divide a day (0 */2), or
// whole days (0 0 */3); seconds are rounded up to a minute. It is /loop's
// interval, as Claude Code's /loop reads it.
func Every(d time.Duration) (expr string, step time.Duration, rounded bool) {
	odd := d%time.Minute != 0 // seconds, rounded up to a minute
	if d < time.Minute {
		d = time.Minute
	}
	mins := int((d + time.Minute - 1) / time.Minute)
	switch {
	case mins < 60:
		n := nearest(mins, []int{1, 2, 3, 4, 5, 6, 10, 12, 15, 20, 30})
		expr = "*/" + strconv.Itoa(n) + " * * * *"
		if n == 1 {
			expr = "* * * * *"
		}
		return expr, time.Duration(n) * time.Minute, n != mins || odd
	case mins < 24*60:
		hours := (mins + 30) / 60
		n := nearest(hours, []int{1, 2, 3, 4, 6, 8, 12})
		expr = "0 */" + strconv.Itoa(n) + " * * *"
		if n == 1 {
			expr = "0 * * * *"
		}
		return expr, time.Duration(n) * time.Hour, n*60 != mins || odd
	}
	days := (mins + 12*60) / (24 * 60)
	n := min(days, 31)
	expr = "0 0 */" + strconv.Itoa(n) + " * *"
	if n == 1 {
		expr = "0 0 * * *"
	}
	return expr, time.Duration(n) * 24 * time.Hour, n*24*60 != mins || odd
}

// nearest is the value of steps closest to n, the smaller one on a tie.
func nearest(n int, steps []int) int {
	best := steps[0]
	for _, s := range steps {
		if abs(s-n) < abs(best-n) {
			best = s
		}
	}
	return best
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
