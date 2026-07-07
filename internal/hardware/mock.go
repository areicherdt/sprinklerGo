package hardware

import (
	"log/slog"
	"sync"
)

// Mock is the "none" backend: it drives nothing and only records the state.
// Useful for development machines and as a safe fallback.
type Mock struct {
	mu    sync.Mutex
	state uint16
}

func NewMock() *Mock { return &Mock{} }

func (m *Mock) Apply(state uint16) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if state != m.state {
		slog.Debug("mock output", "state", state)
	}
	m.state = state
	return nil
}

func (m *Mock) State() uint16 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

func (m *Mock) Close() error { return nil }
