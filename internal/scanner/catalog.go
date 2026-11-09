package scanner

import (
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CatalogSpec describes one row of the clean list: which directories make it
// up, how safe it is to remove them, and what the user loses.
type CatalogSpec struct {
	ID             string
	Category       string
	Name           string
	Roots          []string
	Tier           string
	Impact         string
	Rebuild        string
	ExpandChildren bool
	// Reason explains a review verdict of the whole spec; Classify, when set,
	// decides each path on its own (the user cache folders).
	Reason   string
	Classify func(s *Scanner, path string, size dirSize) verdict
}

// The file cap of a catalog root, a single child of an expanded root, an app
// bundle, an app's leftovers and an orphan folder. Zero is no cap: the walk is
// bounded by scanWalkBudget instead, so big package-manager and AI caches are
// measured completely. Counting directory entries against a file budget made
// ~/Library/Caches report 0.89 GB of an actual 2.18 GB.
const (
	catalogRootFileCap  = 0
	catalogChildFileCap = 0
	appBundleFileCap    = 0
	appLeftoverFileCap  = 0
	orphanFileCap       = 0
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
	path string
	size dirSize
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
	var subpaths []map[string]any
	var total int64
	var files int
	var latest int64
	partial := false
	firstPath := ""
	appendPath := func(path string, size dirSize) {
		claims.add(path, size.bytes)
		total += size.bytes
		files += size.files
		latest = max(latest, size.modified)
		partial = partial || size.partial
		v := verdict{tier: spec.Tier, reason: spec.Reason}
		if spec.Classify != nil {
			v = spec.Classify(s, path, size)
		}
		item := map[string]any{
			"path": s.relativize(path), "size": formatBytes(size.bytes), "sizeGb": plainGB(size.bytes),
			"checked": v.tier == "safe", "partial": size.partial, "tier": v.tier, "isDir": size.directory,
		}
		if v.reason != "" {
			item["reason"] = v.reason
		}
		if v.unlock != "" {
			item["unlock"] = v.unlock
		}
		subpaths = append(subpaths, item)
	}

	for _, root := range spec.Roots {
		path := s.expand(root)
		if !exists(path) {
			continue
		}
		if claims.covers(path) {
			continue
		}
		expandChildren, offered := s.scanMayOffer(path)
		if !offered && spec.Tier != "protected" {
			continue
		}
		expandChildren = spec.ExpandChildren || expandChildren
		if descendants := excludedUnder(excluded, path); len(descendants) > 0 {
			expandChildren = true
		}
		if expandChildren {
			for _, child := range s.catalogChildren(path, claims, excluded) {
				if firstPath == "" {
					firstPath = s.relativize(path)
				}
				appendPath(child.path, child.size)
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
		if firstPath == "" {
			firstPath = s.relativize(path)
		}
		appendPath(path, size)
	}
	if len(subpaths) == 0 {
		return nil
	}
	lastMod := "无法读取修改时间"
	if latest > 0 {
		lastMod = time.Unix(latest, 0).Local().Format("2006-01-02 15:04")
	}
	row := map[string]any{
		"id":          spec.ID,
		"category":    spec.Category,
		"name":        spec.Name,
		"path":        firstPath,
		"sizeGb":      gb(total),
		"origSizeGb":  gb(total),
		"items":       len(subpaths),
		"filesCount":  itoa64(int64(files)) + " 个文件 · " + itoa64(int64(len(subpaths))) + " 个路径",
		"lastMod":     lastMod,
		"partial":     partial,
		"source":      "orbit 本地目录扫描",
		"rebuildTime": spec.Rebuild,
		"tier":        spec.Tier,
		"baseTier":    spec.Tier, // tier before measurement protection; the UI never lifts a "protected" spec
		"checked":     spec.Tier == "safe",
		"cleaned":     false,
		"impact":      spec.Impact,
		"subpaths":    subpaths,
	}
	s.applyCleanupProtection(row)
	return row
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
		children = append(children, catalogChild{path: child, size: size})
	}
	sort.SliceStable(children, func(i, j int) bool {
		return children[i].size.bytes > children[j].size.bytes
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
			ID:             "user_caches",
			Category:       "应用缓存与日志",
			Name:           "用户缓存与诊断日志",
			Roots:          []string{"~/Library/Caches", "~/Library/Logs"},
			Tier:           "review",
			ExpandChildren: true,
			Classify:       (*Scanner).cacheVerdict,
			Impact:         "逐个子路径给出建议：可重建的工具缓存与未运行应用的缓存直接推荐，未识别的缓存说明原因后由你确认，运行中的应用和账户同步数据保持原状。",
			Rebuild:        "重建行为依应用而定，未知用途不处理",
		},
		{
			ID:       "browser_caches",
			Category: "浏览器缓存",
			Name:     "Safari / Chrome / Firefox 缓存",
			Roots: []string{
				"~/Library/Caches/com.apple.Safari",
				"~/Library/Caches/Google",
				"~/Library/Caches/Firefox",
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
			Name:     "开发包管理器下载缓存",
			Roots: []string{
				"~/.npm/_cacache",
				"~/Library/Caches/Yarn",
				"~/.bun/install/cache",
				"~/Library/Caches/pip",
				"~/.cache/uv",
				"~/go/pkg/mod/cache/download",
				"~/.cargo/registry/cache",
				"~/Library/Caches/Homebrew",
			},
			Tier:    "safe",
			Reason:  "包管理器下载缓存，下次安装依赖时会自动重新下载",
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

func (s *Scanner) cleanExcludedRoots() []map[string]any {
	return s.excludedCatalogRoots(cleanGroupSpecs())
}

func (s *Scanner) aiExcludedRoots() []map[string]any {
	var specs []CatalogSpec
	for _, entry := range aiCacheSpecs() {
		if !entry.logFiles && entry.spec.Tier != "protected" {
			specs = append(specs, entry.spec)
		}
	}
	return s.excludedCatalogRoots(specs)
}

func (s *Scanner) excludedCatalogRoots(specs []CatalogSpec) []map[string]any {
	policy := s.whitelist()
	policy.Patterns = append(splitCompositeWhitelistPatterns(policy.Patterns, s.home), policy.Patterns...)
	claims := &catalogClaims{}
	rows := []map[string]any{}
	for _, spec := range specs {
		for _, root := range spec.Roots {
			path := s.expand(root)
			if !exists(path) || claims.covers(path) {
				continue
			}
			protection, protected := policy.Protection(path)
			if !protected || protection.Descendant {
				protection, protected = s.scanProtection(path)
			}
			if !protected || protection.Descendant {
				continue
			}
			reason := protection.Reason
			if reason == "" {
				reason = "白名单排除：" + s.relativize(protection.Pattern)
			}
			claims.add(path, 0)
			rows = append(rows, map[string]any{"path": s.relativize(path), "name": spec.Name, "reason": reason, "tool": spec.Category})
		}
	}
	return rows
}

// scanAICaches lists the AI tool caches the moon base module offers to remove,
// one item per band.
func (s *Scanner) scanAICaches() []map[string]any {
	claims := &catalogClaims{}
	items := s.scanAIVersions(claims)
	// Before the protected session rows, so their measured bytes exclude these units.
	items = append(items, s.scanAISessions(claims)...)
	for _, entry := range aiCacheSpecs() {
		var row map[string]any
		if entry.logFiles {
			row = s.scanAIDiagnosticFiles(entry, claims)
		} else {
			row = s.catalog(entry.spec, claims, nil)
		}
		if row == nil {
			continue
		}
		row["band"], row["tool"] = entry.band, entry.tool
		items = append(items, row)
	}
	items = append(items, s.scanAIWorktrees(claims)...)
	for _, row := range items {
		if tool, _ := row["tool"].(string); s.aiToolUninstalled(tool) {
			row["uninstalled"] = true
		}
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
