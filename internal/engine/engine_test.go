package engine

import (
	"testing"
	"time"

	"sprinklergo/internal/model"
)

// ---- test doubles ----

type fakeSource struct{ cfg model.Config }

func (f *fakeSource) Snapshot() model.Config { return f.cfg.Clone() }

type clock struct{ t time.Time }

func (c *clock) Now() time.Time { return c.t }

type transition struct {
	at    time.Time
	state uint16
}

type recOut struct {
	clk   *clock
	trans []transition
}

func (r *recOut) Apply(s uint16) error {
	r.trans = append(r.trans, transition{r.clk.t, s})
	return nil
}

type logRow struct {
	start    time.Time
	zoneID   int
	seconds  int
	schedID  int
	seasonal int
	weather  int
}

type memLogger struct{ rows []logRow }

func (m *memLogger) LogZoneEvent(start time.Time, zoneID int, d time.Duration, scheduleID, seasonal, weather int) error {
	m.rows = append(m.rows, logRow{start, zoneID, int(d.Seconds()), scheduleID, seasonal, weather})
	return nil
}

// ---- helpers ----

func at(hh, mm int) time.Time {
	// 2026-07-07 is a Tuesday.
	return time.Date(2026, time.July, 7, hh, mm, 0, 0, time.Local)
}

func testConfig() model.Config {
	cfg := model.DefaultConfig()
	cfg.Settings.RunSchedules = true
	for i := range cfg.Zones {
		cfg.Zones[i].Enabled = i < 3
		cfg.Zones[i].Pump = false
	}
	return cfg
}

func everyDaySchedule(name string, startMin int, durations ...int) model.Schedule {
	s := model.Schedule{Name: name, Enabled: true, Kind: model.ScheduleWeekly, StartTimes: []int{startMin}}
	for i := range s.Days {
		s.Days[i] = true
	}
	s.Durations = durations
	return s
}

type rig struct {
	eng *Engine
	clk *clock
	out *recOut
	log *memLogger
	src *fakeSource
}

func newRig(cfg model.Config, start time.Time, weather func() int) *rig {
	clk := &clock{t: start}
	out := &recOut{clk: clk}
	log := &memLogger{}
	src := &fakeSource{cfg: cfg}
	eng := New(src, out, log, weather, clk.Now)
	eng.Tick() // initialization tick
	return &rig{eng: eng, clk: clk, out: out, log: log, src: src}
}

// stepTo advances the clock in one-minute ticks up to and including end.
func (r *rig) stepTo(end time.Time) {
	for r.clk.t.Before(end) {
		r.clk.t = r.clk.t.Add(time.Minute)
		r.eng.Tick()
	}
}

// stepSeconds advances the clock in one-second ticks (for pump pre/post).
func (r *rig) stepSeconds(end time.Time) {
	for r.clk.t.Before(end) {
		r.clk.t = r.clk.t.Add(time.Second)
		r.eng.Tick()
	}
}

func expectTransitions(t *testing.T, got []transition, want []transition) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d transitions, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if !got[i].at.Equal(want[i].at) || got[i].state != want[i].state {
			t.Errorf("transition %d: got %v/%04b, want %v/%04b",
				i, got[i].at, got[i].state, want[i].at, want[i].state)
		}
	}
}

// ---- tests ----

func TestScheduleRunsZonesSequentially(t *testing.T) {
	cfg := testConfig()
	cfg.Zones[0].Pump = true // zone 1 requires the master/pump output (bit 0)
	cfg.Schedules = []model.Schedule{everyDaySchedule("Morgens", 6*60, 10, 20, 0)}

	r := newRig(cfg, at(5, 0), nil)
	r.stepTo(at(6, 35))

	expectTransitions(t, r.out.trans, []transition{
		{at(5, 0), 0},        // boot: everything off
		{at(6, 0), 1<<1 | 1}, // zone 1 + pump
		{at(6, 10), 1 << 2},  // zone 2, no pump
		{at(6, 30), 0},       // done
	})

	if len(r.log.rows) != 2 {
		t.Fatalf("want 2 log rows, got %+v", r.log.rows)
	}
	z0, z1 := r.log.rows[0], r.log.rows[1]
	if z0.zoneID != 0 || z0.seconds != 600 || z0.schedID != 0 || z0.seasonal != 100 || z0.weather != 100 {
		t.Errorf("zone 1 log row wrong: %+v", z0)
	}
	if z1.zoneID != 1 || z1.seconds != 1200 || !z1.start.Equal(at(6, 10)) {
		t.Errorf("zone 2 log row wrong: %+v", z1)
	}
}

func TestStateDuringScheduleRun(t *testing.T) {
	cfg := testConfig()
	cfg.Schedules = []model.Schedule{everyDaySchedule("Morgens", 6*60, 10, 0, 0)}

	r := newRig(cfg, at(5, 59), nil)
	if st := r.eng.State(); st.Mode != "idle" || st.PendingEvents != 1 {
		t.Errorf("before run: %+v", st)
	}
	r.stepTo(at(6, 2))
	st := r.eng.State()
	if st.Mode != "schedule" || st.ZoneID != 0 || st.ScheduleID != 0 {
		t.Errorf("during run: %+v", st)
	}
	if st.RemainingSeconds != 8*60 {
		t.Errorf("remaining = %d, want 480", st.RemainingSeconds)
	}
	if !st.ZoneOn[0] || st.ZoneOn[1] {
		t.Errorf("zone states wrong: %+v", st.ZoneOn)
	}
}

func TestSeasonalAndWeatherAdjustment(t *testing.T) {
	for _, tc := range []struct {
		name        string
		seasonal    int
		weather     int
		wadj        bool
		duration    int
		wantSeconds int
		wantWeather int
	}{
		{"seasonal 50%", 50, 100, false, 10, 5 * 60, 100},
		{"weather 150%", 100, 150, true, 10, 15 * 60, 150},
		{"weather ignored without flag", 100, 150, false, 10, 10 * 60, 100},
		{"combined 50% * 150%", 50, 150, true, 10, 8 * 60, 150}, // (10*75+50)/100 = 8
		{"capped at 255min", 100, 150, true, 200, 255 * 60, 150},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testConfig()
			cfg.Settings.SeasonalAdjust = tc.seasonal
			s := everyDaySchedule("S", 6*60, tc.duration, 0, 0)
			s.WeatherAdjust = tc.wadj
			cfg.Schedules = []model.Schedule{s}

			r := newRig(cfg, at(5, 59), func() int { return tc.weather })
			r.stepTo(at(6, 0).Add(time.Duration(tc.wantSeconds)*time.Second + 2*time.Minute))

			if len(r.log.rows) != 1 {
				t.Fatalf("want 1 log row, got %+v", r.log.rows)
			}
			row := r.log.rows[0]
			if row.seconds != tc.wantSeconds {
				t.Errorf("ran %ds, want %ds", row.seconds, tc.wantSeconds)
			}
			if row.seasonal != tc.seasonal || row.weather != tc.wantWeather {
				t.Errorf("logged adjustments %d/%d, want %d/%d", row.seasonal, row.weather, tc.seasonal, tc.wantWeather)
			}
		})
	}
}

func TestCollidingSchedulesDeferred(t *testing.T) {
	cfg := testConfig()
	cfg.Schedules = []model.Schedule{
		everyDaySchedule("A", 6*60, 2, 0, 0),
		everyDaySchedule("B", 6*60, 0, 3, 0),
	}

	r := newRig(cfg, at(5, 59), nil)
	r.stepTo(at(6, 10))

	expectTransitions(t, r.out.trans, []transition{
		{at(5, 59), 0},
		{at(6, 0), 1 << 1}, // schedule A, zone 1
		{at(6, 2), 0},      // A done
		{at(6, 3), 1 << 2}, // schedule B deferred minute-by-minute, then zone 2
		{at(6, 6), 0},
	})
}

func TestManualZone(t *testing.T) {
	cfg := testConfig()
	cfg.Zones[1].Pump = true
	r := newRig(cfg, at(10, 0), nil)

	if err := r.eng.SetManualZone(1, true, 0); err != nil {
		t.Fatal(err)
	}
	st := r.eng.State()
	if st.Mode != "manual" || st.ZoneID != 1 || st.RemainingSeconds != -1 || !st.PumpOn {
		t.Errorf("manual state wrong: %+v", st)
	}

	r.clk.t = r.clk.t.Add(7 * time.Minute)
	if err := r.eng.SetManualZone(1, false, 0); err != nil {
		t.Fatal(err)
	}
	if st := r.eng.State(); st.Mode != "idle" || st.ZoneOn[1] {
		t.Errorf("after manual off: %+v", st)
	}
	if len(r.log.rows) != 1 {
		t.Fatalf("want 1 log row, got %+v", r.log.rows)
	}
	row := r.log.rows[0]
	if row.zoneID != 1 || row.seconds != 420 || row.schedID != ScheduleManual || row.seasonal != -1 {
		t.Errorf("manual log row wrong: %+v", row)
	}

	if err := r.eng.SetManualZone(99, true, 0); err == nil {
		t.Error("want error for out-of-range zone")
	}
}

func TestManualTimerExpires(t *testing.T) {
	r := newRig(testConfig(), at(10, 0), nil)
	if err := r.eng.SetManualZone(0, true, 5); err != nil {
		t.Fatal(err)
	}
	st := r.eng.State()
	if st.Mode != "manual" || st.RemainingSeconds != 5*60 {
		t.Fatalf("manual timer state wrong: %+v", st)
	}
	r.stepTo(at(10, 7))
	if st := r.eng.State(); st.Mode != "idle" || st.ZoneOn[0] {
		t.Errorf("manual timer must switch the zone off: %+v", st)
	}
	if len(r.log.rows) != 1 || r.log.rows[0].seconds != 5*60 || r.log.rows[0].schedID != ScheduleManual {
		t.Errorf("timed manual run log wrong: %+v", r.log.rows)
	}

	if err := r.eng.SetManualZone(0, true, 99999); err == nil {
		t.Error("want error for out-of-range timer")
	}
}

func TestQuickRunDurations(t *testing.T) {
	cfg := testConfig()
	r := newRig(cfg, at(10, 0), nil)

	if err := r.eng.QuickRunDurations([]int{5, 0, 3}); err != nil {
		t.Fatal(err)
	}
	r.stepTo(at(10, 15))

	expectTransitions(t, r.out.trans, []transition{
		{at(10, 0), 0},
		{at(10, 0), 1 << 1},
		{at(10, 5), 1 << 3}, // zone 2 has duration 0 and is skipped
		{at(10, 8), 0},
	})
	if len(r.log.rows) != 2 {
		t.Fatalf("want 2 log rows, got %+v", r.log.rows)
	}
	if r.log.rows[0].schedID != ScheduleQuick || r.log.rows[0].seasonal != -1 || r.log.rows[0].weather != -1 {
		t.Errorf("quick run must log without adjustments: %+v", r.log.rows[0])
	}

	if err := r.eng.QuickRunDurations([]int{999}); err == nil {
		t.Error("want error for out-of-range duration")
	}
}

func TestQuickRunScheduleAppliesAdjustments(t *testing.T) {
	cfg := testConfig()
	cfg.Settings.SeasonalAdjust = 50
	cfg.Schedules = []model.Schedule{everyDaySchedule("S", 20*60, 10, 0, 0)}

	r := newRig(cfg, at(10, 0), nil)
	if err := r.eng.QuickRunSchedule(0); err != nil {
		t.Fatal(err)
	}
	r.stepTo(at(10, 10))
	if len(r.log.rows) != 1 || r.log.rows[0].seconds != 5*60 || r.log.rows[0].schedID != 0 {
		t.Errorf("quick schedule run wrong: %+v", r.log.rows)
	}

	if err := r.eng.QuickRunSchedule(5); err == nil {
		t.Error("want error for out-of-range schedule")
	}
}

func TestRunSchedulesOffSuppressesRuns(t *testing.T) {
	cfg := testConfig()
	cfg.Settings.RunSchedules = false
	cfg.Schedules = []model.Schedule{everyDaySchedule("S", 6*60, 10, 0, 0)}

	r := newRig(cfg, at(5, 59), nil)
	if st := r.eng.State(); st.PendingEvents != 0 {
		t.Errorf("scheduler off: want no pending events, got %d", st.PendingEvents)
	}
	r.stepTo(at(6, 30))
	expectTransitions(t, r.out.trans, []transition{{at(5, 59), 0}})

	// Quick run must still work with the scheduler switched off.
	if err := r.eng.QuickRunDurations([]int{2}); err != nil {
		t.Fatal(err)
	}
	r.stepTo(at(6, 35))
	if len(r.log.rows) != 1 {
		t.Errorf("quick run with scheduler off failed: %+v", r.log.rows)
	}
}

func TestDisabledZoneSkipped(t *testing.T) {
	cfg := testConfig()
	cfg.Zones[0].Enabled = false
	cfg.Schedules = []model.Schedule{everyDaySchedule("S", 6*60, 10, 5, 0)}

	r := newRig(cfg, at(5, 59), nil)
	r.stepTo(at(6, 10))
	expectTransitions(t, r.out.trans, []transition{
		{at(5, 59), 0},
		{at(6, 0), 1 << 2}, // zone 1 disabled, zone 2 runs immediately
		{at(6, 5), 0},
	})
}

func TestRestartMidDayDoesNotRefirePastSchedules(t *testing.T) {
	cfg := testConfig()
	cfg.Schedules = []model.Schedule{everyDaySchedule("S", 6*60, 10, 0, 0)}

	r := newRig(cfg, at(12, 0), nil)
	if st := r.eng.State(); st.PendingEvents != 0 {
		t.Errorf("restart at noon must not queue the 6:00 run, got %d pending", st.PendingEvents)
	}
}

func TestMidnightReloadFiresMidnightSchedule(t *testing.T) {
	cfg := testConfig()
	cfg.Schedules = []model.Schedule{everyDaySchedule("Mitternacht", 0, 5, 0, 0)}

	r := newRig(cfg, at(23, 58), nil)
	r.stepTo(at(23, 59).Add(7 * time.Minute)) // cross midnight

	next := time.Date(2026, time.July, 8, 0, 0, 0, 0, time.Local)
	expectTransitions(t, r.out.trans, []transition{
		{at(23, 58), 0},
		{next, 1 << 1},
		{next.Add(5 * time.Minute), 0},
	})
}

func TestSoftReloadKeepsRunningSchedule(t *testing.T) {
	cfg := testConfig()
	cfg.Schedules = []model.Schedule{everyDaySchedule("S", 6*60, 30, 0, 0)}

	r := newRig(cfg, at(5, 59), nil)
	r.stepTo(at(6, 5))
	if st := r.eng.State(); st.Mode != "schedule" {
		t.Fatalf("expected running schedule, got %+v", st)
	}
	r.eng.Reload() // e.g. after renaming a zone
	st := r.eng.State()
	if st.Mode != "schedule" || !st.ZoneOn[0] {
		t.Errorf("soft reload must keep the run alive: %+v", st)
	}
	if len(r.log.rows) != 0 {
		t.Errorf("soft reload must not end the zone run: %+v", r.log.rows)
	}

	// An explicit stop ends the run and logs the partial duration.
	r.eng.StopAll()
	st = r.eng.State()
	if st.Mode != "idle" || st.ZoneOn[0] {
		t.Errorf("stop must end the run: %+v", st)
	}
	if len(r.log.rows) != 1 || r.log.rows[0].seconds != 5*60 {
		t.Errorf("partial run not logged: %+v", r.log.rows)
	}
}

func TestRainDelaySuppressesSchedulesOnly(t *testing.T) {
	cfg := testConfig()
	cfg.Schedules = []model.Schedule{everyDaySchedule("S", 6*60, 10, 0, 0)}
	cfg.RainDelayUntil = at(12, 0).Unix()

	r := newRig(cfg, at(5, 0), nil)
	r.stepTo(at(6, 30))
	expectTransitions(t, r.out.trans, []transition{{at(5, 0), 0}})
	if st := r.eng.State(); st.PendingEvents != 0 {
		t.Errorf("suppressed start must be consumed, got %d pending", st.PendingEvents)
	}

	// Manual watering is unaffected by the rain delay.
	if err := r.eng.SetManualZone(0, true, 0); err != nil {
		t.Fatal(err)
	}
	if st := r.eng.State(); st.Mode != "manual" || !st.ZoneOn[0] {
		t.Errorf("manual run must work during rain delay: %+v", st)
	}
	r.eng.SetManualZone(0, false, 0)

	// After the delay expires, later starts run normally.
	r.src.cfg.Schedules[0].StartTimes = []int{13 * 60}
	r.eng.Reload()
	r.stepTo(at(13, 15))
	last := r.out.trans[len(r.out.trans)-1]
	if len(r.log.rows) < 2 || last.state != 0 {
		t.Errorf("schedule after rain delay did not run: trans=%+v", r.out.trans)
	}
}

func TestRunCrossesMidnight(t *testing.T) {
	cfg := testConfig()
	cfg.Schedules = []model.Schedule{everyDaySchedule("Spät", 23*60+58, 5, 0, 0)}

	r := newRig(cfg, at(23, 57), nil)
	r.stepTo(at(23, 59).Add(8 * time.Minute)) // bis 00:07 des Folgetags

	next := time.Date(2026, time.July, 8, 0, 3, 0, 0, time.Local)
	expectTransitions(t, r.out.trans, []transition{
		{at(23, 57), 0},
		{at(23, 58), 1 << 1},
		{next, 0}, // runs through midnight, off at 00:03
	})
	if len(r.log.rows) != 1 || r.log.rows[0].seconds != 5*60 {
		t.Errorf("midnight-crossing run log wrong: %+v", r.log.rows)
	}
}

func TestCycleAndSoak(t *testing.T) {
	cfg := testConfig()
	s := everyDaySchedule("CS", 10*60, 4, 4, 0)
	s.CycleMaxMinutes = 2
	s.SoakMinutes = 3
	cfg.Schedules = []model.Schedule{s}

	r := newRig(cfg, at(9, 59), nil)
	r.stepTo(at(10, 12))

	// z1 10:00-02, z2 10:02-04, pause (z1 soaks until 10:05),
	// z1 10:05-07, z2 10:07-09 (soak already satisfied), off 10:09.
	expectTransitions(t, r.out.trans, []transition{
		{at(9, 59), 0},
		{at(10, 0), 1 << 1},
		{at(10, 2), 1 << 2},
		{at(10, 4), 0}, // soak pause
		{at(10, 5), 1 << 1},
		{at(10, 7), 1 << 2},
		{at(10, 9), 0},
	})
	if len(r.log.rows) != 4 {
		t.Fatalf("want 4 chunk log rows, got %+v", r.log.rows)
	}
	for i, row := range r.log.rows {
		if row.seconds != 120 {
			t.Errorf("chunk %d: %ds, want 120s", i, row.seconds)
		}
	}
}

func TestSoakStateIsVisible(t *testing.T) {
	cfg := testConfig()
	s := everyDaySchedule("CS", 10*60, 4, 0, 0)
	s.CycleMaxMinutes = 2
	s.SoakMinutes = 5
	cfg.Schedules = []model.Schedule{s}

	r := newRig(cfg, at(9, 59), nil)
	r.stepTo(at(10, 3)) // chunk 1 ended 10:02, soak until 10:07
	st := r.eng.State()
	if st.Mode != "soaking" || st.ScheduleID != 0 {
		t.Fatalf("want soaking state, got %+v", st)
	}
	if st.RemainingSeconds != 4*60 {
		t.Errorf("remaining until next cycle = %d, want 240", st.RemainingSeconds)
	}
}

func TestPumpPreAndPostRun(t *testing.T) {
	cfg := testConfig()
	cfg.Settings.PumpPreSeconds = 30
	cfg.Settings.PumpPostSeconds = 45
	cfg.Zones[0].Pump = true

	r := newRig(cfg, at(10, 0), nil)
	if err := r.eng.SetManualZone(0, true, 0); err != nil {
		t.Fatal(err)
	}
	// Pump alone first, zone valve after the 30s pre-run.
	r.stepSeconds(at(10, 1))
	off := at(10, 5)
	r.clk.t = off
	r.eng.Tick()
	if err := r.eng.SetManualZone(0, false, 0); err != nil {
		t.Fatal(err)
	}
	// Zone valve closes, pump finishes its 45s post-run.
	r.stepSeconds(off.Add(time.Minute))

	expectTransitions(t, r.out.trans, []transition{
		{at(10, 0), 0},
		{at(10, 0), 1},                          // pump pre-run
		{at(10, 0).Add(30 * time.Second), 0b11}, // zone 1 + pump
		{off, 1},                                // zone off, pump post-run
		{off.Add(45 * time.Second), 0},
	})
}

func TestMonthlySeasonalProfile(t *testing.T) {
	cfg := testConfig()
	cfg.Settings.SeasonalMode = "monthly"
	cfg.Settings.SeasonalMonthly = model.DefaultSeasonalMonthly()
	cfg.Settings.SeasonalMonthly[6] = 50 // Juli
	cfg.Settings.SeasonalAdjust = 100
	cfg.Schedules = []model.Schedule{everyDaySchedule("S", 6*60, 10, 0, 0)}

	r := newRig(cfg, at(5, 59), nil) // test clock runs in July
	r.stepTo(at(6, 30))
	if len(r.log.rows) != 1 || r.log.rows[0].seconds != 5*60 || r.log.rows[0].seasonal != 50 {
		t.Errorf("monthly profile not applied: %+v", r.log.rows)
	}
}

func TestWaitingQueueIsVisible(t *testing.T) {
	cfg := testConfig()
	cfg.Schedules = []model.Schedule{
		everyDaySchedule("A", 6*60, 5, 0, 0),
		everyDaySchedule("B", 6*60, 0, 3, 0),
	}
	r := newRig(cfg, at(5, 59), nil)
	r.stepTo(at(6, 2)) // A runs, B waits
	st := r.eng.State()
	if len(st.Planned) != 1 || st.Planned[0].ScheduleID != 1 || !st.Planned[0].Waiting {
		t.Fatalf("B must be visible as waiting: %+v", st.Planned)
	}
	r.stepTo(at(6, 10))
	if len(r.log.rows) != 2 {
		t.Errorf("B must run after A: %+v", r.log.rows)
	}
}

func TestIntervalScheduleViaEngine(t *testing.T) {
	cfg := testConfig()
	s := model.Schedule{
		Name: "Intervall", Enabled: true, Kind: model.ScheduleInterval, Interval: 2,
		StartTimes: []int{6 * 60}, Durations: []int{5, 0, 0},
	}
	cfg.Schedules = []model.Schedule{s}

	// Find a day (today or tomorrow) on which the interval matches.
	day := at(5, 59)
	if model.EpochDays(day)%2 != 0 {
		day = day.AddDate(0, 0, 1)
	}
	r := newRig(cfg, day, nil)
	if st := r.eng.State(); st.PendingEvents != 1 {
		t.Errorf("interval day: want 1 pending event, got %d", st.PendingEvents)
	}

	r2 := newRig(cfg, day.AddDate(0, 0, 1), nil)
	if st := r2.eng.State(); st.PendingEvents != 0 {
		t.Errorf("off day: want 0 pending events, got %d", st.PendingEvents)
	}
}
