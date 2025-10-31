package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var GlobalSyncQueue []TaskStatus

// Configurable task timings (tests may shorten these)
var TasksInitialDelay = time.Duration(0)

// Parametric force sync for Radarr/Sonarr
type SyncQueueItem struct {
	TaskId   string
	Queued   time.Time
	Started  time.Time
	Ended    time.Time
	Duration time.Duration
	Status   string
	Error    string
}

// Unified struct for queue, persistent state, and reporting
type TaskStatus struct {
	TaskId        string    `json:"taskId"`
	Name          string    `json:"name,omitempty"`
	Queued        time.Time `json:"queued,omitempty"`
	Started       time.Time `json:"started,omitempty"`
	Ended         time.Time `json:"ended,omitempty"`
	Duration      float64   `json:"duration,omitempty"`
	Interval      int       `json:"interval,omitempty"`
	LastExecution time.Time `json:"lastExecution,omitempty"`
	NextExecution time.Time `json:"nextExecution,omitempty"`
	Status        string    `json:"status"`
	Error         string    `json:"error,omitempty"`
}

// Unified Task struct: combines metadata, state, and scheduling info
type Task struct {
	Meta      TaskMeta
	State     TaskState
	Interval  int
	LogPrefix string
}

// Unified TaskSchedule struct for status/schedule reporting
type TaskSchedule struct {
	TaskID        TaskID    `json:"taskId"`
	Name          string    `json:"name"`
	Interval      int       `json:"interval"`
	LastExecution time.Time `json:"lastExecution"`
	LastDuration  float64   `json:"lastDuration"`
	NextExecution time.Time `json:"nextExecution"`
	Status        string    `json:"status"`
}

var taskStatusClientsMu sync.Mutex
var taskStatusClients = make(map[*websocket.Conn]struct{})

// Broadcasts the full status of all tasks, ignoring partial input
func broadcastTaskStatus(_ map[string]interface{}) {
	// Always send the current status of all tasks
	status := getCurrentTaskStatus()
	taskStatusClientsMu.Lock()
	for conn := range taskStatusClients {
		sendTaskStatus(conn, status)
	}
	taskStatusClientsMu.Unlock()
}

var GlobalTaskStates = make(TaskStates)

// Note: task_times are stored in the embedded store (TaskTimesStoreKey). Disk file support removed.

// TaskID is a string identifier for a scheduled task
type TaskID string

// TaskMeta holds static metadata for a task, including its function and order
type TaskMeta struct {
	ID       TaskID
	Name     string
	Function func()
	Order    int
}

// TaskState holds the persistent state for a scheduled task
type TaskState struct {
	ID            TaskID    `json:"taskId"`
	LastExecution time.Time `json:"lastExecution"`
	LastDuration  float64   `json:"lastDuration"`
	Status        string    `json:"status"`
}

// TaskStates maps TaskID to TaskState
type TaskStates map[TaskID]TaskState

// tasksMeta holds all static task metadata, including the function
var tasksMeta map[TaskID]TaskMeta

func init() {
	// On startup, update any 'running' tasks in the store queue to 'queued'
	client := GetStoreClient()
	ctx := context.Background()
	vals, err := client.LRange(ctx, TaskQueueStoreKey, 0, -1)
	if err == nil {
		for i, v := range vals {
			var qi SyncQueueItem
			if err := json.Unmarshal([]byte(v), &qi); err == nil {
				if qi.Status == "running" {
					qi.Status = "queued"
					if b, err := json.Marshal(qi); err == nil {
						// set the element back at index i
						_ = client.LSet(ctx, TaskQueueStoreKey, int64(i), b)
					}
				}
			}
		}
	}
	tasksMeta = map[TaskID]TaskMeta{
		"healthcheck": {ID: "healthcheck", Name: "Health Check", Function: wrapWithQueue("healthcheck", func() error { runHealthCheckTask(); return nil }), Order: 0},
		"radarr":      {ID: "radarr", Name: "Sync with Radarr", Function: wrapWithQueue("radarr", func() error { return SyncMediaType(MediaTypeMovie) }), Order: 1},
		"sonarr":      {ID: "sonarr", Name: "Sync with Sonarr", Function: wrapWithQueue("sonarr", func() error { return SyncMediaType(MediaTypeTV) }), Order: 2},
		"extras":      {ID: "extras", Name: "Search for Missing Extras", Function: wrapWithQueue("extras", func() error { processExtras(context.Background()); return nil }), Order: 3},
	}
}

// Helper to get all known TaskIDs
func AllTaskIDs() []TaskID {
	ids := make([]TaskID, 0, len(tasksMeta))
	for id := range tasksMeta {
		ids = append(ids, id)
	}
	return ids
}

func LoadTaskStates() (TaskStates, error) {
	// Store-backed task states; disk fallback removed
	client := GetStoreClient()
	ctx := context.Background()
	vals, err := client.LRange(ctx, TaskTimesStoreKey, 0, -1)

	states := make(TaskStates)
	if err == nil && len(vals) > 0 {
		states = parseStatesFromVals(vals)
	}

	if len(states) == 0 {
		initializeDefaultStates(states)
		_ = saveTaskStates(states)
	}

	ensureAllTasksExist(states)

	GlobalTaskStates = states
	return states, nil
}

// parseStatesFromVals parses JSON entries from the store into TaskStates and marks them idle.
func parseStatesFromVals(vals []string) TaskStates {
	states := make(TaskStates)
	for _, v := range vals {
		var t TaskState
		if err := json.Unmarshal([]byte(v), &t); err == nil {
			t.Status = "idle"
			states[t.ID] = t
		}
	}
	return states
}

// initializeDefaultStates populates states with sensible defaults based on Timings.
func initializeDefaultStates(states TaskStates) {
	zeroTime := time.Time{}
	for id := range tasksMeta {
		interval := 0
		if v, ok := Timings[string(id)]; ok {
			interval = v
		}
		if interval == 0 {
			states[id] = TaskState{ID: id, LastExecution: time.Now(), LastDuration: 0}
		} else {
			states[id] = TaskState{ID: id, LastExecution: zeroTime, LastDuration: 0}
		}
	}
}

// ensureAllTasksExist makes sure every task from tasksMeta has an entry in states.
func ensureAllTasksExist(states TaskStates) {
	for id := range tasksMeta {
		if _, ok := states[id]; !ok {
			states[id] = TaskState{ID: id}
		}
	}
}

func saveTaskStates(states TaskStates) error {
	GlobalTaskStates = states
	arr := make([]struct {
		ID            TaskID    `json:"taskId"`
		LastExecution time.Time `json:"lastExecution"`
		LastDuration  float64   `json:"lastDuration"`
	}, 0, len(states))
	for id, t := range states {
		taskId := t.ID
		if taskId == "" {
			taskId = id
		}
		arr = append(arr, struct {
			ID            TaskID    `json:"taskId"`
			LastExecution time.Time `json:"lastExecution"`
			LastDuration  float64   `json:"lastDuration"`
		}{
			ID:            taskId,
			LastExecution: t.LastExecution,
			LastDuration:  t.LastDuration,
		})
	}
	// Persist to the store as list of task states (overwrite by deleting and RPUSH)
	client := GetStoreClient()
	ctx := context.Background()
	_ = client.Del(ctx, TaskTimesStoreKey)
	for _, s := range arr {
		if b, err := json.Marshal(s); err == nil {
			_ = client.RPush(ctx, TaskTimesStoreKey, b)
		}
	}
	return nil
}

func GetAllTasksStatus() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Use running flags for Radarr/Sonarr
		states := GlobalTaskStates
		schedules := buildSchedules(states)
		respondJSON(c, http.StatusOK, gin.H{
			"schedules": schedules,
		})
	}
}

// Helper to calculate next execution time
func calcNext(lastExecution time.Time, interval int) time.Time {
	if lastExecution.IsZero() {
		return time.Now().Add(time.Duration(interval) * time.Minute)
	}
	return lastExecution.Add(time.Duration(interval) * time.Minute)
}

// Helper to build schedules array
func buildSchedules(states TaskStates) []TaskSchedule {
	schedules := make([]TaskSchedule, 0, len(tasksMeta))
	// Build a slice of (order, id) pairs for sorting
	type orderedTask struct {
		order int
		id    TaskID
	}
	ordered := make([]orderedTask, 0, len(tasksMeta))
	for id, meta := range tasksMeta {
		ordered = append(ordered, orderedTask{order: meta.Order, id: id})
	}
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].order < ordered[j].order
	})
	for _, ot := range ordered {
		meta := tasksMeta[ot.id]
		state := states[ot.id]
		interval := Timings[string(ot.id)]
		schedules = append(schedules, TaskSchedule{
			TaskID:        state.ID,
			Name:          meta.Name,
			Interval:      interval,
			LastExecution: state.LastExecution,
			LastDuration:  state.LastDuration,
			NextExecution: calcNext(state.LastExecution, interval),
			Status:        state.Status,
		})
	}
	return schedules
}

func buildTaskQueues() []TaskStatus {
	// Read queue from the persistent store so callers get the same data
	// as the file-based API handler.
	client := GetStoreClient()
	ctx := context.Background()
	vals, err := client.LRange(ctx, TaskQueueStoreKey, 0, -1)
	if err != nil {
		return []TaskStatus{}
	}
	queues := make([]TaskStatus, 0, len(vals))
	for _, v := range vals {
		var qi SyncQueueItem
		if err := json.Unmarshal([]byte(v), &qi); err != nil {
			continue
		}
		queues = append(queues, TaskStatus{
			TaskId:   qi.TaskId,
			Queued:   qi.Queued,
			Started:  qi.Started,
			Ended:    qi.Ended,
			Duration: qi.Duration.Seconds(),
			Status:   qi.Status,
			Error:    qi.Error,
		})
	}
	sortTaskQueuesByQueuedDesc(queues)
	if len(queues) > 100 {
		queues = queues[:100]
	}
	return queues
}

func sortTaskQueuesByQueuedDesc(queues []TaskStatus) {
	sort.Slice(queues, func(i, j int) bool {
		return queues[i].Queued.After(queues[j].Queued)
	})
}

// pushTaskQueueItem appends a sync queue item to the store list
func pushTaskQueueItem(item SyncQueueItem) error {
	client := GetStoreClient()
	ctx := context.Background()
	b, err := json.Marshal(item)
	if err != nil {
		return err
	}
	if err := client.RPush(ctx, TaskQueueStoreKey, b); err != nil {
		TrailarrLog(WARN, "Tasks", "Failed to RPush queue item TaskId=%s Queued=%s: %v", item.TaskId, item.Queued, err)
		return err
	}
	TrailarrLog(INFO, "Tasks", "Pushed queue item TaskId=%s Queued=%s Status=%s", item.TaskId, item.Queued, item.Status)
	// Diagnostic: log current list length after push
	if valsAfterPush, err := client.LRange(ctx, TaskQueueStoreKey, 0, -1); err == nil {
		TrailarrLog(DEBUG, "Tasks", "Queue length after RPush: %d", len(valsAfterPush))
	}

	// Trim to reasonable max
	if err := client.LTrim(ctx, TaskQueueStoreKey, -int64(TaskQueueMaxLen), -1); err != nil {
		TrailarrLog(WARN, "Tasks", "Failed to LTrim queue key %s: %v", TaskQueueStoreKey, err)
		return err
	}
	// Diagnostic: log current list length after trim
	if valsAfterTrim, err := client.LRange(ctx, TaskQueueStoreKey, 0, -1); err == nil {
		TrailarrLog(DEBUG, "Tasks", "Trimmed queue key %s to max len %d; current length=%d", TaskQueueStoreKey, TaskQueueMaxLen, len(valsAfterTrim))
	} else {
		TrailarrLog(DEBUG, "Tasks", "Trimmed queue key %s to max len %d", TaskQueueStoreKey, TaskQueueMaxLen)
	}
	return nil
}

// updateTaskQueueItem searches from the end to find a matching TaskId+Queued and updates it.
// NOTE: This function will only update an item when both TaskId and Queued match exactly.
// The previous behavior attempted a fallback update of the most recent 'running' item for
// the same TaskId which could overwrite unrelated entries; that fallback has been removed
// so updates are strictly exact-match only.
func updateTaskQueueItem(taskId string, queued time.Time, update func(*SyncQueueItem)) error {
	client := GetStoreClient()
	ctx := context.Background()
	vals, err := client.LRange(ctx, TaskQueueStoreKey, 0, -1)
	if err != nil {
		return err
	}

	// search from the back for an exact match on TaskId + Queued
	for i := len(vals) - 1; i >= 0; i-- {
		var qi SyncQueueItem
		if err := json.Unmarshal([]byte(vals[i]), &qi); err != nil {
			continue
		}
		if qi.TaskId == taskId && qi.Queued.Equal(queued) {
			update(&qi)
			b, err := json.Marshal(qi)
			if err != nil {
				TrailarrLog(WARN, "Tasks", "Failed to marshal updated queue item TaskId=%s Queued=%s: %v", qi.TaskId, qi.Queued, err)
				return err
			}
			TrailarrLog(DEBUG, "Tasks", "Updating queue key=%s index=%d total=%d TaskId=%s Queued=%s", TaskQueueStoreKey, i, len(vals), qi.TaskId, qi.Queued)
			if err := client.LSet(ctx, TaskQueueStoreKey, int64(i), b); err != nil {
				TrailarrLog(WARN, "Tasks", "Failed to LSet queue item at index %d TaskId=%s Queued=%s: %v", i, qi.TaskId, qi.Queued, err)
				return err
			}
			TrailarrLog(INFO, "Tasks", "Updated queue item TaskId=%s Queued=%s Status=%s", qi.TaskId, qi.Queued, qi.Status)
			return nil
		}
	}

	TrailarrLog(DEBUG, "Tasks", "No exact-matching queue item found to update for TaskId=%s Queued=%s; appending new entry", taskId, queued)

	// No exact match found: create a new entry with the provided queued timestamp
	// and apply the update callback, then append it so history still records the run.
	var newItem SyncQueueItem
	newItem.TaskId = taskId
	newItem.Queued = queued
	update(&newItem)
	if err := pushTaskQueueItem(newItem); err != nil {
		TrailarrLog(WARN, "Tasks", "Failed to push fallback-new queue item TaskId=%s Queued=%s: %v", newItem.TaskId, newItem.Queued, err)
		return err
	}
	TrailarrLog(INFO, "Tasks", "Appended new queue item TaskId=%s Queued=%s Status=%s", newItem.TaskId, newItem.Queued, newItem.Status)
	return nil
}

func TaskHandler() gin.HandlerFunc {
	// Build tasks map from tasksMeta
	type forceTask struct {
		id       TaskID
		started  *bool
		syncFunc func()
		respond  string
	}
	tasks := make(map[string]forceTask)
	for id, meta := range tasksMeta {
		if meta.Function == nil {
			TrailarrLog(WARN, "Tasks", "No sync function for taskId=%s", id)
			continue
		}
		tasks[string(id)] = forceTask{
			id:       id,
			started:  nil,
			syncFunc: meta.Function,
			respond:  fmt.Sprintf("Sync %s forced", meta.Name),
		}
	}
	return func(c *gin.Context) {
		var req struct {
			TaskId string `json:"taskId"`
		}
		if err := c.BindJSON(&req); err != nil {
			respondError(c, http.StatusBadRequest, "invalid request")
			return
		}
		println("[FORCE] Requested force execution for:", req.TaskId)
		t, ok := tasks[req.TaskId]
		if !ok {
			respondError(c, http.StatusBadRequest, "unknown task")
			return
		}
		// Run all tasks async, status managed in goroutine
		go func(taskId TaskID, syncFunc func()) {
			// Copy current in-memory state to avoid overwriting other running statuses
			states := make(TaskStates)
			for k, v := range GlobalTaskStates {
				states[k] = v
			}
			// Set running flag for this task only
			states[taskId] = TaskState{
				ID:            taskId,
				LastExecution: states[taskId].LastExecution,
				LastDuration:  states[taskId].LastDuration,
				Status:        "running",
			}
			GlobalTaskStates = states
			broadcastTaskStatus(getCurrentTaskStatus())
			start := time.Now()
			syncFunc()
			duration := time.Since(start)
			// Set idle flag for this task only
			states[taskId] = TaskState{
				ID:            taskId,
				LastExecution: start,
				LastDuration:  duration.Seconds(),
				Status:        "idle",
			}
			GlobalTaskStates = states
			broadcastTaskStatus(getCurrentTaskStatus())
			saveTaskStates(states)
		}(t.id, t.syncFunc)
		respondJSON(c, http.StatusOK, gin.H{"status": t.respond})
	}
}

type bgTask struct {
	id        TaskID
	started   *bool
	syncFunc  func()
	interval  time.Duration
	lastExec  time.Time
	logPrefix string
}

func StartBackgroundTasks() {
	TrailarrLog(INFO, "Tasks", "StartBackgroundTasks called. PID=%d, time=%s", os.Getpid(), time.Now().Format(time.RFC3339Nano))
	states, err := LoadTaskStates()
	if err != nil {
		TrailarrLog(WARN, "Tasks", "Could not load last task times: %v", err)
	}

	taskList := buildBgTasks(states)

	for i := range taskList {
		t := taskList[i]
		if t.interval <= 0 {
			TrailarrLog(WARN, "Tasks", "Task %s has non-positive interval, skipping scheduling", t.logPrefix)
			continue
		}
		go scheduleTask(t)
	}
	TrailarrLog(INFO, "Tasks", "Native Go scheduler started. Jobs will persist last execution times to store key %s", TaskTimesStoreKey)
}

func buildBgTasks(states TaskStates) []bgTask {
	var taskList []bgTask
	for id, meta := range tasksMeta {
		intervalVal, ok := Timings[string(id)]
		if !ok {
			TrailarrLog(WARN, "Tasks", "No interval found in Timings for %s", id)
			intervalVal = 0
		}
		interval := time.Duration(intervalVal) * time.Minute
		lastExec := states[id].LastExecution
		if meta.Function == nil {
			TrailarrLog(WARN, "Tasks", "No sync function for taskId=%s", id)
			continue
		}
		taskList = append(taskList, bgTask{
			id:        id,
			started:   nil,
			syncFunc:  meta.Function,
			interval:  interval,
			lastExec:  lastExec,
			logPrefix: meta.Name,
		})
	}
	return taskList
}

func scheduleTask(t bgTask) {
	now := time.Now()
	initialDelay := t.lastExec.Add(t.interval).Sub(now)
	if initialDelay < 0 {
		initialDelay = 0
	}
	// Allow override for tests (TasksInitialDelay) when set to non-zero
	if TasksInitialDelay > 0 {
		time.Sleep(TasksInitialDelay)
	} else {
		time.Sleep(initialDelay)
	}

	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	for {
		if t.id == "extras" {
			// Wait until radarr and sonarr have executed at least once
			for {
				st := GlobalTaskStates
				radLast := st["radarr"].LastExecution
				sonLast := st["sonarr"].LastExecution
				if !radLast.IsZero() && !sonLast.IsZero() {
					break
				}
				TrailarrLog(INFO, "Tasks", "Waiting for radarr/sonarr to run before extras")
				time.Sleep(TasksDepsWaitInterval)
			}
		}
		go runTaskAsync(TaskID(t.id), t.syncFunc)
		<-ticker.C
	}
}

func processExtras(ctx context.Context) {
	// Clean all 429 rejections before starting extras task
	if err := RemoveAll429Rejections(); err != nil {
		TrailarrLog(WARN, "Tasks", "Failed to clean 429 rejections: %v", err)
	} else {
		TrailarrLog(INFO, "Tasks", "Cleaned all 429 rejections before starting extras task.")
	}
	extraTypesCfg, err := GetExtraTypesConfig()
	if err != nil {
		TrailarrLog(WARN, "Tasks", "Could not load extra types config: %v", err)
		return
	}
	TrailarrLog(INFO, "Tasks", "[TASK] Searching for missing movie extras...")
	downloadMissingExtrasWithTypeFilter(ctx, extraTypesCfg, MediaTypeMovie, MoviesStoreKey)
	TrailarrLog(INFO, "Tasks", "[TASK] Searching for missing series extras...")
	downloadMissingExtrasWithTypeFilter(ctx, extraTypesCfg, MediaTypeTV, SeriesStoreKey)
}

func StopExtrasDownloadTask() {
	states, _ := LoadTaskStates()
	if states["extras"].Status == "running" {
		TrailarrLog(INFO, "Tasks", "Stopping extras download task... extrasTaskState.Status=%v", states["extras"].Status)
		states["extras"] = TaskState{
			ID:            "extras",
			LastExecution: states["extras"].LastExecution,
			LastDuration:  states["extras"].LastDuration,
			Status:        "idle",
		}
		saveTaskStates(states)
	} else {
		TrailarrLog(INFO, "Tasks", "StopExtrasDownloadTask called but extrasTaskState.Status is not running")
	}
}

// Shared logic for type-filtered extras download
func downloadMissingExtrasWithTypeFilter(ctx context.Context, cfg ExtraTypesConfig, mediaType MediaType, cacheFile string) {

	// Prefer using the lightweight wanted index to avoid scanning the full cache.
	useWantedIndex := false
	items, err := LoadWantedIndex(cacheFile)
	if err == nil && len(items) > 0 {
		TrailarrLog(INFO, "Tasks", "downloadMissingExtrasWithTypeFilter: using wanted index with %d items for cache=%s mediaType=%v", len(items), cacheFile, mediaType)
		useWantedIndex = true
	} else {
		// Fallback to the full cache when wanted index not available
		items, err = loadCache(cacheFile)
		if err != nil {
			TrailarrLog(WARN, "Tasks", "downloadMissingExtrasWithTypeFilter: failed to load cache %s: %v", cacheFile, err)
			return
		}
		TrailarrLog(DEBUG, "Tasks", "downloadMissingExtrasWithTypeFilter: loaded %d items from cache=%s for mediaType=%v", len(items), cacheFile, mediaType)
	}

	enabledTypes := GetEnabledCanonicalExtraTypes(cfg)
	TrailarrLog(DEBUG, "Tasks", "downloadMissingExtrasWithTypeFilter: enabledTypes=%v for mediaType=%v cache=%s useWantedIndex=%v", enabledTypes, mediaType, cacheFile, useWantedIndex)

	// Filter items: if we used the wanted index the items are already wanted-light entries
	wantedItems := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		if include, mediaId := shouldIncludeWantedItem(item, useWantedIndex, mediaType, enabledTypes, cacheFile); include {
			wantedItems = append(wantedItems, item)
		} else {
			// extra debug already logged by helper when skipping
			_ = mediaId
		}
	}

	TrailarrLog(INFO, "Tasks", "downloadMissingExtrasWithTypeFilter: %d wanted items after filtering for cache=%s mediaType=%v", len(wantedItems), cacheFile, mediaType)
	for _, item := range wantedItems {
		if ctx != nil && ctx.Err() != nil {
			TrailarrLog(INFO, "Tasks", "Extras download cancelled before processing item.")
			break
		}
		processWantedItem(ctx, cfg, mediaType, cacheFile, item, enabledTypes)
	}
}

// Helper: determine whether an item should be included in wantedItems
func shouldIncludeWantedItem(item map[string]interface{}, useWantedIndex bool, mediaType MediaType, enabledTypes []string, cacheFile string) (bool, int) {
	idRaw := item["id"]
	titleRaw := item["title"]
	TrailarrLog(DEBUG, "Tasks", "downloadMissingExtrasWithTypeFilter: inspecting item id=%v title=%v cache=%s", idRaw, titleRaw, cacheFile)
	mediaId, ok := parseMediaID(item["id"])
	if !ok {
		TrailarrLog(DEBUG, "Tasks", "downloadMissingExtrasWithTypeFilter: failed to parse media id for raw=%v, skipping", idRaw)
		return false, 0
	}
	if !useWantedIndex {
		if !isMediaWanted(item) {
			TrailarrLog(DEBUG, "Tasks", "downloadMissingExtrasWithTypeFilter: mediaId=%d not wanted, skipping", mediaId)
			return false, mediaId
		}
	}
	hasAny := HasAnyEnabledExtras(mediaType, mediaId, enabledTypes)
	if hasAny {
		TrailarrLog(DEBUG, "Tasks", "downloadMissingExtrasWithTypeFilter: mediaId=%d already has enabled extras, skipping", mediaId)
		return false, mediaId
	}
	TrailarrLog(DEBUG, "Tasks", "downloadMissingExtrasWithTypeFilter: mediaId=%d wanted and missing extras, adding to wantedItems", mediaId)
	return true, mediaId
}

// processWantedItem encapsulates per-item processing previously inline in the large function.
func processWantedItem(ctx context.Context, cfg ExtraTypesConfig, mediaType MediaType, cacheFile string, item map[string]interface{}, enabledTypes interface{}) {
	mediaId, _ := parseMediaID(item["id"])
	title, _ := item["title"].(string)

	TrailarrLog(DEBUG, "Tasks", "processWantedItem: processing mediaType=%v mediaId=%d title=%q cache=%s enabledTypes=%v", mediaType, mediaId, title, cacheFile, enabledTypes)

	extras, usedTMDB, err := fetchExtrasOrTMDB(mediaType, mediaId, title, enabledTypes)
	if err != nil {
		TrailarrLog(WARN, "Tasks", "SearchExtras/TMDB failed for mediaId=%v, title=%q: %v", mediaId, title, err)
		return
	}
	TrailarrLog(DEBUG, "Tasks", "processWantedItem: fetched extras count=%d usedTMDB=%v for mediaId=%d title=%q", len(extras), usedTMDB, mediaId, title)
	if len(extras) == 0 {
		// Nothing to do
		return
	}

	mediaPath, err := FindMediaPathByID(cacheFile, mediaId)
	if err != nil || mediaPath == "" {
		TrailarrLog(WARN, "Tasks", "FindMediaPathByID failed for mediaId=%v, title=%q cache=%s: %v", mediaId, title, cacheFile, err)
		return
	}

	TrailarrLog(INFO, "Tasks", "Searching extras for %s %v: %s", mediaType, mediaId, item["title"])

	var toDownload []Extra
	if usedTMDB {
		toDownload = extras
	} else {
		MarkDownloadedExtras(extras, mediaPath, "type", "title")
		// Defensive: mark rejected extras before any download
		rejectedExtras := GetRejectedExtrasForMedia(mediaType, mediaId)
		rejectedYoutubeIds := make(map[string]struct{}, len(rejectedExtras))
		for _, r := range rejectedExtras {
			rejectedYoutubeIds[r.YoutubeId] = struct{}{}
		}
		MarkRejectedExtrasInMemory(extras, rejectedYoutubeIds)
		toDownload = extras
	}
	TrailarrLog(DEBUG, "Tasks", "processWantedItem: mediaId=%d toDownload count=%d usedTMDB=%v mediaPath=%s", mediaId, len(toDownload), usedTMDB, mediaPath)

	// For each extra, download sequentially using a helper to reduce nesting.
	for _, extra := range toDownload {
		if ctx != nil && ctx.Err() != nil {
			TrailarrLog(INFO, "Tasks", "Extras download cancelled before processing extra.")
			break
		}
		processExtraDownload(cfg, mediaType, mediaId, extra, usedTMDB)
	}
}

// fetchExtrasOrTMDB centralizes SearchExtras + TMDB fallback and reduces branching in the caller.
func fetchExtrasOrTMDB(mediaType MediaType, mediaId int, title string, enabledTypes interface{}) ([]Extra, bool, error) {
	extras, err := SearchExtras(mediaType, mediaId)
	if err != nil {
		return nil, false, err
	}
	if len(extras) == 0 {
		TrailarrLog(INFO, "Tasks", "No extras found for mediaId=%v, title=%q, enabledTypes=%v, attempting TMDB fetch...", mediaId, title, enabledTypes)
		tmdbExtras, err := FetchTMDBExtrasForMedia(mediaType, mediaId)
		if err != nil {
			return nil, false, err
		}
		if len(tmdbExtras) == 0 {
			TrailarrLog(INFO, "Tasks", "Still no extras after TMDB fetch for mediaId=%v, title=%q", mediaId, title)
			return nil, false, nil
		}
		return tmdbExtras, true, nil
	}
	return extras, false, nil
}

// processExtraDownload handles the per-extra checks and enqueues downloads when appropriate.
func processExtraDownload(cfg ExtraTypesConfig, mediaType MediaType, mediaId int, extra Extra, usedTMDB bool) {
	typ := canonicalizeExtraType(extra.ExtraType)
	TrailarrLog(DEBUG, "Tasks", "processExtraDownload: mediaId=%d extraType=%s status=%s youtubeId=%s usedTMDB=%v", mediaId, extra.ExtraType, extra.Status, extra.YoutubeId, usedTMDB)
	if !isExtraTypeEnabled(cfg, typ) {
		TrailarrLog(DEBUG, "Tasks", "processExtraDownload: extra type %s disabled by config, skipping mediaId=%d", typ, mediaId)
		return
	}
	// Only check rejection for local extras, not TMDB-fetched
	if !usedTMDB && extra.Status == "rejected" {
		TrailarrLog(DEBUG, "Tasks", "processExtraDownload: extra rejected locally, skipping mediaId=%d youtubeId=%s", mediaId, extra.YoutubeId)
		return
	}
	// For TMDB-fetched, always treat as missing if not present locally
	if (usedTMDB && extra.YoutubeId != "") || (!usedTMDB && extra.Status == "missing" && extra.YoutubeId != "") {
		TrailarrLog(INFO, "Tasks", "processExtraDownload: queuing extra mediaId=%d type=%s title=%q youtubeId=%s usedTMDB=%v", mediaId, extra.ExtraType, extra.ExtraTitle, extra.YoutubeId, usedTMDB)
		if err := handleTypeFilteredExtraDownload(mediaType, mediaId, extra); err != nil {
			TrailarrLog(WARN, "Tasks", "[SEQ] Download failed: %v", err)
		}
	} else {
		TrailarrLog(DEBUG, "Tasks", "processExtraDownload: extra does not meet download criteria for mediaId=%d youtubeId=%s status=%s usedTMDB=%v", mediaId, extra.YoutubeId, extra.Status, usedTMDB)
	}
}

// Handles downloading a single extra and appending to history if successful
func handleTypeFilteredExtraDownload(mediaType MediaType, mediaId int, extra Extra) error {
	// Enqueue the extra for download using the queue system
	item := DownloadQueueItem{
		MediaType:  mediaType,
		MediaId:    mediaId,
		ExtraType:  extra.ExtraType,
		ExtraTitle: extra.ExtraTitle,
		YouTubeID:  extra.YoutubeId,
		QueuedAt:   time.Now(),
	}
	// Wait for any currently queued download items to drain before enqueuing
	// to avoid flooding the queue when many extras are discovered by the task.
	waitForDownloadQueueDrain(mediaId, extra.YoutubeId)
	AddToDownloadQueue(item, "task")
	TrailarrLog(INFO, "QUEUE", "[handleTypeFilteredExtraDownload] Enqueued extra: mediaType=%v, mediaId=%v, type=%s, title=%s, youtubeId=%s", mediaType, mediaId, extra.ExtraType, extra.ExtraTitle, extra.YoutubeId)

	// Do not record a "queued" history event here. The downloader will record
	// the final "download" event when the download completes.
	_, _ = resolveCachePath(mediaType) // keep functionality that may require cache resolution
	return nil
}

// waitForDownloadQueueDrain polls the persistent download queue until there are
// no items with status 'queued'. It logs and sleeps between attempts.
func waitForDownloadQueueDrain(mediaId int, youtubeId string) {
	TrailarrLog(INFO, "Tasks", "Waiting for download queue to drain before enqueuing extra: mediaId=%d youtubeId=%s", mediaId, youtubeId)
	for {
		if !isDownloadQueueQueuedPresent() {
			return
		}
		time.Sleep(DownloadQueueWatcherInterval)
	}
}

// isDownloadQueueQueuedPresent returns true if the persistent download queue
// contains any items with Status == "queued".
func isDownloadQueueQueuedPresent() bool {
	ctx := context.Background()
	client := GetStoreClient()
	vals, err := client.LRange(ctx, DownloadQueue, 0, -1)
	if err != nil {
		return false
	}
	for _, v := range vals {
		var q DownloadQueueItem
		if err := json.Unmarshal([]byte(v), &q); err == nil {
			if q.Status == "queued" {
				return true
			}
		}
	}
	return false
}

// Helper: check if extra type is enabled in config
func isExtraTypeEnabled(cfg ExtraTypesConfig, typ string) bool {
	switch typ {
	case "Trailers":
		return cfg.Trailers
	case "Scenes":
		return cfg.Scenes
	case "Behind The Scenes":
		return cfg.BehindTheScenes
	case "Interviews":
		return cfg.Interviews
	case "Featurettes":
		return cfg.Featurettes
	case "Deleted Scenes":
		return cfg.DeletedScenes
	case "Other":
		return cfg.Other
	default:
		return false
	}
}

func getWebSocketUpgrader() *websocket.Upgrader {
	return &websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
}

func addTaskStatusClient(conn *websocket.Conn) {
	taskStatusClientsMu.Lock()
	taskStatusClients[conn] = struct{}{}
	taskStatusClientsMu.Unlock()
	// Send initial status
	go sendCurrentTaskStatus(conn)
}

func removeTaskStatusClient(conn *websocket.Conn) {
	taskStatusClientsMu.Lock()
	delete(taskStatusClients, conn)
	taskStatusClientsMu.Unlock()
}

func sendCurrentTaskStatus(conn *websocket.Conn) {
	status := getCurrentTaskStatus()
	sendTaskStatus(conn, status)
}

func sendTaskStatus(conn *websocket.Conn, status interface{}) {
	data, err := json.Marshal(status)
	if err != nil {
		return
	}
	conn.WriteMessage(websocket.TextMessage, data)
}

// Returns a map with all tasks' current status for broadcasting
func getCurrentTaskStatus() map[string]interface{} {
	states := GlobalTaskStates
	return map[string]interface{}{
		"schedules": buildSchedules(states),
	}
}

// Helper to run a task async and manage status
func runTaskAsync(taskId TaskID, syncFunc func()) {
	// Set running flag
	GlobalTaskStates[taskId] = TaskState{
		ID:            taskId,
		LastExecution: GlobalTaskStates[taskId].LastExecution, // unchanged until end
		LastDuration:  GlobalTaskStates[taskId].LastDuration,
		Status:        "running",
	}
	broadcastTaskStatus(getCurrentTaskStatus())
	start := time.Now()
	syncFunc()
	duration := time.Since(start)
	// Set idle flag and update LastExecution to NOW (end of task)
	GlobalTaskStates[taskId] = TaskState{
		ID:            taskId,
		LastExecution: time.Now(),
		LastDuration:  duration.Seconds(),
		Status:        "idle",
	}
	broadcastTaskStatus(getCurrentTaskStatus())
	saveTaskStates(GlobalTaskStates)
}

// Handler to return only the queue items as 'queues' array
func GetTaskQueueHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		queues := buildTaskQueues()
		sortTaskQueuesByQueuedDesc(queues)
		c.JSON(http.StatusOK, gin.H{
			"queues": queues,
		})
	}
}

// Centralized queue wrapper for all tasks
func wrapWithQueue(taskId TaskID, syncFunc func() error) func() {
	return func() {
		// Add new queue item to the persistent store on start
		queued := time.Now()
		item := SyncQueueItem{
			TaskId:  string(taskId),
			Queued:  queued,
			Status:  "running",
			Started: queued,
		}
		_ = pushTaskQueueItem(item)

		err := syncFunc()
		ended := time.Now()
		duration := ended.Sub(queued)
		status := "success"
		if err != nil {
			status = "failed"
			TrailarrLog(ERROR, "Tasks", "Task %s error: %s", taskId, err.Error())
		} else {
			TrailarrLog(INFO, "Tasks", "Task %s completed successfully.", taskId)
		}
		// Update the last queue item for this task (by TaskId and Queued) in the persistent store
		_ = updateTaskQueueItem(string(taskId), queued, func(qi *SyncQueueItem) {
			qi.Status = status
			qi.Started = queued
			qi.Ended = ended
			qi.Duration = duration
			if err != nil {
				qi.Error = err.Error()
			} else {
				qi.Error = ""
			}
		})
	}
}

// runHealthCheckTask performs provider connectivity checks and records any issues
// into the configured store key. If no issues are found the health issues key
// is cleared so the UI badge/count reflects the current state.
func runHealthCheckTask() {
	// Build issues similar to buildHealth but persist to store
	radarrURL, radarrKey, _ := GetProviderUrlAndApiKey("radarr")
	sonarrURL, sonarrKey, _ := GetProviderUrlAndApiKey("sonarr")

	var issues []HealthMsg
	if radarrURL == "" || radarrKey == "" {
		issues = append(issues, HealthMsg{Message: "Radarr not configured (missing URL or API key)", Source: "Radarr", Level: "warning"})
	} else {
		if err := testMediaConnection(radarrURL, radarrKey, "radarr"); err != nil {
			issues = append(issues, HealthMsg{Message: fmt.Sprintf("Radarr connectivity failed: %v", err), Source: "Radarr", Level: "warning"})
		}
	}

	if sonarrURL == "" || sonarrKey == "" {
		issues = append(issues, HealthMsg{Message: "Sonarr not configured (missing URL or API key)", Source: "Sonarr", Level: "warning"})
	} else {
		if err := testMediaConnection(sonarrURL, sonarrKey, "sonarr"); err != nil {
			issues = append(issues, HealthMsg{Message: fmt.Sprintf("Sonarr connectivity failed: %v", err), Source: "Sonarr", Level: "warning"})
		}
	}

	client := GetStoreClient()
	ctx := context.Background()
	// If no issues, clear the key so the UI stops showing stale problems
	if len(issues) == 0 {
		_ = client.Del(ctx, HealthIssuesStoreKey)
		TrailarrLog(DEBUG, "Tasks", "Health check: no issues found, cleared %s", HealthIssuesStoreKey)
		return
	}

	// Persist issues as JSON entries (one per issue).
	// Replace the existing list so repeated runs don't accumulate duplicates.
	_ = client.Del(ctx, HealthIssuesStoreKey)
	for _, h := range issues {
		if b, err := json.Marshal(h); err == nil {
			_ = client.RPush(ctx, HealthIssuesStoreKey, b)
		}
	}
	// Keep only recent 100 entries (defensive)
	_ = client.LTrim(ctx, HealthIssuesStoreKey, -100, -1)
	TrailarrLog(INFO, "Tasks", "Health check stored %d issue(s) to %s", len(issues), HealthIssuesStoreKey)
}
