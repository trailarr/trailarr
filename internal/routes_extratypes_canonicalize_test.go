package internal

import (
	"encoding/json"
	"testing"
)

func TestExtraTypesSaveAndGet(t *testing.T) {
	r := NewTestRouter()
	RegisterRoutes(r)

	// Save a custom extra types config
	payload := `{"trailers":false,"scenes":true,"behindTheScenes":false,"interviews":false,"featurettes":false,"deletedScenes":false,"shorts":false,"other":false}`
	w := DoRequest(r, "POST", "/api/settings/extratypes", []byte(payload))
	if w.Code != 200 {
		t.Fatalf("expected 200 saving extra types, got %d body=%s", w.Code, w.Body.String())
	}

	// reload config into memory so GetExtraTypesConfig reads the saved file
	if cfgMap, err := readConfigFile(); err != nil {
		t.Fatalf("failed to reload config: %v", err)
	} else {
		Config = cfgMap
	}

	// GET and verify
	w = DoRequest(r, "GET", "/api/settings/extratypes", nil)
	if w.Code != 200 {
		t.Fatalf("expected 200 getting extra types, got %d body=%s", w.Code, w.Body.String())
	}
	var resp []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse extratypes response: %v", err)
	}
	// find trailers entry
	found := false
	for _, e := range resp {
		if k, ok := e["key"].(string); ok && k == "trailers" {
			found = true
			if val, ok := e["value"].(bool); !ok || val != false {
				t.Fatalf("expected trailers value false, got %v", e["value"])
			}
		}
	}
	if !found {
		t.Fatalf("trailers entry not found in response: %v", resp)
	}
}

func TestCanonicalizeSaveAndGet(t *testing.T) {
	r := NewTestRouter()
	RegisterRoutes(r)

	// Save canonicalize mapping
	payload := `{"mapping":{"Trailer":"Trailers","Featurette":"Featurettes"}}`
	w := DoRequest(r, "POST", "/api/settings/canonicalizeextratype", []byte(payload))
	if w.Code != 200 {
		t.Fatalf("expected 200 saving canonicalize mapping, got %d body=%s", w.Code, w.Body.String())
	}

	// reload config into memory so GetCanonicalizeExtraTypeConfigHandler reads the saved file
	if cfgMap, err := readConfigFile(); err != nil {
		t.Fatalf("failed to reload config: %v", err)
	} else {
		Config = cfgMap
	}

	// GET and verify
	w = DoRequest(r, "GET", "/api/settings/canonicalizeextratype", nil)
	if w.Code != 200 {
		t.Fatalf("expected 200 getting canonicalize mapping, got %d body=%s", w.Code, w.Body.String())
	}
	var cfg CanonicalizeExtraTypeConfig
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("failed to parse canonicalize response: %v", err)
	}
	if cfg.Mapping == nil || cfg.Mapping["Trailer"] != "Trailers" {
		t.Fatalf("expected mapping Trailer->Trailers, got %v", cfg.Mapping)
	}
}
