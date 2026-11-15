package scanner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func worktreeGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "core.hooksPath=/dev/null", "-c", "user.name=orbit test", "-c", "user.email=orbit@example.invalid", "-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s, %v", args, output, err)
	}
	return strings.TrimSpace(string(output))
}

func worktreeProject(t *testing.T, s *Scanner) (string, string) {
	t.Helper()
	home, err := filepath.EvalSymlinks(s.home)
	if err != nil {
		t.Fatal(err)
	}
	s.home = home
	project := filepath.Join(s.home, "Projects", "Source Project")
	writeFile(t, filepath.Join(project, "source.txt"), "committed source")
	writeFile(t, filepath.Join(project, ".gitignore"), "node_modules/\nbuild/\n")
	worktreeGit(t, project, "init", "-q", "--initial-branch=main")
	worktreeGit(t, project, "add", "source.txt", ".gitignore")
	worktreeGit(t, project, "commit", "-qm", "source")
	tree := filepath.Join(s.home, ".codex/worktrees/task", "Source Project")
	worktreeGit(t, project, "worktree", "add", "-qb", "task", tree)
	s.processProbe = func() (string, error) { return "/sbin/launchd\n", nil }
	s.handleProbe = func(string) (bool, error) { return false, nil }
	return project, tree
}

func TestGitProjectDiscoveryIncludesDeepAndArtifactNamedRepositories(t *testing.T) {
	home := fixtureHome(t)
	s := &Scanner{home: home}
	root := filepath.Join(home, "Projects")
	for _, name := range []string{"build", "dist", "Library", "a/b/c/d/e/f/g/h/project"} {
		repo := filepath.Join(root, name)
		writeFile(t, filepath.Join(repo, ".git"), "gitdir: /fixture/worktree")
		writeFile(t, filepath.Join(repo, "node_modules/package/index.js"), "dependency")
	}
	writeFile(t, filepath.Join(root, ".git/ignored/node_modules/file"), "metadata")
	var artifacts []artifactCandidate
	repos := s.walkArtifacts(root, time.Now().Add(time.Minute), &artifacts)
	if len(repos) != 5 || len(artifacts) != 4 {
		t.Fatalf("every Git checkout must be analyzed regardless of container depth or name: repos=%v artifacts=%v", repos, artifacts)
	}
	for _, artifact := range artifacts {
		if artifact.name != "node_modules" {
			t.Fatalf("a repository itself was mistaken for disposable build output: %+v", artifact)
		}
	}
}

func TestOldBuildArtifactsIgnoreFreshProjectAndGitMetadata(t *testing.T) {
	s := newTestScanner(t)
	project, _ := worktreeProject(t, s)
	nodeProject(t, s.home, "Projects/Source Project")
	build := fixtureDir(t, s.home, "Projects/Source Project/build", 21_000_000)
	old := time.Now().Add(-83 * 24 * time.Hour)
	for _, path := range []string{filepath.Join(project, "node_modules"), build} {
		if err := filepath.WalkDir(path, func(path string, _ os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			return os.Chtimes(path, old, old)
		}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	for _, path := range []string{project, filepath.Join(project, "package.json"), filepath.Join(project, ".git/index")} {
		if err := os.Chtimes(path, now, now); err != nil {
			t.Fatal(err)
		}
	}
	rows := s.scanBuildArtifactsWithRoots([]string{project})
	if len(rows) != 2 {
		t.Fatalf("expected both old artifacts: %v", rows)
	}
	for _, row := range rows {
		if row["tier"] != "safe" || row["checked"] != true || !strings.Contains(row["reason"].(string), "83") {
			t.Fatalf("fresh parent metadata must not protect old, reproducible artifacts: %v", row)
		}
	}
}

func TestAIWorktreesUseSourceRepositoryAndProtectDirtyCheckouts(t *testing.T) {
	s := newTestScanner(t)
	project, first := worktreeProject(t, s)
	second := filepath.Join(s.home, "Projects", "Source Project-fix")
	worktreeGit(t, project, "worktree", "add", "-qb", "fix", second)
	writeFile(t, filepath.Join(first, "untracked-note.txt"), "unfinished work")
	rows := s.scanAIWorktrees(&catalogClaims{})
	if len(rows) != 2 {
		t.Fatalf("agent-owned and source-adjacent worktrees must both be inventoried once: %v", rows)
	}
	for _, row := range rows {
		if row["sourceProject"] != s.relativize(project) || row["checked"] != false || row["wholeWorktree"] != true {
			t.Fatalf("worktrees must retain their source and never be preselected: %v", row)
		}
		want := "review"
		if row["path"] == s.relativize(first) {
			want = "protected"
		}
		if row["tier"] != want {
			t.Fatalf("clean checkouts need confirmation and dirty checkouts stay protected: %v", row)
		}
	}
	worktreeGit(t, project, "worktree", "lock", second)
	if reason := s.cleanupReason(second, true); !strings.Contains(reason, "锁定") {
		t.Fatalf("locked worktrees must not be moved: %q", reason)
	}
	if reason := s.cleanupReason(project, true); !strings.Contains(reason, "源仓库") {
		t.Fatalf("the source repository must never become a cleanup candidate: %q", reason)
	}
}

func TestWorktreeTrashAndRestorePreserveGitLinksAndUnmergedCommit(t *testing.T) {
	s := newTestScanner(t)
	project, tree := worktreeProject(t, s)
	writeFile(t, filepath.Join(tree, "task.txt"), "work that has never been merged or pushed")
	worktreeGit(t, tree, "add", "task.txt")
	worktreeGit(t, tree, "commit", "-qm", "unmerged task")
	commit := worktreeGit(t, tree, "rev-parse", "HEAD")
	result := s.TrashPaths([]string{tree})
	if len(result.Moved) != 1 || len(result.Failed) != 0 || len(result.Skipped) != 0 || result.RestoreError != "" {
		t.Fatalf("a confirmed clean worktree should move into the isolated Trash: %+v", result)
	}
	trashed := result.Moved[0].Trashed
	info, err := gitWorktreeAt(trashed)
	if err != nil || info.branch != "task" || worktreeGit(t, trashed, "rev-parse", "HEAD") != commit {
		t.Fatalf("both Git pointers and the unmerged commit must survive the move: %+v, %v", info, err)
	}
	if rows := s.scanAIWorktrees(&catalogClaims{}); len(rows) != 0 {
		t.Fatalf("a worktree already in Trash must not reappear as a candidate: %v", rows)
	}
	restored := s.RestoreTrash([]string{tree})
	if len(restored.Restored) != 1 || len(restored.Failed) != 0 {
		t.Fatalf("worktree restore must restore its Git association too: %+v", restored)
	}
	if _, err := gitWorktreeAt(tree); err != nil || worktreeGit(t, tree, "rev-parse", "HEAD") != commit || worktreeGit(t, project, "rev-parse", "task") != commit {
		t.Fatalf("restored checkout and source branch must still reach the unmerged commit: %v", err)
	}
	writeFile(t, filepath.Join(tree, "new-untracked-file.txt"), "new work after preview")
	if result := s.TrashPaths([]string{tree}); len(result.Moved) != 0 || len(result.Skipped) != 1 || !exists(tree) {
		t.Fatalf("changes after preview must preserve the entire worktree at the move boundary: %+v", result)
	}
}

func TestWorktreeWithSubmoduleConfigurationStaysProtected(t *testing.T) {
	s := newTestScanner(t)
	_, tree := worktreeProject(t, s)
	writeFile(t, filepath.Join(tree, ".gitmodules"), "[submodule \"library\"]\n\tpath = library\n\turl = ../library\n")
	worktreeGit(t, tree, "add", ".gitmodules")
	worktreeGit(t, tree, "commit", "-qm", "submodule configuration")
	if state := worktreeGit(t, tree, "status", "--porcelain"); state != "" {
		t.Fatalf("fixture must be a clean checkout: %q", state)
	}
	rows := s.scanAIWorktrees(&catalogClaims{})
	if len(rows) != 1 || rows[0]["tier"] != "protected" || rows[0]["checked"] != false || !strings.Contains(rows[0]["reason"].(string), "子模块配置") {
		t.Fatalf("clean submodule checkouts must retain their Git pointers: %v", rows)
	}
	if result := s.TrashPaths([]string{tree}); len(result.Moved) != 0 || len(result.Skipped) != 1 || !exists(tree) {
		t.Fatalf("submodule configuration must block moving the whole checkout at execution too: %+v", result)
	}
}

func TestWorktreeWithProtectedDescendantCannotOfferSourceChildren(t *testing.T) {
	s := newTestScanner(t)
	_, tree := worktreeProject(t, s)
	writeFile(t, filepath.Join(tree, "other-source.txt"), "another committed source file")
	worktreeGit(t, tree, "add", "other-source.txt")
	worktreeGit(t, tree, "commit", "-qm", "another source file")
	writeFile(t, WhitelistPath(s.home), filepath.Join(tree, "source.txt")+"\n")
	rows := s.scanAIWorktrees(&catalogClaims{})
	if len(rows) != 1 || rows[0]["tier"] != "protected" || rows[0]["checked"] != false || rows[0]["partial"] != true {
		t.Fatalf("a protected descendant blocks the whole checkout and marks its excluded measurement: %v", rows)
	}
	for _, path := range rows[0]["subpaths"].([]map[string]any) {
		if path["tier"] != "protected" || path["checked"] != false {
			t.Fatalf("source children must never become partial worktree cleanup candidates: %v", path)
		}
	}
	if result := s.TrashPaths([]string{tree}); len(result.Moved) != 0 || len(result.Skipped) != 1 || !exists(tree) {
		t.Fatalf("a protected child must block moving its parent at execution too: %+v", result)
	}
}

func TestWorktreeOpenHandlesPreventCleanup(t *testing.T) {
	s := newTestScanner(t)
	_, tree := worktreeProject(t, s)
	s.handleProbe = func(string) (bool, error) { return true, nil }
	if reason := s.cleanupReason(tree, true); !strings.Contains(reason, "被进程打开") {
		t.Fatalf("an open clean checkout must stay protected: %q", reason)
	}
}

func TestWorktreeRepairFailureRollsBackTrashMove(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permissions")
	}
	s := newTestScanner(t)
	_, tree := worktreeProject(t, s)
	data, err := os.ReadFile(filepath.Join(tree, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	admin := strings.TrimSpace(strings.TrimPrefix(string(data), "gitdir:"))
	gitdir := filepath.Join(admin, "gitdir")
	if err := os.Chmod(gitdir, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(admin, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(admin, 0o755); _ = os.Chmod(gitdir, 0o644) })
	result := s.TrashPaths([]string{tree})
	if len(result.Moved) != 0 || len(result.Failed) != 1 || !exists(tree) || !strings.Contains(result.Failed[0].Error, "原路放回") {
		t.Fatalf("a failed Git repair must put the complete checkout back: %+v", result)
	}
	if _, err := gitWorktreeAt(tree); err != nil || worktreeGit(t, tree, "status", "--porcelain") != "" {
		t.Fatalf("rollback must preserve a working, clean Git checkout: %v", err)
	}
}

func TestWorktreeArtifactGuardsTrackedAndAuthoredFiles(t *testing.T) {
	s := newTestScanner(t)
	_, tree := worktreeProject(t, s)
	build := filepath.Join(tree, "build")
	writeFile(t, filepath.Join(build, "manual-source.txt"), "authored source")
	worktreeGit(t, tree, "add", "-f", "build/manual-source.txt")
	if v := s.worktreeArtifactVerdict(build, dirSize{modified: nowUnix() - 83*86400}); v.tier != "protected" {
		t.Fatalf("old tracked output remains protected: %+v", v)
	}
	node := filepath.Join(tree, "node_modules")
	writeFile(t, filepath.Join(node, "wallet-keypair.json"), "authored key")
	if v := s.worktreeArtifactVerdict(node, dirSize{modified: nowUnix() - 83*86400}); v.tier != "protected" {
		t.Fatalf("old dependency folders with authored data remain protected: %+v", v)
	}
	if got := gitCleanState(filepath.Join(tree, "build")); got != "有未提交改动" {
		t.Fatalf("nested project rows must report the owning repository state: %q", got)
	}
}

func TestWorktreeArtifactNamesDistinguishCheckoutAndCacheRoots(t *testing.T) {
	s := newTestScanner(t)
	project, _ := worktreeProject(t, s)
	claude := filepath.Join(s.home, ".claude/worktrees/task")
	worktreeGit(t, project, "worktree", "add", "-qb", "claude", claude)
	nodeProject(t, s.home, ".claude/worktrees/task")
	dependencies := filepath.Join(claude, "node_modules")
	if reason := s.cleanupReason(dependencies, true); reason != "" {
		t.Fatalf("a direct Claude worktree's dependencies must not be mistaken for a whole checkout: %q", reason)
	}
	build := filepath.Join(s.home, "Projects/build")
	worktreeGit(t, project, "worktree", "add", "-qb", "build-task", build)
	if result := s.TrashPaths([]string{build}); len(result.Moved) != 1 || len(result.Failed) != 0 || len(result.Skipped) != 0 {
		t.Fatalf("a registered checkout named build must use worktree safety instead of artifact safety: %+v", result)
	}
}
