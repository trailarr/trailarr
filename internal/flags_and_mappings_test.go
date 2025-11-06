package internal

import (
	"testing"
)

func TestApplyYtdlpFlagsAndSetters(t *testing.T) {
	cfg := DefaultYtdlpFlagsConfig()
	sec := map[string]interface{}{
		"quiet":        true,
		"timeout":      12.5,
		"maxDownloads": 4,
	}
	applyYtdlpFlags(sec, &cfg)
	if !cfg.Quiet || cfg.Timeout != 12.5 || cfg.MaxDownloads != 4 {
		t.Fatalf("applyYtdlpFlags did not apply values correctly: %+v", cfg)
	}
}

func TestParseSectionPathMappingsAndMerge(t *testing.T) {
	config := map[string]interface{}{
		"radarr": map[string]interface{}{
			"pathMappings": []interface{}{
				map[string]interface{}{"from": "/m1", "to": "/t1"},
			},
		},
	}
	sectionData, mappings, mappingSet, pathMappings := parseSectionPathMappings(config, "radarr")
	if sectionData == nil || len(mappings) != 1 || !mappingSet["/m1"] {
		t.Fatalf("parseSectionPathMappings failed: %v %v %v", sectionData, mappings, mappingSet)
	}

	// mergeFoldersIntoMappings should add unknown folders
	folders := []map[string]interface{}{{"path": "/m2"}}
	mergedPathMappings, mergedMappings, updated := mergeFoldersIntoMappings(pathMappings, mappings, mappingSet, folders)
	if !updated {
		t.Fatalf("expected updated to be true when merging new folder")
	}
	if len(mergedMappings) != 2 {
		t.Fatalf("expected 2 mappings after merge, got %d", len(mergedMappings))
	}
	// ensure merged pathMappings contains the new folder
	found := false
	for _, pm := range mergedPathMappings {
		if to, ok := pm["to"].(string); ok && to == "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("merged path mappings did not include new folder: %v", mergedPathMappings)
	}
}
