package scanner

import (
	"path/filepath"
	"testing"
)

func TestReadWarningsBelongToTheirScannerHome(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "Desktop")
	directoryReads.Lock()
	directoryReads.failed[path] = "test permission refusal"
	directoryReads.Unlock()
	t.Cleanup(func() { directoryReads.Lock(); delete(directoryReads.failed, path); directoryReads.Unlock() })
	if warnings := scanReadWarnings(home); len(warnings) != 1 || warnings[0]["path"] != path {
		t.Fatalf("warnings = %v", warnings)
	}
	if warnings := scanReadWarnings(filepath.Join(home, "other")); len(warnings) != 0 {
		t.Fatalf("unrelated warnings = %v", warnings)
	}
}
