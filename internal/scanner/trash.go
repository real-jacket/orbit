package scanner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// The app trashes files itself instead of shelling out to `mo clean`, so it has
// to remember where each item came from. That manifest is what makes the
// "还原" button a real operation rather than a UI state change, and it is also
// what lets the page report exactly what happened instead of assuming success.

// trashManifestVersion guards the on-disk shape of the manifest.
const trashManifestVersion = 1

// trashManifestLimit keeps the manifest bounded; older receipts stop being
// restorable rather than growing the file forever.
const trashManifestLimit = 1000

// TrashEntry records one trashed item: where it came from and where it is now.
type TrashEntry struct {
	Original string `json:"original"`
	Trashed  string `json:"trashed"`
	At       int64  `json:"at"`
}

// TrashFailure is one path the app could not move.
type TrashFailure struct {
	Path  string `json:"path"`
	Error string `json:"error"`
	// Pane names the System Settings pane that would fix this failure, empty
	// when the failure is not a permission problem.
	Pane string `json:"pane,omitempty"`
}

// TrashSkip is one path the app refused to move.
type TrashSkip struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// TrashOutcome is the receipt the page waits for before it claims success.
type TrashOutcome struct {
	Requested int
	Moved     []TrashEntry
	Skipped   []TrashSkip
	Failed    []TrashFailure
}

// RestoreOutcome is the receipt for a restore request.
type RestoreOutcome struct {
	Requested int
	Restored  []string
	Failed    []TrashFailure
}

// StateDir is where the app keeps its own state: window preferences and the
// trash manifest. MOLE_STUDIO_STATE_DIR overrides it, which is what the tests
// and the verification harness use.
func StateDir() string {
	if dir := os.Getenv("MOLE_STUDIO_STATE_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "mole-studio")
	}
	return filepath.Join(home, "Library", "Application Support", "orbit")
}

// trashManifestPath is where the receipts live.
func trashManifestPath() string {
	return filepath.Join(StateDir(), "trash-manifest.json")
}

// trashManifest is the on-disk receipt list, newest first.
type trashManifest struct {
	Version int          `json:"version"`
	Entries []TrashEntry `json:"entries"`
}

// readTrashManifest loads the receipts, and treats a corrupt file as empty: a
// receipt we cannot read must not stop the app from cleaning up.
func readTrashManifest() trashManifest {
	manifest := trashManifest{Version: trashManifestVersion}
	data, err := os.ReadFile(trashManifestPath())
	if err != nil {
		return manifest
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return trashManifest{Version: trashManifestVersion}
	}
	if manifest.Version != trashManifestVersion {
		return trashManifest{Version: trashManifestVersion}
	}
	return manifest
}

// writeTrashManifest saves the receipts atomically, so a crash cannot leave a
// half-written manifest behind.
func writeTrashManifest(manifest trashManifest) error {
	manifest.Version = trashManifestVersion
	if len(manifest.Entries) > trashManifestLimit {
		manifest.Entries = manifest.Entries[:trashManifestLimit]
	}
	if err := os.MkdirAll(filepath.Dir(trashManifestPath()), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	temporary := trashManifestPath() + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(temporary, trashManifestPath())
}

// recordTrash adds entries to the manifest, newest first.
func recordTrash(entries []TrashEntry) error {
	manifest := readTrashManifest()
	manifest.Entries = append(entries, manifest.Entries...)
	return writeTrashManifest(manifest)
}

// forgetTrash drops the receipts for originals that were restored.
func forgetTrash(originals []string) error {
	restored := make(map[string]struct{}, len(originals))
	for _, original := range originals {
		restored[original] = struct{}{}
	}
	manifest := readTrashManifest()
	kept := manifest.Entries[:0]
	for _, entry := range manifest.Entries {
		if _, ok := restored[entry.Original]; ok {
			continue
		}
		kept = append(kept, entry)
	}
	manifest.Entries = kept
	return writeTrashManifest(manifest)
}

// TrashPaths moves paths into the Trash and reports what actually happened.
// Whitelisted and protected paths are refused rather than deleted: the app
// enforces the same protection `mo` does, because it does the deleting itself.
func (s *Scanner) TrashPaths(rawPaths []string) TrashOutcome {
	outcome := TrashOutcome{Requested: len(rawPaths)}
	policy := s.whitelist()
	entries := make([]TrashEntry, 0, len(rawPaths))
	for _, raw := range rawPaths {
		path := strings.TrimSpace(raw)
		if path == "" {
			continue
		}
		path = s.expand(path)
		if !filepath.IsAbs(path) {
			outcome.Skipped = append(outcome.Skipped, TrashSkip{Path: raw, Reason: "不是绝对路径"})
			continue
		}
		path = filepath.Clean(path)
		if isTrashRoot(path, s.home) {
			outcome.Skipped = append(outcome.Skipped, TrashSkip{Path: path, Reason: "拒绝整根移动废纸篓与家目录"})
			continue
		}
		if policy.Protects(path) {
			outcome.Skipped = append(outcome.Skipped, TrashSkip{Path: path, Reason: "白名单保护"})
			continue
		}
		if !exists(path) {
			outcome.Failed = append(outcome.Failed, TrashFailure{Path: path, Error: "路径不存在"})
			continue
		}
		trashed, err, timedOut := moveWithWatchdog(path)
		if timedOut {
			// A blocked rename is what an impossible-to-answer TCC prompt looks
			// like: the syscall never returns, so without this the page waits for
			// a receipt that never comes and the user sees "nothing happened".
			outcome.Failed = append(outcome.Failed, TrashFailure{
				Path: path,
				Error: "系统没有返回结果：macOS 很可能正在等一个授权弹窗。" +
					"请在「系统设置 › 隐私与安全性」里给 MoleStudio 打开「文件与文件夹」或「完全磁盘访问权限」后重试。",
				Pane: "filesAndFolders",
			})
			continue
		}
		if err != nil {
			outcome.Failed = append(outcome.Failed, trashFailure(path, err))
			continue
		}
		outcome.Moved = append(outcome.Moved, TrashEntry{Original: path, Trashed: trashed, At: time.Now().Unix()})
		entries = append(entries, outcome.Moved[len(outcome.Moved)-1])
	}
	defer func() { logTrashOutcome("trash", outcome) }()
	if len(entries) > 0 {
		if err := recordTrash(entries); err != nil {
			// The files are already gone; recording the receipt is best effort,
			// but say so instead of silently losing the ability to restore.
			outcome.Failed = append(outcome.Failed, TrashFailure{Path: trashManifestPath(), Error: "写入还原清单失败：" + err.Error()})
		}
	}
	return outcome
}

// RestoreTrash moves originals back out of the Trash using the manifest. Items
// that are no longer there, or that would overwrite something, are reported
// instead of guessed at.
func (s *Scanner) RestoreTrash(originals []string) RestoreOutcome {
	outcome := RestoreOutcome{Requested: len(originals)}
	manifest := readTrashManifest()
	byOriginal := make(map[string]TrashEntry, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		if _, ok := byOriginal[entry.Original]; !ok {
			byOriginal[entry.Original] = entry
		}
	}
	restored := make([]string, 0, len(originals))
	for _, raw := range originals {
		original := filepath.Clean(s.expand(strings.TrimSpace(raw)))
		if original == "" || original == "." {
			continue
		}
		entry, ok := byOriginal[original]
		if !ok {
			entry = TrashEntry{Original: original, Trashed: filepath.Join(s.home, ".Trash", filepath.Base(original))}
		}
		if !exists(entry.Trashed) {
			outcome.Failed = append(outcome.Failed, TrashFailure{Path: original, Error: "废纸篓中已找不到该项"})
			continue
		}
		if err := moveBack(entry); err != nil {
			outcome.Failed = append(outcome.Failed, trashFailure(original, err))
			continue
		}
		outcome.Restored = append(outcome.Restored, original)
		restored = append(restored, original)
	}
	if len(restored) > 0 {
		_ = forgetTrash(restored)
	}
	logTrashOutcome("restore", TrashOutcome{Requested: outcome.Requested, Failed: outcome.Failed})
	return outcome
}

// moveWatchdog bounds how long the app waits for one move. A TCC prompt that
// can never be answered turns rename(2) into a blocking call; without a bound
// the receipt is never produced and the UI looks frozen rather than refused.
const moveWatchdog = 8 * time.Second

// moveWithWatchdog moves one path to the Trash and reports whether the kernel
// left the call hanging instead of refusing it outright.
func moveWithWatchdog(path string) (string, error, bool) {
	type result struct {
		trashed string
		err     error
	}
	done := make(chan result, 1)
	go func() {
		trashed, err := moveToTrash(path)
		done <- result{trashed: trashed, err: err}
	}()
	select {
	case out := <-done:
		return out.trashed, out.err, false
	case <-time.After(moveWatchdog):
		// The move may still finish later; keep the receipt so restore works.
		go func() {
			out := <-done
			if out.err != nil {
				return
			}
			_ = recordTrash([]TrashEntry{{Original: path, Trashed: out.trashed, At: time.Now().Unix()}})
			logTrashOutcome("trash-late", TrashOutcome{
				Requested: 1,
				Moved:     []TrashEntry{{Original: path, Trashed: out.trashed, At: time.Now().Unix()}},
			})
		}()
		return "", nil, true
	}
}

// logTrashOutcome appends one JSON line to <state>/trash.log so a refusal can be
// diagnosed later without reproducing it in a debugger. It is best effort and
// caps the file so it cannot grow without bound.
func logTrashOutcome(action string, outcome TrashOutcome) {
	record := map[string]any{
		"at":        time.Now().Format(time.RFC3339),
		"action":    action,
		"requested": outcome.Requested,
		"moved":     len(outcome.Moved),
		"skipped":   outcome.Skipped,
		"failed":    outcome.Failed,
	}
	line, err := json.Marshal(record)
	if err != nil {
		return
	}
	path := filepath.Join(StateDir(), "trash.log")
	if info, err := os.Stat(path); err == nil && info.Size() > 256*1024 {
		_ = os.Remove(path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.Write(append(line, '\n'))
}

// moveBack returns one trashed item to its original path, renaming it if
// something already occupies that path.
func moveBack(entry TrashEntry) error {
	target := entry.Original
	if exists(target) {
		target = uniqueSibling(target)
	}
	if dir := filepath.Dir(target); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.Rename(entry.Trashed, target)
}

// uniqueSibling returns path with " 2", " 3", … appended to the stem until it
// is free, which is how Finder names a second copy.
func uniqueSibling(path string) string {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	extension := filepath.Ext(base)
	stem := strings.TrimSuffix(base, extension)
	for index := 2; ; index++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s %d%s", stem, index, extension))
		if !exists(candidate) {
			return candidate
		}
	}
}

// isTrashRoot reports whether path is the Trash itself, the home directory, or
// one of the roots nothing should ever be moved out of.
func isTrashRoot(path, home string) bool {
	switch path {
	case "/", home, filepath.Join(home, ".Trash"), filepath.Join(home, "Library"):
		return true
	}
	return false
}

// humanizeTrashError turns the errors users actually hit into advice.
// trashFailure turns one failed move into the message the user reads, and, for
// permission problems, the System Settings pane that can grant it.
func trashFailure(path string, err error) TrashFailure {
	message, pane := humanizeTrashError(path, err)
	return TrashFailure{Path: path, Error: message, Pane: pane}
}

// humanizeTrashError explains a failed move. macOS gates two different
// permissions behind two different panes, so a bare "permission denied" would
// send the user to the wrong switch: moving an app bundle needs App Management,
// while reading inside another app's Library folder needs Full Disk Access.
func humanizeTrashError(path string, err error) (string, string) {
	switch {
	case errors.Is(err, os.ErrPermission), errors.Is(err, syscall.EPERM), errors.Is(err, syscall.EACCES):
		if isAppBundle(path) {
			return "macOS 拒绝移动应用：需要在「系统设置 › 隐私与安全性 › App 管理」里允许 MoleStudio。" +
				"本地签名版本每次重新打包后系统会当成新应用，需要重新授权。", "appManagement"
		}
		if needsFolderAccess(path) {
			return "权限不足：macOS 把「桌面 / 文稿 / 下载」当作受保护文件夹，需要在「系统设置 › 隐私与安全性 › 文件与文件夹」里允许 MoleStudio 访问。", "filesAndFolders"
		}
		if needsAppDataAccess(path) {
			// An uninstalled app's container belongs to another app. macOS keeps
			// those under its App Data policy even after the owning app is gone,
			// so a "permission denied" here is not a broken file: it is a switch.
			return "这是其它应用留下的数据容器，macOS 需要「完全磁盘访问权限」（部分系统版本在「App 管理」）才能移动。" +
				"授权后重试即可，已经移入废纸篓的条目不受影响。", "fullDiskAccess"
		}
		if needsFullDiskAccess(path) {
			return "权限不足：该目录受 macOS 保护，需要在「系统设置 › 隐私与安全性 › 完全磁盘访问权限」里允许 MoleStudio。", "fullDiskAccess"
		}
		return "权限不足，可能需要在系统设置里授予完全磁盘访问权限", "fullDiskAccess"
	case errors.Is(err, syscall.EXDEV):
		if isSystemVolume(path) {
			return "这是 macOS 系统卷上的文件（只读保护），不能移入废纸篓", ""
		}
		return "跨卷移动失败，请手动处理", ""
	case errors.Is(err, syscall.ENOSPC):
		return "磁盘空间不足", ""
	case errors.Is(err, os.ErrNotExist):
		return "路径不存在", ""
	}
	return err.Error(), ""
}

// isSystemVolume reports whether path lives on the read-only system volume,
// where a move to the Trash can only ever fail with EXDEV.
func isSystemVolume(path string) bool {
	cleaned := filepath.Clean(path)
	for _, prefix := range []string{"/System/", "/usr/", "/bin/", "/sbin/", "/Library/Apple/"} {
		if strings.HasPrefix(cleaned, prefix) {
			return true
		}
	}
	return false
}

// isAppBundle reports whether path is an application bundle, the kind of item
// macOS guards with the App Management permission.
func isAppBundle(path string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimRight(path, "/")), ".app")
}

// needsFolderAccess reports whether path lives in one of the folders macOS
// guards separately, the ones the system prompt calls "Files and Folders": the
// user's Desktop, Documents and Downloads. Deleting the user's own files needs
// this permission, not Full Disk Access.
func needsFolderAccess(path string) bool {
	cleaned := filepath.Clean(path)
	for _, folder := range []string{"Desktop", "Documents", "Downloads"} {
		if strings.Contains(cleaned, "/"+folder+"/") || strings.HasSuffix(cleaned, "/"+folder) {
			return true
		}
	}
	return false
}

// needsAppDataAccess reports whether path is another app's data container. macOS
// keeps ~/Library/Containers and ~/Library/Group Containers under the App Data
// policy, and that policy outlives the app that owned the directory, so leftover
// cleanup from the uninstall module always runs into it.
func needsAppDataAccess(path string) bool {
	cleaned := filepath.Clean(path)
	return strings.Contains(cleaned, "/Library/Containers/") ||
		strings.Contains(cleaned, "/Library/Group Containers/") ||
		strings.HasSuffix(cleaned, "/Library/Containers") ||
		strings.HasSuffix(cleaned, "/Library/Group Containers")
}

// needsFullDiskAccess reports whether reading path is gated by Full Disk Access:
// the Library folder of the user or of the system, and the private trees macOS
// keeps under /private.
func needsFullDiskAccess(path string) bool {
	cleaned := filepath.Clean(path)
	return strings.Contains(cleaned, "/Library/") ||
		strings.HasSuffix(cleaned, "/Library") ||
		strings.HasPrefix(cleaned, "/Library") ||
		strings.HasPrefix(cleaned, "/private/")
}
