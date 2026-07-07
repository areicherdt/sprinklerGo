package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"sprinklergo/internal/model"
)

// OpenMeteo fetches weather data from api.open-meteo.com (free, no API key).
// It ports OpenMeteo.cpp: mean temperature is averaged over yesterday's 24
// hourly values, humidity min/max and rain come from the daily aggregation
// (past_days=1, forecast_days=1), with the original's unit factors
// (°F, rain in 1/100 inch, wind mph*10, UV*10).
type OpenMeteo struct {
	baseURL string
	client  *http.Client
}

func NewOpenMeteo() *OpenMeteo {
	return &OpenMeteo{
		baseURL: "https://api.open-meteo.com",
		client:  &http.Client{Timeout: 12 * time.Second},
	}
}

func (o *OpenMeteo) Name() string { return "openmeteo" }

// ParseLocation splits "lat,lon" (e.g. "52.52,13.40").
func ParseLocation(loc string) (lat, lon string, err error) {
	parts := strings.SplitN(loc, ",", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("location must be \"latitude,longitude\"")
	}
	lat = strings.TrimSpace(parts[0])
	lon = strings.TrimSpace(parts[1])
	if lat == "" || lon == "" {
		return "", "", fmt.Errorf("location must be \"latitude,longitude\"")
	}
	return lat, lon, nil
}

type omResponse struct {
	Hourly struct {
		Temperature2M []float64 `json:"temperature_2m"`
	} `json:"hourly"`
	Daily struct {
		HumidityMax []*float64 `json:"relative_humidity_2m_max"`
		HumidityMin []*float64 `json:"relative_humidity_2m_min"`
		RainSum     []*float64 `json:"rain_sum"`
		WindMean    []*float64 `json:"wind_speed_10m_mean"`
		UVIndexMax  []*float64 `json:"uv_index_max"`
	} `json:"daily"`
}

func (o *OpenMeteo) GetVals(ctx context.Context, s model.Settings) ReturnVals {
	fail := func(msg string) ReturnVals {
		return ReturnVals{Valid: false, Error: msg}
	}
	lat, lon, err := ParseLocation(s.Location)
	if err != nil {
		return fail(err.Error())
	}

	q := url.Values{
		"latitude":           {lat},
		"longitude":          {lon},
		"hourly":             {"temperature_2m"},
		"daily":              {"relative_humidity_2m_max,relative_humidity_2m_min,rain_sum,wind_speed_10m_mean,uv_index_max"},
		"temperature_unit":   {"fahrenheit"},
		"wind_speed_unit":    {"mph"},
		"precipitation_unit": {"inch"},
		// The original hardcodes Europe/Berlin; "auto" aligns the day
		// boundaries with the configured location instead.
		"timezone":      {"auto"},
		"past_days":     {"1"},
		"forecast_days": {"1"},
		"models":        {"icon_seamless"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.baseURL+"/v1/forecast?"+q.Encode(), nil)
	if err != nil {
		return fail(err.Error())
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return fail(err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fail(fmt.Sprintf("open-meteo returned HTTP %d", resp.StatusCode))
	}
	var data omResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fail("bad open-meteo response: " + err.Error())
	}
	return parseOpenMeteo(&data)
}

func parseOpenMeteo(data *omResponse) ReturnVals {
	vals := ReturnVals{Valid: true}

	// Mean temperature: average of yesterday's 24 hourly values (with
	// past_days=1 the first 24 entries are yesterday).
	if len(data.Hourly.Temperature2M) >= 24 {
		sum := 0.0
		for _, v := range data.Hourly.Temperature2M[:24] {
			sum += v
		}
		vals.MeanTempF = int(math.Round(sum / 24))
	} else {
		vals.Valid = false
	}

	// Yesterday's humidity extremes; out-of-range falls back to the neutral
	// value like the original.
	vals.MinHumidity = neutralHumidity
	vals.MaxHumidity = neutralHumidity
	if v := first(data.Daily.HumidityMax); v != nil {
		if *v >= 0 && *v <= 100 {
			vals.MaxHumidity = int(math.Round(*v))
		}
	} else {
		vals.Valid = false
	}
	if v := first(data.Daily.HumidityMin); v != nil {
		if *v >= 0 && *v <= 100 {
			vals.MinHumidity = int(math.Round(*v))
		}
	} else {
		vals.Valid = false
	}

	// Rain: index 0 = yesterday, index 1 = today, in 1/100 inch.
	if len(data.Daily.RainSum) == 2 && data.Daily.RainSum[0] != nil && data.Daily.RainSum[1] != nil {
		vals.PrecipYesterday = int(math.Round(*data.Daily.RainSum[0] * 100))
		vals.PrecipToday = int(math.Round(*data.Daily.RainSum[1] * 100))
	} else {
		vals.Valid = false
	}

	if v := first(data.Daily.WindMean); v != nil {
		vals.WindMPH = int(math.Round(*v * 10))
	} else {
		vals.Valid = false
	}

	// UV max for today (index 1); may legitimately be null.
	if len(data.Daily.UVIndexMax) > 1 {
		if v := data.Daily.UVIndexMax[1]; v != nil {
			vals.UV = int(math.Round(*v * 10))
		}
	} else {
		vals.Valid = false
	}

	if !vals.Valid {
		vals.Error = "open-meteo response is missing expected fields"
	}
	return vals
}

func first(a []*float64) *float64 {
	if len(a) == 0 {
		return nil
	}
	return a[0]
}
