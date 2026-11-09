package scanner

import (
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// verdict is the recommendation for one path.
type verdict struct{ tier, reason, unlock string }

// appOwner is an installed app a cache directory can belong to.
type appOwner struct{ key, name, path string }

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
	if !live && s.processTTL > 0 {
		appOwnerMu.Lock()
		defer appOwnerMu.Unlock()
		if time.Since(appOwnerAt[s.home]) < 10*time.Second {
			return appOwnerCache[s.home]
		}
	}
	var owners []appOwner
	for _, path := range findAppBundles([]string{"/Applications", filepath.Join(s.home, "Applications"), filepath.Join(s.home, "Library/Application Support/Setapp/Applications")}) {
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		info := readBundleInfo(path)
		display := firstNonEmpty(info.displayName, info.bundleName, name)
		for _, key := range []string{info.identifier, info.displayName, info.bundleName, name} {
			if key != "" {
				owners = append(owners, appOwner{key: strings.ToLower(key), name: display, path: path})
			}
		}
	}
	if !live && s.processTTL > 0 {
		appOwnerCache[s.home], appOwnerAt[s.home] = owners, time.Now()
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

// regenerableCaches are tool caches that rebuild on demand, whether or not an
// app owns them.
var regenerableCaches = map[string]string{
	"go-build": "Go 编译缓存", "typescript": "TypeScript 类型缓存", "node-gyp": "node-gyp 头文件缓存",
	"electron": "Electron 下载缓存", "ms-playwright": "Playwright 浏览器下载缓存", "pip": "pip 下载缓存",
	"homebrew": "Homebrew 下载缓存", "bun": "Bun 下载缓存", "yarn": "Yarn 下载缓存", "pnpm": "pnpm 下载缓存",
	"cocoapods": "CocoaPods 缓存", "org.swift.swiftpm": "SwiftPM 缓存", "pypoetry": "Poetry 缓存",
	"pre-commit": "pre-commit 环境缓存", "cypress": "Cypress 下载缓存", "puppeteer": "Puppeteer 下载缓存",
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
	"homekit", "homed", "healthd", "com.apple.containermanagerd",
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
