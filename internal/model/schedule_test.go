package model

import (
	"testing"
	"time"
)

func date(y int, m time.Month, d, hh, mm int) time.Time {
	return time.Date(y, m, d, hh, mm, 0, 0, time.Local)
}

func TestEpochDays(t *testing.T) {
	if got := EpochDays(date(1970, time.January, 1, 12, 0)); got != 0 {
		t.Errorf("EpochDays(1970-01-01) = %d, want 0", got)
	}
	if got := EpochDays(date(1970, time.January, 2, 0, 0)); got != 1 {
		t.Errorf("EpochDays(1970-01-02) = %d, want 1", got)
	}
	// 2026-07-07 is 20641 days after the epoch.
	if got := EpochDays(date(2026, time.July, 7, 23, 59)); got != 20641 {
		t.Errorf("EpochDays(2026-07-07) = %d, want 20641", got)
	}
}

func TestRunsOnWeekly(t *testing.T) {
	s := Schedule{Enabled: true, Kind: ScheduleWeekly}
	s.Days[int(time.Monday)] = true
	s.Days[int(time.Thursday)] = true

	monday := date(2026, time.July, 6, 10, 0)   // a Monday
	tuesday := date(2026, time.July, 7, 10, 0)  // a Tuesday
	thursday := date(2026, time.July, 9, 10, 0) // a Thursday

	for _, tc := range []struct {
		day  time.Time
		want bool
	}{
		{monday, true},
		{tuesday, false},
		{thursday, true},
	} {
		if got := s.RunsOn(tc.day); got != tc.want {
			t.Errorf("RunsOn(%s) = %v, want %v", tc.day.Weekday(), got, tc.want)
		}
	}

	s.Enabled = false
	if s.RunsOn(monday) {
		t.Error("disabled schedule must never run")
	}
}

func TestRunsOnRestriction(t *testing.T) {
	s := Schedule{Enabled: true, Kind: ScheduleWeekly}
	for i := range s.Days {
		s.Days[i] = true
	}
	odd := date(2026, time.July, 7, 10, 0)  // day of month 7
	even := date(2026, time.July, 8, 10, 0) // day of month 8

	s.Restriction = RestrictionOdd
	if !s.RunsOn(odd) || s.RunsOn(even) {
		t.Error("odd restriction: want run on the 7th only")
	}
	s.Restriction = RestrictionEven
	if s.RunsOn(odd) || !s.RunsOn(even) {
		t.Error("even restriction: want run on the 8th only")
	}
	s.Restriction = RestrictionNone
	if !s.RunsOn(odd) || !s.RunsOn(even) {
		t.Error("no restriction: want run on both days")
	}
}

func TestRunsOnInterval(t *testing.T) {
	s := Schedule{Enabled: true, Kind: ScheduleInterval, Interval: 3}
	// Find a day where EpochDays % 3 == 0.
	day := date(2026, time.July, 7, 10, 0)
	for EpochDays(day)%3 != 0 {
		day = day.AddDate(0, 0, 1)
	}
	if !s.RunsOn(day) {
		t.Error("want run on interval day")
	}
	if s.RunsOn(day.AddDate(0, 0, 1)) || s.RunsOn(day.AddDate(0, 0, 2)) {
		t.Error("must not run between interval days")
	}
	if !s.RunsOn(day.AddDate(0, 0, 3)) {
		t.Error("want run again 3 days later")
	}
}

func TestNextRunAfter(t *testing.T) {
	s := Schedule{Enabled: true, Kind: ScheduleWeekly, StartTimes: []int{6 * 60, 18 * 60}}
	s.Days[int(time.Tuesday)] = true

	tuesdayMorning := date(2026, time.July, 7, 5, 0)
	nr := s.NextRunAfter(tuesdayMorning)
	if nr == nil || nr.InDays != 0 || len(nr.Times) != 2 {
		t.Fatalf("want both times today, got %+v", nr)
	}

	// After the first start time only the evening run remains today.
	nr = s.NextRunAfter(date(2026, time.July, 7, 12, 0))
	if nr == nil || nr.InDays != 0 || len(nr.Times) != 1 || nr.Times[0] != 18*60 {
		t.Fatalf("want evening run today, got %+v", nr)
	}

	// After the last start time the next run is a week away.
	nr = s.NextRunAfter(date(2026, time.July, 7, 20, 0))
	if nr == nil || nr.InDays != 7 {
		t.Fatalf("want next run in 7 days, got %+v", nr)
	}

	s2 := Schedule{Enabled: true, Kind: ScheduleWeekly, StartTimes: []int{360}}
	if nr := s2.NextRunAfter(tuesdayMorning); nr != nil {
		t.Errorf("schedule with no weekdays: want nil, got %+v", nr)
	}
}

func TestScheduleValidate(t *testing.T) {
	valid := Schedule{Name: "Rasen", Kind: ScheduleWeekly, StartTimes: []int{360}, Durations: []int{10, 0, 0}}
	if err := valid.Validate(3); err != nil {
		t.Errorf("valid schedule rejected: %v", err)
	}
	for name, s := range map[string]Schedule{
		"empty name":        {Kind: ScheduleWeekly},
		"bad kind":          {Name: "x", Kind: "daily"},
		"bad interval":      {Name: "x", Kind: ScheduleInterval, Interval: 0},
		"bad restriction":   {Name: "x", Kind: ScheduleWeekly, Restriction: 3},
		"too many times":    {Name: "x", Kind: ScheduleWeekly, StartTimes: []int{1, 2, 3, 4, 5}},
		"time out of range": {Name: "x", Kind: ScheduleWeekly, StartTimes: []int{1440}},
		"duration too long": {Name: "x", Kind: ScheduleWeekly, Durations: []int{256}},
	} {
		if err := s.Validate(3); err == nil {
			t.Errorf("%s: want validation error", name)
		}
	}
}

func TestNormalize(t *testing.T) {
	s := Schedule{StartTimes: []int{600, 300}, Durations: []int{1, 2, 3, 4, 5}}
	s.Normalize(3)
	if s.StartTimes[0] != 300 {
		t.Error("start times not sorted")
	}
	if len(s.Durations) != 3 {
		t.Errorf("durations not trimmed: %v", s.Durations)
	}
	s.Normalize(5)
	if len(s.Durations) != 5 || s.Durations[4] != 0 {
		t.Errorf("durations not padded: %v", s.Durations)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
	if len(cfg.Zones) != MaxZones || !cfg.Zones[0].Enabled || cfg.Zones[1].Enabled {
		t.Error("default zones must match original factory defaults")
	}
	if cfg.EnabledZones() != 1 {
		t.Errorf("EnabledZones = %d, want 1", cfg.EnabledZones())
	}
}
