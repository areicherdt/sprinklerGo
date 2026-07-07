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
	if len(pins) != model.NumOutputs {
		return nil, fmt.Errorf("need %d GPIO pins, got %d", model.NumOutputs, len(pins))
	}
	initial := make([]int, len(pins))
	if !activeHigh {
		for i := range initial {
			initial[i] = 1
		}
	}
	lines, err := gpiocdev.RequestLines("gpiochip0", pins,
		gpiocdev.AsOutput(initial...), gpiocdev.WithConsumer("sprinklergo"))
	if err != nil {
		return nil, fmt.Errorf("request GPIO lines: %w", err)
	}
	return &gpioOutput{lines: lines, activeHigh: activeHigh, numPins: len(pins)}, nil
}

func (g *gpioOutput) Apply(state uint16) error {
	values := make([]int, g.numPins)
	for i := range values {
		on := state&(1<<i) != 0
		if on == g.activeHigh {
			values[i] = 1
		}
	}
	return g.lines.SetValues(values)
}

func (g *gpioOutput) Close() error {
	// Leave everything off before releasing the lines.
	g.Apply(0)
	return g.lines.Close()
}
