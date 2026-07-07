package mqttbridge

import (
	"encoding/json"
	"strings"
	"testing"

	"sprinklergo/internal/engine"
	"sprinklergo/internal/model"
)

func testCfg() model.Config {
	cfg := model.DefaultConfig()
	cfg.Zones[0].Name = "Rasen"
	cfg.Zones[1].Enabled = true
	cfg.Zones[1].Name = "Beet"
	cfg.Settings.RunSchedules = true
	return cfg
}

func find(msgs []Message, topic string) *Message {
	for i := range msgs {
		if msgs[i].Topic == topic {
			return &msgs[i]
		}
	}
	return nil
}

func TestStateMessages(t *testing.T) {
	cfg := testCfg()
	cfg.RainDelayUntil = 12345
	st := engine.State{Mode: "manual", ZoneID: 1, RemainingSeconds: 120,
		ZoneOn: []bool{false, true, false}}

	msgs := stateMessages(&cfg, st, 80, "sg")

	for topic, want := range map[string]string{
		"sg/zone/0/state":        "OFF",
		"sg/zone/1/state":        "ON",
		"sg/system/run/state":    "ON",
		"sg/rain_delay/state":    "ON",
		"sg/active_zone/state":   "Beet",
		"sg/weather_scale/state": "80",
	} {
		m := find(msgs, topic)
		if m == nil || m.Payload != want || !m.Retained {
			t.Errorf("%s: got %+v, want retained %q", topic, m, want)
		}
	}
	// Disabled zones must not publish state.
	if find(msgs, "sg/zone/2/state") != nil {
		t.Error("disabled zone must not publish state")
	}
	blob := find(msgs, "sg/state")
	var parsed map[string]any
	if blob == nil || json.Unmarshal([]byte(blob.Payload), &parsed) != nil {
		t.Fatalf("sg/state missing or invalid: %+v", blob)
	}
	if parsed["mode"] != "manual" || parsed["weatherScale"].(float64) != 80 {
		t.Errorf("state blob wrong: %v", parsed)
	}
}

func TestDiscoveryMessages(t *testing.T) {
	cfg := testCfg()
	msgs := discoveryMessages(&cfg, "1.0.0", "sg")

	z0 := find(msgs, "homeassistant/switch/sprinklergo/zone0/config")
	if z0 == nil || !z0.Retained {
		t.Fatal("zone 0 discovery missing")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(z0.Payload), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["name"] != "Rasen" || payload["command_topic"] != "sg/zone/0/set" ||
		payload["availability_topic"] != "sg/availability" {
		t.Errorf("zone discovery payload wrong: %v", payload)
	}
	device, _ := payload["device"].(map[string]any)
	if device == nil || device["sw_version"] != "1.0.0" {
		t.Errorf("device block wrong: %v", device)
	}

	// Disabled zone removes its entity via empty retained payload.
	z2 := find(msgs, "homeassistant/switch/sprinklergo/zone2/config")
	if z2 == nil || z2.Payload != "" || !z2.Retained {
		t.Errorf("disabled zone must publish empty retained config: %+v", z2)
	}

	for _, topic := range []string{
		"homeassistant/switch/sprinklergo/run/config",
		"homeassistant/switch/sprinklergo/rain_delay/config",
		"homeassistant/button/sprinklergo/stop/config",
		"homeassistant/sensor/sprinklergo/active_zone/config",
		"homeassistant/sensor/sprinklergo/weather_scale/config",
	} {
		if m := find(msgs, topic); m == nil || !strings.Contains(m.Payload, "unique_id") {
			t.Errorf("%s missing or incomplete", topic)
		}
	}
}

func TestParseCommand(t *testing.T) {
	for _, tc := range []struct {
		topic, payload string
		want           Command
		ok             bool
	}{
		{"sg/zone/3/set", "ON", Command{Kind: "zone", Zone: 3, On: true}, true},
		{"sg/zone/3/set", "off", Command{Kind: "zone", Zone: 3, On: false}, true},
		{"sg/system/run/set", "ON", Command{Kind: "run", On: true}, true},
		{"sg/rain_delay/set", "OFF", Command{Kind: "rainDelay", On: false}, true},
		{"sg/stop/set", "STOP", Command{Kind: "stop"}, true},
		{"sg/unknown/set", "ON", Command{}, false},
		{"other/zone/1/set", "ON", Command{}, false},
	} {
		got, ok := parseCommand("sg", tc.topic, tc.payload)
		if ok != tc.ok || got != tc.want {
			t.Errorf("parseCommand(%q,%q) = %+v/%v, want %+v/%v", tc.topic, tc.payload, got, ok, tc.want, tc.ok)
		}
	}
}
