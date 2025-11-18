package internal

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestToBoolAndToString(t *testing.T) {
	if b, ok := toBool(true); !ok || !b {
		t.Fatalf("toBool(true) failed")
	}
	if _, ok := toBool("nope"); ok {
		t.Fatalf("toBool should not accept string")
	}
	if s, ok := toString("x"); !ok || s != "x" {
		t.Fatalf("toString failed")
	}
}

func TestToFloat64AndToInt(t *testing.T) {
	if f, ok := toFloat64(3); !ok || f != 3.0 {
		t.Fatalf("toFloat64 int->float failed: %v %v", f, ok)
	}
	if f, ok := toFloat64("2.5"); !ok || f < 2.499 || f > 2.501 {
		t.Fatalf("toFloat64 string parse failed: %v %v", f, ok)
	}
	if i, ok := toInt(7.0); !ok || i != 7 {
		t.Fatalf("toInt float->int failed: %v %v", i, ok)
	}
	if _, ok := toInt("bad"); ok {
		t.Fatalf("toInt should not parse non-int string")
	}
}

func TestConvertTimings(t *testing.T) {
	in := map[string]interface{}{"a": 1, "b": int64(2), "c": 3.0, "d": "4"}
	out := convertTimings(in)
	expected := map[string]int{"a": 1, "b": 2, "c": 3, "d": 4}
	if !reflect.DeepEqual(out, expected) {
		t.Fatalf("convertTimings mismatch: got %v want %v", out, expected)
	}
}

func TestExtractPathMappings(t *testing.T) {
	sec := map[string]interface{}{
		"pathMappings": []interface{}{
			map[string]interface{}{"from": "/from1", "to": "/to1"},
			map[string]interface{}{"from": "", "to": "/to2"},
		},
	}
	got := extractPathMappings(sec)
	if len(got) != 1 || got[0][0] != "/from1" || got[0][1] != "/to1" {
		t.Fatalf("extractPathMappings failed: %v", got)
	}
}

func TestGetEnabledCanonicalExtraTypes(t *testing.T) {
	cfg := ExtraTypesConfig{Trailers: false, Scenes: false, BehindTheScenes: false, Interviews: false, Featurettes: false, DeletedScenes: false, Other: false}
	got := GetEnabledCanonicalExtraTypes(cfg)
	if len(got) != 1 {
		t.Fatalf("expected default single trailer type when none enabled, got %v", got)
	}
	cfg.Trailers = true
	got = GetEnabledCanonicalExtraTypes(cfg)
	if len(got) != 1 {
		t.Fatalf("expected single trailer type when trailers enabled, got %v", got)
	}
}

func TestGetTrustedProxies_DefaultAndConfig(t *testing.T) {
	// Ensure defaults are written when missing
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yml")
	SetConfigPath(cfgPath)
	// Ensure defaults are created
	if err := EnsureConfigDefaults(); err != nil {
		t.Fatalf("EnsureConfigDefaults: %v", err)
	}
	// Read proxies via helper
	proxies, err := GetTrustedProxies()
	if err != nil {
		t.Fatalf("GetTrustedProxies returned error: %v", err)
	}
	if len(proxies) != 1 || proxies[0] != "127.0.0.1" {
		t.Fatalf("Expected default proxies [127.0.0.1], got %v", proxies)
	}
	// Now write a config with a custom proxy and ensure it's returned
	cfgMap := map[string]interface{}{
		"general": map[string]interface{}{
			"trustedProxies": []string{"10.0.0.0/8", "172.16.0.0/12"},
		},
	}
	if err := writeConfigFile(cfgMap); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}
	proxies2, err := GetTrustedProxies()
	if err != nil {
		t.Fatalf("GetTrustedProxies after config returned error: %v", err)
	}
	if len(proxies2) != 2 || proxies2[0] != "10.0.0.0/8" {
		t.Fatalf("Expected proxies from config, got %v", proxies2)
	}
}
