package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

type MediaType string

const (
	MediaTypeMovie MediaType = "movie"
	MediaTypeTV    MediaType = "tv"
)

const (
	cacheControlHeader = "Cache-Control"
	cacheControlValue  = "public, max-age=86400"
	totalTimeLogFormat = "Total time: %v"
)

// SearchExtras merges extras from the main cache and the persistent extras collection for a media item
func SearchExtras(mediaType MediaType, mediaId int) ([]Extra, error) {
	ctx := context.Background()
	entries, err := GetExtrasForMedia(ctx, mediaType, mediaId)
	if err != nil {
		// fallback to old method if needed
		persistent, _ := GetAllExtras(ctx)
		entries = make([]ExtrasEntry, 0)
		for _, e := range persistent {
			if e.MediaType == mediaType && e.MediaId == mediaId {
				entries = append(entries, e)
			}
		}
	}
	result := make([]Extra, 0, len(entries))
	for _, e := range entries {
		result = append(result, Extra{
			ExtraType:  e.ExtraType,
			ExtraTitle: e.ExtraTitle,
			YoutubeId:  e.YoutubeId,
			Status:     e.Status,
		})
	}
	return result, nil
}

// ProxyYouTubeImageHandler proxies YouTube thumbnail images to avoid 404s and CORS issues
func ProxyYouTubeImageHandler(c *gin.Context) {
	youtubeId := c.Param("youtubeId")
	if youtubeId == "" {
		respondError(c, http.StatusBadRequest, "Missing youtubeId")
		return
	}

	cacheDir := filepath.Join(MediaCoverPath, "YouTube")
	ensureDirIfNeeded(MediaCoverPath, "MediaCoverPath")
	ensureDirIfNeeded(cacheDir, "cacheDir")

	if path, ct := cachedYouTubeImage(cacheDir, youtubeId); path != "" {
		serveCachedFile(c, path, ct)
		return
	}

	thumbUrls := []string{
		"https://i.ytimg.com/vi/" + youtubeId + "/maxresdefault.jpg",
		"https://i.ytimg.com/vi/" + youtubeId + "/hqdefault.jpg",
	}
	resp, err := fetchFirstSuccessful(thumbUrls)
	if err != nil || resp == nil {
		serveFallbackSVG(c)
		return
	}
	defer resp.Body.Close()

	ct := resp.Header.Get(HeaderContentType)
	ext := detectImageExt(ct)
	tmpPath := filepath.Join(cacheDir, youtubeId+".tmp")
	finalPath := filepath.Join(cacheDir, youtubeId+ext)

	if err := saveToTmp(resp.Body, tmpPath); err != nil {
		// couldn't cache; stream the response directly
		streamResponse(c, ct, resp.Body)
		return
	}
	_ = os.Rename(tmpPath, finalPath)
	serveCachedFile(c, finalPath, ct)
}

// helper: ensure directory exists but don't fail the whole handler
func ensureDirIfNeeded(path, context string) {
	if err := os.MkdirAll(path, 0775); err != nil {
		TrailarrLog(WARN, "ProxyYouTubeImageHandler", "Failed to create %s %s: %v", context, path, err)
	}
}

// helper: check cached files and return path + content type
func cachedYouTubeImage(cacheDir, id string) (string, string) {
	exts := []struct {
		ext string
		ct  string
	}{
		{".jpg", "image/jpeg"},
		{".jpeg", "image/jpeg"},
		{".png", "image/png"},
		{".webp", "image/webp"},
		{".svg", "image/svg+xml"},
	}
	for _, e := range exts {
		p := filepath.Join(cacheDir, id+e.ext)
		if _, err := os.Stat(p); err == nil {
			return p, e.ct
		}
	}
	return "", ""
}

func serveCachedFile(c *gin.Context, path, contentType string) {
	c.Header(HeaderContentType, contentType)
	c.Header(cacheControlHeader, cacheControlValue)
	if c.Request.Method == http.MethodHead {
		c.Status(http.StatusOK)
		return
	}
	c.File(path)
}

// helper: fetch the first successful response from candidate URLs
func fetchFirstSuccessful(urls []string) (*http.Response, error) {
	for _, u := range urls {
		resp, err := http.Get(u)
		if err != nil {
			if resp != nil {
				resp.Body.Close()
			}
			continue
		}
		if resp.StatusCode == 200 {
			return resp, nil
		}
		resp.Body.Close()
	}
	return nil, fmt.Errorf("no successful response")
}

func serveFallbackSVG(c *gin.Context) {
	svg := `<?xml version="1.0" encoding="UTF-8"?>
			<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 128" width="128" height="128" role="img" aria-label="Unavailable">
			<circle cx="64" cy="64" r="40" fill="none" stroke="#888" stroke-width="12" />
			<line x1="92" y1="36" x2="36" y2="92" stroke="#888" stroke-width="12" stroke-linecap="round" />
			</svg>`
	c.Header(HeaderContentType, "image/svg+xml")
	c.Header("X-Proxy-Fallback", "1")
	c.Header(cacheControlHeader, cacheControlValue)
	c.Status(http.StatusOK)
	if c.Request.Method == http.MethodHead {
		return
	}
	_, _ = c.Writer.Write([]byte(svg))
}

// helper: determine extension from content type
func detectImageExt(ct string) string {
	switch {
	case strings.Contains(ct, "jpeg"):
		return ".jpg"
	case strings.Contains(ct, "png"):
		return ".png"
	case strings.Contains(ct, "webp"):
		return ".webp"
	case strings.Contains(ct, "svg"):
		return ".svg"
	default:
		return ".jpg"
	}
}

// helper: save response body to tmp file
func saveToTmp(r io.Reader, path string) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, r)
	return err
}

func streamResponse(c *gin.Context, ct string, r io.Reader) {
	c.Header(HeaderContentType, ct)
	c.Header(cacheControlHeader, cacheControlValue)
	c.Status(http.StatusOK)
	if c.Request.Method == http.MethodHead {
		return
	}
	_, _ = io.Copy(c.Writer, r)
}

// Syncs media cache and caches poster images for Radarr/Sonarr
func SyncMedia(provider, apiPath, cacheFile string, filter func(map[string]interface{}) bool, posterDir string, posterSuffixes []string) error {
	// Minimal fast sync: fetch the list from provider, apply filter, save to cache.
	// Skip extras scanning, poster caching and new-item background processing to keep this fast.
	start := time.Now()

	allItems, err := fetchProviderItems(provider, apiPath)
	if err != nil {
		TrailarrLog(WARN, "SyncMedia", "Failed to fetch items from provider=%s apiPath=%s: %v", provider, apiPath, err)
		return err
	}

	filtered := make([]map[string]interface{}, 0, len(allItems))
	for _, m := range allItems {
		if filter == nil || filter(m) {
			filtered = append(filtered, m)
		}
	}
	prevItems, _ := loadCache(cacheFile)
	TrailarrLog(DEBUG, "SyncMedia", "Previous cache size for %s: %d", cacheFile, len(prevItems))

	// Save items to the appropriate backend
	if err := saveItems(cacheFile, filtered); err != nil {
		TrailarrLog(WARN, "SyncMedia", "Failed to save cache %s: %v", cacheFile, err)
		return err
	} else {
		TrailarrLog(DEBUG, "SyncMedia", "Saved %d items to %s", len(filtered), cacheFile)
	}
	// Cache poster images for the filtered items as part of sync (best-effort).
	// Run poster caching synchronously here so the cache is populated immediately.
	CacheMediaPosters(
		provider,
		posterDir,
		filtered,
		"id",
		posterSuffixes,
		true, // debug
	)

	// After syncing main cache, update wanted status in main JSON
	if err := updateWantedStatusInStore(cacheFile); err != nil {
		TrailarrLog(WARN, "SyncMediaCache", "updateWantedStatusInStore failed for %s: %v", cacheFile, err)
	} else {
		TrailarrLog(DEBUG, "SyncMediaCache", "updateWantedStatusInStore completed for %s", cacheFile)
	}

	// Handle new items (best-effort, background tasks)
	handleNewItems(provider, filtered, prevItems)
	TrailarrLog(DEBUG, "SyncMedia", "Triggered background processing for new items (provider=%s)", provider)

	TrailarrLog(INFO, "SyncMedia", "[Sync%s] Synced %d items to cache. duration=%v", provider, len(filtered), time.Since(start))
	return nil
}

// Generic handler for listing media (movies/series)
func GetMediaHandler(cacheFile, key string) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := loadCache(cacheFile)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "cache not found")
			return
		}
		idParam := c.Query("id")
		filtered := items
		if idParam != "" {
			filtered = Filter(items, func(m map[string]interface{}) bool {
				id, ok := m[key]
				return ok && fmt.Sprintf("%v", id) == idParam
			})
		}
		respondJSON(c, http.StatusOK, gin.H{"items": filtered})
	}
}

// parseMediaID parses an id from interface{} to int
func parseMediaID(id interface{}) (int, bool) {
	var idInt int
	switch v := id.(type) {
	case int:
		idInt = v
	case float64:
		idInt = int(v)
	case string:
		_, err := fmt.Sscanf(v, "%d", &idInt)
		if err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	return idInt, true
}

// Helper to fetch and cache poster image
func fetchAndCachePoster(localPath, posterUrl, section string) error {
	resp, err := http.Get(posterUrl)
	if err != nil || resp.StatusCode != 200 {
		if resp != nil {
			resp.Body.Close()
		}
		TrailarrLog(WARN, "CacheMediaPosters", "Failed to fetch poster image: %v", err)
		return fmt.Errorf("failed to fetch poster image from %s", section)
	}
	defer resp.Body.Close()
	out, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to cache poster image for %s", section)
	}
	_, _ = io.Copy(out, resp.Body)
	out.Close()
	return nil
}

// Parametrized poster caching for Radarr/Sonarr
func CacheMediaPosters(
	section string, // "radarr" or "sonarr"
	baseDir string, // e.g. MediaCoverPath + "Movies" or MediaCoverPath + "Series"
	idList []map[string]interface{}, // loaded cache
	idKey string, // "id"
	posterSuffixes []string, // ["/poster-500.jpg", "/fanart-1280.jpg"]
	debug bool, // enable debug output
) {
	TrailarrLog(INFO, "CacheMediaPosters", "Starting poster caching for section: %s, baseDir: %s, items: %d", section, baseDir, len(idList))

	// Load provider settings once
	settings, err := loadMediaSettings(section)
	if err != nil {
		TrailarrLog(WARN, "CacheMediaPosters", "Failed to load media settings for section=%s: %v", section, err)
		return
	}
	apiBase := trimTrailingSlash(settings.ProviderURL)

	// Build jobs
	jobsList := make([]posterJob, 0, len(idList)*len(posterSuffixes))
	for _, item := range idList {
		id := fmt.Sprintf("%v", item[idKey])
		idDir := baseDir + "/" + id
		for _, suffix := range posterSuffixes {
			localPath := idDir + suffix
			posterUrl := apiBase + RemoteMediaCoverPath + id + suffix
			jobsList = append(jobsList, posterJob{id, idDir, localPath, posterUrl})
		}
	}

	if len(jobsList) == 0 {
		return
	}

	// Run worker pool and process jobs
	maxWorkers := 8
	if len(jobsList) < maxWorkers {
		maxWorkers = len(jobsList)
	}
	success, failed := processPosterJobs(jobsList, maxWorkers, section)

	TrailarrLog(INFO, "CacheMediaPosters", "Finished poster caching for section=%s workers=%d jobs=%d success=%d failed=%d", section, maxWorkers, len(jobsList), success, failed)
}

// processPosterJobs runs a worker pool to process poster download jobs and returns success/failed counts
func processPosterJobs(jobsList []posterJob, maxWorkers int, section string) (int64, int64) {
	jobs := make(chan posterJob, len(jobsList))
	var wg sync.WaitGroup
	var success int64
	var failed int64

	for w := 0; w < maxWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for job := range jobs {
				_, err := handlePosterJob(job, section)
				if err != nil {
					atomic.AddInt64(&failed, 1)
					TrailarrLog(WARN, "CacheMediaPosters", "worker=%d failed to cache poster for %s id=%s: %v", workerID, section, job.id, err)
					continue
				}
				atomic.AddInt64(&success, 1)
			}
		}(w)
	}

	// Enqueue jobs
	for _, j := range jobsList {
		jobs <- j
	}
	close(jobs)
	wg.Wait()

	return atomic.LoadInt64(&success), atomic.LoadInt64(&failed)
}

// handlePosterJob performs the actual work for a single poster job.
// Returns (skipped, error) where skipped==true means the file already existed.
func handlePosterJob(job posterJob, section string) (bool, error) {
	// ensure directory exists
	if err := os.MkdirAll(job.idDir, 0775); err != nil {
		return false, fmt.Errorf("failed to create dir: %w", err)
	}
	// skip if already cached
	if _, err := os.Stat(job.localPath); err == nil {
		return true, nil
	}
	// download and cache
	if err := fetchAndCachePoster(job.localPath, job.posterUrl, section); err != nil {
		return false, err
	}
	return false, nil
}

// Finds the media path for a given id in a cache file
func FindMediaPathByID(cacheFile string, mediaId int) (string, error) {
	items, err := loadCache(cacheFile)
	if err != nil {
		return "", err
	}
	for _, item := range items {
		idInt, ok := parseMediaID(item["id"])
		if !ok {
			continue
		}
		if idInt == mediaId {
			if p, ok := item["path"].(string); ok {
				return p, nil
			}
			break
		}
	}
	return "", nil
}

// Common settings struct for both Radarr and Sonarr
// Use this for loading settings generically
type MediaSettings struct {
	ProviderURL string `yaml:"url"`
	APIKey      string `yaml:"apiKey"`
}

// posterJob represents a single poster download task used by CacheMediaPosters
type posterJob struct {
	id        string
	idDir     string
	localPath string
	posterUrl string
}

// Trims trailing slash from a URL
func trimTrailingSlash(url string) string {
	if strings.HasSuffix(url, "/") {
		return strings.TrimRight(url, "/")
	}
	return url
}

// Loads a JSON cache file into a generic slice
func loadCache(path string) ([]map[string]interface{}, error) {
	// Only support store-backed caches (bbolt) for media lists.
	if path == MoviesStoreKey || path == SeriesStoreKey {
		items, err := LoadMediaFromStore(path)
		if err != nil {
			return nil, err
		}
		return processLoadedItems(items, path), nil
	}
	return nil, fmt.Errorf("unsupported cache path %s; only store-backed caches are supported", path)
}

// Helper to apply path mappings and title map to loaded items, if applicable.
func processLoadedItems(items []map[string]interface{}, path string) []map[string]interface{} {
	mediaType, mainCachePath := detectMediaTypeAndMainCachePath(path)
	if mediaType == "" {
		return items
	}
	titleMap := getTitleMap(mainCachePath, path)
	mappings, err := GetPathMappings(mediaType)
	if err != nil {
		mappings = nil
	}
	// Debug: log incoming items and mapping/title map sizes for troubleshooting
	mappingsLen := 0
	if mappings != nil {
		mappingsLen = len(mappings)
	}
	TrailarrLog(DEBUG, "processLoadedItems", "path=%s mediaType=%v items=%d titleMap=%d mappings=%d", path, mediaType, len(items), len(titleMap), mappingsLen)
	for _, item := range items {
		updateItemPath(item, mappings)
		updateItemTitle(item, titleMap)
		// Do NOT attach extras from collection; extras are only in the extras collection now
	}
	TrailarrLog(DEBUG, "processLoadedItems", "processed %d items for path=%s", len(items), path)
	return items
}

// LoadMediaFromStore loads movies or series from the persistent store.
// Expects path to be MoviesStoreKey or SeriesStoreKey.
func LoadMediaFromStore(path string) ([]map[string]interface{}, error) {
	client := GetStoreClient()
	ctx := context.Background()
	var storeKey string
	switch path {
	case MoviesStoreKey:
		storeKey = "trailarr:movies"
	case SeriesStoreKey:
		storeKey = "trailarr:series"
	default:
		return nil, fmt.Errorf("unsupported path for bbolt: %s", path)
	}
	val, err := client.Get(ctx, storeKey)
	if err != nil {
		if err == ErrNotFound {
			return []map[string]interface{}{}, nil // treat as empty
		}
		return nil, err
	}
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(val), &items); err != nil {
		return nil, err
	}
	return items, nil
}

// SaveMediaToStore saves movies or series to the persistent store.
// Expects path to be MoviesStoreKey or SeriesStoreKey.
func SaveMediaToStore(path string, items []map[string]interface{}) error {
	client := GetStoreClient()
	ctx := context.Background()
	var storeKey string
	switch path {
	case MoviesStoreKey:
		storeKey = "trailarr:movies"
	case SeriesStoreKey:
		storeKey = "trailarr:series"
	default:
		return fmt.Errorf("unsupported path for bbolt: %s", path)
	}
	data, err := json.Marshal(items)
	if err != nil {
		return err
	}
	// Persist main store
	if err := client.Set(ctx, storeKey, data); err != nil {
		return err
	}
	// Invalidate any lightweight wanted index for this section so subsequent
	// reads will rebuild it from the authoritative main store. This ensures
	// tests and callers that directly manipulate the main cache see fresh
	// results even if an old wanted index exists in-memory or in the store.
	var wantedKey string
	switch path {
	case MoviesStoreKey:
		wantedKey = MoviesWantedStoreKey
	case SeriesStoreKey:
		wantedKey = SeriesWantedStoreKey
	}
	// Remove from in-memory cache
	wantedIndexMu.Lock()
	delete(wantedIndexMem, wantedKey)
	wantedIndexMu.Unlock()
	// Remove persisted lightweight index so loaders fall back to main store
	_ = client.Del(ctx, wantedKey)
	return nil
}

// Helper: Detect media type and main cache path
func detectMediaTypeAndMainCachePath(path string) (MediaType, string) {
	if strings.Contains(path, "movie") || strings.Contains(path, "Movie") {
		return MediaTypeMovie, MoviesStoreKey
	} else if strings.Contains(path, "series") || strings.Contains(path, "Series") {
		return MediaTypeTV, SeriesStoreKey
	}
	return "", ""
}

// Helper: Get title map from main cache if needed
func getTitleMap(mainCachePath, path string) map[string]string {
	if mainCachePath == "" || mainCachePath == path {
		return nil
	}
	titleMap := make(map[string]string)
	var mainItems []map[string]interface{}
	// Only support store-backed main caches for title mapping
	if mainCachePath == MoviesStoreKey || mainCachePath == SeriesStoreKey {
		mi, err := LoadMediaFromStore(mainCachePath)
		if err != nil {
			return nil
		}
		mainItems = mi
	} else {
		return nil
	}
	for _, item := range mainItems {
		if id, ok := item["id"]; ok {
			if title, ok := item["title"].(string); ok {
				titleMap[fmt.Sprintf("%v", id)] = title
			}
		}
	}
	return titleMap
}

// Helper: Update item path using mappings
func updateItemPath(item map[string]interface{}, mappings [][]string) {
	p, ok := item["path"].(string)
	if !ok || p == "" || mappings == nil {
		return
	}
	for _, m := range mappings {
		if strings.HasPrefix(p, m[0]) {
			item["path"] = m[1] + p[len(m[0]):]
			break
		}
	}
}

// Helper: Update item title using title map
func updateItemTitle(item map[string]interface{}, titleMap map[string]string) {
	if titleMap != nil {
		if id, ok := item["id"]; ok {
			if title, exists := titleMap[fmt.Sprintf("%v", id)]; exists {
				item["title"] = title
			}
		}
	} else if title, ok := item["title"].(string); ok {
		item["title"] = title
	}
}

// saveItems persists items either to the embedded store or to a file depending on cacheFile.
func saveItems(cacheFile string, items []map[string]interface{}) error {
	if cacheFile == MoviesStoreKey || cacheFile == SeriesStoreKey {
		return SaveMediaToStore(cacheFile, items)
	}
	return fmt.Errorf("unsupported cacheFile %s; only store-backed caches are supported", cacheFile)
}

// handleNewItems detects newly added items and triggers background processing for each.
func handleNewItems(provider string, items, prevItems []map[string]interface{}) {
	if len(prevItems) == 0 {
		return
	}
	prevIDs := make(map[int]struct{}, len(prevItems))
	for _, pi := range prevItems {
		if idRaw, ok := pi["id"]; ok {
			if idInt, ok2 := parseMediaID(idRaw); ok2 {
				prevIDs[idInt] = struct{}{}
			}
		}
	}

	mediaType := MediaTypeMovie
	if provider == "sonarr" {
		mediaType = MediaTypeTV
	}

	cfg, _ := GetExtraTypesConfig()

	for _, it := range items {
		idRaw, ok := it["id"]
		if !ok {
			continue
		}
		idInt, ok2 := parseMediaID(idRaw)
		if !ok2 {
			continue
		}
		if _, existed := prevIDs[idInt]; existed {
			continue
		}

		// New item detected — trigger TMDB search + enqueue downloads in background
		go processNewMediaExtras(mediaType, idInt, cfg)
	}
}

// processNewMediaExtras fetches TMDB extras, marks downloaded state and enqueues downloads according to config.
func processNewMediaExtras(mediaType MediaType, mediaID int, cfg interface{}) {
	TrailarrLog(INFO, "processNewMediaExtras", "New media detected, triggering extras search: mediaType=%v, id=%d", mediaType, mediaID)
	extras, err := FetchTMDBExtrasForMedia(mediaType, mediaID)
	if err != nil {
		TrailarrLog(WARN, "processNewMediaExtras", "Failed to fetch TMDB extras for mediaType=%v id=%d: %v", mediaType, mediaID, err)
		return
	}
	cacheFile, _ := resolveCachePath(mediaType)
	mediaPath, _ := FindMediaPathByID(cacheFile, mediaID)
	MarkDownloadedExtras(extras, mediaPath, "type", "title")

	// Ensure cfg is the expected ExtraTypesConfig type before calling filterAndDownloadExtras.
	// If it's not present or of wrong type, fall back to zero value (defaults).
	var etcfg ExtraTypesConfig
	if cfg != nil {
		if v, ok := cfg.(ExtraTypesConfig); ok {
			etcfg = v
		} else {
			TrailarrLog(WARN, "processNewMediaExtras", "Invalid extras config type; using defaults")
		}
	}
	filterAndDownloadExtras(mediaType, mediaID, extras, etcfg)
}

// Helper: fetch provider items and decode JSON, with logging preserved
func fetchProviderItems(provider, apiPath string) ([]map[string]interface{}, error) {
	providerURL, apiKey, err := GetProviderUrlAndApiKey(provider)
	if err != nil {
		return nil, fmt.Errorf("%s settings not found: %w", provider, err)
	}
	req, err := http.NewRequest("GET", providerURL+apiPath, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set(HeaderApiKey, apiKey)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error fetching %s: %w", provider, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		TrailarrLog(WARN, "fetchProviderItems", "%s API error: %d", provider, resp.StatusCode)
		return nil, fmt.Errorf("%s API error: %d", provider, resp.StatusCode)
	}
	var allItems []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&allItems); err != nil {
		return nil, fmt.Errorf("failed to decode %s response: %w", provider, err)
	}
	return allItems, nil
}

// Generic background sync for Radarr/Sonarr

// Returns a Gin handler to list media (movies/series) without any downloaded trailer extra
func GetMissingExtrasHandler(wantedPath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Fast-path: try to serve the lightweight wanted index.
		if idx, err := LoadWantedIndex(wantedPath); err == nil {
			TrailarrLog(DEBUG, "GetMissingExtrasHandler", "served %d items from wanted index for %s", len(idx), wantedPath)
			respondJSON(c, http.StatusOK, gin.H{"items": idx})
			return
		}

		// Fallback: load and compute from main store
		mainPath := resolveWantedMainPath(wantedPath)
		items, err := LoadMediaFromStore(mainPath)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "wanted cache not found")
			return
		}

		cfg, err := GetExtraTypesConfig()
		if err != nil {
			respondError(c, http.StatusInternalServerError, "failed to load extra types config")
			return
		}
		requiredTypes := GetEnabledCanonicalExtraTypes(cfg)
		TrailarrLog(INFO, "GetMissingExtrasHandler", "Required extra types: %v", requiredTypes)

		missing := filterWantedItems(items)
		light := make([]map[string]interface{}, 0, len(missing))
		for _, m := range missing {
			light = append(light, buildLightItem(m))
		}

		TrailarrLog(INFO, "GetMissingExtrasHandler", "Found %d items missing extras of types: %v", len(light), requiredTypes)
		respondJSON(c, http.StatusOK, gin.H{"items": light})
	}
}

// resolveWantedMainPath maps a wanted index key to its corresponding main cache path.
func resolveWantedMainPath(wantedPath string) string {
	switch wantedPath {
	case MoviesWantedStoreKey:
		return MoviesStoreKey
	case SeriesWantedStoreKey:
		return SeriesStoreKey
	default:
		return wantedPath
	}
}

// filterWantedItems returns only items explicitly marked wanted==true
func filterWantedItems(items []map[string]interface{}) []map[string]interface{} {
	return Filter(items, func(media map[string]interface{}) bool {
		return isMediaWanted(media)
	})
}

// helper: return true if media map has wanted=true (supports bool/string/number)
func isMediaWanted(media map[string]interface{}) bool {
	if w, ok := media["wanted"]; ok {
		switch v := w.(type) {
		case bool:
			return v
		case string:
			return strings.EqualFold(v, "true")
		case float64:
			return int(v) != 0
		case int:
			return v != 0
		default:
			return false
		}
	}
	return false
}

// helper: create a lightweight map for API responses from full media item
func buildLightItem(m map[string]interface{}) map[string]interface{} {
	lm := map[string]interface{}{}
	if id, ok := m["id"]; ok {
		lm["id"] = id
	}
	if t, ok := m["title"].(string); ok {
		lm["title"] = t
	}
	if st, ok := m["sortTitle"].(string); ok {
		lm["sortTitle"] = st
	}
	if y, ok := m["year"]; ok {
		lm["year"] = y
	}
	if ad, ok := m["airDate"]; ok {
		lm["airDate"] = ad
	}
	return lm
}

// SaveWantedIndex saves a lightweight wanted list for fast reads.
func SaveWantedIndex(cacheFile string, items []map[string]interface{}) error {
	client := GetStoreClient()
	ctx := context.Background()
	var storeKey string
	switch cacheFile {
	case MoviesStoreKey:
		storeKey = MoviesWantedStoreKey
	case SeriesStoreKey:
		storeKey = SeriesWantedStoreKey
	// allow caller to pass the wanted-store key directly
	case MoviesWantedStoreKey:
		storeKey = MoviesWantedStoreKey
	case SeriesWantedStoreKey:
		storeKey = SeriesWantedStoreKey
	default:
		return fmt.Errorf("unsupported cacheFile for wanted index: %s", cacheFile)
	}
	data, err := json.Marshal(items)
	if err != nil {
		return err
	}
	// update in-memory cache for immediate subsequent reads
	storeWantedIndexInMemory(storeKey, items)
	return client.Set(ctx, storeKey, data)
}

// In-memory cache for wanted index to avoid store reads under load.
var wantedIndexMu sync.RWMutex
var wantedIndexMem = map[string][]map[string]interface{}{}

func loadWantedIndexFromMemory(storeKey string) []map[string]interface{} {
	wantedIndexMu.RLock()
	defer wantedIndexMu.RUnlock()
	if v, ok := wantedIndexMem[storeKey]; ok {
		// Return a shallow copy to avoid accidental mutation by callers
		out := make([]map[string]interface{}, len(v))
		copy(out, v)
		return out
	}
	return nil
}

func storeWantedIndexInMemory(storeKey string, items []map[string]interface{}) {
	wantedIndexMu.Lock()
	defer wantedIndexMu.Unlock()
	// Store a shallow copy to decouple caller-owned slices
	out := make([]map[string]interface{}, len(items))
	copy(out, items)
	wantedIndexMem[storeKey] = out
}

// LoadWantedIndex loads the lightweight wanted list for the given cache path.
func LoadWantedIndex(cacheFile string) ([]map[string]interface{}, error) {
	client := GetStoreClient()
	ctx := context.Background()
	var storeKey string
	switch cacheFile {
	case MoviesStoreKey:
		storeKey = MoviesWantedStoreKey
	case SeriesStoreKey:
		storeKey = SeriesWantedStoreKey
	// allow callers to supply the wanted-store key directly
	case MoviesWantedStoreKey:
		storeKey = MoviesWantedStoreKey
	case SeriesWantedStoreKey:
		storeKey = SeriesWantedStoreKey
	default:
		return nil, fmt.Errorf("unsupported cacheFile for wanted index: %s", cacheFile)
	}
	// Fast in-memory cache check
	if items := loadWantedIndexFromMemory(storeKey); items != nil {
		return items, nil
	}
	val, err := client.Get(ctx, storeKey)
	if err != nil {
		return nil, err
	}
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(val), &items); err != nil {
		return nil, err
	}
	// populate in-memory cache for subsequent fast reads
	storeWantedIndexInMemory(storeKey, items)
	return items, nil
}

// sharedExtrasHandler handles extras for both movies and series
func sharedExtrasHandler(mediaType MediaType) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		var id int
		fmt.Sscanf(idStr, "%d", &id)

		// 1. Load persistent extras
		extras, err := SearchExtras(mediaType, id)
		if err != nil {
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}

		// 2. Load TMDB extras (best-effort)
		tmdbExtras, err := FetchTMDBExtrasForMedia(mediaType, id)
		if err != nil {
			TrailarrLog(WARN, "sharedExtrasHandler", "Failed to fetch TMDB extras: %v", err)
			tmdbExtras = nil
		}

		// 3. Merge sources with persistent taking precedence
		finalExtras := mergeExtrasPrioritizePersistent(extras, tmdbExtras)

		// 4. Mark downloaded extras
		cacheFile, _ := resolveCachePath(mediaType)
		mediaPath, err := FindMediaPathByID(cacheFile, id)
		if err != nil {
			respondError(c, http.StatusInternalServerError, fmt.Sprintf("%s cache not found", mediaType))
			return
		}
		MarkDownloadedExtras(finalExtras, mediaPath, "type", "title")

		// 5. Apply rejected extras (preserve reason and include missing rejected entries)
		rejectedExtras := GetRejectedExtrasForMedia(mediaType, id)
		TrailarrLog(DEBUG, "sharedExtrasHandler", "Rejected extras: %+v", rejectedExtras)
		finalExtras = applyRejectedExtras(finalExtras, rejectedExtras)

		respondJSON(c, http.StatusOK, gin.H{"extras": finalExtras})
	}
}

// mergeExtrasPrioritizePersistent merges persistent and TMDB extras using YoutubeId+ExtraType+ExtraTitle as key,
// giving priority to persistent entries when duplicates exist.
func mergeExtrasPrioritizePersistent(persistent, tmdb []Extra) []Extra {
	allMap := make(map[string]Extra)
	keyFor := func(e Extra) string {
		return e.YoutubeId + ":" + e.ExtraType + ":" + e.ExtraTitle
	}
	for _, e := range persistent {
		allMap[keyFor(e)] = e
	}
	for _, e := range tmdb {
		k := keyFor(e)
		if _, exists := allMap[k]; !exists {
			allMap[k] = e
		}
	}
	result := make([]Extra, 0, len(allMap))
	for _, e := range allMap {
		result = append(result, e)
	}
	return result
}

// applyRejectedExtras updates finalExtras with rejected statuses and includes rejected entries not already present.
func applyRejectedExtras(finalExtras []Extra, rejectedExtras []RejectedExtra) []Extra {
	youtubeInFinal := make(map[string]struct{}, len(finalExtras))
	for i := range finalExtras {
		youtubeInFinal[finalExtras[i].YoutubeId] = struct{}{}
	}

	// Map of youtubeId -> reason for quick lookup
	rejectedReason := make(map[string]string, len(rejectedExtras))
	for _, r := range rejectedExtras {
		rejectedReason[r.YoutubeId] = r.Reason
	}

	// Apply reasons to existing extras
	for i := range finalExtras {
		if reason, ok := rejectedReason[finalExtras[i].YoutubeId]; ok {
			finalExtras[i].Status = "rejected"
			finalExtras[i].Reason = reason
		}
	}

	// Append rejected extras that are not present in finalExtras
	for _, r := range rejectedExtras {
		if _, exists := youtubeInFinal[r.YoutubeId]; !exists {
			finalExtras = append(finalExtras, Extra{
				ExtraType:  r.ExtraType,
				ExtraTitle: r.ExtraTitle,
				YoutubeId:  r.YoutubeId,
				Status:     "rejected",
				Reason:     r.Reason,
			})
		}
	}
	return finalExtras
}

// respondError is a helper for Gin error responses
func respondError(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{"error": msg})
}

// respondJSON is a helper for Gin JSON responses
func respondJSON(c *gin.Context, code int, obj interface{}) {
	c.JSON(code, obj)
}

// Updates the main JSON file to mark items as wanted if they have no trailer
func updateWantedStatusInStore(cacheFile string) error {
	// Only support store-backed caches (bbolt) to avoid writing files.
	if cacheFile != MoviesStoreKey && cacheFile != SeriesStoreKey {
		return fmt.Errorf("updateWantedStatusInStore: unsupported cacheFile %s; only store-backed caches are supported", cacheFile)
	}

	// Load items directly from the store and apply runtime processing (path mappings/title updates)
	items, err := LoadMediaFromStore(cacheFile)
	if err != nil {
		return err
	}
	items = processLoadedItems(items, cacheFile)

	// Compute wanted flags on items and build the lightweight wanted index
	trailerCount, wantedLight := computeWantedIndexAndSetWants(items)

	TrailarrLog(INFO, "updateWantedStatusInStore", "processed %d items from %s, trailers found=%d", len(items), cacheFile, trailerCount)
	if err := SaveMediaToStore(cacheFile, items); err != nil {
		return err
	}

	if err := SaveWantedIndex(cacheFile, wantedLight); err != nil {
		TrailarrLog(WARN, "updateWantedStatusInStore", "failed to save wanted index for %s: %v", cacheFile, err)
	} else {
		TrailarrLog(DEBUG, "updateWantedStatusInStore", "saved wanted index for %s items=%d", cacheFile, len(wantedLight))
	}
	return nil
}

// computeWantedIndexAndSetWants iterates the provided items, sets the "wanted"
// flag based on presence of trailer files, logs a few debug lines and returns
// the number of trailer-containing items and the lightweight wanted index.
func computeWantedIndexAndSetWants(items []map[string]interface{}) (int, []map[string]interface{}) {
	trailerCount := 0
	logged := 0
	for _, item := range items {
		mediaId, ok := getMediaID(item)
		if !ok {
			item["wanted"] = false
			continue
		}
		mediaPath := ""
		if p, ok := item["path"].(string); ok {
			mediaPath = p
		}
		hasTrailer := hasTrailerFiles(mediaPath)
		item["wanted"] = !hasTrailer
		if hasTrailer {
			trailerCount++
		}
		if logged < 10 {
			TrailarrLog(DEBUG, "computeWantedIndexAndSetWants", "mediaId=%d mediaPath=%s hasTrailer=%v wanted=%v", mediaId, mediaPath, hasTrailer, item["wanted"])
			logged++
		}
	}

	wantedLight := make([]map[string]interface{}, 0, 32)
	for _, it := range items {
		if isMediaWanted(it) {
			wantedLight = append(wantedLight, buildLightItem(it))
		}
	}
	return trailerCount, wantedLight
}

// getMediaID extracts the integer media id from an item, supporting float64/int/string
func getMediaID(item map[string]interface{}) (int, bool) {
	id := item["id"]
	switch v := id.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	case string:
		var idInt int
		if _, err := fmt.Sscanf(v, "%d", &idInt); err == nil {
			return idInt, true
		}
		return 0, false
	default:
		return 0, false
	}
}

// hasTrailerInExtras returns true if any extra in the slice is a trailer (singular/plural or canonicalized)
// (removed: extras-based trailer detection — now relies on presence of .mkv files in Trailers folders)

// hasTrailerFiles checks for presence of .mkv files in Trailers or Trailer subdirectory of mediaPath.
func hasTrailerFiles(mediaPath string) bool {
	if mediaPath == "" {
		return false
	}
	// check both common directory names
	candidates := []string{filepath.Join(mediaPath, "Trailers"), filepath.Join(mediaPath, "Trailer")}
	for _, dir := range candidates {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := strings.ToLower(e.Name())
			if strings.HasSuffix(name, ".mkv") {
				return true
			}
		}
	}
	return false
}

// Handler to get a single media item by path parameter (e.g. /api/movies/:id)
func GetMediaByIdHandler(cacheFile, key string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		idParam := c.Param("id")
		TrailarrLog(DEBUG, "GetMediaByIdHandler", "HTTP %s %s, idParam: %s", c.Request.Method, c.Request.URL.String(), idParam)
		items, err := loadCache(cacheFile)
		if err != nil {
			TrailarrLog(DEBUG, "GetMediaByIdHandler", "Failed to load cache: %v", err)
			respondError(c, http.StatusInternalServerError, "cache not found")
			TrailarrLog(INFO, "GetMediaByIdHandler", totalTimeLogFormat, time.Since(start))
			return
		}
		filtered := Filter(items, func(m map[string]interface{}) bool {
			id, ok := m[key]
			return ok && fmt.Sprintf("%v", id) == idParam
		})
		TrailarrLog(DEBUG, "GetMediaByIdHandler", "Filtered by id=%s, %d items remain", idParam, len(filtered))
		if len(filtered) == 0 {
			respondError(c, http.StatusNotFound, "item not found")
			TrailarrLog(INFO, "GetMediaByIdHandler", totalTimeLogFormat, time.Since(start))
			return
		}
		TrailarrLog(DEBUG, "GetMediaByIdHandler", "Item: %+v", filtered[0])
		respondJSON(c, http.StatusOK, gin.H{"item": filtered[0]})
		TrailarrLog(INFO, "GetMediaByIdHandler", totalTimeLogFormat, time.Since(start))
	}
}

// Returns true if the media has any extras of the enabled types (case/plural robust)
func HasAnyEnabledExtras(mediaType MediaType, mediaId int, enabledTypes []string) bool {
	if hasPersistedDownloadedExtras(mediaType, mediaId, enabledTypes) {
		return true
	}
	if hasFilesystemExtras(mediaType, mediaId, enabledTypes) {
		return true
	}
	return false
}

// helper: check persisted extras for downloaded status matching enabled types
func hasPersistedDownloadedExtras(mediaType MediaType, mediaId int, enabledTypes []string) bool {
	extras, _ := SearchExtras(mediaType, mediaId)
	for _, e := range extras {
		if !strings.EqualFold(e.Status, "downloaded") {
			continue
		}
		for _, typ := range enabledTypes {
			if strings.EqualFold(e.ExtraType, typ) || strings.EqualFold(e.ExtraType+"s", typ) || strings.EqualFold(e.ExtraType, typ+"s") {
				return true
			}
		}
	}
	return false
}

// helper: scan filesystem for existing extras matching enabled types
func hasFilesystemExtras(mediaType MediaType, mediaId int, enabledTypes []string) bool {
	cacheFile, err := resolveCachePath(mediaType)
	if err != nil {
		return false
	}
	mediaPath, _ := FindMediaPathByID(cacheFile, mediaId)
	if mediaPath == "" {
		return false
	}
	existing := ScanExistingExtras(mediaPath)
	for key := range existing {
		parts := strings.SplitN(key, "|", 2)
		if len(parts) == 0 {
			continue
		}
		et := parts[0]
		for _, typ := range enabledTypes {
			if strings.EqualFold(et, typ) || strings.EqualFold(et+"s", typ) || strings.EqualFold(et, typ+"s") {
				return true
			}
		}
	}
	return false
}

// SyncMediaType syncs Radarr or Sonarr depending on mediaType
func SyncMediaType(mediaType MediaType) error {
	switch mediaType {
	case MediaTypeMovie:
		return SyncMedia(
			"radarr",
			"/api/v3/movie",
			MoviesStoreKey,
			func(m map[string]interface{}) bool {
				hasFile, ok := m["hasFile"].(bool)
				return ok && hasFile
			},
			MediaCoverPath+"/Movies",
			[]string{"/poster-500.jpg", "/fanart-1280.jpg"},
		)
	case MediaTypeTV:
		return SyncMedia(
			"sonarr",
			"/api/v3/series",
			SeriesStoreKey,
			func(m map[string]interface{}) bool {
				stats, ok := m["statistics"].(map[string]interface{})
				if !ok {
					return false
				}
				episodeFileCount, ok := stats["episodeFileCount"].(float64)
				return ok && episodeFileCount >= 1
			},
			MediaCoverPath+"/Series",
			[]string{"/poster-500.jpg", "/fanart-1280.jpg"},
		)
	default:
		return fmt.Errorf("unknown media type: %v", mediaType)
	}
}
