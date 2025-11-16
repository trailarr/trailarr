package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGetMediaHandlerListAndFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := MoviesStoreKey
	items := []map[string]interface{}{{"id": 1, "title": "A"}, {"id": 2, "title": "B"}}
	if err := SaveMediaToStore(cache, items); err != nil {
		t.Fatalf("failed to save cache to store: %v", err)
	}

	// GET all
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/?", nil)
	c.Request = req
	handler := GetMediaHandler(cache, "id")
	handler(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string][]map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if len(resp["items"]) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp["items"]))
	}

	// GET with id filter
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	req2 := httptest.NewRequest("GET", "/?id=2", nil)
	c2.Request = req2
	handler(c2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 on filtered, got %d", w2.Code)
	}
	var resp2 map[string][]map[string]interface{}
	if err := json.Unmarshal(w2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("invalid json response filtered: %v", err)
	}
	if len(resp2["items"]) != 1 {
		t.Fatalf("expected 1 item after filter, got %d", len(resp2["items"]))
	}
}

// Ensure GetMediaHandler returns raw store path values (no mapping applied) for list endpoint
func TestGetMediaHandlerReturnsRawStoreValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmp := t.TempDir()
	old := TrailarrRoot
	oldCfg := GetConfigPath()
	TrailarrRoot = tmp
	SetConfigPath(TrailarrRoot + "/config/config.yml")
	_ = os.MkdirAll(filepath.Dir(GetConfigPath()), 0o755)
	// Configure a mapping that would convert TV -> Movies if applied
	cfg := []byte("sonarr:\n  pathMappings:\n    - from: \"/mnt/unionfs/Media/TV\"\n      to: \"/mnt/unionfs/Media/Movies\"\n")
	_ = os.WriteFile(GetConfigPath(), cfg, 0o644)
	defer func() {
		TrailarrRoot = old
		SetConfigPath(oldCfg)
	}()

	// Save a series item with TV path
	tvPath := "/mnt/unionfs/Media/TV/1923"
	if err := SaveMediaToStore(SeriesStoreKey, []map[string]interface{}{{"id": 292, "path": tvPath}}); err != nil {
		t.Fatalf("SaveMediaToStore failed: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/?", nil)
	c.Request = req
	handler := GetMediaHandler(SeriesStoreKey, "id")
	handler(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string][]map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	// Expect returned path to be raw (tvPath) not mapped to Movies path
	items := resp["items"]
	found := false
	for _, it := range items {
		if id, ok := it["id"].(float64); ok && int(id) == 292 {
			if p, ok := it["path"].(string); ok && p == tvPath {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("expected raw tv path in returned items, got: %v", resp)
	}
}

func TestGetMediaByIdHandlerFoundAndNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := MoviesStoreKey
	items := []map[string]interface{}{{"id": 10, "title": "X"}}
	if err := SaveMediaToStore(cache, items); err != nil {
		t.Fatalf("failed to save cache to store: %v", err)
	}

	handler := GetMediaByIdHandler(cache, "id")

	// found
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/items/10", nil)
	c.Request = req
	c.Params = gin.Params{gin.Param{Key: "id", Value: "10"}}
	handler(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for found item, got %d", w.Code)
	}

	// not found
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	req2 := httptest.NewRequest("GET", "/items/20", nil)
	c2.Request = req2
	c2.Params = gin.Params{gin.Param{Key: "id", Value: "20"}}
	handler(c2)
	if w2.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing item, got %d", w2.Code)
	}
}
