package model

import (
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

type OutputType string

const (
	OutputNone    OutputType = "none"
	OutputScript  OutputType = "script"
	OutputGPIOPos OutputType = "gpio+"
	OutputGPIONeg OutputType = "gpio-"
)

const NumOutputs = MaxZones + 1 // pump/master + 15 zones

// ConfigVersion is the current config.json schema version. Older documents
// are upgraded by the migration chain in the store package.
const ConfigVersion = 6

type Settings struct {
	WebPort    int        `json:"webPort"`
	OutputType OutputType `json:"outputType"`
	// ScriptPath is invoked as "<path> <output> <0|1>" for OutputScript.
	ScriptPath string `json:"scriptPath"`
	// GPIOPins are BCM pin numbers, index 0 = pump/master, 1..15 = zones.
	GPIOPins []int `json:"gpioPins"`
	// SeasonalAdjust scales all schedule durations, percent 0-200
	// (used when SeasonalMode is "global").
	SeasonalAdjust int `json:"seasonalAdjust"`
	// SeasonalMode selects "global" (one percentage) or "monthly"
	// (a 12-entry profile in SeasonalMonthly).
	SeasonalMode    string `json:"seasonalMode"`
	SeasonalMonthly []int  `json:"seasonalMonthly"`
	// Pump/master valve lead and lag relative to the zone valves, seconds.
	PumpPreSeconds  int `json:"pumpPreSeconds"`
	PumpPostSeconds int `json:"pumpPostSeconds"`
	// WeatherProvider selects the runtime weather provider ("none" in phase 1).
	WeatherProvider string `json:"weatherProvider"`
	APIKey          string `json:"apiKey"`
	APISecret       string `json:"apiSecret"`
	Location        string `json:"location"`
	Clock24h        bool   `json:"clock24h"`
	// Language selects the UI language ("de" or "en"); also used for the
	// Home Assistant entity names.
	Language string `json:"language"`
	// RunSchedules is the global scheduler on/off switch.
	RunSchedules bool `json:"runSchedules"`
	// LogRetentionMonths prunes zone log entries older than this many
	// months (0 = keep forever).
	LogRetentionMonths int `json:"logRetentionMonths"`
	// ManualTimerMinutes limits manual zone runs started without an
	// explicit duration (0 = unlimited, the original's behavior).
	ManualTimerMinutes int `json:"manualTimerMinutes"`
	// MQTT integration (Home Assistant discovery included).
	MQTTEnabled     bool   `json:"mqttEnabled"`
	MQTTBroker      string `json:"mqttBroker"` // e.g. "tcp://192.168.1.10:1883"
	MQTTUsername    string `json:"mqttUsername"`
	MQTTPassword    string `json:"mqttPassword"`
	MQTTTopicPrefix string `json:"mqttTopicPrefix"`
	MQTTHADiscovery bool   `json:"mqttHADiscovery"`
	// WebhookURL receives event notifications as JSON POSTs (empty = off).
	WebhookURL string `json:"webhookUrl"`
}

func (s *Settings) Validate() error {
	if s.WebPort < 1 || s.WebPort > 65535 {
		return fmt.Errorf("webPort must be 1-65535")
	}
	switch s.OutputType {
	case OutputNone, OutputScript, OutputGPIOPos, OutputGPIONeg:
	default:
		return fmt.Errorf("outputType must be one of none, script, gpio+, gpio-")
	}
	if s.OutputType == OutputScript && s.ScriptPath == "" {
		return fmt.Errorf("scriptPath is required for outputType script")
	}
	if len(s.GPIOPins) != NumOutputs {
		return fmt.Errorf("gpioPins must have exactly %d entries (pump + %d zones)", NumOutputs, MaxZones)
	}
	if s.SeasonalAdjust < 0 || s.SeasonalAdjust > 200 {
		return fmt.Errorf("seasonalAdjust must be 0-200")
	}
	switch s.SeasonalMode {
	case "global", "monthly":
	default:
		return fmt.Errorf("seasonalMode must be \"global\" or \"monthly\"")
	}
	if len(s.SeasonalMonthly) != 0 && len(s.SeasonalMonthly) != 12 {
		return fmt.Errorf("seasonalMonthly must have exactly 12 entries")
	}
	if s.SeasonalMode == "monthly" && len(s.SeasonalMonthly) != 12 {
		return fmt.Errorf("seasonalMode monthly requires 12 seasonalMonthly entries")
	}
	for i, v := range s.SeasonalMonthly {
		if v < 0 || v > 200 {
			return fmt.Errorf("seasonalMonthly[%d] must be 0-200", i)
		}
	}
	if s.PumpPreSeconds < 0 || s.PumpPreSeconds > 120 || s.PumpPostSeconds < 0 || s.PumpPostSeconds > 120 {
		return fmt.Errorf("pump pre/post run must be 0-120 seconds")
	}
	switch s.Language {
	case "de", "en":
	default:
		return fmt.Errorf("language must be \"de\" or \"en\"")
	}
	if s.LogRetentionMonths < 0 || s.LogRetentionMonths > 120 {
		return fmt.Errorf("logRetentionMonths must be 0-120 (0 = unlimited)")
	}
	if s.ManualTimerMinutes < 0 || s.ManualTimerMinutes > 24*60 {
		return fmt.Errorf("manualTimerMinutes must be 0-1440 (0 = unlimited)")
	}
	if s.MQTTEnabled {
		if s.MQTTBroker == "" {
			return fmt.Errorf("mqttBroker is required when MQTT is enabled")
		}
		if s.MQTTTopicPrefix == "" || strings.ContainsAny(s.MQTTTopicPrefix, "#+/ ") {
			return fmt.Errorf("mqttTopicPrefix must be a single topic level without wildcards")
		}
	}
	if s.WebhookURL != "" {
		u, err := url.Parse(s.WebhookURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("webhookUrl must be an absolute http(s) URL")
		}
	}
	switch s.WeatherProvider {
	case "none":
	case "openmeteo":
		if !validLatLon(s.Location) {
			return fmt.Errorf("open-meteo needs location as \"latitude,longitude\"")
		}
	default:
		return fmt.Errorf("unknown weatherProvider %q", s.WeatherProvider)
	}
	return nil
}

func validLatLon(loc string) bool {
	parts := strings.Split(loc, ",")
	if len(parts) != 2 {
		return false
	}
	for _, p := range parts {
		if _, err := strconv.ParseFloat(strings.TrimSpace(p), 64); err != nil {
			return false
		}
	}
	return true
}

// APIToken is a named automation token; only its SHA-256 digest is stored.
type APIToken struct {
	Name      string `json:"name"`
	SHA256    string `json:"sha256"`
	CreatedAt int64  `json:"createdAt"`
}

// AuthConfig is the optional single-user authentication (off by default).
type AuthConfig struct {
	Enabled      bool       `json:"enabled"`
	PasswordHash string     `json:"passwordHash"` // bcrypt
	Tokens       []APIToken `json:"tokens"`
}

func (a *AuthConfig) Validate() error {
	if a.Enabled && a.PasswordHash == "" {
		return fmt.Errorf("auth cannot be enabled without a password")
	}
	seen := map[string]bool{}
	for _, t := range a.Tokens {
		if t.Name == "" || len(t.Name) > maxNameLen {
			return fmt.Errorf("token name must be 1-%d characters", maxNameLen)
		}
		if seen[t.Name] {
			return fmt.Errorf("duplicate token name %q", t.Name)
		}
		seen[t.Name] = true
	}
	return nil
}

type Config struct {
	Version  int        `json:"version"`
	Settings Settings   `json:"settings"`
	Auth     AuthConfig `json:"auth"`
	// RainDelayUntil suppresses schedule starts until this unix timestamp
	// (0 = no rain delay). Manual and quick runs are unaffected.
	RainDelayUntil int64      `json:"rainDelayUntil"`
	Zones          []Zone     `json:"zones"`
	Schedules      []Schedule `json:"schedules"`
}

func (c *Config) Validate() error {
	if err := c.Settings.Validate(); err != nil {
		return err
	}
	if err := c.Auth.Validate(); err != nil {
		return err
	}
	if len(c.Zones) == 0 || len(c.Zones) > MaxZones {
		return fmt.Errorf("must have 1-%d zones", MaxZones)
	}
	for i := range c.Zones {
		if err := c.Zones[i].Validate(); err != nil {
			return fmt.Errorf("zone %d: %w", i, err)
		}
	}
	if len(c.Schedules) > MaxSchedules {
		return fmt.Errorf("at most %d schedules allowed", MaxSchedules)
	}
	for i := range c.Schedules {
		if err := c.Schedules[i].Validate(len(c.Zones)); err != nil {
			return fmt.Errorf("schedule %d: %w", i, err)
		}
	}
	return nil
}

func (c *Config) Clone() Config {
	out := *c
	out.Zones = slices.Clone(c.Zones)
	out.Settings.GPIOPins = slices.Clone(c.Settings.GPIOPins)
	out.Settings.SeasonalMonthly = slices.Clone(c.Settings.SeasonalMonthly)
	out.Auth.Tokens = slices.Clone(c.Auth.Tokens)
	out.Schedules = make([]Schedule, len(c.Schedules))
	for i := range c.Schedules {
		s := c.Schedules[i]
		s.StartTimes = slices.Clone(s.StartTimes)
		s.Durations = slices.Clone(s.Durations)
		out.Schedules[i] = s
	}
	return out
}

func (c *Config) EnabledZones() int {
	n := 0
	for i := range c.Zones {
		if c.Zones[i].Enabled {
			n++
		}
	}
	return n
}

// DefaultSeasonalMonthly is the neutral 12-month profile (100% each).
func DefaultSeasonalMonthly() []int {
	m := make([]int, 12)
	for i := range m {
		m[i] = 100
	}
	return m
}

// DefaultGPIOPins is the BCM equivalent of the original's wiringPi pin map
// {0..15} (wiringPi numbering translated to BCM for a Raspberry Pi rev2+).
func DefaultGPIOPins() []int {
	return []int{17, 18, 27, 22, 23, 24, 25, 4, 2, 3, 8, 7, 10, 9, 11, 14}
}

// DefaultConfig mirrors the original's factory defaults (ResetEEPROM):
// 15 zones named "Zone n", only zone 1 enabled, pump flag set on all zones,
// port 8080, seasonal adjust 100%, scheduler off, no output hardware.
func DefaultConfig() Config {
	cfg := Config{
		Version: ConfigVersion,
		Settings: Settings{
			WebPort:            8080,
			OutputType:         OutputNone,
			GPIOPins:           DefaultGPIOPins(),
			SeasonalAdjust:     100,
			SeasonalMode:       "global",
			SeasonalMonthly:    DefaultSeasonalMonthly(),
			WeatherProvider:    "none",
			Clock24h:           true,
			Language:           "de",
			RunSchedules:       false,
			LogRetentionMonths: 24,
			ManualTimerMinutes: 30,
			MQTTTopicPrefix:    "sprinklergo",
			MQTTHADiscovery:    true,
		},
	}
	for i := 0; i < MaxZones; i++ {
		cfg.Zones = append(cfg.Zones, Zone{
			Name:    fmt.Sprintf("Zone %d", i+1),
			Enabled: i == 0,
			Pump:    true,
		})
	}
	return cfg
}
