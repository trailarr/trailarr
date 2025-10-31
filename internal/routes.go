package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	iofs "io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	assets "trailarr/web"

	"github.com/gin-gonic/gin"
)

const indexHTMLFilename = "index.html"

// registerFaviconPNGRoutes serves /favicon-*.png from embedded distFS or filesystem fallback
func registerFaviconPNGRoutes(r *gin.Engine, distFS iofs.FS) {
	pngSizes := []string{"16x16", "32x32", "48x48", "64x64", "128x128", "256x256"}
	for _, size := range pngSizes {
		route := "/favicon-" + size + ".png"
		fileName := "favicon-" + size + ".png"
		r.GET(route, func(fileName string) gin.HandlerFunc {
			return func(c *gin.Context) {
				if distFS != nil {
					if data, err := iofs.ReadFile(distFS, fileName); err == nil {
						reader := bytes.NewReader(data)
						http.ServeContent(c.Writer, c.Request, fileName, time.Now(), reader)
						return
					}
				}
				c.File("./web/dist/" + fileName)
			}
		}(fileName))
	}
}

// registerFaviconRoute serves /favicon.ico from embedded distFS or falls back to filesystem
func registerFaviconRoute(r *gin.Engine, distFS iofs.FS) {
	r.GET("/favicon.ico", func(c *gin.Context) {
		if distFS != nil {
			if data, err := iofs.ReadFile(distFS, "favicon.ico"); err == nil {
				reader := bytes.NewReader(data)
				http.ServeContent(c.Writer, c.Request, "favicon.ico", time.Now(), reader)
				return
			}
		}
		c.File("./web/dist/favicon.ico")
	})
}

func RegisterRoutes(r *gin.Engine) {
	// Register grouped routes to keep this function small
	registerCastRoutes(r)
	registerYouTubeAndProxyRoutes(r)
	registerDownloadAndBlacklistRoutes(r)
	registerTaskWebSocketRoutes(r)
	registerLogAndTMDBRoutes(r)
	_ = EnsureYtdlpFlagsConfigExists()
	registerYtdlpRoutes(r)
	registerAPILogMiddleware(r)

	registerProviderAndTestRoutes(r)
	registerHealthAndTaskRoutes(r)

	// Serve embedded or filesystem static assets
	var distFS iofs.FS
	if s, err := iofs.Sub(assets.EmbeddedDist, "dist"); err == nil {
		distFS = s
		registerEmbeddedStaticRoutes(r, distFS)
		registerFaviconRoute(r, distFS)
		registerFaviconPNGRoutes(r, distFS)
	} else {
		// fallback to filesystem if embed not available
		r.Static("/assets", "./web/dist/assets")
		r.StaticFile("/", "./web/dist/index.html")
		r.GET("/favicon.ico", func(c *gin.Context) {
			c.File("./web/dist/favicon.ico")
		})
	}

	registerLogFileRoute(r)
	registerNoRouteHandler(r, distFS)

	// Static media and logo
	r.Static("/mediacover", MediaCoverPath)
	registerLogoRoute(r, distFS)

	registerMediaAndSettingsRoutes(r)

	// Extras and history endpoints
	r.POST("/api/extras/download", downloadExtraHandler)
	r.DELETE("/api/extras", deleteExtraHandler)
	r.GET("/api/extras/existing", existingExtrasHandler)
	r.GET("/api/history", historyHandler)

	// Extra types and canonicalize config endpoints
	r.GET("/api/settings/extratypes", GetExtraTypesConfigHandler)
	r.POST("/api/settings/extratypes", SaveExtraTypesConfigHandler)
	r.GET("/api/settings/canonicalizeextratype", GetCanonicalizeExtraTypeConfigHandler)
	r.POST("/api/settings/canonicalizeextratype", SaveCanonicalizeExtraTypeConfigHandler)

	// TMDB extra types endpoint
	r.GET("/api/tmdb/extratypes", func(c *gin.Context) {
		respondJSON(c, http.StatusOK, gin.H{"tmdbExtraTypes": TMDBExtraTypes})
	})

	// Server-side file browser for directory picker
	r.GET("/api/files/list", ListServerFoldersHandler)
}

func registerCastRoutes(r *gin.Engine) {
	r.GET("/api/movies/:id/cast", func(c *gin.Context) {
		idStr := c.Param("id")
		var id int
		fmt.Sscanf(idStr, "%d", &id)
		tmdbKey, err := GetTMDBKey()
		if err != nil {
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		tmdbId, err := GetTMDBId(MediaTypeMovie, id)
		if err != nil {
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		cast, err := FetchTMDBCast(MediaTypeMovie, tmdbId, tmdbKey)
		if err != nil {
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		respondJSON(c, http.StatusOK, gin.H{"cast": cast})
	})
	r.GET("/api/series/:id/cast", func(c *gin.Context) {
		idStr := c.Param("id")
		var id int
		fmt.Sscanf(idStr, "%d", &id)
		tmdbKey, err := GetTMDBKey()
		if err != nil {
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		tmdbId, err := GetTMDBId(MediaTypeTV, id)
		if err != nil {
			respondError(c, http.StatusInternalServerError,
				err.Error())
			return
		}
		cast, err := FetchTMDBCast(MediaTypeTV, tmdbId, tmdbKey)
		if err != nil {
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		respondJSON(c, http.StatusOK, gin.H{"cast": cast})
	})
}

func registerYouTubeAndProxyRoutes(r *gin.Engine) {
	r.POST("/api/youtube/search", YouTubeTrailerSearchHandler)
	r.GET("/api/youtube/search/stream", YouTubeTrailerSearchStreamHandler)
	r.GET("/api/proxy/youtube-image/:youtubeId", ProxyYouTubeImageHandler)
	r.HEAD("/api/proxy/youtube-image/:youtubeId", ProxyYouTubeImageHandler)
}

func registerDownloadAndBlacklistRoutes(r *gin.Engine) {
	// WebSocket for real-time download queue updates
	r.GET("/ws/download-queue", func(c *gin.Context) {
		wsUpgrader := getWebSocketUpgrader()
		conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			TrailarrLog(WARN, "WS", "WebSocket upgrade failed: %v", err)
			return
		}
		AddDownloadQueueClient(conn)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
		RemoveDownloadQueueClient(conn)
		conn.Close()
	})
	// Download status endpoints
	r.GET("/api/extras/status/:youtubeId", GetDownloadStatusHandler)
	r.POST("/api/extras/status/batch", GetBatchDownloadStatusHandler)
	// Start the download queue worker
	StartDownloadQueueWorker()
	r.GET("/api/blacklist/extras", BlacklistExtrasHandler)
	r.POST("/api/blacklist/extras/remove", RemoveBlacklistExtraHandler)
}

func registerTaskWebSocketRoutes(r *gin.Engine) {
	// WebSocket for real-time task status
	r.GET("/ws/tasks", func(c *gin.Context) {
		wsUpgrader := getWebSocketUpgrader()
		conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			TrailarrLog(WARN, "WS", "WebSocket upgrade failed: %v", err)
			return
		}
		addTaskStatusClient(conn)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
		removeTaskStatusClient(conn)
		conn.Close()
	})
}

func registerLogAndTMDBRoutes(r *gin.Engine) {
	r.GET("/api/logs/list", logsListHandler)
	r.GET("/api/test/tmdb", testTMDBHandler)
}

func logsListHandler(c *gin.Context) {
	entries, err := filepath.Glob(LogsDir + "/*.txt")
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	type LogInfo struct {
		Number    int    `json:"number"`
		Filename  string `json:"filename"`
		LastWrite string `json:"lastWrite"`
	}
	var logs []LogInfo
	for i, path := range entries {
		fi, err := os.Stat(path)
		if err != nil {
			continue
		}
		logs = append(logs, LogInfo{
			Number:    i + 1,
			Filename:  filepath.Base(path),
			LastWrite: fi.ModTime().Format("02 Jan 2006 15:04"),
		})
	}
	respondJSON(c, http.StatusOK, gin.H{"logs": logs, "logDir": LogsDir})
}

func testTMDBHandler(c *gin.Context) {
	apiKey := c.Query("apiKey")
	if apiKey == "" {
		respondError(c, http.StatusBadRequest, "Missing apiKey")
		return
	}
	testUrl := "https://api.themoviedb.org/3/configuration?api_key=" + apiKey
	resp, err := http.Get(testUrl)
	if err != nil {
		respondError(c, http.StatusOK, err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		respondJSON(c, http.StatusOK, gin.H{"success": true})
		return
	}
	var body struct {
		StatusMessage string `json:"status_message"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	msg := body.StatusMessage
	if msg == "" {
		msg = "Invalid TMDB API Key"
	}
	respondError(c, http.StatusOK, msg)
}

func registerYtdlpRoutes(r *gin.Engine) {
	r.GET("/api/settings/ytdlpflags", GetYtdlpFlagsConfigHandler)
	r.POST("/api/settings/ytdlpflags", SaveYtdlpFlagsConfigHandler)
}

func registerAPILogMiddleware(r *gin.Engine) {
	// Log all API calls except /mediacover
	r.Use(func(c *gin.Context) {
		// Omit logging for /mediacover and GET /api/tasks/queue
		if !(c.Request.Method == "GET" && c.Request.URL.Path == "/api/tasks/queue") &&
			(len(c.Request.URL.Path) < 11 || c.Request.URL.Path[:11] != "/mediacover") {
			TrailarrLog(INFO, "API", "%s %s", c.Request.Method, c.Request.URL.Path)
		}
		c.Next()
	})
}

func registerProviderAndTestRoutes(r *gin.Engine) {
	r.GET("/api/rootfolders", func(c *gin.Context) {
		providerURL := c.Query("providerURL")
		apiKey := c.Query("apiKey")
		if providerURL == "" || apiKey == "" {
			respondError(c, http.StatusBadRequest, "Missing providerURL or apiKey")
			return
		}
		folders, err := FetchRootFolders(providerURL, apiKey)
		if err != nil {
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		respondJSON(c, http.StatusOK, folders)
	})

	r.GET("/api/test/:provider", func(c *gin.Context) {
		provider := c.Param("provider")
		url := c.Query("url")
		apiKey := c.Query("apiKey")
		if url == "" || apiKey == "" {
			respondError(c, http.StatusBadRequest,
				"Missing url or apiKey")
			return
		}
		err := testMediaConnection(url, apiKey, provider)
		if err != nil {
			respondError(c, http.StatusOK, err.Error())
		} else {
			respondJSON(c, http.StatusOK, gin.H{"success": true})
		}
	})
}

func registerHealthAndTaskRoutes(r *gin.Engine) {
	// Health check
	r.GET("/api/health", func(c *gin.Context) {
		respondJSON(c, http.StatusOK, gin.H{"status": "ok"})
	})

	// Trigger an immediate healthcheck task run (used by UI test button)
	// This route runs the health check synchronously and returns whether it
	// succeeded (no issues) or failed (issues found / errors).
	r.POST("/api/health/execute", handleHealthExecute)

	// Trigger a provider-specific health check (radarr / sonarr)
	r.POST("/api/health/:id/execute", handleProviderHealthExecute)

	// System status for UI Status page
	r.GET("/api/system/status", SystemStatusHandler())

	// API endpoint for scheduled/queue status
	r.GET("/api/tasks/status", GetAllTasksStatus())
	r.GET("/api/tasks/queue", GetTaskQueueFileHandler())
	// Debug endpoint: raw store contents and count
	r.GET("/api/tasks/queue/debug", GetTaskQueueDebugHandler())
	r.POST("/api/tasks/force", TaskHandler())
}

// handleHealthExecute runs the health check synchronously and responds with success status.
func handleHealthExecute(c *gin.Context) {
	// Run the healthcheck synchronously so the caller knows the result
	runHealthCheckTask()
	// After running, inspect persisted issues: empty == success
	client := GetStoreClient()
	ctx := context.Background()
	vals, err := client.LRange(ctx, HealthIssuesStoreKey, 0, -1)
	if err != nil {
		respondJSON(c, http.StatusOK, gin.H{"success": false, "error": err.Error()})
		return
	}
	if len(vals) == 0 {
		respondJSON(c, http.StatusOK, gin.H{"success": true})
		return
	}
	respondJSON(c, http.StatusOK, gin.H{"success": false, "error": "issues found"})
}

// handleProviderHealthExecute runs a provider-specific connectivity test and updates persisted health issues.
func handleProviderHealthExecute(c *gin.Context) {
	id := c.Param("id")
	provider := strings.ToLower(id)
	if provider != "radarr" && provider != "sonarr" {
		respondError(c, http.StatusBadRequest, "Unknown provider")
		return
	}

	// Load provider settings
	url, apiKey, err := GetProviderUrlAndApiKey(provider)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if url == "" || apiKey == "" {
		respondError(c, http.StatusBadRequest, "Missing provider URL or API key")
		return
	}

	// Run a single provider connectivity test
	if err := testMediaConnection(url, apiKey, provider); err != nil {
		// Persist a health issue for this provider (append)
		persistProviderHealthIssue(provider, err)
		// Match existing pattern for test endpoints: return error text in 200 body
		respondError(c, http.StatusOK, fmt.Sprintf("%v", err))
		return
	}

	// Success: remove any persisted health issues for this provider so UI clears quickly
	clearProviderHealthIssues(provider)
	respondJSON(c, http.StatusOK, gin.H{"success": true})
}

// persistProviderHealthIssue appends a health issue for the provider while removing any prior entries for the same provider.
func persistProviderHealthIssue(provider string, terr error) {
	client := GetStoreClient()
	ctx := context.Background()
	hm := HealthMsg{
		Message: fmt.Sprintf("%s connectivity failed: %v", capitalize(provider), terr),
		Source:  capitalize(provider),
		Level:   "warning",
	}
	b, jerr := json.Marshal(hm)
	if jerr != nil {
		return
	}

	// Load existing entries and keep ones that are not from this provider
	vals, _ := client.LRange(ctx, HealthIssuesStoreKey, 0, -1)
	var keep []string
	for _, v := range vals {
		var ehm HealthMsg
		if err := json.Unmarshal([]byte(v), &ehm); err == nil {
			if strings.ToLower(ehm.Source) == provider {
				// skip existing entries for this provider
				continue
			}
		}
		keep = append(keep, v)
	}

	// Replace list with kept entries and append the new issue
	_ = client.Del(ctx, HealthIssuesStoreKey)
	for _, v := range keep {
		_ = client.RPush(ctx, HealthIssuesStoreKey, []byte(v))
	}
	_ = client.RPush(ctx, HealthIssuesStoreKey, b)
	_ = client.LTrim(ctx, HealthIssuesStoreKey, -100, -1)
}

// capitalize returns the input string with the first rune title-cased in a
// UTF-8 aware manner. We avoid using strings.Title which is deprecated.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}

// clearProviderHealthIssues removes any persisted health issues for the specified provider.
func clearProviderHealthIssues(provider string) {
	client := GetStoreClient()
	ctx := context.Background()
	vals, _ := client.LRange(ctx, HealthIssuesStoreKey, 0, -1)
	if len(vals) == 0 {
		return
	}
	var keep []string
	for _, v := range vals {
		var hm HealthMsg
		if err := json.Unmarshal([]byte(v), &hm); err == nil {
			if strings.ToLower(hm.Source) == provider {
				// drop entries for this provider
				continue
			}
		}
		keep = append(keep, v)
	}
	// Replace list with kept entries
	_ = client.Del(ctx, HealthIssuesStoreKey)
	for _, v := range keep {
		_ = client.RPush(ctx, HealthIssuesStoreKey, []byte(v))
	}
}

func registerEmbeddedStaticRoutes(r *gin.Engine, distFS iofs.FS) {
	// serve assets from embedded dist/assets
	r.GET("/assets/*filepath", func(c *gin.Context) {
		p := c.Param("filepath")
		// attempt to open file first
		if _, err := distFS.Open(filepath.Join("assets", p)); err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		buf, err := iofs.ReadFile(distFS, filepath.Join("assets", p))
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		reader := bytes.NewReader(buf)
		http.ServeContent(c.Writer, c.Request, p, time.Now(), reader)
	})
	// serve index.html at root
	r.GET("/", func(c *gin.Context) {
		data, err := iofs.ReadFile(distFS, indexHTMLFilename)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		reader := bytes.NewReader(data)
		http.ServeContent(c.Writer, c.Request, indexHTMLFilename, time.Now(), reader)
	})
}

func registerLogFileRoute(r *gin.Engine) {
	r.GET("/logs/:filename", func(c *gin.Context) {
		filename := c.Param("filename")
		filePath := LogsDir + "/" + filename
		// Security: only allow .txt files and prevent path traversal
		if len(filename) < 5 || filename[len(filename)-4:] != ".txt" || filename != filepath.Base(filename) {
			respondError(c, http.StatusBadRequest, "Invalid log filename")
			return
		}
		c.File(filePath)
	})
}

func registerNoRouteHandler(r *gin.Engine, distFS iofs.FS) {
	r.NoRoute(func(c *gin.Context) {
		TrailarrLog(INFO, "WEB", "NoRoute handler hit for path: %s", c.Request.URL.Path)
		// Serve index.html from embed if possible
		if distFS != nil {
			data, err := iofs.ReadFile(distFS, indexHTMLFilename)
			if err == nil {
				reader := bytes.NewReader(data)
				http.ServeContent(c.Writer, c.Request, indexHTMLFilename, time.Now(), reader)
				return
			}
		}
		c.File("./web/dist/index.html")
	})
}

func registerLogoRoute(r *gin.Engine, distFS iofs.FS) {
	r.GET("/logo.svg", func(c *gin.Context) {
		if distFS != nil {
			if data, err := iofs.ReadFile(distFS, "logo.svg"); err == nil {
				reader := bytes.NewReader(data)
				http.ServeContent(c.Writer, c.Request, "logo.svg", time.Now(), reader)
				return
			}
		}
		c.File("web/public/logo.svg")
	})
}

func registerMediaAndSettingsRoutes(r *gin.Engine) {
	// Helper for default media path
	// Group movies/series endpoints
	for _, media := range []struct {
		section        string
		cacheStoreKey  string
		wantedStoreKey string
		fallbackPath   string
		extrasType     MediaType
	}{
		{"movies", MoviesStoreKey, MoviesWantedStoreKey, "/Movies", MediaTypeMovie},
		{"series", SeriesStoreKey, SeriesWantedStoreKey, "/Series", MediaTypeTV},
	} {
		r.GET("/api/"+media.section, GetMediaHandler(media.cacheStoreKey, "id"))
		// Use the dedicated wanted-store key for fast-path reads; handler will
		// map back to the main cache when it needs to rebuild the index.
		r.GET("/api/"+media.section+"/wanted", GetMissingExtrasHandler(media.wantedStoreKey))
		r.GET("/api/"+media.section+"/:id", GetMediaByIdHandler(media.cacheStoreKey, "id"))
		r.GET("/api/"+media.section+"/:id/extras", sharedExtrasHandler(media.extrasType))
	}
	// Group settings endpoints for Radarr/Sonarr
	for _, provider := range []string{"radarr", "sonarr"} {
		r.GET("/api/settings/"+provider, GetSettingsHandler(provider))
		r.POST("/api/settings/"+provider, SaveSettingsHandler(provider))
	}
	// General settings (TMDB key)
	r.GET("/api/settings/general", getGeneralSettingsHandler)
	r.POST("/api/settings/general", saveGeneralSettingsHandler)
}
