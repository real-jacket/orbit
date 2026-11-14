package scanner

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// verdict is the recommendation for one path.
type verdict struct{ tier, reason, unlock string }

// appOwner ties an app's aliases and precise user-data roots to its bundle.
type appOwner struct {
	key, name, path string
	roots           []string
}

var (
	appOwnerMu    sync.Mutex
	appOwnerCache = map[string][]appOwner{}
	appOwnerAt    = map[string]time.Time{}
)

// installedAppOwners lists user-installed apps by every name a cache folder may
// use: bundle identifier, display name, bundle name and file name. A scan-scoped
// scanner (processTTL > 0) reuses the list briefly; tests and the move boundary
// always read it fresh.
func (s *Scanner) installedAppOwners(live bool) []appOwner {
	dirs := s.defaultAppDirs()
	if s.appDirs == nil {
		dirs = append(dirs, filepath.Join(s.home, "Library/Application Support/Setapp/Applications"))
	}
	cacheKey := s.home + "\x00installed\x00" + strings.Join(dirs, "\x00")
	if !live && s.processTTL > 0 {
		appOwnerMu.Lock()
		owners, at := appOwnerCache[cacheKey], appOwnerAt[cacheKey]
		appOwnerMu.Unlock()
		if time.Since(at) < 10*time.Second {
			return owners
		}
	}
	var owners []appOwner
	for _, path := range findAppBundles(dirs) {
		owners = append(owners, s.appOwnersFor(path, true)...)
	}
	if !live && s.processTTL > 0 {
		appOwnerMu.Lock()
		appOwnerCache[cacheKey], appOwnerAt[cacheKey] = owners, time.Now()
		appOwnerMu.Unlock()
	}
	return owners
}

// The same metadata indexes installed caches and data used by running helpers.
// Reuse the existing short-lived owner cache for display checks; live checks
// always reread the bundle, including apps outside the usual Applications roots.
func (s *Scanner) appOwnersFor(path string, live bool) []appOwner {
	cacheKey := s.home + "\x00bundle\x00" + path
	if !live && s.processTTL > 0 {
		appOwnerMu.Lock()
		owners, at := appOwnerCache[cacheKey], appOwnerAt[cacheKey]
		appOwnerMu.Unlock()
		if time.Since(at) < 10*time.Second {
			return owners
		}
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	info := readBundleInfo(path)
	if info.err != nil && !errors.Is(info.err, os.ErrNotExist) {
		return nil
	}
	display := firstNonEmpty(info.displayName, info.bundleName, name)
	aliases := []string{info.identifier, info.displayName, info.bundleName, name}
	var roots []string
	for _, alias := range aliases {
		roots = append(roots, s.appConfigCandidates(alias, info.identifier)...)
		if appDataAlias(alias, info.identifier) {
			roots = append(roots, filepath.Join(s.home, "Library/Caches", alias))
		}
	}
	if strings.EqualFold(info.identifier, "com.google.Chrome") {
		roots = append(roots, filepath.Join(s.home, "Library/Caches/Google/Chrome"))
	}
	roots = uniqueAppPaths(roots)
	var owners []appOwner
	for _, alias := range aliases {
		if alias != "" {
			owners = append(owners, appOwner{key: strings.ToLower(alias), name: display, path: path, roots: roots})
		}
	}
	if !live && s.processTTL > 0 {
		appOwnerMu.Lock()
		appOwnerCache[cacheKey], appOwnerAt[cacheKey] = owners, time.Now()
		appOwnerMu.Unlock()
	}
	return owners
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// matchAppOwner finds the installed app a cache folder name belongs to: an exact
// key, or a dotted/dashed continuation of a key long enough to be meaningful
// ("com.foo.app.ShipIt", "slack-updater"). The longest key wins.
func matchAppOwner(lower string, owners []appOwner) (appOwner, bool) {
	var best appOwner
	for _, owner := range owners {
		match := lower == owner.key
		if !match && len(owner.key) >= 4 {
			for _, sep := range []string{".", "-", "_"} {
				match = match || strings.HasPrefix(lower, owner.key+sep)
			}
		}
		if match && len(owner.key) > len(best.key) {
			best = owner
		}
	}
	return best, best.key != ""
}

// cacheOwnerRunningReason blocks a direct child of ~/Library/Caches while the
// app that owns it is running; the cache of a live app is in use.
func (s *Scanner) cacheOwnerRunningReason(path string, live bool) string {
	if filepath.Dir(path) != filepath.Join(s.home, "Library/Caches") {
		return ""
	}
	owner, ok := matchAppOwner(strings.ToLower(filepath.Base(path)), s.installedAppOwners(live))
	if !ok {
		return ""
	}
	processes, err := s.processSnapshot(live)
	if err != nil || strings.TrimSpace(processes) == "" {
		return "无法确认应用是否运行，保持原状"
	}
	if running, _ := appRunning(parseProcessLines(processes), owner.path); running {
		return owner.name + " 正在运行，暂缓清理"
	}
	return ""
}

// Application Support caches and sandbox data have the same running owner as
// that owner's direct Library/Caches entries. Every enclosing .app counts: a
// helper alone still uses the outer app's product data. The move boundary also
// checks open files for unrecognized or shared data such as Group Containers.
func (s *Scanner) appDataCleanupReason(path string, live bool) string {
	appData := false
	for _, family := range []string{"Library/Caches", "Library/Application Support", "Library/Containers", "Library/Group Containers", "Library/Preferences", "Library/Saved Application State"} {
		appData = appData || withinPath(path, filepath.Join(s.home, family))
	}
	if !appData {
		return ""
	}
	owners, err := s.runningAppOwners(live)
	if err != nil {
		return "无法确认应用数据是否闲置，保持原状"
	}
	lowerPath := strings.ToLower(filepath.Clean(path))
	for _, owner := range owners {
		for _, root := range owner.roots {
			if withinPath(lowerPath, root) || withinPath(root, lowerPath) {
				return owner.name + " 正在运行，暂缓清理"
			}
		}
	}
	if live {
		open, err := s.aiHandleOpen(path, true)
		if err != nil {
			return "无法确认应用数据是否仍被打开，保持原状"
		}
		if open {
			return "仍有进程打开这份应用数据，保持原状"
		}
	}
	return ""
}

func (s *Scanner) runningAppOwners(live bool) ([]appOwner, error) {
	if !live && s.runningAppSnapshot != nil {
		return s.runningAppSnapshot()
	}
	processes, err := s.processSnapshot(live)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(processes) == "" {
		return nil, errors.New("empty process table")
	}
	var running []appOwner
	seen := map[string]bool{}
	for _, process := range parseProcessLines(processes) {
		for app := process.path; app != "/" && app != "."; app = filepath.Dir(app) {
			if !strings.EqualFold(filepath.Ext(app), ".app") {
				continue
			}
			key := strings.ToLower(app)
			if seen[key] {
				continue
			}
			seen[key] = true
			owners := s.appOwnersFor(app, live)
			if len(owners) == 0 {
				return nil, errors.New("running app metadata unavailable")
			}
			owner := owners[0]
			owner.roots = append([]string(nil), owner.roots...)
			for i, root := range owner.roots {
				owner.roots[i] = strings.ToLower(root)
			}
			running = append(running, owner)
		}
	}
	return running, nil
}

// regenerableCaches are tool caches that rebuild on demand, whether or not an
// app owns them.
var regenerableCaches = map[string]string{
	"go-build": "Go 编译缓存", "typescript": "TypeScript 类型缓存", "node-gyp": "node-gyp 头文件缓存",
	"electron": "Electron 下载缓存", "pip": "pip 下载缓存",
	"homebrew": "Homebrew 下载缓存", "bun": "Bun 下载缓存", "yarn": "Yarn 下载缓存", "pnpm": "pnpm 下载缓存",
	"cocoapods": "CocoaPods 缓存", "org.swift.swiftpm": "SwiftPM 缓存", "pypoetry": "Poetry 缓存",
	"pre-commit": "pre-commit 环境缓存", "puppeteer": "Puppeteer 下载缓存",
	"prisma": "Prisma 引擎缓存", "jetbrains": "JetBrains 索引缓存", "sentrycrash": "崩溃上报缓存",
	// Library/Caches entries Mole removes.
	// Ported from tw93/Mole V1.70.0 lib/clean/user.sh
	// Ported from tw93/Mole V1.70.0 lib/clean/dev.sh
	// Ported from tw93/Mole V1.70.0 lib/clean/app_caches.sh
	"com.apple.dt.xcode": "Xcode 缓存", "com.apple.python": "macOS 自带 Python 的缓存", "com.apple.wallpaper.caches": "壁纸缓存",
	"com.apple.webkit.networking": "WebKit 网络缓存", "com.apple.helpd": "帮助查看器缓存", "com.apple.parsecd": "Siri 建议缓存",
	"com.apple.duetexpertd": "系统建议缓存", "com.apple.photoanalysisd": "照片分析缓存", "com.apple.applemediaservices": "媒体服务缓存",
	"com.apple.music": "音乐缓存", "com.apple.podcasts": "播客缓存", "com.apple.tv": "Apple TV 缓存", "com.apple.finalcut": "Final Cut Pro 缓存",
	"composer": "Composer 下载缓存", "curl": "curl 缓存", "lima": "Lima 下载缓存", "com.crashlytics.data": "崩溃上报缓存",
}

// regenerablePrefixes are Apple cache families named with a version or id suffix.
var regenerablePrefixes = map[string]string{
	"com.apple.iconservices": "图标服务缓存", "com.apple.iwork.": "iWork 缓存",
}

// notCaches are folders under Caches that hold account, sync or communication
// state; they stay as they are.
var notCaches = []string{
	"keychain", "cloudkit", "icloud", "clouddocs", "com.apple.bird", "cloudd", "familycircle", "accounts",
	"com.apple.mail", "photos", "mobilesms", "messages", "imessage", "addressbook", "contacts", "passwords",
	"homekit", "homed", "healthd", "com.apple.containermanagerd", "coreaudio", "com.apple.audio.",
}

// offlineContent are apps that keep downloaded or synced content in Caches.
var offlineContent = []string{"spotify", "netflix", "dropbox", "onedrive", "boxsync", "synology", "megasync"}

// cacheVerdict classifies one child of ~/Library/Caches or ~/Library/Logs.
// Whether the owning app or tool is running is decided afterwards by the live
// safety check, which demotes the row to protected with an unlock hint.
func (s *Scanner) cacheVerdict(path string, size dirSize) verdict {
	name := filepath.Base(path)
	lower := strings.ToLower(name)
	if withinPath(path, filepath.Join(s.home, "Library/Logs")) {
		if size.modified > 0 && nowUnix()-size.modified >= 30*86400 {
			return verdict{tier: "safe", reason: "超过 30 天没有新内容的诊断日志，只影响回看旧排障记录，新日志会自动生成"}
		}
		return verdict{tier: "review", reason: "近 30 天内仍有写入的诊断日志；移除后无法回看这段排障记录，新日志会自动生成"}
	}
	for _, word := range notCaches {
		if strings.Contains(lower, word) {
			return verdict{tier: "protected", reason: "账户、同步或通信相关数据，不属于普通缓存，保持原状"}
		}
	}
	for _, word := range offlineContent {
		if strings.Contains(lower, word) {
			return verdict{tier: "review", reason: "该应用可能把离线下载或同步内容放在缓存里，清理后需要重新下载，请确认"}
		}
	}
	if what, ok := regenerableCaches[lower]; ok {
		return verdict{tier: "safe", reason: what + "，会在下次使用时自动重新生成"}
	}
	for prefix, what := range regenerablePrefixes {
		if strings.HasPrefix(lower, prefix) {
			return verdict{tier: "safe", reason: what + "，会在下次使用时自动重新生成"}
		}
	}
	if strings.Contains(lower, "-updater") || strings.HasSuffix(lower, ".shipit") || strings.HasSuffix(lower, "updater") {
		return verdict{tier: "safe", reason: "应用更新包的下载缓存，下次检查更新时会自动重新下载"}
	}
	if strings.HasPrefix(lower, "com.apple.") {
		return verdict{tier: "review", reason: "macOS 系统组件缓存：通常可重建，但清理后系统应用首次打开可能变慢"}
	}
	if owner, ok := matchAppOwner(lower, s.installedAppOwners(false)); ok {
		return verdict{tier: "safe", reason: owner.name + " 的缓存，应用当前未运行；下次打开时自动重新生成，个别数据可能需要重新加载"}
	}
	return verdict{tier: "review", reason: "未识别的缓存：通常可重建，但相关应用可能需要重新下载数据或登录"}
}
