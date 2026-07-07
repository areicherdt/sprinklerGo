package weather

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sprinklergo/internal/model"
)

// canned response: 24 hourly temps of 68°F (yesterday) + 24 of 80°F (today),
// humidity 40-60% yesterday, 0.2" rain yesterday / 0.1" today, 5.4 mph wind,
// UV 7.25 today.
func cannedJSON() string {
	temps := make([]string, 48)
	for i := range temps {
		if i < 24 {
			temps[i] = "68"
		} else {
			temps[i] = "80"
		}
	}
	return `{
		"hourly": {"temperature_2m": [` + strings.Join(temps, ",") + `]},
		"daily": {
			"relative_humidity_2m_max": [60, 90],
			"relative_humidity_2m_min": [40, 70],
			"rain_sum": [0.2, 0.1],
			"wind_speed_10m_mean": [5.4, 3.0],
			"uv_index_max": [6.0, 7.25]
		}
	}`
}

func testProvider(t *testing.T, handler http.HandlerFunc) *OpenMeteo {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	p := NewOpenMeteo()
	p.baseURL = srv.URL
	return p
}

func settingsWithLocation(loc string) model.Settings {
	s := model.DefaultConfig().Settings
	s.WeatherProvider = "openmeteo"
	s.Location = loc
	return s
}

func TestOpenMeteoParsesResponse(t *testing.T) {
	var gotQuery string
	p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Write([]byte(cannedJSON()))
	})

	vals := p.GetVals(context.Background(), settingsWithLocation("52.52, 13.40"))
	if !vals.Valid {
		t.Fatalf("want valid, got %+v", vals)
	}
	if vals.MeanTempF != 68 {
		t.Errorf("mean temp = %d, want 68 (yesterday only)", vals.MeanTempF)
	}
	if vals.MinHumidity != 40 || vals.MaxHumidity != 60 {
		t.Errorf("humidity = %d/%d, want 40/60 (yesterday)", vals.MinHumidity, vals.MaxHumidity)
	}
	if vals.PrecipYesterday != 20 || vals.PrecipToday != 10 {
		t.Errorf("precip = %d/%d, want 20/10 (1/100 inch)", vals.PrecipYesterday, vals.PrecipToday)
	}
	if vals.WindMPH != 54 {
		t.Errorf("wind = %d, want 54 (mph*10)", vals.WindMPH)
	}
	if vals.UV != 73 {
		t.Errorf("uv = %d, want 73 (today's max * 10, rounded)", vals.UV)
	}
	for _, part := range []string{"latitude=52.52", "longitude=13.40", "temperature_unit=fahrenheit", "past_days=1"} {
		if !strings.Contains(gotQuery, part) {
			t.Errorf("query missing %q: %s", part, gotQuery)
		}
	}

	// The Scale formula on these values:
	// humid = 30-50 = -20, temp = (68-70)*4 = -8, rain = -(20+10)*2 = -60 => 12
	if got := Scale(vals); got != 12 {
		t.Errorf("Scale = %d, want 12", got)
	}
}

func TestOpenMeteoNullUVAndHumidityFallback(t *testing.T) {
	p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
		body := cannedJSON()
		body = strings.Replace(body, `[6.0, 7.25]`, `[6.0, null]`, 1) // UV today null
		body = strings.Replace(body, `[60, 90]`, `[110, 90]`, 1)      // humidity out of range
		w.Write([]byte(body))
	})
	vals := p.GetVals(context.Background(), settingsWithLocation("52.52,13.40"))
	if !vals.Valid {
		t.Fatalf("null UV must still be valid, got %+v", vals)
	}
	if vals.UV != 0 {
		t.Errorf("uv = %d, want 0 for null", vals.UV)
	}
	if vals.MaxHumidity != 30 {
		t.Errorf("out-of-range humidity must fall back to 30, got %d", vals.MaxHumidity)
	}
}

func TestOpenMeteoIncompleteResponse(t *testing.T) {
	p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"hourly":{"temperature_2m":[1,2,3]},"daily":{}}`))
	})
	vals := p.GetVals(context.Background(), settingsWithLocation("52.52,13.40"))
	if vals.Valid {
		t.Fatal("incomplete response must be invalid")
	}
	if Scale(vals) != 100 {
		t.Error("invalid values must scale to neutral 100")
	}
}

func TestOpenMeteoErrors(t *testing.T) {
	p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	if vals := p.GetVals(context.Background(), settingsWithLocation("52.52,13.40")); vals.Valid || vals.Error == "" {
		t.Errorf("HTTP 500 must yield invalid with error, got %+v", vals)
	}
	if vals := p.GetVals(context.Background(), settingsWithLocation("Berlin")); vals.Valid || vals.Error == "" {
		t.Errorf("bad location must yield invalid with error, got %+v", vals)
	}
}

func TestForSettingsSelectsOpenMeteo(t *testing.T) {
	if p := ForSettings(settingsWithLocation("52.52,13.40")); p.Name() != "openmeteo" {
		t.Errorf("provider = %q, want openmeteo", p.Name())
	}
}

func TestSettingsValidateOpenMeteoLocation(t *testing.T) {
	s := settingsWithLocation("52.52,13.40")
	if err := s.Validate(); err != nil {
		t.Errorf("valid lat,lon rejected: %v", err)
	}
	s.Location = "Berlin"
	if err := s.Validate(); err == nil {
		t.Error("non-coordinate location must be rejected for openmeteo")
	}
}
