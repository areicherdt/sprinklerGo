// Package metrics exposes Prometheus metrics behind the settings flag. It
// consumes the same notify events the webhook does (counters) and reads live
// gauges (scheduler state, weather scale, active zone) at scrape time.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"sprinklergo/internal/notify"
)

// Live supplies the current state for gauge metrics, sampled at scrape time.
type Live struct {
	SchedulerEnabled func() bool
	RainDelayActive  func() bool
	WeatherScale     func() int
	ActiveZone       func() int // 0-based, -1 = none
}

type Metrics struct {
	registry     *prometheus.Registry
	runsStarted  prometheus.Counter
	runsFinished prometheus.Counter
	rainSkips    prometheus.Counter
	errors       *prometheus.CounterVec
}

// New builds the metric set, registers it with a private registry and wires
// the live gauges. version labels a build_info metric.
func New(version string, live Live) *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		registry: reg,
		runsStarted: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "sprinklergo_runs_started_total",
			Help: "Number of schedule/quick/manual runs that started.",
		}),
		runsFinished: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "sprinklergo_runs_finished_total",
			Help: "Number of runs that finished.",
		}),
		rainSkips: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "sprinklergo_rain_delay_skips_total",
			Help: "Schedule starts suppressed by an active rain delay.",
		}),
		errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "sprinklergo_errors_total",
			Help: "Errors by kind (output, weather).",
		}, []string{"kind"}),
	}
	reg.MustRegister(m.runsStarted, m.runsFinished, m.rainSkips, m.errors)

	reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name:        "sprinklergo_build_info",
		Help:        "Build information; the value is always 1.",
		ConstLabels: prometheus.Labels{"version": version},
	}, func() float64 { return 1 }))

	boolGauge := func(name, help string, fn func() bool) {
		reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: name, Help: help}, func() float64 {
			if fn != nil && fn() {
				return 1
			}
			return 0
		}))
	}
	boolGauge("sprinklergo_scheduler_enabled", "1 when the scheduler is running schedules.", live.SchedulerEnabled)
	boolGauge("sprinklergo_rain_delay_active", "1 when a rain delay is active.", live.RainDelayActive)

	reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "sprinklergo_weather_scale_percent",
		Help: "Current weather-based runtime scale in percent.",
	}, func() float64 {
		if live.WeatherScale == nil {
			return 100
		}
		return float64(live.WeatherScale())
	}))
	reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "sprinklergo_active_zone",
		Help: "1-based zone currently watering, 0 when idle.",
	}, func() float64 {
		if live.ActiveZone == nil {
			return 0
		}
		return float64(live.ActiveZone() + 1)
	}))
	return m
}

// Emit implements notify.Sink so the engine's and weather cache's events
// increment the counters.
func (m *Metrics) Emit(e notify.Event) {
	switch e.Type {
	case notify.EventRunStarted:
		m.runsStarted.Inc()
	case notify.EventRunFinished:
		m.runsFinished.Inc()
	case notify.EventRainDelaySkip:
		m.rainSkips.Inc()
	case notify.EventOutputError:
		m.errors.WithLabelValues("output").Inc()
	case notify.EventWeatherError:
		m.errors.WithLabelValues("weather").Inc()
	}
}

// Handler serves the metrics in Prometheus text format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
