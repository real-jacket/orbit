import Foundation
import SwiftUI
import AppKit

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
    @Published public var cliOutputLog: [String] = []
    @Published public var cleanItems: [CleanCategoryItem] = []
    @Published public var aiMoonItems: [AIMoonItem] = []
    @Published public var installedApps: [InstalledAppBundle] = []
    @Published public var optimizeTasks: [OptimizeTaskItem] = []
    @Published public var analyzeEntries: [DiskAnalyzeEntry] = []
    @Published public var largeFiles: [DiskLargeFile] = []
    @Published public var processes: [MoleStatusSnapshot.ProcessItem] = []
    @Published public var purgeArtifacts: [PurgeArtifactItem] = []
    @Published public var installerPackages: [InstallerPackageItem] = []
    @Published public var historyLogs: [OperationHistoryLog] = []
    @Published public var whitelistPaths: [String] = []
    @Published public var doctorChecks: [DoctorDiagnosticItem] = []

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
