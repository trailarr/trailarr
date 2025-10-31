package internal

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	yamlv3 "gopkg.in/yaml.v3"
)

const radarrSettingsPath = "/api/settings/radarr"
const radarrBaseURL = "http://radarr.test"

func TestSaveSettingsHandlerWritesFileAndUpdatesConfig(t *testing.T) {
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

	// ensure in-memory Config exists so handler updates it
	Config = map[string]interface{}{}

	// prepare router
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.POST(radarrSettingsPath, SaveSettingsHandler("radarr"))

	payload := map[string]interface{}{
		"providerURL":  radarrBaseURL,
		"apiKey":       "RKEY",
		"pathMappings": []map[string]string{{"from": "/from", "to": "/to"}},
	}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", radarrSettingsPath, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	// verify file written (use os.ReadFile so tests go through the same IO hook)
	data, err := os.ReadFile(ConfigPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	var cfg map[string]interface{}
	if err := yamlv3.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to unmarshal config file: %v", err)
	}
	sec, ok := cfg["radarr"].(map[string]interface{})
	if !ok {
		t.Fatalf("radarr section missing in config file: %v", cfg)
	}
	if sec["url"] != radarrBaseURL {
		t.Fatalf("unexpected url in config file: %v", sec["url"])
	}
	if sec["apiKey"] != "RKEY" {
		t.Fatalf("unexpected apiKey in config file: %v", sec["apiKey"])
	}

	// verify in-memory Config updated
	if inSec, ok := Config["radarr"].(map[string]interface{}); !ok {
		t.Fatalf("Config radarr section not updated in memory: %v", Config)
	} else {
		if inSec["url"] != radarrBaseURL {
			t.Fatalf("in-memory radarr.url mismatch: %v", inSec["url"])
		}
	}
}

func TestGetSettingsHandlerMergesRootFolders(t *testing.T) {
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

	// start test server to simulate Radarr rootfolder API
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/rootfolder" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"path":"/srv/movies"}]`))
	}))
	defer ts.Close()

	// write config with radarr url pointing to test server
	content := []byte("radarr:\n  url: " + ts.URL + "\n  apiKey: RKEY\n  pathMappings: []\n")
	WriteConfig(t, content)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET(radarrSettingsPath, GetSettingsHandler("radarr"))
	req := httptest.NewRequest("GET", radarrSettingsPath, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp["providerURL"] != ts.URL {
		t.Fatalf("unexpected providerURL: %v", resp["providerURL"])
	}
	// pathMappings should include the server's root folder
	mappings, ok := resp["pathMappings"].([]interface{})
	if !ok || len(mappings) == 0 {
		t.Fatalf("expected pathMappings in response, got: %v", resp["pathMappings"])
	}
	// first mapping should have from = /srv/movies
	first, _ := mappings[0].(map[string]interface{})
	if first["from"] != "/srv/movies" {
		t.Fatalf("expected merged folder /srv/movies, got: %v", first["from"])
	}
}
