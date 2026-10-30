package scanner

import (
	"path/filepath"
	"sort"
	"strings"
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

// The file budget of a catalog root, a single child of an expanded root, and an
// app bundle. The old 1200/1500/2500 budgets counted directory entries as well,
// so a tree with many folders ran out before it counted a meaningful number of
// files: ~/Library/Caches reported 0.89 GB of an actual 2.18 GB and
// GarageBand.app reported 0.039 GB of an actual 1.13 GB.
const (
	catalogRootFileCap  = 25000
	catalogChildFileCap = 25000
	appBundleFileCap    = 50000
	appLeftoverFileCap  = 20000
	orphanFileCap       = 20000
	installerFileCap    = 25000
)

// catalogClaim is a directory a catalog row already reports, so a later row
// does not count the same bytes a second time (S-08).
type catalogClaim struct {
	path  string
	bytes int64
}

// catalogClaims is the set of directories catalog rows already report.
type catalogClaims struct {
	items []catalogClaim
}

// add records that path is reported by a row.
func (c *catalogClaims) add(path string, bytes int64) {
	c.items = append(c.items, catalogClaim{path: path, bytes: bytes})
}

// covers reports whether path is already reported by, or inside a directory
// that is already reported by, an earlier row.
func (c *catalogClaims) covers(path string) bool {
	for _, claim := range c.items {
		if path == claim.path || strings.HasPrefix(path, claim.path+"/") {
			return true
		}
	}
	return false
}

// claimedBytesUnder is the sum of the claimed directories inside path.
func (c *catalogClaims) claimedBytesUnder(path string) int64 {
	var total int64
	for _, claim := range c.items {
		if strings.HasPrefix(claim.path, path+"/") {
			total += claim.bytes
		}
	}
	return total
}

// catalogChild is one directory an expanded root reports on its own.
type catalogChild struct {
	path  string
	bytes int64
}

// catalog measures the directories of spec that exist, and returns a clean list
// row. It returns nil when none of them exist, all of them are empty, or all of
// them are protected by the whitelist.
//
// excluded lists roots other rows already report (a browser group inside
// ~/Library/Caches, for example): an ancestor root is then reported child by
// child instead of as a whole, so every byte is counted once and every row
// stays selectable. A directory the whitelist protects as a whole is never
// emitted; one that is only protected because a child is protected is replaced
// by its unprotected children, because the folder can never be moved whole.
func (s *Scanner) catalog(spec CatalogSpec, claims *catalogClaims, excluded []string) map[string]any {
	checkedByTier := spec.Tier == "safe"
	var subpaths []map[string]any
	var total int64
	firstPath := ""

	for _, root := range spec.Roots {
		path := s.expand(root)
		if !exists(path) {
			continue
		}
		if claims.covers(path) {
			continue
		}
		expandChildren, offered := s.scanMayOffer(path)
		if !offered {
			continue
		}
		if descendants := excludedUnder(excluded, path); len(descendants) > 0 {
			expandChildren = true
		}
		if expandChildren {
			for _, child := range s.catalogChildren(path, claims, excluded) {
				claims.add(child.path, child.bytes)
				total += child.bytes
				if firstPath == "" {
					firstPath = s.relativize(path)
				}
				subpaths = append(subpaths, map[string]any{
					"path":    s.relativize(child.path),
					"size":    formatBytes(child.bytes),
					"sizeGb":  plainGB(child.bytes),
					"checked": checkedByTier,
				})
			}
			continue
		}
		size := s.fastDirectorySize(path, catalogRootFileCap)
		if size.bytes <= 0 {
			continue
		}
		// A parent measured before one of its children was claimed must not
		// count those bytes twice.
		if claimed := claims.claimedBytesUnder(path); claimed > 0 {
			if claimed >= size.bytes {
				continue
			}
			size.bytes -= claimed
		}
		claims.add(path, size.bytes)
		total += size.bytes
		if firstPath == "" {
			firstPath = s.relativize(path)
		}
		subpaths = append(subpaths, map[string]any{
			"path":    s.relativize(path),
			"size":    formatBytes(size.bytes),
			"sizeGb":  plainGB(size.bytes),
			"checked": checkedByTier,
		})
	}
	if len(subpaths) == 0 {
		return nil
	}
	return map[string]any{
		"id":          spec.ID,
		"category":    spec.Category,
		"name":        spec.Name,
		"path":        firstPath,
		"sizeGb":      gb(total),
		"origSizeGb":  gb(total),
		"items":       len(subpaths),
		"filesCount":  itoa64(int64(len(subpaths))) + " 个目录",
		"lastMod":     "本机扫描",
		"rebuildTime": spec.Rebuild,
		"tier":        spec.Tier,
		"checked":     checkedByTier,
		"cleaned":     false,
		"impact":      spec.Impact,
		"subpaths":    subpaths,
	}
}

// catalogChildren lists the direct children of an expanded root that may be
// offered on their own: they are not protected, not already claimed by an
// earlier row, and not roots another row reports.
func (s *Scanner) catalogChildren(parent string, claims *catalogClaims, excluded []string) []catalogChild {
	entries := readDir(parent)
	children := make([]catalogChild, 0, len(entries))
	for _, entry := range entries {
		if isHidden(entry.name) {
			continue
		}
		child := filepath.Join(parent, entry.name)
		if claims.covers(child) {
			continue
		}
		if len(excludedUnder(excluded, child)) > 0 || containsString(excluded, child) {
			continue
		}
		if _, protected := s.scanProtection(child); protected {
			continue
		}
		size := s.fastDirectorySize(child, catalogChildFileCap)
		if size.bytes <= 0 {
			continue
		}
		children = append(children, catalogChild{path: child, bytes: size.bytes})
	}
	sort.SliceStable(children, func(i, j int) bool {
		return children[i].bytes > children[j].bytes
	})
	return children
}

// excludedUnder returns the excluded roots that live under path.
func excludedUnder(excluded []string, path string) []string {
	var under []string
	for _, candidate := range excluded {
		if strings.HasPrefix(candidate, path+"/") {
			under = append(under, candidate)
		}
	}
	return under
}

// specRoots lists the expanded roots of specs other than exceptID, which are
// the roots another catalog row already reports.
func (s *Scanner) specRoots(specs []CatalogSpec, exceptID string) []string {
	var roots []string
	for _, spec := range specs {
		if spec.ID == exceptID {
			continue
		}
		for _, root := range spec.Roots {
			roots = append(roots, s.expand(root))
		}
	}
	return roots
}

// cleanGroupSpecs is the clean module's list of rows.
func cleanGroupSpecs() []CatalogSpec {
	return []CatalogSpec{
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
}

// scanCleanGroups lists the caches and logs the clean module offers to remove.
func (s *Scanner) scanCleanGroups() []map[string]any {
	specs := cleanGroupSpecs()
	claims := &catalogClaims{}
	var groups []map[string]any
	// The AI module reports some of the same directories; its roots take part
	// in the dedupe too, or a cache would be counted in both payload sections.
	moduleSpecs := append([]CatalogSpec{}, specs...)
	for _, entry := range aiCacheSpecs() {
		moduleSpecs = append(moduleSpecs, entry.spec)
	}
	for _, spec := range specs {
		var excluded []string
		if spec.ID == "user_caches" {
			// Cache groups nested inside ~/Library/Caches report their own
			// roots, so user_caches reports the remaining children instead of
			// counting those bytes twice.
			excluded = s.specRoots(moduleSpecs, spec.ID)
		}
		if row := s.catalog(spec, claims, excluded); row != nil {
			groups = append(groups, row)
		}
	}
	return groups
}

// aiCacheSpecs is the moon base module's list of rows, one per band.
func aiCacheSpecs() []struct {
	spec CatalogSpec
	band string
} {
	return []struct {
		spec CatalogSpec
		band string
	}{
		{
			spec: CatalogSpec{
				ID:       "ai_versions",
				Category: "可重建程序包",
				Name:     "Claude / Cursor 程序包与扩展目录",
				Roots:    []string{"~/.local/share/claude/versions", "~/.cursor/extensions"},
				Tier:     "review",
				Impact:   "这里可能包含当前程序版本或已安装扩展，移除后需要重新安装。请先退出相关工具，确认所选路径不再使用。",
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
}

// scanAICaches lists the AI tool caches the moon base module offers to remove,
// one item per band.
func (s *Scanner) scanAICaches() []map[string]any {
	claims := &catalogClaims{}
	var items []map[string]any
	for _, entry := range aiCacheSpecs() {
		row := s.catalog(entry.spec, claims, nil)
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

// catalogRoots lists the expanded roots of every catalog row that is still
// offered, which orphan detection uses to avoid reporting the same directory
// twice. Rows are statically known, so this does not depend on scan order.
func (s *Scanner) catalogRoots() []string {
	specs := cleanGroupSpecs()
	for _, entry := range aiCacheSpecs() {
		specs = append(specs, entry.spec)
	}
	var roots []string
	for _, spec := range specs {
		for _, root := range spec.Roots {
			path := s.expand(root)
			if !exists(path) {
				continue
			}
			expandChildren, offered := s.scanMayOffer(path)
			if !offered {
				continue
			}
			if expandChildren {
				// An expanded row reports its unprotected children, so those
				// are the directories orphan detection must not repeat.
				for _, entry := range readDir(path) {
					if isHidden(entry.name) {
						continue
					}
					child := filepath.Join(path, entry.name)
					if !exists(child) {
						continue
					}
					if _, protected := s.scanProtection(child); protected {
						continue
					}
					roots = append(roots, child)
				}
				continue
			}
			roots = append(roots, path)
		}
	}
	return roots
}

// installerExtensions are the file types the purge module counts as an
// installer.
var installerExtensions = map[string]struct{}{
	"dmg": {}, "pkg": {}, "xip": {}, "iso": {}, "zip": {},
}

// installerZipHints are the words that make a zip archive a plausible
// installer. A zip without one of them is a normal download (family photos,
// project exports), and listing it as a removable installer was wrong.
var installerZipHints = []string{
	"installer", "install", "setup", "update", "upgrade", "macos", "osx",
	"universal", "arm64", "x64", "amd64", "x86_64",
}

// isInstallerFile reports whether name is an installer archive, and whether it
// is a zip that only counts because its name says so.
func isInstallerFile(name string) (installer bool, zipArchive bool) {
	extension := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	if _, ok := installerExtensions[extension]; !ok {
		return false, false
	}
	if extension != "zip" {
		return true, false
	}
	lower := strings.ToLower(name)
	for _, hint := range installerZipHints {
		if strings.Contains(lower, hint) {
			return true, true
		}
	}
	return false, false
}

// scanInstallerPackages lists the installers in the download folders that are
// big enough to be worth removing. Zip archives are only listed when their name
// marks them as installers, and they are never pre-checked.
func (s *Scanner) scanInstallerPackages() []map[string]any {
	roots := []string{"~/Downloads", "~/Library/Caches/Homebrew/downloads"}
	type candidate struct {
		path     string
		name     string
		bytes    int64
		modified int64
		zip      bool
	}
	var candidates []candidate
	scanned := 0
	for _, root := range roots {
		path := s.expand(root)
		walkFiles(path, installerFileCap, &scanned, func(file string, bytes int64, modified int64) {
			if bytes <= 20_000_000 {
				return
			}
			installer, zipArchive := isInstallerFile(baseName(file))
			if !installer {
				return
			}
			if _, protected := s.scanProtection(file); protected {
				return
			}
			candidates = append(candidates, candidate{
				path:     file,
				name:     baseName(file),
				bytes:    bytes,
				modified: modified,
				zip:      zipArchive,
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
			"checked":  !c.zip,
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
			if _, protected := s.scanProtection(path); protected {
				continue
			}
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
