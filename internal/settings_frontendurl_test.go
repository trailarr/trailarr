package internal

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

const generalSettingsPath = "/api/settings/general"

// Test that the GET handler returns a trimmed frontendUrl (no trailing slash)
func TestGetGeneralSettingsReturnsTrimmedFrontendUrl(t *testing.T) {
	// Setup isolated temp config
	tmp := t.TempDir()
	oldRoot := TrailarrRoot
	oldConfig := GetConfigPath()
	TrailarrRoot = tmp
	defer func() {
		TrailarrRoot = oldRoot
		SetConfigPath(oldConfig)
	}()

	// Initialize minimal config
	CreateTempConfig(t)

	// Inject a frontendUrl with trailing slash
	cfg, err := readConfigFile()
	if err != nil {
		t.Fatalf("readConfigFile error: %v", err)
	}
	general := cfg["general"].(map[string]interface{})
	general["frontendUrl"] = "http://example.local/"
	cfg["general"] = general
	if err := writeConfigFile(cfg); err != nil {
		t.Fatalf("writeConfigFile error: %v", err)
	}

	// Create router and register handler
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET(generalSettingsPath, getGeneralSettingsHandler)

	// Perform GET
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", generalSettingsPath, nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200 from GET, got %d body=%s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}
	fu, _ := resp["frontendUrl"].(string)
	if fu != "http://example.local" {
		t.Fatalf("expected trimmed frontendUrl 'http://example.local', got '%s'", fu)
	}
}

// Test that the POST handler saves a trimmed frontendUrl into config.yml
func TestSaveGeneralSettingsSavesTrimmedFrontendUrl(t *testing.T) {
	tmp := t.TempDir()
	oldRoot := TrailarrRoot
	oldConfig := GetConfigPath()
	TrailarrRoot = tmp
	defer func() {
		TrailarrRoot = oldRoot
		SetConfigPath(oldConfig)
	}()

	// Initialize minimal config
	CreateTempConfig(t)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.POST(generalSettingsPath, saveGeneralSettingsHandler)
	r.GET(generalSettingsPath, getGeneralSettingsHandler)

	// POST a frontendUrl with trailing slash
	payload := map[string]interface{}{"frontendUrl": "http://saved.local/"}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", generalSettingsPath, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200 from POST, got %d body=%s", w.Code, w.Body.String())
	}

	// Read config file and ensure frontendUrl was saved without trailing slash
	cfg, err := readConfigFile()
	if err != nil {
		t.Fatalf("readConfigFile error: %v", err)
	}
	general := cfg["general"].(map[string]interface{})
	if general["frontendUrl"] != "http://saved.local" {
		t.Fatalf("expected persisted frontendUrl 'http://saved.local', got '%v'", general["frontendUrl"])
	}

	// Also verify GET returns the trimmed value
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", generalSettingsPath, nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200 from GET after save, got %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}
	fu, _ := resp["frontendUrl"].(string)
	if fu != "http://saved.local" {
		t.Fatalf("expected GET to return trimmed frontendUrl 'http://saved.local', got '%s'", fu)
	}
}
