package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Rules ported from Mole's `mo clean` for places under the home directory that
// the base clean rows (user caches, browsers, Xcode, package managers) do not
// reach. Each rule lists home-relative paths (globs allowed), a tier and the
// reason shown to the user. Mole's "is the app running" checks become owner
// processes, which feed cacheOwnerRules so the scan and the move both protect
// the data while the owner runs (with an unlock hint).
//
// Mole cleans the contents of a folder (`dir/*`); orbit offers the folder
// itself, except for system container temp folders and per-app state folders,
// where the children are offered so the folder stays in place.

// moleRule is one group of paths with one verdict.
type moleRule struct {
	paths      []string
	tier       string
	reason     string
	owners     []string // processes that hold the data open
	cli        string   // command line tool that holds the data open
	keepNewest int      // the newest N matches are kept (protected)
	// find lists matches that a glob cannot express (a guard, an age, a
	// running owner). The rule's paths then only tie the owners to folders.
	find func(s *Scanner) []moleHit
}

// moleHit is one path found by a rule's find hook, with its own reason.
type moleHit struct{ path, reason string }

// moleGroup is one clean row built from rules.
type moleGroup struct {
	id, category, name, impact, rebuild string
	rules                               []moleRule
}

// kindReason is the sentence for a safe rule: "<name> <what it is>, <why it is fine>".
var kindReason = map[string]string{
	"cache": "%s 的缓存，下次使用时会自动重新生成",
	"log":   "%s 的日志，只是历史记录，清理后不影响使用",
	"temp":  "%s 的临时文件，应用会按需重新生成",
	"crash": "%s 的崩溃报告，只用于排查问题",
	"model": "%s 的本地模型缓存，需要时会重新下载",
	"pkg":   "%s 的下载缓存，下次安装依赖时会重新下载",
}

const moleKeepNewest = 2

// Ported from tw93/Mole V1.70.0 lib/clean/dev.sh clean_xcode_device_support
const moleKeepReason = "保留最近使用的 2 个系统版本，设备调试仍可能用到"

func sf(name, kind string, paths ...string) moleRule {
	return moleRule{paths: paths, tier: "safe", reason: fmt.Sprintf(kindReason[kind], name)}
}

// ok is a safe rule with its own sentence.
func ok(reason string, paths ...string) moleRule {
	return moleRule{paths: paths, tier: "safe", reason: reason}
}

func rv(reason string, paths ...string) moleRule {
	return moleRule{paths: paths, tier: "review", reason: reason}
}

func (r moleRule) by(owners ...string) moleRule { r.owners = owners; return r }
func (r moleRule) tool(cli string) moleRule     { r.cli = cli; return r }
func (r moleRule) keep(n int) moleRule          { r.keepNewest = n; return r }

// tools builds one safe rule per (name, path) pair.
func tools(kind string, pairs ...string) []moleRule {
	var rules []moleRule
	for i := 0; i+1 < len(pairs); i += 2 {
		rules = append(rules, sf(pairs[i], kind, pairs[i+1]))
	}
	return rules
}

func join(parts ...[]moleRule) []moleRule {
	var all []moleRule
	for _, p := range parts {
		all = append(all, p...)
	}
	return all
}

// simulatorBooted is the launchd of a booted simulator device. Simulator.app
// need not be open for a device to run (simctl, xcodebuild, Xcode previews).
const (
	simulatorBooted  = "launchd_sim"
	simulatorDevices = "Library/Developer/CoreSimulator/Devices"
)

var simulatorOwners = []string{"Simulator", "CoreSimulatorService", "Xcode", "xcodebuild", simulatorBooted}

// ownerLabel is how a running owner is named to the user.
func ownerLabel(owner string) string {
	if owner == simulatorBooted {
		return "iOS 模拟器"
	}
	return owner
}

const (
	appSupport = "Library/Application Support/"
	containers = "Library/Containers/"
)

type chromiumVariant struct {
	name   string
	owners []string
	roots  []string // under Application Support
	models bool     // Chrome only: on-device model folders
}

// Ported from tw93/Mole V1.70.0 lib/clean/user.sh clean_browsers (Chrome, Arc, Brave, Dia, Helium and the other Chromium browsers)
// shortcut: the union of cache folder names is tried for every variant, a name a browser never creates just does not exist; upgrade if a browser reuses one of them for user data.
func chromiumRules() []moleRule {
	variants := []chromiumVariant{
		{"Chrome", []string{"Google Chrome"}, []string{"Google/Chrome"}, true},
		{"Arc", []string{"Arc"}, []string{"Arc", "Arc/User Data"}, false},
		{"Brave", []string{"Brave Browser"}, []string{"BraveSoftware/Brave-Browser"}, false},
		{"Vivaldi", []string{"Vivaldi"}, []string{"Vivaldi"}, false},
		{"QQ Browser", []string{"QQBrowser"}, []string{"QQBrowser3"}, false},
		{"BrowserOS", []string{"BrowserOS"}, []string{"BrowserOS"}, false},
		{"Helium", []string{"Helium"}, []string{"net.imput.helium"}, false},
		{"Yandex", []string{"Yandex", "YandexBrowser"}, []string{"Yandex/YandexBrowser"}, false},
		{"Dia", []string{"Dia"}, []string{"Dia/User Data"}, false},
	}
	profileLeaves := []string{"Application Cache", "Code Cache", "GPUCache", "DawnCache", "GrShaderCache",
		"GraphiteDawnCache", "DawnGraphiteCache", "DawnWebGPUCache"}
	topLeaves := []string{"ShaderCache", "GrShaderCache", "GraphiteDawnCache", "component_crx_cache",
		"extensions_crx_cache", "GPUPersistentCache"}
	var rules []moleRule
	for _, v := range variants {
		for _, root := range v.roots {
			base := appSupport + root + "/"
			for _, leaf := range profileLeaves {
				rules = append(rules, sf(v.name, "cache", base+"*/"+leaf).by(v.owners...))
			}
			for _, leaf := range topLeaves {
				rules = append(rules, sf(v.name, "cache", base+leaf).by(v.owners...))
			}
			rules = append(rules, sf(v.name, "crash", base+"Crashpad/completed").by(v.owners...))
			rules = append(rules, rv("Service Worker 的缓存存储可能保存网页的离线内容，清理后相关网页需要重新下载，请确认",
				base+"*/Service Worker/CacheStorage").by(v.owners...))
			if v.models {
				rules = append(rules, sf(v.name, "model", base+"OptGuideOnDeviceModel", base+"OptGuideOnDeviceClassifierModel",
					base+"optimization_guide_model_store").by(v.owners...))
			}
		}
	}
	return rules
}

// deviceSupport keeps the newest versions of each platform's debug symbols.
// Ported from tw93/Mole V1.70.0 lib/clean/dev.sh clean_xcode_device_support
func deviceSupportRules() []moleRule {
	var rules []moleRule
	for _, platform := range []string{"iOS", "macOS", "watchOS", "tvOS", "visionOS", "xrOS"} {
		r := ok("旧系统版本的设备调试符号，再次连接设备时会重新生成",
			"Library/Developer/Xcode/"+platform+" DeviceSupport/*").by("Xcode", "xcodebuild").keep(moleKeepNewest)
		rules = append(rules, r)
	}
	return rules
}

var moleGroups = append([]moleGroup{
	{
		id: "browser_profiles", category: "浏览器缓存", name: "Chromium 系浏览器与 Firefox 缓存",
		impact:  "浏览器的页面、代码和着色器缓存可以重建，不影响书签、密码和登录状态；Service Worker 的缓存存储由你确认。",
		rebuild: "下次打开网页时重新生成",
		rules: join(chromiumRules(), []moleRule{
			// Ported from tw93/Mole V1.70.0 lib/clean/user.sh clean_browsers (Firefox, GoogleUpdater)
			sf("Firefox", "cache", appSupport+"Firefox/Profiles/*/cache2").by("Firefox"),
			sf("Google Updater", "cache", appSupport+"Google/GoogleUpdater/crx_cache", appSupport+"Google/GoogleUpdater/*.old"),
		}),
	},
	{
		id: "dev_tools", category: "开发缓存", name: "开发工具缓存与临时文件",
		impact:  "编译器、包管理器和开发工具的下载与中间缓存，可以重新下载或重新生成；用途不明确的内容由你确认。",
		rebuild: "下次使用对应工具时重新生成",
		// Ported from tw93/Mole V1.70.0 lib/clean/dev.sh (the clean_dev_* functions: language toolchains, build tools, containers, mobile SDKs)
		rules: join(
			tools("cache",
				"TypeScript", ".cache/typescript", "Vite", ".cache/vite", "webpack", ".cache/webpack", "ESLint", ".cache/eslint",
				"Prettier", ".cache/prettier", "Ruff", ".cache/ruff", "mypy", ".cache/mypy", "Poetry", ".cache/poetry",
				"SwiftPM", ".cache/swift-package-manager", "Bazel", ".cache/bazel", "Zig", ".cache/zig",
				"Terraform", ".cache/terraform", "GitLab Runner", ".cache/gitlab-runner", "OpenCode", ".cache/opencode",
				"pre-commit", ".cache/pre-commit", "curl", ".cache/curl", "wget", ".cache/wget", "Kaku", ".cache/kaku",
				"Corepack", ".cache/node/corepack", "Turborepo", ".turbo/cache", "Vite", ".vite/cache",
				"Parcel", ".parcel-cache", "pytest", ".pytest_cache", "RubyGems", ".gem/specs",
				"Docker Buildx", ".docker/buildx/cache", "kubectl", ".kube/cache", "Android", ".android/build-cache",
				"Android", ".android/cache", "Grafana", ".grafana/cache", "GitHub CLI", ".github/cache",
				"CircleCI", ".circleci/cache", "SonarScanner", ".sonar", "Oh My Zsh", ".oh-my-zsh/cache",
				"VirtualBox", "VirtualBox VMs/.cache",
				"Expo Go", ".expo/expo-go", "Expo", ".expo/android-apk-cache", "Expo", ".expo/ios-simulator-app-cache",
				"Expo", ".expo/native-modules-cache", "Expo", ".expo/schema-cache", "Expo", ".expo/template-cache",
				"Expo", ".expo/versions-cache"),
			tools("pkg",
				"Electron", ".cache/electron", "Puppeteer", ".cache/puppeteer", "Prisma", ".cache/prisma",
				"node-gyp", ".cache/node-gyp", "node-gyp", ".node-gyp", "pyenv", ".pyenv/cache", "rbenv", ".rbenv/cache",
				"RubyGems", ".gem/ruby/*/cache", "Bundler", ".bundle/cache", "Composer", ".composer/cache",
				"Hex", ".hex/cache", "opam", ".opam/download-cache", "rustup", ".rustup/downloads",
				"tnpm", ".tnpm/_cacache", "npm", ".npm/_prebuilds"),
			tools("log", "tnpm", ".tnpm/_logs", "npm", ".npm/_logs", "Cacher", ".cacher/logs", "Kite", ".kite/logs"),
			tools("temp", "CPAN", ".cpan/build", "Podman", ".local/share/containers/storage/tmp", "Vagrant", ".vagrant.d/tmp",
				"Vim", ".viminfo.tmp", "Volta", ".volta/tmp"),
			// shortcut: version manager caches are not Mole rules; they were found by hand in a storage audit, drop if the reasons prove wrong.
			[]moleRule{
				sf("Go (GVM)", "pkg", ".gvm/pkgsets/*/global/pkg/mod/cache/download").by("go"),
				rv("Volta 下载的 Node、npm 与 Yarn 安装包，已解包的版本不受影响；清理后安装新版本时需要重新下载，请确认", ".volta/tools/inventory/*"),
				rv("Gradle Wrapper 下载的 Gradle 发行版，清理后下次构建需要重新下载，请确认", ".gradle/wrapper/dists").tool("gradle"),
			},
			[]moleRule{
				ok("Zsh 补全缓存，下次启动 shell 时会自动重新生成", ".zcompdump*"),
				ok("less 的命令历史，清理后只是少了翻页搜索记录", ".lesshst"),
				ok("wget 的 HSTS 记录，下次访问时会自动重新生成", ".wget-hsts"),
				rv("Jenkins 工作区的构建输出，清理后下次构建需要重新编译，请确认", ".jenkins/workspace/*/target"),
			},
		),
	},
	{
		id: "xcode_support", category: "开发缓存", name: "Xcode 设备调试符号与测试设备数据",
		impact:  "旧系统版本的调试符号、Xcode 构建产物、测试设备数据和界面构建缓存可以重新生成；每个平台最近使用的两个系统版本会保留。",
		rebuild: "再次连接设备或重新构建时重新生成",
		rules: join(deviceSupportRules(), []moleRule{
			// Ported from tw93/Mole V1.70.0 lib/clean/app_caches.sh clean_xcode_tools
			sf("Xcode", "temp", "Library/Developer/Xcode/Products").by("Xcode", "xcodebuild"),
			sf("Xcode", "cache", "Library/Developer/Xcode/UserData/IB Support/*").by("Xcode"),
			sf("XCTest devices", "temp", "Library/Developer/XCTestDevices/*").by("Xcode", "xcodebuild", "Simulator", simulatorBooted),
			sf("Simulator", "temp", "Library/Developer/CoreSimulator/Devices/*/data/tmp/*").by(simulatorOwners...),
			sf("Simulator", "cache", "Library/Developer/CoreSimulator/Profiles/Runtimes/*/Contents/Resources/RuntimeRoot/System/Library/Caches/*").
				by(simulatorOwners...),
			// Ported from tw93/Mole V1.70.0 lib/clean/app_caches.sh _simulator_device_leaf_targets
			ok("已关机模拟器设备的诊断日志与系统缓存，下次启动该设备时会重新生成", simulatorDevices).by(simulatorOwners...).
				found(simulatorLeaves("data/var/db/diagnostics", "data/var/db/uuidtext", "data/Library/Caches",
					"data/Library/Application Support/PRBPosterExtensionDataStore")),
			rv("已关机模拟器设备下载的系统资产（如 Siri 模型），清理后下次启动该设备会重新下载，体积可能很大，请确认", simulatorDevices).
				by(simulatorOwners...).found(simulatorLeaves("data/private/var/MobileAsset/AssetsV2")),
		}),
	},
	{
		id: "gradle_pods", category: "包管理器缓存", name: "Gradle 与 CocoaPods 缓存",
		impact:  "Gradle 的构建缓存、守护进程日志和 CocoaPods 索引可以重新下载或重新生成；依赖仓库缓存由你确认。",
		rebuild: "下次构建时重新下载",
		rules: []moleRule{
			// Ported from tw93/Mole V1.70.0 lib/clean/dev.sh clean_dev_jvm (Gradle)
			sf("Gradle", "cache", ".gradle/caches/build-cache-*").tool("gradle"),
			sf("Gradle", "log", ".gradle/daemon").tool("gradle"),
			sf("Gradle", "temp", ".gradle/notifications", ".gradle/workers").tool("gradle"),
			// shortcut: modules-2 and the CocoaPods repos are not Mole rules; offered as review because they are large and rebuildable, drop if the reasons prove too alarming.
			rv("Gradle 的依赖仓库缓存，清理后离线时无法构建，下次构建需要重新下载，请确认", ".gradle/caches/modules-2").tool("gradle"),
			rv("CocoaPods 的本地 spec 仓库，清理后 pod install 需要重新克隆，耗时较长，请确认", ".cocoapods/repos"),
		},
	},
	{
		id: "app_support_caches", category: "应用缓存与日志", name: "应用内置缓存与日志",
		impact:  "编辑器、聊天、游戏和创作类应用放在 Application Support 与沙盒容器里的缓存、日志和临时文件，不含账户、聊天记录和项目数据。",
		rebuild: "下次打开应用时重新生成",
		// Ported from tw93/Mole V1.70.0 lib/clean/app_caches.sh (the clean_* functions: editors, communication, media, games, creative and utility apps)
		rules: join(
			electronRules("VS Code", []string{"Visual Studio Code", "Code"}, appSupport+"Code/", "Cache", "CachedData", "CachedExtensionVSIXs", "WebStorage/*/CacheStorage"),
			[]moleRule{
				sf("VS Code", "log", appSupport+"Code/logs").by("Visual Studio Code", "Code"),
				sf("Zed", "cache", appSupport+"Zed/node/cache", appSupport+"Zed/node/node-v*/cache").by("Zed", "zed"),
				sf("CodeBuddy Extension", "cache", appSupport+"CodeBuddyExtension/Cache").by("CodeBuddy", "CodeBuddy CN", "Visual Studio Code", "Code", "Cursor"),
				sf("CodeBuddy Extension", "log", appSupport+"CodeBuddyExtension/logs").by("CodeBuddy", "CodeBuddy CN", "Visual Studio Code", "Code", "Cursor"),
				sf("CodeBuddy CN", "log", appSupport+"CodeBuddy CN/logs").by("CodeBuddy CN"),
				sf("mihomo-party", "log", appSupport+"mihomo-party/logs").by("mihomo-party"),
				sf("Discord", "cache", appSupport+"discord/Cache").by("Discord"),
				sf("Slack", "cache", appSupport+"Slack/Cache").by("Slack"),
				sf("Microsoft Teams", "log", appSupport+"Microsoft/Teams/logs").by("Microsoft Teams", "MSTeams"),
				sf("Microsoft Teams", "temp", appSupport+"Microsoft/Teams/tmp").by("Microsoft Teams", "MSTeams"),
				sf("DingTalk", "log", appSupport+"iDingTalk/log", appSupport+"iDingTalk/holmeslogs").by("DingTalk"),
				sf("Sketch", "cache", appSupport+"com.bohemiancoding.sketch3/cache").by("Sketch"),
				sf("Steam", "cache", appSupport+"Steam/htmlcache", appSupport+"Steam/appcache", appSupport+"Steam/depotcache",
					appSupport+"Steam/steamapps/shadercache").by("steam_osx", "Steam"),
				sf("Steam", "log", appSupport+"Steam/logs").by("steam_osx", "Steam"),
				sf("Battle.net", "cache", appSupport+"Battle.net/Cache").by("Battle.net"),
				sf("Minecraft", "log", appSupport+"minecraft/logs").by("minecraft", "Minecraft"),
				sf("Minecraft", "crash", appSupport+"minecraft/crash-reports").by("minecraft", "Minecraft"),
				sf("Minecraft", "cache", appSupport+"minecraft/webcache", appSupport+"minecraft/webcache2").by("minecraft", "Minecraft"),
				sf("Lunar Client", "cache", ".lunarclient/game-cache", ".lunarclient/launcher-cache").by("Lunar Client"),
				sf("Lunar Client", "log", ".lunarclient/logs", ".lunarclient/offline/*/logs", ".lunarclient/offline/files/*/logs").by("Lunar Client"),
				sf("PCSX2", "cache", appSupport+"PCSX2/cache").by("PCSX2"),
				sf("RPCS3", "log", appSupport+"rpcs3/logs").by("rpcs3"),
				sf("Quark", "cache", appSupport+"Quark/Cache/videoCache").by("Quark"),
				sf("Spacedrive", "cache", appSupport+"spacedrive/thumbnails").by("Spacedrive"),
				sf("Stremio", "cache", appSupport+"stremio/stremio-server/stremio-cache").by("Stremio", "stremio"),
				sf("WeType", "cache", appSupport+"WeType/com.onevcat.Kingfisher.ImageCache.WeType", appSupport+"WeType/DictUpdate").by("WeType"),
				sf("Wondershare", "temp", appSupport+"com.wondershare.Installer"),
				rv("Adobe 的媒体缓存文件可以重新生成，但重新生成耗时较长，请确认", appSupport+"Adobe/Common/Media Cache Files").by("Adobe"),
				rv("DaVinci Resolve 的渲染缓存可以重建，但重建耗时较长，也可能是唯一的渲染结果，请确认", "Movies/CacheClip").by("DaVinci Resolve", "Resolve"),
				// Ported from tw93/Mole V1.70.0 lib/clean/app_caches.sh clean_wechat_container_caches
				sf("WeChat", "log", containers+"com.tencent.xinWeChat/Data/Documents/app_data/log").by("WeChat"),
				sf("WeChat", "cache", containers+"com.tencent.xinWeChat/Data/.wxapplet/WMPF",
					containers+"com.tencent.xinWeChat/Data/Documents/app_data/radium/web/profiles/*/Cache",
					containers+"com.tencent.xinWeChat/Data/Documents/app_data/radium/web/profiles/*/Service Worker/CacheStorage").by("WeChat"),
				sf("WeCom", "log", containers+"com.tencent.WeWorkMac/Data/Library/Application Support/WXWork/Log").by("WeCom", "企业微信"),
			},
			wecomRules(),
			// Ported from tw93/Mole V1.70.0 lib/clean/user.sh clean_office_apps / clean_utm_caches
			officeRules("Microsoft Word", "com.microsoft.Word"),
			officeRules("Microsoft Excel", "com.microsoft.Excel"),
			[]moleRule{
				sf("UTM", "cache", containers+"com.utmapp.UTM/Data/Library/Caches/*").by("UTM"),
				sf("UTM", "temp", containers+"com.utmapp.UTM/Data/tmp/*").by("UTM"),
				sf("NetNewsWire", "cache", containers+"com.ranchero.NetNewsWire-Evergreen/Data/Library/Caches/*").by("NetNewsWire"),
				sf("MindNode", "cache", containers+"com.ideasoncanvas.mindnode/Data/Library/Caches/*").by("MindNode"),
				sf("Folo", "cache", containers+"is.follow/Data/Library/Application Support/Folo/Cache/Cache_Data").by("Folo"),
				sf("QQ Music", "cache", containers+"com.tencent.QQMusicMac/Data/Library/Caches/*",
					containers+"com.tencent.QQMusicMac/Data/Library/Application Support/QQMusicMac/iRRCache",
					containers+"com.tencent.QQMusicMac/Data/Library/Application Support/QQMusicMac/iCache").by("QQMusic", "QQ Music"),
				sf("QQ Music", "log", containers+"com.tencent.QQMusicMac/Data/Library/Application Support/QQMusicMac/iLog").by("QQMusic", "QQ Music"),
				sf("QQ Music", "temp", containers+"com.tencent.QQMusicMac/Data/Library/Application Support/QQMusicMac/iTemp").by("QQMusic", "QQ Music"),
				sf("Tencent Video", "cache", containers+"com.tencent.tenvideo/Data/Library/Application Support/VideoNative",
					containers+"com.tencent.tenvideo/Data/Library/Application Support/documentCache",
					containers+"com.tencent.tenvideo/Data/Library/Application Support/Upgrade").by("Tencent Video", "腾讯视频"),
				sf("SenPlayer", "cache", containers+"com.wuziqi.SenPlayer/Data/tmp/videoCache").by("SenPlayer"),
			},
		),
	},
	{
		id: "macos_user", category: "应用缓存与日志", name: "macOS 用户级缓存与状态",
		impact:  "窗口恢复状态、建议缓存、信息预览缓存和系统应用的沙盒缓存可以重新生成。",
		rebuild: "系统会按需重新生成",
		// Ported from tw93/Mole V1.70.0 lib/clean/user.sh clean_system_state / clean_app_containers
		rules: join(
			[]moleRule{
				ok("各应用的窗口恢复信息，清理后应用以默认窗口打开", "Library/Saved Application State/*"),
				ok("Siri 建议的本地缓存，系统会按需重新生成", "Library/Suggestions/*"),
				rv("系统账户身份缓存，清理后部分账户可能需要重新验证，请确认", "Library/IdentityCaches/*"),
				rv("通讯录头像缓存，清理后头像会重新加载，请确认", appSupport+"AddressBook/Sources/*/Photos.cache"),
				sf("Messages", "cache", "Library/Messages/StickerCache", "Library/Messages/Caches/Previews/Attachments",
					"Library/Messages/Caches/Previews/StickerCache").by("Messages"),
				sf("Podcasts", "temp", containers+"com.apple.podcasts/Data/tmp/StreamedMedia", containers+"com.apple.podcasts/Data/tmp/*.heic",
					containers+"com.apple.podcasts/Data/tmp/*.img", containers+"com.apple.podcasts/Data/tmp/*CFNetworkDownload*.tmp").by("Podcasts"),
			},
			tools("temp",
				"Media analysis", containers+"com.apple.mediaanalysisd/Data/tmp/*",
				"Apple Configurator", containers+"com.apple.configurator.xpc.InternetService/Data/tmp/*",
				"Aerial wallpaper", containers+"com.apple.wallpaper.extension.aerials/Data/tmp/*",
				// shortcut: geod keeps about a thousand tmp files, so its tmp folder is one item instead of Mole's per-file list; the OS recreates the folder, split per file if that proves wrong.
				"Geod", containers+"com.apple.geod/Data/tmp",
				"Apple Media Services", containers+"com.apple.AppleMediaServicesUI.UtilityExtension/Data/tmp/*"),
			tools("cache",
				"Media analysis", containers+"com.apple.mediaanalysisd/Data/Library/Caches/*",
				"App Store", containers+"com.apple.AppStore/Data/Library/Caches/*",
				"Stocks", containers+"com.apple.stocks/Data/Library/Caches/*",
				"Memoji picker", containers+"com.apple.AvatarUI.AvatarPickerMemojiPicker/Data/Library/Caches/*",
				"Music artwork", containers+"com.apple.AMPArtworkAgent/Data/Library/Caches/*",
				"CoreDevice", containers+"com.apple.CoreDevice.CoreDeviceService/Data/Library/Caches/*",
				"Apple Intelligence extension", containers+"com.apple.NeptuneOneExtension/Data/Library/Caches/*"),
		),
	},
}, gapGroups...)

// electronRules is the Chromium-style cache folders of one Electron app.
func electronRules(name string, owners []string, base string, leaves ...string) []moleRule {
	var rules []moleRule
	for _, leaf := range leaves {
		rules = append(rules, sf(name, "cache", base+leaf).by(owners...))
	}
	return rules
}

// wecomRules covers the WeCom Chromium profiles; only Default and wew_* are profiles.
// Ported from tw93/Mole V1.70.0 lib/clean/app_caches.sh clean_wechat_container_caches
func wecomRules() []moleRule {
	base := containers + "com.tencent.WeWorkMac/Data/Documents/cefcache/"
	var paths []string
	for _, profile := range []string{"Default", "wew_*"} {
		for _, leaf := range []string{"Service Worker/CacheStorage", "Cache", "Code Cache", "GPUCache"} {
			paths = append(paths, base+profile+"/"+leaf)
		}
	}
	return []moleRule{sf("WeCom", "cache", paths...).by("WeCom", "企业微信")}
}

func officeRules(name, bundle string) []moleRule {
	base := containers + bundle + "/Data/"
	return []moleRule{
		sf(name, "cache", base+"Library/Caches/*").by(name),
		sf(name, "temp", base+"tmp/*").by(name),
		sf(name, "log", base+"Library/Logs/*").by(name),
	}
}

// globHome expands a home-relative pattern. `*` does not match dotfiles unless
// the segment itself starts with a dot, and symlinks are never returned.
func (s *Scanner) globHome(pattern string) []string {
	current := []string{s.home}
	for _, segment := range strings.Split(pattern, "/") {
		var next []string
		for _, base := range current {
			if !strings.ContainsAny(segment, "*?[") {
				next = append(next, filepath.Join(base, segment))
				continue
			}
			entries, err := os.ReadDir(base)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				name := entry.Name()
				if isHidden(name) && !isHidden(segment) {
					continue
				}
				if matched, _ := filepath.Match(segment, name); matched {
					next = append(next, filepath.Join(base, name))
				}
			}
		}
		current = next
	}
	var found []string
	for _, path := range current {
		if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink == 0 {
			found = append(found, path)
		}
	}
	return found
}

// moleGroupSpecs turns the rule groups into clean rows. Roots are the paths
// that exist right now; a group with none is skipped.
func (s *Scanner) moleGroupSpecs() []CatalogSpec {
	var specs []CatalogSpec
	for _, group := range moleGroups {
		verdicts := map[string]verdict{}
		var roots []string
		tier := "safe"
		for _, rule := range group.rules {
			var matches []string
			hitReason := map[string]string{}
			if rule.find != nil {
				for _, hit := range rule.find(s) {
					matches = append(matches, hit.path)
					hitReason[hit.path] = hit.reason
				}
			} else {
				for _, pattern := range rule.paths {
					matches = append(matches, s.globHome(pattern)...)
				}
			}
			if rule.keepNewest > 0 {
				sort.SliceStable(matches, func(i, j int) bool { return modifiedTime(matches[i]) > modifiedTime(matches[j]) })
			}
			for i, path := range matches {
				if _, seen := verdicts[path]; seen {
					continue
				}
				v := verdict{tier: rule.tier, reason: rule.reason}
				if reason := hitReason[path]; reason != "" {
					v.reason = reason
				}
				if i < rule.keepNewest {
					v = verdict{tier: "protected", reason: moleKeepReason}
				}
				if v.tier == "review" {
					tier = "review"
				}
				verdicts[path] = v
				roots = append(roots, path)
			}
		}
		if len(roots) == 0 {
			continue
		}
		specs = append(specs, CatalogSpec{
			ID: group.id, Category: group.category, Name: group.name, Roots: roots, Tier: tier,
			Impact: group.impact, Rebuild: group.rebuild,
			Classify: func(_ *Scanner, path string, _ dirSize) verdict { return verdicts[path] },
		})
	}
	return specs
}

// moleLeadingRoots is the part of each pattern before its first glob segment,
// which is how a rule's owners are tied to the folders they hold open.
func moleLeadingRoots(patterns []string) []string {
	seen := map[string]bool{}
	var roots []string
	for _, pattern := range patterns {
		var leading []string
		for _, segment := range strings.Split(pattern, "/") {
			if strings.ContainsAny(segment, "*?[") {
				break
			}
			leading = append(leading, segment)
		}
		if root := strings.Join(leading, "/"); root != "" && !seen[root] {
			seen[root] = true
			roots = append(roots, root)
		}
	}
	return roots
}

// The busy checks of Mole's rules become running-owner protection, which the
// scan (applyCleanupProtection) and the move (cleanupReason) both read.
func init() {
	for _, group := range moleGroups {
		for _, rule := range group.rules {
			if len(rule.owners) == 0 && rule.cli == "" {
				continue
			}
			cacheOwnerRules = append(cacheOwnerRules, struct {
				roots, owners []string
				cli           string
			}{moleLeadingRoots(rule.paths), rule.owners, rule.cli})
		}
	}
}
