package weather

import (
	"testing"

	"sprinklergo/internal/model"
)

func TestScale(t *testing.T) {
	for _, tc := range []struct {
		name string
		v    ReturnVals
		want int
	}{
		{"invalid yields neutral", ReturnVals{Valid: false, MeanTempF: 100}, 100},
		// humid = 30-30 = 0, temp = 0, rain = 0
		{"neutral conditions", ReturnVals{Valid: true, MinHumidity: 30, MaxHumidity: 30, MeanTempF: 70}, 100},
		// humid = 30-50 = -20, temp = (80-70)*4 = 40, rain = -30*2 = -60 => 60
		{"mixed", ReturnVals{Valid: true, MinHumidity: 40, MaxHumidity: 60, MeanTempF: 80, PrecipYesterday: 20, PrecipToday: 10}, 60},
		// hot & dry clamps at 200
		{"clamped high", ReturnVals{Valid: true, MinHumidity: 0, MaxHumidity: 0, MeanTempF: 120}, 200},
		// cold & wet clamps at 0
		{"clamped low", ReturnVals{Valid: true, MinHumidity: 90, MaxHumidity: 100, MeanTempF: 40, PrecipYesterday: 100}, 0},
	} {
		if got := Scale(tc.v); got != tc.want {
			t.Errorf("%s: Scale = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestNoneProvider(t *testing.T) {
	p := ForSettings(model.DefaultConfig().Settings)
	if p.Name() != "none" {
		t.Errorf("provider = %q, want none", p.Name())
	}
}
