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
	return s.appendExistingFor(nil, candidates, kind, checked, into)
}

// appendExistingFor is appendExisting for the uninstall of app: the app's own
// personal data (a config row) is no longer protected because the app is
// installed, it becomes a review row that complete uninstall checks.
func (s *Scanner) appendExistingFor(app *uninstallApp, candidates []string, kind string, checked bool, into *[]map[string]any) int64 {
	var total int64
	seen := map[string]bool{}
	for _, candidate := range candidates {
		key := strings.ToLower(filepath.Clean(candidate))
		if seen[key] {
			continue
		}
		seen[key] = true
		size := s.fastDirectorySize(candidate, appLeftoverFileCap)
		if size.bytes <= 0 && !size.partial {
			continue
		}
		// A partial size is a lower bound; it never makes a path protected.
		reason, ownedReason := s.CleanupReason(candidate), ""
		if app != nil && kind == "config" {
			if rule, owner, ok := s.personalRule(candidate); ok && owner.ownedBy(*app) && !s.ownerAbsent(owner, nil) && s.ownerRelaxFor(*app)(rule) {
				if reason = s.cleanupReasonAs(candidate, false, s.ownerRelaxFor(*app)); reason == "" {
					ownedReason = owner.ownedReason()
				}
			}
		}
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
		} else if ownedReason != "" {
			entry["tier"], entry["reason"] = "review", ownedReason
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
		if appDataAlias(component, bundleID) {
			paths = append(paths, filepath.Join(s.home, "Library/Caches", component))
		}
	}
	if bundleID == "com.google.Chrome" {
		paths = append(paths, filepath.Join(s.home, "Library", "Caches", "Google", "Chrome"))
	}
	for _, root := range appSupportRoots(s.home, name, bundleID) {
		paths = append(paths, appSupportCacheCandidates(root, appChromiumBundle(bundleID))...)
	}
	if appDataComponent(bundleID) {
		path := filepath.Join(s.home, "Library/Containers", bundleID, "Data/Library/Caches")
		if info, err := os.Lstat(path); err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			paths = append(paths, path)
		}
	}
	return uniqueAppPaths(paths)
}

var appSupportProducts = map[string]struct {
	path, sharedVendor string
	browser            bool
}{
	"com.google.chrome":            {path: "Google/Chrome", sharedVendor: "Google", browser: true},
	"com.google.chrome.canary":     {path: "Google/Chrome Canary", sharedVendor: "Google", browser: true},
	"com.microsoft.edgemac":        {path: "Microsoft Edge", browser: true},
	"com.brave.browser":            {path: "BraveSoftware/Brave-Browser", sharedVendor: "BraveSoftware", browser: true},
	"company.thebrowser.browser":   {path: "Arc/User Data", browser: true},
	"company.thebrowser.dia":       {path: "Dia/User Data", browser: true},
	"com.citrolabs.ego.lite":       {path: "Citro Labs/ego lite", sharedVendor: "Citro Labs", browser: true},
	"com.citrolabs.ego":            {path: "Citro Labs/ego", sharedVendor: "Citro Labs", browser: true},
	"com.operasoftware.opera":      {path: "com.operasoftware.Opera", browser: true},
	"com.vivaldi.vivaldi":          {path: "Vivaldi", browser: true},
	"com.microsoft.vscode":         {path: "Code"},
	"com.microsoft.vscodeinsiders": {path: "Code - Insiders"},
}

// A display/file name cannot expand a stable product's ownership to its vendor.
// Renaming ego lite.app to Citro Labs.app must not claim EgoUpdater or ego.
func appDataAlias(value, bundleID string) bool {
	product := appSupportProducts[strings.ToLower(bundleID)]
	return appDataComponent(value) && !strings.EqualFold(value, product.sharedVendor)
}

// These roots have a known product owner. A vendor's entire support folder can
// also hold other products and updater state, so only the product subtree is
// considered app configuration or cache.
func appSupportRoots(home, name, bundleID string) []string {
	var roots []string
	if product, known := appSupportProducts[strings.ToLower(bundleID)]; known {
		roots = append(roots, filepath.Join(home, "Library/Application Support", product.path))
	}
	for _, component := range []string{name, bundleID} {
		if appDataAlias(component, bundleID) {
			roots = append(roots, filepath.Join(home, "Library/Application Support", component))
		}
	}
	// Cache discovery checks each root's direct leaves and profiles. Keep a
	// known nested root (Arc/User Data) even when an alias also names its parent.
	seen := map[string]bool{}
	unique := roots[:0]
	for _, root := range roots {
		key := strings.ToLower(filepath.Clean(root))
		if !seen[key] {
			unique = append(unique, root)
			seen[key] = true
		}
	}
	return unique
}

func appChromiumBundle(bundleID string) bool {
	return appSupportProducts[strings.ToLower(bundleID)].browser
}

// Follow Mole's regenerable-marker rule: a generic directory called Cache or
// CachedData is insufficient evidence. Known code/GPU cache leaves establish a
// Chromium/Electron layout. Cookies, IndexedDB, Local Storage, Service Worker
// CacheStorage, extensions and sessions never enter this list.
func appSupportCacheLeaves(root string, browser bool) []string {
	root = filepath.Clean(root)
	var paths []string
	marked := browser
	cacheDirectory := func(leaf string) bool {
		for path := filepath.Join(root, leaf); ; path = filepath.Dir(path) {
			info, err := os.Lstat(path)
			if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return false
			}
			if path == root {
				return true
			}
		}
	}
	for _, leaf := range []string{"Code Cache", "GPUCache", "DawnCache", "GrShaderCache", "ShaderCache", "GraphiteDawnCache", "DawnGraphiteCache", "DawnWebGPUCache", "GPUPersistentCache", "Crashpad/completed", "Service Worker/ScriptCache"} {
		if cacheDirectory(leaf) {
			paths = append(paths, filepath.Join(root, leaf))
			marked = true
		}
	}
	if marked {
		for _, leaf := range []string{"Cache", "CachedData"} {
			if cacheDirectory(leaf) {
				paths = append(paths, filepath.Join(root, leaf))
			}
		}
	}
	if browser {
		for _, leaf := range []string{"component_crx_cache", "extensions_crx_cache"} {
			if cacheDirectory(leaf) {
				paths = append(paths, filepath.Join(root, leaf))
			}
		}
	}
	return paths
}

func appProfileName(name string) bool {
	if name == "Default" || name == "Guest Profile" || name == "System Profile" {
		return true
	}
	if !strings.HasPrefix(name, "Profile ") || len(name) == len("Profile ") {
		return false
	}
	for _, digit := range name[len("Profile "):] {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func appSupportCacheCandidates(root string, browser bool) []string {
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	paths := appSupportCacheLeaves(root, browser)
	entries, err := readDirEntries(root)
	if err != nil {
		return paths
	}
	if len(entries) > 1000 {
		recordReadWarning(root, "应用配置目录超过扫描上限；只检查已识别的直接缓存路径")
		return paths
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if appProfileName(entry.Name()) {
			paths = append(paths, appSupportCacheLeaves(path, browser)...)
		}
		if entry.Name() == "Partitions" {
			partitions, err := readDirEntries(path)
			if err != nil {
				continue
			}
			if len(partitions) > 1000 {
				recordReadWarning(path, "应用配置目录超过扫描上限；只检查已识别的直接缓存路径")
				continue
			}
			for _, partition := range partitions {
				if partition.IsDir() && partition.Type()&os.ModeSymlink == 0 {
					paths = append(paths, appSupportCacheLeaves(filepath.Join(path, partition.Name()), browser)...)
				}
			}
		}
	}
	return uniqueAppPaths(paths)
}

func uniqueAppPaths(paths []string) []string {
	var unique []string
	for _, path := range paths {
		covered := false
		for _, existing := range unique {
			if withinPath(path, existing) {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		kept := unique[:0]
		for _, existing := range unique {
			if !withinPath(existing, path) {
				kept = append(kept, existing)
			}
		}
		unique = append(kept, path)
	}
	return unique
}

// Bundle metadata is not a filesystem path or permission to leave these roots.
func appDataComponent(value string) bool {
	return value != "" && value != "." && value != ".." && !filepath.IsAbs(value) && filepath.Base(value) == value && !strings.ContainsRune(value, 0)
}

func (s *Scanner) appConfigCandidates(name, bundleID string) []string {
	paths := appSupportRoots(s.home, name, bundleID)
	if appDataComponent(bundleID) {
		paths = append(paths, filepath.Join(s.home, "Library/Containers", bundleID),
			filepath.Join(s.home, "Library/Saved Application State", bundleID+".savedState"),
			filepath.Join(s.home, "Library/Preferences", bundleID+".plist"))
	}
	return uniqueAppPaths(paths)
}

// Split only ancestors of measured cache leaves. A config path must never
// contain a separately selectable cache: that counted the bytes twice and let
// complete-uninstall selections contain both a parent and its child.
func partitionAppConfig(root string, caches []string) []string {
	for _, cache := range caches {
		if withinPath(root, cache) {
			return nil
		}
	}
	containsCache := false
	for _, cache := range caches {
		containsCache = containsCache || withinPath(cache, root)
	}
	if !containsCache {
		return []string{root}
	}
	entries, err := readDirEntries(root)
	if err != nil {
		return nil
	}
	var paths []string
	for _, entry := range entries {
		paths = append(paths, partitionAppConfig(filepath.Join(root, entry.Name()), caches)...)
	}
	return paths
}

// defaultAppDirs are the folders scanInstalledApps reads when given none.
// machineWide is false in tests: a verdict then depends only on the scanner's
// home, never on what the Mac running the tests has installed.
var machineWide = true

func (s *Scanner) defaultAppDirs() []string {
	if s.appDirs != nil {
		return s.appDirs
	}
	home := []string{filepath.Join(s.home, "Applications"), filepath.Join(s.home, "Applications/Utilities")}
	if !machineWide {
		return home
	}
	return append([]string{"/Applications", "/Applications/Utilities"}, home...)
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
			needsAdmin := bundleNeedsAdmin(fullPath)
			if needsAdmin && subpaths[0]["tier"] == nil {
				subpaths[0]["tier"], subpaths[0]["reason"], subpaths[0]["checked"] = "protected", appNeedsAdminReason, false
			}
			if size.partial && appBytes == 0 {
				subpaths[0]["size"], subpaths[0]["sizeGb"] = "未读取", nil
			}
			var cacheCandidates, configCandidates []string
			for _, alias := range []string{name, info.bundleName, strings.TrimSuffix(item, ".app")} {
				cacheCandidates = append(cacheCandidates, s.appCacheCandidates(alias, bundleID)...)
				configCandidates = append(configCandidates, s.appConfigCandidates(alias, bundleID)...)
			}
			cacheCandidates = uniqueAppPaths(cacheCandidates)
			configCandidates = uniqueAppPaths(configCandidates)
			// Report the measured cache and configuration sizes separately
			// instead of splitting the leftover total with a made-up 80/20
			// ratio; a group that was not found reports zero.
			self := &uninstallApp{path: fullPath, name: name, bundleID: bundleID}
			cacheBytes := s.appendExistingFor(self, cacheCandidates, "cache", true, &subpaths)
			var measuredCaches []string
			for _, subpath := range subpaths {
				if subpath["kind"] == "cache" {
					measuredCaches = append(measuredCaches, s.expand(subpath["path"].(string)))
				}
			}
			var configPaths []string
			for _, root := range configCandidates {
				configPaths = append(configPaths, partitionAppConfig(root, measuredCaches)...)
			}
			configBytes := s.appendExistingFor(self, uniqueAppPaths(configPaths), "config", false, &subpaths)
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
				"needsAdmin":     needsAdmin,
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
