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
	// Directories a catalog row already reports must not be offered a second
	// time as an orphan (S-08).
	claimedPaths := map[string]struct{}{}
	for _, root := range s.catalogRoots() {
		claimedPaths[root] = struct{}{}
	}
	usedIDs := map[string]struct{}{}

	for _, signature := range AllSignatures(home) {
		stillInstalled := signatureStillInstalled(signature, installedNames, installedBundleIDs)
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
			if _, protected := s.scanProtection(candidate.path); protected {
				continue
			}
			size := s.fastDirectorySize(candidate.path, orphanFileCap)
			if size.bytes <= 0 {
				continue
			}
			claimedPaths[candidate.path] = struct{}{}
			totalBytes += size.bytes
			// A leftover whose owner is still installed is shown but never
			// pre-checked.
			subpaths = append(subpaths, map[string]any{
				"path":    s.relativize(candidate.path),
				"kind":    candidate.kind,
				"size":    formatBytes(size.bytes),
				"checked": !stillInstalled,
			})
		}

		if len(subpaths) == 0 {
			continue
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
			"id":             uniqueOrphanID(usedIDs, signature.ID),
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

	containerOrphans := s.scanUnclaimedContainers(home, claimedPaths, installedNames, installedBundleIDs, usedIDs)
	if len(containerOrphans) > 18 {
		containerOrphans = containerOrphans[:18]
	}
	orphans = append(orphans, containerOrphans...)

	sort.SliceStable(orphans, func(i, j int) bool {
		return floatOf(orphans[i]["leftoverSizeGb"]) > floatOf(orphans[j]["leftoverSizeGb"])
	})
	return orphans
}

// signatureStillInstalled reports whether a signature's app is still present.
func signatureStillInstalled(
	signature OrphanSignature,
	installedNames, installedBundleIDs map[string]struct{},
) bool {
	for _, keyword := range signature.AppKeywords {
		for name := range installedNames {
			if strings.Contains(name, keyword) {
				return true
			}
		}
	}
	if _, ok := installedBundleIDs[strings.ToLower(signature.BundleID)]; ok {
		return true
	}
	return false
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

// uniqueOrphanID keeps row ids unique even when two directories share a name,
// by adding the root that found them before the name (S-12).
func uniqueOrphanID(used map[string]struct{}, id string) string {
	if _, exists := used[id]; !exists {
		used[id] = struct{}{}
		return id
	}
	for index := 2; ; index++ {
		candidate := id + "_" + itoa64(int64(index))
		if _, exists := used[candidate]; !exists {
			used[candidate] = struct{}{}
			return candidate
		}
	}
}

// ignoredPrefixes are the directories under ~/Library that belong to macOS or
// to an app the scanner would rather not call abandoned.
var ignoredPrefixes = []string{
	"com.apple.", "group.com.apple.", "apple", "cloudkit", "knowledge", "caches",
	"crashreporter", "geoservices", "addressbook", "facetime", "homekit", "safari",
	"spotlight", "google", "jetbrains", "mozilla", "microsoft",
	// macOS data, not leftovers: MobileSync, sync services, system agents and
	// the support folders the OS itself keeps under Application Support.
	"mobilesync", "syncservices", "appplaceholdersyncd", "contactsd",
	"icloud", "icdd", "icloudmailagent", "identityservicesd",
	"locationaccessstored", "networkserviceproxy", "privatecloudcomputed",
	"stickersd", "tipsd", "animoji", "app store", "appexit", "askpermission",
	"callhistorydb", "callhistorytransactions", "clouddocs", "controlcenter",
	"differentialprivacy", "dock", "fileprovider", "findmy", "firestore",
	"gk", "homeenergyd", "intelligenceflow", "isolatedstorage", "lifecycle",
	"passwords", "siritts", "stocks", "suggestions", "trial", "wallet", // ggignore: 目录名，不是密码
	"com.apple", "default.store", "devcert", "crash", "diagnosticreports",
	"cookies", "httpstorages", "saved application state", "preferences",
}

// ignored reports whether a directory belongs to macOS or to an app the
// scanner keeps.
func ignored(lower string) bool {
	for _, prefix := range ignoredPrefixes {
		if lower == prefix || strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	// Group containers are prefixed with the developer team id, e.g.
	// 243LU875E5.groups.com.apple.podcasts: the ".apple." marker identifies
	// Apple's own containers whatever the team id is.
	return strings.Contains(lower, ".apple.")
}

// scanUnclaimedContainers lists the directories under ~/Library that belong to
// no installed app: the leftovers of a normal uninstall, where the bundle is
// gone but its support files are not.
func (s *Scanner) scanUnclaimedContainers(
	home string,
	claimedPaths map[string]struct{},
	installedNames, installedBundleIDs map[string]struct{},
	usedIDs map[string]struct{},
) []map[string]any {
	targets := []struct {
		path     string
		kind     string
		category string
		key      string
	}{
		{home + "/Library/Containers", "cache", "孤立沙盒容器残留", "containers"},
		{home + "/Library/Group Containers", "cache", "孤立沙盒容器残留", "group_containers"},
		{home + "/Library/Application Support", "config", "已卸载应用支持残留", "application_support"},
		{home + "/Library/Caches", "cache", "已卸载应用支持残留", "caches"},
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
			// A claimed parent (a catalog root, for example) claims its whole
			// subtree too, or the same bytes appear in both sections.
			if claimed(claimedNow, fullPath) {
				continue
			}
			if _, protected := s.scanProtection(fullPath); protected {
				continue
			}
			if s.matchesInstalled(lower, installedNames, installedBundleIDs) {
				continue
			}
			size := s.fastDirectorySize(fullPath, orphanFileCap)
			if size.bytes < 15_000_000 {
				continue
			}
			claimedNow[fullPath] = struct{}{}
			leftoverGB := round2(float64(size.bytes) / 1_000_000_000)
			if leftoverGB < 0.01 {
				leftoverGB = 0.01
			}
			found = append(found, map[string]any{
				// Two roots can hold a directory with the same name, so the
				// root is part of the id (S-12).
				"id":             uniqueOrphanID(usedIDs, "orphan_"+target.key+"_"+item),
				"name":           item,
				"category":       target.category,
				"bundleId":       item,
				"statusText":     "主程序已卸载 · 孤立残留",
				"leftoverSizeGb": leftoverGB,
				// A directory without a signature match is only a candidate:
				// it is shown, but the user has to select it (S-06).
				"checked": false,
				"impact":  "经与已安装应用列表比对，该目录对应的主程序未发现，属于卸载后遗留的无主目录；请确认后再勾选。",
				"subpaths": []map[string]any{{
					"path":    s.relativize(fullPath),
					"kind":    target.kind,
					"size":    formatBytes(size.bytes),
					"checked": false,
				}},
			})
			if len(found) >= 18 {
				return found
			}
		}
	}
	return found
}

// matchesInstalled reports whether a directory name belongs to an installed
// app, by bundle identifier or, for a name long enough to be meaningful, by
// name. Matching is exact or a dotted/dashed prefix: the old bidirectional
// substring match treated every directory containing "app" as installed.
func (s *Scanner) matchesInstalled(
	lower string,
	installedNames, installedBundleIDs map[string]struct{},
) bool {
	if _, ok := installedBundleIDs[lower]; ok {
		return true
	}
	if _, ok := installedNames[lower]; ok {
		return true
	}
	for bundleID := range installedBundleIDs {
		if strings.HasPrefix(lower, bundleID+".") || strings.HasPrefix(lower, bundleID+"-") {
			return true
		}
	}
	for name := range installedNames {
		if len(name) < 4 {
			continue
		}
		if strings.HasPrefix(lower, name+".") || strings.HasPrefix(lower, name+"-") || strings.HasPrefix(lower, name+"_") {
			return true
		}
	}
	return false
}
