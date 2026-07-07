package hardware

import (
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"sync"
)

// Script invokes an external program as "<path> <output> <0|1>", compatible
// with the original's EXTERNAL_SCRIPT zone control. Output 0 is the
// pump/master valve. The first Apply pushes every output so the script sees a
// defined state; afterwards only changed outputs are invoked.
type Script struct {
	mu         sync.Mutex
	path       string
	numOutputs int
	prev       uint16
	hasPrev    bool
	warned     bool
}

func NewScript(path string, numOutputs int) *Script {
	return &Script{path: path, numOutputs: numOutputs}
}

func (s *Script) Apply(state uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := os.Stat(s.path); err != nil {
		// Like the original: a missing script is not an error, nothing happens.
		if !s.warned {
			slog.Warn("zone control script not found, outputs are not driven", "path", s.path)
			s.warned = true
		}
		return nil
	}
	s.warned = false
	for i := 0; i < s.numOutputs; i++ {
		bit := uint16(1) << i
		if s.hasPrev && s.prev&bit == state&bit {
			continue
		}
		val := "0"
		if state&bit != 0 {
			val = "1"
		}
		cmd := exec.Command(s.path, strconv.Itoa(i), val)
		if err := cmd.Run(); err != nil {
			slog.Error("zone control script failed", "output", i, "value", val, "err", err)
		}
	}
	s.prev = state
	s.hasPrev = true
	return nil
}

func (s *Script) Close() error { return nil }
