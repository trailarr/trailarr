package internal

import (
	"os"
	"testing"
)

func TestGetYtdlpFlagsConfigReadsFromDiskWhenConfigNil(t *testing.T) {
	// rely on package-level TestMain temp root
	// ensure per-test config file so background writers don't interfere
	CreateTempConfig(t)
	// write a config file with specific ytdlpFlags
	content := []byte("ytdlpFlags:\n  quiet: true\n  cookiesFromBrowser: firefox\n")
	WriteConfig(t, content)
	// Debug: log the written config file contents to help CI debugging
	if b, err := os.ReadFile(ConfigPath); err == nil {
		t.Logf("wrote config to %s: %s", ConfigPath, string(b))
	} else {
		t.Logf("failed to read back config %s: %v", ConfigPath, err)
	}
	Config = nil
	cfg, err := GetYtdlpFlagsConfig()
	if err != nil {
		t.Fatalf("GetYtdlpFlagsConfig returned error: %v", err)
	}
	if cfg.Quiet != true {
		t.Fatalf("expected Quiet=true from disk, got %v", cfg.Quiet)
	}
	if cfg.CookiesFromBrowser != "firefox" {
		t.Fatalf("expected CookiesFromBrowser=firefox from disk, got %v", cfg.CookiesFromBrowser)
	}
}

func TestGetCanonicalizeExtraTypeConfigReadsFromDisk(t *testing.T) {
	// rely on package-level TestMain temp root
	// ensure per-test config file so background writers don't interfere
	CreateTempConfig(t)
	// write a config file with canonicalize mapping
	content := []byte("canonicalizeExtraType:\n  mapping:\n    Trailer: Trailers\n    Featurette: Featurettes\n")
	WriteConfig(t, content)
	cfg, err := GetCanonicalizeExtraTypeConfig()
	if err != nil {
		t.Fatalf("GetCanonicalizeExtraTypeConfig returned error: %v", err)
	}
	if v, ok := cfg.Mapping["Trailer"]; !ok || v != "Trailers" {
		t.Fatalf("expected mapping Trailer->Trailers, got %v (ok=%v)", v, ok)
	}
	if v, ok := cfg.Mapping["Featurette"]; !ok || v != "Featurettes" {
		t.Fatalf("expected mapping Featurette->Featurettes, got %v (ok=%v)", v, ok)
	}
}

func TestGetPathMappingsAndProviderUrlApiKey(t *testing.T) {
	// rely on package-level TestMain temp root
	// ensure per-test config file so background writers don't interfere
	CreateTempConfig(t)
	// write a config file with radarr section including pathMappings
	content := []byte("radarr:\n  url: http://radarr.local\n  apiKey: RKEY\n  pathMappings:\n    - from: /mnt/movies\n      to: /media/movies\n")
	WriteConfig(t, content)

	mappings, err := GetPathMappings(MediaTypeMovie)
	if err != nil {
		t.Fatalf("GetPathMappings returned error: %v", err)
	}
	if len(mappings) != 1 || mappings[0][0] != "/mnt/movies" || mappings[0][1] != "/media/movies" {
		t.Fatalf("unexpected path mappings: %v", mappings)
	}

	url, apiKey, err := GetProviderUrlAndApiKey("radarr")
	if err != nil {
		t.Fatalf("GetProviderUrlAndApiKey returned error: %v", err)
	}
	if url != "http://radarr.local" || apiKey != "RKEY" {
		t.Fatalf("unexpected provider url/apiKey: %s / %s", url, apiKey)
	}
}
