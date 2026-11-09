package scanner

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const sharedRemnantsReason = "另一份相同标识的应用仍安装在这台 Mac 上，这些数据还在被它使用"

var versionSuffixPattern = regexp.MustCompile(`^(.+)\s+(Nightly|Beta|Alpha|Dev|Canary|Preview|Insider|Edge|Stable|Release|RC|LTS|Developer Edition|Technology Preview)$`)

// Ported from tw93/Mole V1.70.0 lib/uninstall/batch.sh uninstall_bundles_share_remnants
// Two installed bundles share user data when their bundle ids are equal or one
// extends the other, or when their names match once a channel suffix is cut.
// shortcut: Mole also tells JetBrains products apart by their selector; orbit
// treats them as sharing, so their data stays protected, until that is needed.
func bundlesShareRemnants(selectedID, otherID, selectedPath, otherPath string) bool {
	a, b := strings.ToLower(selectedID), strings.ToLower(otherID)
	if a == b || strings.HasPrefix(a, b+".") || strings.HasPrefix(b, a+".") {
		return true
	}
	stem := func(path string) string {
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if m := versionSuffixPattern.FindStringSubmatch(name); m != nil {
			name = m[1]
		}
		return name
	}
	x, y := stem(selectedPath), stem(otherPath)
	return len(x) >= 2 && len(y) >= 2 && strings.EqualFold(x, y)
}

// protectSharedRemnants keeps the cache, config and launch entries of an app
// out of the removal selection while another installed copy still uses them.
// The app bundle itself stays selectable.
func protectSharedRemnants(results []map[string]any) {
	for i, row := range results {
		for j, other := range results {
			if i == j || row["appPath"] == other["appPath"] ||
				!bundlesShareRemnants(row["bundleId"].(string), other["bundleId"].(string), row["appPath"].(string), other["appPath"].(string)) {
				continue
			}
			for _, sub := range row["subpaths"].([]map[string]any) {
				if sub["kind"] == "app" || sub["tier"] == "protected" {
					continue
				}
				sub["tier"], sub["reason"], sub["checked"] = "protected", sharedRemnantsReason, false
			}
			break
		}
	}
}

// appendExisting measures known app paths, keeping protected entries visible
// but out of the removal selection.
func (s *Scanner) appendExisting(candidates []string, kind string, checked bool, into *[]map[string]any) int64 {
	var total int64
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		size := s.fastDirectorySize(candidate, appLeftoverFileCap)
		if size.bytes <= 0 && !size.partial {
			continue
		}
		// A partial size is a lower bound; it never makes a path protected.
		reason := s.CleanupReason(candidate)
		total += size.bytes
		entry := map[string]any{
			"path":    s.relativize(candidate),
			"size":    formatBytes(size.bytes),
			"sizeGb":  plainGB(size.bytes),
			"kind":    kind,
			"checked": checked && reason == "",
			"partial": size.partial,
		}
		if reason != "" {
			entry["tier"], entry["reason"] = "protected", reason
		}
		if size.partial && size.bytes == 0 {
			entry["size"], entry["sizeGb"] = "未读取", nil
		}
		*into = append(*into, entry)
	}
	return total
}

func (s *Scanner) appCacheCandidates(name, bundleID string) []string {
	var paths []string
	for _, component := range []string{bundleID, name} {
		if appDataComponent(component) {
			paths = append(paths, filepath.Join(s.home, "Library/Caches", component))
		}
	}
	if bundleID == "com.google.Chrome" {
		paths = append(paths, filepath.Join(s.home, "Library", "Caches", "Google", "Chrome"))
	}
	return paths
}

// Bundle metadata is not a filesystem path or permission to leave these roots.
func appDataComponent(value string) bool {
	return value != "" && value != "." && value != ".." && !filepath.IsAbs(value) && filepath.Base(value) == value && !strings.ContainsRune(value, 0)
}

func (s *Scanner) appConfigCandidates(name, bundleID string) []string {
	var paths []string
	for _, component := range []string{name, bundleID} {
		if appDataComponent(component) {
			paths = append(paths, filepath.Join(s.home, "Library/Application Support", component))
		}
	}
	if appDataComponent(bundleID) {
		paths = append(paths, filepath.Join(s.home, "Library/Containers", bundleID),
			filepath.Join(s.home, "Library/Saved Application State", bundleID+".savedState"),
			filepath.Join(s.home, "Library/Preferences", bundleID+".plist"))
	}
	return paths
}

// defaultAppDirs are the folders scanInstalledApps reads when given none.
func (s *Scanner) defaultAppDirs() []string {
	if s.appDirs != nil {
		return s.appDirs
	}
	return []string{"/Applications", "/Applications/Utilities", filepath.Join(s.home, "Applications"), filepath.Join(s.home, "Applications/Utilities")}
}

// scanInstalledApps lists application bundles without truncating before sorting.
func (s *Scanner) scanInstalledApps(appDirs ...string) []map[string]any {
	results := []map[string]any{}
	launchItems := s.loadLaunchItems()
	procs := s.runningProcesses()
	if len(appDirs) == 0 {
		appDirs = s.defaultAppDirs()
	}
	for _, dir := range appDirs {
		items, err := readDirEntries(dir)
		if err != nil {
			continue
		}
		for _, entry := range items {
			item := entry.Name()
			if !strings.EqualFold(filepath.Ext(item), ".app") {
				continue
			}
			if strings.Contains(item, "MoleStudio") || item == "orbit.app" || item == "Safari.app" {
				continue
			}
			fullPath := filepath.Join(dir, item)
			if _, protected := s.scanProtection(fullPath); protected {
				continue
			}
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
			size := s.fastDirectorySize(fullPath, appBundleFileCap)
			appBytes := size.bytes

			subpaths := []map[string]any{{
				"path":    fullPath,
				"size":    formatBytes(appBytes),
				"sizeGb":  plainGB(appBytes),
				"kind":    "app",
				"checked": true,
				"partial": size.partial,
			}}
			if reason := s.CleanupReason(fullPath); reason != "" {
				subpaths[0]["tier"], subpaths[0]["reason"], subpaths[0]["checked"] = "protected", reason, false
			}
			if size.partial && appBytes == 0 {
				subpaths[0]["size"], subpaths[0]["sizeGb"] = "未读取", nil
			}
			cacheCandidates := s.appCacheCandidates(name, bundleID)
			configCandidates := s.appConfigCandidates(name, bundleID)
			// Report the measured cache and configuration sizes separately
			// instead of splitting the leftover total with a made-up 80/20
			// ratio; a group that was not found reports zero.
			cacheBytes := s.appendExisting(cacheCandidates, "cache", true, &subpaths)
			configBytes := s.appendExisting(configCandidates, "config", false, &subpaths)
			// Launch items travel with the app but are not part of the cache or
			// config totals. Root-owned ones are refused by CleanupReason.
			launchPaths := launchPathsFor(launchItems, fullPath, bundleID)
			s.appendExisting(launchPaths, "launch", true, &subpaths)
			impact := "仅把勾选的应用、缓存或配置路径移入废纸篓；受保护的路径保留。"
			if len(launchPaths) > 0 {
				impact += "勾选的启动项移入废纸篓前，会先停止它在当前用户下的后台服务；系统级启动项需要管理员权限，只列出不处理。"
			}
			running, mainProc := appRunning(procs, fullPath)
			cacheGB := plainGB(cacheBytes)
			configGB := plainGB(configBytes)
			leftoverGB := cacheGB + configGB

			results = append(results, map[string]any{
				"id":       fullPath,
				"name":     name,
				"category": "Mac 应用",
				"appPath":  fullPath,
				// The bundle's own artwork, served by IconHandler: drawing a
				// made-up squircle for an installed app made the list look fake.
				"icon":           appIconURL(fullPath),
				"bundleId":       bundleID,
				"version":        version,
				"partial":        size.partial,
				"running":        running,
				"appSizeGb":      plainGB(appBytes),
				"leftoverSizeGb": leftoverGB,
				"cacheSizeGb":    cacheGB,
				"configSizeGb":   configGB,
				// Nothing is pre-selected: uninstalling an app is not a
				// safe-by-default action.
				"checked":      false,
				"keepPersonal": true,
				"impact":       impact,
				"subpaths":     subpaths,
			})
			if running && mainProc.pid > 0 {
				results[len(results)-1]["runningPid"], results[len(results)-1]["runningPath"] = mainProc.pid, mainProc.path
			}
		}
	}

	protectSharedRemnants(results)
	appPaths := make([]string, len(results))
	for i, row := range results {
		appPaths[i] = row["appPath"].(string)
	}
	for i, used := range appLastUsed(appPaths) {
		// Unknown stays unknown: no date is invented for an app Spotlight never saw.
		results[i]["lastUsedAt"], results[i]["lastUsed"] = nil, "未记录"
		if used != nil {
			results[i]["lastUsedAt"], results[i]["lastUsed"] = used.Unix(), used.Format("2006-01-02")
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

// readBundleInfo reads an app bundle's Info.plist, the three keys the uninstall
// list shows. A bundle whose plist is missing or unreadable reports nothing.
func readBundleInfo(path string) bundleInfo {
	props, err := parsePlistFile(filepath.Join(path, "Contents", "Info.plist"))
	if err != nil {
		// Ported from tw93/Mole V1.70.0 lib/clean/apps.sh scan_installed_apps: an iOS app on
		// Apple Silicon keeps its plist under Wrapper/<name>.app, not Contents/.
		wrapped, _ := filepath.Glob(filepath.Join(path, "Wrapper", "*.app", "Info.plist"))
		if len(wrapped) == 0 {
			return bundleInfo{}
		}
		if props, err = parsePlistFile(wrapped[0]); err != nil {
			return bundleInfo{}
		}
	}
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

// installedAppIndex collects the names and bundle identifiers of every app the
// machine reports as installed, which decides whether a leftover directory
// still has an owner. System apps and Homebrew formulas count as installed too,
// otherwise their support folders were offered as "leftovers".
func (s *Scanner) installedAppIndex(home string) (names, bundleIDs map[string]struct{}) {
	names = map[string]struct{}{}
	bundleIDs = map[string]struct{}{}
	appDirs := []string{
		"/Applications",
		"/System/Applications",
		filepath.Join(home, "Applications"),
		"/opt/homebrew/Caskroom",
		"/usr/local/Caskroom",
		filepath.Join(home, "Library/Application Support/Setapp/Applications"),
	}
	for _, path := range findAppBundles(appDirs) {
		names[strings.ToLower(strings.TrimSuffix(baseName(path), filepath.Ext(path)))] = struct{}{}
		info := readBundleInfo(path)
		for _, value := range []string{info.identifier, info.displayName, info.bundleName} {
			if value != "" {
				bundleIDs[strings.ToLower(value)] = struct{}{}
			}
		}
	}
	// Ported from tw93/Mole V1.70.0 lib/clean/apps.sh scan_installed_apps: a login item's
	// LaunchAgent label also names an installed owner.
	for _, path := range s.matchingWildcardPaths(filepath.Join(home, "Library/LaunchAgents"), "") {
		if label, ok := strings.CutSuffix(baseName(path), ".plist"); ok && label != "" {
			bundleIDs[strings.ToLower(label)] = struct{}{}
		}
	}
	// Command line tools of Homebrew and /usr/local have no bundle, but their
	// support folders must not be reported as orphaned.
	for _, dir := range []string{"/opt/homebrew/bin", "/usr/local/bin"} {
		for _, path := range s.matchingWildcardPaths(dir, "") {
			name := strings.ToLower(baseName(path))
			if name == "" || strings.HasPrefix(name, ".") {
				continue
			}
			names[name] = struct{}{}
		}
	}
	// Homebrew formulas whose binaries are linked outside bin, and casks whose
	// app lives inside the Cellar/Caskroom.
	for _, dir := range []string{"/opt/homebrew/Cellar", "/usr/local/Cellar", "/opt/homebrew/Caskroom", "/usr/local/Caskroom"} {
		for _, path := range s.matchingWildcardPaths(dir, "") {
			name := strings.ToLower(baseName(path))
			if name == "" || strings.HasPrefix(name, ".") {
				continue
			}
			names[name] = struct{}{}
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
