// Package api exposes the REST interface and serves the embedded SPA.
package api

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"time"

	"sprinklergo/internal/engine"
	"sprinklergo/internal/model"
	"sprinklergo/internal/store"
	"sprinklergo/internal/weather"
)

//go:embed openapi.json
var openapiSpec []byte

type Server struct {
	version string
	cfg     *store.ConfigStore
	logs    *store.LogStore
	eng     *engine.Engine
	weather *weather.Cache
	static  fs.FS
	// applyOutput rebuilds the hardware backend after output-relevant
	// settings changed. May be nil (tests).
	applyOutput func(model.Settings) error
}

func New(version string, cfg *store.ConfigStore, logs *store.LogStore, eng *engine.Engine, wcache *weather.Cache, static fs.FS, applyOutput func(model.Settings) error) *Server {
	if wcache == nil {
		wcache = weather.NewCache(func() model.Settings { return cfg.Snapshot().Settings })
	}
	return &Server{version: version, cfg: cfg, logs: logs, eng: eng, weather: wcache, static: static, applyOutput: applyOutput}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/state", s.getState)
	mux.HandleFunc("GET /api/zones", s.getZones)
	mux.HandleFunc("PUT /api/zones/{id}", s.putZone)
	mux.HandleFunc("POST /api/zones/{id}/manual", s.postManual)
	mux.HandleFunc("GET /api/schedules", s.getSchedules)
	mux.HandleFunc("POST /api/schedules", s.postSchedule)
	mux.HandleFunc("GET /api/schedules/{id}", s.getSchedule)
	mux.HandleFunc("PUT /api/schedules/{id}", s.putSchedule)
	mux.HandleFunc("DELETE /api/schedules/{id}", s.deleteSchedule)
	mux.HandleFunc("POST /api/quickrun", s.postQuickRun)
	mux.HandleFunc("POST /api/stop", s.postStop)
	mux.HandleFunc("PUT /api/system/run", s.putSystemRun)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.putSettings)
	mux.HandleFunc("GET /api/weather/check", s.getWeatherCheck)
	mux.HandleFunc("GET /api/logs", s.getLogs)
	mux.HandleFunc("PUT /api/rain-delay", s.putRainDelay)
	mux.HandleFunc("GET /api/events", s.getEvents)
	mux.HandleFunc("GET /api/backup", s.getBackup)
	mux.HandleFunc("POST /api/restore", s.postRestore)
	mux.HandleFunc("GET /api/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(openapiSpec)
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "unknown API route")
	})

	if s.static != nil {
		mux.Handle("/", spaHandler(s.static))
	}
	return mux
}

// ---- JSON helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request, max int) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id < 0 || id >= max {
		writeErr(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return id, true
}

// ---- state ----

type plannedDTO struct {
	ScheduleID   int    `json:"scheduleId"`
	ScheduleName string `json:"scheduleName"`
	At           int64  `json:"at"`
}

type zoneRunDTO struct {
	ZoneID   int    `json:"zoneId"`
	ZoneName string `json:"zoneName"`
	Start    int64  `json:"start"`
	End      int64  `json:"end"`
	Done     bool   `json:"done"`
	Active   bool   `json:"active"`
}

type stateDTO struct {
	Version          string            `json:"version"`
	Time             int64             `json:"time"`
	SchedulerEnabled bool              `json:"schedulerEnabled"`
	Mode             string            `json:"mode"`
	ZoneID           int               `json:"zoneId"`
	ZoneName         string            `json:"zoneName,omitempty"`
	ScheduleID       int               `json:"scheduleId"`
	ScheduleName     string            `json:"scheduleName,omitempty"`
	RemainingSeconds int               `json:"remainingSeconds"`
	PendingEvents    int               `json:"pendingEvents"`
	EnabledZones     int               `json:"enabledZones"`
	ScheduleCount    int               `json:"scheduleCount"`
	RainDelayUntil   int64             `json:"rainDelayUntil"` // unix, 0 = keine
	Clock24h         bool              `json:"clock24h"`
	ZonesOn          []bool            `json:"zonesOn"`
	PumpOn           bool              `json:"pumpOn"`
	Planned          []plannedDTO      `json:"planned"`
	Queue            []zoneRunDTO      `json:"queue"`
	Weather          weather.CacheInfo `json:"weather"`
}

func (s *Server) stateDTO() stateDTO {
	cfg := s.cfg.Snapshot()
	st := s.eng.State()
	dto := stateDTO{
		Version:          s.version,
		Time:             time.Now().Unix(),
		SchedulerEnabled: cfg.Settings.RunSchedules,
		Mode:             st.Mode,
		ZoneID:           st.ZoneID,
		ScheduleID:       st.ScheduleID,
		RemainingSeconds: st.RemainingSeconds,
		PendingEvents:    st.PendingEvents,
		EnabledZones:     cfg.EnabledZones(),
		ScheduleCount:    len(cfg.Schedules),
		RainDelayUntil:   cfg.RainDelayUntil,
		Clock24h:         cfg.Settings.Clock24h,
		ZonesOn:          st.ZoneOn,
		PumpOn:           st.PumpOn,
		Planned:          []plannedDTO{},
		Queue:            []zoneRunDTO{},
		Weather:          s.weather.Snapshot(),
	}
	if dto.RainDelayUntil > 0 && dto.RainDelayUntil <= dto.Time {
		dto.RainDelayUntil = 0 // expired
	}
	zoneName := func(id int) string {
		if id >= 0 && id < len(cfg.Zones) {
			return cfg.Zones[id].Name
		}
		return ""
	}
	schedName := func(id int) string {
		if id == engine.ScheduleQuick {
			return "Schnellstart"
		}
		if id >= 0 && id < len(cfg.Schedules) {
			return cfg.Schedules[id].Name
		}
		return ""
	}
	dto.ZoneName = zoneName(st.ZoneID)
	dto.ScheduleName = schedName(st.ScheduleID)
	for _, p := range st.Planned {
		dto.Planned = append(dto.Planned, plannedDTO{
			ScheduleID: p.ScheduleID, ScheduleName: schedName(p.ScheduleID), At: p.At.Unix(),
		})
	}
	for _, q := range st.Queue {
		dto.Queue = append(dto.Queue, zoneRunDTO{
			ZoneID: q.ZoneID, ZoneName: zoneName(q.ZoneID),
			Start: q.Start.Unix(), End: q.End.Unix(), Done: q.Done, Active: q.Active,
		})
	}
	return dto
}

func (s *Server) getState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.stateDTO())
}

// putRainDelay sets or clears the rain delay ({"hours": 0} clears it).
func (s *Server) putRainDelay(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Hours int `json:"hours"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if body.Hours < 0 || body.Hours > 14*24 {
		writeErr(w, http.StatusBadRequest, "hours must be 0-336")
		return
	}
	until := int64(0)
	if body.Hours > 0 {
		until = time.Now().Add(time.Duration(body.Hours) * time.Hour).Unix()
	}
	err := s.cfg.Update(func(c *model.Config) error {
		c.RainDelayUntil = until
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "rainDelayUntil": until})
}

// getEvents streams the state as Server-Sent Events: a push on every
// observable change (engine or config) and at least every 5 seconds while
// clients keep countdowns ticking.
func (s *Server) getEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	send := func() bool {
		data, err := json.Marshal(s.stateDTO())
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "event: state\ndata: %s\n\n", data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	fingerprint := func() [2]int64 { return [2]int64{s.eng.Rev(), s.cfg.Rev()} }
	last := fingerprint()
	lastPush := time.Now()
	if !send() {
		return
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fp := fingerprint()
			if fp != last || time.Since(lastPush) >= 5*time.Second {
				last = fp
				lastPush = time.Now()
				if !send() {
					return
				}
			}
		}
	}
}

// ---- zones ----

type zoneDTO struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Pump    bool   `json:"pump"`
	On      bool   `json:"on"`
}

func (s *Server) getZones(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Snapshot()
	st := s.eng.State()
	zones := make([]zoneDTO, len(cfg.Zones))
	for i, z := range cfg.Zones {
		zones[i] = zoneDTO{ID: i, Name: z.Name, Enabled: z.Enabled, Pump: z.Pump}
		if i < len(st.ZoneOn) {
			zones[i].On = st.ZoneOn[i]
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"zones": zones, "pumpOn": st.PumpOn})
}

func (s *Server) putZone(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, len(s.cfg.Snapshot().Zones))
	if !ok {
		return
	}
	var body model.Zone
	if !readJSON(w, r, &body) {
		return
	}
	err := s.cfg.Update(func(c *model.Config) error {
		c.Zones[id] = body
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// Soft reload keeps a running cycle alive — unless this very zone is
	// currently watering and was just disabled.
	if st := s.eng.State(); st.ZoneID == id && !body.Enabled {
		s.eng.StopAll()
	} else {
		s.eng.Reload()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) postManual(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, len(s.cfg.Snapshot().Zones))
	if !ok {
		return
	}
	var body struct {
		On bool `json:"on"`
		// Minutes limits the run; omitted = the configured default,
		// 0 = unlimited (original behavior).
		Minutes *int `json:"minutes"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	minutes := s.cfg.Snapshot().Settings.ManualTimerMinutes
	if body.Minutes != nil {
		minutes = *body.Minutes
	}
	if err := s.eng.SetManualZone(id, body.On, minutes); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- schedules ----

type scheduleDTO struct {
	ID int `json:"id"`
	model.Schedule
	NextRun *model.NextRun `json:"nextRun"`
}

func (s *Server) scheduleDTO(id int, sched model.Schedule, now time.Time) scheduleDTO {
	return scheduleDTO{ID: id, Schedule: sched, NextRun: sched.NextRunAfter(now)}
}

func (s *Server) getSchedules(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Snapshot()
	now := time.Now()
	list := make([]scheduleDTO, len(cfg.Schedules))
	for i, sched := range cfg.Schedules {
		list[i] = s.scheduleDTO(i, sched, now)
	}
	writeJSON(w, http.StatusOK, map[string]any{"schedules": list})
}

func (s *Server) getSchedule(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Snapshot()
	id, ok := pathID(w, r, len(cfg.Schedules))
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.scheduleDTO(id, cfg.Schedules[id], time.Now()))
}

func (s *Server) postSchedule(w http.ResponseWriter, r *http.Request) {
	var body model.Schedule
	if !readJSON(w, r, &body) {
		return
	}
	newID := -1
	err := s.cfg.Update(func(c *model.Config) error {
		if len(c.Schedules) >= model.MaxSchedules {
			return errors.New("too many schedules")
		}
		body.Normalize(len(c.Zones))
		c.Schedules = append(c.Schedules, body)
		newID = len(c.Schedules) - 1
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.eng.Reload()
	writeJSON(w, http.StatusCreated, map[string]any{"id": newID})
}

func (s *Server) putSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, len(s.cfg.Snapshot().Schedules))
	if !ok {
		return
	}
	var body model.Schedule
	if !readJSON(w, r, &body) {
		return
	}
	err := s.cfg.Update(func(c *model.Config) error {
		body.Normalize(len(c.Zones))
		c.Schedules[id] = body
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.eng.Reload()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, len(s.cfg.Snapshot().Schedules))
	if !ok {
		return
	}
	err := s.cfg.Update(func(c *model.Config) error {
		c.Schedules = append(c.Schedules[:id], c.Schedules[id+1:]...)
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.eng.Reload()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- run control ----

func (s *Server) postQuickRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ScheduleID *int  `json:"scheduleId"`
		Durations  []int `json:"durations"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	var err error
	switch {
	case body.ScheduleID != nil && body.Durations == nil:
		err = s.eng.QuickRunSchedule(*body.ScheduleID)
	case body.ScheduleID == nil && body.Durations != nil:
		err = s.eng.QuickRunDurations(body.Durations)
	default:
		writeErr(w, http.StatusBadRequest, "provide either scheduleId or durations")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) postStop(w http.ResponseWriter, r *http.Request) {
	s.eng.StopAll()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) putSystemRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	err := s.cfg.Update(func(c *model.Config) error {
		c.Settings.RunSchedules = body.Enabled
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if body.Enabled {
		s.eng.Reload()
	} else {
		// Switching the scheduler off stops the water, like the original.
		s.eng.StopAll()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- settings ----

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.Snapshot().Settings)
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	prev := s.cfg.Snapshot().Settings
	var body model.Settings
	if !readJSON(w, r, &body) {
		return
	}
	err := s.cfg.Update(func(c *model.Config) error {
		c.Settings = body
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.eng.Reload()

	if prev.WeatherProvider != body.WeatherProvider || prev.Location != body.Location {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			s.weather.Refresh(ctx)
		}()
	}

	outputChanged := prev.OutputType != body.OutputType ||
		prev.ScriptPath != body.ScriptPath ||
		!equalInts(prev.GPIOPins, body.GPIOPins)
	if outputChanged && s.applyOutput != nil {
		if err := s.applyOutput(body); err != nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"ok": true, "outputError": err.Error(),
			})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"restartRequired": prev.WebPort != body.WebPort,
	})
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ---- backup & restore ----

func (s *Server) getBackup(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Snapshot()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=\"sprinklergo-config-%s.json\"", time.Now().Format("2006-01-02")))
	w.Write(data)
}

func (s *Server) postRestore(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "read backup: "+err.Error())
		return
	}
	prev := s.cfg.Snapshot().Settings
	cfg, err := s.cfg.ReplaceRaw(raw)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// A restore replaces everything — stop hard and rebuild from scratch.
	s.eng.StopAll()
	if s.applyOutput != nil {
		if err := s.applyOutput(cfg.Settings); err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "outputError": err.Error()})
			return
		}
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		s.weather.Refresh(ctx)
	}()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"restartRequired": prev.WebPort != cfg.Settings.WebPort,
	})
}

// ---- weather ----

func (s *Server) getWeatherCheck(w http.ResponseWriter, r *http.Request) {
	// The diagnostics fetch doubles as a cache refresh, so the engine and
	// the dashboard immediately see the same values.
	info := s.weather.Refresh(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"provider":   info.Provider,
		"noProvider": info.Provider == "none",
		"vals":       info.Vals,
		"scale":      info.Scale,
		"fetchedAt":  info.FetchedAt,
	})
}

// ---- logs ----

func (s *Server) getLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	end := time.Now()
	start := end.AddDate(0, 0, -7)
	if v := q.Get("start"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid start")
			return
		}
		start = time.Unix(n, 0)
	}
	if v := q.Get("end"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid end")
			return
		}
		end = time.Unix(n, 0)
	}
	group := store.Grouping(q.Get("group"))
	if group == "" {
		group = store.GroupNone
	}
	switch group {
	case store.GroupNone:
		entries, err := s.logs.Entries(start, end)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"group": group, "entries": entries})
	case store.GroupHour, store.GroupDay, store.GroupMonth:
		series, err := s.logs.Grouped(start, end, group)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"group": group, "series": series})
	default:
		writeErr(w, http.StatusBadRequest, "group must be none, hour, day or month")
	}
}

// ---- static SPA ----

func spaHandler(static fs.FS) http.Handler {
	fileServer := http.FileServerFS(static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path != "/" {
			if f, err := fs.Stat(static, path[1:]); err == nil && !f.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// SPA fallback: unknown paths get index.html so client routing works.
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}
