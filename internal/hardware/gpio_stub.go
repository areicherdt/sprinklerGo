//go:build !linux

package hardware

import "fmt"

func newGPIO(pins []int, activeHigh bool) (Output, error) {
	return nil, fmt.Errorf("GPIO output is only supported on Linux")
}
