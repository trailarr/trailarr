package internal

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fetchGeneralWithRetries attempts to read the config and return the
// parsed config map and the "general" subsection, retrying briefly to
// tolerate background merges/writes during tests.
func fetchGeneralWithRetries() (map[string]interface{}, map[string]interface{}, error) {
	var cfg map[string]interface{}
	var general map[string]interface{}
	var err error
	for i := 0; i < 20; i++ {
		cfg, err = readConfigFile()
		if err == nil {
			if g, ok := cfg["general"].(map[string]interface{}); ok {
				general = g
				if tm, _ := general["tmdbKey"].(string); tm == "abc" {
					return cfg, general, nil
				}
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	return cfg, general, err
}

// TestSettingsPOSTHandlers covers saving radarr/sonarr and general settings via POST
func TestSettingsPOSTHandlers(t *testing.T) {
	// rely on package-level TestMain temp root
	// ensure per-test config file so handlers operate against an isolated config
	CreateTempConfig(t)
	r := NewTestRouter()
	RegisterRoutes(r)

	var lastPostBody string

	// POST radarr settings
	t.Run("radarr", func(t *testing.T) {
		payload := `{"url":"http://example.com","apiKey":"kk"}`
		w := DoRequest(r, "POST", radarrSettingsPath, []byte(payload))
		if w.Code != 200 {
			t.Fatalf("expected 200 saving radarr settings, got %d body=%s", w.Code, w.Body.String())
		}
	})

	// POST sonarr settings
	t.Run("sonarr", func(t *testing.T) {
		payload := `{"url":"http://example.com","apiKey":"kk"}`
		w := DoRequest(r, "POST", "/api/settings/sonarr", []byte(payload))
		if w.Code != 200 {
			t.Fatalf("expected 200 saving sonarr settings, got %d", w.Code)
		}
	})

	// POST general settings (tmdb key and autoDownloadExtras) - handler expects JSON
	t.Run("general", func(t *testing.T) {
		genPayload := `{"tmdbKey":"abc","autoDownloadExtras":true}`
		w := DoRequest(r, "POST", "/api/settings/general", []byte(genPayload))
		if w.Code != 200 {
			t.Fatalf("expected 200 saving general settings, got %d body=%s", w.Code, w.Body.String())
		}
		lastPostBody = w.Body.String()
	})

	// Verify config contents (separated subtest to keep top-level complexity low)
	t.Run("verify", func(t *testing.T) {
		cfg, general, err := fetchGeneralWithRetries()
		if err != nil {
			t.Fatalf("failed to read config after retries: %v", err)
		}
		if tm, _ := general["tmdbKey"].(string); tm != "abc" {
			raw, _ := os.ReadFile(ConfigPath)
			pretty, _ := json.MarshalIndent(cfg, "", "  ")
			t.Logf("ConfigPath=%s", ConfigPath)
			t.Logf("Config file raw contents:\n%s", string(raw))
			t.Logf("Parsed config: %s", string(pretty))
			t.Logf("Last POST response body: %s", lastPostBody)
			t.Fatalf("expected tmdbKey saved as abc, got %v", general["tmdbKey"])
		}
		if auto, ok := general["autoDownloadExtras"].(bool); !ok || auto != true {
			t.Fatalf("expected autoDownloadExtras true, got %v", general["autoDownloadExtras"])
		}
	})

}

// TestExtrasDeleteAndExisting exercises delete extras and existing extras listing
func TestExtrasDeleteAndExisting(t *testing.T) {
	// rely on package-level TestMain temp root

	// seed an extra in the persistent store (persistent collection uses ExtrasEntry)
	entry := ExtrasEntry{
		MediaType:  MediaTypeMovie,
		MediaId:    900,
		ExtraType:  "Trailers",
		ExtraTitle: "X",
		YoutubeId:  "y9",
		Status:     "missing",
	}
	if err := AddOrUpdateExtra(context.Background(), entry); err != nil {
		t.Fatalf("failed to seed extra: %v", err)
	}

	r := NewTestRouter()
	RegisterRoutes(r)

	// create a media path and register media in cache so FindMediaPathByID can locate it
	mediaPath := filepath.Join(TrailarrRoot, "m900")
	_ = os.MkdirAll(filepath.Join(mediaPath, "Trailers"), 0755)
	// create a dummy mkv and meta file so existingExtrasHandler finds it
	_ = os.WriteFile(filepath.Join(mediaPath, "Trailers", "X.mkv"), []byte("x"), 0644)
	meta := `{"extraType":"Trailers","extraTitle":"X","fileName":"X.mkv","youtubeId":"y9","status":"downloaded"}`
	_ = os.WriteFile(filepath.Join(mediaPath, "Trailers", "X.mkv.json"), []byte(meta), 0644)
	movie := map[string]interface{}{"id": 900, "title": "M900", "path": mediaPath}
	if err := SaveMediaToStore(MoviesStoreKey, []map[string]interface{}{movie}); err != nil {
		t.Fatalf("failed to save media to store: %v", err)
	}

	// GET existing extras for this moviePath
	w := DoRequest(r, "GET", "/api/extras/existing?moviePath="+url.QueryEscape(mediaPath), nil)
	if w.Code != 200 {
		t.Fatalf("expected 200 listing existing extras, got %d", w.Code)
	}
	var resp map[string][]map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse existing extras response: %v", err)
	}
	if arr, ok := resp["existing"]; !ok || len(arr) == 0 {
		t.Fatalf("expected at least one existing extra, got %v", resp)
	}

	// DELETE the extra (handler expects mediaType and mediaId)
	delPayload := `{"mediaType":"movie","mediaId":900,"youtubeId":"y9"}`
	w = DoRequest(r, "DELETE", "/api/extras", []byte(delPayload))
	if w.Code != 200 {
		t.Fatalf("expected 200 deleting extra, got %d body=%s", w.Code, w.Body.String())
	}

	// Verify it's gone from persistent extras
	remaining, err := GetAllExtras(context.Background())
	if err != nil {
		t.Fatalf("failed to fetch all extras: %v", err)
	}
	for _, e := range remaining {
		if e.YoutubeId == "y9" {
			t.Fatalf("expected extra y9 to be deleted")
		}
	}

	// cleanup
	_ = GetStoreClient().Del(context.Background(), ExtrasStoreKey)
}
