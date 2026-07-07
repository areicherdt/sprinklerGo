// Package engine ports the scheduling core of sprinklers_pi (core.cpp).
// It keeps the original's semantics: a per-day event list rebuilt at midnight
// and on every configuration change, strictly sequential zone runs, duration
// scaling by seasonal and weather percentages, and per-minute deferral when a
// schedule becomes due while another one is still running.
package engine

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"sprinklergo/internal/model"
)

// Schedule ids reported in run state and zone logs.
const (
	ScheduleManual = -1
	ScheduleQuick  = 99
)

// Output receives the 16-bit output state: bit 0 is the pump/master valve,
// bits 1..15 are the zones. Implemented by the hardware backends.
type Output interface {
	Apply(state uint16) error
}

type ConfigSource interface {
	Snapshot() model.Config
}

type ZoneLogger interface {
	LogZoneEvent(start time.Time, zoneID int, d time.Duration, scheduleID, seasonal, weather int) error
}

type eventKind uint8

const (
	evZoneOn     eventKind = iota + 1 // original command 0x01
	evAllOff                          // 0x02
	evStartSched                      // 0x03
)

type event struct {
	timeMin int // minutes since midnight; -1 = consumed
	kind    eventKind
	zone    int // 1-based valve number (evZoneOn)
	endMin  int // scheduled end, minutes since midnight (evZoneOn)
	sched   int // schedule index (evStartSched)
}

type adjustments struct {
	seasonal int // percent, -1 = not applied (manual/quick)
	weather  int
}

type runState struct {
	schedule  bool
	manual    bool
	schedID   int // schedule index, ScheduleQuick, or -1
	zone      int // 1-based valve number, -1 = none
	endMin    int
	eventTime time.Time // start of the current zone run (for logging)
	adj       adjustments
}

type Engine struct {
	cfg          ConfigSource
	logger       ZoneLogger
	weatherScale func() int // returns percent 0-200; nil = always 100
	now          func() time.Time

	mu          sync.Mutex
	out         Output
	events      []event
	run         runState
	outState    uint16
	prevOut     uint16
	havePrev    bool
	initialized bool
	lastDay     int
	quick       []int // quick-run durations, minutes per zone
}

func New(cfg ConfigSource, out Output, logger ZoneLogger, weatherScale func() int, now func() time.Time) *Engine {
	if now == nil {
		now = time.Now
	}
	return &Engine{
		cfg:          cfg,
		out:          out,
		logger:       logger,
		weatherScale: weatherScale,
		now:          now,
		run:          runState{schedID: -1, zone: -1, adj: adjustments{-1, -1}},
	}
}

// Start runs the 1-second tick loop until ctx is done, then shuts all
// outputs off.
func (e *Engine) Start(ctx context.Context) {
	e.Tick()
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				e.Shutdown()
				return
			case <-t.C:
				e.Tick()
			}
		}
	}()
}

// Tick advances the engine to the current clock time: it reloads the event
// list on day changes, fires due events and latches output changes.
func (e *Engine) Tick() {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	cfg := e.cfg.Snapshot()
	day := model.EpochDays(now)
	if !e.initialized {
		// Like the original's first loop: load only future events so a
		// restart at noon does not re-fire the morning schedules.
		e.initialized = true
		e.lastDay = day
		e.reloadLocked(false, now, &cfg)
	} else if day != e.lastDay {
		e.lastDay = day
		e.reloadLocked(true, now, &cfg)
	}
	e.processEventsLocked(now, &cfg)
	e.latchLocked()
}

// Reload rebuilds the day's event list (future start times only). Any
// running schedule or manual watering is stopped, matching the original's
// behavior after every configuration change.
func (e *Engine) Reload() {
	e.mu.Lock()
	defer e.mu.Unlock()
	cfg := e.cfg.Snapshot()
	e.reloadLocked(false, e.now(), &cfg)
	e.latchLocked()
}

// StopAll turns everything off and rebuilds the pending events.
func (e *Engine) StopAll() { e.Reload() }

// SetManualZone turns a single zone on (with its pump if configured) or
// turns all zones off. zoneID is 0-based.
func (e *Engine) SetManualZone(zoneID int, on bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	cfg := e.cfg.Snapshot()
	if zoneID < 0 || zoneID >= len(cfg.Zones) {
		return fmt.Errorf("zone %d out of range", zoneID)
	}
	now := e.now()
	if on {
		e.turnOnZoneLocked(zoneID+1, &cfg)
		e.setManualLocked(now, true, zoneID+1)
	} else {
		e.outState = 0
		e.setManualLocked(now, false, -1)
	}
	e.latchLocked()
	return nil
}

// QuickRunSchedule immediately runs an existing schedule (with weather and
// seasonal adjustment), stopping whatever is currently running.
func (e *Engine) QuickRunSchedule(idx int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	cfg := e.cfg.Snapshot()
	if idx < 0 || idx >= len(cfg.Schedules) {
		return fmt.Errorf("schedule %d out of range", idx)
	}
	now := e.now()
	e.reloadLocked(false, now, &cfg)
	e.startScheduleLocked(idx, false, now, &cfg)
	// The original fires ProcessEvents in the same loop pass, so the first
	// zone turns on immediately rather than on the next tick.
	e.processEventsLocked(now, &cfg)
	e.latchLocked()
	return nil
}

// QuickRunDurations immediately runs ad-hoc durations (minutes per zone),
// without any adjustment — like the original's quick schedule.
func (e *Engine) QuickRunDurations(durations []int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	cfg := e.cfg.Snapshot()
	if len(durations) > len(cfg.Zones) {
		return fmt.Errorf("more durations (%d) than zones (%d)", len(durations), len(cfg.Zones))
	}
	for i, d := range durations {
		if d < 0 || d > model.MaxZoneMinutes {
			return fmt.Errorf("duration for zone %d out of range 0-%d", i, model.MaxZoneMinutes)
		}
	}
	e.quick = slices.Clone(durations)
	for len(e.quick) < len(cfg.Zones) {
		e.quick = append(e.quick, 0)
	}
	now := e.now()
	e.reloadLocked(false, now, &cfg)
	e.startScheduleLocked(0, true, now, &cfg)
	e.processEventsLocked(now, &cfg)
	e.latchLocked()
	return nil
}

// SwapOutput replaces the hardware backend, transferring the current state.
func (e *Engine) SwapOutput(out Output) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.out = out
	e.havePrev = false // force re-apply on next latch
	e.latchLocked()
}

// Shutdown turns all outputs off and clears pending events.
func (e *Engine) Shutdown() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = nil
	e.clearRunLocked(e.now())
	e.outState = 0
	e.latchLocked()
}

// State is a point-in-time snapshot for the API layer.
type State struct {
	Mode             string // "idle", "schedule", "manual"
	ZoneID           int    // 0-based, -1 = none
	ScheduleID       int    // schedule index, ScheduleQuick, or -1
	RemainingSeconds int    // -1 = unlimited (manual)
	PendingEvents    int
	ZoneOn           []bool
	PumpOn           bool
}

func (e *Engine) State() State {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	cfg := e.cfg.Snapshot()
	st := State{Mode: "idle", ZoneID: -1, ScheduleID: -1, RemainingSeconds: 0}
	switch {
	case e.run.manual && e.run.zone >= 1:
		st.Mode = "manual"
		st.ZoneID = e.run.zone - 1
		st.RemainingSeconds = -1
	case e.run.schedule && e.run.zone >= 1:
		st.Mode = "schedule"
		st.ZoneID = e.run.zone - 1
		st.ScheduleID = e.run.schedID
		st.RemainingSeconds = max(0, e.run.endMin*60-model.SecondsOfDay(now))
	}
	for _, ev := range e.events {
		if ev.timeMin != -1 {
			st.PendingEvents++
		}
	}
	st.ZoneOn = make([]bool, len(cfg.Zones))
	for i := range st.ZoneOn {
		st.ZoneOn[i] = e.outState&(1<<(i+1)) != 0
	}
	st.PumpOn = e.outState&1 != 0
	return st
}

// ---- internals (callers hold e.mu) ----

// reloadLocked ports ReloadEvents: stop everything, then queue one
// evStartSched per due schedule start time. With all=false, start times at or
// before the current minute are skipped.
func (e *Engine) reloadLocked(all bool, now time.Time, cfg *model.Config) {
	e.events = e.events[:0]
	e.clearRunLocked(now)
	e.outState = 0
	if !cfg.Settings.RunSchedules {
		return
	}
	nowMin := model.MinutesOfDay(now)
	for i := range cfg.Schedules {
		s := &cfg.Schedules[i]
		if !s.RunsOn(now) {
			continue
		}
		for _, start := range s.StartTimes {
			if !all && start <= nowMin {
				continue
			}
			e.events = append(e.events, event{timeMin: start, kind: evStartSched, sched: i})
		}
	}
}

// startScheduleLocked ports LoadSchedTimeEvents: expand a schedule (or the
// quick-run durations) into sequential zone on/off events starting now.
func (e *Engine) startScheduleLocked(idx int, quick bool, now time.Time, cfg *model.Config) {
	adj := adjustments{seasonal: -1, weather: -1}
	var durations []int
	if quick {
		durations = e.quick
	} else {
		s := cfg.Schedules[idx]
		s.Normalize(len(cfg.Zones))
		adj.seasonal = cfg.Settings.SeasonalAdjust
		adj.weather = 100
		if s.WeatherAdjust && e.weatherScale != nil {
			adj.weather = e.weatherScale()
		}
		scale := adj.seasonal * adj.weather / 100
		durations = make([]int, len(cfg.Zones))
		for i, d := range s.Durations {
			durations[i] = min((d*scale+50)/100, model.MaxZoneMinutes)
		}
	}

	start := model.MinutesOfDay(now)
	for k := 0; k < len(cfg.Zones) && k < len(durations); k++ {
		if cfg.Zones[k].Enabled && durations[k] > 0 {
			e.events = append(e.events, event{
				timeMin: start, kind: evZoneOn, zone: k + 1, endMin: start + durations[k],
			})
			start += durations[k]
		}
	}
	e.events = append(e.events, event{timeMin: start, kind: evAllOff})

	schedID := idx
	if quick {
		schedID = ScheduleQuick
	}
	e.logRunLocked(now)
	e.run = runState{schedule: true, schedID: schedID, zone: -1, eventTime: now, adj: adj}
}

// processEventsLocked ports ProcessEvents. Events appended while iterating
// (schedule expansion) are visited in the same pass, like the original.
func (e *Engine) processEventsLocked(now time.Time, cfg *model.Config) {
	nowMin := model.MinutesOfDay(now)
	for i := 0; i < len(e.events); i++ {
		if e.events[i].timeMin == -1 || nowMin < e.events[i].timeMin {
			continue
		}
		switch e.events[i].kind {
		case evZoneOn:
			e.turnOnZoneLocked(e.events[i].zone, cfg)
			e.continueScheduleLocked(now, e.events[i].zone, e.events[i].endMin)
			e.events[i].timeMin = -1
		case evAllOff:
			e.outState = 0
			e.clearRunLocked(now)
			e.events[i].timeMin = -1
		case evStartSched:
			if e.run.schedule {
				// Another schedule is running: push this one off a minute.
				e.events[i].timeMin++
			} else {
				sched := e.events[i].sched
				e.events[i].timeMin = -1
				if sched >= 0 && sched < len(cfg.Schedules) {
					e.startScheduleLocked(sched, false, now, cfg)
				}
			}
		}
	}
}

// turnOnZoneLocked ports TurnOnZone: exactly one zone on at a time, plus the
// pump/master output if the zone requires it.
func (e *Engine) turnOnZoneLocked(valve int, cfg *model.Config) {
	if valve < 1 || valve > len(cfg.Zones) {
		return
	}
	e.outState = 1 << valve
	if cfg.Zones[valve-1].Pump {
		e.outState |= 1
	}
}

func (e *Engine) latchLocked() {
	if e.havePrev && e.prevOut == e.outState {
		return
	}
	if e.out != nil {
		if err := e.out.Apply(e.outState); err != nil {
			slog.Error("output apply failed", "state", e.outState, "err", err)
			return // keep prevOut stale so the next tick retries
		}
	}
	e.prevOut = e.outState
	e.havePrev = true
}

// logRunLocked ports LogSchedule: whenever the run state transitions, the
// zone that was running until now gets one log row.
func (e *Engine) logRunLocked(now time.Time) {
	r := &e.run
	if r.eventTime.IsZero() || r.zone < 1 || e.logger == nil {
		return
	}
	schedID := ScheduleManual
	if r.schedule {
		schedID = r.schedID
	}
	if err := e.logger.LogZoneEvent(r.eventTime, r.zone-1, now.Sub(r.eventTime), schedID, r.adj.seasonal, r.adj.weather); err != nil {
		slog.Error("zone log write failed", "err", err)
	}
}

func (e *Engine) clearRunLocked(now time.Time) {
	e.logRunLocked(now)
	e.run = runState{schedID: -1, zone: -1, eventTime: now, adj: adjustments{-1, -1}}
}

func (e *Engine) continueScheduleLocked(now time.Time, zone, endMin int) {
	e.logRunLocked(now)
	e.run = runState{
		schedule: true, schedID: e.run.schedID, zone: zone, endMin: endMin,
		eventTime: now, adj: e.run.adj,
	}
}

func (e *Engine) setManualLocked(now time.Time, on bool, zone int) {
	e.logRunLocked(now)
	e.run = runState{manual: on, schedID: -1, zone: zone, eventTime: now, adj: adjustments{-1, -1}}
}
