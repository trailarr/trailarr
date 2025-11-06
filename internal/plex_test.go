package internal

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// Test that plexOAuthHeaders includes the expected keys and preserves the
// provided client identifier.
func TestPlexOAuthHeaders(t *testing.T) {
	hdrs := plexOAuthHeaders("my-client-id")
	if hdrs["X-Plex-Client-Identifier"] != "my-client-id" {
		t.Fatalf("expected client id header to be set, got: %v", hdrs["X-Plex-Client-Identifier"])
	}
	// Spot-check a couple of required headers exist
	if hdrs["Accept"] != "application/json" {
		t.Fatalf("expected Accept header application/json, got: %v", hdrs["Accept"])
	}
	if hdrs["Content-Type"] != "application/json" {
		t.Fatalf("expected Content-Type header application/json, got: %v", hdrs["Content-Type"])
	}
}

// Test generateUUID returns a non-empty hex string (or well-formed fallback)
func TestGenerateUUID(t *testing.T) {
	u := generateUUID()
	if u == "" {
		t.Fatalf("generateUUID returned empty string")
	}
	// When rand.Read succeeds the UUID is 32 hex chars. Accept either that
	// or the timestamp-based fallback which contains dashes.
	matchHex := regexp.MustCompile(`^[0-9a-fA-F]{32}$`)
	if !matchHex.MatchString(u) {
		// Accept fallback pattern with dashes as a secondary check
		matchFallback := regexp.MustCompile(`^[0-9a-fA-F\-]+$`)
		if !matchFallback.MatchString(u) {
			t.Fatalf("generated uuid has unexpected format: %s", u)
		}
	}
}

// Ensure EnsurePlexClientID generates and persists a clientId into the config file.
func TestEnsurePlexClientIDCreatesAndPersists(t *testing.T) {
	// Use package test temp root (set in TestMain). Create a minimal config.
	// CreateTempConfig will set ConfigPath and write a minimal YAML file.
	CreateTempConfig(t)

	// Remove any existing plex section so EnsurePlexClientID will generate one.
	// Load and write an explicit minimal config to ensure predictable state.
	cfgDir := filepath.Dir(GetConfigPath())
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Start with a minimal config that does NOT include plex
	minimal := []byte("general:\n  logLevel: Info\n")
	if err := os.WriteFile(GetConfigPath(), minimal, 0644); err != nil {
		t.Fatalf("failed to write minimal config: %v", err)
	}

	id, err := EnsurePlexClientID()
	if err != nil {
		t.Fatalf("EnsurePlexClientID returned error: %v", err)
	}
	if id == "" {
		t.Fatalf("EnsurePlexClientID returned empty id")
	}

	// Re-read via GetPlexConfig to verify persisted clientId
	got, err := GetPlexConfig()
	if err != nil {
		t.Fatalf("GetPlexConfig returned error after EnsurePlexClientID: %v", err)
	}
	if got.ClientId == "" {
		t.Fatalf("expected clientId to be persisted in config, got empty")
	}
	if got.ClientId != id {
		t.Fatalf("mismatch persisted clientId (want %s got %s)", id, got.ClientId)
	}

	// Calling EnsurePlexClientID again should return the same id
	id2, err := EnsurePlexClientID()
	if err != nil {
		t.Fatalf("EnsurePlexClientID (2) returned error: %v", err)
	}
	if id2 != id {
		t.Fatalf("EnsurePlexClientID returned different id on second call: %s vs %s", id, id2)
	}
}

// Test SavePlexConfig preserves existing token and clientId when incoming
// payload omits them (empty strings).
func TestSavePlexConfigPreservesTokenAndClientId(t *testing.T) {
	CreateTempConfig(t)

	// Write an initial config with plex.token and plex.clientId set
	initial := []byte("plex:\n  protocol: http\n  ip: localhost\n  port: 32400\n  token: EXISTINGTOKEN\n  clientId: EXISTINGCLIENT\n  enabled: true\n")
	// Use WriteConfig helper for atomic write and better test diagnostics
	WriteConfig(t, initial)

	// (no diagnostics)

	// Save new config that omits token and clientId (empty values)
	err := SavePlexConfig(PlexConfig{
		Protocol: "https",
		IP:       "plex.test",
		Port:     443,
		Token:    "",
		ClientId: "",
		Enabled:  false,
	})
	if err != nil {
		t.Fatalf("SavePlexConfig returned error: %v", err)
	}

	got, err := GetPlexConfig()
	if err != nil {
		t.Fatalf("GetPlexConfig returned error: %v", err)
	}
	if got.Token != "EXISTINGTOKEN" {
		t.Fatalf("expected token to be preserved, got: %v", got.Token)
	}
	if got.ClientId != "EXISTINGCLIENT" {
		t.Fatalf("expected clientId to be preserved, got: %v", got.ClientId)
	}
	// Fields we set should have been saved/updated
	if got.IP != "plex.test" || got.Protocol != "https" || got.Port != 443 {
		t.Fatalf("expected updated fields to be saved, got: %+v", got)
	}
}
