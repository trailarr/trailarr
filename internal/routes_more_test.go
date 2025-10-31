package internal

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsAndFilesListAndDownloadExtra(t *testing.T) {
	// rely on package-level TestMain temp root
	_ = CreateTempConfig(t) // keep call for backward compatibility of config write behavior but ignore returned tmp

	// start with a small config containing radarr/sonarr settings
	WriteConfig(t, []byte(`radarr:
	url: http://localhost:1
	apiKey: k
sonarr:
	url: http://localhost:1
	apiKey: k
`))

	r := NewTestRouter()
	RegisterRoutes(r)

	// Use fake yt-dlp runner for this test to avoid spawning external process
	oldRunner := ytDlpRunner
	ytDlpRunner = &fakeRunner{}
	defer func() { ytDlpRunner = oldRunner }()

	// GET settings general should return default tmdbKey empty and autoDownloadExtras true
	w := DoRequest(r, "GET", "/api/settings/general", nil)
	if w.Code != 200 {
		t.Fatalf("expected 200 for general settings, got %d", w.Code)
	}

	// Test /api/files/list without path: returns allowed roots
	w = DoRequest(r, "GET", "/api/files/list", nil)
	if w.Code != 200 {
		t.Fatalf("expected 200 for files list root, got %d", w.Code)
	}
	var filesResp map[string][]string
	if err := json.Unmarshal(w.Body.Bytes(), &filesResp); err != nil {
		t.Fatalf("failed to parse files list response: %v", err)
	}
	if _, ok := filesResp["folders"]; !ok {
		t.Fatalf("expected folders key in files list response")
	}

	// Test /api/files/list with invalid path
	w = DoRequest(r, "GET", "/api/files/list?path=/etc/passwd", nil)
	if w.Code == 200 {
		t.Fatalf("expected non-200 for invalid path, got 200")
	}

	// Test POST /api/extras/download enqueues item and writes meta file when media path exists
	// seed a movie entry with a path (use global TrailarrRoot)
	movie := map[string]interface{}{"id": 55, "title": "Z", "path": filepath.Join(TrailarrRoot, "m55")}
	_ = os.MkdirAll(movie["path"].(string), 0755)
	_ = SaveMediaToStore(MoviesStoreKey, []map[string]interface{}{movie})

	payload := `{"mediaType":"movie","mediaId":55,"extraType":"Trailers","extraTitle":"ET","youtubeId":"ytx"}`
	w = DoRequest(r, "POST", "/api/extras/download", []byte(payload))
	if w.Code != 200 {
		t.Fatalf("expected 200 on download request, got %d body=%s", w.Code, w.Body.String())
	}

	// Verify that a meta file was created under the movie path
	metaGlob := filepath.Join(movie["path"].(string), "Trailers", "ET.mkv.json")
	if _, err := os.Stat(metaGlob); err != nil {
		t.Fatalf("expected meta file created: %s (err=%v)", metaGlob, err)
	}

	// Cleanup queue
	_ = GetStoreClient().Del(context.Background(), DownloadQueue)
}

func TestLogsListHandler(t *testing.T) {
	_ = CreateTempConfig(t)
	// create a log file
	fpath := filepath.Join(LogsDir, "a.txt")
	if err := os.WriteFile(fpath, []byte("x"), 0644); err != nil {
		t.Fatalf("failed to create log file: %v", err)
	}
	r := NewTestRouter()
	RegisterRoutes(r)
	w := DoRequest(r, "GET", "/api/logs/list", nil)
	if w.Code != 200 {
		t.Fatalf("expected 200 for logs list, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "a.txt") {
		t.Fatalf("expected a.txt in logs response, got %s", w.Body.String())
	}
}
