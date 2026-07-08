package hardware

// gpioValues maps the engine's output state word to per-line GPIO values.
// Bit i of state drives line i; active-low inverts the level. Lines beyond
// the state word's set bits stay off. Extracted here (platform-independent)
// so the mapping can be tested without a real gpiochip.
func gpioValues(state uint16, numPins int, activeHigh bool) []int {
	values := make([]int, numPins)
	for i := range values {
		on := state&(1<<uint(i)) != 0
		if on == activeHigh {
			values[i] = 1
		}
	}
	return values
}
