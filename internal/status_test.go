package internal

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestSystemStatusIncludesLatestReleaseAndWarning(t *testing.T) {
	// Stub the GitHub fetch to avoid network calls
	orig := fetchLatestGithubReleaseTag
	fetchLatestGithubReleaseTag = func(repo string) (string, error) {
		return "2.0.0", nil
	}
	defer func() { fetchLatestGithubReleaseTag = orig }()

	// Ensure AppVersion is older so an update warning is expected
	AppVersion = "1.0.0"

	r := ginDefaultRouterForTests()
	w := DoRequest(r, "GET", "/api/system/status", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/system/status, got %d", w.Code)
	}
	var ss SystemStatus
	if err := json.Unmarshal(w.Body.Bytes(), &ss); err != nil {
		t.Fatalf("failed to decode system status: %v", err)
	}
	if ss.About.LatestRelease != "2.0.0" {
		t.Fatalf("expected LatestRelease to be 2.0.0, got %q", ss.About.LatestRelease)
	}
	found := false
	for _, h := range ss.Health {
		if h.Source == "trailarr" && h.Message == "trailarr update available" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected trailarr update warning in health, got: %v", ss.Health)
	}
}
