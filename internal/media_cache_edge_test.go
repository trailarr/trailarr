package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCacheMediaPostersHandlesFetchFailure(t *testing.T) {
	// server returns 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	tmp := t.TempDir()
	cfg := map[string]interface{}{"radarr": map[string]interface{}{"url": srv.URL, "apiKey": ""}}
	cfgPath := filepath.Join(tmp, "cfg.yml")
	data, _ := json.Marshal(cfg)
	_ = os.WriteFile(cfgPath, data, 0644)
	oldCfg := GetConfigPath()
	SetConfigPath(cfgPath)
	defer func() { SetConfigPath(oldCfg) }()

	baseDir := filepath.Join(tmp, "covers")
	idList := []map[string]interface{}{{"id": 9}}
	// Should not panic; may create directories but file won't be present
	CacheMediaPosters("radarr", baseDir, idList, "id", []string{"/poster.jpg"}, true)
	// check that directory exists but no poster file
	d := filepath.Join(baseDir, "9")
	if _, err := os.Stat(d); err != nil {
		// directory may or may not exist depending on timing, it's acceptable
		return
	}
	// if directory exists, ensure no files
	files, _ := os.ReadDir(d)
	if len(files) != 0 {
		t.Fatalf("expected no cached files when fetch fails, got %d", len(files))
	}
}

func TestCacheMediaPostersHandlesCreateFailure(t *testing.T) {
	// server returns valid image
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("img"))
	}))
	defer srv.Close()

	tmp := t.TempDir()
	cfg := map[string]interface{}{"radarr": map[string]interface{}{"url": srv.URL, "apiKey": ""}}
	cfgPath := filepath.Join(tmp, "cfg.yml")
	data, _ := json.Marshal(cfg)
	_ = os.WriteFile(cfgPath, data, 0644)
	oldCfg := GetConfigPath()
	SetConfigPath(cfgPath)
	defer func() { SetConfigPath(oldCfg) }()

	// Create a file at baseDir/9 so that MkdirAll may still create it, but creating localPath will fail
	baseDir := filepath.Join(tmp, "covers")
	badDir := filepath.Join(baseDir, "9")
	_ = os.MkdirAll(baseDir, 0o755)
	// place a file where directory should be
	_ = os.WriteFile(badDir, []byte("notadir"), 0644)

	idList := []map[string]interface{}{{"id": 9}}
	// Should not panic despite create failure
	CacheMediaPosters("radarr", baseDir, idList, "id", []string{"/poster.jpg"}, true)
}
