package scanner

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestScanWarningsFollowActualModuleScope(t *testing.T) {
	s := &Scanner{home: t.TempDir()}
	project := filepath.Join(s.home, "Work")
	external := t.TempDir()
	paths := []string{
		filepath.Join(s.home, "Library/Containers/com.example"),
		filepath.Join(project, "Example/target"),
		filepath.Join(external, "Example/build"),
		filepath.Join(s.home, "Projects/Other/build"),
		filepath.Join(s.home, "Downloads/Example.dmg"),
		filepath.Join(s.home, ".gradle/caches/build-cache-1"),
	}
	for _, path := range paths {
		recordReadWarningKind(path, "permission denied", warnPermission)
	}
	t.Cleanup(func() {
		directoryReads.Lock()
		defer directoryReads.Unlock()
		for _, path := range paths {
			delete(directoryReads.failed, path)
			delete(directoryReads.kinds, path)
		}
	})
	config := PurgeRoots{Custom: true, Paths: []string{project, external}}
	warnings := s.scopedReadWarnings(config)
	byPath := map[string]map[string]any{}
	for _, warning := range warnings {
		byPath[warning["path"].(string)] = warning
	}
	for index, expectedPurge := range []bool{false, true, true, false, true, false} {
		warning := byPath[paths[index]]
		if warning == nil {
			t.Fatalf("warning missing for %s", paths[index])
		}
		modules := warning["modules"].([]string)
		if slices.Contains(modules, "purge") != expectedPurge {
			t.Errorf("%s modules=%v; purge=%v", paths[index], modules, expectedPurge)
		}
	}
	if !slices.Contains(byPath[paths[0]]["modules"].([]string), "software") {
		t.Fatal("application warning disappeared from its actual section")
	}
	if !slices.Contains(byPath[paths[5]]["modules"].([]string), "clean") {
		t.Fatal("Mole cache warning disappeared from its actual cleanup section")
	}
}

func TestScanWarningResetIncludesConfiguredExternalRoots(t *testing.T) {
	s := &Scanner{home: t.TempDir()}
	volume := t.TempDir()
	selected := filepath.Join(volume, "Projects")
	unrelated := t.TempDir()
	paths := []string{volume, filepath.Join(selected, "Example/target"), unrelated}
	for _, path := range paths {
		recordReadWarningKind(path, "previous scan limit", warnLimit)
	}
	t.Cleanup(func() { clearReadWarningsForRoots(paths) })
	config := PurgeRoots{Custom: true, Paths: []string{selected}}
	clearReadWarningsForRoots(s.scanWarningRoots(config))
	if warnings := s.scopedReadWarnings(config); len(warnings) != 0 {
		t.Fatalf("a new scan retained stale external root or parent warnings: %v", warnings)
	}
	if warnings := scanReadWarnings(unrelated); len(warnings) != 1 || warnings[0]["path"] != unrelated {
		t.Fatalf("resetting the selected scope changed an unrelated scanner: %v", warnings)
	}
}

func TestConfigWarningsBelongOnlyToBuildCleanup(t *testing.T) {
	s := &Scanner{home: t.TempDir()}
	config := PurgeRoots{Custom: true, Warnings: []map[string]any{{"path": PurgePathsPath(s.home), "kind": warnConfig, "reason": "invalid project root"}}}
	warnings := s.scopedReadWarnings(config)
	if len(warnings) != 1 || !slices.Equal(warnings[0]["modules"].([]string), []string{"purge"}) || warnings[0]["kind"] != warnConfig {
		t.Fatalf("a path configuration failure must not prompt for unrelated permissions: %v", warnings)
	}
	if _, mutated := config.Warnings[0]["modules"]; mutated {
		t.Fatal("annotating module warnings mutated the saved scope snapshot")
	}
}
