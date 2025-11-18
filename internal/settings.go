package internal

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	yamlv3 "gopkg.in/yaml.v3"
)

// Trailarr uses a BoltDB-backed storage layer that exposes a store-compatible
// API to the rest of the codebase. An external store is no longer required —
// the code retains some legacy key names for compatibility but the default
// runtime storage is bbolt (on-disk) with an in-memory fallback used for tests.

const (
	ApiReturnedStatusFmt   = "API returned status %d"
	MoviesStoreKey         = "trailarr:movies"
	SeriesStoreKey         = "trailarr:series"
	MoviesWantedStoreKey   = "trailarr:movies:wanted"
	SeriesWantedStoreKey   = "trailarr:series:wanted"
	ExtrasStoreKey         = "trailarr:extras"
	RejectedExtrasStoreKey = "trailarr:extras:rejected"
	DownloadQueue          = "trailarr:download_queue"
	TaskTimesStoreKey      = "trailarr:task_times"
	HealthIssuesStoreKey   = "trailarr:health_issues"
	HistoryStoreKey        = "trailarr:history"
	HistoryMaxLen          = 1000
	TaskQueueStoreKey      = "trailarr:task_queue"
	TaskQueueMaxLen        = 1000
	RemoteMediaCoverPath   = "/MediaCover/"
	// MediaCoverRoute is the HTTP route prefix used to serve media cover images
	// from the server. Keep this constant in sync with routes that register the
	// static handler so other packages can reference it without hardcoding.
	MediaCoverRoute          = "/mediacover"
	HeaderApiKey             = "X-Api-Key"
	HeaderContentType        = "Content-Type"
	ErrInvalidSonarrSettings = "Invalid Sonarr settings"
	ErrInvalidRequest        = "invalid request"
	ErrSectionNotMap         = "section %s is not a map"
)

// Default frontend URL used when no config or env override is provided.
const DefaultFrontendURL = "http://localhost:8080"

// NOTE: store keys are defined below as the primary constants (use these names).

// Path and runtime-configurable variables. Tests may override these.
var (
	TrailarrRoot   = "/var/lib/trailarr"
	ConfigPath     = TrailarrRoot + "/config/config.yml"
	MediaCoverPath = TrailarrRoot + "/MediaCover"
	CookiesFile    = TrailarrRoot + "/cookies.txt"
	LogsDir        = TrailarrRoot + "/logs"
)

// configPathValue holds the current config path in an atomic.Value so tests
// can call SetConfigPath concurrently without causing data races when
// background goroutines read the path via GetConfigPath.
var configPathValue atomic.Value

// GetConfigPath returns the current path used for the config file. Tests
// and callers may call SetConfigPath to inject an alternate path for
// testing or runtime customization.
func GetConfigPath() string {
	// Prefer atomic-backed value when available to avoid races.
	if v := configPathValue.Load(); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ConfigPath
}

// SetConfigPath sets the path used for configuration reads/writes. This is
// a small, test-friendly hook that lets callers inject a per-test config
// file without modifying other code sites directly.
func SetConfigPath(p string) {
	// Store atomically so concurrent readers (background goroutines/tests)
	// don't race with writers. Also keep the plain ConfigPath variable in
	// sync for compatibility with any code that still reads it directly.
	configPathValue.Store(p)
	ConfigPath = p
}

// Initialize atomic config path value at package init to the default
func init() {
	configPathValue.Store(ConfigPath)
}

// Global in-memory config
var Config map[string]interface{}

// LoadConfig reads config.yml into the global Config variable
func LoadConfig() error {
	data, err := os.ReadFile(GetConfigPath())
	if err != nil {
		return err
	}
	var cfg map[string]interface{}
	err = yamlv3.Unmarshal(data, &cfg)
	if err != nil {
		return err
	}
	Config = cfg
	return nil
}

type PlexType string

const (
	BehindTheScenes PlexType = "Behind The Scenes"
	DeletedScenes   PlexType = "Deleted Scenes"
	Featurettes     PlexType = "Featurettes"
	Interviews      PlexType = "Interviews"
	Scenes          PlexType = "Scenes"
	Shorts          PlexType = "Shorts"
	Trailers        PlexType = "Trailers"
	Other           PlexType = "Other"
)

var defaultExtraTypes = ExtraTypesConfig{
	Trailers:        true,
	Scenes:          false,
	BehindTheScenes: false,
	Interviews:      false,
	Featurettes:     false,
	DeletedScenes:   false,
	Other:           false,
}

// CanonicalizeExtraTypeConfig holds mapping from TMDB extra types to Plex extra types
type CanonicalizeExtraTypeConfig struct {
	Mapping map[string]string `yaml:"mapping" json:"mapping"`
}

// GetCanonicalizeExtraTypeConfig loads mapping config from config.yml
func GetCanonicalizeExtraTypeConfig() (CanonicalizeExtraTypeConfig, error) {
	// Read the config file and attempt to extract the canonicalizeExtraType
	// mapping. Tests can spawn background goroutines that also write the
	// config file, which can cause a rare race where the file is briefly
	// overwritten between a test write and this read. To reduce flakiness,
	// retry a few times when the mapping isn't present.
	for attempt := 0; attempt < 5; attempt++ {
		data, err := os.ReadFile(GetConfigPath())
		if err != nil {
			// If file read failed, retry briefly
			time.Sleep(10 * time.Millisecond)
			continue
		}
		cfg := parseCanonicalizeExtraTypeConfig(data)
		if len(cfg.Mapping) > 0 {
			return cfg, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Final fallback: attempt one last read and return whatever we have.
	data, err := os.ReadFile(GetConfigPath())
	if err != nil {
		return CanonicalizeExtraTypeConfig{Mapping: map[string]string{}}, err
	}
	return parseCanonicalizeExtraTypeConfig(data), nil
}

// parseCanonicalizeExtraTypeConfig extracts the canonicalizeExtraType.mapping
// from raw YAML data into a CanonicalizeExtraTypeConfig.
func parseCanonicalizeExtraTypeConfig(data []byte) CanonicalizeExtraTypeConfig {
	// Unmarshal into an empty interface and normalize so that YAML
	// decoder-produced map[interface{}]interface{} values are converted
	// into map[string]interface{}. This makes the parsing robust across
	// environments and avoids flakiness where nested maps have non-string
	// key types.
	var raw interface{}
	_ = yamlv3.Unmarshal(data, &raw)
	normalized := normalizeYAML(raw)

	cfg := CanonicalizeExtraTypeConfig{Mapping: map[string]string{}}
	config, ok := normalized.(map[string]interface{})
	if !ok {
		return cfg
	}
	sec, ok := config["canonicalizeExtraType"].(map[string]interface{})
	if !ok {
		return cfg
	}
	if m, ok := sec["mapping"].(map[string]interface{}); ok {
		for k, v := range m {
			if s, ok := v.(string); ok {
				cfg.Mapping[k] = s
			}
		}
	}
	return cfg
}

// SaveCanonicalizeExtraTypeConfig saves mapping config to config.yml
func SaveCanonicalizeExtraTypeConfig(cfg CanonicalizeExtraTypeConfig) error {
	config, err := readConfigFile()
	if err != nil {
		config = map[string]interface{}{}
	}
	config["canonicalizeExtraType"] = map[string]interface{}{
		"mapping": cfg.Mapping,
	}
	return writeConfigFile(config)
}

// Handler to get canonicalizeExtraType config
func GetCanonicalizeExtraTypeConfigHandler(c *gin.Context) {
	cfg, _ := GetCanonicalizeExtraTypeConfig()
	respondJSON(c, http.StatusOK, cfg)
}

// Handler to save canonicalizeExtraType config
func SaveCanonicalizeExtraTypeConfigHandler(c *gin.Context) {
	var req CanonicalizeExtraTypeConfig
	if err := c.BindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, ErrInvalidRequest)
		return
	}
	if err := SaveCanonicalizeExtraTypeConfig(req); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(c, http.StatusOK, gin.H{"status": "saved"})
}

// Default values for config.yml
func DefaultGeneralConfig() map[string]interface{} {
	return map[string]interface{}{
		"tmdbKey":            "",
		"autoDownloadExtras": true,
		"logLevel":           "Info",
		// frontendUrl may be used by OAuth flows to build redirect targets;
		// keep default pointing at the local dev frontend so devs don't need
		// to set it explicitly.
		"frontendUrl": DefaultFrontendURL,
		// trustedProxies controls the CIDRs Gin trusts when determining the
		// client IP via forwarded headers. For security we default to
		// trusting only the loopback interface. Administrators may override
		// this in config.yml to add their reverse proxy networks.
		"trustedProxies": []string{"127.0.0.1"},
	}
}

// GetTrustedProxies returns the list of CIDRs configured for trusted proxies.
// If not present in config it returns the default of only loopback.
func GetTrustedProxies() ([]string, error) {
	cfg, err := readConfigFile()
	if err != nil {
		// Return default if we can't read config
		return []string{"127.0.0.1"}, nil
	}
	general, ok := cfg["general"].(map[string]interface{})
	if !ok || general == nil {
		return []string{"127.0.0.1"}, nil
	}
	if v, ok := general["trustedProxies"]; ok {
		switch t := v.(type) {
		case []interface{}:
			var res []string
			for _, e := range t {
				if s, ok := e.(string); ok {
					res = append(res, s)
				}
			}
			if len(res) > 0 {
				return res, nil
			}
		case []string:
			if len(t) > 0 {
				return t, nil
			}
		}
	}
	return []string{"127.0.0.1"}, nil
}

// normalizeYAML recursively converts maps with non-string keys (which can be
// produced by some YAML unmarshallers) into map[string]interface{} so callers
// can reliably type-assert on map[string]interface{}.
func normalizeYAML(v interface{}) interface{} {
	switch t := v.(type) {
	case map[interface{}]interface{}:
		m := map[string]interface{}{}
		for k, val := range t {
			m[fmt.Sprintf("%v", k)] = normalizeYAML(val)
		}
		return m
	case map[string]interface{}:
		for k, val := range t {
			t[k] = normalizeYAML(val)
		}
		return t
	case []interface{}:
		for i, val := range t {
			t[i] = normalizeYAML(val)
		}
		return t
	default:
		return v
	}
}

func EnsureConfigDefaults() error {
	var changed bool
	config, err := readConfigFileRaw()
	if err != nil {
		// Create file with core defaults when missing
		config = map[string]interface{}{
			"general":    DefaultGeneralConfig(),
			"ytdlpFlags": DefaultYtdlpFlagsConfig(),
		}
		changed = true
	}
	// Ensure each section's defaults via helper functions
	if ensureGeneralDefaults(config) {
		changed = true
	}
	if ensureYtdlpDefaults(config) {
		changed = true
	}
	if ensureRadarrDefaults(config) {
		changed = true
	}
	if ensureSonarrDefaults(config) {
		changed = true
	}
	if ensurePlexDefaults(config) {
		changed = true
	}
	if ensureExtraTypesDefaults(config) {
		changed = true
	}
	if ensureCanonicalizeExtraTypeDefaults(config) {
		changed = true
	}
	if changed {
		return writeConfigFile(config)
	}
	return nil
}

func ensureGeneralDefaults(config map[string]interface{}) bool {
	defaults := DefaultGeneralConfig()
	if config["general"] == nil {
		config["general"] = defaults
		return true
	}
	general, ok := config["general"].(map[string]interface{})
	if !ok {
		config["general"] = defaults
		return true
	}
	changed := false
	for k, v := range defaults {
		if _, ok := general[k]; !ok {
			general[k] = v
			changed = true
		}
	}
	config["general"] = general
	return changed
}

func ensureYtdlpDefaults(config map[string]interface{}) bool {
	if config["ytdlpFlags"] == nil {
		config["ytdlpFlags"] = DefaultYtdlpFlagsConfig()
		return true
	}
	_, ok := config["ytdlpFlags"].(map[string]interface{})
	if !ok {
		config["ytdlpFlags"] = DefaultYtdlpFlagsConfig()
		return true
	}
	// If the section exists and is a map we don't inject any legacy keys.
	// Tests that need per-test config create their own files.
	return false
}

func ensureRadarrDefaults(config map[string]interface{}) bool {
	defaultConfig := map[string]interface{}{
		"url":          "http://localhost:7878",
		"apiKey":       "",
		"pathMappings": []map[string]string{},
	}
	if config["radarr"] == nil {
		config["radarr"] = defaultConfig
		return true
	}
	radarr, ok := config["radarr"].(map[string]interface{})
	if !ok {
		config["radarr"] = defaultConfig
		return true
	}
	changed := false
	if _, ok := radarr["url"]; !ok {
		radarr["url"] = defaultConfig["url"]
		changed = true
	}
	if _, ok := radarr["apiKey"]; !ok {
		radarr["apiKey"] = defaultConfig["apiKey"]
		changed = true
	}
	if _, ok := radarr["pathMappings"]; !ok {
		radarr["pathMappings"] = defaultConfig["pathMappings"]
		changed = true
	}
	config["radarr"] = radarr
	return changed
}

func ensureSonarrDefaults(config map[string]interface{}) bool {
	defaultConfig := map[string]interface{}{
		"url":          "http://localhost:8989",
		"apiKey":       "",
		"pathMappings": []map[string]string{},
	}
	if config["sonarr"] == nil {
		config["sonarr"] = defaultConfig
		return true
	}
	sonarr, ok := config["sonarr"].(map[string]interface{})
	if !ok {
		config["sonarr"] = defaultConfig
		return true
	}
	changed := false
	if _, ok := sonarr["url"]; !ok {
		sonarr["url"] = defaultConfig["url"]
		changed = true
	}
	if _, ok := sonarr["apiKey"]; !ok {
		sonarr["apiKey"] = defaultConfig["apiKey"]
		changed = true
	}
	if _, ok := sonarr["pathMappings"]; !ok {
		sonarr["pathMappings"] = defaultConfig["pathMappings"]
		changed = true
	}
	config["sonarr"] = sonarr
	return changed
}

func ensurePlexDefaults(config map[string]interface{}) bool {
	defaultConfig := map[string]interface{}{
		"protocol": "http",
		"ip":       "localhost",
		"port":     32400,
		"token":    "",
		"clientId": "",
		"enabled":  false,
	}
	if config["plex"] == nil {
		config["plex"] = defaultConfig
		return true
	}
	plex, ok := config["plex"].(map[string]interface{})
	if !ok {
		config["plex"] = defaultConfig
		return true
	}
	changed := false
	if _, ok := plex["protocol"]; !ok {
		plex["protocol"] = defaultConfig["protocol"]
		changed = true
	}
	if _, ok := plex["ip"]; !ok {
		plex["ip"] = defaultConfig["ip"]
		changed = true
	}
	if _, ok := plex["port"]; !ok {
		plex["port"] = defaultConfig["port"]
		changed = true
	}
	if _, ok := plex["token"]; !ok {
		plex["token"] = defaultConfig["token"]
		changed = true
	}
	if _, ok := plex["clientId"]; !ok {
		plex["clientId"] = defaultConfig["clientId"]
		changed = true
	}
	if _, ok := plex["enabled"]; !ok {
		plex["enabled"] = defaultConfig["enabled"]
		changed = true
	}
	config["plex"] = plex
	return changed
}

func ensureExtraTypesDefaults(config map[string]interface{}) bool {
	if config["extraTypes"] == nil {
		config["extraTypes"] = map[string]interface{}{
			"trailers":        defaultExtraTypes.Trailers,
			"scenes":          defaultExtraTypes.Scenes,
			"behindTheScenes": defaultExtraTypes.BehindTheScenes,
			"interviews":      defaultExtraTypes.Interviews,
			"featurettes":     defaultExtraTypes.Featurettes,
			"deletedScenes":   defaultExtraTypes.DeletedScenes,
			"other":           defaultExtraTypes.Other,
		}
		return true
	}
	extraTypes, ok := config["extraTypes"].(map[string]interface{})
	if !ok {
		config["extraTypes"] = map[string]interface{}{}
		extraTypes = config["extraTypes"].(map[string]interface{})
	}
	changed := false
	defaults := map[string]bool{
		"trailers":        defaultExtraTypes.Trailers,
		"scenes":          defaultExtraTypes.Scenes,
		"behindTheScenes": defaultExtraTypes.BehindTheScenes,
		"interviews":      defaultExtraTypes.Interviews,
		"featurettes":     defaultExtraTypes.Featurettes,
		"deletedScenes":   defaultExtraTypes.DeletedScenes,
		"other":           defaultExtraTypes.Other,
	}
	for k, v := range defaults {
		if _, ok := extraTypes[k]; !ok {
			extraTypes[k] = v
			changed = true
		}
	}
	config["extraTypes"] = extraTypes
	return changed
}

func ensureCanonicalizeExtraTypeDefaults(config map[string]interface{}) bool {
	if config["canonicalizeExtraType"] == nil {
		// Default mapping: singular TMDB types to Plex types
		config["canonicalizeExtraType"] = map[string]interface{}{
			"mapping": map[string]string{
				"Trailer":          string(Trailers),
				"Featurette":       string(Featurettes),
				"Behind the Scene": string(BehindTheScenes),
				"Deleted Scene":    string(DeletedScenes),
				"Interview":        string(Interviews),
				"Scene":            string(Scenes),
				"Short":            string(Shorts),
				"Other":            string(Other),
			},
		}
		return true
	}
	return false
}

// Raw config file reader (no defaults)
func readConfigFileRaw() (map[string]interface{}, error) {
	data, err := os.ReadFile(GetConfigPath())
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		// Treat empty file as missing config
		return nil, fmt.Errorf("empty config file")
	}
	// Unmarshal into an empty interface then normalize to ensure any
	// map[interface{}]interface{} values produced by the YAML decoder are
	// converted into map[string]interface{} so callers can reliably index
	// using string keys.
	var raw interface{}
	if err := yamlv3.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	normalized := normalizeYAML(raw)
	if normalized == nil {
		return nil, fmt.Errorf("invalid config format")
	}
	cfg, ok := normalized.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("config is not a map")
	}
	return cfg, nil
}

// Helper to read config file and unmarshal into map[string]interface{}
func readConfigFile() (map[string]interface{}, error) {
	_ = EnsureConfigDefaults()
	data, err := os.ReadFile(GetConfigPath())
	if err != nil {
		return nil, err
	}
	var config map[string]interface{}
	err = yamlv3.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}
	// Normalize YAML maps to ensure keys are strings and nested maps are
	// represented as map[string]interface{}. This avoids callers having to
	// handle map[interface{}]interface{} returned by the YAML decoder.
	if config != nil {
		normalized := normalizeYAML(config)
		if cfgMap, ok := normalized.(map[string]interface{}); ok {
			return cfgMap, nil
		}
	}
	return map[string]interface{}{}, nil
}

// Helper to write config map to file
func writeConfigFile(config map[string]interface{}) error {
	// Ensure parent directory exists to avoid errors when background
	// goroutines attempt to persist merged settings and the temp test
	// environment's config dir hasn't been created or was removed.
	dir := filepath.Dir(GetConfigPath())
	if dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}

	// Read existing file and merge top-level keys so concurrent writers
	// that only update subsets of the config don't inadvertently wipe
	// keys written by other goroutines. New values in `config` take
	// precedence; missing keys are preserved from disk.
	existing := map[string]interface{}{}
	if data, err := os.ReadFile(GetConfigPath()); err == nil {
		var onDisk map[string]interface{}
		if err := yamlv3.Unmarshal(data, &onDisk); err == nil {
			existing = normalizeYAML(onDisk).(map[string]interface{})
		}
	}
	for k, v := range config {
		existing[k] = v
	}

	out, err := yamlv3.Marshal(existing)
	if err != nil {
		return err
	}

	// Write atomically via temp file + rename to avoid partial writes.
	// Use a unique temp filename to avoid collisions with other writers
	// (tests and background goroutines may create their own temp files).
	tmp := fmt.Sprintf("%s.%d.tmp", GetConfigPath(), time.Now().UnixNano())
	if err := os.WriteFile(tmp, out, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, GetConfigPath())
}

// EnsureYtdlpFlagsConfigExists checks config.yml and writes defaults if missing
func EnsureYtdlpFlagsConfigExists() error {
	config, err := readConfigFile()
	if err != nil {
		// If config file doesn't exist, create it with defaults
		config = map[string]interface{}{
			"ytdlpFlags": DefaultYtdlpFlagsConfig(),
		}
		return writeConfigFile(config)
	}
	if _, ok := config["ytdlpFlags"].(map[string]interface{}); !ok {
		// Add defaults if missing
		config["ytdlpFlags"] = DefaultYtdlpFlagsConfig()
		return writeConfigFile(config)
	}
	return nil
}

// --- YTDLP FLAGS CONFIG ---

// Loads yt-dlp flags config from config.yml
func GetYtdlpFlagsConfig() (YtdlpFlagsConfig, error) {
	// Some tests write the config file concurrently; retry briefly when the
	// 'ytdlpFlags' section is not present to avoid flakiness.
	cfg := DefaultYtdlpFlagsConfig()
	for attempt := 0; attempt < 20; attempt++ {
		data, err := os.ReadFile(GetConfigPath())
		if err != nil {
			// If read failed, retry briefly
			time.Sleep(20 * time.Millisecond)
			continue
		}
		var config map[string]interface{}
		if err := yamlv3.Unmarshal(data, &config); err != nil {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		// Normalize any map[interface{}]interface{} that yaml may have produced.
		config = normalizeYAML(config).(map[string]interface{})
		if sec, ok := config["ytdlpFlags"].(map[string]interface{}); ok {
			applyYtdlpFlags(sec, &cfg)
			return cfg, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Final fallback: return defaults (no error) if the section remains missing.
	return cfg, nil
}

// helper converters extracted to reduce cognitive complexity
func toBool(v interface{}) (bool, bool) {
	if b, ok := v.(bool); ok {
		return b, true
	}
	return false, false
}

func toString(v interface{}) (string, bool) {
	if s, ok := v.(string); ok {
		return s, true
	}
	return "", false
}

func toFloat64(v interface{}) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case string:
		var parsed float64
		_, err := fmt.Sscanf(t, "%f", &parsed)
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func toInt(v interface{}) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case int64:
		return int(t), true
	case float64:
		return int(t), true
	case string:
		var parsed int
		_, err := fmt.Sscanf(t, "%d", &parsed)
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

// applyYtdlpFlags applies values from the parsed 'ytdlpFlags' section into cfg using a table-driven setter map.
func applyYtdlpFlags(sec map[string]interface{}, cfg *YtdlpFlagsConfig) {
	if sec == nil || cfg == nil {
		return
	}
	setters := ytdlpFlagSetters(cfg)
	for k, v := range sec {
		if setter, exists := setters[k]; exists {
			setter(v)
		}
	}
}

// ytdlpFlagSetters returns the map of setters for ytdlp flags bound to the provided cfg.
// Extracting the large table into its own function reduces the cognitive complexity of applyYtdlpFlags.
func ytdlpFlagSetters(cfg *YtdlpFlagsConfig) map[string]func(interface{}) {
	// Factory helpers produce small, typed setter functions to keep this function simple.
	boolSetter := func(dst *bool) func(interface{}) {
		return func(v interface{}) {
			if val, ok := toBool(v); ok {
				*dst = val
			}
		}
	}
	stringSetter := func(dst *string) func(interface{}) {
		return func(v interface{}) {
			if val, ok := toString(v); ok {
				*dst = val
			}
		}
	}
	floatSetter := func(dst *float64) func(interface{}) {
		return func(v interface{}) {
			if val, ok := toFloat64(v); ok {
				*dst = val
			}
		}
	}
	intSetter := func(dst *int) func(interface{}) {
		return func(v interface{}) {
			if val, ok := toInt(v); ok {
				*dst = val
			}
		}
	}

	return map[string]func(interface{}){
		"quiet":            boolSetter(&cfg.Quiet),
		"noprogress":       boolSetter(&cfg.NoProgress),
		"writesubs":        boolSetter(&cfg.WriteSubs),
		"writeautosubs":    boolSetter(&cfg.WriteAutoSubs),
		"embedsubs":        boolSetter(&cfg.EmbedSubs),
		"sublangs":         stringSetter(&cfg.SubLangs),
		"requestedformats": stringSetter(&cfg.RequestedFormats),
		"timeout":          floatSetter(&cfg.Timeout),
		"sleepInterval":    floatSetter(&cfg.SleepInterval),
		"maxDownloads":     intSetter(&cfg.MaxDownloads),
		"limitRate":        stringSetter(&cfg.LimitRate),
		"sleepRequests":    floatSetter(&cfg.SleepRequests),
		"maxSleepInterval": floatSetter(&cfg.MaxSleepInterval),
		"ffmpegLocation":   stringSetter(&cfg.FfmpegLocation),
	}
}

// Saves yt-dlp flags config to config.yml
func SaveYtdlpFlagsConfig(cfg YtdlpFlagsConfig) error {
	config, err := readConfigFile()
	if err != nil {
		config = map[string]interface{}{}
	}
	config["ytdlpFlags"] = map[string]interface{}{
		"quiet":            cfg.Quiet,
		"noprogress":       cfg.NoProgress,
		"writesubs":        cfg.WriteSubs,
		"writeautosubs":    cfg.WriteAutoSubs,
		"embedsubs":        cfg.EmbedSubs,
		"sublangs":         cfg.SubLangs,
		"requestedformats": cfg.RequestedFormats,
		"timeout":          cfg.Timeout,
		"sleepInterval":    cfg.SleepInterval,
		"maxDownloads":     cfg.MaxDownloads,
		"limitRate":        cfg.LimitRate,
		"sleepRequests":    cfg.SleepRequests,
		"maxSleepInterval": cfg.MaxSleepInterval,
		"ffmpegLocation":   cfg.FfmpegLocation,
	}
	return writeConfigFile(config)
}

// Handler to get yt-dlp flags config
func GetYtdlpFlagsConfigHandler(c *gin.Context) {
	cfg, err := GetYtdlpFlagsConfig()
	if err != nil {
		respondJSON(c, http.StatusOK, cfg)
		return
	}
	respondJSON(c, http.StatusOK, cfg)
}

// Handler to save yt-dlp flags config
func SaveYtdlpFlagsConfigHandler(c *gin.Context) {
	var req YtdlpFlagsConfig
	if err := c.BindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, ErrInvalidRequest)
		return
	}
	if err := SaveYtdlpFlagsConfig(req); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(c, http.StatusOK, gin.H{"status": "saved"})
}

var Timings map[string]int

// PlexConfig holds Plex server connection settings
type PlexConfig struct {
	Protocol string `yaml:"protocol" json:"protocol"`
	IP       string `yaml:"ip" json:"ip"`
	Port     int    `yaml:"port" json:"port"`
	Token    string `yaml:"token" json:"token"`
	ClientId string `yaml:"clientId" json:"clientId"`
	Enabled  bool   `yaml:"enabled" json:"enabled"`
}

// ExtraTypesConfig holds config for enabling/disabling specific extra types
type ExtraTypesConfig struct {
	Trailers        bool `yaml:"trailers" json:"trailers"`
	Scenes          bool `yaml:"scenes" json:"scenes"`
	BehindTheScenes bool `yaml:"behindTheScenes" json:"behindTheScenes"`
	Interviews      bool `yaml:"interviews" json:"interviews"`
	Featurettes     bool `yaml:"featurettes" json:"featurettes"`
	DeletedScenes   bool `yaml:"deletedScenes" json:"deletedScenes"`
	Shorts          bool `yaml:"shorts" json:"shorts"`
	Other           bool `yaml:"other" json:"other"`
}

// GetExtraTypesConfig loads extra types config from config.yml
func GetExtraTypesConfig() (ExtraTypesConfig, error) {
	// If we don't have an in-memory config, try to read from disk so callers see persisted values.
	if Config == nil {
		if cfgMap, err := readConfigFile(); err == nil {
			Config = cfgMap
		} else {
			// If read failed, fall back to defaults
			return defaultExtraTypes, nil
		}
	}
	sec, _ := Config["extraTypes"].(map[string]interface{})
	cfg := ExtraTypesConfig{}

	// helper returns the boolean value from the section or the provided default
	getBool := func(key string, def bool) bool {
		if sec == nil {
			return def
		}
		if v, ok := sec[key].(bool); ok {
			return v
		}
		return def
	}

	cfg.Trailers = getBool("trailers", defaultExtraTypes.Trailers)
	cfg.Scenes = getBool("scenes", defaultExtraTypes.Scenes)
	cfg.BehindTheScenes = getBool("behindTheScenes", defaultExtraTypes.BehindTheScenes)
	cfg.Interviews = getBool("interviews", defaultExtraTypes.Interviews)
	cfg.Featurettes = getBool("featurettes", defaultExtraTypes.Featurettes)
	cfg.DeletedScenes = getBool("deletedScenes", defaultExtraTypes.DeletedScenes)
	cfg.Shorts = getBool("shorts", defaultExtraTypes.Shorts)
	cfg.Other = getBool("other", defaultExtraTypes.Other)

	return cfg, nil
}

// SaveExtraTypesConfig saves extra types config to config.yml
func SaveExtraTypesConfig(cfg ExtraTypesConfig) error {
	config, err := readConfigFile()
	if err != nil {
		config = map[string]interface{}{}
	}
	config["extraTypes"] = map[string]interface{}{
		"trailers":        cfg.Trailers,
		"scenes":          cfg.Scenes,
		"behindTheScenes": cfg.BehindTheScenes,
		"interviews":      cfg.Interviews,
		"featurettes":     cfg.Featurettes,
		"deletedScenes":   cfg.DeletedScenes,
		"other":           cfg.Other,
	}
	err = writeConfigFile(config)
	if err == nil {
		// Update in-memory config to reflect persisted changes so future GETs return them.
		if Config == nil {
			Config = config
		} else {
			Config["extraTypes"] = config["extraTypes"]
		}
	}
	return err
}

// Handler to get extra types config
func GetExtraTypesConfigHandler(c *gin.Context) {
	cfg, err := GetExtraTypesConfig()
	if err != nil {
		// Even on error, return defaults plus plex type labels
		plexTypes := []string{string(Trailers), string(Scenes), string(BehindTheScenes), string(Interviews), string(Featurettes), string(DeletedScenes), string(Other)}
		resp := map[string]interface{}{
			"plexTypes": plexTypes,
		}
		respondJSON(c, http.StatusOK, resp)
		return
	}

	// Build resp array with entries of { key, label, value }
	resp := []map[string]interface{}{
		{"key": "trailers", "label": string(Trailers), "value": cfg.Trailers},
		{"key": "scenes", "label": string(Scenes), "value": cfg.Scenes},
		{"key": "behindTheScenes", "label": string(BehindTheScenes), "value": cfg.BehindTheScenes},
		{"key": "interviews", "label": string(Interviews), "value": cfg.Interviews},
		{"key": "featurettes", "label": string(Featurettes), "value": cfg.Featurettes},
		{"key": "deletedScenes", "label": string(DeletedScenes), "value": cfg.DeletedScenes},
		{"key": "shorts", "label": string(Shorts), "value": cfg.Shorts},
		{"key": "other", "label": string(Other), "value": cfg.Other},
	}

	// Respond with only the resp array (frontend expects key/label/value and will use them as-is)
	respondJSON(c, http.StatusOK, resp)
}

// Handler to save extra types config
func SaveExtraTypesConfigHandler(c *gin.Context) {
	var req ExtraTypesConfig
	if err := c.BindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, ErrInvalidRequest)
		return
	}
	if err := SaveExtraTypesConfig(req); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(c, http.StatusOK, gin.H{"status": "saved"})
}

// EnsureSyncTimingsConfig creates config.yml with sync timings if not present, or loads timings if present
func EnsureSyncTimingsConfig() (map[string]int, error) {
	defaultTimings := map[string]int{
		"healthcheck": 360,
		"radarr":      15,
		"sonarr":      15,
		"extras":      360,
	}

	// If the file doesn't exist create it with defaults
	if _, err := os.Stat(GetConfigPath()); os.IsNotExist(err) {
		return createConfigWithTimings(defaultTimings)
	}

	// Load existing config
	data, err := os.ReadFile(GetConfigPath())
	if err != nil {
		return defaultTimings, err
	}
	var cfg map[string]interface{}
	if err := yamlv3.Unmarshal(data, &cfg); err != nil {
		return defaultTimings, err
	}
	cfg = normalizeYAML(cfg).(map[string]interface{})

	// Extract timings section, write defaults if missing
	timings, ok := cfg["syncTimings"].(map[string]interface{})
	if !ok {
		_ = writeSyncTimingsToConfig(cfg, defaultTimings)
		return defaultTimings, nil
	}

	// Ensure extras key exists
	if _, hasExtras := timings["extras"]; !hasExtras {
		timings["extras"] = 360
		cfg["syncTimings"] = timings
		out, err := yamlv3.Marshal(cfg)
		if err == nil {
			_ = os.WriteFile(GetConfigPath(), out, 0644)
		}
	}

	// Ensure healthcheck key exists (6 hours). If missing, persist it so the
	// default schedule is present in config.yml.
	if _, hasHealth := timings["healthcheck"]; !hasHealth {
		timings["healthcheck"] = 360
		cfg["syncTimings"] = timings
		out, err := yamlv3.Marshal(cfg)
		if err == nil {
			_ = os.WriteFile(GetConfigPath(), out, 0644)
		}
	}

	// Convert to map[string]int and return
	return convertTimings(timings), nil
}

// createConfigWithTimings writes a new config file with only syncTimings set to the provided map.
func createConfigWithTimings(timings map[string]int) (map[string]int, error) {
	cfg := map[string]interface{}{"syncTimings": timings}
	out, err := yamlv3.Marshal(cfg)
	if err != nil {
		return timings, err
	}
	// Ensure parent dir exists
	if err := os.MkdirAll(TrailarrRoot+"/config", 0775); err != nil {
		return timings, err
	}
	if err := os.WriteFile(GetConfigPath(), out, 0644); err != nil {
		return timings, err
	}
	return timings, nil
}

// writeSyncTimingsToConfig adds/updates the syncTimings section in the provided config map and writes the file.
// It ignores write errors to preserve original behavior where write failures were non-fatal.
func writeSyncTimingsToConfig(cfg map[string]interface{}, timings map[string]int) error {
	cfg["syncTimings"] = timings
	out, err := yamlv3.Marshal(cfg)
	if err != nil {
		return err
	}
	_ = os.WriteFile(GetConfigPath(), out, 0644)
	return nil
}

// convertTimings converts a map[string]interface{} of timing values into map[string]int.
func convertTimings(timings map[string]interface{}) map[string]int {
	result := map[string]int{}
	for k, v := range timings {
		switch val := v.(type) {
		case int:
			result[k] = val
		case int64:
			result[k] = int(val)
		case float64:
			result[k] = int(val)
		case uint64:
			result[k] = int(val)
		case uint:
			result[k] = int(val)
		case string:
			var parsed int
			_, err := fmt.Sscanf(val, "%d", &parsed)
			if err == nil {
				result[k] = parsed
			}
		default:
			var parsed int
			_, err := fmt.Sscanf(fmt.Sprintf("%v", val), "%d", &parsed)
			if err == nil {
				result[k] = parsed
			}
		}
	}
	return result
}

// Loads settings for a given section ("radarr" or "sonarr")
func loadMediaSettings(section string) (MediaSettings, error) {
	data, err := os.ReadFile(GetConfigPath())
	if err != nil {
		TrailarrLog(WARN, "Settings", "settings not found: %v", err)
		return MediaSettings{}, fmt.Errorf("settings not found: %w", err)
	}
	var allSettings map[string]interface{}
	if err := yamlv3.Unmarshal(data, &allSettings); err != nil {
		TrailarrLog(WARN, "Settings", "invalid settings: %v", err)
		return MediaSettings{}, fmt.Errorf("invalid settings: %w", err)
	}
	// Normalize top-level map keys to strings when needed
	allSettings = normalizeYAML(allSettings).(map[string]interface{})
	secRaw, ok := allSettings[section]
	if !ok {
		TrailarrLog(WARN, "Settings", "section %s not found", section)
		return MediaSettings{}, fmt.Errorf("section %s not found", section)
	}
	sec, ok := secRaw.(map[string]interface{})
	if !ok {
		TrailarrLog(WARN, "Settings", ErrSectionNotMap, section)
		return MediaSettings{}, fmt.Errorf(ErrSectionNotMap, section)
	}
	var ProviderURL, apiKey string
	if v, ok := sec["url"].(string); ok {
		ProviderURL = v
	}
	if v, ok := sec["apiKey"].(string); ok {
		apiKey = v
	}
	return MediaSettings{ProviderURL: ProviderURL, APIKey: apiKey}, nil
}

// GetPathMappings reads pathMappings for a section ("radarr" or "sonarr") from config.yml and returns as [][]string
func GetPathMappings(mediaType MediaType) ([][]string, error) {
	section := "radarr"
	if mediaType == MediaTypeTV {
		section = "sonarr"
	}
	data, err := os.ReadFile(GetConfigPath())
	if err != nil {
		return nil, err
	}
	var config map[string]interface{}
	if err := yamlv3.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	config = normalizeYAML(config).(map[string]interface{})
	sec, _ := config[section].(map[string]interface{})
	return extractPathMappings(sec), nil
}

// extractPathMappings converts a section's pathMappings into [][]string.
func extractPathMappings(sec map[string]interface{}) [][]string {
	if sec == nil {
		return nil
	}
	result := [][]string{}
	pm, ok := sec["pathMappings"].([]interface{})
	if !ok {
		return result
	}
	for _, m := range pm {
		mMap, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		from, _ := mMap["from"].(string)
		to, _ := mMap["to"].(string)
		if from == "" || to == "" {
			continue
		}
		result = append(result, []string{from, to})
	}
	return result
}

// Returns a Gin handler for settings (url, apiKey, pathMappings) for a given section ("radarr" or "sonarr")
// Returns url and apiKey for a given section (radarr/sonarr) from config.yml
func GetProviderUrlAndApiKey(provider string) (string, string, error) {
	data, err := os.ReadFile(GetConfigPath())
	if err != nil {
		return "", "", err
	}
	var config map[string]interface{}
	if err := yamlv3.Unmarshal(data, &config); err != nil {
		return "", "", err
	}
	// Be permissive about the underlying map type because YAML unmarshalling
	// can sometimes produce map[string]interface{} or map[interface{}]interface{}
	// depending on the decoder. First check presence, then handle known map
	// shapes. If the section is missing, return an error as callers expect.
	config = normalizeYAML(config).(map[string]interface{})
	secRaw, exists := config[provider]
	if !exists {
		TrailarrLog(WARN, "Settings", "section %s not found in config", provider)
		return "", "", fmt.Errorf("section %s not found in config", provider)
	}

	// Helper to extract url/apiKey from a generic map
	extract := func(m map[string]interface{}) (string, string) {
		providerURL, _ := m["url"].(string)
		apiKey, _ := m["apiKey"].(string)
		return providerURL, apiKey
	}

	switch sec := secRaw.(type) {
	case map[string]interface{}:
		u, k := extract(sec)
		return u, k, nil
	case map[interface{}]interface{}:
		// convert keys to strings
		conv := map[string]interface{}{}
		for k, v := range sec {
			if ks, ok := k.(string); ok {
				conv[ks] = v
			}
		}
		u, k := extract(conv)
		return u, k, nil
	default:
		TrailarrLog(WARN, "Settings", ErrSectionNotMap, provider)
		return "", "", fmt.Errorf(ErrSectionNotMap, provider)
	}
}

func GetSettingsHandler(section string) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := os.ReadFile(GetConfigPath())
		if err != nil {
			respondJSON(c, http.StatusOK, gin.H{"providerURL": "", "apiKey": ""})
			return
		}

		var config map[string]interface{}
		if err := yamlv3.Unmarshal(data, &config); err != nil {
			respondJSON(c, http.StatusOK, gin.H{"providerURL": "", "apiKey": "", "pathMappings": []interface{}{}})
			return
		}
		// Normalize YAML maps to ensure keys are strings so section lookup works
		config = normalizeYAML(config).(map[string]interface{})

		sectionData, mappings, mappingSet, pathMappings := parseSectionPathMappings(config, section)

		providerURL, apiKey := "", ""
		if sectionData != nil {
			providerURL, _ = sectionData["url"].(string)
			apiKey, _ = sectionData["apiKey"].(string)
		}

		// Attempt to fetch root folders and merge them into the returned
		// pathMappings synchronously so the UI sees merged folders immediately.
		// If the fetch fails we fall back to returning the stored mappings and
		// continue with a background merge attempt to persist any changes.
		pathMappings = tryMergeRemoteFolders(section, sectionData, providerURL, apiKey, pathMappings, mappings, mappingSet)

		TrailarrLog(DEBUG, "Settings", "Loaded settings for %s: URL=%s, APIKey=%s, Mappings=%v", section, providerURL, apiKey, pathMappings)
		respondJSON(c, http.StatusOK, gin.H{"providerURL": providerURL, "apiKey": apiKey, "pathMappings": pathMappings})
	}
}

// tryMergeRemoteFolders attempts a synchronous fetch/merge of remote root folders and
// ensures a background merge is scheduled regardless of success to persist updates later.
func tryMergeRemoteFolders(section string, sectionData map[string]interface{}, providerURL, apiKey string, pathMappings []map[string]interface{}, mappings []map[string]string, mappingSet map[string]bool) []map[string]interface{} {
	if sectionData == nil {
		return pathMappings
	}
	if folders, ferr := FetchRootFolders(providerURL, apiKey); ferr == nil {
		if merged, _, updated := mergeFoldersIntoMappings(pathMappings, mappings, mappingSet, folders); updated {
			// Update response payload with merged folders and schedule background persistence.
			pathMappings = merged
			go backgroundFetchAndMerge(section, providerURL, apiKey, pathMappings, mappings, mappingSet)
		}
	} else {
		TrailarrLog(DEBUG, "Settings", "Background rootfolder fetch failed for %s: %v", section, ferr)
		// Spawn background merge so we still attempt to merge later.
		go backgroundFetchAndMerge(section, providerURL, apiKey, pathMappings, mappings, mappingSet)
	}
	return pathMappings
}

// backgroundFetchAndMerge fetches root folders from the provider and merges any
// new folders into the provided path mappings. This was extracted out of the
// handler to reduce cognitive complexity of GetSettingsHandler.
func backgroundFetchAndMerge(section string, providerURL string, apiKey string, currentPathMappings []map[string]interface{}, mappings []map[string]string, mappingSet map[string]bool) {
	folders, err := FetchRootFolders(providerURL, apiKey)
	if err != nil {
		TrailarrLog(DEBUG, "Settings", "Background rootfolder fetch failed for %s: %v", section, err)
		return
	}
	mergedPathMappings, _, updated := mergeFoldersIntoMappings(currentPathMappings, mappings, mappingSet, folders)
	if !updated {
		return
	}
	cfg, rerr := readConfigFile()
	if rerr != nil {
		TrailarrLog(WARN, "Settings", "Background merge: failed to read config: %v", rerr)
		return
	}
	if secData, ok := cfg[section].(map[string]interface{}); ok {
		secData["pathMappings"] = mergedPathMappings
		cfg[section] = secData
		if werr := writeConfigFile(cfg); werr != nil {
			TrailarrLog(ERROR, "Settings", "Background merge: failed to write config: %v", werr)
		} else {
			TrailarrLog(INFO, "Settings", "Background merge: updated config with new root folders for %s", section)
		}
	}
}

// parseSectionPathMappings extracts section data and path mappings from a loaded config.
// Returns sectionData (may be nil), mappings (slice of {"from","to"}), mappingSet (set of from paths), and pathMappings (slice of map[string]interface{}).
func parseSectionPathMappings(config map[string]interface{}, section string) (map[string]interface{}, []map[string]string, map[string]bool, []map[string]interface{}) {
	sectionData, _ := config[section].(map[string]interface{})
	var mappings []map[string]string
	mappingSet := map[string]bool{}
	// Initialize slices so that an empty 'pathMappings' in the config YAML
	// results in an empty JSON array ([]) in responses instead of `null`.
	var pathMappings []map[string]interface{}
	// When the section exists we should return empty slices rather than nil
	// to make client expectations simpler (and match tests).
	if sectionData == nil {
		return sectionData, mappings, mappingSet, pathMappings
	}
	if pm, ok := sectionData["pathMappings"].([]interface{}); ok {
		// Ensure mappings and pathMappings are non-nil even if the list is empty
		mappings = []map[string]string{}
		pathMappings = []map[string]interface{}{}
		for _, m := range pm {
			if mMap, ok := m.(map[string]interface{}); ok {
				from := ""
				to := ""
				if v, ok := mMap["from"].(string); ok {
					from = v
				}
				if v, ok := mMap["to"].(string); ok {
					to = v
				}
				mappings = append(mappings, map[string]string{"from": from, "to": to})
				mappingSet[from] = true
				pathMappings = append(pathMappings, map[string]interface{}{"from": from, "to": to})
			}
		}
	}
	return sectionData, mappings, mappingSet, pathMappings
}

// mergeFoldersIntoMappings adds any unknown folders to the mappings and returns updated slices and a boolean flag if changes were made.
func mergeFoldersIntoMappings(pathMappings []map[string]interface{}, mappings []map[string]string, mappingSet map[string]bool, folders []map[string]interface{}) ([]map[string]interface{}, []map[string]string, bool) {
	updated := false
	for _, f := range folders {
		if path, ok := f["path"].(string); ok {
			if !mappingSet[path] {
				pathMappings = append(pathMappings, map[string]interface{}{"from": path, "to": ""})
				mappings = append(mappings, map[string]string{"from": path, "to": ""})
				updated = true
			}
		}
	}
	return pathMappings, mappings, updated
}

// Returns a Gin handler to save settings (providerURL, apiKey, pathMappings) for a given section ("radarr" or "sonarr")
func SaveSettingsHandler(section string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			ProviderURL  string `json:"providerURL" yaml:"url"`
			APIKey       string `json:"apiKey" yaml:"apiKey"`
			PathMappings []struct {
				From string `json:"from" yaml:"from"`
				To   string `json:"to" yaml:"to"`
			} `json:"pathMappings" yaml:"pathMappings"`
		}
		if err := c.BindJSON(&req); err != nil {
			respondError(c, http.StatusBadRequest, ErrInvalidRequest)
			return
		}
		config, err := readConfigFile()
		if err != nil {
			config = map[string]interface{}{}
		}
		sectionData := map[string]interface{}{
			"url":          req.ProviderURL,
			"apiKey":       req.APIKey,
			"pathMappings": req.PathMappings,
		}
		config[section] = sectionData

		// Persist config
		if err := writeConfigFile(config); err != nil {
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}

		// Update in-memory config
		if Config != nil {
			Config[section] = sectionData
		}

		// Trigger an immediate healthcheck task run so UI reflects new provider settings
		triggerHealthcheckTaskAsync()

		respondJSON(c, http.StatusOK, gin.H{"status": "saved"})
	}
}

// triggerHealthcheckTaskAsync runs the healthcheck task in the background if available.
func triggerHealthcheckTaskAsync() {
	go func() {
		if meta, ok := tasksMeta["healthcheck"]; ok && meta.Function != nil {
			// Run via runTaskAsync to get proper status updates persisted
			go runTaskAsync(meta.ID, meta.Function)
		}
	}()
}

func getGeneralSettingsHandler(c *gin.Context) {
	data, err := os.ReadFile(GetConfigPath())
	if err != nil {
		respondJSON(c, http.StatusOK, gin.H{"tmdbKey": "", "autoDownloadExtras": true, "frontendUrl": DefaultFrontendURL})
		return
	}
	var config map[string]interface{}
	if err := yamlv3.Unmarshal(data, &config); err != nil {
		respondJSON(c, http.StatusOK, gin.H{"tmdbKey": "", "autoDownloadExtras": true, "frontendUrl": DefaultFrontendURL})
		return
	}
	config = normalizeYAML(config).(map[string]interface{})
	var tmdbKey string
	var autoDownloadExtras bool = true
	var logLevel string = "Info"
	var frontendUrl string = DefaultFrontendURL
	if general, ok := config["general"].(map[string]interface{}); ok {
		if v, ok := general["tmdbKey"].(string); ok {
			tmdbKey = v
		}
		if v, ok := general["autoDownloadExtras"].(bool); ok {
			autoDownloadExtras = v
		}
		if v, ok := general["logLevel"].(string); ok {
			logLevel = v
		}
		if v, ok := general["frontendUrl"].(string); ok && v != "" {
			frontendUrl = strings.TrimRight(v, "/")
		}
	}
	respondJSON(c, http.StatusOK, gin.H{"tmdbKey": tmdbKey, "autoDownloadExtras": autoDownloadExtras, "logLevel": logLevel, "frontendUrl": frontendUrl})
}

func saveGeneralSettingsHandler(c *gin.Context) {
	var req struct {
		TMDBApiKey         string `json:"tmdbKey" yaml:"tmdbKey"`
		AutoDownloadExtras *bool  `json:"autoDownloadExtras" yaml:"autoDownloadExtras"`
		LogLevel           string `json:"logLevel" yaml:"logLevel"`
		FrontendUrl        string `json:"frontendUrl" yaml:"frontendUrl"`
	}
	// Read and decode JSON manually to avoid issues where Gin's BindJSON
	// may behave unexpectedly in some test environments. We still accept
	// the same field names (tmdbKey, autoDownloadExtras, logLevel).
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		respondError(c, http.StatusBadRequest, ErrInvalidRequest)
		return
	}
	// Restore Body for potential downstream readers
	c.Request.Body = io.NopCloser(bytes.NewBuffer(raw))
	if len(raw) == 0 {
		respondError(c, http.StatusBadRequest, ErrInvalidRequest)
		return
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		respondError(c, http.StatusBadRequest, ErrInvalidRequest)
		return
	}
	// (no-op) proceed to save parsed request
	// Debug logging to help diagnose CI failure where tmdbKey is not persisted.
	TrailarrLog(DEBUG, "Settings", "saveGeneralSettingsHandler parsed request: tmdbKey=%s autoDownloadExtras=%v logLevel=%s", req.TMDBApiKey, req.AutoDownloadExtras, req.LogLevel)
	// Read existing settings as map[string]interface{} to preserve all keys
	config, err := readConfigFile()
	if err != nil {
		config = map[string]interface{}{}
	}
	if config["general"] == nil {
		config["general"] = map[string]interface{}{}
	}
	general := config["general"].(map[string]interface{})
	general["tmdbKey"] = req.TMDBApiKey
	// var prevAutoDownload bool
	// if v, ok := general["autoDownloadExtras"].(bool); ok {
	// 	prevAutoDownload = v
	// } else {
	// 	prevAutoDownload = true
	// }
	// if req.AutoDownloadExtras != nil {
	// 	general["autoDownloadExtras"] = *req.AutoDownloadExtras
	// 	// Trigger start/stop of extras download task if changed
	// 	if *req.AutoDownloadExtras && !prevAutoDownload {
	// 		StartExtrasDownloadTask()
	// 	} else if !*req.AutoDownloadExtras && prevAutoDownload {
	// 		StopExtrasDownloadTask()
	// 	}
	// }
	if req.LogLevel != "" {
		general["logLevel"] = req.LogLevel
	}
	// Save frontendUrl if provided. Trim any trailing slash before persisting.
	if req.FrontendUrl != "" {
		general["frontendUrl"] = strings.TrimRight(req.FrontendUrl, "/")
	}
	config["general"] = general
	err = writeConfigFile(config)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Update in-memory config to reflect the values we just saved. Avoid
	// blindly re-reading from disk here because background goroutines may
	// concurrently modify the file and overwrite our changes; keeping the
	// in-memory representation consistent with the successful write ensures
	// immediate reads by other handlers return the values the client saved.
	Config = config

	respondJSON(c, http.StatusOK, gin.H{"status": "saved"})
}

// GetPlexConfig loads Plex config from config.yml
func GetPlexConfig() (PlexConfig, error) {
	data, err := os.ReadFile(GetConfigPath())
	if err != nil {
		TrailarrLog(WARN, "Settings", "Plex settings not found: %v", err)
		return PlexConfig{}, fmt.Errorf("settings not found: %w", err)
	}
	var allSettings map[string]interface{}
	if err := yamlv3.Unmarshal(data, &allSettings); err != nil {
		TrailarrLog(WARN, "Settings", "invalid settings: %v", err)
		return PlexConfig{}, fmt.Errorf("invalid settings: %w", err)
	}
	allSettings = normalizeYAML(allSettings).(map[string]interface{})
	secRaw, ok := allSettings["plex"]
	if !ok {
		TrailarrLog(WARN, "Settings", "plex section not found")
		return PlexConfig{}, fmt.Errorf("plex section not found")
	}
	sec, ok := secRaw.(map[string]interface{})
	if !ok {
		TrailarrLog(WARN, "Settings", ErrSectionNotMap, "plex")
		return PlexConfig{}, fmt.Errorf(ErrSectionNotMap, "plex")
	}
	cfg := PlexConfig{}
	if v, ok := sec["protocol"].(string); ok {
		cfg.Protocol = v
	}
	if v, ok := sec["ip"].(string); ok {
		cfg.IP = v
	}
	if v, ok := sec["port"]; ok {
		switch t := v.(type) {
		case float64:
			cfg.Port = int(t)
		case int:
			cfg.Port = t
		case int64:
			cfg.Port = int(t)
		}
	}
	if v, ok := sec["token"].(string); ok {
		cfg.Token = v
	}
	if v, ok := sec["clientId"].(string); ok {
		cfg.ClientId = v
	}
	if v, ok := sec["enabled"].(bool); ok {
		cfg.Enabled = v
	}
	return cfg, nil
}

// SavePlexConfig saves Plex config to config.yml
func SavePlexConfig(cfg PlexConfig) error {
	config, err := readConfigFile()
	if err != nil {
		config = map[string]interface{}{}
	}

	// Preserve existing token if the incoming cfg.Token is empty. This
	// prevents unintentionally deleting a stored token when the frontend
	// saves other Plex settings without returning the token value.
	existingToken := ""
	if secRaw, ok := config["plex"]; ok {
		switch sec := secRaw.(type) {
		case map[string]interface{}:
			if t, ok := sec["token"].(string); ok {
				existingToken = t
			}
		default:
			// Defensive: log unexpected type for debugging in tests
			TrailarrLog(DEBUG, "Settings", "SavePlexConfig: unexpected plex section type: %T", secRaw)
		}
	}
	tokenToSave := cfg.Token
	if tokenToSave == "" {
		tokenToSave = existingToken
	}

	// Preserve existing clientId if incoming cfg.ClientId is empty. This
	// prevents overwriting a generated clientId with an empty value when the
	// frontend doesn't include it in the payload.
	existingClientId := ""
	if sec, ok := config["plex"].(map[string]interface{}); ok {
		if c, ok := sec["clientId"].(string); ok {
			existingClientId = c
		}
	}
	clientIdToSave := cfg.ClientId
	if clientIdToSave == "" {
		clientIdToSave = existingClientId
	}

	config["plex"] = map[string]interface{}{
		"protocol": cfg.Protocol,
		"ip":       cfg.IP,
		"port":     cfg.Port,
		"token":    tokenToSave,
		"clientId": clientIdToSave,
		"enabled":  cfg.Enabled,
	}
	return writeConfigFile(config)
}

// Handler to get Plex config
func GetPlexConfigHandler(c *gin.Context) {
	cfg, err := GetPlexConfig()
	if err != nil {
		// Return defaults on error
		respondJSON(c, http.StatusOK, PlexConfig{
			Protocol: "http",
			IP:       "localhost",
			Port:     32400,
			Enabled:  false,
		})
		return
	}
	// Don't expose token in response for security
	cfg.Token = ""
	respondJSON(c, http.StatusOK, cfg)
}

// Handler to save Plex config
func SavePlexConfigHandler(c *gin.Context) {
	var req PlexConfig
	if err := c.BindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, ErrInvalidRequest)
		return
	}
	if err := SavePlexConfig(req); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(c, http.StatusOK, gin.H{"status": "saved"})
}

// GenerateUUID generates a random UUID-like string (16 hex bytes = 32 chars)
func generateUUID() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		// Fallback: generate a simple timestamp-based UUID if rand fails
		return fmt.Sprintf("%x-%x-%x-%x-%x", time.Now().UnixNano()&0xFFFFFFFF, time.Now().UnixNano()>>32&0xFFFF, time.Now().UnixNano()>>48&0xFFFF, time.Now().UnixNano()&0xFFFFFFFF, time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// PlexOAuthHeaders returns standard Plex OAuth headers
func plexOAuthHeaders(clientID string) map[string]string {
	return map[string]string{
		"Accept":                   "application/json",
		"X-Plex-Client-Identifier": clientID,
		"X-Plex-Product":           "Trailarr",
		"X-Plex-Version":           "1.0",
		"X-Plex-Platform":          "Linux",
		"X-Plex-Platform-Version":  "1.0",
		"X-Plex-Device":            "Server",
		"X-Plex-Device-Name":       "Trailarr",
		"X-Plex-Model":             "Plex OAuth",
		"Content-Type":             "application/json",
	}
}

// EnsurePlexClientID generates and stores a client ID if one doesn't exist
func EnsurePlexClientID() (string, error) {
	cfg, err := GetPlexConfig()
	if err != nil {
		// If config doesn't exist, create defaults
		cfg = PlexConfig{
			Protocol: "http",
			IP:       "localhost",
			Port:     32400,
			Enabled:  false,
		}
	}

	// If clientId exists and is not empty, return it
	if cfg.ClientId != "" {
		return cfg.ClientId, nil
	}

	// Generate new clientId
	cfg.ClientId = generateUUID()

	// Save updated config
	if err := SavePlexConfig(cfg); err != nil {
		TrailarrLog(ERROR, "Settings", "Failed to save Plex clientId: %v", err)
		return cfg.ClientId, err
	}

	TrailarrLog(INFO, "Settings", "Generated new Plex clientId: %s", cfg.ClientId)
	return cfg.ClientId, nil
}

// PlexOAuthResponse holds the OAuth flow information
type PlexOAuthResponse struct {
	LoginURL string `json:"loginUrl"`
	PinID    int64  `json:"pinId"`
	ClientID string `json:"clientId"`
	Code     string `json:"code"`
}

// GetPlexOAuthLoginURL initiates Plex OAuth device flow and returns login URL
func GetPlexOAuthLoginURL() (PlexOAuthResponse, error) {
	clientID, err := EnsurePlexClientID()
	if err != nil {
		TrailarrLog(ERROR, "Settings", "Failed to get/generate clientId: %v", err)
		return PlexOAuthResponse{}, err
	}

	// Request a PIN from Plex API
	pinReqBody := map[string]interface{}{
		"strong": true,
	}
	pinReqBodyJSON, _ := json.Marshal(pinReqBody)

	req, err := http.NewRequest("POST", "https://plex.tv/api/v2/pins", bytes.NewReader(pinReqBodyJSON))
	if err != nil {
		return PlexOAuthResponse{}, err
	}

	// Set headers
	headers := plexOAuthHeaders(clientID)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		TrailarrLog(ERROR, "Settings", "Failed to request Plex PIN: %v", err)
		return PlexOAuthResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		TrailarrLog(WARN, "Settings", "Plex API returned status %d", resp.StatusCode)
		return PlexOAuthResponse{}, fmt.Errorf("plex API returned status %d", resp.StatusCode)
	}

	var pinResp struct {
		ID   int64  `json:"id"`
		Code string `json:"code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pinResp); err != nil {
		TrailarrLog(ERROR, "Settings", "Failed to decode Plex PIN response: %v", err)
		return PlexOAuthResponse{}, err
	}

	// Build login URL with context params.
	// Prefer a configured value in config.yml (general.frontendUrl). If
	// not present, fall back to the FRONTEND_URL environment variable. If
	// neither is set, use the local dev server default.
	frontend := ""
	if cfg, err := readConfigFile(); err == nil {
		if gen, ok := cfg["general"].(map[string]interface{}); ok {
			if v, ok := gen["frontendUrl"].(string); ok && v != "" {
				frontend = v
			}
		}
	}
	if frontend == "" {
		frontend = os.Getenv("FRONTEND_URL")
	}
	if frontend == "" {
		frontend = DefaultFrontendURL
	}
	forwardUrl := fmt.Sprintf("%s/settings/plex", strings.TrimRight(frontend, "/"))

	loginURL := fmt.Sprintf(
		"https://app.plex.tv/auth/#!?clientID=%s&code=%s&forwardUrl=%s",
		clientID,
		pinResp.Code,
		forwardUrl,
	)

	TrailarrLog(INFO, "Settings", "Generated Plex OAuth login URL with code: %s", pinResp.Code)

	return PlexOAuthResponse{
		LoginURL: loginURL,
		PinID:    pinResp.ID,
		ClientID: clientID,
		Code:     pinResp.Code,
	}, nil
}

// ExchangePlexCodeForToken exchanges Plex OAuth code for auth token
func ExchangePlexCodeForToken(code string, pinID int64) (string, error) {
	clientID, err := EnsurePlexClientID()
	if err != nil {
		return "", err
	}

	// Poll Plex API to check if user authorized the PIN
	req, err := http.NewRequest("GET", fmt.Sprintf("https://plex.tv/api/v2/pins/%d", pinID), nil)
	if err != nil {
		return "", err
	}

	// Set headers
	headers := plexOAuthHeaders(clientID)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		TrailarrLog(ERROR, "Settings", "Failed to check PIN authorization: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		TrailarrLog(WARN, "Settings", "Plex PIN check returned status %d", resp.StatusCode)
		return "", fmt.Errorf("plex PIN check returned status %d", resp.StatusCode)
	}

	var pinCheckResp struct {
		AuthToken string `json:"authToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pinCheckResp); err != nil {
		TrailarrLog(ERROR, "Settings", "Failed to decode PIN check response: %v", err)
		return "", err
	}

	if pinCheckResp.AuthToken == "" {
		return "", fmt.Errorf("user has not authorized the request yet")
	}

	// Save token to Plex config
	// Ensure a clientId is present and persisted before saving the token.
	// This guarantees clientId is recorded in config.yml even if the
	// initial device flow step failed to persist it earlier.
	if _, cerr := EnsurePlexClientID(); cerr != nil {
		TrailarrLog(WARN, "Settings", "Failed to ensure Plex clientId before saving token: %v", cerr)
	}

	cfg, _ := GetPlexConfig()
	// Make sure cfg.ClientId contains the current clientID
	if cfg.ClientId == "" && clientID != "" {
		cfg.ClientId = clientID
	}
	cfg.Token = pinCheckResp.AuthToken
	if err := SavePlexConfig(cfg); err != nil {
		TrailarrLog(ERROR, "Settings", "Failed to save Plex auth token: %v", err)
		return "", err
	}

	TrailarrLog(INFO, "Settings", "Successfully obtained Plex auth token")
	return pinCheckResp.AuthToken, nil
}

// GetPlexOAuthLoginHandler handles GET /api/plex/login - initiates OAuth flow
func GetPlexOAuthLoginHandler(c *gin.Context) {
	oauthResp, err := GetPlexOAuthLoginURL()
	if err != nil {
		TrailarrLog(ERROR, "Settings", "Failed to get Plex OAuth login URL: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, oauthResp)
}

// ExchangePlexCodeHandler handles POST /api/plex/exchange - exchanges code for token
func ExchangePlexCodeHandler(c *gin.Context) {
	var req struct {
		Code  string `json:"code" binding:"required"`
		PinID int64  `json:"pinId" binding:"required"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing code or pinId"})
		return
	}

	token, err := ExchangePlexCodeForToken(req.Code, req.PinID)
	if err != nil {
		TrailarrLog(ERROR, "Settings", "Failed to exchange Plex code for token: %v", err)
		// If the user has not yet authorized the PIN, return 202 Accepted
		// so the frontend can poll until authorization completes instead of
		// treating the condition as an internal server error.
		if strings.Contains(strings.ToLower(err.Error()), "not authorized") || strings.Contains(strings.ToLower(err.Error()), "has not authorized") {
			c.JSON(http.StatusAccepted, gin.H{"status": "pending", "message": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token})
}

// Fetch root folders from Radarr or Sonarr API
func FetchRootFolders(apiURL, apiKey string) ([]map[string]interface{}, error) {
	req, err := http.NewRequest("GET", apiURL+"/api/v3/rootfolder", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(HeaderApiKey, apiKey)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		TrailarrLog(WARN, "Settings", ApiReturnedStatusFmt, resp.StatusCode)
		return nil, fmt.Errorf(ApiReturnedStatusFmt, resp.StatusCode)
	}
	var folders []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&folders); err != nil {
		return nil, err
	}
	// Only return root folder paths
	var rootFolderPaths []map[string]interface{}
	for _, folder := range folders {
		if path, ok := folder["path"].(string); ok {
			rootFolderPaths = append(rootFolderPaths, map[string]interface{}{"path": path})
		}
	}
	return rootFolderPaths, nil
}

// Test connection to Radarr/Sonarr by calling /api/v3/system/status
func testMediaConnection(providerURL, apiKey, _ string) error {
	endpoint := "/api/v3/system/status"
	req, err := http.NewRequest("GET", providerURL+endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set(HeaderApiKey, apiKey)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		TrailarrLog(WARN, "Settings", ApiReturnedStatusFmt, resp.StatusCode)
		return fmt.Errorf(ApiReturnedStatusFmt, resp.StatusCode)
	}
	return nil
}

// Returns a slice of canonical extra types enabled in config
func GetEnabledCanonicalExtraTypes(cfg ExtraTypesConfig) []string {
	types := make([]string, 0)
	if cfg.Trailers {
		types = append(types, canonicalizeExtraType("trailers"))
	}
	if cfg.Scenes {
		types = append(types, canonicalizeExtraType("scenes"))
	}
	if cfg.BehindTheScenes {
		types = append(types, canonicalizeExtraType("behindTheScenes"))
	}
	if cfg.Interviews {
		types = append(types, canonicalizeExtraType("interviews"))
	}
	if cfg.Featurettes {
		types = append(types, canonicalizeExtraType("featurettes"))
	}
	if cfg.DeletedScenes {
		types = append(types, canonicalizeExtraType("deletedScenes"))
	}
	if cfg.Other {
		types = append(types, canonicalizeExtraType("other"))
	}
	if len(types) == 0 {
		types = []string{canonicalizeExtraType("trailers")}
	}
	return types
}
