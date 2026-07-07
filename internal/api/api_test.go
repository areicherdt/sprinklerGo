package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sprinklergo/internal/engine"
	"sprinklergo/internal/hardware"
	"sprinklergo/internal/store"
)

type testEnv struct {
	ts   *httptest.Server
	cfg  *store.ConfigStore
	logs *store.LogStore
}

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	dir := t.TempDir()
	cfg, err := store.OpenConfig(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	logs, err := store.OpenLog(filepath.Join(dir, "zonelog.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { logs.Close() })
	eng := engine.New(cfg, hardware.NewMock(), logs, nil, nil)
	srv := New("test", cfg, logs, eng, nil, nil, nil)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return &testEnv{ts: ts, cfg: cfg, logs: logs}
}

// call performs a request and decodes the JSON response into a generic map.
func (e *testEnv) call(t *testing.T, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, e.ts.URL+path, &buf)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("%s %s: response is not JSON: %v", method, path, err)
	}
	return resp.StatusCode, out
}

func validSchedule() map[string]any {
	return map[string]any{
		"name": "Rasen", "enabled": true, "kind": "weekly",
		"days":     []bool{true, true, true, true, true, true, true},
		"interval": 0, "restriction": 0, "weatherAdjust": false,
		"startTimes": []int{360}, "durations": []int{10},
	}
}

func TestStateInitial(t *testing.T) {
	e := newEnv(t)
	code, st := e.call(t, "GET", "/api/state", nil)
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	if st["version"] != "test" || st["mode"] != "idle" || st["schedulerEnabled"] != false {
		t.Errorf("state: %+v", st)
	}
	if st["enabledZones"].(float64) != 1 || st["scheduleCount"].(float64) != 0 {
		t.Errorf("counts wrong: %+v", st)
	}
}

func TestZonesUpdateAndManual(t *testing.T) {
	e := newEnv(t)
	code, res := e.call(t, "GET", "/api/zones", nil)
	if code != 200 || len(res["zones"].([]any)) != 15 {
		t.Fatalf("zones list: %d %+v", code, res)
	}

	code, _ = e.call(t, "PUT", "/api/zones/2", map[string]any{"name": "Beet", "enabled": true, "pump": false})
	if code != 200 {
		t.Fatalf("put zone: %d", code)
	}
	_, res = e.call(t, "GET", "/api/zones", nil)
	z2 := res["zones"].([]any)[2].(map[string]any)
	if z2["name"] != "Beet" || z2["enabled"] != true {
		t.Errorf("zone not updated: %+v", z2)
	}

	// Validation error surfaces as {"error": ...}.
	code, res = e.call(t, "PUT", "/api/zones/2", map[string]any{"name": "", "enabled": true, "pump": false})
	if code != 400 || res["error"] == nil {
		t.Errorf("want 400 with error, got %d %+v", code, res)
	}
	if code, _ = e.call(t, "PUT", "/api/zones/99", map[string]any{"name": "x"}); code != 404 {
		t.Errorf("out of range zone: want 404, got %d", code)
	}

	// Manual on/off.
	code, _ = e.call(t, "POST", "/api/zones/2/manual", map[string]any{"on": true})
	if code != 200 {
		t.Fatalf("manual on: %d", code)
	}
	_, st := e.call(t, "GET", "/api/state", nil)
	if st["mode"] != "manual" || st["zoneId"].(float64) != 2 || st["zoneName"] != "Beet" {
		t.Errorf("manual state: %+v", st)
	}
	_, res = e.call(t, "GET", "/api/zones", nil)
	if res["zones"].([]any)[2].(map[string]any)["on"] != true {
		t.Error("zone 2 must report on")
	}
	e.call(t, "POST", "/api/zones/2/manual", map[string]any{"on": false})
	_, st = e.call(t, "GET", "/api/state", nil)
	if st["mode"] != "idle" {
		t.Errorf("after manual off: %+v", st)
	}
}

func TestScheduleCRUD(t *testing.T) {
	e := newEnv(t)

	code, res := e.call(t, "POST", "/api/schedules", validSchedule())
	if code != 201 || res["id"].(float64) != 0 {
		t.Fatalf("create: %d %+v", code, res)
	}

	_, res = e.call(t, "GET", "/api/schedules", nil)
	list := res["schedules"].([]any)
	if len(list) != 1 {
		t.Fatalf("list: %+v", res)
	}
	first := list[0].(map[string]any)
	if first["name"] != "Rasen" || first["nextRun"] == nil {
		t.Errorf("schedule DTO: %+v", first)
	}
	// Durations must be padded to the number of zones.
	if n := len(first["durations"].([]any)); n != 15 {
		t.Errorf("durations padded to %d, want 15", n)
	}

	upd := validSchedule()
	upd["name"] = "Rasen NEU"
	if code, _ = e.call(t, "PUT", "/api/schedules/0", upd); code != 200 {
		t.Fatalf("update: %d", code)
	}
	_, res = e.call(t, "GET", "/api/schedules/0", nil)
	if res["name"] != "Rasen NEU" {
		t.Errorf("after update: %+v", res)
	}

	bad := validSchedule()
	bad["kind"] = "daily"
	if code, res = e.call(t, "POST", "/api/schedules", bad); code != 400 || res["error"] == nil {
		t.Errorf("invalid schedule: want 400, got %d %+v", code, res)
	}

	if code, _ = e.call(t, "DELETE", "/api/schedules/0", nil); code != 200 {
		t.Fatalf("delete: %d", code)
	}
	if code, _ = e.call(t, "GET", "/api/schedules/0", nil); code != 404 {
		t.Errorf("deleted schedule: want 404, got %d", code)
	}
}

func TestQuickRunAndStop(t *testing.T) {
	e := newEnv(t)

	code, _ := e.call(t, "POST", "/api/quickrun", map[string]any{"durations": []int{5}})
	if code != 200 {
		t.Fatalf("quickrun: %d", code)
	}
	_, st := e.call(t, "GET", "/api/state", nil)
	if st["mode"] != "schedule" || st["scheduleId"].(float64) != 99 || st["scheduleName"] != "Schnellstart" {
		t.Errorf("quickrun state: %+v", st)
	}

	if code, _ = e.call(t, "POST", "/api/stop", nil); code != 200 {
		t.Fatalf("stop: %d", code)
	}
	_, st = e.call(t, "GET", "/api/state", nil)
	if st["mode"] != "idle" {
		t.Errorf("after stop: %+v", st)
	}

	// Exactly one of scheduleId/durations must be present.
	if code, _ = e.call(t, "POST", "/api/quickrun", map[string]any{}); code != 400 {
		t.Errorf("empty quickrun: want 400, got %d", code)
	}
	if code, _ = e.call(t, "POST", "/api/quickrun", map[string]any{"scheduleId": 0, "durations": []int{1}}); code != 400 {
		t.Errorf("ambiguous quickrun: want 400, got %d", code)
	}

	// Quick run of a stored schedule.
	e.call(t, "POST", "/api/schedules", validSchedule())
	code, _ = e.call(t, "POST", "/api/quickrun", map[string]any{"scheduleId": 0})
	if code != 200 {
		t.Fatalf("quickrun schedule: %d", code)
	}
	_, st = e.call(t, "GET", "/api/state", nil)
	if st["mode"] != "schedule" || st["scheduleName"] != "Rasen" {
		t.Errorf("quickrun schedule state: %+v", st)
	}
}

func TestSystemRunToggle(t *testing.T) {
	e := newEnv(t)
	code, _ := e.call(t, "PUT", "/api/system/run", map[string]any{"enabled": true})
	if code != 200 {
		t.Fatalf("system run: %d", code)
	}
	_, st := e.call(t, "GET", "/api/state", nil)
	if st["schedulerEnabled"] != true {
		t.Errorf("scheduler not enabled: %+v", st)
	}
}

func TestSettings(t *testing.T) {
	e := newEnv(t)
	code, s := e.call(t, "GET", "/api/settings", nil)
	if code != 200 || s["webPort"].(float64) != 8080 || s["seasonalAdjust"].(float64) != 100 {
		t.Fatalf("settings: %d %+v", code, s)
	}

	s["seasonalAdjust"] = 80
	code, res := e.call(t, "PUT", "/api/settings", s)
	if code != 200 || res["restartRequired"] != false {
		t.Fatalf("put settings: %d %+v", code, res)
	}
	if e.cfg.Snapshot().Settings.SeasonalAdjust != 80 {
		t.Error("settings not persisted")
	}

	s["webPort"] = 9090
	if _, res = e.call(t, "PUT", "/api/settings", s); res["restartRequired"] != true {
		t.Errorf("webPort change must flag restartRequired: %+v", res)
	}

	s["seasonalAdjust"] = 999
	if code, _ = e.call(t, "PUT", "/api/settings", s); code != 400 {
		t.Errorf("invalid settings: want 400, got %d", code)
	}
}

func TestWeatherCheck(t *testing.T) {
	e := newEnv(t)
	code, res := e.call(t, "GET", "/api/weather/check", nil)
	if code != 200 || res["noProvider"] != true || res["scale"].(float64) != 100 {
		t.Errorf("weather check: %d %+v", code, res)
	}
}

func TestLogs(t *testing.T) {
	e := newEnv(t)
	now := time.Now()
	e.logs.LogZoneEvent(now.Add(-2*time.Hour), 0, 10*time.Minute, 0, 100, 100)
	e.logs.LogZoneEvent(now.Add(-1*time.Hour), 1, 5*time.Minute, store.LogScheduleManual, -1, -1)

	code, res := e.call(t, "GET", "/api/logs", nil)
	if code != 200 || len(res["entries"].([]any)) != 2 {
		t.Fatalf("logs: %d %+v", code, res)
	}

	code, res = e.call(t, "GET", "/api/logs?group=hour", nil)
	if code != 200 || res["series"] == nil {
		t.Fatalf("grouped logs: %d %+v", code, res)
	}

	if code, _ = e.call(t, "GET", "/api/logs?group=bogus", nil); code != 400 {
		t.Errorf("bad group: want 400, got %d", code)
	}
	if code, _ = e.call(t, "GET", "/api/logs?start=abc", nil); code != 400 {
		t.Errorf("bad start: want 400, got %d", code)
	}
}

func TestRainDelay(t *testing.T) {
	e := newEnv(t)
	code, res := e.call(t, "PUT", "/api/rain-delay", map[string]any{"hours": 24})
	if code != 200 || res["rainDelayUntil"].(float64) <= 0 {
		t.Fatalf("set rain delay: %d %+v", code, res)
	}
	_, st := e.call(t, "GET", "/api/state", nil)
	if st["rainDelayUntil"].(float64) <= 0 {
		t.Errorf("state must expose the rain delay: %+v", st["rainDelayUntil"])
	}

	if code, _ = e.call(t, "PUT", "/api/rain-delay", map[string]any{"hours": 0}); code != 200 {
		t.Fatalf("clear rain delay: %d", code)
	}
	_, st = e.call(t, "GET", "/api/state", nil)
	if st["rainDelayUntil"].(float64) != 0 {
		t.Errorf("rain delay not cleared: %+v", st["rainDelayUntil"])
	}

	if code, _ = e.call(t, "PUT", "/api/rain-delay", map[string]any{"hours": 999}); code != 400 {
		t.Errorf("out-of-range hours: want 400, got %d", code)
	}
}

func TestManualTimerDefaults(t *testing.T) {
	e := newEnv(t)
	// Default from settings (30 min).
	e.call(t, "POST", "/api/zones/0/manual", map[string]any{"on": true})
	_, st := e.call(t, "GET", "/api/state", nil)
	if rem := st["remainingSeconds"].(float64); rem <= 5*60 || rem > 30*60 {
		t.Errorf("default manual timer: remaining %v, want ~1800", rem)
	}
	// Explicit override.
	e.call(t, "POST", "/api/zones/0/manual", map[string]any{"on": true, "minutes": 5})
	_, st = e.call(t, "GET", "/api/state", nil)
	if rem := st["remainingSeconds"].(float64); rem <= 0 || rem > 5*60 {
		t.Errorf("manual override: remaining %v, want <=300", rem)
	}
	// Explicit unlimited.
	e.call(t, "POST", "/api/zones/0/manual", map[string]any{"on": true, "minutes": 0})
	_, st = e.call(t, "GET", "/api/state", nil)
	if st["remainingSeconds"].(float64) != -1 {
		t.Errorf("unlimited manual: remaining %v, want -1", st["remainingSeconds"])
	}
	e.call(t, "POST", "/api/zones/0/manual", map[string]any{"on": false})
}

func TestSSEStreamsState(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", e.ts.URL+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	reader := bufio.NewReader(resp.Body)
	eventLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(eventLine, "event: state") {
		t.Fatalf("first line %q", eventLine)
	}
	dataLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	var st map[string]any
	if err := json.Unmarshal([]byte(strings.TrimPrefix(strings.TrimSpace(dataLine), "data: ")), &st); err != nil {
		t.Fatalf("data line is not JSON: %v (%q)", err, dataLine)
	}
	if st["mode"] != "idle" || st["version"] != "test" {
		t.Errorf("initial SSE state wrong: %+v", st)
	}
}

func TestBackupAndRestore(t *testing.T) {
	e := newEnv(t)
	// Change something, back it up.
	e.call(t, "PUT", "/api/zones/2", map[string]any{"name": "Backup-Zone", "enabled": true, "pump": false})
	resp, err := http.Get(e.ts.URL + "/api/backup")
	if err != nil {
		t.Fatal(err)
	}
	backup, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Errorf("content disposition %q", cd)
	}

	// Break the config, then restore the backup.
	e.call(t, "PUT", "/api/zones/2", map[string]any{"name": "Kaputt", "enabled": false, "pump": true})
	req, _ := http.NewRequest("POST", e.ts.URL+"/api/restore", bytes.NewReader(backup))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("restore status %d", res.StatusCode)
	}
	if got := e.cfg.Snapshot().Zones[2].Name; got != "Backup-Zone" {
		t.Errorf("restore did not apply: zone name %q", got)
	}

	// Garbage is rejected and leaves the config untouched.
	req, _ = http.NewRequest("POST", e.ts.URL+"/api/restore", strings.NewReader(`{"version":1,"zones":[]}`))
	res, _ = http.DefaultClient.Do(req)
	res.Body.Close()
	if res.StatusCode != 400 {
		t.Errorf("invalid restore: want 400, got %d", res.StatusCode)
	}
	if got := e.cfg.Snapshot().Zones[2].Name; got != "Backup-Zone" {
		t.Errorf("failed restore must not change config: %q", got)
	}
}

func TestOpenAPISpec(t *testing.T) {
	e := newEnv(t)
	code, spec := e.call(t, "GET", "/api/openapi.json", nil)
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	if spec["openapi"] != "3.1.0" {
		t.Errorf("openapi version: %v", spec["openapi"])
	}
	paths, ok := spec["paths"].(map[string]any)
	if !ok || paths["/api/state"] == nil || paths["/api/schedules/{id}"] == nil {
		t.Errorf("spec is missing expected paths")
	}
}

func TestUnknownRouteReturnsJSONError(t *testing.T) {
	e := newEnv(t)
	code, res := e.call(t, "GET", "/api/nope", nil)
	if code != 404 || res["error"] == nil {
		t.Errorf("unknown route: %d %+v", code, res)
	}
}
