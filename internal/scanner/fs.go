package scanner

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mymoles/mole-studio/internal/fsread"
)

// fileEntry is one entry of a directory listing.
type fileEntry struct {
	name  string
	isDir bool
}

// Read warnings the page shows. kind tells the page what can be done about it.
const (
	warnPermission = "permission" // the filesystem refused a directory read
	warnSystem     = "system"     // macOS keeps third-party apps out, granted or not
	warnLimit      = "limit"      // a traversal cap was hit; the size is a lower bound
	warnTimeout    = "timeout"    // the filesystem did not answer before the deadline
	warnConfig     = "config"     // a saved scan scope could not be read or validated
	warnError      = "error"
)

// directoryReads holds the warnings of the running scan: a reason and a kind per
// path. A path without a kind (written by hand) reads as an error.
var directoryReads = struct {
	sync.Mutex
	failed map[string]string
	kinds  map[string]string
}{failed: map[string]string{}, kinds: map[string]string{}}

func readDir(dir string) []fileEntry {
	entries, err := readDirEntries(dir)
	if err != nil {
		return nil
	}
	out := make([]fileEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, fileEntry{name: entry.Name(), isDir: entry.IsDir()})
	}
	return out
}

func readDirEntries(dir string) ([]os.DirEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	entries, err := fsread.ReadDir(ctx, dir)
	if err != nil && !os.IsNotExist(err) {
		recordReadError(dir, err)
		return entries, err
	}
	directoryReads.Lock()
	// Listing the directory again cannot complete a previously capped traversal
	// or cache discovery. Keep that limit until the next full scan resets it;
	// successful reads may still clear a recovered read failure.
	if err != nil || directoryReads.kinds[dir] != warnLimit {
		delete(directoryReads.failed, dir)
		delete(directoryReads.kinds, dir)
	}
	directoryReads.Unlock()
	return entries, err
}

// recordReadWarning notes that path was only partly measured because of a cap or
// timeout the scan imposed on itself.
func recordReadWarning(path, reason string) {
	recordReadWarningKind(path, reason, warnLimit)
}

func recordReadWarningKind(path, reason, kind string) {
	directoryReads.Lock()
	directoryReads.failed[path] = reason
	directoryReads.kinds[path] = kind
	directoryReads.Unlock()
}

// recordReadError notes that path could not be read.
func recordReadError(path string, err error) {
	kind, reason := classifyReadError(path, err)
	recordReadWarningKind(path, reason, kind)
}

// clearReadWarnings forgets the warnings overlapping home before a new scan.
func clearReadWarnings(home string) {
	clearReadWarningsForRoots([]string{home})
}

// Configured project roots can live on another volume. Clear failed parents
// too, since they can prevent discovery of a selected root.
func clearReadWarningsForRoots(roots []string) {
	directoryReads.Lock()
	defer directoryReads.Unlock()
	for path := range directoryReads.failed {
		for _, root := range roots {
			if root != "" && (withinPath(path, root) || withinPath(root, path)) {
				delete(directoryReads.failed, path)
				delete(directoryReads.kinds, path)
				break
			}
		}
	}
}

// classifyReadError sorts a read failure by what the user can do about it.
func classifyReadError(path string, err error) (kind, reason string) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return warnTimeout, "目录读取超时，未计入本次扫描；请重新扫描或检查存储连接"
	case errors.Is(err, syscall.EPERM) && isSystemProtected(path):
		return warnSystem, "macOS 系统保护的目录，授予完全磁盘访问权限后也无法读取；未计入本次扫描"
	case errors.Is(err, fs.ErrPermission):
		return warnPermission, "目录读取被拒绝，未计入本次扫描；请检查完全磁盘访问权限和该目录的访问权限后重新扫描"
	}
	return warnError, err.Error()
}

// isSystemProtected reports whether path is one of the Apple caches macOS keeps
// behind SIP, data vaults or TCC containers (com.apple.containermanagerd,
// com.apple.homed, familycircled, ...). shortcut: matched by name under
// ~/Library/Caches only; extend when another protected location shows up.
func isSystemProtected(path string) bool {
	_, rest, found := strings.Cut(path, "/Library/Caches/")
	if !found {
		return false
	}
	name, _, _ := strings.Cut(rest, "/")
	// Safari's caches open once Full Disk Access is granted, so a denial there is a missing grant.
	if strings.HasPrefix(name, "com.apple.Safari") {
		return false
	}
	return strings.HasPrefix(name, "com.apple.") || name == "familycircled"
}

// scanReadWarnings lists the scan's warnings under home. A warning on a path
// inside another path with the same kind is dropped: the user sees one entry per
// measured root, not one per folder the walk went through.
func scanReadWarnings(home string) []map[string]any {
	return scanReadWarningsForRoots([]string{home})
}

// scanReadWarningsForRoots includes failed descendants and failed parents of
// the directories actually being scanned. Configured projects can live outside
// the home directory; a parent read refusal can prevent discovering a root.
func scanReadWarningsForRoots(roots []string) []map[string]any {
	var scope []string
	for _, root := range roots {
		if root != "" {
			scope = append(scope, filepath.Clean(root))
		}
	}
	directoryReads.Lock()
	defer directoryReads.Unlock()
	var paths []string
	for path := range directoryReads.failed {
		cleanPath := filepath.Clean(path)
		for _, root := range scope {
			if withinPath(cleanPath, root) || withinPath(root, cleanPath) {
				paths = append(paths, path)
				break
			}
		}
	}
	sort.Strings(paths)
	kindOf := func(path string) string {
		if kind := directoryReads.kinds[path]; kind != "" {
			return kind
		}
		return warnError
	}
	warnings := make([]map[string]any, 0, len(paths))
	var outer []string // kept paths in sorted order, so an ancestor precedes its descendants
	for _, path := range paths {
		covered := false
		for _, parent := range outer {
			if kindOf(parent) == kindOf(path) && strings.HasPrefix(path, parent+string(os.PathSeparator)) {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		outer = append(outer, path)
		warnings = append(warnings, map[string]any{"path": path, "reason": directoryReads.failed[path], "kind": kindOf(path)})
	}
	return warnings
}

var bundleExtensions = map[string]bool{
	"app": true, "pkg": true, "mpkg": true, "framework": true, "bundle": true, "xcarchive": true,
	"plugin": true, "appex": true, "xpc": true, "kext": true, "prefpane": true, "saver": true,
	"component": true, "dsym": true, "xcodeproj": true, "xcworkspace": true, "photoslibrary": true,
}

// isBundleDir reports whether a directory name is a macOS bundle or package.
func isBundleDir(name string) bool {
	return bundleExtensions[strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))]
}

// hasExtension reports whether file's extension is in extensions.
func hasExtension(file string, extensions map[string]struct{}) bool {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(file), "."))
	_, ok := extensions[ext]
	return ok
}

// baseName is the last element of path.
func baseName(path string) string {
	return filepath.Base(path)
}

// dirName is the directory path is in.
func dirName(path string) string {
	return filepath.Dir(path)
}

// isHidden reports whether name starts with a dot.
func isHidden(name string) bool {
	return strings.HasPrefix(name, ".")
}

// modifiedTime is path's modification time in seconds since the epoch, or zero
// when it cannot be read.
func modifiedTime(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.ModTime().Unix()
}
