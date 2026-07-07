package store

import (
	"encoding/json"
	"errors"
	"fmt"
)

// A migration upgrades a raw config document by exactly one version step.
type migration func(cfg map[string]any) error

// migrations[i] upgrades version i+1 to version i+2. The current config
// version is therefore len(migrations)+1 and must match model.ConfigVersion
// (asserted in tests).
var migrations = []migration{
	migrateV1AddLogRetention,
	migrateV2AddManualTimer,
	migrateV3AddIntegration,
	migrateV4AddSeasonalProfileAndAuth,
}

// v1 → v2: introduce settings.logRetentionMonths (default 24; 0 = unlimited).
func migrateV1AddLogRetention(cfg map[string]any) error {
	settings, ok := cfg["settings"].(map[string]any)
	if !ok {
		return errors.New("config has no settings object")
	}
	if _, exists := settings["logRetentionMonths"]; !exists {
		settings["logRetentionMonths"] = 24
	}
	return nil
}

// v2 → v3: introduce settings.manualTimerMinutes (default 30; 0 = unlimited).
func migrateV2AddManualTimer(cfg map[string]any) error {
	settings, ok := cfg["settings"].(map[string]any)
	if !ok {
		return errors.New("config has no settings object")
	}
	if _, exists := settings["manualTimerMinutes"]; !exists {
		settings["manualTimerMinutes"] = 30
	}
	return nil
}

// v3 → v4: introduce the integration settings (MQTT + webhook). Booleans and
// empty strings default naturally; only the discovery flag and topic prefix
// need explicit defaults.
func migrateV3AddIntegration(cfg map[string]any) error {
	settings, ok := cfg["settings"].(map[string]any)
	if !ok {
		return errors.New("config has no settings object")
	}
	if _, exists := settings["mqttTopicPrefix"]; !exists {
		settings["mqttTopicPrefix"] = "sprinklergo"
	}
	if _, exists := settings["mqttHADiscovery"]; !exists {
		settings["mqttHADiscovery"] = true
	}
	return nil
}

// v4 → v5: seasonal profile (mode + 12-month curve), pump pre/post run and
// the auth section. Numeric zero defaults need no migration; the mode and
// the profile do.
func migrateV4AddSeasonalProfileAndAuth(cfg map[string]any) error {
	settings, ok := cfg["settings"].(map[string]any)
	if !ok {
		return errors.New("config has no settings object")
	}
	if _, exists := settings["seasonalMode"]; !exists {
		settings["seasonalMode"] = "global"
	}
	if _, exists := settings["seasonalMonthly"]; !exists {
		monthly := make([]any, 12)
		for i := range monthly {
			monthly[i] = 100
		}
		settings["seasonalMonthly"] = monthly
	}
	if _, exists := cfg["auth"]; !exists {
		cfg["auth"] = map[string]any{"enabled": false}
	}
	return nil
}

// migrateRaw upgrades a raw config JSON document to the target version using
// the given chain. It reports whether anything changed. A document newer than
// the target is rejected instead of being silently downgraded.
func migrateRaw(raw []byte, target int, chain []migration) ([]byte, bool, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, false, err
	}
	version := 1
	if v, ok := doc["version"].(float64); ok && v >= 1 {
		version = int(v)
	}
	if version > target {
		return nil, false, fmt.Errorf("config version %d is newer than supported version %d — update sprinklerd", version, target)
	}
	if version == target {
		return raw, false, nil
	}
	for v := version; v < target; v++ {
		if err := chain[v-1](doc); err != nil {
			return nil, false, fmt.Errorf("migrate config v%d -> v%d: %w", v, v+1, err)
		}
	}
	doc["version"] = target
	out, err := json.Marshal(doc)
	return out, true, err
}
