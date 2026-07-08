package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"

	"sprinklergo/internal/notify"
)

func scrape(t *testing.T, m *Metrics) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("scrape status %d", rec.Code)
	}
	return rec.Body.String()
}

func TestMetricsCountersAndGauges(t *testing.T) {
	scale := 80
	zone := -1
	m := New("1.2.3", Live{
		SchedulerEnabled: func() bool { return true },
		RainDelayActive:  func() bool { return false },
		WeatherScale:     func() int { return scale },
		ActiveZone:       func() int { return zone },
	})

	m.Emit(notify.Event{Type: notify.EventRunStarted})
	m.Emit(notify.Event{Type: notify.EventRunFinished})
	m.Emit(notify.Event{Type: notify.EventRainDelaySkip})
	m.Emit(notify.Event{Type: notify.EventOutputError})
	m.Emit(notify.Event{Type: notify.EventWeatherError})
	m.Emit(notify.Event{Type: notify.EventWeatherError})

	body := scrape(t, m)
	for _, want := range []string{
		"sprinklergo_runs_started_total 1",
		"sprinklergo_runs_finished_total 1",
		"sprinklergo_rain_delay_skips_total 1",
		`sprinklergo_errors_total{kind="output"} 1`,
		`sprinklergo_errors_total{kind="weather"} 2`,
		"sprinklergo_scheduler_enabled 1",
		"sprinklergo_rain_delay_active 0",
		"sprinklergo_weather_scale_percent 80",
		"sprinklergo_active_zone 0", // -1 + 1 = idle
		`sprinklergo_build_info{version="1.2.3"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output missing %q", want)
		}
	}

	// Gauges reflect live state at scrape time.
	scale = 120
	zone = 2
	body = scrape(t, m)
	if !strings.Contains(body, "sprinklergo_weather_scale_percent 120") {
		t.Error("weather scale gauge did not update")
	}
	if !strings.Contains(body, "sprinklergo_active_zone 3") {
		t.Error("active zone gauge did not update")
	}
}

func TestMetricsNilLiveFuncs(t *testing.T) {
	m := New("dev", Live{}) // all callbacks nil
	body := scrape(t, m)
	if !strings.Contains(body, "sprinklergo_weather_scale_percent 100") {
		t.Error("nil weather scale must default to 100")
	}
	if !strings.Contains(body, "sprinklergo_scheduler_enabled 0") {
		t.Error("nil scheduler func must default to 0")
	}
}
