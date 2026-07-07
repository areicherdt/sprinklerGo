package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"sprinklergo/internal/model"
)

func TestConfigFirstBootAndRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s, err := OpenConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file not created on first boot: %v", err)
	}

	err = s.Update(func(c *model.Config) error {
		c.Zones[2].Name = "Beet"
		c.Zones[2].Enabled = true
		c.Settings.SeasonalAdjust = 80
		c.Schedules = append(c.Schedules, model.Schedule{
			Name: "Morgens", Enabled: true, Kind: model.ScheduleWeekly,
			StartTimes: []int{360}, Durations: make([]int, len(c.Zones)),
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Reopen and verify persistence.
	s2, err := OpenConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := s2.Snapshot()
	if cfg.Zones[2].Name != "Beet" || cfg.Settings.SeasonalAdjust != 80 || len(cfg.Schedules) != 1 {
		t.Errorf("persisted config mismatch: %+v", cfg)
	}
}

func TestConfigUpdateRejectsInvalid(t *testing.T) {
	s, err := OpenConfig(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	err = s.Update(func(c *model.Config) error {
		c.Settings.SeasonalAdjust = 999
		return nil
	})
	if err == nil {
		t.Fatal("want validation error")
	}
	if s.Snapshot().Settings.SeasonalAdjust != 100 {
		t.Error("failed update must not change state")
	}
}

func TestSnapshotIsDeepCopy(t *testing.T) {
	s, err := OpenConfig(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	snap := s.Snapshot()
	snap.Zones[0].Name = "mutated"
	if s.Snapshot().Zones[0].Name == "mutated" {
		t.Error("Snapshot must return a deep copy")
	}
}

func TestLogStore(t *testing.T) {
	l, err := OpenLog(filepath.Join(t.TempDir(), "zonelog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	base := time.Date(2026, time.July, 7, 6, 0, 0, 0, time.Local)
	if err := l.LogZoneEvent(base, 0, 10*time.Minute, 0, 100, 100); err != nil {
		t.Fatal(err)
	}
	if err := l.LogZoneEvent(base.Add(10*time.Minute), 1, 5*time.Minute, 0, 100, 100); err != nil {
		t.Fatal(err)
	}
	if err := l.LogZoneEvent(base.Add(24*time.Hour), 0, 3*time.Minute, LogScheduleManual, -1, -1); err != nil {
		t.Fatal(err)
	}

	entries, err := l.Entries(base.Add(-time.Hour), base.Add(48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("want 3 entries, got %d", len(entries))
	}
	if entries[0].Start < entries[1].Start {
		t.Error("entries must be newest first")
	}
	if entries[0].ScheduleID != LogScheduleManual {
		t.Errorf("want manual schedule id, got %d", entries[0].ScheduleID)
	}

	// Range filter.
	entries, err = l.Entries(base.Add(-time.Hour), base.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries in range, got %d", len(entries))
	}

	// Hourly grouping: zone 0 has runs at 6:00 on two days => one bucket, 780s.
	series, err := l.Grouped(base.Add(-time.Hour), base.Add(48*time.Hour), GroupHour)
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 2 {
		t.Fatalf("want 2 zone series, got %d", len(series))
	}
	if series[0].ZoneID != 0 || len(series[0].Buckets) != 1 || series[0].Buckets[0].Bucket != 6 || series[0].Buckets[0].Seconds != 780 {
		t.Errorf("zone 0 hourly buckets wrong: %+v", series[0])
	}

	if _, err := l.Grouped(base, base, Grouping("bogus")); err == nil {
		t.Error("want error for unknown grouping")
	}
}
