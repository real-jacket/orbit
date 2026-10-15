import Foundation
import SwiftUI

public enum MoleModule: String, CaseIterable, Identifiable, Sendable {
    case clean = "clean"
    case aiMoon = "ai_moon"
    case software = "software"
    case optimize = "optimize"
    case analyze = "analyze"
    case status = "status"
    case purge = "purge"
    case history = "history"

    public var id: String { rawValue }

    public func sidebarTitle(isZH: Bool) -> String {
        switch self {
        case .clean:    return isZH ? "深度清理 · Earth" : "Deep Clean · Earth"
        case .aiMoon:   return isZH ? "AI 清理 · Moon" : "AI Care · Moon"
        case .software: return isZH ? "软件卸载 · Mars" : "Uninstaller · Mars"
        case .optimize: return isZH ? "系统优化 · Mercury" : "Optimize · Mercury"
        case .analyze:  return isZH ? "磁盘分析 · Jupiter" : "Disk Analyze · Jupiter"
        case .status:   return isZH ? "状态监控 · Sun" : "Live Status · Sun"
        case .purge:    return isZH ? "构建与安装包 · Purge" : "Build & Installers · Purge"
        case .history:  return isZH ? "操作记录与白名单" : "History & Whitelist"
        }
    }

    public func headerTitle(isZH: Bool) -> String {
        switch self {
        case .clean:    return isZH ? "深度清理 (Earth 🌍)" : "Deep Clean (Earth 🌍)"
        case .aiMoon:   return isZH ? "AI 清理 (Moon 🌕)" : "AI Care (Moon 🌕)"
        case .software: return isZH ? "软件卸载 (Mars 🔴)" : "App Uninstaller (Mars 🔴)"
        case .optimize: return isZH ? "系统优化 (Mercury 🪐)" : "System Optimize (Mercury 🪐)"
        case .analyze:  return isZH ? "磁盘分析 (Jupiter 🟠)" : "Disk Analyzer (Jupiter 🟠)"
        case .status:   return isZH ? "状态监控 (Sun ☀️)" : "Hardware Status (Sun ☀️)"
        case .purge:    return isZH ? "构建产物与安装包清理 (Purge & Installer)" : "Build Artifacts & Installers"
        case .history:  return isZH ? "操作记录与白名单保护" : "Trash History & Whitelist"
        }
    }

    public var sidebarBadge: String {
        switch self {
        case .clean:    return "12.5 GB"
        case .aiMoon:   return "12.2 GB"
        case .software: return "12.8 GB"
        case .optimize: return "7 项"
        case .analyze:  return "2D 树图"
        case .status:   return "实时"
        case .purge:    return "24.9 GB"
        case .history:  return "3 快照"
        }
    }

    public var cliBadgeText: String {
        switch self {
        case .clean:    return "$ mo clean --dry-run"
        case .aiMoon:   return "$ mo clean --ai-care --retention 30d"
        case .software: return "$ mo uninstall --dry-run"
        case .optimize: return "$ mo optimize"
        case .analyze:  return "$ mo analyze --json ~"
        case .status:   return "$ mo status --json --watch"
        case .purge:    return "$ mo purge && mo installer"
        case .history:  return "$ mo whitelist"
        }
    }

    public var dotColor: Color {
        switch self {
        case .clean:    return Color(red: 0.29, green: 0.51, blue: 0.94)
        case .aiMoon:   return Color(red: 0.85, green: 0.47, blue: 0.34)
        case .software: return Color(red: 0.88, green: 0.32, blue: 0.39)
        case .optimize: return Color(red: 0.61, green: 0.43, blue: 0.95)
        case .analyze:  return Color(red: 0.85, green: 0.61, blue: 0.15)
        case .status:   return Color(red: 0.92, green: 0.70, blue: 0.03)
        case .purge:    return Color(red: 0.18, green: 0.62, blue: 0.42)
        case .history:  return Color(red: 0.54, green: 0.52, blue: 0.49)
        }
    }

    public var primaryColor: Color { dotColor }

    public var hasPlanetaryRing: Bool {
        self == .optimize || self == .analyze
    }
}

public enum SafetyTier: String, Codable, Sendable {
    case safeRegenerable = "safe"
    case reviewOnly = "review"
    case protectedSystem = "protected"

    public func label(isZH: Bool) -> String {
        switch self {
        case .safeRegenerable: return isZH ? "安全可再生" : "Safe"
        case .reviewOnly:      return isZH ? "需确认" : "Review"
        case .protectedSystem: return isZH ? "硬件保护" : "Protected"
        }
    }

    public var color: Color {
        switch self {
        case .safeRegenerable: return Color(red: 0.18, green: 0.62, blue: 0.42)
        case .reviewOnly:      return Color(red: 0.85, green: 0.61, blue: 0.15)
        case .protectedSystem: return Color(red: 0.61, green: 0.43, blue: 0.95)
        }
    }
}

public struct SubpathEntry: Identifiable, Hashable, Sendable {
    public let id: String
    public var path: String
    public var sizeText: String
    public var isSelected: Bool

    public init(path: String, sizeText: String, isSelected: Bool = true) {
        self.id = path
        self.path = path
        self.sizeText = sizeText
        self.isSelected = isSelected
    }
}

public struct CleanCategoryItem: Identifiable, Hashable, Sendable {
    public let id: String
    public var category: String
    public var name: String
    public var path: String
    public var sizeGB: Double
    public var itemCount: Int
    public var filesCountText: String
    public var lastModifiedText: String
    public var rebuildTimeText: String
    public var cliCommand: String
    public var impactDescription: String
    public var tier: SafetyTier
    public var isSelected: Bool
    public var subpaths: [SubpathEntry]
}

public enum AIRetentionWindow: String, CaseIterable, Identifiable, Sendable {
    case days15 = "保留 15 天"
    case days30 = "保留 30 天"
    case days90 = "保留 90 天"
    public var id: String { rawValue }
}

public struct AIMoonItem: Identifiable, Hashable, Sendable {
    public let id: String
    public var bandTitle: String
    public var name: String
    public var path: String
    public var sizeGB: Double
    public var filesCountText: String
    public var lastModifiedText: String
    public var rebuildTimeText: String
    public var cliCommand: String
    public var impactDescription: String
    public var tier: SafetyTier
    public var isSelected: Bool
    public var subpaths: [SubpathEntry]
}

public struct InstalledAppBundle: Identifiable, Hashable, Sendable {
    public let id: String
    public var name: String
    public var bundleIdentifier: String
    public var version: String
    public var lastUsed: String
    public var appSizeGB: Double
    public var leftoverSizeGB: Double
    public var isSelected: Bool
    public var keepPersonalData: Bool
    public var impactDescription: String
    public var subpaths: [SubpathEntry]
    public var totalSizeGB: Double { appSizeGB + leftoverSizeGB }
}

public enum OptimizeTaskStatus: String, Sendable {
    case applied
    case skipped
}

public struct OptimizeTaskItem: Identifiable, Hashable, Sendable {
    public let id: String
    public var title: String
    public var command: String
    public var status: OptimizeTaskStatus
    public var reasonOrDetail: String
}

public struct DiskAnalyzeEntry: Identifiable, Codable, Hashable, Sendable {
    public var id: String { path }
    public let name: String
    public let path: String
    public let size: Int64
    public let is_dir: Bool
    public let scan_status: String?
    public var sizeGB: Double { Double(size) / 1_073_741_824.0 }
}

public struct DiskLargeFile: Identifiable, Codable, Hashable, Sendable {
    public var id: String { path }
    public let name: String
    public let path: String
    public let size: Int64
    public var sizeGB: Double { Double(size) / 1_073_741_824.0 }
}

public struct TreemapRect: Identifiable, Sendable {
    public let id: String
    public let entry: DiskAnalyzeEntry
    public let normalizedRect: CGRect
    public let percentage: Double
    public let color: Color
}

public struct MoleStatusSnapshot: Codable, Sendable {
    public struct ProcessItem: Codable, Identifiable, Hashable, Sendable {
        public var id: Int { pid }
        public let pid: Int
        public let name: String
        public let cpu: Double
        public let memory_mb: Int
        public let power: Double
        public var pinned: Bool
        public let path: String
    }
}

public struct PurgeArtifactItem: Identifiable, Hashable, Sendable {
    public let id: String
    public var projectPath: String
    public var artifactName: String
    public var kindText: String
    public var sizeGB: Double
    public var ageDescription: String
    public var gitStatusText: String
    public var rebuildCommand: String
    public var isRecentUnder7Days: Bool
    public var isSelected: Bool
}

public struct InstallerPackageItem: Identifiable, Hashable, Sendable {
    public let id: String
    public var fileName: String
    public var sourceLocation: String
    public var sizeGB: Double
    public var isSelected: Bool
}

public struct OperationHistoryLog: Identifiable, Hashable, Sendable {
    public let id: String
    public var timeText: String
    public var actionTitle: String
    public var sizeGB: Double
}

public struct DoctorDiagnosticItem: Identifiable, Hashable, Sendable {
    public let id: String
    public var title: String
    public var detail: String
    public var passed: Bool
}
