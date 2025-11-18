package internal

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestChooseFfmpegAssetPreference(t *testing.T) {
	assets := []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	}{
		{Name: "ffmpeg-2025-10-01-linux64-gpl-static.zip", BrowserDownloadURL: "a"},
		{Name: "ffmpeg-2025-10-01-linux64-gpl.zip", BrowserDownloadURL: "b"},
		{Name: "ffmpeg-2025-10-01-windows.exe", BrowserDownloadURL: "c"},
	}
	u := chooseFfmpegAsset(assets)
	if u != "a" {
		t.Fatalf("expected static asset 'a' preference, got %s", u)
	}
}

func TestChooseFfmpegAsset_ChooseBtbNShared(t *testing.T) {
	assets := []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	}{
		{Name: "ffmpeg-master-latest-linux64-lgpl-shared.tar.xz", BrowserDownloadURL: "shared_linux"},
		{Name: "ffmpeg-master-latest-win64-gpl-shared.zip", BrowserDownloadURL: "shared_win"},
		{Name: "ffmpeg-2025-10-01-linux64-gpl-static.tar.xz", BrowserDownloadURL: "static_linux"},
	}
	// ensure runtime.GOOS != windows for this test
	if runtime.GOOS == "windows" {
		t.Skip("linux-specific selection test")
	}
	u := chooseFfmpegAsset(assets)
	if u != "shared_linux" {
		t.Fatalf("expected shared Linux BtbN asset preference, got %s", u)
	}
}

// TestExtractAndInstall_XZStream tests that a single-file xz compressed binary
// is correctly detected, decompressed, and installed as an executable on disk.
func TestExtractAndInstall_XZStream(t *testing.T) {
	if _, err := exec.LookPath("xz"); err != nil {
		t.Skip("xz not found; skipping test")
	}
	// create a small executable file
	td := t.TempDir()
	src := filepath.Join(td, "ffmpeg")
	if err := os.WriteFile(src, []byte("#!/bin/sh\necho hello\n"), 0755); err != nil {
		t.Fatalf("write src: %v", err)
	}
	// compress using xz to create single-file compressed stream
	var out bytes.Buffer
	cmd := exec.Command("xz", "-c", src)
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("xz compress failed: %v", err)
	}
	asset := filepath.Join(td, "ffmpeg.xz")
	if err := os.WriteFile(asset, out.Bytes(), 0644); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	dest := filepath.Join(td, "installed")
	if err := extractAndInstall(asset, dest); err != nil {
		t.Fatalf("extractAndInstall failed: %v", err)
	}
	st, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	if st.Mode()&0111 == 0 {
		t.Fatalf("expected executable permission on installed file")
	}
}

// TestExtractAndInstall_TarXz ensures a tar.xz containing ffmpeg is extracted
// correctly and installed.
func TestExtractAndInstall_TarXz(t *testing.T) {
	// need tar to create a tar.xz archive
	if _, err := exec.LookPath("tar"); err != nil {
		t.Skip("tar not found; skipping test")
	}
	if _, err := exec.LookPath("xz"); err != nil {
		t.Skip("xz not found; skipping test")
	}
	td := t.TempDir()
	srcDir := filepath.Join(td, "srcdir")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := filepath.Join(srcDir, "ffmpeg")
	if err := os.WriteFile(src, []byte("#!/bin/sh\necho hi\n"), 0755); err != nil {
		t.Fatalf("write src: %v", err)
	}
	asset := filepath.Join(td, "ffmpeg.tar.xz")
	cmd := exec.Command("tar", "-C", srcDir, "-cJf", asset, "ffmpeg")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tar create failed: %v (%s)", err, string(out))
	}
	dest := filepath.Join(td, "installed")
	if err := extractAndInstall(asset, dest); err != nil {
		t.Fatalf("extractAndInstall failed: %v", err)
	}
	st, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	if st.Mode()&0111 == 0 {
		t.Fatalf("expected executable permission on installed file")
	}
}

// TestExtractAndInstall_TarXz_Shared ensures a tar.xz with ffmpeg and shared
// libraries is installed and the wrapper + libs are created.
func TestExtractAndInstall_TarXz_Shared(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shared tar.xz test only for unix-like")
	}
	if _, err := exec.LookPath("tar"); err != nil {
		t.Skip("tar not found; skipping test")
	}
	if _, err := exec.LookPath("xz"); err != nil {
		t.Skip("xz not found; skipping test")
	}
	td := t.TempDir()
	srcDir := filepath.Join(td, "srcdir")
	if err := os.MkdirAll(filepath.Join(srcDir, "bin"), 0755); err != nil {
		t.Fatalf("mkdir srcdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "lib"), 0755); err != nil {
		t.Fatalf("mkdir lib: %v", err)
	}
	src := filepath.Join(srcDir, "bin", "ffmpeg")
	if err := os.WriteFile(src, []byte("#!/bin/sh\necho hi\n"), 0755); err != nil {
		t.Fatalf("write src: %v", err)
	}
	lib := filepath.Join(srcDir, "lib", "libavdevice.so.62")
	if err := os.WriteFile(lib, []byte("dummy"), 0644); err != nil {
		t.Fatalf("write lib: %v", err)
	}
	asset := filepath.Join(td, "ffmpeg.tar.xz")
	cmd := exec.Command("tar", "-C", srcDir, "-cJf", asset, "bin", "lib")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tar create failed: %v (%s)", err, string(out))
	}
	dest := filepath.Join(td, "installed")
	if err := extractAndInstall(asset, dest); err != nil {
		t.Fatalf("extractAndInstall failed: %v", err)
	}
	// Check wrapper and real binary
	real := dest + ".real"
	if _, err := os.Stat(real); err != nil {
		t.Fatalf("real binary missing: %v", err)
	}
	// wrapper should be a shell script with LD_LIBRARY_PATH
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read wrapper: %v", err)
	}
	if !strings.Contains(string(data), "LD_LIBRARY_PATH=") {
		t.Fatalf("wrapper does not set LD_LIBRARY_PATH")
	}
	// libs should be copied into TrailarrRoot/lib
	libDir := filepath.Join(TrailarrRoot, "lib")
	if _, err := os.Stat(filepath.Join(libDir, "libavdevice.so.62")); err != nil {
		t.Fatalf("lib not installed: %v", err)
	}
}
