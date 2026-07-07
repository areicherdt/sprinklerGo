// Package notify carries operational events (run finished, errors, …) from
// the engine and the weather cache to outbound channels such as webhooks.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// Event types emitted by the system.
const (
	EventRunStarted    = "run_started"
	EventRunFinished   = "run_finished"
	EventRainDelaySkip = "rain_delay_skip"
	EventOutputError   = "output_error"
	EventWeatherError  = "weather_error"
)

type Event struct {
	Type string         `json:"type"`
	Time time.Time      `json:"time"`
	Data map[string]any `json:"data,omitempty"`
}

// Sink consumes events. Implementations must not block the caller.
type Sink interface {
	Emit(Event)
}

// Fanout distributes one event to several sinks.
type Fanout []Sink

func (f Fanout) Emit(e Event) {
	for _, s := range f {
		s.Emit(e)
	}
}

// Webhook POSTs events as JSON to a configurable URL. The URL is read per
// event, so settings changes apply without restart; an empty URL disables
// delivery.
type Webhook struct {
	URL    func() string
	Client *http.Client
}

func NewWebhook(url func() string) *Webhook {
	return &Webhook{URL: url, Client: &http.Client{Timeout: 10 * time.Second}}
}

func (w *Webhook) Emit(e Event) {
	url := w.URL()
	if url == "" {
		return
	}
	body, err := json.Marshal(e)
	if err != nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			slog.Warn("webhook request failed", "err", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := w.Client.Do(req)
		if err != nil {
			slog.Warn("webhook delivery failed", "event", e.Type, "err", err)
			return
		}
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			slog.Warn("webhook rejected", "event", e.Type, "status", resp.StatusCode)
		}
	}()
}
