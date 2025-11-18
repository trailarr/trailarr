package internal

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSanitizeFileName(t *testing.T) {
	input := "inva/lid\\name:with*chars?\"<>|"
	got := sanitizeFileName(input)
	if got == input {
		t.Fatalf("sanitizeFileName did not change forbidden chars: %s", got)
	}
	// none of the forbidden runes should remain
	forbidden := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	for _, f := range forbidden {
		if bytes.Contains([]byte(got), []byte(f)) {
			t.Fatalf("sanitized name still contains forbidden char %q in %q", f, got)
		}
	}
}

func TestDeduplicateByKey(t *testing.T) {
	list := []map[string]string{
		{"id": "1", "name": "a"},
		{"id": "2", "name": "b"},
		{"id": "1", "name": "a-dup"},
	}
	out := DeduplicateByKey(list, "id")
	if len(out) != 2 {
		t.Fatalf("expected 2 unique items, got %d", len(out))
	}
	ids := map[string]bool{}
	for _, it := range out {
		ids[it["id"]] = true
	}
	if !ids["1"] || !ids["2"] {
		t.Fatalf("unexpected ids in output: %+v", out)
	}
}

func TestParseYtDlpLine(t *testing.T) {
	videoIdSet := map[string]bool{}
	results := &[]gin.H{}
	// build a JSON line similar to yt-dlp output
	item := map[string]string{"id": "abc123", "title": "Test Title", "thumbnail": "http://img"}
	b, _ := json.Marshal(item)
	// call parseYtDlpLine (which expects a []byte)
	parseYtDlpLine(b, videoIdSet, results)
	if len(*results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(*results))
	}
	// calling again with same id should not add duplicate
	parseYtDlpLine(b, videoIdSet, results)
	if len(*results) != 1 {
		t.Fatalf("expected 1 result after duplicate, got %d", len(*results))
	}
}

func TestBuildYtDlpArgs(t *testing.T) {
	// Prepare a minimal downloadInfo
	info := &downloadInfo{TempFile: "tmpfile.mkv"}
	// ensure GetYtdlpFlagsConfig returns valid config
	cfg := DefaultYtdlpFlagsConfig()
	_ = cfg
	// Test with default config
	args := buildYtDlpArgs(info, "ytid", true)
	if len(args) == 0 {
		t.Fatalf("expected args, got none")
	}
	// last arg should be youtube id
	if args[len(args)-1] != "ytid" {
		t.Fatalf("expected last arg to be youtube id, got %v", args[len(args)-1])
	}
	// when impersonate=false, args should not contain "--impersonate"
	args2 := buildYtDlpArgs(info, "ytid", false)
	for i := 0; i < len(args2); i++ {
		if args2[i] == "--impersonate" {
			t.Fatalf("did not expect --impersonate when impersonate=false")
		}
	}
}

// Test that buildYtDlpArgs includes --ffmpeg-location when config or FfmpegPath is set
func TestBuildYtDlpArgs_FfmpegLocation(t *testing.T) {
	td := t.TempDir()
	// Prepare a fake ffmpeg and set FfmpegPath to that file
	ffPath := filepath.Join(td, "ffmpeg")
	if err := os.WriteFile(ffPath, []byte("#!/bin/sh\necho test\n"), 0755); err != nil {
		t.Fatalf("write ffmpeg: %v", err)
	}
	oldFfmpegPath := FfmpegPath
	FfmpegPath = ffPath
	defer func() { FfmpegPath = oldFfmpegPath }()

	// Ensure config is empty to test fallback
	cfg := DefaultYtdlpFlagsConfig()
	cfg.FfmpegLocation = ""
	if err := SaveYtdlpFlagsConfig(cfg); err != nil {
		t.Fatalf("failed to save cfg: %v", err)
	}
	info := &downloadInfo{TempFile: "tmpfile.mkv"}
	args := buildYtDlpArgs(info, "ytid", true)
	found := false
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--ffmpeg-location" && args[i+1] == ffPath {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected args to contain --ffmpeg-location with fallback, got %v", args)
	}

	// Now set explicit config for FfmpegLocation and verify it takes precedence
	cfg.FfmpegLocation = "/custom/ffmpeg"
	if err := SaveYtdlpFlagsConfig(cfg); err != nil {
		t.Fatalf("failed to save cfg: %v", err)
	}
	args2 := buildYtDlpArgs(info, "ytid", true)
	found2 := false
	for i := 0; i < len(args2)-1; i++ {
		if args2[i] == "--ffmpeg-location" && args2[i+1] == cfg.FfmpegLocation {
			found2 = true
			break
		}
	}
	if !found2 {
		t.Fatalf("expected args to contain --ffmpeg-location from config, got %v", args2)
	}
}

// Small sanity test that the runner interface exists and default runner implements it
func TestDefaultRunnerImplementsInterface(t *testing.T) {
	var _ YtDlpRunner = &DefaultYtDlpRunner{}
}
