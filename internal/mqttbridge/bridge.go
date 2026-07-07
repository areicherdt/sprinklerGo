package mqttbridge

import (
	"context"
	"log/slog"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"sprinklergo/internal/engine"
	"sprinklergo/internal/model"
	"sprinklergo/internal/store"
	"sprinklergo/internal/weather"
)

// Bridge connects the controller to an MQTT broker according to the
// settings; it reconnects when the MQTT settings change and republishes
// state whenever the engine or configuration changes.
type Bridge struct {
	cfg     *store.ConfigStore
	eng     *engine.Engine
	weather *weather.Cache
	version string

	client    mqtt.Client
	connFP    string
	nextRetry time.Time
}

func New(cfg *store.ConfigStore, eng *engine.Engine, wcache *weather.Cache, version string) *Bridge {
	return &Bridge{cfg: cfg, eng: eng, weather: wcache, version: version}
}

func mqttFingerprint(s model.Settings) string {
	if !s.MQTTEnabled {
		return ""
	}
	return s.MQTTBroker + "\x00" + s.MQTTUsername + "\x00" + s.MQTTPassword + "\x00" +
		s.MQTTTopicPrefix + "\x00" + map[bool]string{true: "d1", false: "d0"}[s.MQTTHADiscovery]
}

// Run drives the bridge until ctx ends. It is safe to run with MQTT
// disabled; the loop idles and reacts once the settings enable it.
func (b *Bridge) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var lastState [2]int64
	for {
		select {
		case <-ctx.Done():
			b.disconnect()
			return
		case <-ticker.C:
		}
		cfg := b.cfg.Snapshot()
		fp := mqttFingerprint(cfg.Settings)
		if fp == "" {
			b.disconnect()
			continue
		}
		if b.client == nil || b.connFP != fp {
			b.disconnect()
			if time.Now().Before(b.nextRetry) {
				continue
			}
			if err := b.connect(cfg.Settings); err != nil {
				slog.Warn("mqtt connect failed", "broker", cfg.Settings.MQTTBroker, "err", err)
				b.nextRetry = time.Now().Add(15 * time.Second)
				continue
			}
			b.connFP = fp
			lastState = [2]int64{-1, -1} // force full publish
		}
		if state := [2]int64{b.eng.Rev(), b.cfg.Rev()}; state != lastState {
			cfgChanged := state[1] != lastState[1]
			lastState = state
			b.publish(&cfg, cfgChanged && cfg.Settings.MQTTHADiscovery)
		}
	}
}

func (b *Bridge) connect(s model.Settings) error {
	prefix := s.MQTTTopicPrefix
	opts := mqtt.NewClientOptions().
		AddBroker(s.MQTTBroker).
		SetClientID("sprinklergo").
		SetUsername(s.MQTTUsername).
		SetPassword(s.MQTTPassword).
		SetAutoReconnect(true).
		SetConnectTimeout(10*time.Second).
		SetWill(availabilityTopic(prefix), "offline", 1, true)

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		c.Publish(availabilityTopic(prefix), 1, true, "online")
		c.Subscribe(prefix+"/zone/+/set", 1, b.onCommand(prefix))
		c.Subscribe(prefix+"/system/run/set", 1, b.onCommand(prefix))
		c.Subscribe(prefix+"/rain_delay/set", 1, b.onCommand(prefix))
		c.Subscribe(prefix+"/stop/set", 1, b.onCommand(prefix))
		slog.Info("mqtt connected", "broker", s.MQTTBroker, "prefix", prefix)
	})

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(12*time.Second) || token.Error() != nil {
		client.Disconnect(0)
		if token.Error() != nil {
			return token.Error()
		}
		return context.DeadlineExceeded
	}
	b.client = client
	return nil
}

func (b *Bridge) disconnect() {
	if b.client == nil {
		return
	}
	if b.client.IsConnected() {
		prefix := b.cfg.Snapshot().Settings.MQTTTopicPrefix
		b.client.Publish(availabilityTopic(prefix), 1, true, "offline").WaitTimeout(2 * time.Second)
	}
	b.client.Disconnect(250)
	b.client = nil
	b.connFP = ""
}

func (b *Bridge) publish(cfg *model.Config, includeDiscovery bool) {
	if b.client == nil || !b.client.IsConnected() {
		return
	}
	prefix := cfg.Settings.MQTTTopicPrefix
	msgs := stateMessages(cfg, b.eng.State(), b.weather.Scale(), prefix)
	if includeDiscovery {
		msgs = append(discoveryMessages(cfg, b.version, prefix), msgs...)
	}
	for _, m := range msgs {
		b.client.Publish(m.Topic, 0, m.Retained, m.Payload)
	}
}

func (b *Bridge) onCommand(prefix string) mqtt.MessageHandler {
	return func(_ mqtt.Client, msg mqtt.Message) {
		cmd, ok := parseCommand(prefix, msg.Topic(), string(msg.Payload()))
		if !ok {
			return
		}
		slog.Info("mqtt command", "topic", msg.Topic(), "payload", string(msg.Payload()))
		switch cmd.Kind {
		case "zone":
			minutes := 0
			if cmd.On {
				minutes = b.cfg.Snapshot().Settings.ManualTimerMinutes
			}
			if err := b.eng.SetManualZone(cmd.Zone, cmd.On, minutes); err != nil {
				slog.Warn("mqtt zone command rejected", "zone", cmd.Zone, "err", err)
			}
		case "run":
			err := b.cfg.Update(func(c *model.Config) error {
				c.Settings.RunSchedules = cmd.On
				return nil
			})
			if err != nil {
				slog.Warn("mqtt run command failed", "err", err)
				return
			}
			if cmd.On {
				b.eng.Reload()
			} else {
				b.eng.StopAll()
			}
		case "rainDelay":
			until := int64(0)
			if cmd.On {
				until = time.Now().Add(24 * time.Hour).Unix()
			}
			err := b.cfg.Update(func(c *model.Config) error {
				c.RainDelayUntil = until
				return nil
			})
			if err != nil {
				slog.Warn("mqtt rain delay command failed", "err", err)
			}
		case "stop":
			b.eng.StopAll()
		}
	}
}
