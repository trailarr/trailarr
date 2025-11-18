package internal

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetYtdlpFlagsConfigReadsFromDiskWhenConfigNil(t *testing.T) {
	// rely on package-level TestMain temp root
	// ensure per-test config file so background writers don't interfere
	CreateTempConfig(t)
	// Prevent background tasks (healthcheck) from running concurrently
	// and potentially overwriting the config. Save/restore the tasksMeta
	// so other tests remain unaffected. Also clear any queued tasks in
	// the store before writing the config so that previously scheduled
	// jobs don't run and overwrite our changes.
	origTasksMeta := tasksMeta
	tasksMeta = map[TaskID]TaskMeta{}
	ctx := context.Background()
	_ = GetStoreClient().Del(ctx, TaskQueueStoreKey)
	_ = GetStoreClient().Del(ctx, HealthIssuesStoreKey)
	defer func() { tasksMeta = origTasksMeta }()

	// write a config file with specific ytdlpFlags. Include radarr/sonarr
	// sections so background writers don't inject defaults that would
	// overwrite our intended value on CI.
	content := []byte("ytdlpFlags:\n  quiet: true\nradarr:\n  url: http://localhost:7878\n  apiKey: \"\"\n  pathMappings: []\nsonarr:\n  url: http://localhost:8989\n  apiKey: \"\"\n  pathMappings: []\n")
	WriteConfig(t, content)
	// Ensure package-level in-memory Config isn't used so helpers read from disk
	Config = nil
	// Debug: log the written config file contents to help CI debugging
	if b, err := os.ReadFile(GetConfigPath()); err == nil {
		t.Logf("wrote config to %s: %s", GetConfigPath(), string(b))
	} else {
		t.Logf("failed to read back config %s: %v", GetConfigPath(), err)
	}
	Config = nil
	// Retry to avoid transient CI races where background goroutines may
	// overwrite the config after we write it. Try briefly until we observe
	// the expected value or time out.
	var cfg YtdlpFlagsConfig
	var err error
	for attempt := 0; attempt < 20; attempt++ {
		cfg, err = GetYtdlpFlagsConfig()
		if err == nil && cfg.Quiet == true {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
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

	// Prevent background tasks (healthcheck) from running concurrently
	// and potentially overwriting the config. Save/restore the tasksMeta
	// so other tests remain unaffected. Do this before we write the file so
	// no background task reads an empty file and writes defaults afterwards.
	origTasksMeta := tasksMeta
	tasksMeta = map[TaskID]TaskMeta{}
	// Clear any existing queued tasks in the store so previously scheduled
	// healthcheck jobs don't run during this test.
	ctx := context.Background()
	_ = GetStoreClient().Del(ctx, TaskQueueStoreKey)
	_ = GetStoreClient().Del(ctx, HealthIssuesStoreKey)
	defer func() { tasksMeta = origTasksMeta }()

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

	// Force reading from disk rather than in-memory cache which can be stale
	// Call repeatedly to avoid transient CI races where background goroutines
	// may briefly overwrite the config file. Retry briefly until the expected
	// provider values appear or we time out.
	var url, apiKey string
	for attempt := 0; attempt < 20; attempt++ {
		Config = nil
		url, apiKey, err = GetProviderUrlAndApiKey("radarr")
		if err == nil && url == "http://radarr.local" && apiKey == "RKEY" {
			break
		}
		// Sleep briefly before retrying; keep loop fairly quick to avoid
		// slowing down tests but long enough for background writers to finish.
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GetProviderUrlAndApiKey returned error: %v", err)
	}
	if url != "http://radarr.local" || apiKey != "RKEY" {
		t.Fatalf("unexpected provider url/apiKey: %s / %s", url, apiKey)
	}
}
