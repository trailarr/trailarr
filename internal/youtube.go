package internal

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Package-level constants and variables. Keep these together so tests can
// override timings or swap implementations (eg. fakeRunner) in TestMain.
const (
	ytDlpSkipDownload = "--skip-download"
	ytDlpSearchPrefix = "ytsearch10:"
	YtDlpCmd          = "yt-dlp"
)

var (
	// Test hooks: when true, yt-dlp calls are mocked/simulated for faster tests.
	YtDlpTestMode = false

	// Configurable delays (can be shortened during tests)
	QueueItemRemoveDelay = 10 * time.Second
	QueuePollInterval    = 2 * time.Second

	// Additional configurable timings used across the package. Tests can shorten these.
	DownloadQueueWatcherInterval    = 1 * time.Second
	TooManyRequestsPauseDuration    = 5 * time.Minute
	TooManyRequestsPauseLogInterval = 30 * time.Second
	TasksDepsWaitInterval           = 5 * time.Second

	// runtime state (moved here for tidy grouping)
	downloadStatusMap = make(map[string]*DownloadStatus) // keyed by YouTubeID
	queueMutex        sync.Mutex
)

// YouTube trailer search SSE handler (progressive results)
func YouTubeTrailerSearchStreamHandler(c *gin.Context) {
	c.Writer.Header().Set(HeaderContentType, "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Flush()

	mediaType := c.Query("mediaType")
	mediaIdStr := c.Query("mediaId")
	if mediaType == "" || mediaIdStr == "" {
		TrailarrLog(WARN, "YouTube", "Missing mediaType or mediaId in query params (SSE)")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing mediaType or mediaId"})
		return
	}
	var mediaId int
	_, err := fmt.Sscanf(mediaIdStr, "%d", &mediaId)
	if err != nil || mediaId == 0 {
		TrailarrLog(WARN, "YouTube", "Invalid mediaId in query params (SSE): %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid mediaId"})
		return
	}
	TrailarrLog(INFO, "YouTube", "YouTubeTrailerSearchStreamHandler GET: mediaType=%s, mediaId=%d", mediaType, mediaId)

	// Lookup media title/originalTitle using existing helper
	title, originalTitle, err := getTitlesFromCache(MediaType(mediaType), mediaId)
	if err != nil {
		TrailarrLog(ERROR, "YouTube", "Failed to load cache for mediaType=%s: %v", mediaType, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Media cache not found"})
		return
	}
	if title == "" && originalTitle == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Media not found"})
		return
	}

	// Build search terms (original title first, then title if different)
	var searchTerms []string
	if originalTitle != "" {
		searchTerms = append(searchTerms, originalTitle)
	}
	if title != "" && originalTitle != title {
		searchTerms = append(searchTerms, title)
	}
	videoIdSet := make(map[string]bool)
	totalCount := 0
	const maxResults = 10

	for _, term := range searchTerms {
		if totalCount >= maxResults {
			break
		}
		added, err := streamYtDlpSearchForTerm(term, maxResults-totalCount, videoIdSet, c)
		if err != nil {
			TrailarrLog(ERROR, "YouTube", "yt-dlp search error for term '%s': %v", term, err)
			// continue to next term
		}
		totalCount += added
	}

	// Send done event and flush
	fmt.Fprintf(c.Writer, "event: done\ndata: {}\n\n")
	c.Writer.Flush()
}

// streamYtDlpSearchForTerm runs a single yt-dlp search for "term trailer", streams JSON lines,
// emits SSE events to the gin context writer for unique video IDs, and returns how many items were added.
func streamYtDlpSearchForTerm(term string, remaining int, videoIdSet map[string]bool, c *gin.Context) (int, error) {
	if remaining <= 0 {
		return 0, nil
	}
	searchQuery := term + " trailer"
	ytDlpArgs := []string{"-j", ytDlpSearchPrefix + searchQuery, ytDlpSkipDownload}
	TrailarrLog(INFO, "YouTube", "yt-dlp command (SSE): yt-dlp %v", ytDlpArgs)

	if YtDlpTestMode {
		// Fast test-mode: emit fake IDs based on term
		added := 0
		for i := 0; i < remaining; i++ {
			fakeID := fmt.Sprintf("test-%s-%d", strings.ReplaceAll(term, " ", "-"), i)
			if !videoIdSet[fakeID] {
				videoIdSet[fakeID] = true
				result := gin.H{"id": gin.H{"videoId": fakeID}, "snippet": gin.H{"title": fakeID}}
				b, _ := json.Marshal(result)
				fmt.Fprintf(c.Writer, "data: %s\n\n", b)
				c.Writer.Flush()
				added++
			}
		}
		return added, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	reader, cmd, err := startYtDlpCommand(ctx, YtDlpCmd, ytDlpArgs)
	if err != nil {
		return 0, err
	}
	// Ensure we wait for the command to exit
	defer func() {
		_ = cmd.Wait()
	}()

	added, err := streamYtDlpOutput(reader, cmd, remaining, videoIdSet, c)
	if ctx.Err() == context.DeadlineExceeded {
		TrailarrLog(ERROR, "YouTube", "[SSE] yt-dlp search timed out for query: %s", searchQuery)
	}
	return added, err
}

// startYtDlpCommand starts the yt-dlp command with the provided args and returns a buffered reader for stdout.
func startYtDlpCommand(ctx context.Context, cmdName string, args []string) (*bufio.Reader, *exec.Cmd, error) {
	// Delegate to the configurable runner
	stdout, cmd, err := ytDlpRunner.StartCommand(ctx, cmdName, args)
	if err != nil {
		TrailarrLog(ERROR, "YouTube", "Failed to start yt-dlp via runner: %v", err)
		return nil, nil, err
	}
	TrailarrLog(INFO, "YouTube", "Started yt-dlp process (SSE streaming) via runner...")
	return bufio.NewReader(stdout), cmd, nil
}

// streamYtDlpOutput reads lines from reader and processes JSON lines until remaining items are found or reader ends.
func streamYtDlpOutput(reader *bufio.Reader, cmd *exec.Cmd, remaining int, videoIdSet map[string]bool, c *gin.Context) (int, error) {
	added := 0

	for added < remaining {
		done, err := readProcessLine(reader, &added, remaining, videoIdSet, c)
		if done {
			if err != nil && err != io.EOF {
				TrailarrLog(ERROR, "YouTube", "[SSE] Reader error: %v", err)
			}
			break
		}
	}

	// kill process early if we collected enough results
	if added >= remaining && cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	return added, nil
}

// readProcessLine reads a single line from the reader, processes it and updates added; it returns
// (true, err) when the caller should stop the loop (on error or if added reached remaining).
func readProcessLine(reader *bufio.Reader, added *int, remaining int, videoIdSet map[string]bool, c *gin.Context) (bool, error) {
	line, err := reader.ReadBytes('\n')

	// process any non-empty trimmed line
	if len(bytes.TrimSpace(line)) > 0 {
		if inc, ok := handleYtDlpJSONLine(line, videoIdSet, c); ok {
			*added += inc
			if *added >= remaining {
				return true, nil
			}
		}
	}

	// if there was an error reading, indicate caller should stop
	if err != nil {
		return len(bytes.TrimSpace(line)) == 0, err
	}
	return false, nil
}

// handleYtDlpJSONLine parses a single JSON line from yt-dlp, sends an SSE event for a new ID,
// and returns (increment, true) on success or (0, false) if nothing was emitted.
func handleYtDlpJSONLine(line []byte, videoIdSet map[string]bool, c *gin.Context) (int, bool) {
	var item struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Thumbnail   string `json:"thumbnail"`
		Channel     string `json:"channel"`
		ChannelID   string `json:"channel_id"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(line), &item); err != nil {
		// ignore unparsable lines silently for streaming
		if len(bytes.TrimSpace(line)) > 0 {
			TrailarrLog(DEBUG, "YouTube", "Ignored unparsable line: %s | error: %v", string(line), err)
		}
		return 0, false
	}
	if item.ID == "" || videoIdSet[item.ID] {
		return 0, false
	}

	videoIdSet[item.ID] = true
	result := gin.H{
		"id": gin.H{"videoId": item.ID},
		"snippet": gin.H{
			"title":       item.Title,
			"description": item.Description,
			"thumbnails": gin.H{
				"default": gin.H{"url": item.Thumbnail},
			},
			"channelTitle": item.Channel,
			"channelId":    item.ChannelID,
		},
	}
	b, _ := json.Marshal(result)
	fmt.Fprintf(c.Writer, "data: %s\n\n", b)
	c.Writer.Flush()
	return 1, true
}

type YtdlpFlagsConfig struct {
	Quiet              bool    `yaml:"quiet" json:"quiet"`
	NoProgress         bool    `yaml:"noprogress" json:"noprogress"`
	WriteSubs          bool    `yaml:"writesubs" json:"writesubs"`
	WriteAutoSubs      bool    `yaml:"writeautosubs" json:"writeautosubs"`
	EmbedSubs          bool    `yaml:"embedsubs" json:"embedsubs"`
	SubLangs           string  `yaml:"sublangs" json:"sublangs"`
	RequestedFormats   string  `yaml:"requestedformats" json:"requestedformats"`
	Timeout            float64 `yaml:"timeout" json:"timeout"`
	SleepInterval      float64 `yaml:"sleepInterval" json:"sleepInterval"`
	MaxDownloads       int     `yaml:"maxDownloads" json:"maxDownloads"`
	LimitRate          string  `yaml:"limitRate" json:"limitRate"`
	SleepRequests      float64 `yaml:"sleepRequests" json:"sleepRequests"`
	MaxSleepInterval   float64 `yaml:"maxSleepInterval" json:"maxSleepInterval"`
	CookiesFromBrowser string  `yaml:"cookiesFromBrowser" json:"cookiesFromBrowser"`
}

// YtdlpFlagsConfig holds configuration flags for yt-dlp command-line invocations.
// Fields mirror available CLI flags and are used to build the yt-dlp arguments.

// DownloadQueueItem represents a single download request
type DownloadQueueItem struct {
	MediaType  MediaType `json:"mediaType"`
	MediaId    int       `json:"mediaId"`
	MediaTitle string    `json:"mediaTitle"`
	ExtraType  string    `json:"extraType"`
	ExtraTitle string    `json:"extraTitle"`
	YouTubeID  string    `json:"youtubeId"`
	QueuedAt   time.Time `json:"queuedAt"`
	Status     string    `json:"status"` // "queued", "downloading", etc.
	Reason     string    `json:"reason,omitempty"`
}

// DownloadStatus holds the status of a download
type DownloadStatus struct {
	Status    string // e.g. "queued", "downloading", "downloaded", "failed", "exists", "rejected"
	UpdatedAt time.Time
	Error     string
}

// runtime state (declared above in the package var block)

// BatchStatusRequest is the request body for batch status queries
type BatchStatusRequest struct {
	YoutubeIds []string `json:"youtubeIds"`
}

// BatchStatusResponse is the response body for batch status queries
type BatchStatusResponse struct {
	Statuses map[string]*DownloadStatus `json:"statuses"`
}

// GetBatchDownloadStatusHandler returns the status for multiple YouTube IDs
func GetBatchDownloadStatusHandler(c *gin.Context) {
	var req BatchStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.YoutubeIds) == 0 {
		TrailarrLog(WARN, "BATCH", "/api/extras/status/batch invalid request: %v, body: %v", err, c.Request.Body)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	TrailarrLog(INFO, "BATCH", "/api/extras/status/batch request: %+v", req)

	statuses := make(map[string]*DownloadStatus, len(req.YoutubeIds))

	ctx := context.Background()
	queue := loadQueueFromStore(ctx)
	rejectedMap := buildRejectedMap(ctx)
	movieCache, _ := LoadMediaFromStore(MoviesStoreKey)
	seriesCache, _ := LoadMediaFromStore(SeriesStoreKey)
	existsInCache := makeExistsInCacheFunc(movieCache, seriesCache)

	queueMutex.Lock()
	for _, id := range req.YoutubeIds {
		// 1. In-memory status
		if st, ok := downloadStatusMap[id]; ok {
			statuses[id] = st
			continue
		}
		// 2. Persistent queue file (last known status)
		if st := findLastQueueStatus(queue, id); st != nil {
			statuses[id] = st
			continue
		}
		// 3. Rejected file
		if r, ok := rejectedMap[id]; ok {
			statuses[id] = &DownloadStatus{Status: "rejected", UpdatedAt: time.Now(), Error: r.Reason}
			continue
		}
		// 4. Cache files (exists)
		if existsInCache(id) {
			statuses[id] = &DownloadStatus{Status: "exists", UpdatedAt: time.Now()}
			continue
		}
		// 5. Fallback to missing
		statuses[id] = &DownloadStatus{Status: "missing"}
	}
	queueMutex.Unlock()

	// Log actual status values, not just pointers
	statusLog := make(map[string]DownloadStatus)
	for k, v := range statuses {
		if v != nil {
			statusLog[k] = *v
		}
	}
	TrailarrLog(INFO, "BATCH", "/api/extras/status/batch response: %+v", statusLog)
	c.JSON(http.StatusOK, BatchStatusResponse{Statuses: statuses})
}

// loadQueueFromStore returns the persisted queue entries from the store as DownloadQueueItem slice.
func loadQueueFromStore(ctx context.Context) []DownloadQueueItem {
	var queue []DownloadQueueItem
	client := GetStoreClient()
	TrailarrLog(INFO, "QUEUE", "[loadQueueFromStore] QueueKey=%v, StoreClient=%#v", DownloadQueue, client)
	items, err := client.LRange(ctx, DownloadQueue, 0, -1)
	if err != nil {
		return queue
	}
	for _, itemStr := range items {
		var item DownloadQueueItem
		if err := json.Unmarshal([]byte(itemStr), &item); err == nil {
			queue = append(queue, item)
		}
	}
	return queue
}

// buildRejectedMap builds a quick lookup map of rejected extras by youtubeId.
func buildRejectedMap(ctx context.Context) map[string]RejectedExtra {
	rejectedMap := make(map[string]RejectedExtra)
	extras, err := GetAllExtras(ctx)
	if err != nil {
		return rejectedMap
	}
	for _, e := range extras {
		if e.Status == "rejected" {
			rejectedMap[e.YoutubeId] = RejectedExtra{
				MediaType:  e.MediaType,
				MediaId:    e.MediaId,
				ExtraType:  e.ExtraType,
				ExtraTitle: e.ExtraTitle,
				YoutubeId:  e.YoutubeId,
				Reason:     e.Reason,
			}
		}
	}
	return rejectedMap
}

// findLastQueueStatus returns the last known DownloadStatus for youtubeId from the queue, or nil.
func findLastQueueStatus(queue []DownloadQueueItem, youtubeId string) *DownloadStatus {
	for i := len(queue) - 1; i >= 0; i-- {
		if queue[i].YouTubeID == youtubeId {
			return &DownloadStatus{Status: queue[i].Status, UpdatedAt: queue[i].QueuedAt}
		}
	}
	return nil
}

// makeExistsInCacheFunc returns a function that checks the provided movie/series caches for a youtubeId.
func makeExistsInCacheFunc(movieCache, seriesCache []map[string]interface{}) func(string) bool {
	return func(yid string) bool {
		for _, m := range movieCache {
			if v, ok := m["youtubeId"]; ok && v == yid {
				return true
			}
		}
		for _, m := range seriesCache {
			if v, ok := m["youtubeId"]; ok && v == yid {
				return true
			}
		}
		return false
	}
}

// AddToDownloadQueue adds a new download request to the queue and persists in the store
// source: "task" (block if queue not empty), "api" (always append)
func AddToDownloadQueue(item DownloadQueueItem, source string) {
	TrailarrLog(INFO, "QUEUE", "[AddToDownloadQueue] Entered. YouTubeID=%s, source=%s", item.YouTubeID, source)
	ctx := context.Background()
	client := GetStoreClient()

	// Lookup media title if not set
	fillMediaTitleIfMissing(&item)

	// Always append to the persistent queue; higher-level callers decide if
	// they should wait to avoid flooding (e.g. extras task). API callers expect
	// immediate enqueue behavior.
	item.Status = "queued"
	item.QueuedAt = time.Now()
	b, err := json.Marshal(item)
	TrailarrLog(INFO, "QUEUE", "[AddToDownloadQueue] Marshaled JSON: %s", string(b))
	if err != nil {
		TrailarrLog(ERROR, "QUEUE", "[AddToDownloadQueue] Failed to marshal item: %v", err)
		return
	}
	err = client.RPush(ctx, DownloadQueue, b)
	TrailarrLog(INFO, "QUEUE", "[AddToDownloadQueue] RPush error: %v", err)
	if err != nil {
		TrailarrLog(ERROR, "QUEUE", "[AddToDownloadQueue] Failed to push to store: %v", err)
	} else {
		TrailarrLog(INFO, "QUEUE", "[AddToDownloadQueue] Successfully enqueued item. StoreKey=%s, YouTubeID=%s", DownloadQueue, item.YouTubeID)
		// Broadcast updated queue to all WebSocket clients
		BroadcastDownloadQueueChanges([]DownloadQueueItem{item})
	}
	downloadStatusMap[item.YouTubeID] = &DownloadStatus{Status: "queued", UpdatedAt: time.Now()}
	TrailarrLog(INFO, "QUEUE", "[AddToDownloadQueue] Enqueued: mediaType=%v, mediaId=%v, extraType=%s, extraTitle=%s, youtubeId=%s, source=%s", item.MediaType, item.MediaId, item.ExtraType, item.ExtraTitle, item.YouTubeID, source)
}

// fillMediaTitleIfMissing attempts to populate MediaTitle on the queue item using the cache.
func fillMediaTitleIfMissing(item *DownloadQueueItem) {
	if item.MediaTitle != "" {
		return
	}
	cacheFile, _ := resolveCachePath(item.MediaType)
	if cacheFile == "" {
		return
	}
	items, _ := loadCache(cacheFile)
	for _, m := range items {
		idInt, ok := parseMediaID(m["id"])
		if ok && idInt == item.MediaId {
			if t, ok := m["title"].(string); ok {
				item.MediaTitle = t
				return
			}
		}
	}
}

// GetDownloadStatus returns the status for a YouTube ID
func GetDownloadStatus(youtubeID string) *DownloadStatus {
	queueMutex.Lock()
	defer queueMutex.Unlock()
	if status, ok := downloadStatusMap[youtubeID]; ok {
		return status
	}
	return nil
}

// NextQueuedItem fetches the next queued item from the store and its index
func NextQueuedItem() (int, DownloadQueueItem, bool) {
	ctx := context.Background()
	client := GetStoreClient()
	queue, err := client.LRange(ctx, DownloadQueue, 0, -1)
	if err != nil {
		return -1, DownloadQueueItem{}, false
	}
	for i, qstr := range queue {
		var item DownloadQueueItem
		if err := json.Unmarshal([]byte(qstr), &item); err == nil {
			if item.Status == "queued" {
				return i, item, true
			}
		}
	}
	return -1, DownloadQueueItem{}, false
}

// StartDownloadQueueWorker starts a goroutine to process the download queue from the store
func StartDownloadQueueWorker() {
	go func() {
		ctx := context.Background()
		client := GetStoreClient()
		// Clean the queue at startup
		_ = client.Del(ctx, DownloadQueue)
		for {
			idx, item, ok := NextQueuedItem()
			if !ok {
				time.Sleep(2 * time.Second)
				continue
			}
			if err := processQueueItem(ctx, idx, item); err != nil {
				TrailarrLog(ERROR, "QUEUE", "[StartDownloadQueueWorker] processQueueItem error: %v", err)
			}
		}
	}()
}

// processQueueItem handles a single queue item end-to-end and returns an error only for unexpected conditions.
func processQueueItem(ctx context.Context, idx int, item DownloadQueueItem) error {
	client := GetStoreClient()

	// 1) Skip and remove rejected extras
	if skipped, err := skipRejectedExtra(ctx, item); err != nil {
		return err
	} else if skipped {
		return nil
	}

	// 2) Mark as downloading
	if err := markItemDownloading(ctx, idx, item); err != nil {
		// log and continue attempting download even if marking failed
		TrailarrLog(WARN, "QUEUE", "[processQueueItem] Failed to mark downloading in store: %v", err)
	}

	// 3) Perform the download
	meta, metaErr := DownloadYouTubeExtra(item.MediaType, item.MediaId, item.ExtraType, item.ExtraTitle, item.YouTubeID)

	// 4) If 429, pause the queue (handled inside)
	if metaErr != nil {
		if tooMany, ok := metaErr.(*TooManyRequestsError); ok {
			handleTooManyRequestsPause(tooMany)
		}
	}

	// 5) Determine final status and update in-memory map
	var finalStatus, failReason string
	if metaErr != nil {
		finalStatus = "failed"
		failReason = metaErr.Error()
		downloadStatusMap[item.YouTubeID] = &DownloadStatus{Status: finalStatus, UpdatedAt: time.Now(), Error: failReason}
	} else if meta != nil {
		finalStatus = meta.Status
		downloadStatusMap[item.YouTubeID] = &DownloadStatus{Status: finalStatus, UpdatedAt: time.Now()}
	} else {
		finalStatus = "failed"
		failReason = "No metadata returned from download"
		downloadStatusMap[item.YouTubeID] = &DownloadStatus{Status: finalStatus, UpdatedAt: time.Now(), Error: failReason}
	}

	// 6) Update the queue entry in the store and broadcast final status
	if err := updateFinalStatusInStore(ctx, idx, finalStatus, failReason); err != nil {
		// If updating the store failed, still broadcast the status using the item
		item.Status = finalStatus
		if finalStatus == "failed" && failReason != "" {
			item.Reason = failReason
		}
		BroadcastDownloadQueueChanges([]DownloadQueueItem{item})
	}

	// 7) Wait briefly then remove from queue (configurable for tests)
	time.Sleep(QueueItemRemoveDelay)
	b, _ := json.Marshal(item)
	_ = client.LRem(ctx, DownloadQueue, 1, b)

	return nil
}

func skipRejectedExtra(ctx context.Context, item DownloadQueueItem) (bool, error) {
	entry, err := GetExtraByYoutubeId(ctx, item.YouTubeID, item.MediaType, item.MediaId)
	if err != nil {
		// treat errors as non-fatal; log and proceed
		TrailarrLog(DEBUG, "QUEUE", "[skipRejectedExtra] lookup error: %v", err)
		return false, nil
	}
	if entry != nil && entry.Status == "rejected" {
		TrailarrLog(WARN, "QUEUE", "[StartDownloadQueueWorker] Skipping rejected extra: mediaType=%v, mediaId=%v, extraType=%s, extraTitle=%s, youtubeId=%s", item.MediaType, item.MediaId, item.ExtraType, item.ExtraTitle, item.YouTubeID)
		b, _ := json.Marshal(item)
		// Remove from queue immediately
		_ = GetStoreClient().LRem(ctx, DownloadQueue, 1, b)
		BroadcastDownloadQueueChanges([]DownloadQueueItem{item})
		return true, nil
	}
	return false, nil
}

func markItemDownloading(ctx context.Context, idx int, item DownloadQueueItem) error {
	queue, err := GetStoreClient().LRange(ctx, DownloadQueue, 0, -1)
	if err != nil {
		return err
	}
	if idx >= 0 && idx < len(queue) {
		var q DownloadQueueItem
		if err := json.Unmarshal([]byte(queue[idx]), &q); err == nil {
			q.Status = "downloading"
			b, _ := json.Marshal(q)
			_ = GetStoreClient().LSet(ctx, DownloadQueue, int64(idx), b)
			downloadStatusMap[item.YouTubeID] = &DownloadStatus{Status: "downloading", UpdatedAt: time.Now()}
			BroadcastDownloadQueueChanges([]DownloadQueueItem{q})
		}
	}
	return nil
}

func handleTooManyRequestsPause(err429 *TooManyRequestsError) {
	TrailarrLog(WARN, "QUEUE", "[StartDownloadQueueWorker] 429 detected, pausing queue for %v: %s", TooManyRequestsPauseDuration, err429.Error())
	pauseUntil := time.Now().Add(TooManyRequestsPauseDuration)
	for time.Now().Before(pauseUntil) {
		TrailarrLog(INFO, "QUEUE", "[StartDownloadQueueWorker] Queue paused for 429. Resuming in %v seconds...", int(time.Until(pauseUntil).Seconds()))
		time.Sleep(TooManyRequestsPauseLogInterval)
	}
	TrailarrLog(INFO, "QUEUE", "[StartDownloadQueueWorker] %v pause for 429 complete. Resuming queue.", TooManyRequestsPauseDuration)
}

func updateFinalStatusInStore(ctx context.Context, idx int, finalStatus, failReason string) error {
	queue, err := GetStoreClient().LRange(ctx, DownloadQueue, 0, -1)
	if err != nil {
		return err
	}
	if idx >= 0 && idx < len(queue) {
		var q DownloadQueueItem
		if err := json.Unmarshal([]byte(queue[idx]), &q); err == nil {
			q.Status = finalStatus
			if finalStatus == "failed" && failReason != "" {
				q.Reason = failReason
			}
			b, _ := json.Marshal(q)
			_ = GetStoreClient().LSet(ctx, DownloadQueue, int64(idx), b)
			BroadcastDownloadQueueChanges([]DownloadQueueItem{q})
		}
	}
	return nil
}

// GetDownloadStatusHandler returns the status of a download by YouTube ID
func GetDownloadStatusHandler(c *gin.Context) {
	youtubeId := c.Param("youtubeId")
	status := GetDownloadStatus(youtubeId)
	if status == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": status})
}

func DefaultYtdlpFlagsConfig() YtdlpFlagsConfig {
	return YtdlpFlagsConfig{
		Quiet:            false,
		NoProgress:       false,
		WriteSubs:        true,
		WriteAutoSubs:    true,
		EmbedSubs:        true,
		SubLangs:         "es.*",
		RequestedFormats: "best[height<=1080]",
		Timeout:          3.0,
		SleepInterval:    5.0,
		MaxDownloads:     5,
		LimitRate:        "30M",
		SleepRequests:    3.0,
		MaxSleepInterval: 120.0,
	}
}

// DeduplicateByKey returns a new slice containing the first occurrence of each
// map in the provided list, deduplicated by the value at the provided key.
func DeduplicateByKey(list []map[string]string, key string) []map[string]string {
	seen := make(map[string]bool)
	unique := make([]map[string]string, 0, len(list))
	for _, item := range list {
		k := item[key]
		if !seen[k] {
			unique = append(unique, item)
			seen[k] = true
		}
	}
	return unique
}

type ExtraDownloadMetadata struct {
	MediaType  MediaType // "movie" or "series"
	MediaId    int       // Radarr or Sonarr ID as int
	MediaTitle string    // Movie or Series title
	ExtraType  string    // e.g. "Trailer"
	ExtraTitle string    // e.g. "Official Trailer"
	YouTubeID  string
	FileName   string
	Status     string
}

// NewExtraDownloadMetadata constructs an ExtraDownloadMetadata with status and all fields
func NewExtraDownloadMetadata(info *downloadInfo, youtubeId string, status string) *ExtraDownloadMetadata {
	return &ExtraDownloadMetadata{
		MediaType:  info.MediaType,
		MediaId:    info.MediaId,
		MediaTitle: info.MediaTitle,
		ExtraTitle: info.ExtraTitle,
		ExtraType:  info.ExtraType,
		YouTubeID:  info.YouTubeID,
		FileName:   info.OutFile,
		Status:     status,
	}
}

type RejectedExtra struct {
	MediaType  MediaType `json:"mediaType"`
	MediaId    int       `json:"mediaId"`
	ExtraType  string    `json:"extraType"`
	ExtraTitle string    `json:"extraTitle"`
	YoutubeId  string    `json:"youtubeId"`
	Reason     string    `json:"reason"`
}

// DownloadYouTubeExtra downloads the specified YouTube extra (trailer/clip)
// for the given media and returns metadata about the downloaded file. If
// forceDownload is provided and true, an existing file may be re-downloaded.
func DownloadYouTubeExtra(mediaType MediaType, mediaId int, extraType, extraTitle, youtubeId string, forceDownload ...bool) (*ExtraDownloadMetadata, error) {
	TrailarrLog(DEBUG, "YouTube", "DownloadYouTubeExtra called with mediaType=%s, mediaId=%d, extraType=%s, extraTitle=%s, youtubeId=%s, forceDownload=%v",
		mediaType, mediaId, extraType, extraTitle, youtubeId, forceDownload)

	downloadInfo, err := prepareDownloadInfo(mediaType, mediaId, extraType, extraTitle, youtubeId)
	if err != nil {
		return nil, err
	}

	// Always clean up temp dir after download attempt
	defer func() {
		if downloadInfo != nil && downloadInfo.TempDir != "" {
			os.RemoveAll(downloadInfo.TempDir)
		}
	}()

	// Log resolved media title and download intent
	TrailarrLog(INFO, "YouTube", "Downloading YouTube extra: mediaType=%s, mediaTitle=%s, type=%s, title=%s, youtubeId=%s",
		downloadInfo.MediaType, downloadInfo.MediaTitle, extraType, extraTitle, youtubeId)

	// Check if extra is rejected or already exists
	if meta, err := checkExistingExtra(downloadInfo, youtubeId); meta != nil || err != nil {
		return meta, err
	}

	// Perform the download
	return performDownload(downloadInfo, youtubeId)
}

type downloadInfo struct {
	MediaType  MediaType
	MediaId    int
	MediaTitle string
	OutDir     string
	OutFile    string
	TempDir    string
	TempFile   string
	YouTubeID  string
	ExtraType  string
	ExtraTitle string
	SafeTitle  string
}

func prepareDownloadInfo(mediaType MediaType, mediaId int, extraType, extraTitle, youtubeID string) (*downloadInfo, error) {
	// Resolve cache file and media title
	cacheFile, mediaTitle := resolveCacheAndTitle(mediaType, mediaId)

	// Get path mappings (log on error, but continue)
	mappings := getPathMappingsSafe(mediaType)

	// Try to find a mapped media path using cache + mappings
	mappedMediaPath := findMappedMediaPath(cacheFile, mappings, mediaId)

	// Derive base path using mapped path, mappings fallback, or media title
	basePath := deriveBasePath(mappedMediaPath, mappings, mediaTitle)

	// Build output directory and sanitize title
	canonicalType := canonicalizeExtraType(extraType)
	outDir := filepath.Join(basePath, canonicalType)
	safeTitle := sanitizeFileName(extraTitle)

	// Prepare filenames
	outExt := "mkv"
	outFile := filepath.Join(outDir, fmt.Sprintf("%s.%s", safeTitle, outExt))

	// Create temp directory and temp file path
	tempDir, tempFile, err := createTempPaths(safeTitle, outExt)
	if err != nil {
		TrailarrLog(ERROR, "YouTube", "Failed to create temp dir for yt-dlp: %v", err)
		return nil, fmt.Errorf("failed to create temp dir for yt-dlp: %w", err)
	}

	TrailarrLog(DEBUG, "YouTube", "Resolved output directory: %s", outDir)
	TrailarrLog(DEBUG, "YouTube", "Resolved safe title: %s", safeTitle)
	TrailarrLog(DEBUG, "YouTube", "mediaType=%s, mediaTitle=%s, canonicalType=%s, outDir=%s, outFile=%s, tempDir=%s, tempFile=%s",
		mediaType, mediaTitle, canonicalType, outDir, outFile, tempDir, tempFile)

	return &downloadInfo{
		MediaType:  mediaType,
		MediaId:    mediaId,
		MediaTitle: mediaTitle,
		OutDir:     outDir,
		OutFile:    outFile,
		TempDir:    tempDir,
		TempFile:   tempFile,
		YouTubeID:  youtubeID,
		ExtraType:  extraType,
		ExtraTitle: extraTitle,
		SafeTitle:  safeTitle,
	}, nil
}

// Helper: resolve cache path and media title
func resolveCacheAndTitle(mediaType MediaType, mediaId int) (string, string) {
	cacheFile, _ := resolveCachePath(mediaType)
	if cacheFile == "" {
		return "", ""
	}
	items, _ := loadCache(cacheFile)
	for _, m := range items {
		idInt, ok := parseMediaID(m["id"])
		if ok && idInt == mediaId {
			if t, ok := m["title"].(string); ok {
				return cacheFile, t
			}
		}
	}
	return cacheFile, ""
}

// Helper: safely get path mappings, log errors and return empty slice on failure
func getPathMappingsSafe(mediaType MediaType) [][]string {
	mappings, err := GetPathMappings(mediaType)
	if err != nil {
		TrailarrLog(ERROR, "YouTube", "Failed to get path mappings: %v", err)
		return [][]string{}
	}
	return mappings
}

// Helper: find mapped media path by applying mappings to the media path found in cache
func findMappedMediaPath(cacheFile string, mappings [][]string, mediaId int) string {
	if cacheFile == "" || len(mappings) == 0 {
		return ""
	}
	mediaPath, lookupErr := FindMediaPathByID(cacheFile, mediaId)
	if lookupErr != nil || mediaPath == "" {
		return ""
	}
	// Apply first mapping that matches prefix
	for _, m := range mappings {
		if len(m) > 1 && strings.HasPrefix(mediaPath, m[0]) {
			return m[1] + mediaPath[len(m[0]):]
		}
	}
	// If no mapping matched, return the raw media path
	return mediaPath
}

// Helper: derive base path from mapped path, fallback to mappings[0][1] + title, or title, or empty
func deriveBasePath(mappedMediaPath string, mappings [][]string, mediaTitle string) string {
	if mappedMediaPath != "" {
		return mappedMediaPath
	}
	if len(mappings) > 0 && len(mappings[0]) > 1 && mappings[0][1] != "" {
		if mediaTitle != "" {
			return filepath.Join(mappings[0][1], mediaTitle)
		}
		return mappings[0][1]
	}
	if mediaTitle != "" {
		return mediaTitle
	}
	return ""
}

// Helper: sanitize a filename/title for use as a file (replace forbidden chars with _)
func sanitizeFileName(name string) string {
	forbidden := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	safe := name
	for _, c := range forbidden {
		safe = strings.ReplaceAll(safe, c, "_")
	}
	return safe
}

// Helper: create temp dir and return tempDir and tempFile path
func createTempPaths(safeTitle, ext string) (string, string, error) {
	// Create temp dirs under the package-level TrailarrRoot so we only create
	// a single top-level test temp directory during test runs instead of
	// multiple top-level /tmp entries.
	// Ensure TrailarrRoot exists (TestMain should have set it for tests).
	_ = os.MkdirAll(TrailarrRoot, 0755)
	tempDir, err := os.MkdirTemp(TrailarrRoot, "yt-dlp-tmp-*")
	if err != nil {
		return "", "", err
	}
	// Register temp dir so TestMain can clean it up at the end of the test run
	RegisterTempDir(tempDir)
	tempFile := filepath.Join(tempDir, fmt.Sprintf("%s.%s", safeTitle, ext))
	return tempDir, tempFile, nil
}

func checkExistingExtra(info *downloadInfo, youtubeId string) (*ExtraDownloadMetadata, error) {
	// Check if extra is in rejected_extras.json
	if meta := checkRejectedExtras(info, youtubeId); meta != nil {
		return meta, nil
	}

	// Skip download if file already exists
	if _, err := os.Stat(info.OutFile); err == nil {
		TrailarrLog(INFO, "YouTube", "File already exists, skipping: %s", info.OutFile)
		return NewExtraDownloadMetadata(info, youtubeId, "exists"), nil
	}

	return nil, nil
}

func checkRejectedExtras(info *downloadInfo, youtubeId string) *ExtraDownloadMetadata {
	// Use the hash-based approach: check if extra is marked as rejected in the hash
	ctx := context.Background()
	entry, err := GetExtraByYoutubeId(ctx, youtubeId, info.MediaType, info.MediaId)
	if err == nil && entry != nil && entry.Status == "rejected" {
		return NewExtraDownloadMetadata(info, youtubeId, "rejected")
	}
	return nil
}

func performDownload(info *downloadInfo, youtubeId string) (*ExtraDownloadMetadata, error) {
	args := buildYtDlpArgs(info, youtubeId, true)
	// Execute yt-dlp command via configurable runner
	output, err := ytDlpRunner.CombinedOutput(YtDlpCmd, args, info.TempDir)

	if err != nil && isImpersonationErrorNative(string(output)) {
		TrailarrLog(WARN, "YouTube", "Impersonation failed for %s, retrying without impersonation", youtubeId)
		args = buildYtDlpArgs(info, youtubeId, false)
		output, err = ytDlpRunner.CombinedOutput(YtDlpCmd, args, info.TempDir)
	}
	TrailarrLog(DEBUG, "YouTube", "yt-dlp command executed: %s %s", YtDlpCmd, strings.Join(args, " "))

	if len(output) > 0 {
		for _, line := range strings.Split(string(output), "\n") {
			if strings.TrimSpace(line) != "" {
				TrailarrLog(DEBUG, "YouTube", "yt-dlp output for %s: %s", youtubeId, line)
			}
		}
	}
	if err != nil {
		// Check for 429/Too Many Requests in output
		if strings.Contains(string(output), "429") || strings.Contains(strings.ToLower(string(output)), "too many requests") {
			return nil, &TooManyRequestsError{Message: "yt-dlp hit 429 Too Many Requests"}
		}
		return nil, handleDownloadErrorNative(info, youtubeId, err, string(output))
	}

	// Move file to final location
	if err := moveDownloadedFile(info); err != nil {
		return nil, err
	}

	// Create metadata
	return createSuccessMetadata(info, youtubeId)
}

// TooManyRequestsError is returned when a 429/Too Many Requests is detected
type TooManyRequestsError struct {
	Message string
}

func (e *TooManyRequestsError) Error() string {
	return e.Message
}

func isImpersonationErrorNative(output string) bool {
	return strings.Contains(output, "Impersonate target") ||
		strings.Contains(output, "is not available") ||
		strings.Contains(output, "missing dependencies required to support this target")
}

func buildYtDlpArgs(info *downloadInfo, youtubeId string, impersonate bool) []string {
	cfg, _ := GetYtdlpFlagsConfig()
	args := []string{
		"--cookies", CookiesFile,
		"--remux-video", "mkv",
		"--format", cfg.RequestedFormats,
		"--output", info.TempFile,
		"--max-downloads", fmt.Sprintf("%d", cfg.MaxDownloads),
		"--limit-rate", cfg.LimitRate,
		"--sleep-interval", fmt.Sprintf("%.0f", cfg.SleepInterval),
		"--sleep-requests", fmt.Sprintf("%.0f", cfg.SleepRequests),
		"--max-sleep-interval", fmt.Sprintf("%.0f", cfg.MaxSleepInterval),
		"--socket-timeout", fmt.Sprintf("%.0f", cfg.Timeout),
	}
	if cfg.Quiet {
		args = append(args, "--quiet")
	}
	if cfg.NoProgress {
		args = append(args, "--no-progress")
	}
	if cfg.WriteSubs {
		args = append(args, "--write-subs")
		args = append(args, "--sub-format", "srt")
		if cfg.WriteAutoSubs {
			args = append(args, "--write-auto-subs")
		}
		if cfg.EmbedSubs {
			args = append(args, "--embed-subs")
		}
		if cfg.SubLangs != "" {
			args = append(args, "--sub-langs", cfg.SubLangs)
		}
	}
	if impersonate {
		args = append(args, "--impersonate", "chrome")
	}

	args = append(args, "--", youtubeId)
	return args
}

func handleDownloadErrorNative(info *downloadInfo, youtubeId string, err error, output string) error {
	reason := err.Error()
	if output != "" {
		reason += " | output: " + output
	}

	TrailarrLog(ERROR, "YouTube", "Download failed for %s: %s", youtubeId, reason)
	addToRejectedExtras(info, youtubeId, reason)
	// Also update the unified extras collection in the persistent store
	errMark := SetExtraRejectedPersistent(info.MediaType, info.MediaId, info.ExtraType, info.ExtraTitle, youtubeId, reason)
	if errMark != nil {
		TrailarrLog(ERROR, "YouTube", "Failed to mark extra as rejected in store: %v", errMark)
	}
	return fmt.Errorf(reason+": %w", err)
}

func addToRejectedExtras(info *downloadInfo, youtubeId, reason string) {
	// Use the hash-based approach: mark as rejected in the hash only if not already rejected
	ctx := context.Background()
	entry, err := GetExtraByYoutubeId(ctx, youtubeId, info.MediaType, info.MediaId)
	if err == nil && entry != nil && entry.Status == "rejected" {
		return
	}
	// Add or update as rejected
	_ = SetExtraRejectedPersistent(info.MediaType, info.MediaId, info.ExtraType, info.ExtraTitle, youtubeId, reason)
}

func moveDownloadedFile(info *downloadInfo) error {
	if _, statErr := os.Stat(info.TempFile); statErr != nil {
		TrailarrLog(ERROR, "YouTube", "yt-dlp did not produce expected output file: %s", info.TempFile)
		return fmt.Errorf("yt-dlp did not produce expected output file: %s", info.TempFile)
	}

	// If OutDir is relative, ensure it is anchored under the test/runtime TrailarrRoot so
	// tests running in the repository root don't write files into the project tree.
	if !filepath.IsAbs(info.OutDir) {
		info.OutDir = filepath.Join(TrailarrRoot, info.OutDir)
	}

	if err := os.MkdirAll(info.OutDir, 0755); err != nil {
		TrailarrLog(ERROR, "YouTube", "Failed to create output dir '%s': %v", info.OutDir, err)
		return fmt.Errorf("failed to create output dir '%s': %w", info.OutDir, err)
	}

	// Ensure OutFile is absolute and located under TrailarrRoot when tests run in repo root.
	if !filepath.IsAbs(info.OutFile) {
		info.OutFile = filepath.Join(TrailarrRoot, info.OutFile)
	}

	if moveErr := os.Rename(info.TempFile, info.OutFile); moveErr != nil {
		return handleCrossDeviceMove(info.TempFile, info.OutFile, moveErr)
	}

	return nil
}

func handleCrossDeviceMove(tempFile, outFile string, moveErr error) error {
	if linkErr, ok := moveErr.(*os.LinkError); ok && strings.Contains(linkErr.Error(), "cross-device link") {
		return copyFileAcrossDevices(tempFile, outFile)
	}
	TrailarrLog(ERROR, "YouTube", "Failed to move downloaded file to output dir: %v", moveErr)
	return fmt.Errorf("failed to move downloaded file to output dir: %w", moveErr)
}

func copyFileAcrossDevices(tempFile, outFile string) error {
	in, err := os.Open(tempFile)
	if err != nil {
		TrailarrLog(ERROR, "YouTube", "Failed to open temp file for copy: %v", err)
		return fmt.Errorf("failed to open temp file for copy: %w", err)
	}
	defer in.Close()

	out, err := os.Create(outFile)
	if err != nil {
		TrailarrLog(ERROR, "YouTube", "Failed to create output file for copy: %v", err)
		return fmt.Errorf("failed to create output file for copy: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		TrailarrLog(ERROR, "YouTube", "Failed to copy file across devices: %v", err)
		return fmt.Errorf("failed to copy file across devices: %w", err)
	}

	if err := out.Sync(); err != nil {
		TrailarrLog(ERROR, "YouTube", "Failed to sync output file: %v", err)
		return fmt.Errorf("failed to sync output file: %w", err)
	}

	if rmErr := os.Remove(tempFile); rmErr != nil {
		TrailarrLog(WARN, "YouTube", "Failed to remove temp file after copy: %v", rmErr)
	}

	return nil
}

func createSuccessMetadata(info *downloadInfo, youtubeId string) (*ExtraDownloadMetadata, error) {
	meta := NewExtraDownloadMetadata(info, youtubeId, "downloaded")

	// Persist the extra entry
	entry := ExtrasEntry{
		MediaType:  info.MediaType,
		MediaId:    info.MediaId,
		ExtraTitle: info.ExtraTitle,
		ExtraType:  info.ExtraType,
		FileName:   info.OutFile,
		YoutubeId:  youtubeId,
		Status:     "downloaded",
	}
	persistExtraEntry(entry)

	// Mark media as not wanted and persist wanted-index asynchronously
	markMediaNotWantedAndPersistAsync(info)

	// Record history and write the metadata file
	recordDownloadHistory(info)
	writeMetaFile(meta, info.OutFile)

	TrailarrLog(INFO, "YouTube", "Downloaded %s to %s", info.ExtraTitle, info.OutFile)
	return meta, nil
}

// persistExtraEntry saves the ExtrasEntry into the store and logs warnings on failure.
func persistExtraEntry(entry ExtrasEntry) {
	ctx := context.Background()
	if err := AddOrUpdateExtra(ctx, entry); err != nil {
		TrailarrLog(WARN, "YouTube", "Failed to add/update extra in store after download: %v", err)
	}
}

// markMediaNotWantedAndPersistAsync sets wanted=false for the media in the
// main cache (if present) and triggers updateWantedStatusInStore asynchronously
// to refresh the lightweight wanted index.
func markMediaNotWantedAndPersistAsync(info *downloadInfo) {
	mainCacheFile, _ := resolveCachePath(info.MediaType)
	if mainCacheFile != "" {
		if items, err := LoadMediaFromStore(mainCacheFile); err == nil {
			for _, m := range items {
				if idInt, ok := parseMediaID(m["id"]); ok && idInt == info.MediaId {
					m["wanted"] = false
					break
				}
			}
		}
		go func(path string) {
			if err := updateWantedStatusInStore(path); err != nil {
				TrailarrLog(WARN, "YouTube", "updateWantedStatusInStore failed for %s: %v", path, err)
			}
		}(mainCacheFile)
	}
}

// recordDownloadHistory appends a download event to the history.
func recordDownloadHistory(info *downloadInfo) {
	mediaTitle := getMediaTitleFromCache(info.MediaType, info.MediaId)
	if mediaTitle == "" {
		mediaTitle = "Unknown"
	}
	event := HistoryEvent{
		Action:     "download",
		MediaTitle: mediaTitle,
		MediaType:  info.MediaType,
		MediaId:    info.MediaId,
		ExtraType:  info.ExtraType,
		ExtraTitle: info.ExtraTitle,
		Date:       time.Now(),
	}
	_ = AppendHistoryEvent(event)
}

// getMediaTitleFromCache returns the title for mediaId from the cache file, or empty string.
func getMediaTitleFromCache(mediaType MediaType, mediaId int) string {
	cacheFile, _ := resolveCachePath(mediaType)
	if cacheFile == "" {
		return ""
	}
	items, _ := loadCache(cacheFile)
	for _, m := range items {
		if idInt, ok := parseMediaID(m["id"]); ok && idInt == mediaId {
			if t, ok := m["title"].(string); ok {
				return t
			}
		}
	}
	return ""
}

// writeMetaFile writes the JSON metadata file next to the downloaded file.
func writeMetaFile(meta *ExtraDownloadMetadata, outFile string) {
	metaFile := outFile + ".json"
	if metaBytes, err := json.MarshalIndent(meta, "", "  "); err == nil {
		_ = os.WriteFile(metaFile, metaBytes, 0644)
	} else {
		TrailarrLog(DEBUG, "YouTube", "Failed to marshal metadata file for %s: %v", outFile, err)
	}
}

// YouTube trailer search proxy handler (POST: mediaType, mediaId)
func YouTubeTrailerSearchHandler(c *gin.Context) {
	var req struct {
		MediaType string `json:"mediaType"`
		MediaId   int    `json:"mediaId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.MediaType == "" || req.MediaId == 0 {
		TrailarrLog(WARN, "YouTube", "Invalid POST body for YouTube search: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing mediaType or mediaId"})
		return
	}
	TrailarrLog(INFO, "YouTube", "YouTubeTrailerSearchHandler POST: mediaType=%s, mediaId=%d", req.MediaType, req.MediaId)

	title, originalTitle, err := getTitlesFromCache(MediaType(req.MediaType), req.MediaId)
	if err != nil {
		TrailarrLog(ERROR, "YouTube", "Failed to load cache for mediaType=%s: %v", req.MediaType, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Media cache not found"})
		return
	}
	if title == "" && originalTitle == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Media not found"})
		return
	}

	// Build search terms (original title first, then title if different)
	var searchTerms []string
	if originalTitle != "" {
		searchTerms = append(searchTerms, originalTitle)
	}
	if title != "" && originalTitle != title {
		searchTerms = append(searchTerms, title)
	}

	results, _ := searchYtDlpForTerms(searchTerms, 10)
	if len(results) > 10 {
		results = results[:10]
	}
	TrailarrLog(INFO, "YouTube", "YouTubeTrailerSearchHandler returning %d results", len(results))
	c.JSON(http.StatusOK, gin.H{"items": results})
}

// getTitlesFromCache fetches title and originalTitle for the given media type/id
func getTitlesFromCache(mediaType MediaType, mediaId int) (string, string, error) {
	cacheFile, _ := resolveCachePath(mediaType)
	if cacheFile == "" {
		return "", "", fmt.Errorf("no cache file")
	}
	items, err := loadCache(cacheFile)
	if err != nil {
		return "", "", err
	}
	var title, originalTitle string
	for _, m := range items {
		idInt, ok := parseMediaID(m["id"])
		if ok && idInt == mediaId {
			if t, ok := m["title"].(string); ok {
				title = t
			}
			if ot, ok := m["originalTitle"].(string); ok {
				originalTitle = ot
			}
			break
		}
	}
	return title, originalTitle, nil
}

// searchYtDlpForTerms runs yt-dlp searches for the provided terms and returns up to maxResults unique items
func searchYtDlpForTerms(terms []string, maxResults int) ([]gin.H, error) {
	var allResults []gin.H
	videoIdSet := make(map[string]bool)
	for _, term := range terms {
		if len(allResults) >= maxResults {
			break
		}
		searchQuery := term + " trailer"
		TrailarrLog(INFO, "YouTube", "yt-dlp command: yt-dlp %v", []string{"-j", ytDlpSearchPrefix + searchQuery, ytDlpSkipDownload})
		if err := runYtDlpSearch(searchQuery, videoIdSet, &allResults, maxResults); err != nil {
			TrailarrLog(ERROR, "YouTube", "yt-dlp search error for query '%s': %v", searchQuery, err)
			// continue searching other terms despite the error
		}
	}
	return allResults, nil
}

// runYtDlpSearch executes yt-dlp for a single searchQuery, appending unique results to results up to maxResults.
func runYtDlpSearch(searchQuery string, videoIdSet map[string]bool, results *[]gin.H, maxResults int) error {
	ytDlpArgs := []string{"-j", ytDlpSearchPrefix + searchQuery, ytDlpSkipDownload}
	if YtDlpTestMode {
		runYtDlpSearchTestMode(searchQuery, videoIdSet, results, maxResults)
		return nil
	}
	return runYtDlpSearchReal(searchQuery, videoIdSet, results, maxResults, ytDlpArgs)
}

func runYtDlpSearchReal(searchQuery string, videoIdSet map[string]bool, results *[]gin.H, maxResults int, ytDlpArgs []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	stdout, cmd, err := ytDlpRunner.StartCommand(ctx, YtDlpCmd, ytDlpArgs)
	if err != nil {
		return fmt.Errorf("failed to start yt-dlp via runner: %w", err)
	}
	TrailarrLog(INFO, "YouTube", "Started yt-dlp process (streaming) via runner...")
	reader := bufio.NewReader(stdout)

	for {
		if len(*results) >= maxResults {
			break
		}
		line, err := reader.ReadBytes('\n')
		// process any non-empty line
		if len(line) > 0 {
			TrailarrLog(DEBUG, "YouTube", "Raw yt-dlp output line: %s", string(line))
			parseYtDlpLine(line, videoIdSet, results)
		}
		if err != nil {
			if err != io.EOF {
				TrailarrLog(ERROR, "YouTube", "Reader error: %v", err)
			}
			break
		}
	}

	// If we reached max results kill the process early
	if len(*results) >= maxResults && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	_ = cmd.Wait()
	if ctx.Err() == context.DeadlineExceeded {
		TrailarrLog(ERROR, "YouTube", "yt-dlp search timed out for query: %s", searchQuery)
	}
	return nil
}

// runYtDlpSearchTestMode appends deterministic fake results for tests.
func runYtDlpSearchTestMode(searchQuery string, videoIdSet map[string]bool, results *[]gin.H, maxResults int) {
	for i := 0; i < maxResults; i++ {
		id := fmt.Sprintf("test-%s-%d", strings.ReplaceAll(searchQuery, " ", "-"), i)
		if !videoIdSet[id] {
			videoIdSet[id] = true
			*results = append(*results, gin.H{"id": gin.H{"videoId": id}, "snippet": gin.H{"title": id}})
		}
	}
}

// parseYtDlpLine parses a single yt-dlp JSON line and appends it to results if unique.
func parseYtDlpLine(line []byte, videoIdSet map[string]bool, results *[]gin.H) {
	type ytItem struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Thumbnail   string `json:"thumbnail"`
		Channel     string `json:"channel"`
		ChannelID   string `json:"channel_id"`
	}
	var it ytItem
	if err := json.Unmarshal(bytes.TrimSpace(line), &it); err != nil {
		if len(bytes.TrimSpace(line)) > 0 {
			TrailarrLog(WARN, "YouTube", "Failed to parse yt-dlp output line: %s | error: %v", string(line), err)
		}
		return
	}
	TrailarrLog(DEBUG, "YouTube", "Parsed yt-dlp item: id=%s title=%s", it.ID, it.Title)
	if it.ID == "" {
		return
	}
	if videoIdSet[it.ID] {
		return
	}
	*results = append(*results, gin.H{
		"id": gin.H{"videoId": it.ID},
		"snippet": gin.H{
			"title":       it.Title,
			"description": it.Description,
			"thumbnails": gin.H{
				"default": gin.H{"url": it.Thumbnail},
			},
			"channelTitle": it.Channel,
			"channelId":    it.ChannelID,
		},
	})
	videoIdSet[it.ID] = true
}
