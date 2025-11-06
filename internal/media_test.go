package internal

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

const (
	mimeJPEG = "image/jpeg"
	mimePNG  = "image/png"
)

// Tests for various helpers in media.go. They are intentionally focused on
// pure helpers and small I/O behaviors that are safe to run in unit tests.

func TestTrimTrailingSlashAndDetectImageExt(t *testing.T) {
	const ex = "http://example"
	if trimTrailingSlash(ex+"/") != ex {
		t.Fatalf("expected trimmed slash")
	}
	if trimTrailingSlash(ex) != ex {
		t.Fatalf("expected unchanged")
	}
	if detectImageExt(mimeJPEG) != ".jpg" {
		t.Fatalf("expected .jpg for jpeg")
	}
	if detectImageExt(mimePNG) != ".png" {
		t.Fatalf("expected .png for png")
	}
}

func TestParseMediaID(t *testing.T) {
	if v, ok := parseMediaID(123); !ok || v != 123 {
		t.Fatalf("int parsing failed")
	}
	if v, ok := parseMediaID(45.0); !ok || v != 45 {
		t.Fatalf("float parsing failed")
	}
	if v, ok := parseMediaID("77"); !ok || v != 77 {
		t.Fatalf("string parsing failed")
	}
	if _, ok := parseMediaID([]int{1}); ok {
		t.Fatalf("unexpected success for invalid type")
	}
}

func TestHasTrailerFiles(t *testing.T) {
	// empty path -> false
	if hasTrailerFiles("") {
		t.Fatalf("expected no trailers for empty path")
	}

	tmp := t.TempDir()
	// case 1: no Trailers dir
	if hasTrailerFiles(tmp) {
		t.Fatalf("expected no trailers when Trailers dir absent")
	}

	// create Trailers with a non-mkv file
	trailers := filepath.Join(tmp, "Trailers")
	if err := os.MkdirAll(trailers, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	_ = os.WriteFile(filepath.Join(trailers, "preview.mp4"), []byte("x"), 0644)
	if hasTrailerFiles(tmp) {
		t.Fatalf("expected no trailers when only non-mkv present")
	}

	// add an mkv file
	_ = os.WriteFile(filepath.Join(trailers, "trailer.MKV"), []byte("x"), 0644)
	if !hasTrailerFiles(tmp) {
		t.Fatalf("expected trailer detection to find .mkv file")
	}

	// also ensure 'Trailer' (singular) name is checked
	tmp2 := t.TempDir()
	single := filepath.Join(tmp2, "Trailer")
	if err := os.MkdirAll(single, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	_ = os.WriteFile(filepath.Join(single, "video.mkv"), []byte("x"), 0644)
	if !hasTrailerFiles(tmp2) {
		t.Fatalf("expected trailer detection to find .mkv in 'Trailer' dir")
	}
}

func TestFetchFirstSuccessful(t *testing.T) {
	// first returns 500, second returns 200
	s1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer s1.Close()
	s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer s2.Close()

	resp, err := fetchFirstSuccessful([]string{s1.URL, s2.URL})
	if err != nil {
		t.Fatalf("expected success, got err: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "ok" {
		t.Fatalf("unexpected body: %s", string(b))
	}
}

func TestSaveToTmpAndStreamAndFallbackAndServeCached(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// saveToTmp
	tmp := t.TempDir()
	tmpf := filepath.Join(tmp, "x.tmp")
	if err := saveToTmp(bytes.NewReader([]byte("abc")), tmpf); err != nil {
		t.Fatalf("saveToTmp failed: %v", err)
	}

	// serveFallbackSVG
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/", nil)
	c.Request = req
	serveFallbackSVG(c)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("fallback svg not ok: %d", w.Result().StatusCode)
	}

	// serveCachedFile (HEAD)
	fpath := filepath.Join(tmp, "file.txt")
	os.WriteFile(fpath, []byte("hello"), 0644)
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	req2 := httptest.NewRequest("HEAD", "/", nil)
	c2.Request = req2
	serveCachedFile(c2, fpath, textPlain)
	if w2.Result().StatusCode != http.StatusOK {
		t.Fatalf("serveCachedFile head failed: %d", w2.Result().StatusCode)
	}
}

func TestFetchAndCachePosterAndCacheMediaPosters(t *testing.T) {
	// create server that serves poster bytes for any path
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("IMGDATA"))
	}))
	defer s.Close()

	tmp := t.TempDir()
	// test fetchAndCachePoster
	local := filepath.Join(tmp, "p.jpg")
	if err := fetchAndCachePoster(local, s.URL+"/p.jpg", "radarr"); err != nil {
		t.Fatalf("fetchAndCachePoster failed: %v", err)
	}
	b, _ := os.ReadFile(local)
	if string(b) != "IMGDATA" {
		t.Fatalf("unexpected cached data")
	}

	// test CacheMediaPosters by writing a minimal config and calling the function
	// set ConfigPath to a temp file with radarr.url = s.URL
	cfg := map[string]interface{}{"radarr": map[string]interface{}{"url": s.URL, "apiKey": ""}}
	cfgPath := filepath.Join(tmp, "cfg.yml")
	data, _ := json.Marshal(cfg)
	// write as YAML-like simple file (the loader uses yaml.Unmarshal which accepts JSON)
	os.WriteFile(cfgPath, data, 0644)
	// override globals (use SetConfigPath to update the atomic-backed path)
	oldRoot := TrailarrRoot
	oldCfg := GetConfigPath()
	TrailarrRoot = tmp
	SetConfigPath(cfgPath)
	defer func() { TrailarrRoot = oldRoot; SetConfigPath(oldCfg) }()

	baseDir := filepath.Join(tmp, "covers")
	idList := []map[string]interface{}{{"id": 123}}
	CacheMediaPosters("radarr", baseDir, idList, "id", []string{"/poster-500.jpg"}, true)
	// file should exist
	// The code constructs localPath as idDir + suffix, and idDir includes a leading slash in suffix
	// ensure we find any file under baseDir/123
	d := filepath.Join(baseDir, "123")
	files, _ := os.ReadDir(d)
	if len(files) == 0 {
		t.Fatalf("expected cached poster file under %s", d)
	}
}

func TestCachedYouTubeImage(t *testing.T) {
	tmp := t.TempDir()
	// create png file
	id := "yt123"
	p := filepath.Join(tmp, id+".png")
	os.WriteFile(p, []byte("x"), 0644)
	path, ct := cachedYouTubeImage(tmp, id)
	if path == "" || ct != "image/png" {
		t.Fatalf("cachedYouTubeImage failed: %s %s", path, ct)
	}
}

func TestDetectMediaTypeAndTitleAndPathUpdate(t *testing.T) {
	// detectMediaTypeAndMainCachePath
	mt, _ := detectMediaTypeAndMainCachePath("/foo/movie/cache.json")
	if mt != MediaTypeMovie {
		t.Fatalf("expected movie type")
	}
	// getTitleMap: write main cache (store-backed) and ensure mapping
	main := MoviesStoreKey
	items := []map[string]interface{}{{"id": 1, "title": "X"}}
	if err := SaveMediaToStore(main, items); err != nil {
		t.Fatalf("failed to save main cache to store: %v", err)
	}
	titleMap := getTitleMap(main, "/some/other.json")
	if titleMap["1"] != "X" {
		t.Fatalf("title map missing")
	}

}

func TestEnsureDirIfNeeded(t *testing.T) {
	tmp := t.TempDir()
	d := filepath.Join(tmp, "a", "b")
	ensureDirIfNeeded(d, "ctx")
	if _, err := os.Stat(d); err != nil {
		t.Fatalf("ensureDirIfNeeded did not create dir: %v", err)
	}
}
