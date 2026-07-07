package model

import (
	"fmt"
	"sort"
	"time"
)

const (
	MaxSchedules   = 50
	MaxStartTimes  = 4
	MaxZoneMinutes = 255
	MaxInterval    = 365
)

type ScheduleKind string

const (
	ScheduleWeekly   ScheduleKind = "weekly"
	ScheduleInterval ScheduleKind = "interval"
)

// Day-of-month restrictions, matching the original's semantics.
const (
	RestrictionNone = 0
	RestrictionOdd  = 1
	RestrictionEven = 2
)

type Schedule struct {
	Name    string       `json:"name"`
	Enabled bool         `json:"enabled"`
	Kind    ScheduleKind `json:"kind"`
	// Days is indexed by time.Weekday (0 = Sunday). Used when Kind == weekly.
	Days [7]bool `json:"days"`
	// Interval runs the schedule every N days (EpochDays % N == 0).
	// Used when Kind == interval.
	Interval      int  `json:"interval"`
	Restriction   int  `json:"restriction"`
	WeatherAdjust bool `json:"weatherAdjust"`
	// StartTimes are minutes since local midnight, at most MaxStartTimes.
	StartTimes []int `json:"startTimes"`
	// Durations holds minutes per zone (0 = skip zone), indexed by zone id.
	Durations []int `json:"durations"`
}

func (s *Schedule) Validate(numZones int) error {
	if len(s.Name) == 0 || len(s.Name) > maxNameLen {
		return fmt.Errorf("schedule name must be 1-%d characters", maxNameLen)
	}
	switch s.Kind {
	case ScheduleWeekly:
	case ScheduleInterval:
		if s.Interval < 1 || s.Interval > MaxInterval {
			return fmt.Errorf("interval must be 1-%d days", MaxInterval)
		}
	default:
		return fmt.Errorf("schedule kind must be %q or %q", ScheduleWeekly, ScheduleInterval)
	}
	if s.Restriction < RestrictionNone || s.Restriction > RestrictionEven {
		return fmt.Errorf("restriction must be 0 (none), 1 (odd) or 2 (even)")
	}
	if len(s.StartTimes) > MaxStartTimes {
		return fmt.Errorf("at most %d start times allowed", MaxStartTimes)
	}
	for _, t := range s.StartTimes {
		if t < 0 || t > 23*60+59 {
			return fmt.Errorf("start time %d out of range 0-1439", t)
		}
	}
	if len(s.Durations) > numZones {
		return fmt.Errorf("more durations (%d) than zones (%d)", len(s.Durations), numZones)
	}
	for i, d := range s.Durations {
		if d < 0 || d > MaxZoneMinutes {
			return fmt.Errorf("duration for zone %d out of range 0-%d", i, MaxZoneMinutes)
		}
	}
	return nil
}

// Normalize sorts start times and pads Durations to numZones entries.
func (s *Schedule) Normalize(numZones int) {
	sort.Ints(s.StartTimes)
	for len(s.Durations) < numZones {
		s.Durations = append(s.Durations, 0)
	}
	s.Durations = s.Durations[:numZones]
}

// RunsOn reports whether the schedule is due on the local calendar day
// containing t. This mirrors Schedule::IsRunToday from the original:
// interval schedules run when EpochDays %% Interval == 0; weekly schedules
// honor the odd/even day-of-month restriction and the weekday flags.
func (s *Schedule) RunsOn(t time.Time) bool {
	if !s.Enabled {
		return false
	}
	if s.Kind == ScheduleInterval {
		if s.Interval < 1 {
			return false
		}
		return EpochDays(t)%s.Interval == 0
	}
	if s.Restriction != RestrictionNone && t.Day()%2 != s.Restriction%2 {
		return false
	}
	return s.Days[int(t.Weekday())]
}

type NextRun struct {
	// Date is the local calendar date in ISO format (YYYY-MM-DD).
	Date string `json:"date"`
	// InDays is 0 for today, 1 for tomorrow, ...
	InDays int `json:"inDays"`
	// Times are the start times (minutes since midnight) on that day.
	Times []int `json:"times"`
}

// NextRunAfter finds the next day within 14 days on which the schedule will
// start, considering only start times still in the future when the day is
// today. Returns nil if there is no run within 14 days.
func (s *Schedule) NextRunAfter(now time.Time) *NextRun {
	if !s.Enabled || len(s.StartTimes) == 0 {
		return nil
	}
	nowMin := MinutesOfDay(now)
	for d := 0; d <= 14; d++ {
		day := now.AddDate(0, 0, d)
		if !s.RunsOn(day) {
			continue
		}
		times := make([]int, 0, len(s.StartTimes))
		for _, t := range s.StartTimes {
			if d == 0 && t <= nowMin {
				continue
			}
			times = append(times, t)
		}
		if len(times) == 0 {
			continue
		}
		sort.Ints(times)
		return &NextRun{Date: day.Format("2006-01-02"), InDays: d, Times: times}
	}
	return nil
}
