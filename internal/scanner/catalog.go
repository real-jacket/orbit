package scanner

import (
	"sort"
)

// CatalogSpec describes one row of the clean list: which directories make it
// up, how safe it is to remove them, and what the user loses.
type CatalogSpec struct {
	ID       string
	Category string
	Name     string
	Roots    []string
	Tier     string
	Impact   string
	Rebuild  string
}

// catalog measures the directories of spec that exist, and returns a clean
// list row. It returns nil when none of them exist or all of them are empty,
// which is what the Swift app did.
func (s *Scanner) catalog(spec CatalogSpec) map[string]any {
	var subpaths []map[string]any
	var total int64
	for _, root := range spec.Roots {
		path := s.expand(root)
		if !exists(path) {
			continue
		}
		size := s.fastDirectorySize(path, 2500)
		if size.bytes <= 0 {
			continue
		}
		total += size.bytes
		subpaths = append(subpaths, map[string]any{
			"path":    s.relativize(path),
			"size":    formatBytes(size.bytes),
			"sizeGb":  gb(size.bytes),
			"checked": spec.Tier != "protected",
		})
	}
	if len(subpaths) == 0 {
		return nil
	}
	return map[string]any{
		"id":          spec.ID,
		"category":    spec.Category,
		"name":        spec.Name,
		"path":        subpaths[0]["path"],
		"sizeGb":      gb(total),
		"origSizeGb":  gb(total),
		"items":       len(subpaths),
		"filesCount":  itoa64(int64(len(subpaths))) + " 个目录",
		"lastMod":     "本机扫描",
		"rebuildTime": spec.Rebuild,
		"tier":        spec.Tier,
		"checked":     spec.Tier != "protected",
		"cleaned":     false,
		"impact":      spec.Impact,
		"subpaths":    subpaths,
	}
}

// scanCleanGroups lists the caches and logs the clean module offers to remove.
func (s *Scanner) scanCleanGroups() []map[string]any {
	specs := []CatalogSpec{
		{
			ID:       "user_caches",
			Category: "应用缓存与日志",
			Name:     "用户缓存与诊断日志",
			Roots:    []string{"~/Library/Caches", "~/Library/Logs"},
			Tier:     "safe",
			Impact:   "只统计本机实际存在的缓存和日志目录。",
			Rebuild:  "应用下次启动时按需重建",
		},
		{
			ID:       "browser_caches",
			Category: "浏览器缓存",
			Name:     "Safari / Chrome / Firefox 缓存",
			Roots: []string{
				"~/Library/Caches/com.apple.Safari",
				"~/Library/Caches/Google",
			},
			Tier:    "safe",
			Impact:  "浏览器缓存可以重建，不影响书签和登录状态。",
			Rebuild: "下次打开网页时重新生成",
		},
		{
			ID:       "xcode_derived",
			Category: "开发缓存",
			Name:     "Xcode DerivedData 与模拟器缓存",
			Roots: []string{
				"~/Library/Developer/Xcode/DerivedData",
				"~/Library/Developer/CoreSimulator/Caches",
			},
			Tier:    "safe",
			Impact:  "编译中间产物可以删除，下次编译会重新生成。",
			Rebuild: "下次编译重新生成",
		},
		{
			ID:       "package_caches",
			Category: "包管理器缓存",
			Name:     "npm / pip / Homebrew 下载缓存",
			Roots: []string{
				"~/.npm/_cacache",
				"~/Library/Caches/pip",
				"~/Library/Caches/Homebrew",
			},
			Tier:    "review",
			Impact:  "包管理器缓存可以重新下载。",
			Rebuild: "下次安装时重新下载",
		},
	}
	var groups []map[string]any
	for _, spec := range specs {
		if row := s.catalog(spec); row != nil {
			groups = append(groups, row)
		}
	}
	return groups
}

// scanAICaches lists the AI tool caches the moon base module offers to remove,
// one item per band.
func (s *Scanner) scanAICaches() []map[string]any {
	specs := []struct {
		spec CatalogSpec
		band string
	}{
		{
			spec: CatalogSpec{
				ID:       "ai_versions",
				Category: "可重建程序包",
				Name:     "Claude / Cursor 旧版本与扩展缓存",
				Roots:    []string{"~/.local/share/claude/versions", "~/.cursor/extensions"},
				Tier:     "safe",
				Impact:   "旧版本程序包和扩展缓存可以重新下载。",
			},
			band: "可重建程序包",
		},
		{
			spec: CatalogSpec{
				ID:       "ai_runtime_cache",
				Category: "运行缓存",
				Name:     "Cursor / Claude 运行缓存与日志",
				Roots:    []string{"~/Library/Caches/Cursor", "~/Library/Logs/Claude"},
				Tier:     "safe",
				Impact:   "运行缓存和日志不影响对话记录。",
			},
			band: "运行缓存",
		},
		{
			spec: CatalogSpec{
				ID:       "ai_worktrees",
				Category: "临时工作树",
				Name:     "Codex 临时工作树与模型下载缓存",
				Roots:    []string{"~/.codex/worktrees", "~/.cache/huggingface"},
				Tier:     "review",
				Impact:   "删除前确认没有未提交的工作树。",
			},
			band: "临时工作树",
		},
	}
	var items []map[string]any
	for _, entry := range specs {
		row := s.catalog(entry.spec)
		if row == nil {
			continue
		}
		row["band"] = entry.band
		row["filesCount"] = itoa64(int64(intOf(row["items"]))) + " 个目录"
		row["lastMod"] = "本机扫描"
		row["rebuildTime"] = "可重新下载或重新生成"
		items = append(items, row)
	}
	return items
}

// installerExtensions are the file types the purge module counts as an
// installer.
var installerExtensions = map[string]struct{}{
	"dmg": {}, "pkg": {}, "xip": {}, "iso": {}, "zip": {},
}

// scanInstallerPackages lists the installers in the download folders that are
// big enough to be worth removing.
func (s *Scanner) scanInstallerPackages() []map[string]any {
	roots := []string{"~/Downloads", "~/Library/Caches/Homebrew/downloads"}
	type candidate struct {
		path     string
		name     string
		bytes    int64
		modified int64
	}
	var candidates []candidate
	scanned := 0
	for _, root := range roots {
		path := s.expand(root)
		walkFiles(path, 2500, &scanned, func(file string, bytes int64, modified int64) {
			if bytes <= 20_000_000 {
				return
			}
			if !hasExtension(file, installerExtensions) {
				return
			}
			candidates = append(candidates, candidate{
				path:     file,
				name:     baseName(file),
				bytes:    bytes,
				modified: modified,
			})
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].bytes > candidates[j].bytes
	})
	if len(candidates) > 20 {
		candidates = candidates[:20]
	}
	items := []map[string]any{}
	for i, c := range candidates {
		items = append(items, map[string]any{
			"id":       "installer_" + itoa64(int64(i)),
			"name":     c.name,
			"location": dirName(c.path),
			"path":     s.relativize(c.path),
			"sizeGb":   gb(c.bytes),
			"age":      ageText(c.modified),
			"checked":  true,
		})
	}
	return items
}

// artifactKinds names the build artifacts the purge module recognizes.
var artifactKinds = map[string]string{
	"node_modules": "前端依赖目录",
	"target":       "Rust / Java 构建产物",
	"DerivedData":  "Xcode 构建缓存",
	".venv":        "Python 虚拟环境",
	"venv":         "Python 虚拟环境",
	"dist":         "前端构建产物",
	"build":        "构建产物",
}

// scanBuildArtifacts lists the build artifacts in the project folders that are
// big enough to be worth removing.
func (s *Scanner) scanBuildArtifacts() []map[string]any {
	roots := []string{"~/Projects", "~/GitHub", "~/Developer", "~/code", "~/src"}
	var candidates []artifactCandidate
	scanned := 0
	for _, root := range roots {
		s.walkArtifacts(s.expand(root), 4000, &scanned, &candidates)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].bytes > candidates[j].bytes
	})
	if len(candidates) > 20 {
		candidates = candidates[:20]
	}
	items := []map[string]any{}
	for i, c := range candidates {
		items = append(items, map[string]any{
			"id":          "artifact_" + itoa64(int64(i)),
			"project":     dirName(c.path),
			"artifact":    c.name,
			"name":        c.name,
			"path":        s.relativize(c.path),
			"kind":        c.kind,
			"sizeGb":      gb(c.bytes),
			"origSizeGb":  gb(c.bytes),
			"age":         ageText(c.modified),
			"rebuildNote": "删除后可重新安装或重新编译",
			"gitClean":    "本机扫描",
			"checked":     true,
		})
	}
	return items
}

// artifactCandidate is one build artifact the scan found.
type artifactCandidate struct {
	path     string
	name     string
	kind     string
	bytes    int64
	modified int64
}

// walkArtifacts walks dir looking for build artifacts, and does not descend
// into a directory it already counts as one.
func (s *Scanner) walkArtifacts(dir string, maxFiles int, scanned *int, out *[]artifactCandidate) {
	entries := readDir(dir)
	for _, entry := range entries {
		if *scanned > maxFiles {
			return
		}
		*scanned++
		name := entry.name
		path := dir + "/" + name
		kind, isArtifact := artifactKinds[name]
		if isArtifact {
			size := s.fastDirectorySize(path, 1500)
			if size.bytes >= 20_000_000 {
				*out = append(*out, artifactCandidate{
					path:     path,
					name:     name,
					kind:     kind,
					bytes:    size.bytes,
					modified: modifiedTime(path),
				})
			}
			continue
		}
		if entry.isDir && !isHidden(name) {
			s.walkArtifacts(path, maxFiles, scanned, out)
		}
	}
}
