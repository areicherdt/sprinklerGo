package hardware

import (
	"reflect"
	"testing"

	"sprinklergo/internal/model"
)

func TestGpioValuesActiveHigh(t *testing.T) {
	// bit 0 (pump) + bit 2 (zone 2) on, active high.
	got := gpioValues(0b101, 4, true)
	if want := []int{1, 0, 1, 0}; !reflect.DeepEqual(got, want) {
		t.Errorf("active-high = %v, want %v", got, want)
	}
	// All off.
	if got := gpioValues(0, 3, true); !reflect.DeepEqual(got, []int{0, 0, 0}) {
		t.Errorf("active-high off = %v, want all 0", got)
	}
}

func TestGpioValuesActiveLow(t *testing.T) {
	// Active low inverts: on lines go 0, off lines go 1.
	got := gpioValues(0b101, 4, false)
	if want := []int{0, 1, 0, 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("active-low = %v, want %v", got, want)
	}
	if got := gpioValues(0, 3, false); !reflect.DeepEqual(got, []int{1, 1, 1}) {
		t.Errorf("active-low off = %v, want all 1", got)
	}
}

// GreenIQ drives 7 active-high lines (master + 6 zones); zone bits above the
// pin count are simply ignored.
func TestGpioValuesGreenIQShape(t *testing.T) {
	pins := model.GreenIQPins()
	if len(pins) != model.GreenIQZones+1 {
		t.Fatalf("GreenIQPins has %d entries, want %d", len(pins), model.GreenIQZones+1)
	}
	// Zone 3 running with its pump: bit 0 (master) + bit 3 (zone 3).
	got := gpioValues(1<<0|1<<3, len(pins), true)
	if want := []int{1, 0, 0, 1, 0, 0, 0}; !reflect.DeepEqual(got, want) {
		t.Errorf("GreenIQ zone 3 + pump = %v, want %v", got, want)
	}
	// A zone beyond the board (zone 9, bit 9) does not drive any line.
	got = gpioValues(1<<9, len(pins), true)
	if want := []int{0, 0, 0, 0, 0, 0, 0}; !reflect.DeepEqual(got, want) {
		t.Errorf("out-of-range zone = %v, want all 0", got)
	}
}

func TestGreenIQPinMap(t *testing.T) {
	pins := model.GreenIQPins()
	// Master valve first, then zones 1..6 (BCM, from the wiringPi map).
	if want := []int{24, 4, 17, 18, 27, 22, 23}; !reflect.DeepEqual(pins, want) {
		t.Errorf("GreenIQPins = %v, want %v", pins, want)
	}
}
