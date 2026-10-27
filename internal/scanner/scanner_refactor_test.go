package scanner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// This file ports the intent of Tests/MoleStudioTests/ScannerRefactorTests.swift:
// the orphan signature catalogue must stay populated and uniquely identified,
// the full-scan payload must keep the JSON keys the frontend binds to, and every
// orphan row must carry the fields the UI reads.

// TestOrphanSignaturesAreNonEmptyAndUnique is the Go port of
// testOrphanSignaturesAreNonEmptyAndUnique: the catalogue is never empty and its
// ids are unique, so the UI can key rows by id.
func TestOrphanSignaturesAreNonEmptyAndUnique(t *testing.T) {
	for _, home := range []string{t.TempDir(), "/Users/example"} {
		t.Run(home, func(t *testing.T) {
			signatures := AllSignatures(home)
			if len(signatures) == 0 {
				t.Fatal("AllSignatures returned no signatures; the orphan catalogue must not be empty")
			}
			seen := make(map[string]int, len(signatures))
			for i, signature := range signatures {
				if signature.ID == "" {
					t.Errorf("signature %d (%q) has an empty id", i, signature.Name)
				}
				if first, duplicate := seen[signature.ID]; duplicate {
					t.Errorf("duplicate signature id %q at index %d and %d", signature.ID, first, i)
				}
				seen[signature.ID] = i
				if signature.Name == "" {
					t.Errorf("signature %q has an empty name", signature.ID)
				}
			}
		})
	}
}

// TestFullScanPayloadKeepsItsKeys is the Go port of
// testFullScanPayloadKeepsItsKeys. It is the single test that touches the real
// home directory, exactly as the Swift test did; skip it with -short.
func TestFullScanPayloadKeepsItsKeys(t *testing.T) {
	if testing.Short() {
		t.Skip("real-home scan skipped in short mode")
	}
	payload := Shared.BuildFullScanPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal(FullScanPayload) failed: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(payload) failed: %v", err)
	}
	for _, key := range []string{"orphanLeftoversList", "cleanGroups", "appsList"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("buildFullScanPayload must still emit %q", key)
		}
	}
}

// TestOrphanRowsCarryTheFieldsTheUIBinds is the Go port of
// testOrphanRowsCarryTheFieldsTheUIBinds. It runs hermetically against a
// temporary home whose Docker container directory makes one signature match, so
// no real user directory is scanned.
func TestOrphanRowsCarryTheFieldsTheUIBinds(t *testing.T) {
	home := t.TempDir()
	// orphan_docker claims this fixed path, so one row is guaranteed.
	container := filepath.Join(home, "Library", "Containers", "com.docker.docker")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", container, err)
	}
	writeTestFile(t, filepath.Join(container, "data.bin"), 4096)

	s := &Scanner{home: home}
	rows := s.scanOrphanedLeftovers()
	if len(rows) == 0 {
		t.Fatal("scanOrphanedLeftovers returned no rows for a home with a Docker container leftover")
	}

	requiredRowKeys := []string{"id", "name", "leftoverSizeGb", "subpaths", "statusText", "checked"}
	for i, row := range rows {
		decoded := marshalRow(t, i, row)
		for _, key := range requiredRowKeys {
			raw, ok := decoded[key]
			if !ok {
				t.Errorf("row %d is missing the %q key", i, key)
				continue
			}
			if string(raw) == "null" {
				t.Errorf("row %d has a null %q value", i, key)
			}
		}

		sizeGb, ok := row["leftoverSizeGb"].(float64)
		if !ok {
			t.Errorf("row %d has leftoverSizeGb of type %T, want float64", i, row["leftoverSizeGb"])
		} else if sizeGb < 0.01 {
			t.Errorf("row %d has leftoverSizeGb = %v, want >= 0.01", i, sizeGb)
		}

		if _, isBool := row["checked"].(bool); !isBool {
			t.Errorf("row %d has checked of type %T, want bool", i, row["checked"])
		}

		subpaths, ok := row["subpaths"].([]map[string]any)
		if !ok {
			t.Errorf("row %d has subpaths of type %T, want []map[string]any", i, row["subpaths"])
			continue
		}
		if len(subpaths) == 0 {
			t.Errorf("row %d has no subpaths", i)
		}
		for j, subpath := range subpaths {
			subDecoded := marshalRow(t, j, subpath)
			for _, key := range []string{"path", "kind", "size", "checked"} {
				if raw, ok := subDecoded[key]; !ok || string(raw) == "null" {
					t.Errorf("row %d subpath %d is missing a non-null %q key", i, j, key)
				}
			}
		}
	}
}

// marshalRow renders one payload row as JSON so the test can assert on the key
// names the frontend actually reads, not on Go struct fields.
func marshalRow(t *testing.T, index int, row map[string]any) map[string]json.RawMessage {
	t.Helper()
	data, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("json.Marshal(row %d) failed: %v", index, err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(row %d) failed: %v", index, err)
	}
	return decoded
}
