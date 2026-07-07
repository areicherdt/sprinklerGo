package notify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestWebhookDeliversEvent(t *testing.T) {
	var mu sync.Mutex
	var got []Event
	done := make(chan struct{}, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var e Event
		if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
			t.Errorf("bad webhook body: %v", err)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content type %q", ct)
		}
		mu.Lock()
		got = append(got, e)
		mu.Unlock()
		done <- struct{}{}
	}))
	defer srv.Close()

	w := NewWebhook(func() string { return srv.URL })
	w.Emit(Event{Type: EventRunFinished, Time: time.Now(), Data: map[string]any{"scheduleId": 0}})

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("webhook not delivered")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0].Type != EventRunFinished {
		t.Errorf("delivered events: %+v", got)
	}
}

func TestWebhookEmptyURLIsNoop(t *testing.T) {
	w := NewWebhook(func() string { return "" })
	w.Emit(Event{Type: EventRunStarted}) // must not panic or block
}

func TestFanout(t *testing.T) {
	var a, b []Event
	f := Fanout{sinkFunc(func(e Event) { a = append(a, e) }), sinkFunc(func(e Event) { b = append(b, e) })}
	f.Emit(Event{Type: EventOutputError})
	if len(a) != 1 || len(b) != 1 {
		t.Errorf("fanout did not reach all sinks: %d/%d", len(a), len(b))
	}
}

type sinkFunc func(Event)

func (f sinkFunc) Emit(e Event) { f(e) }
