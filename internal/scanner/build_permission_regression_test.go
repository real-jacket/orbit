package scanner

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBuildScanHonorsConfiguredRootsInsteadOfFallbackFolders(t *testing.T) {
	home := fixtureHome(t)
	artifact := fixtureDir(t, home, "Work/Example/target", 21_000_000)
	fixtureDir(t, home, "Projects/OutsideSelectedScope/target", 21_000_000)
	writeFile(t, filepath.Join(home, ".config/mole/purge_paths"), "# Custom scope\n ~/Work \n~/MissingProjectRoot\n")
	scanner := &Scanner{home: home, processProbe: func() (string, error) { return "/sbin/launchd\n", nil }}
	rows := scanner.scanBuildArtifacts()
	if len(rows) != 1 || rows[0]["path"] != scanner.relativize(artifact) {
		t.Fatalf("configured scope must be authoritative: rows=%v want=%s", rows, scanner.relativize(artifact))
	}
	if warnings := scanReadWarnings(home); len(warnings) != 0 {
		t.Fatalf("a missing optional root is not a permission failure: %v", warnings)
	}
}

func TestUnreadableBuildConfigDoesNotBroadenTheScanScope(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read the denied fixture")
	}
	home := fixtureHome(t)
	fixtureDir(t, home, "Projects/OutsideSelectedScope/target", 21_000_000)
	config := filepath.Join(home, ".config/mole/purge_paths")
	writeFile(t, config, "~/Work\n")
	if err := os.Chmod(config, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(config, 0o600) })
	scanner := &Scanner{home: home, processProbe: func() (string, error) { return "/sbin/launchd\n", nil }}
	snapshot := scanner.PurgeRoots()
	if !snapshot.Custom || snapshot.Error == "" || len(snapshot.AbsolutePaths()) != 0 || len(snapshot.Warnings) != 1 || snapshot.Warnings[0]["kind"] != warnConfig {
		t.Fatalf("unreadable saved configuration must disclose its empty scope: %+v", snapshot)
	}
	rows := scanner.scanBuildArtifacts()
	if len(rows) != 0 {
		t.Fatalf("unreadable saved scope must not silently activate fallback roots: %v", rows)
	}
}

func TestBuildConfigReportsMissingRootsWithoutPermissionWarnings(t *testing.T) {
	home := fixtureHome(t)
	if err := os.MkdirAll(filepath.Join(home, "Work"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(home, ".config/mole/purge_paths"), " # Selected project roots\n\t~/Work \t\n\n ~/MissingProjectRoot \n")
	snapshot := (&Scanner{home: home}).PurgeRoots()
	if !snapshot.Custom || !reflect.DeepEqual(snapshot.Paths, []string{filepath.Join(home, "Work"), filepath.Join(home, "MissingProjectRoot")}) {
		t.Fatalf("configured root snapshot = %+v", snapshot)
	}
	if len(snapshot.Roots) != 2 || snapshot.Roots[0].Status != "readable" || snapshot.Roots[1].Status != "missing" {
		t.Fatalf("root status must distinguish missing from denied: %+v", snapshot.Roots)
	}
	if len(snapshot.Warnings) != 0 || len(scanReadWarnings(home)) != 0 {
		t.Fatalf("missing optional roots must not ask for disk access: %+v", snapshot)
	}
}

func TestBuildConfigReadableEmptyScopeUsesDocumentedMoleDefaults(t *testing.T) {
	for _, contents := range []string{"", " \n # Delete all paths to use defaults\n\t"} {
		t.Run(strings.ReplaceAll(contents, "\n", "_"), func(t *testing.T) {
			home := fixtureHome(t)
			configPath := filepath.Join(home, ".config/mole/purge_paths")
			writeFile(t, configPath, contents)
			scanner := &Scanner{home: home}
			var want []string
			for _, root := range defaultPurgeRoots {
				want = append(want, scanner.expand(root))
			}
			snapshot := scanner.PurgeRoots()
			if snapshot.Custom || !reflect.DeepEqual(snapshot.Paths, want) || len(snapshot.Warnings) != 0 {
				t.Fatalf("a readable empty Mole path file requests defaults: %+v", snapshot)
			}
			if data, err := os.ReadFile(configPath); err != nil || string(data) != contents {
				t.Fatalf("reading a default scope must not rewrite its saved file: %q, %v", data, err)
			}
		})
	}
}

func TestBuildConfigInvalidSavedScopeDoesNotActivateDefaults(t *testing.T) {
	home := fixtureHome(t)
	fixtureDir(t, home, "Projects/OutsideSelectedScope/target", 21_000_000)
	if err := os.MkdirAll(filepath.Join(home, "Library"), 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(home, "LibraryAlias")
	if err := os.Symlink(filepath.Join(home, "Library"), alias); err != nil {
		t.Fatal(err)
	}
	invalidRoots := []string{"~", "/", "~/Library", "~/LibraryAlias", "~/libraryalias", "/System/Projects", "/Applications/Fake.app/Projects", "relative/projects"}
	writeFile(t, filepath.Join(home, ".config/mole/purge_paths"), strings.Join(invalidRoots, "\n"))
	scanner := &Scanner{home: home, processProbe: func() (string, error) { return "/sbin/launchd\n", nil }}
	snapshot := scanner.PurgeRoots()
	if !snapshot.Custom || len(snapshot.Paths) != 0 || len(snapshot.AbsolutePaths()) != 0 || len(snapshot.Warnings) != len(invalidRoots) {
		t.Fatalf("invalid saved scope must remain visible and empty: %+v", snapshot)
	}
	for _, warning := range snapshot.Warnings {
		if warning["kind"] != "config" {
			t.Fatalf("invalid roots must be configuration warnings: %v", warning)
		}
	}
	if rows := scanner.scanBuildArtifactsWithRoots(snapshot.AbsolutePaths()); len(rows) != 0 {
		t.Fatalf("invalid roots must not scan default project directories: %v", rows)
	}
}

func TestBuildConfigDeduplicatesPhysicalAliasesAndPreservesHomeSpelling(t *testing.T) {
	physicalHome := fixtureHome(t)
	if err := os.MkdirAll(filepath.Join(physicalHome, "Work"), 0o700); err != nil {
		t.Fatal(err)
	}
	aliasHome := filepath.Join(t.TempDir(), "AliasHome")
	if err := os.Symlink(physicalHome, aliasHome); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(physicalHome, "Work"), filepath.Join(physicalHome, "WorkAlias")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(physicalHome, ".config/mole/purge_paths"), "~/Work\n~/WorkAlias\n"+filepath.Join(physicalHome, "Work")+"\n~/Work\n")
	snapshot := (&Scanner{home: aliasHome}).PurgeRoots()
	if !reflect.DeepEqual(snapshot.Paths, []string{filepath.Join(aliasHome, "Work")}) || !reflect.DeepEqual(snapshot.AbsolutePaths(), []string{filepath.Join(aliasHome, "Work")}) {
		t.Fatalf("physical aliases must be counted once and keep the scanner home: %+v", snapshot)
	}
}

func TestBuildConfigReloadsSavedRootsBetweenScans(t *testing.T) {
	home := fixtureHome(t)
	first := fixtureDir(t, home, "First/Project/target", 21_000_000)
	second := fixtureDir(t, home, "Second/Project/target", 21_000_000)
	configPath := filepath.Join(home, ".config/mole/purge_paths")
	scanner := &Scanner{home: home, processProbe: func() (string, error) { return "/sbin/launchd\n", nil }}
	for _, selection := range []struct{ root, artifact string }{{"~/First", first}, {"~/Second", second}} {
		writeFile(t, configPath, selection.root+"\n")
		rows := scanner.scanBuildArtifacts()
		if len(rows) != 1 || rows[0]["path"] != scanner.relativize(selection.artifact) {
			t.Fatalf("changed saved roots must take effect on the next scan: %v; root=%s", rows, selection.root)
		}
	}
}

func TestBuildReadWarningsUseSelectedRootsAndDeniedAncestors(t *testing.T) {
	workspace := t.TempDir()
	selected := filepath.Join(workspace, "Work")
	deniedAncestor := filepath.Join(workspace, "External")
	external := filepath.Join(deniedAncestor, "Projects")
	paths := []string{
		filepath.Join(selected, "Project/target"),
		deniedAncestor,
		filepath.Join(external, "Project/build"),
		filepath.Join(workspace, "WorkOther/Project/build"),
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
	warnings := scanReadWarningsForRoots([]string{selected + string(filepath.Separator), external, strings.ToUpper(selected), ""})
	if len(warnings) != 2 {
		t.Fatalf("selected roots and denied ancestors must appear once without repeated descendants: %v", warnings)
	}
	for _, warning := range warnings {
		if warning["path"] == paths[3] || warning["kind"] != "permission" {
			t.Fatalf("a sibling directory cannot acquire the selected root's warning: %v", warning)
		}
	}
	if warnings := scanReadWarningsForRoots(nil); len(warnings) != 0 {
		t.Fatalf("an explicitly empty scan scope has no read warnings: %v", warnings)
	}
}

func TestBuildReadWarningsSeparateAccessDenialFromLimits(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read the denied fixture")
	}
	home := fixtureHome(t)
	root := filepath.Join(home, "Work")
	denied := filepath.Join(root, "DeniedProject")
	if err := os.MkdirAll(denied, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(denied, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(denied, 0o700) })
	scanner := &Scanner{home: home}
	t.Cleanup(func() { clearReadWarnings(home) })
	var artifacts []artifactCandidate
	scanner.walkArtifacts(root, time.Now().Add(time.Minute), &artifacts)
	warnings := scanReadWarnings(home)
	if len(warnings) != 1 || warnings[0]["path"] != denied || warnings[0]["kind"] != "permission" {
		t.Fatalf("real access denial must be typed as permission: %v", warnings)
	}
	if err := os.Chmod(denied, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	clearReadWarnings(home)
	scanner.walkArtifacts(root, time.Now().Add(-time.Second), &artifacts)
	warnings = scanReadWarnings(home)
	var foundLimit bool
	for _, warning := range warnings {
		if strings.Contains(warning["reason"].(string), "上限") {
			foundLimit = true
			if warning["kind"] != "limit" {
				t.Fatalf("a traversal cap must not ask for Full Disk Access: %v", warning)
			}
		}
	}
	if !foundLimit {
		t.Fatalf("bounded build discovery must disclose its limit: %v", warnings)
	}
}
