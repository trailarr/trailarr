package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetYtdlpFlagsConfigReadsFromDiskWhenConfigNil(t *testing.T) {
	// rely on package-level TestMain temp root
	// ensure per-test config file so background writers don't interfere
	CreateTempConfig(t)
	// write a config file with specific ytdlpFlags. Include radarr/sonarr
	// sections so background writers don't inject defaults that would
	// overwrite our intended value on CI.
	content := []byte("ytdlpFlags:\n  quiet: true\nradarr:\n  url: http://localhost:7878\n  apiKey: \"\"\n  pathMappings: []\nsonarr:\n  url: http://localhost:8989\n  apiKey: \"\"\n  pathMappings: []\n")
	WriteConfig(t, content)
	// Debug: log the written config file contents to help CI debugging
	if b, err := os.ReadFile(GetConfigPath()); err == nil {
		t.Logf("wrote config to %s: %s", GetConfigPath(), string(b))
	} else {
		t.Logf("failed to read back config %s: %v", GetConfigPath(), err)
	}
	Config = nil
	cfg, err := GetYtdlpFlagsConfig()
	if err != nil {
		t.Fatalf("GetYtdlpFlagsConfig returned error: %v", err)
	}
	if cfg.Quiet != true {
		t.Fatalf("expected Quiet=true from disk, got %v", cfg.Quiet)
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
	// Use a per-test temp dir to avoid interference from other tests
	tmp := t.TempDir()
	oldRoot := TrailarrRoot
	oldConfig := GetConfigPath()
	defer func() {
		TrailarrRoot = oldRoot
		SetConfigPath(oldConfig)
	}()
	TrailarrRoot = tmp
	cfgDir := filepath.Join(TrailarrRoot, "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	SetConfigPath(filepath.Join(cfgDir, "config.yml"))

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
