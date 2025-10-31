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
	"time"

	"github.com/gin-gonic/gin"
)

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
		// If no live health issues detected, consult the persisted health issues
		// collection so the UI can surface recent problems even when the on-demand
		// checks are not performed at the same instant.
		if len(health) == 0 {
			client := GetStoreClient()
			ctx := context.Background()
			vals, err := client.LRange(ctx, HealthIssuesStoreKey, 0, -1)
			if err == nil && len(vals) > 0 {
				for _, v := range vals {
					var hm HealthMsg
					if err := json.Unmarshal([]byte(v), &hm); err == nil {
						health = append(health, hm)
					}
				}
			}
		}
		pathSet := buildPathSet(radarrURL, radarrKey, sonarrURL, sonarrKey)
		disks := buildDisks(pathSet)

		about := AboutInfo{
			Version:          getModuleVersion(),
			AppDataDirectory: TrailarrRoot,
			StartupDirectory: getStartupDir(),
			Mode:             "server",
			Uptime:           formatDuration(time.Since(StartTime)),
			YtdlpVersion:     getYtdlpVersion(),
			FfmpegVersion:    getFfmpegVersion(),
		}

		more := map[string]string{
			"source": "https://github.com/trailarr/trailarr",
		}

		donations := map[string]string{
			"paypal": "",
		}

		ss := SystemStatus{
			Health:    health,
			Disks:     disks,
			About:     about,
			MoreInfo:  more,
			Donations: donations,
		}
		c.JSON(http.StatusOK, ss)
	}
}

func buildHealth(radarrURL, radarrKey, sonarrURL, sonarrKey string) []HealthMsg {
	var health []HealthMsg
	// Radarr: report only when misconfigured or unreachable. Do not append a "reachable" info message.
	if radarrURL == "" || radarrKey == "" {
		health = append(health, HealthMsg{Message: "Radarr not configured (missing URL or API key)", Source: "Radarr", Level: "warning"})
	} else {
		if err := testMediaConnection(radarrURL, radarrKey, "radarr"); err != nil {
			health = append(health, HealthMsg{Message: fmt.Sprintf("Radarr connectivity failed: %v", err), Source: "Radarr", Level: "warning"})
		}
	}

	// Sonarr: same behavior as Radarr — only report problems.
	if sonarrURL == "" || sonarrKey == "" {
		health = append(health, HealthMsg{Message: "Sonarr not configured (missing URL or API key)", Source: "Sonarr", Level: "warning"})
	} else {
		if err := testMediaConnection(sonarrURL, sonarrKey, "sonarr"); err != nil {
			health = append(health, HealthMsg{Message: fmt.Sprintf("Sonarr connectivity failed: %v", err), Source: "Sonarr", Level: "warning"})
		}
	}
	return health
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
	if out, err := ytDlpRunner.CombinedOutput(YtDlpCmd, []string{"--version"}, ""); err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

func getFfmpegVersion() string {
	// Prefer returning a friendly "Not found" if ffmpeg is not available.
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return "Not found"
	}

	// Use a short timeout to avoid hanging if ffmpeg misbehaves.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffmpeg", "-version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return ""
	}
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
