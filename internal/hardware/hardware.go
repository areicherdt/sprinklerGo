// Package hardware drives the physical outputs. Bit 0 of the state word is
// the pump/master valve, bits 1..15 are the zone valves — the same layout the
// original uses in io_latch().
package hardware

import (
	"fmt"

	"sprinklergo/internal/model"
)

type Output interface {
	Apply(state uint16) error
	Close() error
}

// ForSettings builds the backend selected in the settings.
func ForSettings(s model.Settings) (Output, error) {
	switch s.OutputType {
	case model.OutputNone:
		return NewMock(), nil
	case model.OutputScript:
		return NewScript(s.ScriptPath, model.NumOutputs), nil
	case model.OutputGPIOPos:
		return newGPIO(s.GPIOPins, true)
	case model.OutputGPIONeg:
		return newGPIO(s.GPIOPins, false)
	default:
		return nil, fmt.Errorf("unknown output type %q", s.OutputType)
	}
}
