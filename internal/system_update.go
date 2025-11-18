package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// handleYtdlpUpdate attempts to download the latest yt-dlp release asset and replace
// the current binary in-place. This requires write permissions to the binary path.
func handleYtdlpUpdate(c *gin.Context) {
	TrailarrLog(INFO, "SystemUpdate", "Request to update yt-dlp via API")
	err := updateYtdlp()
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(c, http.StatusOK, gin.H{"success": true})
}

// handleFfmpegUpdate attempts to download and install an ffmpeg binary
// at TrailarrRoot/bin/ffmpeg. Implementation is best-effort; the ffmpeg
// official repo may not provide prebuilt binaries for all platforms.
func handleFfmpegUpdate(c *gin.Context) {
	TrailarrLog(INFO, "SystemUpdate", "Request to update ffmpeg via API")
	err := updateFfmpeg()
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(c, http.StatusOK, gin.H{"success": true})
}

// updateFfmpeg downloads a ffmpeg asset (best-effort) and installs it under TrailarrRoot/bin.
func updateFfmpeg() error {
	// Read configured timeout (may be increased for slow Docker hosts)
	timeout, err := GetFfmpegDownloadTimeout()
	if err != nil {
		TrailarrLog(WARN, "SystemUpdate", "Failed to parse ffmpegDownloadTimeout; falling back to 10m: %v", err)
		timeout = 10 * time.Minute
	}
	client := &http.Client{Timeout: timeout}
	// Prefer BtbN binary builds, which publish prebuilt static assets per platform
	url := "https://api.github.com/repos/BtbN/FFmpeg-Builds/releases/latest"
	req, _ := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	req.Header.Set("User-Agent", "trailarr")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch latest ffmpeg release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("no release found for ffmpeg on GitHub (404). ffmpeg often doesn't publish prebuilt binaries on GitHub; consider using your OS package manager or a supported prebuilt provider")
		}
		return fmt.Errorf("unexpected response from github: %s", resp.Status)
	}
	var payload struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("failed to decode ffmpeg release metadata: %w", err)
	}
	assetURL := chooseFfmpegAsset(payload.Assets)
	// If possible, prefer exact BtbN 'latest' shared assets for Linux/Windows
	if assetURL == "" {
		// If choose failed to find an asset in the release, attempt a direct
		// known url for the BtbN 'shared' builds and verify via a HEAD request.
		if runtime.GOOS == "linux" {
			mustURL := "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-linux64-lgpl-shared.tar.xz"
			headReq, _ := http.NewRequestWithContext(context.Background(), "HEAD", mustURL, nil)
			headReq.Header.Set("User-Agent", "trailarr")
			if r, err := client.Do(headReq); err == nil && r != nil && r.StatusCode == http.StatusOK {
				assetURL = mustURL
			}
		}
		if runtime.GOOS == "windows" {
			mustURL := "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-win64-gpl-shared.zip"
			headReq, _ := http.NewRequestWithContext(context.Background(), "HEAD", mustURL, nil)
			headReq.Header.Set("User-Agent", "trailarr")
			if r, err := client.Do(headReq); err == nil && r != nil && r.StatusCode == http.StatusOK {
				assetURL = mustURL
			}
		}
	}
	if assetURL == "" {
		return errors.New("no suitable ffmpeg asset found for this platform")
	}
	tmpFile, err := os.CreateTemp("", "ffmpeg-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() { tmpFile.Close(); _ = os.Remove(tmpFile.Name()) }()

	// Download asset with configured timeout and retries
	if err := downloadAssetToFile(assetURL, tmpFile, timeout); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		_ = tmpFile.Chmod(0755)
	}

	// Use configured FfmpegPath, defaulting to TrailarrRoot/bin/ffmpeg
	path := FfmpegPath
	if path == "" {
		path = filepath.Join(TrailarrRoot, "bin", FfmpegCmd)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		destDir := filepath.Join(TrailarrRoot, "bin")
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return fmt.Errorf("ffmpeg not at configured path and unable to create bin dir: %w", err)
		}
		dest := filepath.Join(destDir, FfmpegCmd)
		// If the asset is an archive, extract ffmpeg binary from it. If
		// it's a raw binary, install directly. We detect archives by file
		// suffixes and support .tar.xz, .tar.gz and .zip.
		if err := extractAndInstall(tmpFile.Name(), dest); err != nil {
			return err
		}
		FfmpegPath = dest
		return nil
	}
	// Replace the existing binary (backup & install)
	backup := path + ".bak"
	_ = os.Remove(backup)
	if err := os.Rename(path, backup); err != nil {
		return fmt.Errorf("failed to backup existing ffmpeg binary: %w", err)
	}
	// For archives, extract; otherwise, copy raw binary into path
	if err := extractAndInstall(tmpFile.Name(), path); err != nil {
		_ = os.Rename(backup, path)
		return fmt.Errorf("failed to install ffmpeg: %w", err)
	}
	// Validate that the installed binary runs correctly. If a wrapper was
	// created, the actual binary will be dest.real; otherwise, it's dest.
	if runtime.GOOS != "windows" {
		realBin := path
		if _, err := os.Stat(path + ".real"); err == nil {
			realBin = path + ".real"
		}
		// If we installed libs into TrailarrRoot/lib, ensure the binary can run
		// by invoking it with LD_LIBRARY_PATH set to that directory.
		cmd := exec.Command(realBin, "-version")
		env := os.Environ()
		libDir := filepath.Join(TrailarrRoot, "lib")
		if fi, err := os.Stat(libDir); err == nil && fi.IsDir() {
			env = append(env, "LD_LIBRARY_PATH="+libDir+":"+os.Getenv("LD_LIBRARY_PATH"))
		}
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			_ = os.Rename(backup, path)
			return fmt.Errorf("installed ffmpeg failed to run: %v: %s", err, strings.TrimSpace(string(out)))
		}
	}
	_ = os.Remove(backup)
	return nil
}

// chooseFfmpegAsset chooses a suitable release asset for current OS/ARCH for ffmpeg
func chooseFfmpegAsset(assets []struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}) string {
	// Choose by filename heuristics: prefer names containing 'ffmpeg' and matching platform
	wantedExt := ""
	if runtime.GOOS == "windows" {
		wantedExt = ".exe"
	}
	// Prefer the BtbN 'shared' builds for Linux and Windows explicitly
	// if available (these contain shared libraries that will be installed
	// next to the binary). If those aren't available, fall back to the
	// earlier heuristics (static/gpl) to find a usable asset.
	// The user's specific requested assets are:
	// - linux: ffmpeg-master-latest-linux64-lgpl-shared.tar.xz
	// - windows: ffmpeg-master-latest-win64-gpl-shared.zip
	// We'll match those patterns first.
	// First pass: look for shared builds matching our platform
	for _, a := range assets {
		n := strings.ToLower(a.Name)
		if runtime.GOOS == "linux" {
			if strings.Contains(n, "linux64") && strings.Contains(n, "lgpl") && strings.Contains(n, "shared") && strings.HasSuffix(n, ".tar.xz") {
				return a.BrowserDownloadURL
			}
		}
		if runtime.GOOS == "windows" {
			if strings.Contains(n, "win64") && strings.Contains(n, "gpl") && strings.Contains(n, "shared") && strings.HasSuffix(n, ".zip") {
				return a.BrowserDownloadURL
			}
		}
	}

	// If we didn't find a platform-specific 'shared' build, fall back to
	// the previous heuristics for static or gpl builds.
	for _, a := range assets {
		n := strings.ToLower(a.Name)
		if strings.Contains(n, "ffmpeg") {
			// Ensure candidate matches platform (non .exe on linux/mac, .exe on windows)
			if wantedExt != "" && !strings.HasSuffix(n, wantedExt) {
				continue
			}
			if wantedExt == "" && strings.HasSuffix(n, ".exe") {
				continue
			}
			// Prefer assets that indicate static linking or gpl + linux
			if strings.Contains(n, "static") && (strings.Contains(n, "linux") || strings.Contains(n, "linux64") || strings.Contains(n, "linux64")) && strings.Contains(n, "x86") {
				return a.BrowserDownloadURL
			}
			if strings.Contains(n, "gpl") && (strings.Contains(n, "linux") || strings.Contains(n, "linux64") || strings.Contains(n, "linux64")) {
				return a.BrowserDownloadURL
			}
			// Otherwise prefer assets with 'linux' or 'x86' in name, if not static/gpl
			if strings.Contains(n, "linux") || strings.Contains(n, "x86") || strings.Contains(n, "x64") || strings.Contains(n, "amd64") {
				return a.BrowserDownloadURL
			}
		}
	}
	// fallback to first asset containing ffmpeg
	for _, a := range assets {
		if strings.Contains(strings.ToLower(a.Name), "ffmpeg") {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

// updateYtdlp performs the download and installation of the latest yt-dlp release.
// It returns an error if anything fails.
func updateYtdlp() error {
	// Fetch latest release metadata
	timeout, err := GetYtDlpDownloadTimeout()
	if err != nil {
		TrailarrLog(WARN, "SystemUpdate", "Failed to parse ytdlpDownloadTimeout; falling back to 5m: %v", err)
		timeout = 5 * time.Minute
	}
	client := &http.Client{Timeout: timeout}
	url := "https://api.github.com/repos/yt-dlp/yt-dlp/releases/latest"
	req, _ := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	req.Header.Set("User-Agent", "trailarr")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch latest release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("no release found for yt-dlp on GitHub (404). Please check GitHub repo and network or update manually")
		}
		return fmt.Errorf("unexpected response from github: %s", resp.Status)
	}

	var payload struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("failed to decode release metadata: %w", err)
	}

	// choose asset
	assetURL := chooseYtDlpAsset(payload.Assets)
	if assetURL == "" {
		return errors.New("no suitable yt-dlp asset found for this platform")
	}

	// Download asset to temp file
	tmpFile, err := os.CreateTemp("", "yt-dlp-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
	}()

	// Download asset with configured timeout and retries
	if err := downloadAssetToFile(assetURL, tmpFile, timeout); err != nil {
		TrailarrLog(WARN, "SystemUpdate", "download attempts failed for %s: %v", assetURL, err)
		return err
	}

	// Ensure executable bit
	if runtime.GOOS != "windows" {
		if err := tmpFile.Chmod(0755); err != nil {
			// best-effort
			_ = tmpFile.Chmod(0744)
		}
	}

	// Find existing yt-dlp path at our configured YtDlpPath
	path := YtDlpPath
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Not found at configured path: ensure TrailarrRoot/bin exists then install
		destDir := filepath.Join(TrailarrRoot, "bin")
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return fmt.Errorf("yt-dlp not at configured path and unable to create bin dir: %w", err)
		}
		dest := filepath.Join(destDir, YtDlpCmd)
		// Update YtDlpPath to the new dest if install successful
		if err := installBinary(tmpFile.Name(), dest); err != nil {
			return err
		}
		YtDlpPath = dest
		return nil
	}

	// Replace the existing binary (make a backup)
	backup := path + ".bak"
	_ = os.Remove(backup)
	if err := os.Rename(path, backup); err != nil {
		// If rename fails due to permissions, fail fast
		return fmt.Errorf("failed to backup existing yt-dlp binary: %w", err)
	}
	if err := installBinary(tmpFile.Name(), path); err != nil {
		// attempt to restore backup
		_ = os.Rename(backup, path)
		return fmt.Errorf("failed to install yt-dlp: %w", err)
	}
	// remove backup on success
	_ = os.Remove(backup)
	return nil
}

// chooseYtDlpAsset chooses a suitable release asset for current OS/ARCH.
func chooseYtDlpAsset(assets []struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}) string {
	// prefer exact names, try to find asset matching interpreter OS
	wanted := YtDlpCmd
	if runtime.GOOS == "windows" {
		wanted = YtDlpCmd + ".exe"
	}
	for _, a := range assets {
		if a.Name == wanted {
			return a.BrowserDownloadURL
		}
	}
	// fallback: find asset that contains OS name or 'yt-dlp' without extension
	for _, a := range assets {
		lname := strings.ToLower(a.Name)
		if strings.Contains(lname, "yt-dlp") && (runtime.GOOS == "windows" && strings.HasSuffix(lname, ".exe") || runtime.GOOS != "windows" && !strings.HasSuffix(lname, ".exe")) {
			return a.BrowserDownloadURL
		}
	}
	// last resort: pick the first asset named containing yt-dlp
	for _, a := range assets {
		if strings.Contains(strings.ToLower(a.Name), "yt-dlp") {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

// installBinary copies tmpFile to dest and sets executable bit.
func installBinary(tmpFile, dest string) error {
	input, err := os.Open(tmpFile)
	if err != nil {
		return err
	}
	defer input.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, input); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		if err := out.Chmod(0755); err != nil {
			// best effort
			_ = out.Chmod(0744)
		}
	}
	return nil
}

// copyFile copies a file from src to dest with specified permission bits.
func copyFile(src, dest string, perm os.FileMode) error {
	input, err := os.Open(src)
	if err != nil {
		return err
	}
	defer input.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, input); err != nil {
		return err
	}
	return nil
}

// installFFmpegFromExtracted copies the extracted ffmpeg binary and any
// supporting shared libraries from the tmpDir into TrailarrRoot/bin and
// TrailarrRoot/lib. For Linux, if shared libraries are present, we create
// a small wrapper script at dest to set LD_LIBRARY_PATH to the installed
// lib directory before executing the actual binary.
func installFFmpegFromExtracted(tmpDir, ffPath, dest string) error {
	// find libraries in tmpDir (lib or lib64)
	var libFiles []string
	filepath.Walk(tmpDir, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi == nil {
			return nil
		}
		if fi.IsDir() {
			return nil
		}
		bn := filepath.Base(p)
		if strings.HasPrefix(bn, "lib") && strings.Contains(bn, ".so") || strings.HasSuffix(bn, ".so") || strings.HasSuffix(bn, ".so.1") {
			libFiles = append(libFiles, p)
		}
		return nil
	})

	destDir := filepath.Dir(dest)
	if runtime.GOOS == "windows" {
		// copy the executable into dest and copy any DLLs to the dest bin folder
		if err := installBinary(ffPath, dest); err != nil {
			return err
		}
		for _, lf := range libFiles {
			dst := filepath.Join(destDir, filepath.Base(lf))
			if err := copyFile(lf, dst, 0644); err != nil {
				return err
			}
		}
		return nil
	}

	// Unix-like: install binary into dest.bin and copy libs into libDir
	libDir := filepath.Join(TrailarrRoot, "lib")
	if len(libFiles) > 0 {
		if err := os.MkdirAll(libDir, 0755); err != nil {
			return fmt.Errorf("unable to create lib dir: %w", err)
		}
		for _, lf := range libFiles {
			dst := filepath.Join(libDir, filepath.Base(lf))
			if err := copyFile(lf, dst, 0644); err != nil {
				return err
			}
		}
	}
	// install binary into a non-wrapper path; we'll create wrapper at dest
	realBin := dest + ".real"
	if err := installBinary(ffPath, realBin); err != nil {
		return err
	}
	// wrapper script that sets LD_LIBRARY_PATH when executing binary
	wrapper := fmt.Sprintf("#!/bin/sh\nLD_LIBRARY_PATH=%s:${LD_LIBRARY_PATH}\nexport LD_LIBRARY_PATH\nexec \"%s\" \"$@\"\n", libDir, realBin)
	if err := os.WriteFile(dest, []byte(wrapper), 0755); err != nil {
		return err
	}
	return nil
}

// extractAndInstall handles both raw binaries and archived distributions.
// If the tmpFile is an archive (.tar.xz, .tar.gz, .zip), it extracts the
// `ffmpeg` binary from the archive and writes it to dest. Otherwise, it
// copies the file as-is (for raw binaries).
func extractAndInstall(tmpFile, dest string) error {
	// Detect content by magic numbers because temp filenames don't retain
	// original extension. We support tar.xz, tar.gz (tgz), zip and raw
	// compressed single-file streams (.xz/.gz).
	f, err := os.Open(tmpFile)
	if err != nil {
		return err
	}
	defer f.Close()
	hdr := make([]byte, 6)
	n, _ := f.Read(hdr)
	if n > 0 {
		_, _ = f.Seek(0, io.SeekStart)
	}
	isXZ := n >= 6 && hdr[0] == 0xFD && hdr[1] == 0x37 && hdr[2] == 0x7A && hdr[3] == 0x58 && hdr[4] == 0x5A && hdr[5] == 0x00
	isGzip := n >= 2 && hdr[0] == 0x1F && hdr[1] == 0x8B
	isZip := n >= 4 && hdr[0] == 0x50 && hdr[1] == 0x4B && hdr[2] == 0x03 && hdr[3] == 0x04

	if isXZ || isGzip {
		tmpDir, err := os.MkdirTemp("", "ffmpeg-extract-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)
		// Try extract as tar first (for tar.xz or tar.gz). If tar fails, then
		// fallback to stream decompression of the single file into a temp file.
		var tarArgs []string
		if isXZ {
			tarArgs = []string{"-xJf", tmpFile, "-C", tmpDir}
		} else {
			tarArgs = []string{"-xzf", tmpFile, "-C", tmpDir}
		}
		tarCmd := exec.Command("tar", tarArgs...)
		if _, err := tarCmd.CombinedOutput(); err == nil {
			var ffPath string
			filepath.Walk(tmpDir, func(p string, fi os.FileInfo, err error) error {
				if err != nil || fi == nil {
					return nil
				}
				if fi.IsDir() {
					return nil
				}
				base := filepath.Base(p)
				if base == "ffmpeg" || base == "ffmpeg.exe" {
					ffPath = p
					return io.EOF
				}
				return nil
			})
			if ffPath != "" {
				return installFFmpegFromExtracted(tmpDir, ffPath, dest)
			}
			// tar succeeded but ffmpeg not found in extracted tree; fall back to
			// stream decompression for single-file archives.
			// (This can happen when the asset is a single-file xz/gzip stream.)
			// noop so we reach the decomp fallback below while keeping the
			// tmpDir for cleanup.
		} else {
			// tar extraction failed; attempt to decompress stream
			var decompCmd *exec.Cmd
			if isXZ {
				decompCmd = exec.Command("xz", "-dc", tmpFile)
			} else {
				decompCmd = exec.Command("gzip", "-dc", tmpFile)
			}
			outTmp, err := os.CreateTemp("", "ffmpeg-decompressed-*")
			if err != nil {
				return err
			}
			defer func() { outTmp.Close(); _ = os.Remove(outTmp.Name()) }()
			decompCmd.Stdout = outTmp
			if out, err := decompCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to decompress archive stream: %v: %s", err, string(out))
			}
			if runtime.GOOS != "windows" {
				_ = outTmp.Chmod(0755)
			}
			return installBinary(outTmp.Name(), dest)
		}
	}
	if isZip {
		tmpDir, err := os.MkdirTemp("", "ffmpeg-extract-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)
		// unzip -qq
		cmd := exec.Command("unzip", "-qq", tmpFile, "-d", tmpDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to unzip archive: %v: %s", err, string(out))
		}
		var ffPath string
		filepath.Walk(tmpDir, func(p string, fi os.FileInfo, err error) error {
			if err != nil || fi == nil {
				return nil
			}
			if fi.IsDir() {
				return nil
			}
			base := filepath.Base(p)
			if base == "ffmpeg" || base == "ffmpeg.exe" {
				ffPath = p
				return io.EOF
			}
			return nil
		})
		if ffPath == "" {
			return fmt.Errorf("ffmpeg binary not found in zip archive")
		}
		return installFFmpegFromExtracted(tmpDir, ffPath, dest)
	}
	// Not an archive: treat as raw binary
	return installBinary(tmpFile, dest)
}

// downloadAssetToFile downloads a URL into the provided destination file. It
// performs a number of retries with exponential backoff and uses a custom
// timeout for the transfer to avoid small default timeouts failing large
// assets (e.g. ffmpeg tarballs). The caller must ensure dest is an open file
// for writing and will receive the file pointer at the end of the download.
func downloadAssetToFile(assetURL string, dest *os.File, timeout time.Duration) error {
	const attempts = 3
	for i := 0; i < attempts; i++ {
		client := &http.Client{Timeout: timeout}
		req, _ := http.NewRequestWithContext(context.Background(), "GET", assetURL, nil)
		req.Header.Set("User-Agent", "trailarr")
		resp, err := client.Do(req)
		if err != nil {
			TrailarrLog(WARN, "SystemUpdate", "download attempt %d failed for %s: %v", i+1, assetURL, err)
			time.Sleep(time.Duration(1<<i) * time.Second)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			// Non-recoverable responses like 404/403 should bail out quickly
			if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
				return fmt.Errorf("unexpected response from github asset: %s", resp.Status)
			}
			TrailarrLog(WARN, "SystemUpdate", "download attempt %d got non-OK status %s for %s", i+1, resp.Status, assetURL)
			time.Sleep(time.Duration(1<<i) * time.Second)
			continue
		}

		// Ensure we're writing at the start of the file and truncated, in
		// case of retries where the temp file may contain partial data.
		if _, err := dest.Seek(0, io.SeekStart); err != nil {
			resp.Body.Close()
			return fmt.Errorf("failed to seek temp file: %w", err)
		}
		if err := dest.Truncate(0); err != nil {
			resp.Body.Close()
			return fmt.Errorf("failed to truncate temp file: %w", err)
		}

		written, err := io.Copy(dest, resp.Body)
		if err != nil {
			TrailarrLog(WARN, "SystemUpdate", "write attempt %d failed for %s: %v", i+1, assetURL, err)
			resp.Body.Close()
			time.Sleep(time.Duration(1<<i) * time.Second)
			continue
		}
		TrailarrLog(INFO, "SystemUpdate", "download attempt %d succeeded for %s (bytes=%d)", i+1, assetURL, written)
		// success
		resp.Body.Close()
		return nil
	}
	return fmt.Errorf("failed to download asset after %d attempts", attempts)
}
