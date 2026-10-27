package scanner

import (
	"os"
	"sort"
	"strings"
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
	scanned := 0
	for _, root := range roots {
		s.collectLargeFiles(s.expand(root), 1800, &scanned, &candidates)
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

// collectLargeFiles walks dir looking for regular files bigger than 15 MB, and
// does not descend into app bundles or hidden directories.
func (s *Scanner) collectLargeFiles(dir string, maxFiles int, scanned *int, out *[]largeFile) {
	entries := readDir(dir)
	for _, entry := range entries {
		if *scanned > maxFiles {
			return
		}
		*scanned++
		name := entry.name
		if isHidden(name) {
			continue
		}
		path := dir + "/" + name
		if entry.isDir {
			if isPackage(name) {
				continue
			}
			s.collectLargeFiles(path, maxFiles, scanned, out)
			continue
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		size := allocatedSize(info)
		if size <= 15_000_000 {
			continue
		}
		*out = append(*out, largeFile{path: path, bytes: size, kind: largeFileKind(name)})
	}
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
