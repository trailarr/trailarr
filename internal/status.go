package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Cache for latest releases to avoid hitting GitHub on every health check
var latestReleaseCacheMu sync.Mutex
var latestReleaseCache = map[string]struct {
	tag string
	ts  time.Time
}{}
var latestReleaseTTL = 12 * time.Hour

// It caches results for `latestReleaseTTL` to avoid frequent API calls.
// Implemented as a variable so tests can override it.
var fetchLatestGithubReleaseTag = func(repo string) (string, error) {
	latestReleaseCacheMu.Lock()
	cached, ok := latestReleaseCache[repo]
	latestReleaseCacheMu.Unlock()
	if ok && time.Since(cached.ts) < latestReleaseTTL {
		return cached.tag, nil
	}

	// Use a short client timeout
	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	req.Header.Set(HeaderUserAgent, defaultUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected response: %s", resp.Status)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	tag := strings.TrimPrefix(strings.TrimSpace(payload.TagName), "v")
	latestReleaseCacheMu.Lock()
	latestReleaseCache[repo] = struct {
		tag string
		ts  time.Time
	}{tag, time.Now()}
	latestReleaseCacheMu.Unlock()
	return tag, nil
}

// compareVersion tries to perform a loose semver numeric comparison of two versions.
// Returns 1 if a > b, -1 if a < b, 0 if equal or unparsable. We compare numeric sequences left-to-right.
func compareVersion(a, b string) int {
	if a == b {
		return 0
	}
	pa := parseVersion(a)
	pb := parseVersion(b)
	return compareVersionParts(pa, pb)
}

// parseVersion extracts numeric parts from a version string.
func parseVersion(s string) []int {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '.' || r == '-' || r == '_' })
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		out = append(out, extractNumber(p))
	}
	return out
}

// extractNumber extracts the leading numeric value from a string.
func extractNumber(s string) int {
	var num int
	n, _ := fmt.Sscanf(s, "%d", &num)
	if n == 1 {
		return num
	}
	// try to extract leading digits
	digits := extractLeadingDigits(s)
	if digits != "" {
		fmt.Sscanf(digits, "%d", &num)
	}
	return num
}

// extractLeadingDigits extracts the leading digit characters from a string.
func extractLeadingDigits(s string) string {
	for i, ch := range s {
		if ch < '0' || ch > '9' {
			return s[:i]
		}
	}
	return s
}

// compareVersionParts compares two parsed version arrays.
func compareVersionParts(pa, pb []int) int {
	max := len(pa)
	if len(pb) > max {
		max = len(pb)
	}
	for i := 0; i < max; i++ {
		va := getVersionPart(pa, i)
		vb := getVersionPart(pb, i)
		if cmp := compareInts(va, vb); cmp != 0 {
			return cmp
		}
	}
	return 0
}

// getVersionPart safely gets a version part, returning 0 if out of bounds.
func getVersionPart(parts []int, idx int) int {
	if idx < len(parts) {
		return parts[idx]
	}
	return 0
}

// compareInts compares two integers.
func compareInts(a, b int) int {
	if a > b {
		return 1
	}
	if a < b {
		return -1
	}
	return 0
}

// checkVersionUpdate checks if a newer version is available and appends appropriate health message.
func checkVersionUpdate(health []HealthMsg, version, repo, source string) []HealthMsg {
	if version == "" || version == "Not found" {
		return append(health, HealthMsg{Message: source + " is not installed", Source: source, Level: "error"})
	}
	tag, err := fetchLatestGithubReleaseTag(repo)
	if err != nil || tag == "" {
		return health
	}
	if compareVersion(tag, version) > 0 {
		return append(health, HealthMsg{Message: source + " update available", Source: source, Level: "warning"})
	}
	return health
}

// appendUpdateWarnings consults the network for latest releases and appends health warnings
// if installed versions are behind the latest release.
func appendUpdateWarnings(health []HealthMsg, about AboutInfo) []HealthMsg {
	// If running offline or no net available, don't block; attempt best-effort fetch.
	health = checkVersionUpdate(health, strings.TrimSpace(about.YtdlpVersion), "yt-dlp/yt-dlp", "yt-dlp")
	health = checkVersionUpdate(health, strings.TrimSpace(about.FfmpegVersion), "ffmpeg/ffmpeg", "ffmpeg")

	// Check Trailarr itself (trailarr/trailarr) for newer releases
	av := strings.TrimSpace(about.Version)
	if av != "" && av != "dev" {
		health = checkVersionUpdate(health, av, "trailarr/trailarr", "trailarr")
	}

	return health
}

// StartTime records the process start time for uptime calculations.
var StartTime = time.Now()

// AppVersion may be set at build time using -ldflags "-X 'trailarr/internal.AppVersion=<version>'".
// If unset, getModuleVersion will fall back to build info or "dev".
var AppVersion string

type HealthMsg struct {
	Message string `json:"message"`
	Source  string `json:"source"`
	Level   string `json:"level"`
}

type DiskInfo struct {
	Location string `json:"location"`
	Free     uint64 `json:"freeBytes"`
	Total    uint64 `json:"totalBytes"`
	FreeStr  string `json:"freeHuman"`
	TotalStr string `json:"totalHuman"`
	UsedPct  int    `json:"usedPercent"`
}

type AboutInfo struct {
	Version          string `json:"version"`
	AppDataDirectory string `json:"appDataDirectory"`
	StartupDirectory string `json:"startupDirectory"`
	Mode             string `json:"mode"`
	Uptime           string `json:"uptime"`
	YtdlpVersion     string `json:"ytdlpVersion"`
	FfmpegVersion    string `json:"ffmpegVersion"`
	LatestRelease    string `json:"latestRelease,omitempty"`
}

type SystemStatus struct {
	Health    []HealthMsg       `json:"health,omitempty"`
	Disks     []DiskInfo        `json:"disks"`
	About     AboutInfo         `json:"about"`
	MoreInfo  map[string]string `json:"moreInfo"`
	Donations map[string]string `json:"donations"`
}

// SystemStatusHandler returns a snapshot of basic system information used by the Status page.
func SystemStatusHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		radarrURL, radarrKey, _ := GetProviderUrlAndApiKey("radarr")
		sonarrURL, sonarrKey, _ := GetProviderUrlAndApiKey("sonarr")

		health := buildHealth(radarrURL, radarrKey, sonarrURL, sonarrKey)
		health = getHealthWithPersistedIssues(health)

		pathSet := buildPathSet(radarrURL, radarrKey, sonarrURL, sonarrKey)
		disks := buildDisks(pathSet)

		about := buildAboutInfo()

		health = enhanceHealthInfo(health, about)

		ss := buildSystemStatus(health, disks, about)
		c.JSON(http.StatusOK, ss)
	}
}

func getHealthWithPersistedIssues(health []HealthMsg) []HealthMsg {
	// If no live health issues detected, consult the persisted health issues
	// collection so the UI can surface recent problems even when the on-demand
	// checks are not performed at the same instant.
	if len(health) > 0 {
		return health
	}

	client := GetStoreClient()
	ctx := context.Background()
	vals, err := client.LRange(ctx, HealthIssuesStoreKey, 0, -1)
	if err != nil || len(vals) == 0 {
		return health
	}

	for _, v := range vals {
		var hm HealthMsg
		if err := json.Unmarshal([]byte(v), &hm); err == nil {
			health = append(health, hm)
		}
	}
	return health
}

func buildAboutInfo() AboutInfo {
	about := AboutInfo{
		Version:          getModuleVersion(),
		AppDataDirectory: TrailarrRoot,
		StartupDirectory: getStartupDir(),
		Mode:             "server",
		Uptime:           formatDuration(time.Since(StartTime)),
		YtdlpVersion:     getYtdlpVersion(),
		FfmpegVersion:    getFfmpegVersion(),
	}
	// Best-effort: set the latest release tag for Trailarr so the UI can show it.
	if tag, err := fetchLatestGithubReleaseTag("trailarr/trailarr"); err == nil && tag != "" {
		about.LatestRelease = tag
	}
	return about
}

func enhanceHealthInfo(health []HealthMsg, about AboutInfo) []HealthMsg {
	// Add TMDB connectivity check (error-level) and update warnings for yt-dlp/ffmpeg (best effort with caching)
	if tmdbHM := tmdbHealthCheck(); tmdbHM != nil {
		health = append(health, *tmdbHM)
	}
	return appendUpdateWarnings(health, about)
}

func buildSystemStatus(health []HealthMsg, disks []DiskInfo, about AboutInfo) SystemStatus {
	more := map[string]string{
		"source": "https://github.com/trailarr/trailarr",
	}

	donations := map[string]string{
		"paypal": "",
	}

	return SystemStatus{
		Health:    health,
		Disks:     disks,
		About:     about,
		MoreInfo:  more,
		Donations: donations,
	}
}

func buildHealth(radarrURL, radarrKey, sonarrURL, sonarrKey string) []HealthMsg {
	var health []HealthMsg
	// Radarr: report only when misconfigured or unreachable. Do not append a "reachable" info message.
	if radarrURL == "" || radarrKey == "" {
		health = append(health, HealthMsg{Message: "Radarr not configured (missing URL or API key)", Source: "Radarr", Level: "error"})
	} else {
		if err := testMediaConnection(radarrURL, radarrKey, "radarr"); err != nil {
			health = append(health, HealthMsg{Message: fmt.Sprintf("Radarr connectivity failed: %v", err), Source: "Radarr", Level: "error"})
		}
	}

	// Sonarr: same behavior as Radarr — only report problems.
	if sonarrURL == "" || sonarrKey == "" {
		health = append(health, HealthMsg{Message: "Sonarr not configured (missing URL or API key)", Source: "Sonarr", Level: "error"})
	} else {
		if err := testMediaConnection(sonarrURL, sonarrKey, "sonarr"); err != nil {
			health = append(health, HealthMsg{Message: fmt.Sprintf("Sonarr connectivity failed: %v", err), Source: "Sonarr", Level: "error"})
		}
	}
	return health
}

// tmdbHealthCheck performs a best-effort check of TMDB configuration and connectivity.
// If issues are detected, it returns a HealthMsg to append to the system health.
func tmdbHealthCheck() *HealthMsg {
	// If no key, report an error-level health message
	if _, err := GetTMDBKey(); err != nil {
		return &HealthMsg{Message: "TMDB not configured (missing API key)", Source: "TMDB", Level: "error"}
	}
	// If there is a key, perform a quick config check so that connectivity problems are surfaced
	key, _ := GetTMDBKey()
	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("https://api.themoviedb.org/3/configuration?api_key=%s", key)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	req.Header.Set(HeaderUserAgent, defaultUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return &HealthMsg{Message: fmt.Sprintf("TMDB connectivity failed: %v", err), Source: "TMDB", Level: "error"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &HealthMsg{Message: fmt.Sprintf("TMDB connectivity failed: %s", resp.Status), Source: "TMDB", Level: "error"}
	}
	return nil
}

func buildPathSet(radarrURL, radarrKey, sonarrURL, sonarrKey string) map[string]bool {
	pathSet := map[string]bool{}

	if data, err := os.ReadFile("/proc/mounts"); err == nil {
		for p := range parseProcMounts(data) {
			pathSet[p] = true
		}
		return pathSet
	}

	addProviderRoots(pathSet, radarrURL, radarrKey)
	addProviderRoots(pathSet, sonarrURL, sonarrKey)

	return pathSet
}

func parseProcMounts(data []byte) map[string]bool {
	lines := strings.Split(string(data), "\n")
	exclude := map[string]bool{
		"proc": true, "sysfs": true, "tmpfs": true, "devtmpfs": true,
		"devpts": true, "securityfs": true, "cgroup": true, "cgroup2": true,
		"pstore": true, "overlay": true, "tracefs": true, "configfs": true,
		"fusectl": true, "debugfs": true, "hugetlbfs": true, "rpc_pipefs": true,
		"mqueue": true, "autofs": true,
	}
	result := map[string]bool{}
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		fields := strings.Fields(l)
		if len(fields) < 3 {
			continue
		}
		src := fields[0]
		mp := fields[1]
		fstype := fields[2]
		if exclude[fstype] {
			continue
		}
		if strings.HasPrefix(src, "/dev/") {
			result[mp] = true
		}
	}
	return result
}

func addProviderRoots(pathSet map[string]bool, url, key string) {
	if url == "" {
		return
	}
	if folders, err := FetchRootFolders(url, key); err == nil {
		for _, f := range folders {
			if p, ok := f["path"].(string); ok && p != "" {
				pathSet[p] = true
			}
		}
	}
}

func getModuleVersion() string {
	// Prefer an explicitly injected version (set via ldflags) when available.
	if AppVersion != "" {
		return AppVersion
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "dev"
}

func getYtdlpVersion() string {
	if out, err := ytDlpRunner.CombinedOutput(YtDlpPath, []string{"--version"}, ""); err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

func isFfmpegAvailable() bool {
	if FfmpegPath != "" {
		fi, err := os.Stat(FfmpegPath)
		if err == nil && !fi.IsDir() {
			return true
		}
	}
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

func getFfmpegPath() string {
	if FfmpegPath != "" {
		fi, err := os.Stat(FfmpegPath)
		if err == nil && !fi.IsDir() {
			return FfmpegPath
		}
	}
	return "ffmpeg"
}

func extractVersionFromFfmpegOutput(s string) string {
	lines := strings.SplitN(s, "\n", 2)
	first := lines[0]
	parts := strings.Fields(first)
	for i, p := range parts {
		if strings.ToLower(p) == "version" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return first
}

func getFfmpegVersion() string {
	if !isFfmpegAvailable() {
		return "Not found"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmdName := getFfmpegPath()
	cmd := exec.CommandContext(ctx, cmdName, "-version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return ""
	}
	return extractVersionFromFfmpegOutput(s)
}

func getStartupDir() string {
	startupDir := "."
	if exe, err := os.Executable(); err == nil {
		startupDir = filepath.Dir(exe)
	}
	return startupDir
}

func humanBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmtBytes(b) + " B"
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	value := float64(b) / float64(div)
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	if exp < 0 || exp >= len(units) { // fallback
		return fmt.Sprintf("%.2f", value)
	}
	return fmt.Sprintf("%.2f %s", value, units[exp])
}

func fmtBytes(b uint64) string {
	return fmt.Sprintf("%d", b)
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	return fmt.Sprintf("%dd %02d:%02d:%02d", days, hours, minutes, int(d.Seconds())%60)
}
