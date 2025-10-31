package internal

import (
	"os"
	"path/filepath"
	"testing"
)

// Test that GetExtraTypesConfig reads persisted values from disk when in-memory Config is nil.
func TestGetExtraTypesConfigReadsFromDiskWhenConfigNil(t *testing.T) {
	tmp := t.TempDir()
	// point trails root and config path to temp dir
	oldRoot := TrailarrRoot
	oldConfigPath := ConfigPath
	defer func() {
		TrailarrRoot = oldRoot
		ConfigPath = oldConfigPath
	}()
	TrailarrRoot = tmp
	// ensure config dir exists
	cfgDir := filepath.Join(TrailarrRoot, "config")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	ConfigPath = filepath.Join(cfgDir, "config.yml")

	// write a config file with extraTypes disabling trailers
	content := []byte("extraTypes:\n  trailers: false\n  scenes: true\n")
	WriteConfig(t, content)

	// ensure in-memory Config is nil
	Config = nil

	cfg, err := GetExtraTypesConfig()
	if err != nil {
		t.Fatalf("GetExtraTypesConfig returned error: %v", err)
	}
	if cfg.Trailers != false {
		t.Fatalf("expected trailers=false from disk, got %v", cfg.Trailers)
	}
	if cfg.Scenes != true {
		t.Fatalf("expected scenes=true from disk, got %v", cfg.Scenes)
	}
}
