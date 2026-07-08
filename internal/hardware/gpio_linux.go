//go:build linux

package hardware

import (
	"fmt"

	"github.com/warthog618/go-gpiocdev"

	"sprinklergo/internal/model"
)

// gpioOutput drives the outputs directly via the Linux gpiochip character
// device (the modern replacement for the original's wiringPi backend).
type gpioOutput struct {
	lines      *gpiocdev.Lines
	activeHigh bool
	numPins    int
}

func newGPIO(pins []int, activeHigh bool) (Output, error) {
	if len(pins) < 1 || len(pins) > model.NumOutputs {
		return nil, fmt.Errorf("need 1-%d GPIO pins, got %d", model.NumOutputs, len(pins))
	}
	// Initial level = off: low for active-high, high for active-low.
	initial := gpioValues(0, len(pins), activeHigh)
	lines, err := gpiocdev.RequestLines("gpiochip0", pins,
		gpiocdev.AsOutput(initial...), gpiocdev.WithConsumer("sprinklergo"))
	if err != nil {
		return nil, fmt.Errorf("request GPIO lines: %w", err)
	}
	return &gpioOutput{lines: lines, activeHigh: activeHigh, numPins: len(pins)}, nil
}

func (g *gpioOutput) Apply(state uint16) error {
	return g.lines.SetValues(gpioValues(state, g.numPins, g.activeHigh))
}

func (g *gpioOutput) Close() error {
	// Leave everything off before releasing the lines.
	g.Apply(0)
	return g.lines.Close()
}
