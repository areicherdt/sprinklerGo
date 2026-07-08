package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sprinklergo/internal/model"
)

// The migration chain must always line up with the model's schema version.
func TestMigrationChainMatchesModelVersion(t *testing.T) {
	if got := len(migrations) + 1; got != model.ConfigVersion {
		t.Fatalf("len(migrations)+1 = %d, but model.ConfigVersion = %d — add the missing migration", got, model.ConfigVersion)
	}
}

func TestMigrateRawV1ToCurrent(t *testing.T) {
	// A pre-retention v1 document, as written by sprinklerd 0.1.
	v1 := []byte(`{"version":1,"settings":{"webPort":8080,"outputType":"none",
		"gpioPins":[17,18,27,22,23,24,25,4,2,3,8,7,10,9,11,14],
		"seasonalAdjust":100,"weatherProvider":"none","clock24h":true},
		"zones":[{"name":"Zone 1","enabled":true,"pump":true}],"schedules":[]}`)

	out, migrated, err := migrateRaw(v1, model.ConfigVersion, migrations)
	if err != nil {
		t.Fatal(err)
	}
	if !migrated {
		t.Fatal("v1 document must be migrated")
	}
	var cfg model.Config
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Version != model.ConfigVersion {
		t.Errorf("version = %d, want %d", cfg.Version, model.ConfigVersion)
	}
	if cfg.Settings.LogRetentionMonths != 24 {
		t.Errorf("logRetentionMonths = %d, want default 24", cfg.Settings.LogRetentionMonths)
	}
	if cfg.Settings.ManualTimerMinutes != 30 {
		t.Errorf("manualTimerMinutes = %d, want default 30", cfg.Settings.ManualTimerMinutes)
	}
	if cfg.Settings.Language != "de" {
		t.Errorf("language = %q, want default de", cfg.Settings.Language)
	}
	if cfg.Settings.SeasonalMode != "global" {
		t.Errorf("seasonalMode = %q, want default global", cfg.Settings.SeasonalMode)
	}
	if cfg.Settings.MetricsEnabled {
		t.Errorf("metricsEnabled = true, want default false")
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("migrated config invalid: %v", err)
	}

	// Idempotence: a current document passes through unchanged.
	if _, migrated, err = migrateRaw(out, model.ConfigVersion, migrations); err != nil || migrated {
		t.Errorf("current document must pass through, got migrated=%v err=%v", migrated, err)
	}
}

func TestMigrateRawPreservesExplicitValue(t *testing.T) {
	v1 := []byte(`{"version":1,"settings":{"logRetentionMonths":6}}`)
	out, _, err := migrateRaw(v1, model.ConfigVersion, migrations)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Settings struct {
			LogRetentionMonths int `json:"logRetentionMonths"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Settings.LogRetentionMonths != 6 {
		t.Errorf("explicit value overwritten: %d", doc.Settings.LogRetentionMonths)
	}
}

func TestMigrateRawRejectsNewerVersion(t *testing.T) {
	newer := []byte(`{"version":99,"settings":{}}`)
	if _, _, err := migrateRaw(newer, model.ConfigVersion, migrations); err == nil {
		t.Fatal("newer config version must be rejected, not downgraded")
	}
}

func TestMigrateRawChainStepFailure(t *testing.T) {
	failing := []migration{func(map[string]any) error { return errors.New("boom") }}
	if _, _, err := migrateRaw([]byte(`{"version":1}`), 2, failing); err == nil {
		t.Fatal("failing migration step must surface an error")
	}
}

// OpenConfig must migrate on-disk v1 files and persist the upgrade.
func TestOpenConfigMigratesV1File(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	v1 := model.DefaultConfig()
	v1.Version = 1
	v1.Settings.LogRetentionMonths = 0
	raw, _ := json.Marshal(map[string]any{
		"version": 1, "settings": stripField(t, v1.Settings), "zones": v1.Zones, "schedules": []any{},
	})
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := OpenConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := s.Snapshot()
	if cfg.Version != model.ConfigVersion || cfg.Settings.LogRetentionMonths != 24 {
		t.Errorf("migration not applied: version=%d retention=%d", cfg.Version, cfg.Settings.LogRetentionMonths)
	}

	// The upgraded document must be persisted.
	data, _ := os.ReadFile(path)
	var onDisk map[string]any
	json.Unmarshal(data, &onDisk)
	if onDisk["version"].(float64) != float64(model.ConfigVersion) {
		t.Errorf("migrated config not persisted: %v", onDisk["version"])
	}
}

// stripField serializes settings without the fields added after v1 to fake
// a v1 file.
func stripField(t *testing.T, s model.Settings) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(s)
	var m map[string]any
	json.Unmarshal(raw, &m)
	delete(m, "logRetentionMonths")
	delete(m, "manualTimerMinutes")
	delete(m, "language")
	delete(m, "seasonalMode")
	delete(m, "seasonalMonthly")
	delete(m, "metricsEnabled")
	return m
}

func TestPrune(t *testing.T) {
	l, err := OpenLog(filepath.Join(t.TempDir(), "zonelog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	old := time.Now().AddDate(0, -3, 0)
	if err := l.LogZoneEvent(old, 0, time.Minute, 0, 100, 100); err != nil {
		t.Fatal(err)
	}
	if err := l.LogZoneEvent(time.Now().Add(-time.Hour), 1, time.Minute, 0, 100, 100); err != nil {
		t.Fatal(err)
	}

	// Retention 0 keeps everything.
	if n, err := l.Prune(0); err != nil || n != 0 {
		t.Errorf("Prune(0) = %d, %v — want 0, nil", n, err)
	}

	n, err := l.Prune(1)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("pruned %d rows, want 1", n)
	}
	entries, err := l.Entries(time.Now().AddDate(-1, 0, 0), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ZoneID != 1 {
		t.Errorf("wrong entries survived: %+v", entries)
	}
}
