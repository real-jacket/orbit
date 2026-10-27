package scanner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// appendExisting appends a subpath entry for each candidate that exists and is
// not empty, and reports how large they are together.
func (s *Scanner) appendExisting(candidates []string, kind string, checked bool, into *[]map[string]any) int64 {
	var total int64
	for _, candidate := range candidates {
		if !exists(candidate) {
			continue
		}
		size := s.fastDirectorySize(candidate, 600)
		if size.bytes <= 0 {
			continue
		}
		total += size.bytes
		*into = append(*into, map[string]any{
			"path":    s.relativize(candidate),
			"size":    formatBytes(size.bytes),
			"kind":    kind,
			"checked": checked,
		})
	}
	return total
}

// scanInstalledApps lists the apps in /Applications and ~/Applications, with
// what each one left behind in ~/Library.
func (s *Scanner) scanInstalledApps() []map[string]any {
	results := []map[string]any{}
	appDirs := []string{"/Applications", filepath.Join(s.home, "Applications")}
	home := s.home

	for _, dir := range appDirs {
		items, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range items {
			item := entry.Name()
			if !strings.HasSuffix(item, ".app") {
				continue
			}
			if strings.Contains(item, "MoleStudio") || item == "Safari.app" {
				continue
			}
			fullPath := filepath.Join(dir, item)
			info := readBundleInfo(fullPath)
			name := info.displayName
			if name == "" {
				name = info.bundleName
			}
			if name == "" {
				name = strings.TrimSuffix(item, ".app")
			}
			bundleID := info.identifier
			if bundleID == "" {
				bundleID = "local.app." + name
			}
			version := info.shortVersion
			if version == "" {
				version = "1.0"
			}
			size := s.fastDirectorySize(fullPath, 1200)
			appGB := float64(size.bytes) / 1_000_000_000
			if appGB < 0.05 {
				appGB = 0.05
			}

			appSize := size.bytes
			if appSize < 50_000_000 {
				appSize = 50_000_000
			}
			subpaths := []map[string]any{{
				"path":    fullPath,
				"size":    formatBytes(appSize),
				"kind":    "app",
				"checked": true,
			}}
			var leftoverBytes int64
			cacheCandidates := []string{
				home + "/Library/Caches/" + bundleID,
				home + "/Library/Caches/" + name,
			}
			configCandidates := []string{
				home + "/Library/Application Support/" + name,
				home + "/Library/Application Support/" + bundleID,
				home + "/Library/Containers/" + bundleID,
				home + "/Library/Saved Application State/" + bundleID + ".savedState",
				home + "/Library/Preferences/" + bundleID + ".plist",
			}
			leftoverBytes += s.appendExisting(cacheCandidates, "cache", true, &subpaths)
			leftoverBytes += s.appendExisting(configCandidates, "config", false, &subpaths)

			leftoverGB := float64(leftoverBytes) / 1_000_000_000
			cacheGB := leftoverGB * 0.8
			configGB := leftoverGB - cacheGB
			if configGB < 0 {
				configGB = 0
			}
			results = append(results, map[string]any{
				"id":             bundleID,
				"name":           name,
				"category":       "Mac 应用",
				"appPath":        fullPath,
				"bundleId":       bundleID,
				"version":        version,
				"lastUsed":       "本机已安装",
				"appSizeGb":      round2(appGB),
				"leftoverSizeGb": round2(leftoverGB),
				"cacheSizeGb":    round2(cacheGB),
				"configSizeGb":   round2(configGB),
				"checked":        len(results) == 0,
				"keepPersonal":   true,
				"impact": "卸载时将把 " + fullPath + " 及 " +
					itoa64(int64(len(subpaths)-1)) + " 处 ~/Library 关联缓存移入废纸篓 (~/.Trash)。",
				"subpaths": subpaths,
			})
			if len(results) >= 16 {
				break
			}
		}
		if len(results) >= 16 {
			break
		}
	}

	sort.SliceStable(results, func(i, j int) bool {
		left := floatOf(results[i]["appSizeGb"]) + floatOf(results[i]["leftoverSizeGb"])
		right := floatOf(results[j]["appSizeGb"]) + floatOf(results[j]["leftoverSizeGb"])
		return left > right
	})
	return results
}

// bundleInfo is what the app reads from an app bundle's Info.plist.
type bundleInfo struct {
	displayName  string
	bundleName   string
	identifier   string
	shortVersion string
}

// bundleInfo reads an app bundle's Info.plist, the three keys the uninstall
// list shows. A bundle whose plist is missing or unreadable reports nothing.
func readBundleInfo(path string) bundleInfo {
	data, err := os.ReadFile(filepath.Join(path, "Contents", "Info.plist"))
	if err != nil {
		return bundleInfo{}
	}
	props := parsePlist(data)
	return bundleInfo{
		displayName:  props["CFBundleDisplayName"],
		bundleName:   props["CFBundleName"],
		identifier:   props["CFBundleIdentifier"],
		shortVersion: props["CFBundleShortVersionString"],
	}
}

// matchingWildcardPaths lists the entries of parentDir whose name starts with
// prefix, then makes them full paths. An empty prefix matches everything.
func (s *Scanner) matchingWildcardPaths(parentDir, prefix string) []string {
	expanded := s.expand(parentDir)
	items, err := os.ReadDir(expanded)
	if err != nil {
		return nil
	}
	lowerPrefix := strings.ToLower(prefix)
	var paths []string
	for _, entry := range items {
		name := entry.Name()
		if prefix != "" && !strings.HasPrefix(strings.ToLower(name), lowerPrefix) {
			continue
		}
		paths = append(paths, filepath.Join(expanded, name))
	}
	return paths
}

// installedAppIndex collects the names and bundle identifiers of every app in
// /Applications, which decides whether a leftover directory still has an owner.
func (s *Scanner) installedAppIndex(home string) (names, bundleIDs map[string]struct{}) {
	names = map[string]struct{}{}
	bundleIDs = map[string]struct{}{}
	dirs := []string{"/Applications", filepath.Join(home, "Applications")}
	for _, dir := range dirs {
		items, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range items {
			item := entry.Name()
			if !strings.HasSuffix(item, ".app") {
				continue
			}
			names[strings.ToLower(strings.TrimSuffix(item, ".app"))] = struct{}{}
			info := readBundleInfo(filepath.Join(dir, item))
			for _, value := range []string{info.identifier, info.displayName, info.bundleName} {
				if value != "" {
					bundleIDs[strings.ToLower(value)] = struct{}{}
				}
			}
		}
	}
	return names, bundleIDs
}

// floatOf reads a float from a payload value.
func floatOf(v any) float64 {
	switch value := v.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	}
	return 0
}

// intOf reads an int from a payload value.
func intOf(v any) int {
	switch value := v.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	}
	return 0
}
