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
	timeout := getFfmpegTimeout()
	assetURL, err := fetchFfmpegAssetURL(timeout)
	if err != nil {
		return err
	}

	tmpFile, err := downloadFfmpegAsset(assetURL, timeout)
	if err != nil {
		return err
	}
	defer func() { tmpFile.Close(); _ = os.Remove(tmpFile.Name()) }()

	path := getFfmpegInstallPath()
	if err := installOrReplaceFfmpeg(tmpFile.Name(), path); err != nil {
		return err
	}

	if runtime.GOOS != "windows" {
		return validateFfmpegInstall(path)
	}
	return nil
}

// getFfmpegTimeout retrieves the configured ffmpeg download timeout.
func getFfmpegTimeout() time.Duration {
	timeout, err := GetFfmpegDownloadTimeout()
	if err != nil {
		TrailarrLog(WARN, "SystemUpdate", "Failed to parse ffmpegDownloadTimeout; falling back to 10m: %v", err)
		timeout = 10 * time.Minute
	}
	return timeout
}

// fetchFfmpegAssetURL retrieves the latest ffmpeg release and returns a suitable asset URL.
func fetchFfmpegAssetURL(timeout time.Duration) (string, error) {
	client := &http.Client{Timeout: timeout}
	url := "https://api.github.com/repos/BtbN/FFmpeg-Builds/releases/latest"
	req, _ := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	req.Header.Set(HeaderUserAgent, defaultUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch latest ffmpeg release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("no release found for ffmpeg on GitHub (404). ffmpeg often doesn't publish prebuilt binaries on GitHub; consider using your OS package manager or a supported prebuilt provider")
		}
		return "", fmt.Errorf("unexpected response from github: %s", resp.Status)
	}

	var payload struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("failed to decode ffmpeg release metadata: %w", err)
	}

	assetURL := chooseFfmpegAsset(payload.Assets)
	if assetURL == "" {
		assetURL = tryFallbackFfmpegAsset(client)
	}
	if assetURL == "" {
		return "", errors.New("no suitable ffmpeg asset found for this platform")
	}
	return assetURL, nil
}

// tryFallbackFfmpegAsset attempts known BtbN 'latest' URLs as a fallback.
func tryFallbackFfmpegAsset(client *http.Client) string {
	if runtime.GOOS == "linux" {
		if checkAssetExists(client, "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-linux64-lgpl-shared.tar.xz") {
			return "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-linux64-lgpl-shared.tar.xz"
		}
	}
	if runtime.GOOS == "windows" {
		if checkAssetExists(client, "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-win64-gpl-shared.zip") {
			return "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-win64-gpl-shared.zip"
		}
	}
	return ""
}

// checkAssetExists verifies if a URL returns HTTP 200 via HEAD request.
func checkAssetExists(client *http.Client, url string) bool {
	headReq, _ := http.NewRequestWithContext(context.Background(), "HEAD", url, nil)
	headReq.Header.Set(HeaderUserAgent, defaultUserAgent)
	if r, err := client.Do(headReq); err == nil && r != nil && r.StatusCode == http.StatusOK {
		return true
	}
	return false
}

// downloadFfmpegAsset downloads the ffmpeg asset to a temporary file.
func downloadFfmpegAsset(assetURL string, timeout time.Duration) (*os.File, error) {
	tmpFile, err := os.CreateTemp("", "ffmpeg-update-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	if err := downloadAssetToFile(assetURL, tmpFile, timeout); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return nil, err
	}
	if runtime.GOOS != "windows" {
		_ = tmpFile.Chmod(0755)
	}
	return tmpFile, nil
}

// getFfmpegInstallPath returns the configured ffmpeg path or default.
func getFfmpegInstallPath() string {
	if FfmpegPath != "" {
		return FfmpegPath
	}
	return filepath.Join(TrailarrRoot, "bin", FfmpegCmd)
}

// installOrReplaceFfmpeg installs ffmpeg either fresh or replacing an existing binary.
func installOrReplaceFfmpeg(tmpFile, path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return installFfmpegFresh(tmpFile, path)
	}
	return replaceFfmpegBinary(tmpFile, path)
}

// installFfmpegFresh installs ffmpeg to a new location.
func installFfmpegFresh(tmpFile, path string) error {
	destDir := filepath.Join(TrailarrRoot, "bin")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("ffmpeg not at configured path and unable to create bin dir: %w", err)
	}
	if err := extractAndInstall(tmpFile, path); err != nil {
		return err
	}
	FfmpegPath = path
	return nil
}

// replaceFfmpegBinary replaces an existing ffmpeg binary with a backup.
func replaceFfmpegBinary(tmpFile, path string) error {
	backup := path + ".bak"
	_ = os.Remove(backup)
	if err := os.Rename(path, backup); err != nil {
		return fmt.Errorf("failed to backup existing ffmpeg binary: %w", err)
	}
	if err := extractAndInstall(tmpFile, path); err != nil {
		_ = os.Rename(backup, path)
		return fmt.Errorf("failed to install ffmpeg: %w", err)
	}
	_ = os.Remove(backup)
	return nil
}

// validateFfmpegInstall verifies the installed ffmpeg binary runs correctly.
func validateFfmpegInstall(path string) error {
	realBin := path
	if _, err := os.Stat(path + ".real"); err == nil {
		realBin = path + ".real"
	}

	cmd := exec.Command(realBin, "-version")
	env := os.Environ()
	libDir := filepath.Join(TrailarrRoot, "lib")
	if fi, err := os.Stat(libDir); err == nil && fi.IsDir() {
		env = append(env, "LD_LIBRARY_PATH="+libDir+":"+os.Getenv("LD_LIBRARY_PATH"))
	}
	cmd.Env = env

	if out, err := cmd.CombinedOutput(); err != nil {
		backup := path + ".bak"
		_ = os.Rename(backup, path)
		return fmt.Errorf("installed ffmpeg failed to run: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// isSharedBuildMatch checks if an asset is a shared build matching the current platform.
func isSharedBuildMatch(name string) bool {
	n := strings.ToLower(name)
	if runtime.GOOS == "linux" {
		return strings.Contains(n, "linux64") && strings.Contains(n, "lgpl") &&
			strings.Contains(n, "shared") && strings.HasSuffix(n, ".tar.xz")
	}
	if runtime.GOOS == "windows" {
		return strings.Contains(n, "win64") && strings.Contains(n, "gpl") &&
			strings.Contains(n, "shared") && strings.HasSuffix(n, ".zip")
	}
	return false
}

// matchesPlatformExt checks if the asset name matches the expected extension for the platform.
func matchesPlatformExt(name string, wantedExt string) bool {
	n := strings.ToLower(name)
	if wantedExt != "" && !strings.HasSuffix(n, wantedExt) {
		return false
	}
	if wantedExt == "" && strings.HasSuffix(n, ".exe") {
		return false
	}
	return true
}

// isStaticOrGplBuild checks if the asset is a static or gpl build for Linux.
func isStaticOrGplBuild(name string) bool {
	n := strings.ToLower(name)
	linuxVariants := []string{"linux", "linux64"}

	if strings.Contains(n, "static") {
		for _, variant := range linuxVariants {
			if strings.Contains(n, variant) && strings.Contains(n, "x86") {
				return true
			}
		}
	}

	if strings.Contains(n, "gpl") {
		for _, variant := range linuxVariants {
			if strings.Contains(n, variant) {
				return true
			}
		}
	}
	return false
}

// isLinuxOrX86Build checks if the asset is a Linux or x86 build.
func isLinuxOrX86Build(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "linux") || strings.Contains(n, "x86") ||
		strings.Contains(n, "x64") || strings.Contains(n, "amd64")
}

// chooseFfmpegAsset chooses a suitable release asset for current OS/ARCH for ffmpeg
func chooseFfmpegAsset(assets []struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}) string {
	// First pass: look for shared builds matching our platform
	if url := findAssetByMatch(assets, isSharedBuildMatch); url != "" {
		return url
	}

	// Second pass: fall back to static or gpl builds
	wantedExt := getWantedExt()
	if url := findCompatibleAsset(assets, wantedExt); url != "" {
		return url
	}

	// Fallback to first asset containing ffmpeg
	return findFirstFfmpegAsset(assets)
}

// getWantedExt returns the file extension for the current OS
func getWantedExt() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// findAssetByMatch finds the first asset matching the given condition
func findAssetByMatch(assets []struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}, match func(string) bool) string {
	for _, a := range assets {
		if match(a.Name) {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

// findCompatibleAsset finds an asset that matches platform requirements
func findCompatibleAsset(assets []struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}, wantedExt string) string {
	for _, a := range assets {
		if !isFfmpegAsset(a.Name) || !matchesPlatformExt(a.Name, wantedExt) {
			continue
		}
		if isStaticOrGplBuild(a.Name) || isLinuxOrX86Build(a.Name) {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

// findFirstFfmpegAsset returns the first asset containing ffmpeg
func findFirstFfmpegAsset(assets []struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}) string {
	for _, a := range assets {
		if isFfmpegAsset(a.Name) {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

// isFfmpegAsset checks if the asset name contains ffmpeg
func isFfmpegAsset(name string) bool {
	return strings.Contains(strings.ToLower(name), "ffmpeg")
}

// updateYtdlp performs the download and installation of the latest yt-dlp release.
// It returns an error if anything fails.
func updateYtdlp() error {
	timeout := getYtDlpTimeout()
	resp, err := fetchYtDlpRelease(timeout)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	payload, err := decodeYtDlpPayload(resp.Body)
	if err != nil {
		return err
	}

	assetURL := chooseYtDlpAsset(payload.Assets)
	if assetURL == "" {
		return errors.New("no suitable yt-dlp asset found for this platform")
	}

	tmpFile, err := os.CreateTemp("", "yt-dlp-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
	}()

	if err := downloadAssetToFile(assetURL, tmpFile, timeout); err != nil {
		TrailarrLog(WARN, "SystemUpdate", "download attempts failed for %s: %v", assetURL, err)
		return err
	}

	ensureExecutableBit(tmpFile)

	return installYtDlpBinary(tmpFile.Name())
}

// getYtDlpTimeout returns the configured timeout or default 5m
func getYtDlpTimeout() time.Duration {
	timeout, err := GetYtDlpDownloadTimeout()
	if err != nil {
		TrailarrLog(WARN, "SystemUpdate", "Failed to parse ytdlpDownloadTimeout; falling back to 5m: %v", err)
		return 5 * time.Minute
	}
	return timeout
}

// fetchYtDlpRelease fetches the latest yt-dlp release from GitHub
func fetchYtDlpRelease(timeout time.Duration) (*http.Response, error) {
	client := &http.Client{Timeout: timeout}
	url := "https://api.github.com/repos/yt-dlp/yt-dlp/releases/latest"
	req, _ := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	req.Header.Set(HeaderUserAgent, defaultUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest release: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("no release found for yt-dlp on GitHub (404). Please check GitHub repo and network or update manually")
		}
		return nil, fmt.Errorf("unexpected response from github: %s", resp.Status)
	}

	return resp, nil
}

// decodeYtDlpPayload decodes the release metadata from response body
func decodeYtDlpPayload(body io.ReadCloser) (*struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}, error) {
	var payload struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("failed to decode release metadata: %w", err)
	}
	return &payload, nil
}

// ensureExecutableBit sets executable permissions on non-Windows platforms
func ensureExecutableBit(f *os.File) {
	if runtime.GOOS != "windows" {
		if err := f.Chmod(0755); err != nil {
			_ = f.Chmod(0744)
		}
	}
}

// installYtDlpBinary installs the downloaded binary to the correct location
func installYtDlpBinary(tmpPath string) error {
	path := YtDlpPath
	if pathExists(path) {
		return replaceExistingBinary(tmpPath, path)
	}
	return installNewBinary(tmpPath)
}

// pathExists checks if a path exists
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// installNewBinary installs yt-dlp when no existing binary is found
func installNewBinary(tmpPath string) error {
	destDir := filepath.Join(TrailarrRoot, "bin")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("yt-dlp not at configured path and unable to create bin dir: %w", err)
	}
	dest := filepath.Join(destDir, YtDlpCmd)
	if err := installBinary(tmpPath, dest); err != nil {
		return err
	}
	YtDlpPath = dest
	return nil
}

// replaceExistingBinary replaces the existing yt-dlp binary with a backup
func replaceExistingBinary(tmpPath, path string) error {
	backup := path + ".bak"
	_ = os.Remove(backup)
	if err := os.Rename(path, backup); err != nil {
		return fmt.Errorf("failed to backup existing yt-dlp binary: %w", err)
	}
	if err := installBinary(tmpPath, path); err != nil {
		_ = os.Rename(backup, path)
		return fmt.Errorf("failed to install yt-dlp: %w", err)
	}
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

// findLibraryFiles searches tmpDir for shared library files.
func findLibraryFiles(tmpDir string) ([]string, error) {
	var libFiles []string
	err := filepath.Walk(tmpDir, func(p string, fi os.FileInfo, err error) error {
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
	return libFiles, err
}

// installFFmpegWindows handles FFmpeg installation on Windows.
func installFFmpegWindows(ffPath, dest string, libFiles []string) error {
	if err := installBinary(ffPath, dest); err != nil {
		return err
	}
	destDir := filepath.Dir(dest)
	for _, lf := range libFiles {
		dst := filepath.Join(destDir, filepath.Base(lf))
		if err := copyFile(lf, dst, 0644); err != nil {
			return err
		}
	}
	return nil
}

// copyLibrariesToDir copies library files to the specified directory.
func copyLibrariesToDir(libFiles []string, libDir string) error {
	if len(libFiles) == 0 {
		return nil
	}
	if err := os.MkdirAll(libDir, 0755); err != nil {
		return fmt.Errorf("unable to create lib dir: %w", err)
	}
	for _, lf := range libFiles {
		dst := filepath.Join(libDir, filepath.Base(lf))
		if err := copyFile(lf, dst, 0644); err != nil {
			return err
		}
	}
	return nil
}

// installFFmpegUnix handles FFmpeg installation on Unix-like systems.
func installFFmpegUnix(ffPath, dest string, libFiles []string) error {
	libDir := filepath.Join(TrailarrRoot, "lib")
	if err := copyLibrariesToDir(libFiles, libDir); err != nil {
		return err
	}
	realBin := dest + ".real"
	if err := installBinary(ffPath, realBin); err != nil {
		return err
	}
	wrapper := fmt.Sprintf("#!/bin/sh\nLD_LIBRARY_PATH=%s:${LD_LIBRARY_PATH}\nexport LD_LIBRARY_PATH\nexec \"%s\" \"$@\"\n", libDir, realBin)
	if err := os.WriteFile(dest, []byte(wrapper), 0755); err != nil {
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
	libFiles, err := findLibraryFiles(tmpDir)
	if err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		return installFFmpegWindows(ffPath, dest, libFiles)
	}

	return installFFmpegUnix(ffPath, dest, libFiles)
}

// fileTypeInfo detects the file type by magic numbers.
type fileTypeInfo struct {
	isXZ   bool
	isGzip bool
	isZip  bool
}

// detectFileType reads the first 6 bytes to identify archive format.
func detectFileType(tmpFile string) (fileTypeInfo, error) {
	f, err := os.Open(tmpFile)
	if err != nil {
		return fileTypeInfo{}, err
	}
	defer f.Close()
	hdr := make([]byte, 6)
	n, _ := f.Read(hdr)
	if n > 0 {
		_, _ = f.Seek(0, io.SeekStart)
	}
	return fileTypeInfo{
		isXZ:   n >= 6 && hdr[0] == 0xFD && hdr[1] == 0x37 && hdr[2] == 0x7A && hdr[3] == 0x58 && hdr[4] == 0x5A && hdr[5] == 0x00,
		isGzip: n >= 2 && hdr[0] == 0x1F && hdr[1] == 0x8B,
		isZip:  n >= 4 && hdr[0] == 0x50 && hdr[1] == 0x4B && hdr[2] == 0x03 && hdr[3] == 0x04,
	}, nil
}

// findFFmpegInDir walks tmpDir and returns the path to ffmpeg binary.
func findFFmpegInDir(tmpDir string) string {
	var ffPath string
	filepath.Walk(tmpDir, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi == nil || fi.IsDir() {
			return nil
		}
		base := filepath.Base(p)
		if base == "ffmpeg" || base == "ffmpeg.exe" {
			ffPath = p
			return io.EOF
		}
		return nil
	})
	return ffPath
}

// extractTarArchive attempts to extract a tar.xz or tar.gz archive.
// If successful and ffmpeg is found, it installs it. Otherwise returns false
// to indicate fallback to stream decompression is needed.
func extractTarArchive(tmpFile, dest string, isXZ bool) (bool, error) {
	tmpDir, err := os.MkdirTemp("", "ffmpeg-extract-*")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(tmpDir)

	var tarArgs []string
	if isXZ {
		tarArgs = []string{"-xJf", tmpFile, "-C", tmpDir}
	} else {
		tarArgs = []string{"-xzf", tmpFile, "-C", tmpDir}
	}
	tarCmd := exec.Command("tar", tarArgs...)
	if _, err := tarCmd.CombinedOutput(); err != nil {
		return false, nil // Signal fallback to stream decompression
	}

	ffPath := findFFmpegInDir(tmpDir)
	if ffPath != "" {
		return true, installFFmpegFromExtracted(tmpDir, ffPath, dest)
	}
	return false, nil // ffmpeg not found; fallback to stream decompression
}

// decompressStream decompresses a single-file xz/gzip stream and installs it.
func decompressStream(tmpFile, dest string, isXZ bool) error {
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

	// Write decompressed bytes to outTmp and capture stderr separately. Do not
	// use CombinedOutput here: it sets up a StdoutPipe, which fails with
	// "exec: Stdout already set" because Stdout is assigned above.
	decompCmd.Stdout = outTmp
	var stderr strings.Builder
	decompCmd.Stderr = &stderr
	if err := decompCmd.Run(); err != nil {
		return fmt.Errorf("failed to decompress archive stream: %v: %s", err, stderr.String())
	}
	if runtime.GOOS != "windows" {
		_ = outTmp.Chmod(0755)
	}
	return installBinary(outTmp.Name(), dest)
}

// extractTarOrStream tries tar extraction first, then falls back to stream decompression.
func extractTarOrStream(tmpFile, dest string, isXZ bool) error {
	ok, err := extractTarArchive(tmpFile, dest, isXZ)
	if ok || err != nil {
		return err
	}
	return decompressStream(tmpFile, dest, isXZ)
}

// extractZipArchive extracts a zip archive and installs ffmpeg.
func extractZipArchive(tmpFile, dest string) error {
	tmpDir, err := os.MkdirTemp("", "ffmpeg-extract-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command("unzip", "-qq", tmpFile, "-d", tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to unzip archive: %v: %s", err, string(out))
	}

	ffPath := findFFmpegInDir(tmpDir)
	if ffPath == "" {
		return fmt.Errorf("ffmpeg binary not found in zip archive")
	}
	return installFFmpegFromExtracted(tmpDir, ffPath, dest)
}

// extractAndInstall handles both raw binaries and archived distributions.
// If the tmpFile is an archive (.tar.xz, .tar.gz, .zip), it extracts the
// `ffmpeg` binary from the archive and writes it to dest. Otherwise, it
// copies the file as-is (for raw binaries).
func extractAndInstall(tmpFile, dest string) error {
	// Detect content by magic numbers because temp filenames don't retain
	// original extension. We support tar.xz, tar.gz (tgz), zip and raw
	// compressed single-file streams (.xz/.gz).
	info, err := detectFileType(tmpFile)
	if err != nil {
		return err
	}

	if info.isXZ || info.isGzip {
		return extractTarOrStream(tmpFile, dest, info.isXZ)
	}
	if info.isZip {
		return extractZipArchive(tmpFile, dest)
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
		req.Header.Set(HeaderUserAgent, defaultUserAgent)
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
