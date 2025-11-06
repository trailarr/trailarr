package internal

import "testing"

func TestEnsureGeneralDefaults(t *testing.T) {
	cfg := map[string]interface{}{}
	changed := ensureGeneralDefaults(cfg)
	if !changed {
		t.Fatalf("expected changed true for empty config")
	}
	if cfg["general"] == nil {
		t.Fatalf("general section missing after ensureGeneralDefaults")
	}
}

func TestEnsureYtdlpDefaults(t *testing.T) {
	cfg := map[string]interface{}{}
	if !ensureYtdlpDefaults(cfg) {
		t.Fatalf("expected ensureYtdlpDefaults to set defaults when missing")
	}
	// ensure existing map keeps values (section exists -> no changes)
	cfg2 := map[string]interface{}{"ytdlpFlags": map[string]interface{}{"quiet": true}}
	if ensureYtdlpDefaults(cfg2) {
		t.Fatalf("did not expect changes when ytdlpFlags section exists")
	}
}

func TestEnsureRadarrAndSonarrDefaults(t *testing.T) {
	cfg := map[string]interface{}{}
	if !ensureRadarrDefaults(cfg) {
		t.Fatalf("ensureRadarrDefaults should set defaults")
	}
	if !ensureSonarrDefaults(cfg) {
		t.Fatalf("ensureSonarrDefaults should set defaults")
	}
}

func TestEnsureExtraTypesAndCanonicalizeDefaults(t *testing.T) {
	cfg := map[string]interface{}{}
	if !ensureExtraTypesDefaults(cfg) {
		t.Fatalf("ensureExtraTypesDefaults should set defaults")
	}
	if !ensureCanonicalizeExtraTypeDefaults(cfg) {
		t.Fatalf("ensureCanonicalizeExtraTypeDefaults should set defaults")
	}
}
