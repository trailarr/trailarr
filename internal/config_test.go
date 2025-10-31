package internal

import (
	"os"
	"testing"
)

func TestEnsureConfigDefaultsAndCanonicalizeConfig(t *testing.T) {
	// Use package-level test root and create a per-test config file there.
	CreateTempConfig(t)
	// Ensure defaults creates a config file
	if err := EnsureConfigDefaults(); err != nil {
		t.Fatalf("EnsureConfigDefaults failed: %v", err)
	}
	// Read canonicalize mapping (should succeed even if empty)
	_, err := GetCanonicalizeExtraTypeConfig()
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("GetCanonicalizeExtraTypeConfig unexpected error: %v", err)
	}
	// Save a mapping and read back
	testMap := CanonicalizeExtraTypeConfig{Mapping: map[string]string{"Trailer": "Trailers"}}
	if err := SaveCanonicalizeExtraTypeConfig(testMap); err != nil {
		t.Fatalf("SaveCanonicalizeExtraTypeConfig failed: %v", err)
	}
	got, err := GetCanonicalizeExtraTypeConfig()
	if err != nil {
		t.Fatalf("GetCanonicalizeExtraTypeConfig after save failed: %v", err)
	}
	if got.Mapping["Trailer"] != "Trailers" {
		t.Fatalf("mapping roundtrip failed: got %v", got.Mapping)
	}
}

func TestEnsureSyncTimingsConfig(t *testing.T) {
	// Use package-level test root and per-test config.
	CreateTempConfig(t)
	timings, err := EnsureSyncTimingsConfig()
	if err != nil {
		t.Fatalf("EnsureSyncTimingsConfig failed: %v", err)
	}
	if _, ok := timings["radarr"]; !ok {
		t.Fatalf("expected radarr timing in default timings, got %v", timings)
	}
}
