package internal

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDownloadAssetToFile_RetriesAndSucceeds(t *testing.T) {
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if count == 1 {
			// First attempt: return non-OK so the helper will retry
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		// Second attempt: success
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	tmp, err := os.CreateTemp("", "test-download-*")
	if err != nil {
		t.Fatalf("create tmp: %v", err)
	}
	defer func() { tmp.Close(); _ = os.Remove(tmp.Name()) }()

	if err := downloadAssetToFile(srv.URL, tmp, 2*time.Second); err != nil {
		t.Fatalf("download failed: %v", err)
	}
	// read file
	if _, err := tmp.Seek(0, 0); err != nil {
		t.Fatalf("seek tmp: %v", err)
	}
	b, err := os.ReadFile(tmp.Name())
	if err != nil {
		t.Fatalf("read tmp: %v", err)
	}
	if string(b) != "hello" {
		t.Fatalf("unexpected body: %s", string(b))
	}
}

func TestGetFfmpegDownloadTimeout_EnvAndConfig(t *testing.T) {
	// env override
	os.Setenv("FFMPEG_DOWNLOAD_TIMEOUT", "1s")
	defer os.Unsetenv("FFMPEG_DOWNLOAD_TIMEOUT")
	d, err := GetFfmpegDownloadTimeout()
	if err != nil {
		t.Fatalf("GetFfmpegDownloadTimeout env parse err: %v", err)
	}
	if d < time.Second || d > time.Second+time.Millisecond*1000 {
		t.Fatalf("expected ~1s for env override, got %v", d)
	}
	// Remove env and test config fallback
	os.Unsetenv("FFMPEG_DOWNLOAD_TIMEOUT")
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yml")
	SetConfigPath(cfgPath)
	// write config with custom ffmpegDownloadTimeout
	config := map[string]interface{}{"general": map[string]interface{}{"ffmpegDownloadTimeout": "2s"}}
	if err := writeConfigFile(config); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}
	d2, err := GetFfmpegDownloadTimeout()
	if err != nil {
		t.Fatalf("GetFfmpegDownloadTimeout config parse err: %v", err)
	}
	if d2 < 2*time.Second || d2 > 3*time.Second {
		t.Fatalf("expected ~2s from config, got %v", d2)
	}
}
