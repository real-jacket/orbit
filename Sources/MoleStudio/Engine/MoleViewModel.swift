import Foundation
import SwiftUI
import AppKit

/// Resolved at runtime so the bundled demo dataset never hard-codes a `/Users/<name>` path.
private let homePath = FileManager.default.homeDirectoryForCurrentUser.path

@MainActor
public final class MoleViewModel: ObservableObject {
    @Published public var selectedModule: MoleModule = .clean
    @Published public var cleanFilter: String = "all"
    @Published public var selectedCleanIndex: Int = 2
    @Published public var selectedMoonIndex: Int = 0
    @Published public var selectedAppIndex: Int = 0
    @Published public var selectedPurgeIndex: Int = 0
    @Published public var searchQuery: String = ""

    @Published public var isZH: Bool = true
    @Published public var isDarkMode: Bool = true
    @Published public var useFahrenheit: Bool = false
    @Published public var reduceMotion: Bool = false
    @Published public var dryRunMode: Bool = true
    @Published public var useWebKitPrecisionMode: Bool = true

    @Published public var showDoctorModal: Bool = false
    @Published public var showConfirmSheet: Bool = false
    @Published public var showCommandPalette: Bool = false
    @Published public var showHudPopover: Bool = false
    @Published public var toastMessage: String? = nil

    @Published public var aiRetention: AIRetentionWindow = .days30
    @Published public var fanMode: String = "Auto"
    @Published public var selectedRunner: String = "🐹"
    @Published public var cliOutputLog: [String] = [
        "[orbit v9.0] Ready — Bundled tw93/mole CLI v1.59.1 verified.",
        "$ mo clean --dry-run -> 12.49 GB reclaimable across 115 items (0.38s)."
    ]

    @Published public var cleanItems: [CleanCategoryItem] = [
        CleanCategoryItem(
            id: "user_app_caches",
            category: "应用缓存与日志",
            name: "macOS 用户应用缓存与 CrashReporter 崩溃日志",
            path: "~/Library/Caches & ~/Library/Logs",
            sizeGB: 3.15,
            itemCount: 38,
            filesCountText: "24,180 个文件",
            lastModifiedText: "今天 10:42",
            rebuildTimeText: "小于 2 秒 (无感重建)",
            cliCommand: "$ mo clean --target app-caches --dry-run",
            impactDescription: "完全安全：仅清理应用的临时渲染与网络图片缓存，不会影响登录态、文稿或偏好设置。下次启动应用时会自动按需重建。",
            tier: .safeRegenerable,
            isSelected: true,
            subpaths: [
                SubpathEntry(path: "~/Library/Caches/com.spotify.client/Data", sizeText: "1.12 GB"),
                SubpathEntry(path: "~/Library/Caches/com.apple.helpd", sizeText: "680 MB"),
                SubpathEntry(path: "~/Library/Logs/DiagnosticReports", sizeText: "540 MB"),
                SubpathEntry(path: "~/Library/Caches/CloudKit", sizeText: "490 MB"),
                SubpathEntry(path: "~/Library/Caches/com.raycast.macos", sizeText: "320 MB")
            ]
        ),
        CleanCategoryItem(
            id: "browser_caches",
            category: "浏览器渲染缓存",
            name: "Chrome、Safari 与 Arc 离线脚本及 GPU 着色器缓存",
            path: "~/Library/Caches/Google/Chrome & com.apple.Safari",
            sizeGB: 2.07,
            itemCount: 50,
            filesCountText: "41,920 个文件",
            lastModifiedText: "今天 11:15",
            rebuildTimeText: "小于 3 秒 (网页首屏加载)",
            cliCommand: "$ mo clean --target browser-caches --dry-run",
            impactDescription: "完全安全：仅清理 HTTP 媒体缓存、WebGL 着色器缓存与失效的 ServiceWorker，严格保留书签、历史记录、Cookie 与已保存密码。",
            tier: .safeRegenerable,
            isSelected: true,
            subpaths: [
                SubpathEntry(path: "~/Library/Caches/Google/Chrome/Default/Cache", sizeText: "1.18 GB"),
                SubpathEntry(path: "~/Library/Caches/com.apple.Safari/WebKitCache", sizeText: "520 MB"),
                SubpathEntry(path: "~/Library/Caches/company.thebrowser.Browser/GPUCache", sizeText: "370 MB")
            ]
        ),
        CleanCategoryItem(
            id: "xcode_derived",
            category: "开发者构建产物",
            name: "Xcode 编译中间产物 (DerivedData) 与旧版模拟器缓存",
            path: "~/Library/Developer/Xcode/DerivedData",
            sizeGB: 4.65,
            itemCount: 12,
            filesCountText: "18,420 个文件",
            lastModifiedText: "18 天前",
            rebuildTimeText: "约 15~30 秒 (下次编译)",
            cliCommand: "$ mo clean --target xcode-derived --dry-run",
            impactDescription: "安全可再生：清理 Xcode 历史项目的编译索引、PCH 预编译头与旧版 iOS Simulator 临时镜像。下次打开对应工程编译时会自动重新生成。",
            tier: .safeRegenerable,
            isSelected: true,
            subpaths: [
                SubpathEntry(path: "~/Library/Developer/Xcode/DerivedData/MoleStudio-f8a92c", sizeText: "1.84 GB"),
                SubpathEntry(path: "~/Library/Developer/Xcode/DerivedData/ModuleCache.noindex", sizeText: "1.15 GB"),
                SubpathEntry(path: "~/Library/Developer/CoreSimulator/Caches/dyld", sizeText: "1.08 GB"),
                SubpathEntry(path: "~/Library/Developer/Xcode/iOS DeviceSupport/17.4 (21E219)", sizeText: "580 MB")
            ]
        ),
        CleanCategoryItem(
            id: "pkg_http_caches",
            category: "包管理器缓存",
            name: "npm、pnpm、pip 与 Cargo 离线 Tarball 压缩包缓存",
            path: "~/.npm/_cacache & ~/Library/Caches/pip",
            sizeGB: 1.88,
            itemCount: 9,
            filesCountText: "12,640 个文件",
            lastModifiedText: "9 天前",
            rebuildTimeText: "视网络情况按需下载",
            cliCommand: "$ mo clean --target pkg-managers --dry-run",
            impactDescription: "需确认项：清理后不影响现有项目的 node_modules 或虚拟环境；但在离线环境下重新安装旧依赖包时需重新从镜像源下载。",
            tier: .reviewOnly,
            isSelected: true,
            subpaths: [
                SubpathEntry(path: "~/.npm/_cacache/content-v2/sha512", sizeText: "920 MB"),
                SubpathEntry(path: "~/Library/Caches/pip/http-v2", sizeText: "540 MB"),
                SubpathEntry(path: "~/.cargo/registry/cache", sizeText: "420 MB")
            ]
        ),
        CleanCategoryItem(
            id: "orphaned_leftovers",
            category: "已卸载软件残留",
            name: "已卸载应用的孤立沙盒容器与 Application Support 残留",
            path: "~/Library/Application Support (6 个孤立目录)",
            sizeGB: 0.74,
            itemCount: 6,
            filesCountText: "3,120 个文件",
            lastModifiedText: "46 天前",
            rebuildTimeText: "无需重建 (无主残留)",
            cliCommand: "$ mo clean --target orphaned-containers --dry-run",
            impactDescription: "完全安全：经与 /Applications 及 Spotlight 注册表比对，以下目录对应的主程序均已卸载超过 30 天，属于无主残留文件。",
            tier: .safeRegenerable,
            isSelected: true,
            subpaths: [
                SubpathEntry(path: "~/Library/Application Support/Slack/Service Worker", sizeText: "410 MB"),
                SubpathEntry(path: "~/Library/Containers/com.tencent.qq/Data/Library/Caches", sizeText: "330 MB")
            ]
        ),
        CleanCategoryItem(
            id: "local_snapshots",
            category: "系统快照与壁纸",
            name: "APFS 本地时间机器快照与未使用的超清动态壁纸包",
            path: "com.apple.TimeMachine.localsnapshots",
            sizeGB: 4.20,
            itemCount: 3,
            filesCountText: "3 个 APFS 快照卷",
            lastModifiedText: "2 天前",
            rebuildTimeText: "下次 Time Machine 备份",
            cliCommand: "$ mo clean --target apfs-snapshots --dry-run",
            impactDescription: "需确认项：通过 tmutil thinLocalSnapshots 释放本地 APFS 历史快照空间，默认不勾选，建议仅在磁盘空间紧张时手动开启。",
            tier: .reviewOnly,
            isSelected: false,
            subpaths: [
                SubpathEntry(path: "/System/Volumes/Data/.timemachine/2026-10-12-092104.backup", sizeText: "3.10 GB", isSelected: false),
                SubpathEntry(path: "/Library/Application Support/com.apple.idleassetsd/Customer", sizeText: "1.10 GB", isSelected: false)
            ]
        ),
        CleanCategoryItem(
            id: "apple_neural_engine",
            category: "硬件级保护项",
            name: "Apple 神经网络引擎 (ANE) 预编译模型缓存",
            path: "~/Library/Caches/com.apple.e5rt.e5bundlecache",
            sizeGB: 1.92,
            itemCount: 14,
            filesCountText: "840 个模型分片",
            lastModifiedText: "系统实时维护",
            rebuildTimeText: "禁止清理 (硬件级锁定)",
            cliCommand: "# Protected by Mole ANE Hardware Guardrail",
            impactDescription: "内置硬件保护（不可勾选删除）：该目录存储 Apple Silicon 神经网络引擎编译后的 CoreML 权重缓存，若误删会导致系统与本地 AI 推理出现重新编译卡顿。",
            tier: .protectedSystem,
            isSelected: false,
            subpaths: [
                SubpathEntry(path: "~/Library/Caches/com.apple.e5rt.e5bundlecache/ANECompiler", sizeText: "1.92 GB", isSelected: false)
            ]
        )
    ]

    @Published public var aiMoonItems: [AIMoonItem] = [
        AIMoonItem(
            id: "ai_bins",
            bandTitle: "第一安全带 · 旧版本二进制包",
            name: "Claude Code、Cursor Agent 与 Codex 历史自更新旧版本包 (11 个旧版)",
            path: "~/.local/share/claude/versions & ~/.cursor-agent/releases",
            sizeGB: 3.42,
            filesCountText: "11 个可执行包",
            lastModifiedText: "14~60 天前",
            rebuildTimeText: "无需重建 (已升级)",
            cliCommand: "$ mo clean --ai-care --band 1 --dry-run",
            impactDescription: "完全安全：AI 命令行工具每次后台自动升级都会在本地留下 200MB~350MB 的旧版二进制可执行文件。Mole 仅清理已被替换的旧版本，严格保留当前正在运行的最新版本。",
            tier: .safeRegenerable,
            isSelected: true,
            subpaths: [
                SubpathEntry(path: "~/.local/share/claude/versions/1.0.24 ~ 1.0.31 (8 versions)", sizeText: "2.14 GB"),
                SubpathEntry(path: "~/.cursor-agent/releases/0.41.0 ~ 0.43.2 (3 versions)", sizeText: "1.28 GB")
            ]
        ),
        AIMoonItem(
            id: "ai_caches",
            bandTitle: "第一安全带 · 渲染与日志缓存",
            name: "Claude Desktop、Cursor 与 Codex 渲染缓存及历史崩溃转储",
            path: "~/Library/Caches/com.todesktop.230313mzl4w4u92 & ~/.cache/codex",
            sizeGB: 2.18,
            filesCountText: "9,410 个文件",
            lastModifiedText: "今天 09:30",
            rebuildTimeText: "小于 2 秒 (启动自动重建)",
            cliCommand: "$ mo clean --ai-care --caches --dry-run",
            impactDescription: "完全安全：仅清理 Electron / Chromium GPU 缓存与过期调试日志，不影响任何本地配置或扩展插件。",
            tier: .safeRegenerable,
            isSelected: true,
            subpaths: [
                SubpathEntry(path: "~/Library/Caches/Cursor/GPUCache & Code Cache", sizeText: "1.46 GB"),
                SubpathEntry(path: "~/.cache/codex/logs", sizeText: "720 MB")
            ]
        ),
        AIMoonItem(
            id: "ai_worktrees",
            bandTitle: "第二安全带 · 过期 Worktree (>30天)",
            name: "已合并超过 30 天的 Agent Git Worktrees 与孤立截图缓存",
            path: "~/.codex/worktrees & ~/.claude/orphaned-renders",
            sizeGB: 4.95,
            filesCountText: "14 个临时树",
            lastModifiedText: "38 天前",
            rebuildTimeText: "无需重建 (分支已合并)",
            cliCommand: "$ mo clean --ai-care --worktrees --retention 30d",
            impactDescription: "智能生命周期保护：仅列出超过 30 天未修改且无未提交变更的临时 Git Worktree；24 小时内的活跃任务会被强制排除。",
            tier: .reviewOnly,
            isSelected: true,
            subpaths: [
                SubpathEntry(path: "~/.codex/worktrees/feat-refactor-202608", sizeText: "3.40 GB"),
                SubpathEntry(path: "~/.claude/orphaned-renders/2026-08", sizeText: "1.55 GB")
            ]
        ),
        AIMoonItem(
            id: "ai_vscdb",
            bandTitle: "第三安全带 · 状态库原地压缩",
            name: "Cursor 与 Claude 本地状态库无损碎片整理 (>200MB state.vscdb)",
            path: "~/Library/Application Support/Cursor/User/globalStorage/state.vscdb",
            sizeGB: 1.65,
            filesCountText: "2 个 SQLite 库",
            lastModifiedText: "昨天 22:10",
            rebuildTimeText: "约 4 秒 (原地 VACUUM)",
            cliCommand: "$ sqlite3 state.vscdb 'VACUUM;'",
            impactDescription: "无损原地压缩：不删除任何键值数据，仅在检测到宿主应用已关闭时执行 sqlite3 VACUUM 回收游离页碎片。",
            tier: .reviewOnly,
            isSelected: true,
            subpaths: [
                SubpathEntry(path: "~/Library/Application Support/Cursor/User/globalStorage/state.vscdb", sizeText: "可缩减 1.65 GB")
            ]
        ),
        AIMoonItem(
            id: "ai_protected",
            bandTitle: "核心资产永久保护（绝不清理）",
            name: "AI 历史对话记录、长期记忆 (Memories)、Plans 与自定义 Skills",
            path: "~/.claude/projects, ~/.codex/sessions, ~/.claude/skills",
            sizeGB: 1.14,
            filesCountText: "1,280 个会话",
            lastModifiedText: "实时保护中",
            rebuildTimeText: "禁止清理 (永久锁定)",
            cliCommand: "# Protected by Mole AI Conversation Guardrail",
            impactDescription: "永久锁定保护：您的所有对话上下文、项目级 CLAUDE.md 记忆与自定义技能脚本均受硬件级白名单保护，永远不会被清理。",
            tier: .protectedSystem,
            isSelected: false,
            subpaths: [
                SubpathEntry(path: "~/.claude/projects & ~/.claude/skills", sizeText: "1.14 GB (永久保护)", isSelected: false)
            ]
        )
    ]

    @Published public var installedApps: [InstalledAppBundle] = [
        InstalledAppBundle(
            id: "photoshop", name: "Adobe Photoshop 2026", bundleIdentifier: "com.adobe.Photoshop",
            version: "27.1.0", lastUsed: "62 天前", appSizeGB: 4.20, leftoverSizeGB: 8.60,
            isSelected: true, keepPersonalData: true,
            impactDescription: "已开启「保留个人配置 (D)」：将彻底移除主程序（4.20 GB）及 8.60 GB 缓存与守护进程，但为您保留笔刷、色板与快捷键预设文件。",
            subpaths: [
                SubpathEntry(path: "/Applications/Adobe Photoshop 2026/Adobe Photoshop 2026.app", sizeText: "4.20 GB"),
                SubpathEntry(path: "~/Library/Application Support/Adobe/Adobe Photoshop 2026", sizeText: "5.82 GB"),
                SubpathEntry(path: "~/Library/Caches/Adobe/Photoshop", sizeText: "2.41 GB"),
                SubpathEntry(path: "~/Library/Logs/Adobe/Photoshop", sizeText: "364 MB"),
                SubpathEntry(path: "~/Library/LaunchAgents/com.adobe.GC.Invoker-1.0.plist", sizeText: "4 KB")
            ]
        ),
        InstalledAppBundle(
            id: "docker", name: "Docker Desktop", bundleIdentifier: "com.docker.docker",
            version: "4.38.0", lastUsed: "14 天前", appSizeGB: 1.85, leftoverSizeGB: 6.45,
            isSelected: false, keepPersonalData: true,
            impactDescription: "深度扫描到 ~/Library/Containers/com.docker.docker 下的虚拟磁盘镜像与后台 privileged helper 守护服务。",
            subpaths: [
                SubpathEntry(path: "/Applications/Docker.app", sizeText: "1.85 GB"),
                SubpathEntry(path: "~/Library/Containers/com.docker.docker/Data/vms", sizeText: "5.90 GB"),
                SubpathEntry(path: "~/Library/Group Containers/group.com.docker", sizeText: "550 MB")
            ]
        ),
        InstalledAppBundle(
            id: "intellij", name: "IntelliJ IDEA Ultimate", bundleIdentifier: "com.jetbrains.intellij",
            version: "2026.1", lastUsed: "21 天前", appSizeGB: 2.40, leftoverSizeGB: 3.10,
            isSelected: false, keepPersonalData: true,
            impactDescription: "精准定位 JetBrains 索引缓存与历史日志目录，同时保护同厂商其他 IDE 的共享许可证配置。",
            subpaths: [
                SubpathEntry(path: "/Applications/IntelliJ IDEA.app", sizeText: "2.40 GB"),
                SubpathEntry(path: "~/Library/Caches/JetBrains/IntelliJIdea2026.1", sizeText: "2.35 GB"),
                SubpathEntry(path: "~/Library/Application Support/JetBrains/IntelliJIdea2026.1/plugins", sizeText: "750 MB")
            ]
        ),
        InstalledAppBundle(
            id: "premiere", name: "Adobe Premiere Pro 2026", bundleIdentifier: "com.adobe.PremierePro",
            version: "26.0.1", lastUsed: "45 天前", appSizeGB: 3.40, leftoverSizeGB: 4.90,
            isSelected: false, keepPersonalData: true,
            impactDescription: "包含 Media Cache 音视频峰值缓存与插件配置，开启保留个人配置后将保留您的序列预设与工作区布局。",
            subpaths: [
                SubpathEntry(path: "/Applications/Adobe Premiere Pro 2026", sizeText: "3.40 GB"),
                SubpathEntry(path: "~/Library/Application Support/Adobe/Common/Media Cache Files", sizeText: "4.90 GB")
            ]
        ),
        InstalledAppBundle(
            id: "blender", name: "Blender 4.3 LTS", bundleIdentifier: "org.blenderfoundation.blender",
            version: "4.3.2", lastUsed: "3 个月前", appSizeGB: 1.15, leftoverSizeGB: 1.85,
            isSelected: false, keepPersonalData: true,
            impactDescription: "包含 Cycles 渲染内核着色器缓存与扩展插件目录。",
            subpaths: [
                SubpathEntry(path: "/Applications/Blender.app", sizeText: "1.15 GB"),
                SubpathEntry(path: "~/Library/Application Support/Blender/4.3", sizeText: "1.85 GB")
            ]
        ),
        InstalledAppBundle(
            id: "android_studio", name: "Android Studio Ladybug", bundleIdentifier: "com.google.android.studio",
            version: "2024.2.1", lastUsed: "4 个月前", appSizeGB: 2.10, leftoverSizeGB: 6.20,
            isSelected: false, keepPersonalData: true,
            impactDescription: "包含 Gradle 全局构建缓存与 Android AVD 模拟器镜像目录。",
            subpaths: [
                SubpathEntry(path: "/Applications/Android Studio.app", sizeText: "2.10 GB"),
                SubpathEntry(path: "~/Library/Android/sdk & ~/.android/avd", sizeText: "6.20 GB")
            ]
        )
    ]

    // Demo rows are deliberately dense one-line records, so the line-length rule is exempted
    // for the sample-data tables only.
    // swiftlint:disable line_length
    @Published public var optimizeTasks: [OptimizeTaskItem] = [
        OptimizeTaskItem(id: "ime", title: "输入法切换延迟修复与 Spotlight 索引健康校验", command: "killall -HUP TextInputMenuAgent && mdutil -s /", status: .applied, reasonOrDetail: "已刷新中文输入法状态机缓存并验证 Spotlight 索引完整性"),
        OptimizeTaskItem(id: "ql", title: "重建 QuickLook 缩略图与 Finder 图标缓存", command: "qlmanage -r cache && killall Finder", status: .applied, reasonOrDetail: "已修复部分文稿图标白板问题并重载预览服务"),
        OptimizeTaskItem(id: "dns", title: "刷新系统 DNS 解析缓存 (mDNSResponder)", command: "dscacheutil -flushcache && killall -HUP mDNSResponder", status: .applied, reasonOrDetail: "已清空 312 条过期本地域名解析记录"),
        OptimizeTaskItem(id: "sqlite", title: "无损压缩系统通知中心、Quarantine 与 Dock SQLite 数据库", command: "sqlite3 QuarantineEventsV2 'VACUUM;'", status: .applied, reasonOrDetail: "已在后台整理 3 个系统级 SQLite 数据库碎片 (-148 MB)"),
        OptimizeTaskItem(id: "plist", title: "扫描并修复损坏的用户偏好设置 (.plist) 与失效启动项", command: "plutil -lint ~/Library/Preferences/*.plist", status: .applied, reasonOrDetail: "已校验 418 个配置文件，隔离 1 个指向已删除程序的失效 LaunchAgent"),
        OptimizeTaskItem(id: "safari", title: "整理 Safari 历史记录与 Apple Mail 索引数据库", command: "sqlite3 ~/Library/Safari/History.db 'VACUUM;'", status: .skipped, reasonOrDetail: "安全门控自动跳过：检测到 Safari.app 正在前台运行（关闭应用后可执行）"),
        OptimizeTaskItem(id: "purge_mem", title: "强制回收内核非活跃内存页 (purge)", command: "purge", status: .skipped, reasonOrDetail: "安全门控自动跳过：当前统一内存压力为「正常」（仍有 44.2% 充裕空间，无需强制刷写）")
    ]
    // swiftlint:enable line_length

    @Published public var analyzeEntries: [DiskAnalyzeEntry] = [
        DiskAnalyzeEntry(name: "Library (系统与应用库)", path: "\(homePath)/Library", size: 94918787072, is_dir: true, scan_status: "Complete"),
        DiskAnalyzeEntry(name: "Projects (代码仓库)", path: "\(homePath)/Projects", size: 67001489817, is_dir: true, scan_status: "Complete"),
        DiskAnalyzeEntry(name: "Applications (应用程序)", path: "/Applications", size: 50251117363, is_dir: true, scan_status: "Complete"),
        DiskAnalyzeEntry(name: "Downloads (下载目录)", path: "\(homePath)/Downloads", size: 31031138713, is_dir: true, scan_status: "Complete"),
        DiskAnalyzeEntry(name: "Documents (个人文稿)", path: "\(homePath)/Documents", size: 15247133900, is_dir: true, scan_status: "Complete"),
        DiskAnalyzeEntry(name: "Movies (视频媒体)", path: "\(homePath)/Movies", size: 8375186227, is_dir: true, scan_status: "Complete")
    ]

    @Published public var largeFiles: [DiskLargeFile] = [
        DiskLargeFile(name: "Docker.raw (Linux 容器虚拟磁盘镜像)", path: "~/Library/Containers/com.docker.docker/Data/vms/0/data/Docker.raw", size: 25769803776),
        DiskLargeFile(name: "Xcode_16.2_Universal.xip (离线安装包)", path: "~/Downloads/Xcode_16.2_Universal.xip", size: 9040905830),
        DiskLargeFile(name: "MacTeX-2026.pkg (Homebrew 缓存包)", path: "~/Library/Caches/Homebrew/downloads/MacTeX-2026.pkg", size: 5207647846)
    ]

    @Published public var processes: [MoleStatusSnapshot.ProcessItem] = [
        .init(pid: 1842, name: "Cursor", cpu: 24.6, memory_mb: 2840, power: 8.4, pinned: true, path: "/Applications/Cursor.app/Contents/MacOS/Cursor"),
        .init(pid: 2190, name: "Xcode", cpu: 18.2, memory_mb: 3420, power: 6.1, pinned: true, path: "/Applications/Xcode.app/Contents/MacOS/Xcode"),
        .init(pid: 941, name: "WindowServer", cpu: 11.4, memory_mb: 980, power: 3.7, pinned: false, path: "/System/Library/PrivateFrameworks/SkyLight.framework/Resources/WindowServer"),
        .init(pid: 4242, name: "Google Chrome", cpu: 9.8, memory_mb: 3650, power: 4.2, pinned: false, path: "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"),
        .init(pid: 5110, name: "Docker Desktop", cpu: 6.5, memory_mb: 2190, power: 2.8, pinned: false, path: "/Applications/Docker.app/Contents/MacOS/Docker")
    ]

    // swiftlint:disable line_length
    @Published public var purgeArtifacts: [PurgeArtifactItem] = [
        PurgeArtifactItem(id: "p1", projectPath: "~/Projects/rust-indexer", artifactName: "target", kindText: "Rust Cargo 构建产物", sizeGB: 4.62, ageDescription: "48 天未修改", gitStatusText: "分支 main · 工作区干净", rebuildCommand: "cargo build --release", isRecentUnder7Days: false, isSelected: true),
        PurgeArtifactItem(id: "p2", projectPath: "~/GitHub/ios-client", artifactName: "DerivedData", kindText: "Xcode 工程构建缓存", sizeGB: 3.15, ageDescription: "21 天未修改", gitStatusText: "分支 main · 工作区干净", rebuildCommand: "xcodebuild -scheme App", isRecentUnder7Days: false, isSelected: true),
        PurgeArtifactItem(id: "p3", projectPath: "~/Projects/mole-studio", artifactName: "node_modules", kindText: "Node.js 依赖目录", sizeGB: 1.84, ageDescription: "34 天未修改", gitStatusText: "分支 feat/ui · 工作区干净", rebuildCommand: "pnpm install", isRecentUnder7Days: false, isSelected: true),
        PurgeArtifactItem(id: "p4", projectPath: "~/Projects/ai-agent", artifactName: ".venv", kindText: "Python 虚拟环境", sizeGB: 1.28, ageDescription: "19 天未修改", gitStatusText: "分支 main · 工作区干净", rebuildCommand: "uv sync", isRecentUnder7Days: false, isSelected: true),
        PurgeArtifactItem(id: "p5", projectPath: "~/Projects/active-webapp", artifactName: ".next", kindText: "Next.js 编译缓存", sizeGB: 0.41, ageDescription: "2 天内活跃 (自动保护)", gitStatusText: "存在未提交修改 (自动保护)", rebuildCommand: "pnpm build", isRecentUnder7Days: true, isSelected: false)
    ]
    // swiftlint:enable line_length

    @Published public var installerPackages: [InstallerPackageItem] = [
        InstallerPackageItem(id: "i1", fileName: "Xcode_16.2_Universal.xip", sourceLocation: "~/Downloads", sizeGB: 8.42, isSelected: true),
        InstallerPackageItem(id: "i2", fileName: "MacTeX-2026.pkg", sourceLocation: "~/Library/Caches/Homebrew/downloads", sizeGB: 4.85, isSelected: true),
        InstallerPackageItem(id: "i3", fileName: "Docker-AppleSilicon.dmg", sourceLocation: "~/Downloads", sizeGB: 0.78, isSelected: true)
    ]

    @Published public var historyLogs: [OperationHistoryLog] = [
        OperationHistoryLog(id: "#20261014-1148", timeText: "今天 11:48", actionTitle: "深度清理 (Xcode DerivedData 与应用缓存)", sizeGB: 12.49),
        OperationHistoryLog(id: "#20261012-1920", timeText: "10月12日 19:20", actionTitle: "AI 专项清理 (清理 11 个旧版本 CLI 二进制包)", sizeGB: 5.60),
        OperationHistoryLog(id: "#20261009-1015", timeText: "10月09日 10:15", actionTitle: "项目产物清理 (4 个陈旧 node_modules / target)", sizeGB: 10.89)
    ]

    @Published public var whitelistPaths: [String] = [
        "~/.ollama/models (本地大模型权重)",
        "~/.cache/huggingface (HF 数据集与模型缓存)",
        "~/.claude/projects (AI 历史会话与记忆)",
        "~/Library/Caches/com.apple.e5rt.e5bundlecache (Apple 神经网络引擎编译缓存)",
        "~/Documents/Financials (用户自定义保护目录)"
    ]

    @Published public var doctorChecks: [DoctorDiagnosticItem] = [
        DoctorDiagnosticItem(id: "fda", title: "完全磁盘访问权限 (Full Disk Access / TCC)", detail: "已授权访问 ~/Library/Caches、~/Library/Logs 与 Safari 容器", passed: true),
        DoctorDiagnosticItem(id: "mem", title: "系统统一内存压力评估", detail: "压力正常（已用 18.7 GB / 32.0 GB，Swap 交换仅 256 MB）", passed: true),
        DoctorDiagnosticItem(id: "disk", title: "磁盘剩余空间与 SMART 寿命状态", detail: "剩余 302.1 GB 可用 (45.8%) · NVMe SMART 状态验证通过", passed: true),
        DoctorDiagnosticItem(id: "bat", title: "电池循环健康度与散热表现", detail: "最大容量 98%（142 次循环）· 待机核心温度 31.4°C", passed: true),
        DoctorDiagnosticItem(id: "up", title: "内核运行时间与重启建议", detail: "已连续运行 6 天 14 小时 · 无挂起的 macOS 系统更新锁", passed: true),
        DoctorDiagnosticItem(id: "log", title: "操作审计日志可写性检查", detail: "正常写入 ~/Library/Logs/mole/operations.log", passed: true),
        DoctorDiagnosticItem(id: "ane", title: "硬件与 AI 核心数据保护锁", detail: "Apple 神经网络引擎缓存 (e5bundlecache) 与 ~/.claude/projects 已锁定保护", passed: true)
    ]

    public init() {}

    public var totalSelectedCleanGB: Double {
        cleanItems.filter { $0.isSelected }.reduce(0) { $0 + $1.sizeGB }
    }

    public var totalSelectedCleanItemsCount: Int {
        cleanItems.filter { $0.isSelected }.reduce(0) { $0 + $1.itemCount }
    }

    public func toggleCleanSelection(id: String) {
        guard let idx = cleanItems.firstIndex(where: { $0.id == id }) else { return }
        guard cleanItems[idx].tier != .protectedSystem else { return }
        cleanItems[idx].isSelected.toggle()
    }

    public func revealInFinder(_ rawPath: String) {
        let cleaned = rawPath.components(separatedBy: " ").first ?? rawPath
        let expanded = (cleaned as NSString).expandingTildeInPath
        NSWorkspace.shared.selectFile(nil, inFileViewerRootedAtPath: expanded)
        showToast("已在 Finder 中打开 \(cleaned)")
    }

    public func copyToPasteboard(_ text: String) {
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(text, forType: .string)
        showToast("已复制 CLI 命令至剪贴板")
    }

    public func addToWhitelist(_ path: String) {
        if !whitelistPaths.contains(path) {
            whitelistPaths.append(path)
        }
        showToast("已将 \(path) 加入白名单保护")
    }

    public func runCLISubcommand(_ args: [String]) {
        Task {
            let res = await MoleCLIRunner.shared.execute(args)
            cliOutputLog.append("$ mo \(args.joined(separator: " ")) (exit \(res.exitCode), \(res.elapsedMs)ms)")
            showToast("已完成: mo \(args.joined(separator: " "))")
        }
    }

    public func showToast(_ msg: String) {
        toastMessage = msg
        Task {
            try? await Task.sleep(nanoseconds: 2_600_000_000)
            if toastMessage == msg { toastMessage = nil }
        }
    }
}
