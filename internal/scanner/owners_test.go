package scanner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const cursorPlist = `<plist><dict><key>CFBundleIdentifier</key><string>` + cursorBundleID + `</string></dict></plist>`

// ownerScanner has isolated app folders and no process running.
func ownerScanner(t *testing.T) (*Scanner, string) {
	t.Helper()
	s := newTestScanner(t)
	apps := filepath.Join(s.home, "Applications")
	s.appDirs = []string{apps}
	s.processProbe = func() (string, error) { return "/sbin/launchd\n", nil }
	s.handleProbe = func(string) (bool, error) { return false, nil }
	return s, apps
}

func ownerRow(rows []map[string]any, tool, path string) map[string]any {
	for _, row := range rows {
		if row["tool"] != tool {
			continue
		}
		for _, sub := range row["subpaths"].([]map[string]any) {
			if sub["path"] == path {
				return sub
			}
		}
	}
	return nil
}

func TestPersonalDataStaysProtectedWhileItsOwnerIsInstalled(t *testing.T) {
	s, _ := ownerScanner(t)
	session := filepath.Join(s.home, ".codeium/windsurf/cascade/session.pb")
	writeFile(t, session, "chat")
	writeFile(t, filepath.Join(s.home, ".local/bin/windsurf"), "launcher")
	before := s.CleanupReason(session)
	if before == "" || strings.Contains(before, "已卸载") {
		t.Fatalf("installed owner must keep the protected verdict: %q", before)
	}
	if s.cleanupReason(session, true) == "" {
		t.Fatal("the live boundary must refuse too")
	}
	// An unreadable extension folder means the owner cannot be ruled out.
	if err := os.Remove(filepath.Join(s.home, ".local/bin/windsurf")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(s.home, ".vscode"), "not a folder")
	if _, err := os.ReadDir(filepath.Join(s.home, ".vscode/extensions")); err == nil || os.IsNotExist(err) {
		t.Skip("fixture does not produce an undecidable error")
	}
	if s.CleanupReason(session) == "" {
		t.Fatal("an undecidable owner check fails closed")
	}
}

func TestLeftoverDataOfAnUninstalledToolIsReviewableAndPassesTheLiveBoundary(t *testing.T) {
	s, _ := ownerScanner(t)
	session := filepath.Join(s.home, ".codeium/windsurf/cascade/session.pb")
	writeFile(t, session, "chat")
	chat := filepath.Join(s.home, "Library/Application Support/Code/User/globalStorage/github.copilot-chat/history.json")
	writeFile(t, chat, "chat")
	cli := filepath.Join(s.home, ".copilot/session-state/s.json")
	writeFile(t, cli, "chat")
	for _, path := range []string{session, chat, cli} {
		if reason := s.cleanupReason(path, true); reason != "" {
			t.Errorf("leftover data must pass the live boundary: %s: %s", path, reason)
		}
	}
	rows := s.scanAICaches()
	for _, tool := range []string{"Windsurf", "GitHub Copilot Chat", "GitHub Copilot CLI"} {
		var row map[string]any
		for _, r := range rows {
			if r["tool"] == tool {
				row = r
			}
		}
		if row == nil || row["tier"] != "review" || row["checked"] != false || !strings.Contains(row["reason"].(string), tool+" 已卸载") {
			t.Errorf("%s leftover must be an unchecked review row: %v", tool, row)
		}
	}
	// Running owner: protected again, even though the app is gone.
	s.processProbe = func() (string, error) { return "/Applications/Windsurf.app/Contents/MacOS/Windsurf\n", nil }
	if reason := s.cleanupReason(session, true); !strings.Contains(reason, "运行") {
		t.Fatalf("a running owner keeps its data: %q", reason)
	}
	s.processProbe = func() (string, error) { return "", nil }
	if reason := s.cleanupReason(session, true); !strings.Contains(reason, "无法确认") {
		t.Fatalf("an unreadable process table fails closed: %q", reason)
	}
}

func TestOwnerRulesNeverTouchSecretsOrOrbitItself(t *testing.T) {
	s, _ := ownerScanner(t)
	for _, rel := range []string{".ssh/id_ed25519", ".aws/credentials", ".gnupg/key", ".config/gh/hosts.yml", "Library/Keychains/login.keychain-db", "Library/Application Support/orbit/state.json", ".ollama/models/blob", ".cache/huggingface/model"} {
		path := filepath.Join(s.home, rel)
		writeFile(t, path, "secret")
		if s.cleanupReason(path, true) == "" {
			t.Errorf("%s must stay protected without any owner", rel)
		}
	}
}

func cursorFixture(t *testing.T) (*Scanner, string, string) {
	s, apps := ownerScanner(t)
	app := filepath.Join(apps, "Cursor.app")
	writeFile(t, filepath.Join(app, "Contents/Info.plist"), cursorPlist)
	writeFile(t, filepath.Join(app, "Contents/MacOS/Cursor"), "fixture")
	support := filepath.Join(s.home, "Library/Application Support/Cursor")
	writeFile(t, filepath.Join(support, "User/workspaceStorage/chat.vscdb"), strings.Repeat("c", 4096))
	return s, app, support
}

func TestUninstallScanOffersTheAppsOwnPersonalConfigAsReview(t *testing.T) {
	s, app, support := cursorFixture(t)
	if reason := s.CleanupReason(support); reason == "" {
		t.Fatal("outside an uninstall the installed app's data stays protected")
	}
	var found map[string]any
	for _, sub := range s.scanInstalledApps(filepath.Dir(app))[0]["subpaths"].([]map[string]any) {
		if sub["path"] == s.relativize(support) {
			found = sub
		}
	}
	if found == nil || found["kind"] != "config" || found["tier"] != "review" || found["checked"] != false || !strings.Contains(found["reason"].(string), "Cursor") {
		t.Fatalf("the config row must be a review row: %v", found)
	}
}

func TestUninstallScanKeepsConfigProtectedWhenASecondCopyStays(t *testing.T) {
	s, app, support := cursorFixture(t)
	writeFile(t, filepath.Join(s.home, "Applications/Cursor Nightly.app/Contents/Info.plist"), cursorPlist)
	for _, row := range s.scanInstalledApps(filepath.Dir(app)) {
		if row["name"] != "Cursor" {
			continue
		}
		for _, sub := range row["subpaths"].([]map[string]any) {
			if sub["path"] == s.relativize(support) && sub["tier"] != "protected" {
				t.Fatalf("data shared with the copy that stays must remain protected: %v", sub)
			}
		}
	}
}

func TestTrashUninstallMovesPersonalConfigOnlyWithItsAppBundle(t *testing.T) {
	s, app, support := cursorFixture(t)
	f := newFakeLaunchd(s)
	// Without the bundle in the request the data stays.
	outcome, _ := trashUninstallSync(s, []string{support}, nil)
	if len(outcome.Moved) != 0 || len(outcome.Skipped) != 1 {
		t.Fatalf("personal data moved without its app: %+v", outcome)
	}
	if _, err := os.Stat(support); err != nil {
		t.Fatal("the folder disappeared")
	}
	// With the bundle both go, and a restore brings the data back.
	outcome, _ = trashUninstallSync(s, []string{support, app}, nil)
	if len(outcome.Moved) != 2 || len(outcome.Skipped)+len(outcome.Failed) != 0 {
		t.Fatalf("moved %d, skipped %v, failed %v, log %v", len(outcome.Moved), outcome.Skipped, outcome.Failed, f.log)
	}
	if _, err := os.Stat(support); !os.IsNotExist(err) {
		t.Fatal("the personal folder is still in place")
	}
	var ids []string
	for _, moved := range outcome.Moved {
		ids = append(ids, moved.Original)
	}
	if restored := s.RestoreTrash(ids); len(restored.Failed) != 0 || len(restored.Restored) != 2 {
		t.Fatalf("restore refused: %+v", restored)
	}
	if _, err := os.Stat(support); err != nil {
		t.Fatal("restore did not bring the folder back")
	}
}

func TestTrashUninstallMovesLeftoverPersonalConfigOfAnAppThatIsAlreadyGone(t *testing.T) {
	s, app, support := cursorFixture(t)
	newFakeLaunchd(s)
	if err := os.RemoveAll(app); err != nil {
		t.Fatal(err)
	}
	if outcome, _ := trashUninstallSync(s, []string{support}, nil); len(outcome.Moved) != 1 {
		t.Fatalf("leftover data of a removed app: %+v", outcome)
	}
}

func TestUninstallReceiptDoesNotWaitForLaunchServices(t *testing.T) {
	s, _, app, _ := uninstallFixture(t)
	release := make(chan struct{})
	var started atomic.Bool
	inner := s.optimizeRun
	s.optimizeRun = func(ctx context.Context, stdin, name string, args ...string) (string, error) {
		if strings.Contains(name, "lsregister") && len(args) > 0 && args[0] == "-gc" {
			started.Store(true)
			<-release
		}
		return inner(ctx, stdin, name, args...)
	}
	done := make(chan struct{})
	go func() {
		s.TrashUninstall([]string{app}, nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("the receipt waited for lsregister -gc")
	}
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for !started.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !started.Load() {
		t.Fatal("lsregister -gc never ran")
	}
}

func TestCodexAppFoldersBelongToTheAppAloneWhileDotCodexAlsoBelongsToTheCLI(t *testing.T) {
	s, apps := ownerScanner(t)
	support := filepath.Join(s.home, "Library/Application Support/com.openai.codex/state.db")
	caches := filepath.Join(s.home, "Library/Caches/com.openai.codex/run.json")
	dot := filepath.Join(s.home, ".codex/sessions/s.jsonl")
	for _, path := range []string{support, caches, dot} {
		writeFile(t, path, "chat")
	}
	writeFile(t, filepath.Join(s.home, ".local/bin/codex"), "cli")
	// The CLI alone keeps ~/.codex but not the app's folders.
	if s.cleanupReason(dot, true) == "" {
		t.Error(".codex stays protected while the CLI is installed")
	}
	for _, path := range []string{support, caches} {
		if reason := s.cleanupReason(path, true); reason != "" {
			t.Errorf("the CLI does not own the app's folders: %s: %s", path, reason)
		}
	}
	// ChatGPT.app that carries the Codex bundle id is the app: it owns all three.
	writeFile(t, filepath.Join(apps, "ChatGPT.app/Contents/Info.plist"), `<plist><dict><key>CFBundleIdentifier</key><string>com.openai.codex</string></dict></plist>`)
	writeFile(t, filepath.Join(apps, "ChatGPT.app/Contents/MacOS/ChatGPT"), "fixture")
	for _, path := range []string{support, caches, dot} {
		if s.cleanupReason(path, true) == "" {
			t.Errorf("the installed app keeps %s", path)
		}
	}
	// Uninstalling that app turns its own folder into a review row, but only with the bundle in the request.
	var found map[string]any
	for _, sub := range s.scanInstalledApps(apps)[0]["subpaths"].([]map[string]any) {
		if sub["path"] == s.relativize(filepath.Dir(support)) {
			found = sub
		}
	}
	if found == nil || found["tier"] != "review" || found["checked"] != false {
		t.Fatalf("the app's own support folder must be a review row: %v", found)
	}
	newFakeLaunchd(s)
	if outcome, _ := trashUninstallSync(s, []string{filepath.Dir(support)}, nil); len(outcome.Moved) != 0 {
		t.Fatalf("moved without the app bundle: %+v", outcome)
	}
	if outcome, _ := trashUninstallSync(s, []string{filepath.Dir(support), filepath.Join(apps, "ChatGPT.app")}, nil); len(outcome.Moved) != 2 {
		t.Fatalf("app and folder should move together: %+v", outcome)
	}
}

func TestDiagnosticLogKeepRulesStopWhenTheToolIsGone(t *testing.T) {
	s, _ := ownerScanner(t)
	root := filepath.Join(s.home, ".copilot/logs")
	latest, recent := filepath.Join(root, "process-new.log"), filepath.Join(root, "process-recent.log")
	writeFile(t, latest, "new")
	writeFile(t, recent, "recent")
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(recent, past, past); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{latest, recent} {
		if reason := s.cleanupReason(path, true); reason != "" {
			t.Errorf("a gone tool's log is no longer kept: %s: %s", path, reason)
		}
	}
	var row map[string]any
	for _, r := range s.scanAICaches() {
		if r["tool"] == "GitHub Copilot CLI" && r["band"] == "诊断日志" {
			row = r
		}
	}
	if row == nil || row["tier"] != "review" || row["checked"] != false || !strings.Contains(row["reason"].(string), "GitHub Copilot CLI 已卸载") {
		t.Fatalf("logs of a gone tool are an unchecked review row: %v", row)
	}
	// Installed again: the newest and recent logs are kept as before.
	writeFile(t, filepath.Join(s.home, ".local/bin/copilot"), "cli")
	for _, path := range []string{latest, recent} {
		if s.cleanupReason(path, true) == "" {
			t.Errorf("the keep rules apply while the tool is installed: %s", path)
		}
	}
}
