// Package engine ports the scheduling core of sprinklers_pi (core.cpp) and
// modernizes its workflow: events carry absolute times (so runs may cross
// midnight), configuration changes reload softly (a running cycle finishes
// with its old values), rain delay suppresses schedule starts, and manual
// runs can carry a timer. The original's semantics are otherwise kept:
// strictly sequential zone runs, duration scaling by seasonal and weather
// percentages, and per-minute deferral when a schedule becomes due while
// another one is still running.
package engine

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"sprinklergo/internal/model"
	"sprinklergo/internal/notify"
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
	at    time.Time
	kind  eventKind
	zone  int       // 1-based valve number (evZoneOn)
	endAt time.Time // scheduled end (evZoneOn)
	sched int       // schedule index (evStartSched)
	done  bool
}

type adjustments struct {
	seasonal int // percent, -1 = not applied (manual/quick)
	weather  int
}

type runState struct {
	schedule  bool
	manual    bool
	schedID   int       // schedule index, ScheduleQuick, or -1
	zone      int       // 1-based valve number, -1 = none
	endAt     time.Time // zero = unlimited (manual without timer)
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
	pending     []event // today's schedule starts (evStartSched)
	running     []event // expansion of the active run (evZoneOn/evAllOff)
	run         runState
	outState    uint16
	prevOut     uint16
	havePrev    bool
	initialized bool
	lastDay     int
	quick       []int // quick-run durations, minutes per zone
	rev         int64 // bumped on every observable change (SSE fingerprint)
	sink        notify.Sink
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

// Tick advances the engine to the current clock time: it rebuilds the
// pending starts on day changes (keeping a run that crosses midnight alive),
// fires due events and latches output changes.
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
		e.rebuildPendingLocked(false, now, &cfg)
	} else if day != e.lastDay {
		e.lastDay = day
		e.rebuildPendingLocked(true, now, &cfg)
	}
	e.processLocked(now, &cfg)
	e.latchLocked()
}

// Reload is the soft reload after configuration changes: only the pending
// schedule starts are rebuilt. A running cycle (scheduled or manual) keeps
// going with the values it started with.
func (e *Engine) Reload() {
	e.mu.Lock()
	defer e.mu.Unlock()
	cfg := e.cfg.Snapshot()
	e.rebuildPendingLocked(false, e.now(), &cfg)
	e.latchLocked()
}

// StopAll turns everything off, discards the active run and rebuilds the
// pending starts.
func (e *Engine) StopAll() {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	cfg := e.cfg.Snapshot()
	e.running = e.running[:0]
	e.clearRunLocked(now)
	e.outState = 0
	e.rebuildPendingLocked(false, now, &cfg)
	e.latchLocked()
}

// SetManualZone turns a single zone on (with its pump if configured) or
// turns all zones off. zoneID is 0-based. minutes limits the run
// (0 = unlimited, like the original).
func (e *Engine) SetManualZone(zoneID int, on bool, minutes int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	cfg := e.cfg.Snapshot()
	if zoneID < 0 || zoneID >= len(cfg.Zones) {
		return fmt.Errorf("zone %d out of range", zoneID)
	}
	if minutes < 0 || minutes > 24*60 {
		return fmt.Errorf("manual timer out of range 0-%d minutes", 24*60)
	}
	now := e.now()
	if on {
		e.turnOnZoneLocked(zoneID+1, &cfg)
		e.setManualLocked(now, true, zoneID+1)
		if minutes > 0 {
			e.run.endAt = now.Add(time.Duration(minutes) * time.Minute)
		}
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
	e.clearRunLocked(now)
	e.outState = 0
	e.rebuildPendingLocked(false, now, &cfg)
	e.startScheduleLocked(idx, false, now, &cfg)
	// The original fires ProcessEvents in the same loop pass, so the first
	// zone turns on immediately rather than on the next tick.
	e.processLocked(now, &cfg)
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
	e.clearRunLocked(now)
	e.outState = 0
	e.rebuildPendingLocked(false, now, &cfg)
	e.startScheduleLocked(0, true, now, &cfg)
	e.processLocked(now, &cfg)
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
	e.pending = nil
	e.running = nil
	e.clearRunLocked(e.now())
	e.outState = 0
	e.latchLocked()
}

// SetEventSink registers a consumer for operational events (run started/
// finished, rain-delay skips, output errors). Sinks must not block.
func (e *Engine) SetEventSink(s notify.Sink) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sink = s
}

// Rev returns a counter that increases with every observable change; used
// by the SSE endpoint as a cheap change fingerprint.
func (e *Engine) Rev() int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.rev
}

// PlannedStart is an upcoming schedule start for the current day.
type PlannedStart struct {
	ScheduleID int
	At         time.Time
}

// ZoneRun is one zone slot in the active run's queue.
type ZoneRun struct {
	ZoneID int
	Start  time.Time
	End    time.Time
	Done   bool
	Active bool
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
	Planned          []PlannedStart
	Queue            []ZoneRun
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
		if !e.run.endAt.IsZero() {
			st.RemainingSeconds = max(0, int(e.run.endAt.Sub(now).Seconds()))
		}
	case e.run.schedule && e.run.zone >= 1:
		st.Mode = "schedule"
		st.ZoneID = e.run.zone - 1
		st.ScheduleID = e.run.schedID
		st.RemainingSeconds = max(0, int(e.run.endAt.Sub(now).Seconds()))
	}
	for _, ev := range e.pending {
		if !ev.done {
			st.PendingEvents++
			st.Planned = append(st.Planned, PlannedStart{ScheduleID: ev.sched, At: ev.at})
		}
	}
	if e.run.schedule {
		for _, ev := range e.running {
			if ev.kind != evZoneOn {
				if !ev.done {
					st.PendingEvents++
				}
				continue
			}
			st.Queue = append(st.Queue, ZoneRun{
				ZoneID: ev.zone - 1, Start: ev.at, End: ev.endAt,
				Done:   ev.done && ev.zone != e.run.zone,
				Active: ev.done && ev.zone == e.run.zone,
			})
			if !ev.done {
				st.PendingEvents++
			}
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

func (e *Engine) bumpLocked() { e.rev++ }

func (e *Engine) emitLocked(typ string, data map[string]any) {
	if e.sink != nil {
		e.sink.Emit(notify.Event{Type: typ, Time: e.now(), Data: data})
	}
}

// rebuildPendingLocked ports ReloadEvents' start-list construction: one
// evStartSched per due schedule start time of the current day. With
// includePast=false, start times at or before now are skipped. The active
// run is left untouched (soft reload).
func (e *Engine) rebuildPendingLocked(includePast bool, now time.Time, cfg *model.Config) {
	e.pending = e.pending[:0]
	e.bumpLocked()
	if !cfg.Settings.RunSchedules {
		return
	}
	y, m, d := now.Date()
	midnight := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	for i := range cfg.Schedules {
		s := &cfg.Schedules[i]
		if !s.RunsOn(now) {
			continue
		}
		for _, start := range s.StartTimes {
			at := midnight.Add(time.Duration(start) * time.Minute)
			if !includePast && !at.After(now) {
				continue
			}
			e.pending = append(e.pending, event{at: at, kind: evStartSched, sched: i})
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

	e.running = e.running[:0]
	at := now
	for k := 0; k < len(cfg.Zones) && k < len(durations); k++ {
		if cfg.Zones[k].Enabled && durations[k] > 0 {
			end := at.Add(time.Duration(durations[k]) * time.Minute)
			e.running = append(e.running, event{at: at, kind: evZoneOn, zone: k + 1, endAt: end})
			at = end
		}
	}
	e.running = append(e.running, event{at: at, kind: evAllOff})

	schedID := idx
	if quick {
		schedID = ScheduleQuick
	}
	e.logRunLocked(now)
	e.run = runState{schedule: true, schedID: schedID, zone: -1, eventTime: now, adj: adj}
	e.bumpLocked()
	e.emitLocked(notify.EventRunStarted, map[string]any{
		"scheduleId": schedID, "seasonal": adj.seasonal, "weather": adj.weather,
	})
}

// processLocked ports ProcessEvents. Pending starts are handled first so a
// freshly expanded schedule fires its first zone in the same pass, like the
// original's single event array.
func (e *Engine) processLocked(now time.Time, cfg *model.Config) {
	rainDelayed := cfg.RainDelayUntil > 0 && now.Unix() < cfg.RainDelayUntil
	for i := 0; i < len(e.pending); i++ {
		if e.pending[i].done || now.Before(e.pending[i].at) {
			continue
		}
		if rainDelayed {
			e.pending[i].done = true
			e.bumpLocked()
			slog.Info("schedule start suppressed by rain delay", "schedule", e.pending[i].sched)
			e.emitLocked(notify.EventRainDelaySkip, map[string]any{"scheduleId": e.pending[i].sched})
			continue
		}
		if e.run.schedule {
			// Another schedule is running: push this one off a minute.
			e.pending[i].at = e.pending[i].at.Add(time.Minute)
			continue
		}
		sched := e.pending[i].sched
		e.pending[i].done = true
		if sched >= 0 && sched < len(cfg.Schedules) {
			e.startScheduleLocked(sched, false, now, cfg)
		}
	}
	for i := 0; i < len(e.running); i++ {
		if e.running[i].done || now.Before(e.running[i].at) {
			continue
		}
		switch e.running[i].kind {
		case evZoneOn:
			e.turnOnZoneLocked(e.running[i].zone, cfg)
			e.continueScheduleLocked(now, e.running[i].zone, e.running[i].endAt)
			e.running[i].done = true
		case evAllOff:
			finished := e.run.schedID
			e.outState = 0
			e.clearRunLocked(now)
			e.running[i].done = true
			e.emitLocked(notify.EventRunFinished, map[string]any{"scheduleId": finished})
		}
	}
	// Manual timer expiry.
	if e.run.manual && !e.run.endAt.IsZero() && !now.Before(e.run.endAt) {
		zone := e.run.zone - 1
		e.outState = 0
		e.setManualLocked(now, false, -1)
		e.emitLocked(notify.EventRunFinished, map[string]any{"scheduleId": ScheduleManual, "zoneId": zone})
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
			e.emitLocked(notify.EventOutputError, map[string]any{"error": err.Error()})
			return // keep prevOut stale so the next tick retries
		}
	}
	e.prevOut = e.outState
	e.havePrev = true
	e.bumpLocked()
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
	e.bumpLocked()
}

func (e *Engine) continueScheduleLocked(now time.Time, zone int, endAt time.Time) {
	e.logRunLocked(now)
	e.run = runState{
		schedule: true, schedID: e.run.schedID, zone: zone, endAt: endAt,
		eventTime: now, adj: e.run.adj,
	}
	e.bumpLocked()
}

func (e *Engine) setManualLocked(now time.Time, on bool, zone int) {
	e.logRunLocked(now)
	e.run = runState{manual: on, schedID: -1, zone: zone, eventTime: now, adj: adjustments{-1, -1}}
	e.bumpLocked()
}
