package weather

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sprinklergo/internal/model"
)

func owProvider(t *testing.T, handler http.HandlerFunc) *OpenWeather {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	p := NewOpenWeather()
	p.baseURL = srv.URL
	return p
}

func owSettings(loc, key string) model.Settings {
	s := model.DefaultConfig().Settings
	s.WeatherProvider = "openweather"
	s.Location = loc
	s.APISecret = key
	return s
}

// The day_summary endpoint is called once for yesterday and once for today;
// the handler answers by the requested date.
func TestOpenWeatherParsesDaySummaries(t *testing.T) {
	var dates []string
	p := owProvider(t, func(w http.ResponseWriter, r *http.Request) {
		date := r.URL.Query().Get("date")
		dates = append(dates, date)
		// yesterday sorts before today, so the first call is yesterday.
		if len(dates) == 1 {
			w.Write([]byte(`{"temperature":{"min":60,"max":80},"humidity":{"afternoon":50},
				"precipitation":{"total":5.08},"wind":{"max":{"speed":6.0}}}`))
		} else {
			w.Write([]byte(`{"temperature":{"min":66,"max":88},"humidity":{"afternoon":40},
				"precipitation":{"total":2.54},"wind":{"max":{"speed":4.0}}}`))
		}
	})

	vals := p.GetVals(context.Background(), owSettings("52.52,13.40", "key"))
	if !vals.Valid {
		t.Fatalf("want valid, got %+v", vals)
	}
	if vals.MeanTempF != 70 { // (60+80)/2
		t.Errorf("mean temp = %d, want 70", vals.MeanTempF)
	}
	if vals.MinHumidity != 50 || vals.MaxHumidity != 50 {
		t.Errorf("humidity = %d/%d, want 50/50", vals.MinHumidity, vals.MaxHumidity)
	}
	if vals.PrecipYesterday != 20 { // 5.08mm / 25.4 * 100 = 20
		t.Errorf("precip yesterday = %d, want 20", vals.PrecipYesterday)
	}
	if vals.PrecipToday != 10 { // 2.54mm = 10 (1/100 inch)
		t.Errorf("precip today = %d, want 10", vals.PrecipToday)
	}
	if vals.WindMPH != 60 {
		t.Errorf("wind = %d, want 60", vals.WindMPH)
	}
	if len(dates) != 2 || dates[0] >= dates[1] {
		t.Errorf("expected yesterday then today, got %v", dates)
	}

	// humid = 30-50 = -20, temp = (70-70)*4 = 0, rain = -(20+10)*2 = -60 => 20
	if got := Scale(vals); got != 20 {
		t.Errorf("Scale = %d, want 20", got)
	}
}

func TestOpenWeatherInvalidKey(t *testing.T) {
	p := owProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"cod":401,"message":"Invalid API key"}`))
	})
	vals := p.GetVals(context.Background(), owSettings("52.52,13.40", "bad"))
	if vals.Valid || !vals.KeyNotFound {
		t.Errorf("401 must set keyNotFound, got %+v", vals)
	}
	if Scale(vals) != 100 {
		t.Error("invalid values must scale to neutral 100")
	}
}

func TestOpenWeatherMissingFields(t *testing.T) {
	p := owProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"humidity":{"afternoon":50}}`))
	})
	if vals := p.GetVals(context.Background(), owSettings("52.52,13.40", "key")); vals.Valid {
		t.Error("response without temperature/precip must be invalid")
	}
}

func TestOpenWeatherRequiresKeyAndLocation(t *testing.T) {
	p := NewOpenWeather()
	if vals := p.GetVals(context.Background(), owSettings("52.52,13.40", "")); vals.Valid || vals.Error == "" {
		t.Error("missing key must fail with an error")
	}
	if vals := p.GetVals(context.Background(), owSettings("Berlin", "key")); vals.Valid || vals.Error == "" {
		t.Error("bad location must fail with an error")
	}
}

func TestForSettingsSelectsOpenWeather(t *testing.T) {
	if p := ForSettings(owSettings("52.52,13.40", "key")); p.Name() != "openweather" {
		t.Errorf("provider = %q, want openweather", p.Name())
	}
}

func TestSettingsValidateOpenWeather(t *testing.T) {
	s := owSettings("52.52,13.40", "key")
	if err := s.Validate(); err != nil {
		t.Errorf("valid openweather settings rejected: %v", err)
	}
	noKey := owSettings("52.52,13.40", "")
	if err := noKey.Validate(); err == nil || !strings.Contains(err.Error(), "API key") {
		t.Errorf("missing key must be rejected, got %v", err)
	}
	badLoc := owSettings("Berlin", "key")
	if err := badLoc.Validate(); err == nil {
		t.Error("non-coordinate location must be rejected for openweather")
	}
}
