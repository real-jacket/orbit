package scanner

import (
	"errors"
	"io/fs"
	"sort"
	"strings"
	"time"
)

// largeFile is one file the large file list shows.
type largeFile struct {
	path  string
	bytes int64
	kind  string
}

// scanLargeFiles lists the biggest files in the user's folders.
func (s *Scanner) scanLargeFiles() []map[string]any {
	roots := []string{"~/Downloads", "~/Documents", "~/Desktop", "~/Movies"}
	var candidates []largeFile
	for _, root := range roots {
		path := s.expand(root)
		if !s.collectLargeFiles(path, path, time.Now().Add(largeFileBudget), &candidates) {
			recordReadWarning(path, "大文件遍历达到扫描上限；只统计已访问路径")
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].bytes > candidates[j].bytes
	})
	if len(candidates) > 10 {
		candidates = candidates[:10]
	}
	items := []map[string]any{}
	for i, c := range candidates {
		items = append(items, map[string]any{
			"id":     "real_lf_" + itoa64(int64(i)),
			"name":   baseName(c.path),
			"path":   s.relativize(c.path),
			"sizeGb": gb02(c.bytes),
			"kind":   c.kind,
		})
	}
	return items
}

// gb02 converts bytes to gigabytes with a 0.02 GB floor, which is how the
// large file list reports sizes.
func gb02(bytes int64) float64 {
	value := round2(float64(bytes) / 1_000_000_000)
	if value < 0.02 {
		return 0.02
	}
	return value
}

// largeFileBudget is how long the large file walk may spend in one folder.
const largeFileBudget = 20 * time.Second

// collectLargeFiles walks dir looking for regular files bigger than 15 MB, and
// does not descend into app bundles or hidden directories. It reports false when
// it stopped at the deadline; the caller records the single warning for root.
func (s *Scanner) collectLargeFiles(root, dir string, deadline time.Time, out *[]largeFile) bool {
	list := listDirItems
	if dir == root {
		list = listDirGuarded
	}
	items, err := list(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		recordReadError(dir, err)
	}
	for _, item := range items {
		if time.Now().After(deadline) {
			return false
		}
		if isHidden(item.name) {
			continue
		}
		path := dir + "/" + item.name
		switch {
		case item.kind == itemDir:
			if !isPackage(item.name) && !s.collectLargeFiles(root, path, deadline, out) {
				return false
			}
		case item.kind == itemFile && item.bytes > 15_000_000:
			*out = append(*out, largeFile{path: path, bytes: item.bytes, kind: largeFileKind(item.name)})
		}
	}
	return true
}

// largeFileKind names the file type the large file list shows.
func largeFileKind(name string) string {
	switch ext := lowerExt(name); ext {
	case "dmg", "pkg", "xip", "iso", "zip":
		return "安装归档"
	case "mp4", "mov", "mkv":
		return "视频媒体"
	}
	return "大体积文件"
}

// isPackage reports whether name is an app bundle or another directory the
// Finder shows as a single item.
func isPackage(name string) bool {
	switch lowerExt(name) {
	case "app", "framework", "bundle", "plugin", "kext", "pkg":
		return true
	}
	return false
}

// lowerExt is the file extension in lower case, without the dot.
func lowerExt(name string) string {
	if dot := strings.LastIndex(name, "."); dot >= 0 {
		return strings.ToLower(name[dot+1:])
	}
	return ""
}
