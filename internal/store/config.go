// Package store persists the configuration (config.json) and the zone run
// log (SQLite). The config file replaces the original's EEPROM image.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"sprinklergo/internal/model"
)

type ConfigStore struct {
	mu   sync.RWMutex
	path string
	cfg  model.Config
	rev  atomic.Int64
}

// Rev increases with every successful Update; used as a cheap change
// fingerprint by the SSE endpoint.
func (s *ConfigStore) Rev() int64 { return s.rev.Load() }

// OpenConfig loads the configuration from path, creating it with factory
// defaults on first boot (the ResetEEPROM equivalent).
func OpenConfig(path string) (*ConfigStore, error) {
	s := &ConfigStore{path: path}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		s.cfg = model.DefaultConfig()
		if err := s.save(); err != nil {
			return nil, fmt.Errorf("write initial config: %w", err)
		}
		return s, nil
	case err != nil:
		return nil, err
	}
	data, migrated, err := migrateRaw(data, model.ConfigVersion, migrations)
	if err != nil {
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &s.cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := s.cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}
	if migrated {
		if err := s.save(); err != nil {
			return nil, fmt.Errorf("write migrated config: %w", err)
		}
	}
	return s, nil
}

// Snapshot returns a deep copy of the current configuration.
func (s *ConfigStore) Snapshot() model.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Clone()
}

// Update mutates the configuration under lock. The mutation is validated and
// atomically written to disk; on any error the previous state is kept.
func (s *ConfigStore) Update(fn func(*model.Config) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cfg.Clone()
	if err := fn(&next); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	prev := s.cfg
	s.cfg = next
	if err := s.save(); err != nil {
		s.cfg = prev
		return fmt.Errorf("save config: %w", err)
	}
	s.rev.Add(1)
	return nil
}

// ReplaceRaw swaps in a full configuration document (a restored backup),
// migrating and validating it first. Returns the applied configuration.
func (s *ConfigStore) ReplaceRaw(raw []byte) (model.Config, error) {
	data, _, err := migrateRaw(raw, model.ConfigVersion, migrations)
	if err != nil {
		return model.Config{}, err
	}
	var next model.Config
	if err := json.Unmarshal(data, &next); err != nil {
		return model.Config{}, fmt.Errorf("parse backup: %w", err)
	}
	if err := next.Validate(); err != nil {
		return model.Config{}, fmt.Errorf("invalid backup: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.cfg
	s.cfg = next
	if err := s.save(); err != nil {
		s.cfg = prev
		return model.Config{}, fmt.Errorf("save restored config: %w", err)
	}
	s.rev.Add(1)
	return next.Clone(), nil
}

// save writes the config via a temp file + rename so a crash mid-write can
// never leave a truncated config behind. Caller must hold the lock.
func (s *ConfigStore) save() error {
	data, err := json.MarshalIndent(&s.cfg, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".config-*.json.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}
