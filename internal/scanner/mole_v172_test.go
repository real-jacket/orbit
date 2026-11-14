package scanner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSwiftPMDiscoveryOffersOutputsWithoutDependencyOrWorkspaceState(t *testing.T) {
	s, _ := ownerScanner(t)
	project := filepath.Join(s.home, "Projects/Swift")
	writeFile(t, filepath.Join(project, "Package.swift"), "// swift package")
	scratch := filepath.Join(project, ".build")
	writeFile(t, filepath.Join(scratch, "checkouts/Package/.git/config"), "dependency")
	writeFile(t, filepath.Join(scratch, "repositories/Package/config"), "repository")
	writeFile(t, filepath.Join(scratch, "workspace-state.json"), "resolved state")
	output := fixtureDir(t, project, ".build/arm64-apple-macosx", 4096)
	indexed := fixtureDir(t, project, ".build/index-build/arm64-apple-macosx", 4096)
	writeFile(t, filepath.Join(scratch, "index-build/checkouts/Package/.git/config"), "index dependency")
	if err := os.Symlink(output, filepath.Join(scratch, "x86_64-apple-macosx")); err != nil {
		t.Fatal(err)
	}
	var candidates []artifactCandidate
	s.walkArtifacts(project, time.Now().Add(time.Second), &candidates)
	var paths []string
	for _, candidate := range candidates {
		paths = append(paths, candidate.path)
	}
	if want := []string{indexed, output}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("only real compilation and index outputs may be offered: %v", paths)
	}
	if got := worktreeArtifacts(project); !reflect.DeepEqual(got, []string{indexed, output}) {
		t.Fatalf("agent worktrees must use the same decomposition: %v", got)
	}
	if swiftPMOutputRoot(indexed) != scratch || rebuildCommand(project, ".build") != "swift build" {
		t.Fatal("nested index outputs lost their Swift project attribution")
	}
}

func TestSwiftPMCustomScratchRequiresItsOwnTagAndKeepsBinaryDependencies(t *testing.T) {
	s, _ := ownerScanner(t)
	project := filepath.Join(s.home, "Projects/CustomSwift")
	writeFile(t, filepath.Join(project, "Package.swift"), "// swift package")
	custom := filepath.Join(project, "scratch")
	writeFile(t, filepath.Join(custom, "CACHEDIR.TAG"), cacheDirTagSig+"\n# Swift Package Manager\n")
	writeFile(t, filepath.Join(custom, "artifacts/Binary.xcframework/payload"), "downloaded dependency")
	output := fixtureDir(t, project, "scratch/out", 4096)
	if got := swiftPMArtifacts(custom); !reflect.DeepEqual(got, []string{output}) {
		t.Fatalf("binary-only packages must keep downloads and offer only output: %v", got)
	}
	ordinary := filepath.Join(project, "build")
	writeFile(t, filepath.Join(ordinary, "CACHEDIR.TAG"), cacheDirTagSig+"\n# Swift Package Manager\n")
	writeFile(t, filepath.Join(ordinary, "workspace-state.json"), "not proof of SwiftPM")
	if isSwiftPMScratchRoot(ordinary) || !reflect.DeepEqual(swiftPMArtifacts(ordinary), []string{ordinary}) {
		t.Fatal("another build target must retain its own safety checks")
	}
	writeFile(t, filepath.Join(custom, "CACHEDIR.TAG"), "invalid signature\nSwift Package Manager")
	if isSwiftPMScratchRoot(custom) {
		t.Fatal("an invalid custom cache tag cannot establish a SwiftPM scratch boundary")
	}
	writeFile(t, filepath.Join(custom, "CACHEDIR.TAG"), cacheDirTagSig+strings.Repeat(" ", 4096)+"Swift Package Manager")
	if isSwiftPMScratchRoot(custom) {
		t.Fatal("custom scratch proof must stay inside the bounded tag header")
	}
}

func TestSwiftPMMoveRechecksNewDependenciesActivityAndAuthoredFiles(t *testing.T) {
	s, _ := ownerScanner(t)
	writeFile(t, WhitelistPath(s.home), "")
	s.builderProbe = func() ([]builderProc, error) { return nil, nil }
	project := filepath.Join(s.home, "Projects/Swift")
	writeFile(t, filepath.Join(project, "Package.swift"), "// swift package")
	output := fixtureDir(t, project, ".build/out", 4096)
	scratch := filepath.Dir(output)
	if got := swiftPMArtifacts(scratch); !reflect.DeepEqual(got, []string{scratch}) {
		t.Fatal("an unresolved scratch root was not discovered")
	}
	custom := fixtureDir(t, project, "swift-scratch/out", 4096)
	custom = filepath.Dir(custom)
	writeFile(t, filepath.Join(custom, "CACHEDIR.TAG"), cacheDirTagSig+"\n# Swift Package Manager\n")
	for _, root := range []string{scratch, custom} {
		s.handleProbe = func(string) (bool, error) { return true, nil }
		if reason := s.cleanupReason(root, true); !strings.Contains(reason, "打开") {
			t.Fatalf("whole scratch roots must retain open files: %s, %q", root, reason)
		}
		s.handleProbe = func(string) (bool, error) { return false, nil }
		s.builderProbe = func() ([]builderProc, error) { return []builderProc{{name: "swift", cwd: project}}, nil }
		if reason := s.cleanupReason(root, true); !strings.Contains(reason, "正在运行") {
			t.Fatalf("custom scratch roots cannot bypass builder checks: %s, %q", root, reason)
		}
		s.builderProbe = func() ([]builderProc, error) { return nil, nil }
	}
	writeFile(t, filepath.Join(scratch, "checkouts/Package/file.swift"), "new dependency after scan")
	if outcome := s.TrashPaths([]string{scratch}); len(outcome.Moved) != 0 || len(outcome.Skipped) != 1 || !strings.Contains(outcome.Skipped[0].Reason, "SwiftPM") {
		t.Fatalf("new dependencies must block a stale whole-root move: %+v", outcome)
	}
	s.builderProbe = func() ([]builderProc, error) {
		return []builderProc{{name: "sourcekit-lsp", cwd: project}}, nil
	}
	if reason := s.cleanupReason(output, true); !strings.Contains(reason, "正在运行") {
		t.Fatalf("indexing must retain SwiftPM output: %q", reason)
	}
	s.builderProbe = func() ([]builderProc, error) { return nil, nil }
	s.handleProbe = func(string) (bool, error) { return true, nil }
	if reason := s.cleanupReason(output, true); !strings.Contains(reason, "打开") {
		t.Fatalf("open output must stay even after its builder exits: %q", reason)
	}
	s.handleProbe = func(string) (bool, error) { return false, nil }
	writeWhitelist(t, s.home, "~/Projects/Swift/.build")
	if reason := s.cleanupReason(output, true); !strings.Contains(reason, "白名单") {
		t.Fatalf("decomposition cannot bypass a parent whitelist: %q", reason)
	}
	writeWhitelist(t, s.home)
	writeFile(t, filepath.Join(output, ".git/config"), "authored content after scan")
	if reason := s.cleanupReason(output, true); !strings.Contains(reason, ".git") {
		t.Fatalf("new authored content must block the final move: %q", reason)
	}
}

func TestLaunchAgentOwnershipDropsOnlyProvablyDeadJobs(t *testing.T) {
	s, _ := ownerScanner(t)
	physical, err := filepath.EvalSymlinks(s.home)
	if err != nil {
		t.Fatal(err)
	}
	s.home = physical
	missing := filepath.Join(s.home, "Applications/Gone.app/Contents/MacOS/Gone")
	live := filepath.Join(s.home, ".local/bin/live")
	writeFile(t, live, "installed program")
	plist := filepath.Join(s.home, "Library/LaunchAgents/org.gone.plist")
	writeFile(t, plist, "fixture")
	missingPathErr := fmt.Errorf("%w: stat: %s", exec.Command("/bin/sh", "-c", "exit 1").Run(), syscall.ENOENT.Error())
	if !s.launchAgentPathMissing(context.Background(), missing, nil) {
		t.Fatal("a native missing-path probe did not recognize a dead local executable")
	}
	for _, test := range []struct {
		name  string
		props map[string]any
		keep  bool
	}{
		{"missing program", map[string]any{"Program": missing}, false},
		{"missing plain script", map[string]any{"Program": "/bin/sh", "ProgramArguments": []string{"sh", missing}}, false},
		{"inline shell code", map[string]any{"Program": "/bin/sh", "ProgramArguments": []string{"sh", "-c", missing}}, true},
		{"unknown Program precedence", map[string]any{"Program": nil, "ProgramArguments": []string{missing}}, true},
		{"PATH command", map[string]any{"ProgramArguments": []string{"gone-tool"}}, true},
		{"disabled live job", map[string]any{"Disabled": true, "Program": live}, true},
		{"privacy alias", map[string]any{"Program": filepath.Join(s.home, "documents/Gone/tool")}, true},
		{"unplugged volume", map[string]any{"Program": "/Volumes/Offline/Gone"}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			data, _ := json.Marshal(test.props)
			s.optimizeRun = func(_ context.Context, _ string, name string, _ ...string) (string, error) {
				if name == "/usr/bin/env" {
					return "", missingPathErr
				}
				return string(data), nil
			}
			if got := s.launchAgentCanRun(context.Background(), plist, nil); got != test.keep {
				t.Fatalf("keep ownership = %v, want %v", got, test.keep)
			}
		})
	}
	alias := filepath.Join(s.home, "Applications/Alias")
	if err := os.MkdirAll(filepath.Dir(alias), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(s.home, "Offline"), alias); err != nil {
		t.Fatal(err)
	}
	if s.launchAgentPathMissing(context.Background(), filepath.Join(alias, "tool"), nil) || s.launchAgentPathMissing(context.Background(), missing, []string{filepath.Join(s.home, "Applications")}) {
		t.Fatal("links and configured mounts must retain the ownership claim")
	}
	s.optimizeRun = func(_ context.Context, _ string, name string, _ ...string) (string, error) {
		if name == "/usr/bin/env" {
			return "", missingPathErr
		}
		return `{"Program":"` + missing + `"}`, nil
	}
	_, ids := s.installedAppIndex(s.home)
	if _, exists := ids["org.gone"]; exists {
		t.Fatal("a verified dead agent still prevented orphan discovery")
	}
	s.optimizeRun = func(context.Context, string, string, ...string) (string, error) {
		return "", errors.New("unreadable plist")
	}
	_, ids = s.installedAppIndex(s.home)
	if _, exists := ids["org.gone"]; !exists {
		t.Fatal("unknown agent metadata lost its ownership claim")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !s.launchAgentCanRun(ctx, plist, nil) {
		t.Fatal("a spent judging budget cannot declare a job dead")
	}
	localHome := s.home
	s.home = "/Volumes/Offline/Users/fixture"
	if s.launchAgentPathMissing(context.Background(), s.home+"/Applications/Gone.app/tool", nil) {
		t.Fatal("an external home cannot make an unplugged volume look like an uninstalled app")
	}
	s.home = localHome
	for _, err := range []error{context.DeadlineExceeded, errors.New("stat: Permission denied")} {
		s.optimizeRun = func(context.Context, string, string, ...string) (string, error) { return "", err }
		if s.launchAgentPathMissing(context.Background(), missing, nil) {
			t.Fatalf("an uncertain path probe cannot drop ownership: %v", err)
		}
	}
}

func TestLaunchAgentMountConfigurationProtectsDirectMapsAndEscapedPaths(t *testing.T) {
	dir := t.TempDir()
	saved := launchAgentMountFiles
	t.Cleanup(func() { launchAgentMountFiles = saved })
	launchAgentMountFiles = []struct{ path, format string }{{filepath.Join(dir, "fstab"), "fstab"}, {filepath.Join(dir, "auto_master"), "master"}}
	writeFile(t, filepath.Join(dir, "fstab"), "UUID=fixture /Applications/Offline\\040Disk apfs rw 0 0\n")
	writeFile(t, filepath.Join(dir, "auto_master"), "# comment\n+auto_master\n/net -hosts\n/- auto_direct\n")
	writeFile(t, filepath.Join(dir, "auto_direct"), "/opt/homebrew/Offline -fstype=nfs server:/share\n")
	mounts, err := configuredLaunchAgentMounts()
	if want := []string{"/Applications/Offline Disk", "/net", "/opt/homebrew/Offline"}; err != nil || !reflect.DeepEqual(mounts, want) {
		t.Fatalf("configured mount exclusions = %v, %v", mounts, err)
	}
	writeFile(t, filepath.Join(dir, "auto_direct"), strings.Repeat("x", (1<<20)+1))
	if _, err := configuredLaunchAgentMounts(); err == nil {
		t.Fatal("unknown mount configuration must keep every remaining claim")
	}
}

func TestContainerSystemProtectionRunsOnlyAtRootMoveWithFullDiskEvidence(t *testing.T) {
	s, _ := ownerScanner(t)
	writeFile(t, WhitelistPath(s.home), "")
	container := fixtureDir(t, s.home, "Library/Containers/org.fixture.gone", 4096)
	writeFile(t, s.tccDBPath(), "synthetic FDA probe evidence")
	calls := 0
	s.optimizeRun = func(_ context.Context, _ string, name string, args ...string) (string, error) {
		if name != "/usr/bin/find" || args[0] != container || !strings.Contains(strings.Join(args, " "), "com.apple.rootless") {
			t.Fatalf("unexpected container protection command: %s %v", name, args)
		}
		calls++
		return filepath.Join(container, "protected-entry"), nil
	}
	_ = s.CleanupReason(container)
	if calls != 0 {
		t.Fatal("display scans must not probe protected container descendants")
	}
	if outcome := s.TrashPaths([]string{container}); len(outcome.Moved) != 0 || len(outcome.Skipped) != 1 || !strings.Contains(outcome.Skipped[0].Reason, "废纸篓") || calls != 1 {
		t.Fatalf("a protected root must stay outside Trash, probed once: %+v, calls %d", outcome, calls)
	}
	s.optimizeRun = func(context.Context, string, string, ...string) (string, error) { return "", context.DeadlineExceeded }
	if reason := s.cleanupReason(container, true); !strings.Contains(reason, "无法核验沙盒") {
		t.Fatalf("uncertain system protection must preserve the container: %q", reason)
	}
	if reason := s.probeContainerSystemProtection(filepath.Join(container, "Data/Library/Caches")); reason != "" {
		t.Fatal("cache leaves must not trigger a root-container protection walk")
	}
	if err := os.Remove(s.tccDBPath()); err != nil {
		t.Fatal(err)
	}
	if reason := s.probeContainerSystemProtection(container); reason != "" {
		t.Fatal("without confirmed FDA no descendant walk may run")
	}
}

func TestClaudeHistoryPuppeteerAndRecentVMDescendantsStayProtected(t *testing.T) {
	s, _ := ownerScanner(t)
	writeFile(t, WhitelistPath(s.home), "")
	for _, raw := range []string{".claude", ".claude/projects", ".CLAUDE/FILE-HISTORY", ".claude/history.jsonl", ".cache/puppeteer"} {
		if reason := s.cleanupReason(filepath.Join(s.home, raw), true); reason == "" {
			t.Errorf("an absent owner cannot relax this protection: %s", raw)
		}
	}
	vm := fixtureDir(t, s.home, claudeVMDir+"/old.bundle", 4096)
	old := time.Now().Add(-10 * 24 * time.Hour)
	if err := os.Chtimes(vm, old, old); err != nil {
		t.Fatal(err)
	}
	if reason := s.claudeVMReason(vm, true); !strings.Contains(reason, "7 天") {
		t.Fatalf("a recent disk image inside an old VM root must stay: %q", reason)
	}
	if reason := s.claudeVMReason(filepath.Join(s.home, claudeSupportDir, "vm/old.bundle"), true); reason == "" {
		t.Fatal("unidentified Claude bundle locations cannot be treated as VM orphans")
	}
}
