// Package weather defines the provider interface and the duration scaling
// formula from the original Weather.cpp. Providers are selected at runtime
// via the settings; phase 1 ships only the "none" provider (scale 100%).
package weather

import (
	"context"

	"sprinklergo/internal/model"
)

// ReturnVals mirrors Weather::ReturnVals. Units follow the original:
// temperatures in °F, precipitation in 1/100 inch, wind in mph*10, UV*10.
type ReturnVals struct {
	Valid           bool   `json:"valid"`
	KeyNotFound     bool   `json:"keyNotFound"`
	Error           string `json:"error,omitempty"`
	MinHumidity     int    `json:"minHumidity"`
	MaxHumidity     int    `json:"maxHumidity"`
	MeanTempF       int    `json:"meanTempF"`
	PrecipYesterday int    `json:"precipYesterday"`
	PrecipToday     int    `json:"precipToday"`
	WindMPH         int    `json:"windMph"`
	UV              int    `json:"uv"`
}

const neutralHumidity = 30

// Scale computes the watering scale percentage (0-200) from yesterday's
// weather, exactly like Weather::GetScale: 100 + humidity factor +
// temperature factor + rain factor, clamped to [0, 200]. Invalid values
// yield the neutral 100%.
func Scale(v ReturnVals) int {
	if !v.Valid {
		return 100
	}
	humidFactor := neutralHumidity - (v.MaxHumidity+v.MinHumidity)/2
	tempFactor := (v.MeanTempF - 70) * 4
	rainFactor := (v.PrecipYesterday + v.PrecipToday) * -2
	return min(max(0, 100+humidFactor+tempFactor+rainFactor), 200)
}

type Provider interface {
	Name() string
	GetVals(ctx context.Context, s model.Settings) ReturnVals
}

// None is the placeholder provider: no data, scale stays at 100%.
type None struct{}

func (None) Name() string { return "none" }

func (None) GetVals(ctx context.Context, s model.Settings) ReturnVals {
	return ReturnVals{Valid: false}
}

// ForSettings returns the provider configured in the settings.
func ForSettings(s model.Settings) Provider {
	switch s.WeatherProvider {
	case "openmeteo":
		return NewOpenMeteo()
	default:
		return None{}
	}
}
