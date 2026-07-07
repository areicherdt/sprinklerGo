package model

import "fmt"

// MaxZones is a hard limit: zone outputs live in a 16-bit field where bit 0
// is the pump/master valve and bits 1..15 are the zones.
const MaxZones = 15

const maxNameLen = 50

type Zone struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Pump    bool   `json:"pump"`
}

func (z *Zone) Validate() error {
	if len(z.Name) == 0 || len(z.Name) > maxNameLen {
		return fmt.Errorf("zone name must be 1-%d characters", maxNameLen)
	}
	return nil
}
