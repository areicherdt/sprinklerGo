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

	"sprinklergo/internal/model"
)

type ConfigStore struct {
	mu   sync.RWMutex
	path string
	cfg  model.Config
}

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
	return nil
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
