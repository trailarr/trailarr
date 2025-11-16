package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const extrasEntryKeyFmt = "%s:%s:%d"
const perMediaKeyFmt = "trailarr:extras:%s:%d"
const mkvJSONSuffix = ".mkv.json"
const rejectedIndexSaveErrFmt = "failed to save rejected index: %v"

// RemoveAll429Rejections removes all extras with status 'rejected' and reason containing '429' from the extras collection
func RemoveAll429Rejections() error {
	ctx := context.Background()
	client := GetStoreClient()
	vals, err := client.HVals(ctx, ExtrasStoreKey)
	if err != nil {
		return err
	}
	for _, v := range vals {
		var entry ExtrasEntry
		if err := json.Unmarshal([]byte(v), &entry); err == nil {
			if entry.Status == "rejected" && strings.Contains(entry.Reason, "429") {
				entryKey := fmt.Sprintf(extrasEntryKeyFmt, entry.YoutubeId, entry.MediaType, entry.MediaId)
				if err := client.HDel(ctx, ExtrasStoreKey, entryKey); err != nil {
					TrailarrLog(WARN, "Extras", "Failed to remove 429 rejected extra: %v", err)
				}
				// Also remove from per-media hash
				perMediaKey := fmt.Sprintf(extrasEntryKeyFmt, ExtrasStoreKey, entry.MediaType, entry.MediaId)
				_ = client.HDel(ctx, perMediaKey, entryKey)
			}
		}
	}
	// After bulk removals, refresh the rejected-index asynchronously.
	go func() {
		if err := SaveRejectedIndex(); err != nil {
			TrailarrLog(WARN, "RemoveAll429Rejections", rejectedIndexSaveErrFmt, err)
		}
	}()
	return nil
}

// GetExtrasForMedia efficiently returns all extras for a given mediaType and mediaId
func GetExtrasForMedia(ctx context.Context, mediaType MediaType, mediaId int) ([]ExtrasEntry, error) {
	client := GetStoreClient()
	perMediaKey := fmt.Sprintf(perMediaKeyFmt, mediaType, mediaId)

	vals, err := client.HVals(ctx, perMediaKey)
	if err != nil {
		return nil, err
	}

	parseEntries := func(raw []string) []ExtrasEntry {
		var out []ExtrasEntry
		for _, v := range raw {
			var entry ExtrasEntry
			if err := json.Unmarshal([]byte(v), &entry); err == nil {
				out = append(out, entry)
			}
		}
		return out
	}

	// Try per-media hash first
	result := parseEntries(vals)
	if len(result) > 0 {
		return result, nil
	}

	// Fallback: if nothing found, try global (legacy) and filter
	globalVals, err := client.HVals(ctx, ExtrasStoreKey)
	if err != nil {
		return nil, err
	}
	all := parseEntries(globalVals)
	var filtered []ExtrasEntry
	for _, entry := range all {
		if entry.MediaType == mediaType && entry.MediaId == mediaId {
			filtered = append(filtered, entry)
		}
	}
	return filtered, nil
}

// ListSubdirectories returns all subdirectories for a given path
func ListSubdirectories(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(path, entry.Name()))
		}
	}
	return dirs, nil
}

// Sanitize filename for OS conflicts (remove/replace invalid chars)
func SanitizeFilename(name string) string {
	// Remove any character not allowed in filenames
	// Windows: \/:*?"<>|, Linux: /
	re := regexp.MustCompile(`[\\/:*?"<>|]`)
	name = re.ReplaceAllString(name, "_")
	name = strings.TrimSpace(name)
	return name
}

// ExtrasEntry is the flat structure for each extra in the new collection
type ExtrasEntry struct {
	MediaType  MediaType `json:"mediaType"`
	MediaId    int       `json:"mediaId"`
	MediaTitle string    `json:"mediaTitle"`
	ExtraTitle string    `json:"extraTitle"`
	ExtraType  string    `json:"extraType"`
	FileName   string    `json:"fileName"`
	YoutubeId  string    `json:"youtubeId"`
	Status     string    `json:"status"`
	Reason     string    `json:"reason,omitempty"`
}

// MarkRejectedExtrasInMemory sets Status="rejected" for extras whose YoutubeId is in rejectedYoutubeIds (in-memory only)
func MarkRejectedExtrasInMemory(extras []Extra, rejectedYoutubeIds map[string]struct{}) {
	for i := range extras {
		if _, exists := rejectedYoutubeIds[extras[i].YoutubeId]; exists {
			extras[i].Status = "rejected"
		}
	}
}

// AddOrUpdateExtra stores or updates an extra in the unified collection
func AddOrUpdateExtra(ctx context.Context, entry ExtrasEntry) error {
	client := GetStoreClient()
	key := ExtrasStoreKey
	// Use YoutubeId+MediaType+MediaId as unique identifier
	entryKey := fmt.Sprintf(extrasEntryKeyFmt, entry.YoutubeId, entry.MediaType, entry.MediaId)
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	// Write to global hash
	if err := client.HSet(ctx, key, entryKey, data); err != nil {
		return err
	}
	// Write to per-media hash for fast lookup
	perMediaKey := fmt.Sprintf(perMediaKeyFmt, entry.MediaType, entry.MediaId)
	if err := client.HSet(ctx, perMediaKey, entryKey, data); err != nil {
		return err
	}
	return nil
}

// GetExtraByYoutubeId fetches an extra by YoutubeId, MediaType, and MediaId
func GetExtraByYoutubeId(ctx context.Context, youtubeId string, mediaType MediaType, mediaId int) (*ExtrasEntry, error) {
	client := GetStoreClient()
	key := ExtrasStoreKey
	entryKey := fmt.Sprintf(extrasEntryKeyFmt, youtubeId, mediaType, mediaId)
	val, err := client.HGet(ctx, key, entryKey)
	if err == ErrNotFound {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var entry ExtrasEntry
	if err := json.Unmarshal([]byte(val), &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

// GetAllExtras returns all extras in the collection
func loadTitles(cacheKey string) map[int]string {
	titles := make(map[int]string)
	// Use raw store-backed load to avoid processLoadedItems which may trigger
	// unwanted re-processing during simple title lookups.
	items, _ := LoadMediaFromStore(cacheKey)
	for _, m := range items {
		idInt, ok := parseMediaID(m["id"])
		if !ok {
			continue
		}
		if t, ok := m["title"].(string); ok {
			titles[idInt] = t
		}
	}
	return titles
}

func fillMediaTitle(entry *ExtrasEntry, movieTitles, seriesTitles map[int]string) {
	if entry.MediaTitle != "" {
		return
	}
	switch entry.MediaType {
	case MediaTypeMovie:
		if t, ok := movieTitles[entry.MediaId]; ok {
			entry.MediaTitle = t
		}
	case MediaTypeTV:
		if t, ok := seriesTitles[entry.MediaId]; ok {
			entry.MediaTitle = t
		}
	}
}

func GetAllExtras(ctx context.Context) ([]ExtrasEntry, error) {
	client := GetStoreClient()
	key := ExtrasStoreKey
	vals, err := client.HVals(ctx, key)
	if err != nil {
		return nil, err
	}
	var result []ExtrasEntry

	// Load movie and series title maps once
	movieTitles := loadTitles(MoviesStoreKey)
	seriesTitles := loadTitles(SeriesStoreKey)

	for _, v := range vals {
		var entry ExtrasEntry
		if err := json.Unmarshal([]byte(v), &entry); err != nil {
			continue
		}
		fillMediaTitle(&entry, movieTitles, seriesTitles)
		result = append(result, entry)
	}
	return result, nil
}

// RemoveExtra removes an extra from the collection
func RemoveExtra(ctx context.Context, youtubeId string, mediaType MediaType, mediaId int) error {
	client := GetStoreClient()
	key := ExtrasStoreKey
	entryKey := fmt.Sprintf(extrasEntryKeyFmt, youtubeId, mediaType, mediaId)
	// Remove from the global extras hash
	errGlobal := client.HDel(ctx, key, entryKey)
	// Also remove from the per-media hash for fast lookup
	perMediaKey := fmt.Sprintf(perMediaKeyFmt, mediaType, mediaId)
	errPerMedia := client.HDel(ctx, perMediaKey, entryKey)

	if errGlobal != nil {
		// If per-media deletion also failed, combine errors
		if errPerMedia != nil {
			return fmt.Errorf("global: %v; per-media: %v", errGlobal, errPerMedia)
		}
		return errGlobal
	}
	return errPerMedia
}

type Extra struct {
	ID         string
	ExtraType  string
	ExtraTitle string
	YoutubeId  string
	Status     string
	Reason     string
}

// GetRejectedExtrasForMedia returns rejected extras for a given media type and id, using the store cache
func GetRejectedExtrasForMedia(mediaType MediaType, id int) []RejectedExtra {
	// Use the lightweight rejected index (in-memory or store) when available
	if idx, err := LoadRejectedIndex(); err == nil {
		var rejected []RejectedExtra
		for _, e := range idx {
			if e.MediaType == mediaType && e.MediaId == id {
				rejected = append(rejected, RejectedExtra{
					MediaType:  e.MediaType,
					MediaId:    e.MediaId,
					ExtraType:  e.ExtraType,
					ExtraTitle: e.ExtraTitle,
					YoutubeId:  e.YoutubeId,
					Reason:     e.Reason,
				})
			}
		}
		return rejected
	}
	// Fallback: scan the full collection when index not available
	ctx := context.Background()
	extras, err := GetAllExtras(ctx)
	if err != nil {
		return nil
	}
	var rejected []RejectedExtra
	for _, e := range extras {
		if e.MediaType == mediaType && e.MediaId == id && e.Status == "rejected" {
			rejected = append(rejected, RejectedExtra{
				MediaType:  e.MediaType,
				MediaId:    e.MediaId,
				ExtraType:  e.ExtraType,
				ExtraTitle: e.ExtraTitle,
				YoutubeId:  e.YoutubeId,
				Reason:     e.Reason,
			})
		}
	}
	return rejected
}

// SetExtraRejectedPersistent sets the Status of an extra to "rejected" in the store, adding it if not present (persistent)
func SetExtraRejectedPersistent(mediaType MediaType, mediaId int, extraType, extraTitle, youtubeId, reason string) error {
	TrailarrLog(INFO, "SetExtraRejectedPersistent", "Attempting to mark rejected: mediaType=%s, mediaId=%d, extraType=%s, extraTitle=%s, youtubeId=%s, reason=%s", mediaType, mediaId, extraType, extraTitle, youtubeId, reason)
	ctx := context.Background()
	entry := ExtrasEntry{
		MediaType:  mediaType,
		MediaId:    mediaId,
		ExtraType:  extraType,
		ExtraTitle: extraTitle,
		YoutubeId:  youtubeId,
		Status:     "rejected",
		Reason:     reason,
	}
	if err := AddOrUpdateExtra(ctx, entry); err != nil {
		return err
	}
	// Update rejected-index async to avoid blocking the caller.
	go func() {
		if err := SaveRejectedIndex(); err != nil {
			TrailarrLog(WARN, "SetExtraRejectedPersistent", rejectedIndexSaveErrFmt, err)
		}
	}()
	return nil
}

// UnmarkExtraRejected clears the Status of an extra if it is "rejected" in the store, but keeps the extra in the array
func UnmarkExtraRejected(mediaType MediaType, mediaId int, extraType, extraTitle, youtubeId string) error {
	ctx := context.Background()
	if err := RemoveExtra(ctx, youtubeId, mediaType, mediaId); err != nil {
		return err
	}
	// Update rejected-index async
	go func() {
		if err := SaveRejectedIndex(); err != nil {
			TrailarrLog(WARN, "UnmarkExtraRejected", rejectedIndexSaveErrFmt, err)
		}
	}()
	return nil
}

// MarkExtraDownloaded sets the Status of an extra to "downloaded" in the store, if present
func MarkExtraDownloaded(mediaType MediaType, mediaId int, extraType, extraTitle, youtubeId string) error {
	ctx := context.Background()
	entry := ExtrasEntry{
		MediaType:  mediaType,
		MediaId:    mediaId,
		ExtraType:  extraType,
		ExtraTitle: extraTitle,
		YoutubeId:  youtubeId,
		Status:     "downloaded",
	}
	if err := AddOrUpdateExtra(ctx, entry); err != nil {
		return err
	}
	// Update rejected-index async in case this cleared a rejected flag
	go func() {
		if err := SaveRejectedIndex(); err != nil {
			TrailarrLog(WARN, "MarkExtraDownloaded", rejectedIndexSaveErrFmt, err)
		}
	}()
	return nil
}

// MarkExtraDeleted sets the Status of an extra to "deleted" in the store, if present (does not remove)
func MarkExtraDeleted(mediaType MediaType, mediaId int, extraType, extraTitle, youtubeId string) error {
	ctx := context.Background()
	entry := ExtrasEntry{
		MediaType:  mediaType,
		MediaId:    mediaId,
		ExtraType:  extraType,
		ExtraTitle: extraTitle,
		YoutubeId:  youtubeId,
		Status:     "deleted",
	}
	if err := AddOrUpdateExtra(ctx, entry); err != nil {
		return err
	}
	// Update rejected-index async in case this cleared a rejected flag
	go func() {
		if err := SaveRejectedIndex(); err != nil {
			TrailarrLog(WARN, "MarkExtraDeleted", rejectedIndexSaveErrFmt, err)
		}
	}()
	return nil
}

// Handler to delete an extra and record history
func deleteExtraHandler(c *gin.Context) {
	var req struct {
		MediaType MediaType `json:"mediaType"`
		MediaId   int       `json:"mediaId"`
		YoutubeId string    `json:"youtubeId"`
	}
	if err := c.BindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, ErrInvalidRequest)
		return
	}

	cacheFile, _ := resolveCachePath(req.MediaType)
	mediaPath, err := FindMediaPathByID(cacheFile, req.MediaId)
	if err != nil || mediaPath == "" {
		respondError(c, http.StatusNotFound, "Media not found")
		return
	}

	// Find the extra's extraType and extraTitle by YoutubeId from the unified collection
	ctx := context.Background()
	entry, err := GetExtraByYoutubeId(ctx, req.YoutubeId, req.MediaType, req.MediaId)
	if err != nil || entry == nil {
		respondError(c, http.StatusNotFound, "Extra not found in collection")
		return
	}
	// Try to delete files, but do not fail if missing
	_ = deleteExtraFiles(mediaPath, entry.ExtraType, entry.ExtraTitle)

	// Remove from the unified collection in the store
	if err := RemoveExtra(ctx, req.YoutubeId, req.MediaType, req.MediaId); err != nil {
		TrailarrLog(WARN, "Extras", "Failed to remove extra from store: %v", err)
	}

	recordDeleteHistory(req.MediaType, req.MediaId, entry.ExtraType, entry.ExtraTitle)
	respondJSON(c, http.StatusOK, gin.H{"status": "deleted"})
}

func resolveCachePath(mediaType MediaType) (string, error) {
	switch mediaType {
	case MediaTypeMovie:
		return MoviesStoreKey, nil
	case MediaTypeTV:
		return SeriesStoreKey, nil
	}
	return "", fmt.Errorf("unknown media type: %v", mediaType)
}

func lookupMediaTitle(cacheFile string, mediaId int) string {
	items, err := loadCache(cacheFile)
	if err != nil {
		return ""
	}
	for _, m := range items {
		idInt, ok := parseMediaID(m["id"])
		if ok && idInt == mediaId {
			// Use "title" for both movies and series
			if t, ok := m["title"].(string); ok {
				return t
			}
		}
	}
	return ""
}

func deleteExtraFiles(mediaPath, extraType, extraTitle string) error {
	extraDir := mediaPath + "/" + extraType
	extraFile := extraDir + "/" + SanitizeFilename(extraTitle) + ".mkv"
	metaFile := extraDir + "/" + SanitizeFilename(extraTitle) + mkvJSONSuffix
	err1 := os.Remove(extraFile)
	err2 := os.Remove(metaFile)
	if err1 != nil && err2 != nil {
		return fmt.Errorf("file error: %v, meta error: %v", err1, err2)
	}
	return nil
}

func recordDeleteHistory(mediaType MediaType, mediaId int, extraType, extraTitle string) {
	cacheFile, _ := resolveCachePath(mediaType)
	mediaTitle := lookupMediaTitle(cacheFile, mediaId)
	if mediaTitle == "" {
		panic(fmt.Errorf("recordDeleteHistory: could not find media title for mediaType=%v, mediaId=%v", mediaType, mediaId))
	}
	event := HistoryEvent{
		Action:     "delete",
		MediaTitle: mediaTitle,
		MediaType:  mediaType,
		MediaId:    mediaId,
		ExtraType:  extraType,
		ExtraTitle: extraTitle,
		Date:       time.Now(),
	}
	_ = AppendHistoryEvent(event)
}

func canonicalizeExtraType(extraType string) string {
	cfg, err := GetCanonicalizeExtraTypeConfig()
	if err == nil {
		if mapped, ok := cfg.Mapping[extraType]; ok {
			return mapped
		}
	}
	return extraType
}

// FetchTMDBExtrasForMedia fetches extras from TMDB for a given media item
func FetchTMDBExtrasForMedia(mediaType MediaType, id int) ([]Extra, error) {
	tmdbKey, err := GetTMDBKey()
	if err != nil {
		return nil, err
	}

	tmdbId, err := GetTMDBId(mediaType, id)
	if err != nil {
		return nil, err
	}

	extras, err := FetchTMDBExtras(mediaType, tmdbId, tmdbKey)
	if err != nil {
		return nil, err
	}

	// Canonicalize ExtraType for each extra before returning
	for i := range extras {
		extras[i].ExtraType = canonicalizeExtraType(extras[i].ExtraType)
	}
	return extras, nil
}

// Handler to list existing extras for a movie path
func collectExistingFromSubdir(subdir string, dupCount map[string]int) []map[string]interface{} {
	var results []map[string]interface{}
	dirName := filepath.Base(subdir)
	files, _ := os.ReadDir(subdir)
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".mkv") {
			continue
		}
		metaFile := filepath.Join(subdir, strings.TrimSuffix(f.Name(), ".mkv")+mkvJSONSuffix)
		var meta struct {
			ExtraType  string `json:"extraType"`
			ExtraTitle string `json:"extraTitle"`
			FileName   string `json:"fileName"`
			YoutubeId  string `json:"youtubeId"`
			Status     string `json:"status"`
		}
		status := "not-downloaded"
		if err := ReadJSONFile(metaFile, &meta); err == nil {
			status = meta.Status
			if status == "" {
				status = "downloaded"
			}
		}
		key := dirName + "|" + meta.ExtraTitle
		dupCount[key]++
		results = append(results, map[string]interface{}{
			"type":       dirName,
			"extraType":  meta.ExtraType,
			"extraTitle": meta.ExtraTitle,
			"fileName":   meta.FileName,
			"YoutubeId":  meta.YoutubeId,
			"_dupIndex":  dupCount[key],
			"status":     status,
		})
	}
	return results
}

func existingExtrasHandler(c *gin.Context) {
	moviePath := c.Query("moviePath")
	if moviePath == "" {
		respondError(c, http.StatusBadRequest, "moviePath required")
		return
	}
	// Scan subfolders for .mkv files and their metadata
	var existing []map[string]interface{}
	subdirs, err := ListSubdirectories(moviePath)
	if err != nil {
		respondJSON(c, http.StatusOK, gin.H{"existing": []map[string]interface{}{}})
		return
	}
	// Track duplicate index for each extraType/extraTitle
	dupCount := make(map[string]int)
	for _, subdir := range subdirs {
		existing = append(existing, collectExistingFromSubdir(subdir, dupCount)...)
	}
	respondJSON(c, http.StatusOK, gin.H{"existing": existing})
}

func downloadExtraHandler(c *gin.Context) {
	var req struct {
		MediaType  MediaType `json:"mediaType"`
		MediaId    int       `json:"mediaId"`
		ExtraType  string    `json:"extraType"`
		ExtraTitle string    `json:"extraTitle"`
		YoutubeId  string    `json:"youtubeId"`
	}
	if err := c.BindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, ErrInvalidRequest)
		return
	}
	TrailarrLog(INFO, "Extras", "[downloadExtraHandler] Download request: mediaType=%s, mediaId=%d, extraType=%s, extraTitle=%s, youtubeId=%s",
		req.MediaType, req.MediaId, req.ExtraType, req.ExtraTitle, req.YoutubeId)

	// Enqueue the download request
	item := DownloadQueueItem{
		MediaType:  req.MediaType,
		MediaId:    req.MediaId,
		ExtraType:  req.ExtraType,
		ExtraTitle: req.ExtraTitle,
		YouTubeID:  req.YoutubeId,
		QueuedAt:   time.Now(),
	}
	AddToDownloadQueue(item, "api")
	TrailarrLog(INFO, "Extras", "[downloadExtraHandler] Enqueued download: mediaType=%s, mediaId=%d, extraType=%s, extraTitle=%s, youtubeId=%s", req.MediaType, req.MediaId, req.ExtraType, req.ExtraTitle, req.YoutubeId)

	// Write .mkv.json meta file for manual download
	cacheFile, _ := resolveCachePath(req.MediaType)
	mediaPath, err := FindMediaPathByID(cacheFile, req.MediaId)
	if err == nil && mediaPath != "" {
		extraDir := mediaPath + "/" + req.ExtraType
		if err := os.MkdirAll(extraDir, 0775); err == nil {
			metaFile := extraDir + "/" + SanitizeFilename(req.ExtraTitle) + mkvJSONSuffix
			meta := struct {
				ExtraType  string `json:"extraType"`
				ExtraTitle string `json:"extraTitle"`
				FileName   string `json:"fileName"`
				YoutubeId  string `json:"youtubeId"`
				Status     string `json:"status"`
			}{
				ExtraType:  req.ExtraType,
				ExtraTitle: req.ExtraTitle,
				FileName:   SanitizeFilename(req.ExtraTitle) + ".mkv",
				YoutubeId:  req.YoutubeId,
				Status:     "queued",
			}
			// Use shared helper to write JSON with indentation
			_ = WriteJSONFile(metaFile, meta)
		}
	}
	respondJSON(c, http.StatusOK, gin.H{"status": "queued"})
}

// shouldDownloadExtra determines if an extra should be downloaded
func shouldDownloadExtra(extra Extra, config ExtraTypesConfig) bool {
	if extra.Status != "missing" || extra.YoutubeId == "" {
		return false
	}
	if extra.Status == "rejected" {
		return false
	}
	canonical := canonicalizeExtraType(extra.ExtraType)
	return isExtraTypeEnabled(config, canonical)
}

// handleExtraDownload downloads an extra unless it's rejected
func handleExtraDownload(mediaType MediaType, mediaId int, extra Extra) error {
	if extra.Status == "rejected" {
		TrailarrLog(INFO, "DownloadMissingExtras", "Skipping rejected extra: mediaType=%v, mediaId=%v, extraType=%s, extraTitle=%s, youtubeId=%s", mediaType, mediaId, extra.ExtraType, extra.ExtraTitle, extra.YoutubeId)
		return nil
	}
	// Enqueue the extra for download using the queue system
	item := DownloadQueueItem{
		MediaType:  mediaType,
		MediaId:    mediaId,
		ExtraType:  extra.ExtraType,
		ExtraTitle: extra.ExtraTitle,
		YouTubeID:  extra.YoutubeId,
		QueuedAt:   time.Now(),
	}
	AddToDownloadQueue(item, "task")
	TrailarrLog(INFO, "QUEUE", "[handleExtraDownload] Enqueued extra: mediaType=%v, mediaId=%v, extraType=%s, extraTitle=%s, youtubeId=%s", mediaType, mediaId, extra.ExtraType, extra.ExtraTitle, extra.YoutubeId)
	return nil
}

// Scans a media path and returns a map of existing extras (type|title)
func ScanExistingExtras(mediaPath string) map[string]bool {
	existing := map[string]bool{}
	if mediaPath == "" {
		return existing
	}
	subdirs, err := ListSubdirectories(mediaPath)
	if err != nil {
		return existing
	}
	for _, subdir := range subdirs {
		dirName := filepath.Base(subdir)
		files, _ := os.ReadDir(subdir)
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".mkv") {
				title := strings.TrimSuffix(f.Name(), ".mkv")
				key := dirName + "|" + title
				existing[key] = true
			}
		}
	}
	return existing
}

// Checks which extras are downloaded in the given media path and marks them in the extras list
// extras: slice of Extra (from TMDB), mediaPath: path to the movie/series folder
// typeKey: the key in the extra map for the type (usually "type"), titleKey: the key for the title (usually "title")
func MarkDownloadedExtras(extras []Extra, mediaPath string, typeKey, titleKey string) {
	existing := ScanExistingExtras(mediaPath)
	for i := range extras {
		typeStr := canonicalizeExtraType(extras[i].ExtraType)
		extras[i].ExtraType = typeStr
		title := SanitizeFilename(extras[i].ExtraTitle)
		extras[i].Status = "missing"

		// Iterate existing extras and match by sanitized title and by
		// canonicalized extra type using a tolerant comparison that
		// accepts singular/plural and case variations.
		for key := range existing {
			parts := strings.SplitN(key, "|", 2)
			if len(parts) != 2 {
				continue
			}
			existingType := parts[0]
			existingTitle := parts[1]
			if !strings.EqualFold(existingTitle, title) {
				continue
			}
			// tolerant type match: e.g. Trailer vs Trailers
			if strings.EqualFold(existingType, typeStr) || strings.EqualFold(existingType+"s", typeStr) || strings.EqualFold(existingType, typeStr+"s") {
				extras[i].Status = "downloaded"
				break
			}
		}
	}
}

// DownloadMissingExtras downloads missing extras for a given media type ("movie" or "tv")
func DownloadMissingExtras(mediaType MediaType, cacheFile string) error {
	TrailarrLog(INFO, "DownloadMissingExtras", "DownloadMissingExtras: mediaType=%s, cacheFile=%s", mediaType, cacheFile)

	items, err := loadCache(cacheFile)
	if err != nil {
		TrailarrLog(ERROR, "QUEUE", "[EXTRAS] Failed to load cache for %s: %v", mediaType, err)
		return err
	}
	type downloadItem struct {
		idInt     int
		mediaPath string
		extras    []Extra
	}
	config, _ := GetExtraTypesConfig()
	filtered := Filter(items, func(m map[string]interface{}) bool {
		idInt, ok := parseMediaID(m["id"])
		if !ok {
			TrailarrLog(WARN, "DownloadMissingExtras", "Missing or invalid id in item: %v", m)
			return false
		}
		_, err := FetchTMDBExtrasForMedia(mediaType, idInt)
		if err != nil {
			TrailarrLog(WARN, "DownloadMissingExtras", "SearchExtras error: %v", err)
			return false
		}
		mediaPath, err := FindMediaPathByID(cacheFile, idInt)
		if err != nil || mediaPath == "" {
			TrailarrLog(WARN, "DownloadMissingExtras", "FindMediaPathByID error or empty: %v, mediaPath=%s", err, mediaPath)
			return false
		}
		return true
	})
	mapped := Map(filtered, func(media map[string]interface{}) downloadItem {
		idInt, _ := parseMediaID(media["id"])
		extras, _ := FetchTMDBExtrasForMedia(mediaType, idInt)
		mediaPath, _ := FindMediaPathByID(cacheFile, idInt)
		MarkDownloadedExtras(extras, mediaPath, "type", "title")
		// Defensive: mark rejected extras before any download
		rejectedExtras := GetRejectedExtrasForMedia(mediaType, idInt)
		rejectedYoutubeIds := make(map[string]struct{})
		for _, r := range rejectedExtras {
			rejectedYoutubeIds[r.YoutubeId] = struct{}{}
		}
		MarkRejectedExtrasInMemory(extras, rejectedYoutubeIds)
		return downloadItem{idInt, mediaPath, extras}
	})
	for _, di := range mapped {
		filterAndDownloadExtras(mediaType, di.idInt, di.extras, config)
	}
	return nil
}

// filterAndDownloadExtras filters extras and downloads them if enabled
func filterAndDownloadExtras(mediaType MediaType, mediaId int, extras []Extra, config ExtraTypesConfig) {
	// Mark extras as rejected if their YouTube ID matches any in rejected_extras.json
	rejectedExtras := GetRejectedExtrasForMedia(mediaType, mediaId)
	rejectedYoutubeIds := make(map[string]struct{})
	for _, r := range rejectedExtras {
		rejectedYoutubeIds[r.YoutubeId] = struct{}{}
	}
	MarkRejectedExtrasInMemory(extras, rejectedYoutubeIds)
	// Filter extras according to config and status
	filtered := Filter(extras, func(extra Extra) bool {
		return shouldDownloadExtra(extra, config)
	})
	// Debug: log what will be processed
	if len(extras) == 0 {
		TrailarrLog(DEBUG, "Extras", "No extras found for mediaType=%v id=%d", mediaType, mediaId)
	} else {
		idsFiltered := make([]string, 0, len(filtered))
		for _, e := range filtered {
			idsFiltered = append(idsFiltered, e.YoutubeId)
		}
		// Only log when we will process some filtered extras
		if len(filtered) > 0 {
			TrailarrLog(INFO, "Extras", "Media %v:%d — will process %d extras: %v", mediaType, mediaId, len(filtered), idsFiltered)
		}
	}
	for _, extra := range filtered {
		TrailarrLog(INFO, "Extras", "Enqueuing extra for mediaType=%v id=%d youtubeId=%s title=%s", mediaType, mediaId, extra.YoutubeId, extra.ExtraTitle)
		err := handleExtraDownload(mediaType, mediaId, extra)
		if err != nil {
			TrailarrLog(WARN, "Extras", "Failed to download extra: %v", err)
		}
	}
}

func canonicalizeMeta(meta map[string]interface{}) map[string]interface{} {
	canonical := make(map[string]interface{})
	for k, v := range meta {
		switch strings.ToLower(k) {
		case "title", "extratitle":
			canonical["Title"] = v
		case "filename", "fileName":
			canonical["FileName"] = v
		case "youtubeid":
			canonical["YoutubeId"] = v
		case "status":
			canonical["Status"] = v
		default:
			canonical[k] = v
		}
	}
	return canonical
}

func scanExtrasInfo(mediaPath string) map[string][]map[string]interface{} {
	extrasInfo := make(map[string][]map[string]interface{})
	if mediaPath == "" {
		return extrasInfo
	}
	subdirs, err := ListSubdirectories(mediaPath)
	if err != nil {
		return extrasInfo
	}
	for _, subdir := range subdirs {
		extraType := filepath.Base(subdir)
		files, _ := os.ReadDir(subdir)
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			name := f.Name()
			if !strings.HasSuffix(name, mkvJSONSuffix) {
				continue
			}
			filePath := filepath.Join(subdir, name)
			var meta map[string]interface{}
			if err := ReadJSONFile(filePath, &meta); err == nil {
				extrasInfo[extraType] = append(extrasInfo[extraType], canonicalizeMeta(meta))
			}
		}
	}
	return extrasInfo
}

// In-memory cache for the rejected extras index to avoid store reads under load.
var rejectedIndexMu sync.RWMutex
var rejectedIndexMem []ExtrasEntry

func loadRejectedIndexFromMemory() []ExtrasEntry {
	rejectedIndexMu.RLock()
	defer rejectedIndexMu.RUnlock()
	if rejectedIndexMem == nil {
		return nil
	}
	out := make([]ExtrasEntry, len(rejectedIndexMem))
	copy(out, rejectedIndexMem)
	return out
}

func storeRejectedIndexInMemory(items []ExtrasEntry) {
	rejectedIndexMu.Lock()
	defer rejectedIndexMu.Unlock()
	out := make([]ExtrasEntry, len(items))
	copy(out, items)
	rejectedIndexMem = out
}

// SaveRejectedIndex builds a lightweight list of rejected extras and persists it
// to the store so the blacklist handler can serve it quickly.
func SaveRejectedIndex() error {
	ctx := context.Background()
	extras, err := GetAllExtras(ctx)
	if err != nil {
		return err
	}
	var rejected []ExtrasEntry
	for _, e := range extras {
		if e.Status == "rejected" {
			rejected = append(rejected, e)
		}
	}
	data, err := json.Marshal(rejected)
	if err != nil {
		return err
	}
	client := GetStoreClient()
	if err := client.Set(ctx, RejectedExtrasStoreKey, data); err != nil {
		return err
	}
	storeRejectedIndexInMemory(rejected)
	return nil
}

// LoadRejectedIndex attempts to load the rejected extras index from the
// in-memory cache first, then the store. Returns ErrNotFound-style errors as
// returned by the underlying store Get.
func LoadRejectedIndex() ([]ExtrasEntry, error) {
	if items := loadRejectedIndexFromMemory(); items != nil {
		return items, nil
	}
	client := GetStoreClient()
	ctx := context.Background()
	val, err := client.Get(ctx, RejectedExtrasStoreKey)
	if err != nil {
		return nil, err
	}
	var items []ExtrasEntry
	if err := json.Unmarshal([]byte(val), &items); err != nil {
		return nil, err
	}
	storeRejectedIndexInMemory(items)
	return items, nil
}

// Handler for serving the rejected extras blacklist
// BlacklistExtrasHandler aggregates all rejected extras from the persistent store for both movies and series
func BlacklistExtrasHandler(c *gin.Context) {
	// Fast-path: try to load a precomputed rejected-index from the store or
	// in-memory cache. If it's available, return it immediately to avoid
	// scanning and unmarshalling the entire extras collection on every
	// request. Fall back to full scan if the index is missing or errors.
	if idx, err := LoadRejectedIndex(); err == nil {
		TrailarrLog(DEBUG, "BlacklistExtrasHandler", "served %d items from rejected index", len(idx))
		respondJSON(c, http.StatusOK, idx)
		return
	}

	// Fallback to full scan when index is not available
	ctx := context.Background()
	extras, err := GetAllExtras(ctx)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "Failed to fetch extras: "+err.Error())
		return
	}
	var rejected []ExtrasEntry
	for _, extra := range extras {
		if extra.Status == "rejected" {
			rejected = append(rejected, extra)
		}
	}
	if rejected == nil {
		rejected = make([]ExtrasEntry, 0)
	}
	respondJSON(c, http.StatusOK, rejected)
}

// Handler to remove an entry from the rejected extras blacklist
func RemoveBlacklistExtraHandler(c *gin.Context) {
	var req struct {
		MediaType  string `json:"mediaType"`
		MediaId    int    `json:"mediaId"`
		ExtraType  string `json:"extraType"`
		ExtraTitle string `json:"extraTitle"`
		YoutubeId  string `json:"youtubeId"`
	}
	if err := c.BindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, ErrInvalidRequest)
		return
	}
	var mt MediaType
	switch req.MediaType {
	case string(MediaTypeMovie):
		mt = MediaTypeMovie
	case string(MediaTypeTV):
		mt = MediaTypeTV
	default:
		respondError(c, http.StatusBadRequest, "Invalid mediaType")
		return
	}
	// Use UnmarkExtraRejected which removes the extra and triggers an
	// asynchronous SaveRejectedIndex to keep the lightweight rejected index
	// in sync with the unified extras collection.
	if err := UnmarkExtraRejected(mt, req.MediaId, req.ExtraType, req.ExtraTitle, req.YoutubeId); err != nil {
		respondError(c, http.StatusInternalServerError, "Could not remove extra from collection: "+err.Error())
		return
	}
	respondJSON(c, http.StatusOK, gin.H{"status": "removed"})
}
