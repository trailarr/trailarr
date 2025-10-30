package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	yamlv3 "gopkg.in/yaml.v3"
)

// Trailarr uses a BoltDB-backed storage layer that exposes a store-compatible
// API to the rest of the codebase. An external store is no longer required —
// the code retains some legacy key names for compatibility but the default
// runtime storage is bbolt (on-disk) with an in-memory fallback used for tests.

const (
	ApiReturnedStatusFmt     = "API returned status %d"
	MoviesStoreKey           = "trailarr:movies"
	SeriesStoreKey           = "trailarr:series"
	MoviesWantedStoreKey     = "trailarr:movies:wanted"
	SeriesWantedStoreKey     = "trailarr:series:wanted"
	ExtrasStoreKey           = "trailarr:extras"
	RejectedExtrasStoreKey   = "trailarr:extras:rejected"
	DownloadQueue            = "trailarr:download_queue"
	TaskTimesStoreKey        = "trailarr:task_times"
	HealthIssuesStoreKey     = "trailarr:health_issues"
	HistoryStoreKey          = "trailarr:history"
	HistoryMaxLen            = 1000
	TaskQueueStoreKey        = "trailarr:task_queue"
	TaskQueueMaxLen          = 1000
	RemoteMediaCoverPath     = "/MediaCover/"
	HeaderApiKey             = "X-Api-Key"
	HeaderContentType        = "Content-Type"
	ErrInvalidSonarrSettings = "Invalid Sonarr settings"
	ErrInvalidRequest        = "invalid request"
)

// NOTE: store keys are defined below as the primary constants (use these names).

// Path and runtime-configurable variables. Tests may override these.
var (
	TrailarrRoot   = "/var/lib/trailarr"
	ConfigPath     = TrailarrRoot + "/config/config.yml"
	MediaCoverPath = TrailarrRoot + "/MediaCover"
	CookiesFile    = TrailarrRoot + "/.config/google-chrome/cookies.txt"
	LogsDir        = TrailarrRoot + "/logs"
)

// Global in-memory config
var Config map[string]interface{}

// LoadConfig reads config.yml into the global Config variable
func LoadConfig() error {
	data, err := os.ReadFile(ConfigPath)
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
	data, err := os.ReadFile(ConfigPath)
	if err != nil {
		return CanonicalizeExtraTypeConfig{Mapping: map[string]string{}}, err
	}
	var config map[string]interface{}
	_ = yamlv3.Unmarshal(data, &config)
	sec, ok := config["canonicalizeExtraType"].(map[string]interface{})
	cfg := CanonicalizeExtraTypeConfig{Mapping: map[string]string{}}
	if ok {
		if m, ok := sec["mapping"].(map[string]interface{}); ok {
			for k, v := range m {
				if s, ok := v.(string); ok {
					cfg.Mapping[k] = s
				}
			}
		}
	}
	return cfg, nil
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
	ytdlpFlags, ok := config["ytdlpFlags"].(map[string]interface{})
	if !ok {
		config["ytdlpFlags"] = DefaultYtdlpFlagsConfig()
		return true
	}
	if _, ok := ytdlpFlags["cookiesFromBrowser"]; !ok {
		ytdlpFlags["cookiesFromBrowser"] = "chrome"
		config["ytdlpFlags"] = ytdlpFlags
		return true
	}
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
	data, err := os.ReadFile(ConfigPath)
	if err != nil {
		return nil, err
	}
	var config map[string]interface{}
	if len(data) == 0 {
		// Treat empty file as missing config
		return nil, fmt.Errorf("empty config file")
	}
	err = yamlv3.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

// Helper to read config file and unmarshal into map[string]interface{}
func readConfigFile() (map[string]interface{}, error) {
	_ = EnsureConfigDefaults()
	data, err := os.ReadFile(ConfigPath)
	if err != nil {
		return nil, err
	}
	var config map[string]interface{}
	err = yamlv3.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

// Helper to write config map to file
func writeConfigFile(config map[string]interface{}) error {
	out, err := yamlv3.Marshal(config)
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigPath, out, 0644)
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
	data, err := os.ReadFile(ConfigPath)
	if err != nil {
		return DefaultYtdlpFlagsConfig(), err
	}
	var config map[string]interface{}
	if err := yamlv3.Unmarshal(data, &config); err != nil {
		return DefaultYtdlpFlagsConfig(), err
	}
	sec, ok := config["ytdlpFlags"].(map[string]interface{})
	cfg := DefaultYtdlpFlagsConfig()
	if !ok {
		return cfg, nil
	}

	applyYtdlpFlags(sec, &cfg)
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
		"quiet":              boolSetter(&cfg.Quiet),
		"noprogress":         boolSetter(&cfg.NoProgress),
		"cookiesFromBrowser": stringSetter(&cfg.CookiesFromBrowser),
		"writesubs":          boolSetter(&cfg.WriteSubs),
		"writeautosubs":      boolSetter(&cfg.WriteAutoSubs),
		"embedsubs":          boolSetter(&cfg.EmbedSubs),
		"sublangs":           stringSetter(&cfg.SubLangs),
		"requestedformats":   stringSetter(&cfg.RequestedFormats),
		"timeout":            floatSetter(&cfg.Timeout),
		"sleepInterval":      floatSetter(&cfg.SleepInterval),
		"maxDownloads":       intSetter(&cfg.MaxDownloads),
		"limitRate":          stringSetter(&cfg.LimitRate),
		"sleepRequests":      floatSetter(&cfg.SleepRequests),
		"maxSleepInterval":   floatSetter(&cfg.MaxSleepInterval),
	}
}

// Saves yt-dlp flags config to config.yml
func SaveYtdlpFlagsConfig(cfg YtdlpFlagsConfig) error {
	config, err := readConfigFile()
	if err != nil {
		config = map[string]interface{}{}
	}
	config["ytdlpFlags"] = map[string]interface{}{
		"quiet":              cfg.Quiet,
		"noprogress":         cfg.NoProgress,
		"writesubs":          cfg.WriteSubs,
		"writeautosubs":      cfg.WriteAutoSubs,
		"embedsubs":          cfg.EmbedSubs,
		"sublangs":           cfg.SubLangs,
		"requestedformats":   cfg.RequestedFormats,
		"timeout":            cfg.Timeout,
		"sleepInterval":      cfg.SleepInterval,
		"maxDownloads":       cfg.MaxDownloads,
		"limitRate":          cfg.LimitRate,
		"sleepRequests":      cfg.SleepRequests,
		"maxSleepInterval":   cfg.MaxSleepInterval,
		"cookiesFromBrowser": cfg.CookiesFromBrowser,
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
	if _, err := os.Stat(ConfigPath); os.IsNotExist(err) {
		return createConfigWithTimings(defaultTimings)
	}

	// Load existing config
	data, err := os.ReadFile(ConfigPath)
	if err != nil {
		return defaultTimings, err
	}
	var cfg map[string]interface{}
	if err := yamlv3.Unmarshal(data, &cfg); err != nil {
		return defaultTimings, err
	}

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
			_ = os.WriteFile(ConfigPath, out, 0644)
		}
	}

	// Ensure healthcheck key exists (6 hours). If missing, persist it so the
	// default schedule is present in config.yml.
	if _, hasHealth := timings["healthcheck"]; !hasHealth {
		timings["healthcheck"] = 360
		cfg["syncTimings"] = timings
		out, err := yamlv3.Marshal(cfg)
		if err == nil {
			_ = os.WriteFile(ConfigPath, out, 0644)
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
	if err := os.WriteFile(ConfigPath, out, 0644); err != nil {
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
	_ = os.WriteFile(ConfigPath, out, 0644)
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
	data, err := os.ReadFile(ConfigPath)
	if err != nil {
		TrailarrLog(WARN, "Settings", "settings not found: %v", err)
		return MediaSettings{}, fmt.Errorf("settings not found: %w", err)
	}
	var allSettings map[string]interface{}
	if err := yamlv3.Unmarshal(data, &allSettings); err != nil {
		TrailarrLog(WARN, "Settings", "invalid settings: %v", err)
		return MediaSettings{}, fmt.Errorf("invalid settings: %w", err)
	}
	secRaw, ok := allSettings[section]
	if !ok {
		TrailarrLog(WARN, "Settings", "section %s not found", section)
		return MediaSettings{}, fmt.Errorf("section %s not found", section)
	}
	sec, ok := secRaw.(map[string]interface{})
	if !ok {
		TrailarrLog(WARN, "Settings", "section %s is not a map", section)
		return MediaSettings{}, fmt.Errorf("section %s is not a map", section)
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
	data, err := os.ReadFile(ConfigPath)
	if err != nil {
		return nil, err
	}
	var config map[string]interface{}
	if err := yamlv3.Unmarshal(data, &config); err != nil {
		return nil, err
	}
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
	data, err := os.ReadFile(ConfigPath)
	if err != nil {
		return "", "", err
	}
	var config map[string]interface{}
	if err := yamlv3.Unmarshal(data, &config); err != nil {
		return "", "", err
	}
	sec, ok := config[provider].(map[string]interface{})
	if !ok {
		TrailarrLog(WARN, "Settings", "section %s not found in config", provider)
		return "", "", fmt.Errorf("section %s not found in config", provider)
	}
	providerURL, _ := sec["url"].(string)
	apiKey, _ := sec["apiKey"].(string)
	return providerURL, apiKey, nil
}

func GetSettingsHandler(section string) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := os.ReadFile(ConfigPath)
		if err != nil {
			respondJSON(c, http.StatusOK, gin.H{"providerURL": "", "apiKey": ""})
			return
		}

		var config map[string]interface{}
		if err := yamlv3.Unmarshal(data, &config); err != nil {
			respondJSON(c, http.StatusOK, gin.H{"providerURL": "", "apiKey": "", "pathMappings": []interface{}{}})
			return
		}

		sectionData, mappings, mappingSet, pathMappings := parseSectionPathMappings(config, section)

		providerURL, apiKey := "", ""
		if sectionData != nil {
			providerURL, _ = sectionData["url"].(string)
			apiKey, _ = sectionData["apiKey"].(string)
		}

		// Respond immediately with the stored settings so the UI can render fast.
		TrailarrLog(DEBUG, "Settings", "Loaded settings for %s: URL=%s, APIKey=%s, Mappings=%v", section, providerURL, apiKey, pathMappings)
		respondJSON(c, http.StatusOK, gin.H{"providerURL": providerURL, "apiKey": apiKey, "pathMappings": pathMappings})

		// Spawn a background goroutine to fetch root folders and merge them into
		// the config file if there are new folders. Doing this asynchronously
		// avoids blocking the HTTP response when the provider API is slow.
		if sectionData != nil {
			go func(section string, providerURL string, apiKey string, currentPathMappings []map[string]interface{}) {
				folders, err := FetchRootFolders(providerURL, apiKey)
				if err != nil {
					TrailarrLog(DEBUG, "Settings", "Background rootfolder fetch failed for %s: %v", section, err)
					return
				}
				mergedPathMappings, _, updated := mergeFoldersIntoMappings(currentPathMappings, mappings, mappingSet, folders)
				if updated {
					// Read current config and attempt to update section pathMappings.
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
			}(section, providerURL, apiKey, pathMappings)
		}
	}
}

// parseSectionPathMappings extracts section data and path mappings from a loaded config.
// Returns sectionData (may be nil), mappings (slice of {"from","to"}), mappingSet (set of from paths), and pathMappings (slice of map[string]interface{}).
func parseSectionPathMappings(config map[string]interface{}, section string) (map[string]interface{}, []map[string]string, map[string]bool, []map[string]interface{}) {
	sectionData, _ := config[section].(map[string]interface{})
	var mappings []map[string]string
	mappingSet := map[string]bool{}
	var pathMappings []map[string]interface{}
	if sectionData == nil {
		return sectionData, mappings, mappingSet, pathMappings
	}
	if pm, ok := sectionData["pathMappings"].([]interface{}); ok {
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
	data, err := os.ReadFile(ConfigPath)
	if err != nil {
		respondJSON(c, http.StatusOK, gin.H{"tmdbKey": "", "autoDownloadExtras": true})
		return
	}
	var config map[string]interface{}
	_ = yamlv3.Unmarshal(data, &config)
	var tmdbKey string
	var autoDownloadExtras bool = true
	var logLevel string = "Info"
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
	}
	respondJSON(c, http.StatusOK, gin.H{"tmdbKey": tmdbKey, "autoDownloadExtras": autoDownloadExtras, "logLevel": logLevel})
}

func saveGeneralSettingsHandler(c *gin.Context) {
	var req struct {
		TMDBApiKey         string `json:"tmdbKey" yaml:"tmdbKey"`
		AutoDownloadExtras *bool  `json:"autoDownloadExtras" yaml:"autoDownloadExtras"`
		LogLevel           string `json:"logLevel" yaml:"logLevel"`
	}
	if err := c.BindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, ErrInvalidRequest)
		return
	}
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
	config["general"] = general
	err = writeConfigFile(config)
	if err == nil {
		// Update in-memory config
		if Config != nil {
			Config["general"] = general
		}
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(c, http.StatusOK, gin.H{"status": "saved"})
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
