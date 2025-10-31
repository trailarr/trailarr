package internal

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSanitizeFilenameAndListSubdirs(t *testing.T) {
	tmp := t.TempDir()
	// create subdirs
	_ = os.MkdirAll(filepath.Join(tmp, "A"), 0o755)
	_ = os.MkdirAll(filepath.Join(tmp, "B"), 0o755)
	dirs, err := ListSubdirectories(tmp)
	if err != nil {
		t.Fatalf("ListSubdirectories failed: %v", err)
	}
	if len(dirs) != 2 {
		t.Fatalf("unexpected subdir count: %v", dirs)
	}

	// sanitize
	s := SanitizeFilename(`bad:\/:*?"<>|name `)
	if s == "bad\\/:*?\"<>|name" {
		t.Fatalf("SanitizeFilename did not change invalid chars: %s", s)
	}
}

func TestMarkRejectedExtrasInMemory(t *testing.T) {
	extras := []Extra{{YoutubeId: "a"}, {YoutubeId: "b"}, {YoutubeId: "c"}}
	rej := map[string]struct{}{"b": {}}
	MarkRejectedExtrasInMemory(extras, rej)
	if extras[1].Status != "rejected" {
		t.Fatalf("expected b to be rejected: %v", extras)
	}
}

func TestCollectExistingFromSubdirAndScan(t *testing.T) {
	tmp := t.TempDir()
	sub := filepath.Join(tmp, "ExtrasType")
	_ = os.MkdirAll(sub, 0o755)
	// create mkv file and meta
	file := filepath.Join(sub, "MyExtra.mkv")
	_ = os.WriteFile(file, []byte("dummy"), 0o644)
	meta := filepath.Join(sub, "MyExtra.mkv.json")
	_ = os.WriteFile(meta, []byte(`{"extraType":"Type","extraTitle":"MyExtra","fileName":"MyExtra.mkv","youtubeId":"y1","status":"downloaded"}`), 0o644)

	dup := make(map[string]int)
	res := collectExistingFromSubdir(sub, dup)
	if len(res) != 1 {
		t.Fatalf("collectExistingFromSubdir unexpected: %v", res)
	}

	scan := ScanExistingExtras(tmp)
	if len(scan) == 0 {
		t.Fatalf("ScanExistingExtras failed: %v", scan)
	}
}

func TestMarkDownloadedExtrasAndCanonicalizeMeta(t *testing.T) {
	// prepare extras and a media path with mkv files
	tmp := t.TempDir()
	movieDir := filepath.Join(tmp, "Movie")
	_ = os.MkdirAll(filepath.Join(movieDir, "Type"), 0o755)
	_ = os.WriteFile(filepath.Join(movieDir, "Type", "MyExtra.mkv"), []byte(""), 0o644)

	extras := []Extra{{ExtraType: "Type", ExtraTitle: "MyExtra", YoutubeId: "y1"}, {ExtraType: "Other", ExtraTitle: "Nope", YoutubeId: "y2"}}
	MarkDownloadedExtras(extras, movieDir, "type", "title")
	// since we placed a file under tmp/Type/MyExtra.mkv, the key should be Type|MyExtra
	found := false
	for _, e := range extras {
		if e.ExtraTitle == "MyExtra" && e.Status == "downloaded" {
			found = true
		}
	}
	if !found {
		t.Fatalf("MarkDownloadedExtras did not mark downloaded: %v", extras)
	}

	// canonicalizeMeta
	meta := map[string]interface{}{"title": "T", "fileName": "F", "youtubeId": "Y", "status": "S", "other": 1}
	canon := canonicalizeMeta(meta)
	if reflect.DeepEqual(canon, meta) {
		t.Fatalf("canonicalizeMeta did not change keys: %v", canon)
	}
}

func TestDeleteExtraFiles(t *testing.T) {
	tmp := t.TempDir()
	mediaPath := filepath.Join(tmp, "M")
	extraDir := filepath.Join(mediaPath, "Type")
	_ = os.MkdirAll(extraDir, 0o755)
	// create only meta file, not mkv
	meta := filepath.Join(extraDir, "File.mkv.json")
	_ = os.WriteFile(meta, []byte("{}"), 0o644)
	// deleteExtraFiles should succeed even if mkv missing
	if err := deleteExtraFiles(mediaPath, "Type", "File"); err != nil {
		t.Fatalf("deleteExtraFiles failed: %v", err)
	}
	// now remove both files to provoke error
	_ = os.Remove(meta)
	if err := deleteExtraFiles(mediaPath, "Type", "File"); err == nil {
		t.Fatalf("deleteExtraFiles expected error when both missing")
	}
}

func TestCanonicalizeExtraTypeMapping(t *testing.T) {
	// temporarily write config file
	tmp := t.TempDir()
	old := TrailarrRoot
	TrailarrRoot = tmp
	oldCfg := ConfigPath
	ConfigPath = TrailarrRoot + "/config/config.yml"
	_ = os.MkdirAll(filepath.Dir(ConfigPath), 0o755)
	// write config with mapping
	cfg := []byte("canonicalizeExtraType:\n  mapping:\n    OldType: NewType\n")
	WriteConfig(t, cfg)
	defer func() {
		TrailarrRoot = old
		ConfigPath = oldCfg
	}()
	got := canonicalizeExtraType("OldType")
	if got != "NewType" {
		t.Fatalf("canonicalizeExtraType did not map: %s", got)
	}
}

func TestGetExtrasForMediaFallback(t *testing.T) {
	// exercise GetExtrasForMedia fallback logic by using BoltClient-backed store shim
	// We'll use AddOrUpdateExtra and GetExtrasForMedia that interact with the store client in tests
	ctx := context.Background()
	// ensure extras key empty
	_ = GetStoreClient().Del(ctx, ExtrasStoreKey)
	// add global entry
	entry := ExtrasEntry{MediaType: MediaTypeMovie, MediaId: 42, YoutubeId: "y42", ExtraTitle: "E", ExtraType: "T", Status: "downloaded"}
	_ = AddOrUpdateExtra(ctx, entry)
	res, err := GetExtrasForMedia(ctx, MediaTypeMovie, 42)
	if err != nil {
		t.Fatalf("GetExtrasForMedia failed: %v", err)
	}
	if len(res) == 0 {
		t.Fatalf("GetExtrasForMedia returned empty fallback: %v", res)
	}

}
