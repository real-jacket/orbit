package scanner

import (
	"path/filepath"
	"sort"
	"strings"
)

// scanOrphanedLeftovers lists the directories of apps that are gone, plus the
// directories under ~/Library that no installed app claims.
func (s *Scanner) scanOrphanedLeftovers() []map[string]any {
	home := s.home
	installedNames, installedBundleIDs := s.installedAppIndex(home)
	orphans := []map[string]any{}
	claimedPaths := map[string]struct{}{}

	for _, signature := range AllSignatures(home) {
		type candidate struct {
			path string
			kind string
		}
		candidates := make([]candidate, 0, len(signature.FixedPaths)+4)
		for _, fixed := range signature.FixedPaths {
			candidates = append(candidates, candidate{path: fixed.Path, kind: fixed.Kind})
		}
		for _, wildcard := range signature.Wildcards {
			for _, matched := range s.matchingWildcardPaths(wildcard.Parent, wildcard.Prefix) {
				candidates = append(candidates, candidate{path: matched, kind: wildcard.Kind})
			}
		}

		var subpaths []map[string]any
		var totalBytes int64
		for _, candidate := range candidates {
			if claimed(claimedPaths, candidate.path) {
				continue
			}
			if !exists(candidate.path) {
				continue
			}
			size := s.fastDirectorySize(candidate.path, 3500)
			if size.bytes <= 0 {
				continue
			}
			claimedPaths[candidate.path] = struct{}{}
			totalBytes += size.bytes
			subpaths = append(subpaths, map[string]any{
				"path":    s.relativize(candidate.path),
				"kind":    candidate.kind,
				"size":    formatBytes(size.bytes),
				"checked": true,
			})
		}

		if len(subpaths) == 0 {
			continue
		}
		stillInstalled := false
		for _, keyword := range signature.AppKeywords {
			for name := range installedNames {
				if strings.Contains(name, keyword) {
					stillInstalled = true
				}
			}
		}
		if _, ok := installedBundleIDs[strings.ToLower(signature.BundleID)]; ok {
			stillInstalled = true
		}
		statusText := "主程序已卸载 · 孤立残留"
		if stillInstalled {
			statusText = "深层容器与 SDK 残留"
		}
		leftoverGB := round2(float64(totalBytes) / 1_000_000_000)
		if leftoverGB < 0.01 {
			leftoverGB = 0.01
		}
		orphans = append(orphans, map[string]any{
			"id":             signature.ID,
			"name":           signature.Name,
			"category":       signature.Category,
			"bundleId":       signature.BundleID,
			"statusText":     statusText,
			"leftoverSizeGb": leftoverGB,
			"checked":        !stillInstalled,
			"impact":         signature.Impact,
			"subpaths":       subpaths,
		})
	}

	containerOrphans := s.scanUnclaimedContainers(home, claimedPaths, installedNames, installedBundleIDs)
	if len(containerOrphans) > 18 {
		containerOrphans = containerOrphans[:18]
	}
	orphans = append(orphans, containerOrphans...)

	sort.SliceStable(orphans, func(i, j int) bool {
		return floatOf(orphans[i]["leftoverSizeGb"]) > floatOf(orphans[j]["leftoverSizeGb"])
	})
	return orphans
}

// claimed reports whether path is already taken by, or contains, a path the
// scan already counted.
func claimed(claimedPaths map[string]struct{}, path string) bool {
	for existing := range claimedPaths {
		if strings.HasPrefix(path, existing+"/") || strings.HasPrefix(existing, path+"/") {
			return true
		}
	}
	return false
}

// ignoredPrefixes are the directories under ~/Library that belong to macOS or
// to an app the scanner would rather not call abandoned.
var ignoredPrefixes = []string{
	"com.apple.", "group.com.apple.", "apple", "cloudkit", "knowledge", "caches",
	"crashreporter", "geoservices", "addressbook", "facetime", "homekit", "safari",
	"spotlight", "google", "jetbrains", "mozilla", "microsoft",
}

// scanUnclaimedContainers lists the directories under ~/Library that belong to
// no installed app: the leftovers of a normal uninstall, where the bundle is
// gone but its support files are not.
func (s *Scanner) scanUnclaimedContainers(
	home string,
	claimedPaths map[string]struct{},
	installedNames, installedBundleIDs map[string]struct{},
) []map[string]any {
	targets := []struct {
		path     string
		kind     string
		category string
	}{
		{home + "/Library/Containers", "cache", "孤立沙盒容器残留"},
		{home + "/Library/Group Containers", "cache", "孤立沙盒容器残留"},
		{home + "/Library/Application Support", "config", "已卸载应用支持残留"},
		{home + "/Library/Caches", "cache", "已卸载应用支持残留"},
	}

	found := []map[string]any{}
	claimedNow := make(map[string]struct{}, len(claimedPaths))
	for path := range claimedPaths {
		claimedNow[path] = struct{}{}
	}
	for _, target := range targets {
		entries := readDir(target.path)
		for _, entry := range entries {
			item := entry.name
			if strings.HasPrefix(item, ".") {
				continue
			}
			lower := strings.ToLower(item)
			if ignored(lower) {
				continue
			}
			fullPath := filepath.Join(target.path, item)
			if _, ok := claimedNow[fullPath]; ok {
				continue
			}
			if s.matchesInstalled(lower, installedNames, installedBundleIDs) {
				continue
			}
			size := s.fastDirectorySize(fullPath, 1200)
			if size.bytes < 15_000_000 {
				continue
			}
			claimedNow[fullPath] = struct{}{}
			leftoverGB := round2(float64(size.bytes) / 1_000_000_000)
			if leftoverGB < 0.02 {
				leftoverGB = 0.02
			}
			found = append(found, map[string]any{
				"id":             "orphan_" + item,
				"name":           item,
				"category":       target.category,
				"bundleId":       item,
				"statusText":     "主程序已卸载 · 孤立残留",
				"leftoverSizeGb": leftoverGB,
				"checked":        true,
				"impact":         "经与 /Applications 已安装列表比对，该目录对应的主程序已不存在，属于卸载后遗留的无主残留目录。",
				"subpaths": []map[string]any{{
					"path":    s.relativize(fullPath),
					"kind":    target.kind,
					"size":    formatBytes(size.bytes),
					"checked": true,
				}},
			})
			if len(found) >= 18 {
				return found
			}
		}
	}
	return found
}

// ignored reports whether a directory belongs to macOS or to an app the
// scanner keeps.
func ignored(lower string) bool {
	for _, prefix := range ignoredPrefixes {
		if lower == prefix || strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// matchesInstalled reports whether a directory name belongs to an installed
// app, by bundle identifier or, for a name long enough to be meaningful, by
// name.
func (s *Scanner) matchesInstalled(
	lower string,
	installedNames, installedBundleIDs map[string]struct{},
) bool {
	if _, ok := installedBundleIDs[lower]; ok {
		return true
	}
	for bundleID := range installedBundleIDs {
		if strings.Contains(lower, bundleID) || strings.Contains(bundleID, lower) {
			return true
		}
	}
	for name := range installedNames {
		if len(name) < 3 {
			continue
		}
		if strings.Contains(lower, name) || strings.Contains(name, lower) {
			return true
		}
	}
	return false
}
