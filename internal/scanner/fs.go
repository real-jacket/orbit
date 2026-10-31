package scanner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mymoles/mole-studio/internal/fsread"
)

// fileEntry is one entry of a directory listing.
type fileEntry struct {
	name  string
	isDir bool
}

var directoryReads = struct {
	sync.Mutex
	failed map[string]string
}{failed: map[string]string{}}

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
	directoryReads.Lock()
	delete(directoryReads.failed, dir)
	if err != nil && !os.IsNotExist(err) {
		directoryReads.failed[dir] = err.Error()
		if errors.Is(err, context.DeadlineExceeded) {
			directoryReads.failed[dir] = "目录读取超时，可能正在等待 macOS 授权；未计入本次扫描"
		}
	}
	directoryReads.Unlock()
	return entries, err
}

func recordReadWarning(path, reason string) {
	directoryReads.Lock()
	directoryReads.failed[path] = reason
	directoryReads.Unlock()
}

func scanReadWarnings(home string) []map[string]any {
	directoryReads.Lock()
	defer directoryReads.Unlock()
	var paths []string
	for path := range directoryReads.failed {
		if path == home || strings.HasPrefix(path, home+string(os.PathSeparator)) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	warnings := make([]map[string]any, 0, len(paths))
	for _, path := range paths {
		warnings = append(warnings, map[string]any{"path": path, "reason": directoryReads.failed[path]})
	}
	return warnings
}

// walkFiles calls visit for every regular file under dir, up to maxFiles
// entries, and reports each file's size and modification time.
func walkFiles(dir string, maxFiles int, scanned *int, visit func(file string, bytes, modified int64)) {
	entries := readDir(dir)
	for _, entry := range entries {
		if maxFiles > 0 && *scanned > maxFiles {
			return
		}
		*scanned++
		path := dir + "/" + entry.name
		if entry.isDir {
			walkFiles(path, maxFiles, scanned, func(file string, bytes, modified int64) {
				if !isHidden(baseName(file)) {
					visit(file, bytes, modified)
				}
			})
			continue
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		visit(path, allocatedSize(info), info.ModTime().Unix())
	}
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
