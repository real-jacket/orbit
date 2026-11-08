package scanner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAIDesktopCachesHaveNarrowIdleCleanupScopes(t *testing.T) {
	home := fixtureHome(t)
	scanner := &Scanner{home: home, processProbe: func() (string, error) { return "/usr/bin/idle\n", nil }, handleProbe: func(string) (bool, error) { return false, nil }}
	cache := fixtureDir(t, home, "Library/Application Support/Claude/Code Cache", 4096)
	personal := fixtureDir(t, home, "Library/Application Support/Claude/IndexedDB", 4096)
	if reason := scanner.CleanupReason(cache); reason != "" {
		t.Fatalf("a verified idle Chromium code cache should be separately reviewable: %s", reason)
	}
	if reason := scanner.CleanupReason(personal); reason == "" {
		t.Fatal("an AI app's personal data cannot share the cache exception")
	}
	rows := scanner.scanAICaches()
	found := false
	for _, row := range rows {
		for _, path := range row["subpaths"].([]map[string]any) {
			if path["path"] == scanner.relativize(cache) {
				found = true
				if row["tool"] != "Claude Desktop" || row["tier"] != "review" || row["checked"] != false || path["checked"] != false {
					t.Fatalf("new app caches must identify their tool and require selection: %v", row)
				}
			}
		}
	}
	if !found {
		t.Fatal("Claude Desktop's real cache is absent from AI cleanup")
	}
	scanner.processProbe = func() (string, error) {
		return "/Applications/Claude.app/Contents/MacOS/Claude Helper (Renderer)\n", nil
	}
	if reason := scanner.CleanupReason(cache); !strings.Contains(reason, "运行") {
		t.Fatalf("a helper that started after preview must protect the cache: %s", reason)
	}
	scanner.processProbe = func() (string, error) { return "/usr/bin/idle\n", nil }
	scanner.handleProbe = func(string) (bool, error) { return true, nil }
	if reason := scanner.CleanupReason(cache); !strings.Contains(reason, "打开") {
		t.Fatalf("an open cache stays protected even if its owner name is unknown: %s", reason)
	}
}

func TestAIAgentDiagnosticsUseExactFilesAndRetention(t *testing.T) {
	home := fixtureHome(t)
	scanner := &Scanner{home: home, processProbe: func() (string, error) { return "/usr/bin/idle\n", nil }, handleProbe: func(string) (bool, error) { return false, nil }}
	root := filepath.Join(home, ".copilot/logs")
	old := filepath.Join(root, "process-old.log")
	latest := filepath.Join(root, "process-current.log")
	transcript := filepath.Join(root, "transcript.jsonl")
	writeFile(t, old, "old diagnostic")
	writeFile(t, latest, "current diagnostic")
	writeFile(t, transcript, "session data")
	oldTime := time.Now().Add(-60 * 24 * time.Hour)
	for _, path := range []string{old, latest, transcript} {
		if err := os.Chtimes(path, oldTime, oldTime); err != nil {
			t.Fatal(err)
		}
	}
	latestTime := time.Now().Add(-40 * 24 * time.Hour)
	if err := os.Chtimes(latest, latestTime, latestTime); err != nil {
		t.Fatal(err)
	}
	if reason := scanner.CleanupReason(old); reason != "" {
		t.Fatalf("a closed old diagnostic should be explicitly reviewable: %s", reason)
	}
	if reason := scanner.CleanupReason(latest); !strings.Contains(reason, "最新") {
		t.Fatalf("even an old last diagnostic must be retained: %s", reason)
	}
	if reason := scanner.CleanupReason(transcript); reason == "" {
		t.Fatal("a JSONL transcript cannot be classified as a disposable log")
	}
	if reason := scanner.CleanupReason(root); reason == "" {
		t.Fatal("diagnostic directories cannot be moved whole")
	}
	rows := scanner.scanAICaches()
	found := false
	for _, row := range rows {
		if row["tool"] != "GitHub Copilot CLI" || row["band"] != "诊断日志" {
			continue
		}
		found = true
		if row["checked"] != false {
			t.Fatal("old logs are never preselected")
		}
		for _, path := range row["subpaths"].([]map[string]any) {
			if path["path"] == scanner.relativize(transcript) {
				t.Fatal("transcripts must stay outside the diagnostic candidate set")
			}
		}
	}
	if !found {
		t.Fatal("Copilot diagnostics are not included in the AI inventory")
	}
	if err := os.Chtimes(old, time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if reason := scanner.CleanupReason(old); !strings.Contains(reason, "30") {
		t.Fatalf("a log updated after preview must no longer be disposable: %s", reason)
	}
}

func TestAdditionalAgentSessionAndModelDataStayProtected(t *testing.T) {
	scanner := &Scanner{home: fixtureHome(t)}
	for _, relative := range []string{
		".gemini/tmp/session.json", ".copilot/session-state/session.json", ".copilot/mcp-secrets/token",
		".local/share/opencode/opencode.db", ".local/share/opencode/snapshot/repo", ".continue/sessions/session.json",
		".codeium/windsurf/cascade/session", ".lmstudio/models/model.gguf",
		"Library/Application Support/Code/User/globalStorage/rooveterinaryinc.roo-cline/task",
		"Library/Application Support/Code/User/globalStorage/saoudrizwan.claude-dev/task",
	} {
		path := filepath.Join(scanner.home, relative)
		writeFile(t, path, "protected AI data")
		if reason := scanner.CleanupReason(path); reason == "" {
			t.Errorf("agent session, credentials or models were offered as removable: %s", relative)
		}
	}
}

func TestAIDataExceptionsKeepWhitelistAndAliasProtection(t *testing.T) {
	home := fixtureHome(t)
	cache := fixtureDir(t, home, "Library/Application Support/Claude/GPUCache", 4096)
	scanner := &Scanner{home: home, processProbe: func() (string, error) { return "/usr/bin/idle\n", nil }, handleProbe: func(string) (bool, error) { return false, nil }}
	writeWhitelist(t, home, "~/Library/Application Support")
	before, err := os.ReadFile(WhitelistPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if reason := scanner.CleanupReason(cache); !strings.Contains(reason, "白名单") {
		t.Fatalf("specific cache recognition cannot override the user's parent whitelist: %s", reason)
	}
	scanner.scanAICaches()
	after, err := os.ReadFile(WhitelistPath(home))
	if err != nil || string(after) != string(before) {
		t.Fatal("AI inventory changed the user's protection policy")
	}
	writeWhitelist(t, home)
	alias := filepath.Join(home, "Library/Application Support/Claude/Cache")
	if err := os.Symlink(filepath.Join(home, ".codex/sessions"), alias); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(home, ".codex/sessions/session.json"), "session")
	if reason := scanner.CleanupReason(alias); reason == "" {
		t.Fatal("a cache-named symlink cannot authorize removing session data")
	}
	scanner.processProbe = func() (string, error) { return "", errors.New("process state unavailable") }
	if reason := scanner.CleanupReason(cache); reason == "" {
		t.Fatal("missing process evidence must retain the recognized cache")
	}
}

func TestAICacheCaseAliasesKeepWhitelistProtection(t *testing.T) {
	home := fixtureHome(t)
	cache := fixtureDir(t, home, "Library/Application Support/Claude/GPUCache", 4096)
	alias := filepath.Join(home, "library/application support/claude/gpucache")
	original, err := os.Stat(cache)
	if err != nil {
		t.Fatal(err)
	}
	alternate, err := os.Stat(alias)
	if os.IsNotExist(err) {
		t.Skip("the fixture filesystem distinguishes path case")
	}
	if err != nil || !os.SameFile(original, alternate) {
		t.Fatalf("cannot verify a case-insensitive cache alias: %v", err)
	}
	t.Setenv("MOLE_STUDIO_TRASH_DIR", t.TempDir())
	t.Setenv("MOLE_STUDIO_STATE_DIR", t.TempDir())
	scanner := &Scanner{home: home, processProbe: func() (string, error) { return "/usr/bin/idle\n", nil }, handleProbe: func(string) (bool, error) { return false, nil }}
	for _, pattern := range []string{"~/Library/Application Support/Claude/GPUCache", "~/Library/Application Support/Claude", "~/Library/Application Support/Claude/GPU*", "~/Library/Application Support/Claude/GPUCache & ~/.gemini"} {
		t.Run(pattern, func(t *testing.T) {
			writeWhitelist(t, home, pattern)
			before, err := os.ReadFile(WhitelistPath(home))
			if err != nil {
				t.Fatal(err)
			}
			if reason := scanner.CleanupReason(alias); !strings.Contains(reason, "白名单") {
				t.Fatalf("a different spelling of the same cache bypassed its whitelist: %q", reason)
			}
			outcome := scanner.TrashPaths([]string{alias})
			if len(outcome.Moved) != 0 || len(outcome.Skipped) != 1 || !exists(filepath.Join(cache, "payload.bin")) {
				t.Fatalf("a whitelisted case alias reached the file move boundary: %+v", outcome)
			}
			after, err := os.ReadFile(WhitelistPath(home))
			if err != nil || string(after) != string(before) {
				t.Fatal("case-alias protection rewrote the user's whitelist")
			}
		})
	}
}

func TestAIAgentLogLatestReferenceAndOpenHandleAreRetained(t *testing.T) {
	home := fixtureHome(t)
	root := filepath.Join(home, ".claude/debug")
	old := filepath.Join(root, "old.txt")
	current := filepath.Join(root, "current.txt")
	writeFile(t, old, "old diagnostic")
	writeFile(t, current, "current diagnostic")
	oldTime := time.Now().Add(-60 * 24 * time.Hour)
	if err := os.Chtimes(old, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	scanner := &Scanner{home: home, processProbe: func() (string, error) { return "/usr/bin/idle\n", nil }, handleProbe: func(string) (bool, error) { return false, nil }}
	latest := filepath.Join(root, "latest")
	if err := os.Symlink(old, latest); err != nil {
		t.Fatal(err)
	}
	if reason := scanner.CleanupReason(old); !strings.Contains(reason, "latest") {
		t.Fatalf("a latest log target must be retained independently of its age: %s", reason)
	}
	if err := os.Remove(latest); err != nil {
		t.Fatal(err)
	}
	scanner.handleProbe = func(string) (bool, error) { return true, nil }
	if reason := scanner.CleanupReason(old); !strings.Contains(reason, "打开") {
		t.Fatalf("an open old log cannot be removed: %s", reason)
	}
	scanner.handleProbe = func(string) (bool, error) { return false, nil }
	scanner.processProbe = func() (string, error) { return "/usr/local/bin/claude\n", nil }
	if reason := scanner.CleanupReason(old); !strings.Contains(reason, "运行") {
		t.Fatalf("running Claude Code protects diagnostics even without an open handle: %s", reason)
	}
	scanner.processProbe = func() (string, error) { return "/usr/bin/idle\n", nil }
	if err := os.Link(old, filepath.Join(home, "saved-diagnostic.txt")); err != nil {
		t.Fatal(err)
	}
	if reason := scanner.CleanupReason(old); !strings.Contains(reason, "硬链接") {
		t.Fatalf("a diagnostic shared with another path remains protected: %s", reason)
	}
}

func TestAIAgentCachesStayDisjointAndReportWhitelistExclusions(t *testing.T) {
	home := fixtureHome(t)
	cache := fixtureDir(t, home, "Library/Caches/Cursor", 4096)
	codeCache := fixtureDir(t, home, "Library/Application Support/Claude/Code Cache", 4096)
	scanner := &Scanner{home: home, processProbe: func() (string, error) { return "/usr/bin/idle\n", nil }, handleProbe: func(string) (bool, error) { return false, nil }}
	for _, row := range scanner.scanCleanGroups() {
		for _, path := range subpathPaths(row) {
			if path == scanner.relativize(cache) {
				t.Fatal("the same AI cache is reported again by generic cleanup")
			}
		}
	}
	writeWhitelist(t, home, "~/Library/Application Support")
	scanner.refreshWhitelist()
	rows := scanner.aiExcludedRoots()
	if len(rows) != 1 || rows[0]["path"] != scanner.relativize(codeCache) || rows[0]["tool"] != "Claude Desktop" {
		t.Fatalf("a recognized AI cache must explain its whitelist exclusion: %v", rows)
	}
	for _, row := range scanner.scanAICaches() {
		for _, path := range subpathPaths(row) {
			if path == scanner.relativize(codeCache) {
				t.Fatal("excluded cache evidence cannot re-add a protected candidate")
			}
		}
	}
}

func TestAIDiagnosticLimitRemainsVisibleAfterSafetyChecks(t *testing.T) {
	home := fixtureHome(t)
	root := filepath.Join(home, ".copilot/logs")
	oldTime := time.Now().Add(-60 * 24 * time.Hour)
	for index := range 42 {
		path := filepath.Join(root, fmt.Sprintf("process-%02d.log", index))
		writeFile(t, path, "diagnostic")
		if err := os.Chtimes(path, oldTime.Add(time.Duration(index)*time.Minute), oldTime.Add(time.Duration(index)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	scanner := &Scanner{home: home, processProbe: func() (string, error) { return "/usr/bin/idle\n", nil }, handleProbe: func(string) (bool, error) { return false, nil }}
	rows := scanner.scanAICaches()
	for _, row := range rows {
		if row["tool"] == "GitHub Copilot CLI" && row["band"] == "诊断日志" && len(row["subpaths"].([]map[string]any)) > 40 {
			t.Fatal("diagnostic candidates exceeded the declared sample limit")
		}
	}
	for _, warning := range scanReadWarnings(home) {
		if warning["path"] == root && strings.Contains(warning["reason"].(string), "40") {
			return
		}
	}
	t.Fatal("the diagnostic sample limit disappeared after per-file safety checks")
}
