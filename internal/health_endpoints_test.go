package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const errFailedToDecodeResponse = "failed to decode response: %v"

// Test that the global health execute route runs the full check and returns
// success=false when providers are missing or unreachable.
func TestGlobalHealthExecuteReturnsFailure(t *testing.T) {
	// rely on package-level TestMain temp root
	ctx := context.Background()
	_ = GetStoreClient().Del(ctx, HealthIssuesStoreKey)

	r := ginDefaultRouterForTests()
	w := DoRequest(r, "POST", "/api/health/execute", nil)
	if w.Code != 200 {
		t.Fatalf("expected 200 for /api/health/execute, got %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf(errFailedToDecodeResponse, err)
	}
	if ok, _ := resp["success"].(bool); ok {
		t.Fatalf("expected success=false when providers missing, got success=true")
	}
	// ensure persisted issues exist
	vals, err := GetStoreClient().LRange(ctx, HealthIssuesStoreKey, 0, -1)
	if err != nil {
		t.Fatalf("failed to read health issues from store: %v", err)
	}
	if len(vals) == 0 {
		t.Fatalf("expected persisted health issues after running global healthcheck")
	}
}

// Test provider-specific execute: success when provider endpoint returns 200.
func TestProviderHealthExecuteSuccess(t *testing.T) {
	// rely on package-level TestMain temp root
	ctx := context.Background()
	_ = GetStoreClient().Del(ctx, HealthIssuesStoreKey)

	// start a test server that simulates a provider returning OK for /api/v3/system/status
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/system/status" {
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer ts.Close()

	// write config with radarr pointing to our test server and an apiKey
	cfg := map[string]interface{}{
		"general":    DefaultGeneralConfig(),
		"radarr":     map[string]interface{}{"url": ts.URL, "apiKey": "x"},
		"ytdlpFlags": DefaultYtdlpFlagsConfig(),
	}
	if err := writeConfigFile(cfg); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	r := ginDefaultRouterForTests()
	w := DoRequest(r, "POST", "/api/health/radarr/execute", nil)
	if w.Code != 200 {
		t.Fatalf("expected 200 for /api/health/radarr/execute, got %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf(errFailedToDecodeResponse, err)
	}
	if ok, _ := resp["success"].(bool); !ok {
		t.Fatalf("expected success=true for provider execute when provider is reachable; resp=%v", resp)
	}
	// store should not contain radarr issues
	vals, _ := GetStoreClient().LRange(ctx, HealthIssuesStoreKey, 0, -1)
	for _, v := range vals {
		var hm HealthMsg
		_ = json.Unmarshal([]byte(v), &hm)
		if hm.Source == "Radarr" {
			t.Fatalf("expected no persisted Radarr issues after successful provider execute; found: %v", hm)
		}
	}
}

// Test provider-specific execute failure: provider returns 500 -> persisted issue and success=false
func TestProviderHealthExecuteFailure(t *testing.T) {
	// rely on package-level TestMain temp root
	ctx := context.Background()
	_ = GetStoreClient().Del(ctx, HealthIssuesStoreKey)

	// test server returns 500 for the status endpoint
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/system/status" {
			w.WriteHeader(500)
			_, _ = w.Write([]byte("server error"))
			return
		}
		w.WriteHeader(404)
	}))
	defer ts.Close()

	cfg := map[string]interface{}{
		"general":    DefaultGeneralConfig(),
		"radarr":     map[string]interface{}{"url": ts.URL, "apiKey": "x"},
		"ytdlpFlags": DefaultYtdlpFlagsConfig(),
	}
	if err := writeConfigFile(cfg); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	r := ginDefaultRouterForTests()
	w := DoRequest(r, "POST", "/api/health/radarr/execute", nil)
	if w.Code != 200 {
		t.Fatalf("expected 200 for /api/health/radarr/execute, got %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf(errFailedToDecodeResponse, err)
	}
	if ok, _ := resp["success"].(bool); ok {
		t.Fatalf("expected success=false for provider execute when provider returns error; resp=%v", resp)
	}
	// persisted issues should include Radarr
	vals, _ := GetStoreClient().LRange(ctx, HealthIssuesStoreKey, 0, -1)
	found := false
	for _, v := range vals {
		var hm HealthMsg
		_ = json.Unmarshal([]byte(v), &hm)
		if hm.Source == "Radarr" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected persisted Radarr issue after failed provider execute")
	}
}
