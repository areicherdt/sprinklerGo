// Package mqttbridge publishes the controller state to an MQTT broker and
// accepts commands, including Home Assistant discovery so zones and switches
// appear automatically.
package mqttbridge

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"sprinklergo/internal/engine"
	"sprinklergo/internal/model"
)

// Message is one MQTT publication.
type Message struct {
	Topic    string
	Payload  string
	Retained bool
}

func availabilityTopic(prefix string) string { return prefix + "/availability" }

// stateMessages renders the current controller state as retained topics.
func stateMessages(cfg *model.Config, st engine.State, scale int, prefix string) []Message {
	msgs := []Message{}
	for i := range cfg.Zones {
		if !cfg.Zones[i].Enabled {
			continue
		}
		payload := "OFF"
		if i < len(st.ZoneOn) && st.ZoneOn[i] {
			payload = "ON"
		}
		msgs = append(msgs, Message{fmt.Sprintf("%s/zone/%d/state", prefix, i), payload, true})
	}
	run := "OFF"
	if cfg.Settings.RunSchedules {
		run = "ON"
	}
	msgs = append(msgs, Message{prefix + "/system/run/state", run, true})

	rain := "OFF"
	if cfg.RainDelayUntil > 0 {
		rain = "ON"
	}
	msgs = append(msgs, Message{prefix + "/rain_delay/state", rain, true})

	active := "idle"
	if st.ZoneID >= 0 && st.ZoneID < len(cfg.Zones) {
		active = cfg.Zones[st.ZoneID].Name
	}
	msgs = append(msgs, Message{prefix + "/active_zone/state", active, true})
	msgs = append(msgs, Message{prefix + "/weather_scale/state", strconv.Itoa(scale), true})

	blob, _ := json.Marshal(map[string]any{
		"mode":             st.Mode,
		"zoneId":           st.ZoneID,
		"remainingSeconds": st.RemainingSeconds,
		"rainDelayUntil":   cfg.RainDelayUntil,
		"weatherScale":     scale,
	})
	msgs = append(msgs, Message{prefix + "/state", string(blob), true})
	return msgs
}

// label picks the entity name for the configured UI language.
func label(lang, de, en string) string {
	if lang == "en" {
		return en
	}
	return de
}

// discoveryMessages renders Home Assistant MQTT discovery configs (retained).
// Disabled zones get an empty retained payload, which removes the entity.
// Entity names follow the configured UI language.
func discoveryMessages(cfg *model.Config, version, prefix string) []Message {
	lang := cfg.Settings.Language
	device := map[string]any{
		"identifiers":  []string{"sprinklergo"},
		"name":         "sprinklerGo",
		"manufacturer": "sprinklerGo (sprinklers_pi port)",
		"sw_version":   version,
	}
	avail := availabilityTopic(prefix)
	entity := func(extra map[string]any) string {
		payload := map[string]any{"availability_topic": avail, "device": device}
		for k, v := range extra {
			payload[k] = v
		}
		blob, _ := json.Marshal(payload)
		return string(blob)
	}

	msgs := []Message{}
	for i := range cfg.Zones {
		topic := fmt.Sprintf("homeassistant/switch/sprinklergo/zone%d/config", i)
		if !cfg.Zones[i].Enabled {
			msgs = append(msgs, Message{topic, "", true})
			continue
		}
		msgs = append(msgs, Message{topic, entity(map[string]any{
			"name":          cfg.Zones[i].Name,
			"unique_id":     fmt.Sprintf("sprinklergo_zone_%d", i),
			"state_topic":   fmt.Sprintf("%s/zone/%d/state", prefix, i),
			"command_topic": fmt.Sprintf("%s/zone/%d/set", prefix, i),
			"icon":          "mdi:sprinkler",
		}), true})
	}
	msgs = append(msgs,
		Message{"homeassistant/switch/sprinklergo/run/config", entity(map[string]any{
			"name":          label(lang, "Automatik", "Automatic"),
			"unique_id":     "sprinklergo_run",
			"state_topic":   prefix + "/system/run/state",
			"command_topic": prefix + "/system/run/set",
			"icon":          "mdi:calendar-check",
		}), true},
		Message{"homeassistant/switch/sprinklergo/rain_delay/config", entity(map[string]any{
			"name":          label(lang, "Regenpause", "Rain delay"),
			"unique_id":     "sprinklergo_rain_delay",
			"state_topic":   prefix + "/rain_delay/state",
			"command_topic": prefix + "/rain_delay/set",
			"icon":          "mdi:weather-rainy",
		}), true},
		Message{"homeassistant/button/sprinklergo/stop/config", entity(map[string]any{
			"name":          label(lang, "Alles stoppen", "Stop all"),
			"unique_id":     "sprinklergo_stop",
			"command_topic": prefix + "/stop/set",
			"payload_press": "STOP",
			"icon":          "mdi:stop",
		}), true},
		Message{"homeassistant/sensor/sprinklergo/active_zone/config", entity(map[string]any{
			"name":        label(lang, "Aktive Zone", "Active zone"),
			"unique_id":   "sprinklergo_active_zone",
			"state_topic": prefix + "/active_zone/state",
			"icon":        "mdi:water",
		}), true},
		Message{"homeassistant/sensor/sprinklergo/weather_scale/config", entity(map[string]any{
			"name":                label(lang, "Wetter-Skalierung", "Weather scale"),
			"unique_id":           "sprinklergo_weather_scale",
			"state_topic":         prefix + "/weather_scale/state",
			"unit_of_measurement": "%",
			"icon":                "mdi:weather-partly-rainy",
		}), true},
	)
	return msgs
}

// Command is a parsed inbound MQTT command.
type Command struct {
	Kind string // "zone", "run", "rainDelay", "stop"
	Zone int
	On   bool
}

// parseCommand maps an inbound topic/payload to a Command.
func parseCommand(prefix, topic, payload string) (Command, bool) {
	on := strings.EqualFold(strings.TrimSpace(payload), "ON")
	switch topic {
	case prefix + "/system/run/set":
		return Command{Kind: "run", On: on}, true
	case prefix + "/rain_delay/set":
		return Command{Kind: "rainDelay", On: on}, true
	case prefix + "/stop/set":
		return Command{Kind: "stop"}, true
	}
	var zone int
	if n, err := fmt.Sscanf(topic, prefix+"/zone/%d/set", &zone); err == nil && n == 1 {
		return Command{Kind: "zone", Zone: zone, On: on}, true
	}
	return Command{}, false
}
