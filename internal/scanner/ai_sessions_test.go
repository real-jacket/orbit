package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func ageFile(t *testing.T, path string, days int) {
	t.Helper()
	when := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

func sessionScanner(t *testing.T, installed bool) *Scanner {
	s := newTestScanner(t)
	old := aiUnitCacheTTL
	aiUnitCacheTTL = 0 // these tests edit files between calls
	t.Cleanup(func() { aiUnitCacheTTL = old })
	s.processProbe = func() (string, error) { return "/usr/bin/idle\n", nil }
	s.handleProbe = func(string) (bool, error) { return false, nil }
	s.installProbe = func(aiInstall) bool { return installed }
	return s
}

func TestAIExpiredCodexSessionsKeepNewestAndRecent(t *testing.T) {
	s := sessionScanner(t, true)
	installOwners(t, s.home)
	var rollouts []string
	for i := 0; i < 8; i++ { // days 200..130, all expired by age
		path := filepath.Join(s.home, ".codex/sessions/2026/01/02", fmt.Sprintf("rollout-2026-01-02T00-00-0%d-id.jsonl", i))
		writeFile(t, path, "x")
		ageFile(t, path, 200-i*10)
		rollouts = append(rollouts, path)
	}
	recent := filepath.Join(s.home, ".codex/archived_sessions/rollout-recent.jsonl.zst")
	writeFile(t, recent, "x")
	ageFile(t, recent, 10)
	auth := filepath.Join(s.home, ".codex/auth.json")
	writeFile(t, auth, "secret")
	ageFile(t, auth, 400)
	index := filepath.Join(s.home, ".codex/session_index.jsonl")
	writeFile(t, index, "x")
	ageFile(t, index, 400)

	if reason := s.CleanupReason(rollouts[0]); reason != "" { // oldest: not among the newest 5 of 9
		t.Fatalf("an expired old rollout should be removable: %s", reason)
	}
	if reason := s.CleanupReason(rollouts[7]); reason == "" {
		t.Fatal("the newest sessions of the tool are retained even when old")
	}
	for _, path := range []string{recent, auth, index, filepath.Join(s.home, ".codex/sessions")} {
		if reason := s.CleanupReason(path); reason == "" {
			t.Fatalf("%s must stay protected", path)
		}
	}
	if outcome := s.TrashPaths([]string{recent, rollouts[7]}); len(outcome.Moved) != 0 || len(outcome.Skipped) != 2 {
		t.Fatalf("the move boundary must refuse non-expired sessions sent by the page: %+v", outcome)
	}
	s.handleProbe = func(path string) (bool, error) { return path == rollouts[0], nil }
	if reason := s.CleanupReason(rollouts[0]); !strings.Contains(reason, "打开") {
		t.Fatalf("an open rollout stays: %s", reason)
	}
	s.handleProbe = func(string) (bool, error) { return false, nil }

	listed := 0
	for _, row := range s.scanAICaches() {
		if row["band"] == "过期会话" && row["tool"] == "Codex" {
			if row["tier"] != "review" || row["checked"] != false || row["uninstalled"] == true || row["ageText"] == nil {
				t.Fatalf("expired sessions are an unchecked review band: %v", row)
			}
			listed += row["items"].(int)
		}
	}
	if listed != 4 { // 9 rollouts minus the newest 5
		t.Fatalf("listed %d expired rollouts", listed)
	}
	if outcome := s.TrashPaths([]string{rollouts[0]}); len(outcome.Moved) != 1 {
		t.Fatalf("an expired unit moves to the Trash: %+v", outcome)
	}

	s.installProbe = func(aiInstall) bool { return false }
	if reason := s.CleanupReason(rollouts[7]); reason == "" {
		t.Fatal("the newest 5 are kept even when the tool looks uninstalled")
	}
	if reason := s.CleanupReason(recent); reason == "" {
		t.Fatal("recent sessions stay even when the tool is gone")
	}
	for _, row := range s.scanAICaches() {
		if row["band"] == "过期会话" && row["uninstalled"] != true {
			t.Fatalf("uninstalled tools are marked: %v", row)
		}
	}
}

func TestAIExpiredClaudeSessionUnitsAreWholeAndRankedPerProject(t *testing.T) {
	s := sessionScanner(t, true)
	installOwners(t, s.home)
	project := filepath.Join(s.home, ".claude/projects/-tmp-proj")
	ids := make([]string, 7)
	for i := range ids {
		ids[i] = fmt.Sprintf("00000000-0000-0000-0000-00000000000%d", i)
		writeFile(t, filepath.Join(project, ids[i]+".jsonl"), "x")
		writeFile(t, filepath.Join(project, ids[i], "subagents/a.jsonl"), "x")
		writeFile(t, filepath.Join(s.home, ".claude/file-history", ids[i], "v1"), "x")
		for _, path := range []string{filepath.Join(project, ids[i], "subagents/a.jsonl"), filepath.Join(project, ids[i], "subagents"), filepath.Join(project, ids[i]), filepath.Join(project, ids[i]+".jsonl"), filepath.Join(s.home, ".claude/file-history", ids[i], "v1"), filepath.Join(s.home, ".claude/file-history", ids[i])} {
			ageFile(t, path, 300-i*10)
		}
	}
	memory := filepath.Join(project, "memory")
	writeFile(t, filepath.Join(memory, "note.md"), "x")
	ageFile(t, memory, 400)
	ageFile(t, filepath.Join(memory, "note.md"), 400)
	// A recent write deep inside an old session keeps the whole unit.
	touched := filepath.Join(project, ids[0], "subagents/a.jsonl")
	ageFile(t, touched, 1)
	if reason := s.CleanupReason(filepath.Join(project, ids[0]+".jsonl")); reason == "" {
		t.Fatal("recent activity inside the unit keeps its transcript")
	}
	ageFile(t, touched, 300)
	for _, path := range []string{filepath.Join(project, ids[0]+".jsonl"), filepath.Join(project, ids[0]), filepath.Join(s.home, ".claude/file-history", ids[0])} {
		if reason := s.CleanupReason(path); reason != "" {
			t.Fatalf("an expired unit member should be removable together: %s %s", path, reason)
		}
	}
	// Newest 5 of 7 remain: ids[0] and ids[1] are the oldest.
	if reason := s.CleanupReason(filepath.Join(project, ids[2]+".jsonl")); reason == "" {
		t.Fatal("the newest 5 sessions of a project are kept")
	}
	for _, path := range []string{memory, filepath.Join(memory, "note.md"), filepath.Join(project, "notauuid.jsonl"), filepath.Join(s.home, ".claude/history.jsonl"), filepath.Join(s.home, ".claude/settings.json")} {
		if reason := s.CleanupReason(path); reason == "" {
			t.Fatalf("%s must stay protected", path)
		}
	}
	snapshot := filepath.Join(s.home, ".claude/shell-snapshots/snapshot-zsh-1.sh")
	writeFile(t, snapshot, "x")
	ageFile(t, snapshot, 120)
	if reason := s.CleanupReason(snapshot); reason != "" {
		t.Fatalf("an expired shell snapshot is disposable: %s", reason)
	}
	s.processProbe = func() (string, error) { return "", fmt.Errorf("ps failed") }
	if reason := s.cleanupReason(snapshot, true); reason == "" {
		t.Fatal("unknown process state keeps even expired units")
	}
}

func TestAISessionCacheRechecksTheUnitAtDecisionTime(t *testing.T) {
	s := sessionScanner(t, true)
	aiUnitCacheTTL = time.Minute
	var rollouts []string
	for i := 0; i < 7; i++ {
		path := filepath.Join(s.home, ".codex/archived_sessions", fmt.Sprintf("rollout-%d.jsonl", i))
		writeFile(t, path, "x")
		ageFile(t, path, 300-i*10)
		rollouts = append(rollouts, path)
	}
	if reason := s.CleanupReason(rollouts[0]); reason != "" {
		t.Fatalf("expired and not among the newest 5: %s", reason)
	}
	if s.unitCache == nil {
		t.Fatal("the enumeration should be cached for later calls")
	}
	ageFile(t, rollouts[0], 0) // touched after it was cached
	if reason := s.CleanupReason(rollouts[0]); reason == "" {
		t.Fatal("a cached unit that is now recent must be refused for display")
	}
	if outcome := s.TrashPaths([]string{rollouts[0]}); len(outcome.Moved) != 0 || len(outcome.Skipped) != 1 {
		t.Fatalf("the move boundary must re-stat the cached unit: %+v", outcome)
	}
}
