package internal

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

const ytdlpFlagsSettingsPath = "/api/settings/ytdlpflags"

func TestSaveAndGetYtdlpFlagsHandler(t *testing.T) {
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

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.POST(ytdlpFlagsSettingsPath, SaveYtdlpFlagsConfigHandler)
	r.GET(ytdlpFlagsSettingsPath, GetYtdlpFlagsConfigHandler)

	payload := map[string]interface{}{
		"quiet": true,
	}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", ytdlpFlagsSettingsPath, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200 saving ytdlp flags, got %d body=%s", w.Code, w.Body.String())
	}

	// GET and verify
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", ytdlpFlagsSettingsPath, nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200 getting ytdlp flags, got %d body=%s", w.Code, w.Body.String())
	}
}
