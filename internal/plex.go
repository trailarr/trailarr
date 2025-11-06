package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

const (
	// PlexHeader is the standard header name for the Plex token
	PlexHeader = "X-Plex-Token"
)

// keysOfMap returns the sorted keys of the provided map. Used for debug logging.
func keysOfMap(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// performPlexSearch runs a Plex search for the given title and returns the
// HTTP status, raw body and the first ratingKey found (if any).
// plexType: 1=movie, 2=show
func performPlexSearch(base, title, token string, plexType int) (int, []byte, string, error) {
	u := base + "/search?query=" + url.QueryEscape(title) + "&type=" + strconv.Itoa(plexType)
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set(PlexHeader, token)
	req.Header.Set("Accept", "application/json")
	// Log search request (mask token)
	masked := token
	if len(masked) > 8 {
		masked = masked[:4] + "..." + masked[len(masked)-4:]
	}
	TrailarrLog(INFO, "Plex", "performPlexSearch: GET %s header %s=%s Accept=application/json", u, PlexHeader, masked)
	client := &http.Client{Timeout: 10 * time.Second}
	sresp, sErr := client.Do(req)
	var sBody []byte
	var sStatus int
	if sErr != nil {
		return 0, nil, "", sErr
	}
	sBody, _ = io.ReadAll(sresp.Body)
	_ = sresp.Body.Close()
	sStatus = sresp.StatusCode
	TrailarrLog(INFO, "Plex", "performPlexSearch: response status=%d bodyLen=%d for %s", sStatus, len(sBody), u)

	var ratingKey string
	if sStatus == 200 {
		var searchResp struct {
			MediaContainer struct {
				Metadata []struct {
					RatingKey string `json:"ratingKey"`
					Title     string `json:"title"`
				} `json:"Metadata"`
			} `json:"MediaContainer"`
		}
		if err := json.Unmarshal(sBody, &searchResp); err == nil && len(searchResp.MediaContainer.Metadata) > 0 {
			ratingKey = searchResp.MediaContainer.Metadata[0].RatingKey
		}
	}
	return sStatus, sBody, ratingKey, nil
}

// getFirstSearchItem parses the Plex search response body and returns the
// first Metadata item's ratingKey, librarySectionID (raw), and title.
func getFirstSearchItem(body []byte) (string, interface{}, string, error) {
	var searchResp struct {
		MediaContainer struct {
			Metadata []struct {
				RatingKey        string      `json:"ratingKey"`
				LibrarySectionID interface{} `json:"librarySectionID"`
				Title            string      `json:"title"`
			} `json:"Metadata"`
		} `json:"MediaContainer"`
	}
	if err := json.Unmarshal(body, &searchResp); err != nil {
		TrailarrLog(WARN, "Plex", "Failed to decode Plex search response: %v", err)
		return "", nil, "", err
	}
	if len(searchResp.MediaContainer.Metadata) == 0 {
		TrailarrLog(WARN, "Plex", "No matching show found in Plex for search response")
		return "", nil, "", fmt.Errorf("no matching show found in plex search response")
	}
	item := searchResp.MediaContainer.Metadata[0]
	if item.RatingKey == "" {
		TrailarrLog(WARN, "Plex", "Chosen Plex item has no ratingKey: %+v", item)
		return "", nil, "", fmt.Errorf("no ratingKey found for plex item")
	}
	return item.RatingKey, item.LibrarySectionID, item.Title, nil
}

// RefreshPlexShowMetadata attempts to find the corresponding show in Plex by title
// and requests a metadata refresh for that item. This is best-effort and will
// return an error if Plex is not configured or the show cannot be located.
func RefreshPlexShowMetadata(mediaId int) error {
	// Load Plex config
	cfg, err := GetPlexConfig()
	if err != nil {
		return err
	}
	if !cfg.Enabled || cfg.Token == "" || cfg.IP == "" {
		return fmt.Errorf("plex not configured or disabled")
	}

	// Resolve the media title from cache
	title := getMediaTitleFromCache(MediaTypeTV, mediaId)
	if title == "" {
		return fmt.Errorf("media title not found for id %d", mediaId)
	}

	base := fmt.Sprintf("%s://%s:%d", cfg.Protocol, cfg.IP, cfg.Port)

	// Try cache-based refresh first
	if ok, err := attemptCacheBasedPlexRefresh(mediaId, base, cfg.Token); err == nil && ok {
		return nil
	} else if err != nil {
		TrailarrLog(DEBUG, "Plex", "Cache-based plex refresh attempt failed: %v", err)
	}

	// Fall back to searching Plex and refreshing the found item (type=2 show)
	if err := searchAndRefresh(title, base, cfg.Token, 2); err != nil {
		return err
	}
	return nil
}

// RefreshPlexMovieMetadata attempts to find the corresponding movie in Plex by title
// and requests a metadata refresh for that item. Mirrors show behavior but searches
// Plex with type=1 (movie).
func RefreshPlexMovieMetadata(mediaId int) error {
	// Load Plex config
	cfg, err := GetPlexConfig()
	if err != nil {
		return err
	}
	if !cfg.Enabled || cfg.Token == "" || cfg.IP == "" {
		return fmt.Errorf("plex not configured or disabled")
	}

	// Resolve the media title from cache
	title := getMediaTitleFromCache(MediaTypeMovie, mediaId)
	if title == "" {
		return fmt.Errorf("media title not found for id %d", mediaId)
	}

	base := fmt.Sprintf("%s://%s:%d", cfg.Protocol, cfg.IP, cfg.Port)

	// Try cache-based refresh first (movie cache)
	if ok, err := attemptCacheBasedPlexRefreshMovie(mediaId, base, cfg.Token); err == nil && ok {
		return nil
	} else if err != nil {
		TrailarrLog(DEBUG, "Plex", "Cache-based plex refresh attempt failed: %v", err)
	}

	// Fall back to searching Plex and refreshing the found item (type=1 movie)
	if err := searchAndRefresh(title, base, cfg.Token, 1); err != nil {
		return err
	}
	return nil
}

// attemptCacheBasedPlexRefresh looks for candidate ratingKey fields in the series cache
// and tries a refresh for each candidate. Returns (true, nil) if a refresh was requested
// successfully (HTTP 200/204).
func attemptCacheBasedPlexRefresh(mediaId int, base, token string) (bool, error) {
	items, err := LoadMediaFromStore(SeriesStoreKey)
	if err != nil {
		return false, err
	}
	for _, it := range items {
		id, ok := it["id"]
		if !ok {
			continue
		}
		idInt, ok2 := parseMediaID(id)
		if !ok2 || idInt != mediaId {
			continue
		}
		TrailarrLog(DEBUG, "Plex", "Found series in cache for mediaId=%d: keys=%v", mediaId, keysOfMap(it))
		ok, err := tryCandidatesFromMap(it, base, token)
		if err != nil {
			TrailarrLog(DEBUG, "Plex", "tryCandidatesFromMap error: %v", err)
		}
		if ok {
			return true, nil
		}
		break
	}
	return false, nil
}

// attemptCacheBasedPlexRefreshMovie is the movie-specific variant that inspects
// the movies cache for known Plex ID keys and tries item refreshes.
func attemptCacheBasedPlexRefreshMovie(mediaId int, base, token string) (bool, error) {
	items, err := LoadMediaFromStore(MoviesStoreKey)
	if err != nil {
		return false, err
	}
	for _, it := range items {
		id, ok := it["id"]
		if !ok {
			continue
		}
		idInt, ok2 := parseMediaID(id)
		if !ok2 || idInt != mediaId {
			continue
		}
		TrailarrLog(DEBUG, "Plex", "Found movie in cache for mediaId=%d: keys=%v", mediaId, keysOfMap(it))
		ok, err := tryCandidatesFromMap(it, base, token)
		if err != nil {
			TrailarrLog(DEBUG, "Plex", "tryCandidatesFromMap error: %v", err)
		}
		if ok {
			return true, nil
		}
		break
	}
	return false, nil
}

// tryCandidatesFromMap inspects a series cache map for common Plex ID keys and tries refreshes.
func tryCandidatesFromMap(it map[string]interface{}, base, token string) (bool, error) {
	candidates := []string{"ratingKey", "plexRatingKey", "plexId", "rating_key", "plex_rating_key"}
	for _, k := range candidates {
		if v, ok := it[k]; ok {
			ratingKeyStr := fmt.Sprintf("%v", v)
			if ratingKeyStr == "" || ratingKeyStr == "0" {
				continue
			}
			// Log candidate we are about to try
			TrailarrLog(INFO, "Plex", "tryCandidatesFromMap: trying candidate key=%s value=%s for base=%s", k, ratingKeyStr, base)
			ok, err := tryRefreshRatingKey(base, ratingKeyStr, token)
			if err != nil {
				TrailarrLog(DEBUG, "Plex", "Plex refresh (cache) request error for ratingKey=%s: %v", ratingKeyStr, err)
				continue
			}
			if ok {
				return true, nil
			}
		}
	}
	return false, nil
}

// tryRefreshRatingKey attempts a single refresh for ratingKey and returns true on success (200/204).
func tryRefreshRatingKey(base, ratingKey, token string) (bool, error) {
	masked := token
	if len(masked) > 8 {
		masked = masked[:4] + "..." + masked[len(masked)-4:]
	}
	TrailarrLog(INFO, "Plex", "tryRefreshRatingKey: attempting item refresh for ratingKey=%s base=%s token=%s", ratingKey, base, masked)
	status, body, err := doRefreshRequest(base, ratingKey, token)
	if err != nil {
		TrailarrLog(INFO, "Plex", "Plex refresh (cache) request failed for ratingKey=%s err=%v", ratingKey, err)
	} else {
		TrailarrLog(INFO, "Plex", "Plex refresh (cache) response for ratingKey=%s status=%d bodyLen=%d", ratingKey, status, len(body))
	}
	if err != nil {
		return false, err
	}
	return status == 200 || status == 204, nil
}

// searchAndRefresh searches Plex for the given title (plexType: 1=movie,2=show),
// picks the first result and requests a metadata refresh for its ratingKey.
func searchAndRefresh(title, base, token string, plexType int) error {
	// Use centralized helper to perform Plex search and fetch first item's ratingKey
	TrailarrLog(DEBUG, "Plex", "searchAndRefresh: performing search for title=%q (type=%d)", title, plexType)
	sStatus, bodyBytes, _, sErr := performPlexSearch(base, title, token, plexType)
	if sErr != nil {
		TrailarrLog(WARN, "Plex", "Plex search request failed: %v", sErr)
		return sErr
	}
	if sStatus != 200 {
		return fmt.Errorf("plex search returned status %d", sStatus)
	}

	ratingKey, libSection, itemTitle, perr := getFirstSearchItem(bodyBytes)
	if perr != nil {
		return perr
	}
	TrailarrLog(DEBUG, "Plex", "Chosen Plex item: ratingKey=%s title=%s librarySectionID=%v", ratingKey, itemTitle, libSection)

	// Log and call item refresh
	TrailarrLog(INFO, "Plex", "searchAndRefresh: requesting item refresh for ratingKey=%s", ratingKey)
	status, body, err := doRefreshRequest(base, ratingKey, token)
	if err != nil {
		TrailarrLog(WARN, "Plex", "searchAndRefresh: item refresh request failed for ratingKey=%s err=%v", ratingKey, err)
	} else {
		TrailarrLog(DEBUG, "Plex", "searchAndRefresh: item refresh response for ratingKey=%s status=%d bodyLen=%d", ratingKey, status, len(body))
	}
	if err != nil {
		TrailarrLog(WARN, "Plex", "Plex refresh request failed: %v", err)
		return err
	}
	if status == 200 || status == 204 {
		TrailarrLog(INFO, "Plex", "Requested metadata refresh for Plex ratingKey=%s (title=%s)", ratingKey, title)
		return nil
	}

	// Do NOT attempt section-level refresh here. We want a failure if the
	// specific item refresh is rejected so callers (and admins) can diagnose
	// the root cause (proxy rules, Plex permissions, incorrect path).
	TrailarrLog(INFO, "Plex", "Plex item refresh for ratingKey=%s returned status %d; not falling back to section refresh", ratingKey, status)
	return fmt.Errorf("plex item refresh returned status %d", status)
}

// doRefreshRequest performs the single metadata refresh request for a ratingKey and
// returns the HTTP status and body (or an error).
func doRefreshRequest(base, ratingKey, token string) (int, string, error) {
	// Use PUT with token as query parameter. Some Plex setups (or proxies)
	// accept the refresh only when token is provided as X-Plex-Token query.
	// This mirrors the curl variant that worked: `curl -X PUT "$BASE/library/metadata/$R/refresh?X-Plex-Token=$T"`
	refreshURL := fmt.Sprintf("%s/library/metadata/%s/refresh?X-Plex-Token=%s", base, ratingKey, url.QueryEscape(token))
	rreq, err := http.NewRequest("PUT", refreshURL, nil)
	if err != nil {
		return 0, "", err
	}
	// Add some standard Plex client headers which may be expected by some
	// fronting proxies or Plex auth rules. Use configured clientId if present.
	if cfg, gerr := GetPlexConfig(); gerr == nil {
		if cfg.ClientId != "" {
			rreq.Header.Set("X-Plex-Client-Identifier", cfg.ClientId)
		}
	}
	rreq.Header.Set("X-Plex-Product", "Trailarr")
	rreq.Header.Set("Accept", "application/json")

	// Log the outgoing refresh request (mask token)
	masked := token
	if len(masked) > 8 {
		masked = masked[:4] + "..." + masked[len(masked)-4:]
	}
	TrailarrLog(INFO, "Plex", "doRefreshRequest: PUT %s tokenQuery=%s", refreshURL, masked)
	client := &http.Client{Timeout: 10 * time.Second}
	rresp, err := client.Do(rreq)
	if err != nil {
		return 0, "", err
	}
	defer rresp.Body.Close()
	rbody, _ := io.ReadAll(rresp.Body)
	// Log response summary
	TrailarrLog(INFO, "Plex", "doRefreshRequest: response for %s status=%d bodyLen=%d", refreshURL, rresp.StatusCode, len(rbody))
	return rresp.StatusCode, string(rbody), nil
}
