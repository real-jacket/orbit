package scanner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// The main window, tray, and late file moves can all record receipts at once.
var trashManifestMu sync.Mutex
var trashOperationMu sync.Mutex

// TrashEntry records one trashed item: where it came from and where it is now.
type TrashEntry struct {
	Original string `json:"original"`
	Trashed  string `json:"trashed"`
	At       int64  `json:"at"`
	Device   uint64 `json:"device,omitempty"`
	Inode    uint64 `json:"inode,omitempty"`
	// Unloaded lists the launchd jobs stopped just before this plist moved, so a
	// restore can start them again.
	Unloaded []LaunchUnload `json:"unloaded,omitempty"`
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
	Requested    int
	Moved        []TrashEntry
	Skipped      []TrashSkip
	Failed       []TrashFailure
	RestoreError string
}

// RestoreOutcome is the receipt for a restore request.
type RestoreOutcome struct {
	Requested int
	Restored  []string
	Failed    []TrashFailure
	// Restarted and RestartFailed report the background services a restored
	// LaunchAgent plist had before the uninstall stopped them.
	Restarted     []string
	RestartFailed []ServiceFailure
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

// Unreadable receipts must not be silently replaced or guessed from filenames.
func readTrashManifest() (trashManifest, error) {
	manifest := trashManifest{Version: trashManifestVersion}
	data, err := os.ReadFile(trashManifestPath())
	if os.IsNotExist(err) {
		return manifest, nil
	}
	if err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, fmt.Errorf("还原清单损坏：%w", err)
	}
	if manifest.Version != trashManifestVersion {
		return manifest, fmt.Errorf("不支持的还原清单版本 %d", manifest.Version)
	}
	return manifest, nil
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
	trashManifestMu.Lock()
	defer trashManifestMu.Unlock()
	manifest, err := readTrashManifest()
	if err != nil {
		return err
	}
	manifest.Entries = append(entries, manifest.Entries...)
	return writeTrashManifest(manifest)
}

// forgetTrash drops the receipts for originals that were restored.
func forgetTrash(restored []TrashEntry) error {
	trashManifestMu.Lock()
	defer trashManifestMu.Unlock()
	done := make(map[[2]string]struct{}, len(restored))
	for _, entry := range restored {
		done[[2]string{entry.Original, entry.Trashed}] = struct{}{}
	}
	manifest, err := readTrashManifest()
	if err != nil {
		return err
	}
	kept := manifest.Entries[:0]
	for _, entry := range manifest.Entries {
		if _, ok := done[[2]string{entry.Original, entry.Trashed}]; ok {
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
	return s.TrashPathsChecked(rawPaths, nil)
}

// FileIdentity is what a scan saw of a file: logical size and mtime in seconds.
type FileIdentity struct{ Size, Mtime int64 }

// TrashPathsChecked is TrashPaths, and a path with an identity is only moved when
// it is still the same regular file the scan listed. Mole's installer cleanup
// re-checks the same way before it deletes.
func (s *Scanner) TrashPathsChecked(rawPaths []string, identities map[string]FileIdentity) TrashOutcome {
	return s.trashPaths(rawPaths, identities, nil)
}

func (s *Scanner) trashPaths(rawPaths []string, identities map[string]FileIdentity, hooks *trashHooks) TrashOutcome {
	outcome := TrashOutcome{Requested: len(rawPaths)}
	if !trashOperationMu.TryLock() {
		for _, path := range rawPaths {
			outcome.Skipped = append(outcome.Skipped, TrashSkip{Path: s.expand(path), Reason: "已有文件操作等待系统结果，不能重复处理"})
		}
		return outcome
	}
	defer trashOperationMu.Unlock()
	if _, err := readTrashManifest(); err != nil {
		for _, path := range rawPaths {
			outcome.Failed = append(outcome.Failed, TrashFailure{Path: s.expand(path), Error: "无法读取还原清单，未移动：" + err.Error()})
		}
		return outcome
	}
	entries := make([]TrashEntry, 0, len(rawPaths))
	for _, raw := range rawPaths {
		path := raw
		if strings.TrimSpace(path) == "" {
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
		var relax func(string) bool
		if hooks != nil {
			relax = hooks.relax
		}
		if reason := s.cleanupReasonAs(path, true, relax); reason != "" {
			outcome.Skipped = append(outcome.Skipped, TrashSkip{Path: path, Reason: reason})
			continue
		}
		if !exists(path) {
			outcome.Failed = append(outcome.Failed, TrashFailure{Path: path, Error: "路径不存在"})
			continue
		}
		if want, ok := identities[raw]; ok {
			if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() || info.Size() != want.Size || info.ModTime().Unix() != want.Mtime {
				outcome.Skipped = append(outcome.Skipped, TrashSkip{Path: path, Reason: "文件在扫描之后有变化，已跳过；请重新扫描后再清理"})
				continue
			}
		}
		// Dispatch runs off the UI thread; a timeout cannot cancel rename(2).
		var unloaded []LaunchUnload
		if hooks != nil {
			unloaded = hooks.before(path)
		}
		trashed, err := moveToTrash(path)
		if err != nil {
			if hooks != nil {
				hooks.failed(path, unloaded)
			}
			outcome.Failed = append(outcome.Failed, trashFailure(path, err))
			continue
		}
		if hooks != nil {
			hooks.moved(path, trashed)
		}
		entry := TrashEntry{Original: path, Trashed: trashed, At: time.Now().Unix(), Unloaded: unloaded}
		if info, err := os.Lstat(trashed); err == nil {
			if stat, ok := info.Sys().(*syscall.Stat_t); ok {
				entry.Device, entry.Inode = uint64(stat.Dev), uint64(stat.Ino)
			}
		}
		if entry.Inode == 0 {
			outcome.RestoreError = "文件已移动，但废纸篓中的文件身份无法核验；请在访达检查"
		}
		outcome.Moved = append(outcome.Moved, entry)
		entries = append(entries, outcome.Moved[len(outcome.Moved)-1])
	}
	defer func() { logTrashOutcome("trash", outcome) }()
	if len(entries) > 0 {
		if err := recordTrash(entries); err != nil {
			// The files are already gone; recording the receipt is best effort,
			// but say so instead of silently losing the ability to restore.
			outcome.RestoreError = "写入还原清单失败：" + err.Error()
			outcome.Failed = append(outcome.Failed, TrashFailure{Path: trashManifestPath(), Error: outcome.RestoreError})
		}
	}
	return outcome
}

// RestoreTrash moves originals back out of the Trash using the manifest. Items
// that are no longer there, or that would overwrite something, are reported
// instead of guessed at.
func (s *Scanner) RestoreTrash(originals []string, expectedTrashed ...map[string]string) RestoreOutcome {
	outcome := RestoreOutcome{Requested: len(originals)}
	if !trashOperationMu.TryLock() {
		for _, path := range originals {
			outcome.Failed = append(outcome.Failed, TrashFailure{Path: s.expand(path), Error: "已有文件操作等待系统结果，不能同时还原"})
		}
		return outcome
	}
	defer trashOperationMu.Unlock()
	manifest, err := readTrashManifest()
	if err != nil {
		for _, path := range originals {
			outcome.Failed = append(outcome.Failed, TrashFailure{Path: s.expand(path), Error: err.Error()})
		}
		return outcome
	}
	trash, err := trashDir()
	if err != nil {
		for _, path := range originals {
			outcome.Failed = append(outcome.Failed, trashFailure(s.expand(path), err))
		}
		return outcome
	}
	byOriginal := make(map[string][]TrashEntry, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		byOriginal[entry.Original] = append(byOriginal[entry.Original], entry)
	}
	restored := make([]TrashEntry, 0, len(originals))
	for _, raw := range originals {
		original := filepath.Clean(s.expand(raw))
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if !filepath.IsAbs(original) || isTrashRoot(original, s.home) {
			outcome.Failed = append(outcome.Failed, TrashFailure{Path: raw, Error: "还原目标不是可核验的绝对路径"})
			continue
		}
		if _, protected := builtinCleanupProtection(original, s.home, ownerRule); protected {
			outcome.Failed = append(outcome.Failed, TrashFailure{Path: original, Error: "应用不会移动的核心路径不能通过还原清单写入；请在访达中检查"})
			continue
		}
		candidates := byOriginal[original]
		if len(expectedTrashed) > 0 {
			wanted := expectedTrashed[0][original]
			if wanted == "" {
				outcome.Failed = append(outcome.Failed, TrashFailure{Path: original, Error: "这条历史记录未保存具体废纸篓路径，不能区分版本；请在访达中检查"})
				continue
			}
			candidates = nil
			for _, entry := range byOriginal[original] {
				if entry.Trashed == wanted {
					candidates = append(candidates, entry)
				}
			}
		}
		if len(candidates) == 0 {
			outcome.Failed = append(outcome.Failed, TrashFailure{Path: original, Error: "没有匹配的还原清单；请在访达中检查废纸篓，不按同名文件猜测"})
			continue
		}
		if len(candidates) != 1 {
			outcome.Failed = append(outcome.Failed, TrashFailure{Path: original, Error: "同一原始路径有多份清单，无法判断这条记录对应哪份；请在访达中检查"})
			continue
		}
		entry := candidates[0]
		if !filepath.IsAbs(entry.Trashed) || filepath.Dir(filepath.Clean(entry.Trashed)) != filepath.Clean(trash) {
			outcome.Failed = append(outcome.Failed, TrashFailure{Path: original, Error: "还原清单的来源不在当前废纸篓中，保持原状"})
			continue
		}
		entry.Trashed = filepath.Clean(entry.Trashed)
		if !exists(entry.Trashed) {
			outcome.Failed = append(outcome.Failed, TrashFailure{Path: original, Error: "废纸篓中已找不到该项"})
			continue
		}
		info, err := os.Lstat(entry.Trashed)
		if err != nil {
			outcome.Failed = append(outcome.Failed, trashFailure(original, err))
			continue
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || entry.Inode == 0 || uint64(stat.Dev) != entry.Device || uint64(stat.Ino) != entry.Inode {
			outcome.Failed = append(outcome.Failed, TrashFailure{Path: original, Error: "废纸篓文件身份已变化或旧版清单未保存身份，不能确认这份数据；请在访达中检查"})
			continue
		}
		if err := moveBack(entry); err != nil {
			outcome.Failed = append(outcome.Failed, trashFailure(original, err))
			continue
		}
		outcome.Restored = append(outcome.Restored, original)
		restored = append(restored, entry)
		started, failed := s.restartServices(entry)
		outcome.Restarted = append(outcome.Restarted, started...)
		outcome.RestartFailed = append(outcome.RestartFailed, failed...)
	}
	if len(restored) > 0 {
		_ = forgetTrash(restored)
	}
	logTrashOutcome("restore", TrashOutcome{Requested: outcome.Requested, Failed: outcome.Failed})
	return outcome
}

// logTrashOutcome appends one JSON line to <state>/trash.log so a refusal can be
// diagnosed later without reproducing it in a debugger. It is best effort and
// caps the file so it cannot grow without bound.
func logTrashOutcome(action string, outcome TrashOutcome) {
	record := map[string]any{
		"at":           time.Now().Format(time.RFC3339),
		"action":       action,
		"requested":    outcome.Requested,
		"moved":        len(outcome.Moved),
		"skipped":      outcome.Skipped,
		"failed":       outcome.Failed,
		"restoreError": outcome.RestoreError,
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

// moveBack returns one trashed item to its original path without replacing it.
func moveBack(entry TrashEntry) error {
	for parent := filepath.Dir(entry.Original); ; parent = filepath.Dir(parent) {
		resolved, err := filepath.EvalSymlinks(parent)
		if os.IsNotExist(err) && parent != "/" {
			continue
		}
		if err != nil {
			return err
		}
		if resolved != parent && (resolved != "/private"+parent || !(withinPath(parent, "/var") || withinPath(parent, "/tmp"))) {
			return fmt.Errorf("还原目标的父目录经过符号链接，不能确认原路还原；请在访达中检查")
		}
		break
	}
	if dir := filepath.Dir(entry.Original); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return renameWithoutOverwrite(entry.Trashed, entry.Original)
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
			return "macOS 拒绝移动应用：需要在「系统设置 › 隐私与安全性 › App 管理」里允许 orbit。" +
				"本地签名版本每次重新打包后系统会当成新应用，需要重新授权。", "appManagement"
		}
		if needsFolderAccess(path) {
			return "权限不足：macOS 把「桌面 / 文稿 / 下载」当作受保护文件夹，需要在「系统设置 › 隐私与安全性 › 文件与文件夹」里允许 orbit 访问。", "filesAndFolders"
		}
		if needsAppDataAccess(path) {
			// An uninstalled app's container belongs to another app. macOS keeps
			// those under its App Data policy even after the owning app is gone,
			// so a "permission denied" here is not a broken file: it is a switch.
			return "这是其它应用留下的数据容器，macOS 需要「完全磁盘访问权限」（部分系统版本在「App 管理」）才能移动。" +
				"授权后重试即可，已经移入废纸篓的条目不受影响。", "fullDiskAccess"
		}
		if needsFullDiskAccess(path) {
			return "权限不足：该目录受 macOS 保护，需要在「系统设置 › 隐私与安全性 › 完全磁盘访问权限」里允许 orbit。", "fullDiskAccess"
		}
		return "权限不足，可能需要在系统设置里授予完全磁盘访问权限", "fullDiskAccess"
	case errors.Is(err, syscall.EXDEV):
		if isSystemVolume(path) {
			return "这是 macOS 系统卷上的文件（只读保护），不能移入废纸篓", ""
		}
		return "跨卷移动失败，请手动处理", ""
	case errors.Is(err, syscall.ENOSPC):
		return "磁盘空间不足", ""
	case errors.Is(err, os.ErrExist):
		return "还原目标已存在，原文件与废纸篓条目均保留；请在访达中处理同名文件后重试", ""
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
