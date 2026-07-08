package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"time"

	"sprinklergo/internal/model"
)

// OpenWeather fetches weather data from OpenWeatherMap's One Call 3.0 "Day
// Summary" endpoint (requires an API key with the One Call subscription).
// The original OpenWeather.cpp used the deprecated One Call 2.5 timemachine;
// the day-summary aggregation gives the same fields the scale formula needs
// (yesterday's temperature/humidity/rain plus today's rain) in one call per
// day. Units follow the original: °F, wind mph, precipitation converted from
// millimeters to 1/100 inch.
type OpenWeather struct {
	baseURL string
	client  *http.Client
}

func NewOpenWeather() *OpenWeather {
	return &OpenWeather{
		baseURL: "https://api.openweathermap.org",
		client:  &http.Client{Timeout: 12 * time.Second},
	}
}

func (o *OpenWeather) Name() string { return "openweather" }

type owDaySummary struct {
	Cod         any `json:"cod"` // present (401/…) only on errors
	Message     string
	Temperature struct {
		Min *float64 `json:"min"`
		Max *float64 `json:"max"`
	} `json:"temperature"`
	Humidity struct {
		Afternoon *float64 `json:"afternoon"`
	} `json:"humidity"`
	Precipitation struct {
		Total *float64 `json:"total"`
	} `json:"precipitation"`
	Wind struct {
		Max struct {
			Speed *float64 `json:"speed"`
		} `json:"max"`
	} `json:"wind"`
}

func (o *OpenWeather) daySummary(ctx context.Context, s model.Settings, lat, lon, date string) (owDaySummary, int, error) {
	q := url.Values{
		"lat":   {lat},
		"lon":   {lon},
		"date":  {date},
		"appid": {s.APISecret},
		"units": {"imperial"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		o.baseURL+"/data/3.0/onecall/day_summary?"+q.Encode(), nil)
	if err != nil {
		return owDaySummary{}, 0, err
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return owDaySummary{}, 0, err
	}
	defer resp.Body.Close()
	var day owDaySummary
	if err := json.NewDecoder(resp.Body).Decode(&day); err != nil {
		return owDaySummary{}, resp.StatusCode, fmt.Errorf("bad open-weather response: %w", err)
	}
	return day, resp.StatusCode, nil
}

func (o *OpenWeather) GetVals(ctx context.Context, s model.Settings) ReturnVals {
	fail := func(msg string) ReturnVals { return ReturnVals{Valid: false, Error: msg} }
	if s.APISecret == "" {
		return fail("open-weather needs an API key (API secret)")
	}
	lat, lon, err := ParseLocation(s.Location)
	if err != nil {
		return fail(err.Error())
	}

	now := time.Now().UTC()
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	today := now.Format("2006-01-02")

	yDay, status, err := o.daySummary(ctx, s, lat, lon, yesterday)
	if err != nil {
		return fail(err.Error())
	}
	if status == http.StatusUnauthorized {
		return ReturnVals{Valid: false, KeyNotFound: true, Error: "invalid OpenWeather API key"}
	}
	if status != http.StatusOK {
		return fail(fmt.Sprintf("open-weather returned HTTP %d", status))
	}

	vals := ReturnVals{Valid: true, MinHumidity: neutralHumidity, MaxHumidity: neutralHumidity}

	// Yesterday drives the scale: mean temp, humidity and rain.
	if yDay.Temperature.Min != nil && yDay.Temperature.Max != nil {
		vals.MeanTempF = int(math.Round((*yDay.Temperature.Min + *yDay.Temperature.Max) / 2))
	} else {
		vals.Valid = false
	}
	if h := yDay.Humidity.Afternoon; h != nil && *h >= 0 && *h <= 100 {
		vals.MinHumidity = int(math.Round(*h))
		vals.MaxHumidity = int(math.Round(*h))
	}
	if p := yDay.Precipitation.Total; p != nil {
		vals.PrecipYesterday = mmToHundredthInch(*p)
	} else {
		vals.Valid = false
	}
	if w := yDay.Wind.Max.Speed; w != nil {
		vals.WindMPH = int(math.Round(*w * 10))
	}

	// Today's rain so far (best effort; a failure here is not fatal).
	if tDay, st, terr := o.daySummary(ctx, s, lat, lon, today); terr == nil && st == http.StatusOK {
		if p := tDay.Precipitation.Total; p != nil {
			vals.PrecipToday = mmToHundredthInch(*p)
		}
	}

	if !vals.Valid {
		vals.Error = "open-weather day summary is missing expected fields"
	}
	return vals
}

// mmToHundredthInch converts millimeters of rain to the original's unit of
// 1/100 inch.
func mmToHundredthInch(mm float64) int {
	return int(math.Round(mm / 25.4 * 100))
}
