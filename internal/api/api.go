// Package api exposes the REST interface and serves the embedded SPA.
package api

import (
	_ "embed"
	"encoding/json"
	"errors"
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
	static  fs.FS
	// applyOutput rebuilds the hardware backend after output-relevant
	// settings changed. May be nil (tests).
	applyOutput func(model.Settings) error
}

func New(version string, cfg *store.ConfigStore, logs *store.LogStore, eng *engine.Engine, static fs.FS, applyOutput func(model.Settings) error) *Server {
	return &Server{version: version, cfg: cfg, logs: logs, eng: eng, static: static, applyOutput: applyOutput}
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

type stateDTO struct {
	Version          string `json:"version"`
	Time             int64  `json:"time"`
	SchedulerEnabled bool   `json:"schedulerEnabled"`
	Mode             string `json:"mode"`
	ZoneID           int    `json:"zoneId"`
	ZoneName         string `json:"zoneName,omitempty"`
	ScheduleID       int    `json:"scheduleId"`
	ScheduleName     string `json:"scheduleName,omitempty"`
	RemainingSeconds int    `json:"remainingSeconds"`
	PendingEvents    int    `json:"pendingEvents"`
	EnabledZones     int    `json:"enabledZones"`
	ScheduleCount    int    `json:"scheduleCount"`
}

func (s *Server) getState(w http.ResponseWriter, r *http.Request) {
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
	}
	if st.ZoneID >= 0 && st.ZoneID < len(cfg.Zones) {
		dto.ZoneName = cfg.Zones[st.ZoneID].Name
	}
	switch {
	case st.ScheduleID == engine.ScheduleQuick:
		dto.ScheduleName = "Schnellstart"
	case st.ScheduleID >= 0 && st.ScheduleID < len(cfg.Schedules):
		dto.ScheduleName = cfg.Schedules[st.ScheduleID].Name
	}
	writeJSON(w, http.StatusOK, dto)
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
	s.eng.Reload()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) postManual(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, len(s.cfg.Snapshot().Zones))
	if !ok {
		return
	}
	var body struct {
		On bool `json:"on"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if err := s.eng.SetManualZone(id, body.On); err != nil {
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
	s.eng.Reload()
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

// ---- weather ----

func (s *Server) getWeatherCheck(w http.ResponseWriter, r *http.Request) {
	settings := s.cfg.Snapshot().Settings
	provider := weather.ForSettings(settings)
	vals := provider.GetVals(r.Context(), settings)
	writeJSON(w, http.StatusOK, map[string]any{
		"provider":   provider.Name(),
		"noProvider": provider.Name() == "none",
		"vals":       vals,
		"scale":      weather.Scale(vals),
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
