package internal

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// CreateTempConfig creates a temp config directory and returns its path and sets TrailarrRoot/ConfigPath.
func CreateTempConfig(t *testing.T) string {
	// Do not override TrailarrRoot here; tests should rely on the package-level
	// TestMain-created temp root. CreateTempConfig now only ensures a minimal
	// config file exists at the package `ConfigPath` and returns the current
	// TrailarrRoot.
	oldRoot := TrailarrRoot
	oldConfig := ConfigPath
	t.Cleanup(func() {
		TrailarrRoot = oldRoot
		ConfigPath = oldConfig
	})
	cfgDir := filepath.Join(TrailarrRoot, "config")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	// Use a fixed config filename inside the per-run temp config dir so tests
	// and helpers that expect the default name operate on the same file.
	// This reduces CI flakiness caused by other code reading/writing the
	// canonical `config.yml` name in the same directory.
	ConfigPath = filepath.Join(cfgDir, "config.yml")
	// Write a minimal config file so code that expects sections won't panic.
	// Use the same defaults as production helpers.
	minimal := map[string]interface{}{
		"general":    DefaultGeneralConfig(),
		"ytdlpFlags": DefaultYtdlpFlagsConfig(),
	}
	if err := writeConfigFile(minimal); err != nil {
		t.Fatalf("failed to write initial config file: %v", err)
	}
	return TrailarrRoot
}

// WriteConfig writes content to the current ConfigPath.
func WriteConfig(t *testing.T, content []byte) {
	// Write atomically to avoid races where other goroutines/readers may
	// observe a partially written or empty file. Write to a temp file and
	// rename into place.
	tmp := ConfigPath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		t.Fatalf("failed to create temp config file: %v", err)
	}
	if _, err := f.Write(content); err != nil {
		f.Close()
		t.Fatalf("failed to write temp config file: %v", err)
	}
	// Ensure data is flushed to disk before rename so readers see full content.
	if err := f.Sync(); err != nil {
		f.Close()
		t.Fatalf("failed to sync temp config file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("failed to close temp config file: %v", err)
	}
	if err := os.Rename(tmp, ConfigPath); err != nil {
		t.Fatalf("failed to rename temp config file: %v", err)
	}
	// Debug: read back and log file contents and metadata to help CI debugging
	if b, err := os.ReadFile(ConfigPath); err == nil {
		t.Logf("WriteConfig: wrote %d bytes to %s", len(b), ConfigPath)
		t.Logf("WriteConfig: content:\n%s", string(b))
	} else {
		t.Logf("WriteConfig: failed to read back %s: %v", ConfigPath, err)
	}
	if fi, err := os.Stat(ConfigPath); err == nil {
		t.Logf("WriteConfig: file mod time=%s size=%d", fi.ModTime().Format(time.RFC3339Nano), fi.Size())
	} else {
		t.Logf("WriteConfig: stat failed for %s: %v", ConfigPath, err)
	}
}

// NewTestRouter returns a new Gin engine in release mode for tests.
func NewTestRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	return gin.New()
}

// DoRequest is a small helper to make HTTP requests against a handler.
func DoRequest(r http.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
