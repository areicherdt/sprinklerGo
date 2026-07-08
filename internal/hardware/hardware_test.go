package hardware

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sprinklergo/internal/model"
)

func TestForSettings(t *testing.T) {
	s := model.DefaultConfig().Settings
	out, err := ForSettings(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out.(*Mock); !ok {
		t.Errorf("outputType none must yield the mock backend, got %T", out)
	}

	s.OutputType = model.OutputScript
	s.ScriptPath = "/usr/local/bin/zone"
	out, err = ForSettings(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out.(*Script); !ok {
		t.Errorf("want script backend, got %T", out)
	}
}

func TestGreenIQSettingsValid(t *testing.T) {
	s := model.DefaultConfig().Settings
	s.OutputType = model.OutputGreenIQ
	// GreenIQ uses the fixed internal pin map, so no scriptPath or custom
	// gpioPins are required beyond the defaults.
	if err := s.Validate(); err != nil {
		t.Errorf("greeniq settings rejected: %v", err)
	}
}

func TestMock(t *testing.T) {
	m := NewMock()
	if err := m.Apply(0b110); err != nil {
		t.Fatal(err)
	}
	if m.State() != 0b110 {
		t.Errorf("state = %b, want 110", m.State())
	}
}

func TestScriptInvokesChangedOutputs(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "calls.txt")
	script := filepath.Join(dir, "zone.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"$1=$2\" >> "+outFile+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	s := NewScript(script, 4)
	if err := s.Apply(0b0110); err != nil {
		t.Fatal(err)
	}
	calls, _ := os.ReadFile(outFile)
	// First apply pushes all 4 outputs.
	if got := strings.TrimSpace(string(calls)); got != "0=0\n1=1\n2=1\n3=0" {
		t.Errorf("first apply calls:\n%s", got)
	}

	os.Remove(outFile)
	if err := s.Apply(0b0100); err != nil {
		t.Fatal(err)
	}
	calls, _ = os.ReadFile(outFile)
	// Only output 1 changed.
	if got := strings.TrimSpace(string(calls)); got != "1=0" {
		t.Errorf("second apply calls:\n%s", got)
	}
}

func TestScriptMissingIsNoop(t *testing.T) {
	s := NewScript(filepath.Join(t.TempDir(), "missing.sh"), 4)
	if err := s.Apply(0b1); err != nil {
		t.Errorf("missing script must be a silent no-op, got %v", err)
	}
}
