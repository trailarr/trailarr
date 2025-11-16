package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSharedExtrasHandlerMergesPersistentAndTMDB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// prepare persistent extras
	ctx := context.Background()
	// clear extras key in store
	_ = GetStoreClient().Del(ctx, ExtrasStoreKey)

	// add a persistent extra
	pe := ExtrasEntry{MediaType: MediaTypeMovie, MediaId: 5, ExtraType: "Trailer", ExtraTitle: "P1", YoutubeId: "y1", Status: "downloaded"}
	if err := AddOrUpdateExtra(ctx, pe); err != nil {
		t.Fatalf("AddOrUpdateExtra failed: %v", err)
	}

	// Do NOT call external TMDB; sharedExtrasHandler will attempt to fetch TMDB extras but may error — persistent extras should still show up.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{gin.Param{Key: "id", Value: "5"}}
	handler := sharedExtrasHandler(MediaTypeMovie)
	handler(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from sharedExtrasHandler, got %d", w.Code)
	}
	var resp map[string][]Extra
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	// response should include the persistent extra
	found := false
	for _, e := range resp["extras"] {
		if e.YoutubeId == "y1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("persistent extra not present in response: %+v", resp)
	}
}

func TestGetMissingExtrasHandlerReturnsMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := MoviesStoreKey
	// item with id 20 and explicitly wanted
	if err := SaveMediaToStore(cache, []map[string]interface{}{{"id": 20, "wanted": true}}); err != nil {
		t.Fatalf("failed to save wanted cache to store: %v", err)
	}

	// ensure extras collection does not have trailers for id 20
	ctx := context.Background()
	_ = GetStoreClient().Del(ctx, ExtrasStoreKey)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	handler := GetMissingExtrasHandler(cache)
	handler(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from GetMissingExtrasHandler, got %d", w.Code)
	}
	var resp map[string][]map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(resp["items"]) == 0 {
		t.Fatalf("expected missing extras item, got none")
	}
}

func TestUpdateWantedStatusInStoreSeriesDetectsTrailers(t *testing.T) {
	tmp := t.TempDir()
	// make a series path
	seriesRoot := filepath.Join(tmp, "TV", "1923")
	_ = os.MkdirAll(filepath.Join(seriesRoot, "Trailers"), 0o755)
	// create mkv file
	_ = os.WriteFile(filepath.Join(seriesRoot, "Trailers", "Official UK Trailer.mkv"), []byte(""), 0o644)

	// Save a series item with id 292 and path pointing to seriesRoot
	item := map[string]interface{}{"id": 292, "title": "1923", "path": seriesRoot}
	if err := SaveMediaToStore(SeriesStoreKey, []map[string]interface{}{item}); err != nil {
		t.Fatalf("SaveMediaToStore failed: %v", err)
	}

	// Run updateWantedStatusInStore for series
	if err := updateWantedStatusInStore(SeriesStoreKey); err != nil {
		t.Fatalf("updateWantedStatusInStore failed: %v", err)
	}
	// Load store and validate wanted flag is false (since trailer present)
	items, err := LoadMediaFromStore(SeriesStoreKey)
	if err != nil {
		t.Fatalf("LoadMediaFromStore failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("unexpected items: %v", items)
	}
	if wanted, ok := items[0]["wanted"].(bool); !ok || wanted {
		t.Fatalf("expected wanted=false for id=292, got: %v", items[0]["wanted"])
	}
}

func TestProcessLoadedItemsSkipsInvalidMappingForSeries(t *testing.T) {
	tmp := t.TempDir()
	oldRoot := TrailarrRoot
	oldCfg := GetConfigPath()
	TrailarrRoot = tmp
	SetConfigPath(TrailarrRoot + "/config/config.yml")
	_ = os.MkdirAll(filepath.Dir(GetConfigPath()), 0o755)
	// Write a config that maps sonarr path to movies path (invalid mapping)
	cfg := []byte("sonarr:\n  pathMappings:\n    - from: \"/mnt/unionfs/Media/TV\"\n      to: \"/mnt/unionfs/Media/Movies\"\n")
	_ = os.WriteFile(GetConfigPath(), cfg, 0o644)
	defer func() {
		TrailarrRoot = oldRoot
		SetConfigPath(oldCfg)
	}()

	items := []map[string]interface{}{{"id": 292, "path": "/mnt/unionfs/Media/TV/1923"}}
	out := processLoadedItems(items, SeriesStoreKey)
	// Since mapping would convert TV path to Movies, it should be skipped and path should remain the same
	if out[0]["path"] != "/mnt/unionfs/Media/TV/1923" {
		t.Fatalf("expected path unchanged, got: %v", out[0]["path"])
	}
}

func TestUpdateWantedStatusFixesWrongMoviesPathForSeries(t *testing.T) {
	tmp := t.TempDir()
	// create TV folder with Trailers
	tvRoot := filepath.Join(tmp, "Media", "TV", "1923")
	err := os.MkdirAll(filepath.Join(tvRoot, "Trailers"), 0o755)
	if err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	// create mkv under the TV path
	_ = os.WriteFile(filepath.Join(tvRoot, "Trailers", "Official.mkv"), []byte(""), 0o644)

	// save a series item in the store with Movies path (wrong)
	wrongMoviesPath := filepath.Join(tmp, "Media", "Movies", "1923")
	_ = os.MkdirAll(wrongMoviesPath, 0o755)
	item := map[string]interface{}{"id": 292, "title": "1923", "path": wrongMoviesPath}
	if err := SaveMediaToStore(SeriesStoreKey, []map[string]interface{}{item}); err != nil {
		t.Fatalf("SaveMediaToStore failed: %v", err)
	}

	// The update uses raw store path values (no fallback). Since the path
	// is set to Movies (wrong), the trailer under TV won't be detected
	// and wanted should remain true.
	if err := updateWantedStatusInStore(SeriesStoreKey); err != nil {
		t.Fatalf("updateWantedStatusInStore failed: %v", err)
	}
	items, err := LoadMediaFromStore(SeriesStoreKey)
	if err != nil {
		t.Fatalf("LoadMediaFromStore failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got: %v", items)
	}
	wantedVal, ok := items[0]["wanted"].(bool)
	if !ok || !wantedVal {
		t.Fatalf("expected wanted=true because stored path is wrong and fallback disabled, got: %v", items[0]["wanted"])
	}
}

func TestGetMediaByIdReturnsRawStorePath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmp := t.TempDir()
	old := TrailarrRoot
	oldCfg := GetConfigPath()
	TrailarrRoot = tmp
	SetConfigPath(TrailarrRoot + "/config/config.yml")
	_ = os.MkdirAll(filepath.Dir(GetConfigPath()), 0o755)
	// set mapping that would convert TV -> Movies when applied
	cfg := []byte("sonarr:\n  pathMappings:\n    - from: \"/mnt/unionfs/Media/TV\"\n      to: \"/mnt/unionfs/Media/Movies\"\n")
	_ = os.WriteFile(GetConfigPath(), cfg, 0o644)
	defer func() {
		TrailarrRoot = old
		SetConfigPath(oldCfg)
	}()

	// Save a series item with TV path
	tvPath := "/mnt/unionfs/Media/TV/1923"
	item := map[string]interface{}{"id": 292, "path": tvPath}
	if err := SaveMediaToStore(SeriesStoreKey, []map[string]interface{}{item}); err != nil {
		t.Fatalf("SaveMediaToStore failed: %v", err)
	}

	// Request just this id via GetMediaByIdHandler and we should get raw path
	handler := GetMediaByIdHandler(SeriesStoreKey, "id")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	// set a valid request so handler logging doesn't panic
	c.Request = httptest.NewRequest("GET", "/series/292", nil)
	c.Params = gin.Params{gin.Param{Key: "id", Value: "292"}}
	handler(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from GetMediaByIdHandler, got %d", w.Code)
	}
	var resp map[string]map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["item"]["path"] != tvPath {
		t.Fatalf("expected raw TV path to be returned, got: %v", resp["item"]["path"])
	}
}

func TestSharedExtrasHandlerLogsWhenMediaPathMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Clear extras and series store
	ctx := context.Background()
	_ = GetStoreClient().Del(ctx, ExtrasStoreKey)
	_ = GetStoreClient().Del(ctx, SeriesStoreKey)

	// Create a persistent extra for id=999 that does not exist in series store
	pe := ExtrasEntry{MediaType: MediaTypeTV, MediaId: 999, ExtraType: "Trailer", ExtraTitle: "Missing", YoutubeId: "y-missing", Status: "downloaded"}
	if err := AddOrUpdateExtra(ctx, pe); err != nil {
		t.Fatalf("AddOrUpdateExtra failed: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	// Request extras for id 999 (no media path present)
	c.Params = gin.Params{gin.Param{Key: "id", Value: "999"}}
	handler := sharedExtrasHandler(MediaTypeTV)
	handler(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from sharedExtrasHandler, got %d", w.Code)
	}
	// Ensure response returns the persistent extra even when path is missing
	var resp map[string][]Extra
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	found := false
	for _, e := range resp["extras"] {
		if e.YoutubeId == "y-missing" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected persistent extra despite missing path; got: %v", resp)
	}
}
