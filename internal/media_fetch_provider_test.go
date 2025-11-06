package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFetchProviderItemsFailureNoConfig(t *testing.T) {
	// ensure ConfigPath points to non-existent file
	old := GetConfigPath()
	SetConfigPath(filepath.Join(t.TempDir(), "nope.yml"))
	defer func() { SetConfigPath(old) }()

	if _, err := fetchProviderItems("radarr", "/api/v3/movie"); err == nil {
		t.Fatalf("expected error when provider config missing")
	}
}

func TestFetchProviderItemsSuccess(t *testing.T) {
	// setup provider HTTP server returning JSON array
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		items := []map[string]interface{}{{"id": 7, "title": "T"}}
		_ = json.NewEncoder(w).Encode(items)
	}))
	defer srv.Close()

	// write config file with radarr url
	tmp := t.TempDir()
	cfg := map[string]interface{}{"radarr": map[string]interface{}{"url": srv.URL, "apiKey": ""}}
	cfgPath := filepath.Join(tmp, "cfg.yml")
	data, _ := json.Marshal(cfg)
	_ = os.WriteFile(cfgPath, data, 0644)

	old := GetConfigPath()
	SetConfigPath(cfgPath)
	defer func() { SetConfigPath(old) }()

	items, err := fetchProviderItems("radarr", "/api/v3/movie")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 || items[0]["id"] == nil {
		t.Fatalf("unexpected items: %+v", items)
	}
}
