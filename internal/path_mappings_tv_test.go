package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetPathMappingsTVAndProviderMissing(t *testing.T) {
	tmp := t.TempDir()
	oldRoot := TrailarrRoot
	oldConfigPath := ConfigPath
	defer func() {
		TrailarrRoot = oldRoot
		ConfigPath = oldConfigPath
	}()
	TrailarrRoot = tmp
	cfgDir := filepath.Join(TrailarrRoot, "config")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	ConfigPath = filepath.Join(cfgDir, "config.yml")

	content := []byte("sonarr:\n  url: http://sonarr.local\n  apiKey: SKEY\n  pathMappings:\n    - from: /mnt/tv\n      to: /media/tv\n")
	WriteConfig(t, content)

	mappings, err := GetPathMappings(MediaTypeTV)
	if err != nil {
		t.Fatalf("GetPathMappings returned error: %v", err)
	}
	if len(mappings) != 1 || mappings[0][0] != "/mnt/tv" || mappings[0][1] != "/media/tv" {
		t.Fatalf("unexpected path mappings for TV: %v", mappings)
	}

	// provider missing case
	_, _, err = GetProviderUrlAndApiKey("notexist")
	if err == nil {
		t.Fatalf("expected error when provider section missing")
	}
}
