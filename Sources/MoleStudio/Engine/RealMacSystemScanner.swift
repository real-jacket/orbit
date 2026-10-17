import Cocoa
import Darwin

/// Samples installed apps, leftover directories, large files, and telemetry for the WebKit UI.
final class RealMacSystemScanner {
    static let shared = RealMacSystemScanner()
    /// Shared by the extensions in this folder (orphan scanning, telemetry).
    let fm = FileManager.default

    func fastDirectorySize(path: String, maxFiles: Int = 2500) -> (bytes: Int64, count: Int) {
        let expanded = (path as NSString).expandingTildeInPath
        var isDir: ObjCBool = false
        guard fm.fileExists(atPath: expanded, isDirectory: &isDir) else { return (0, 0) }
        if !isDir.boolValue {
            let size = (try? fm.attributesOfItem(atPath: expanded)[.size] as? Int64) ?? 0
            return (size, 1)
        }
        let url = URL(fileURLWithPath: expanded)
        guard let enumerator = fm.enumerator(
            at: url,
            includingPropertiesForKeys: [.fileSizeKey, .totalFileAllocatedSizeKey, .isRegularFileKey],
            options: []
        ) else { return (0, 0) }

        var totalBytes: Int64 = 0
        var count = 0
        for case let fileURL as URL in enumerator {
            if let vals = try? fileURL.resourceValues(forKeys: [.fileSizeKey, .totalFileAllocatedSizeKey, .isRegularFileKey]),
               vals.isRegularFile == true {
                let actual = vals.totalFileAllocatedSize ?? vals.fileSize ?? 0
                totalBytes += Int64(actual)
                count += 1
                if count >= maxFiles { break }
            }
        }
        return (totalBytes, count)
    }

    func formatBytes(_ bytes: Int64) -> String {
        let gb = Double(bytes) / 1_000_000_000.0
        if gb >= 0.1 { return String(format: "%.2f GB", gb) }
        let mb = Double(bytes) / 1_000_000.0
        return String(format: "%.0f MB", max(1, mb))
    }

    func queryDiskSpace() -> (freeGb: Double, totalGb: Double) {
        var fs = statfs()
        if statfs(NSHomeDirectory(), &fs) == 0 {
            let freeBytes = Double(UInt64(fs.f_bavail) * UInt64(fs.f_bsize))
            let totalBytes = Double(UInt64(fs.f_blocks) * UInt64(fs.f_bsize))
            return (freeBytes / 1_000_000_000.0, totalBytes / 1_000_000_000.0)
        }
        return (0, 0)
    }

    func queryMemoryUsage() -> (usedGb: Double, totalGb: Double) {
        let totalBytes = Double(ProcessInfo.processInfo.physicalMemory)
        let totalGb = totalBytes / 1_073_741_824.0
        var stats = vm_statistics64()
        var count = mach_msg_type_number_t(MemoryLayout<vm_statistics64>.size / MemoryLayout<integer_t>.size)
        let hostPort = mach_host_self()
        let result = withUnsafeMutablePointer(to: &stats) {
            $0.withMemoryRebound(to: integer_t.self, capacity: Int(count)) {
                host_statistics64(hostPort, HOST_VM_INFO64, $0, &count)
            }
        }
        if result == KERN_SUCCESS {
            let pageSize = Double(vm_kernel_page_size)
            let usedPages = Double(stats.active_count + stats.wire_count + stats.compressor_page_count)
            let usedGb = min(totalGb * 0.96, (usedPages * pageSize) / 1_073_741_824.0)
            return (usedGb, totalGb)
        }
        return (totalGb * 0.55, totalGb)
    }

    func queryCPUModel() -> String {
        var size = 0
        sysctlbyname("machdep.cpu.brand_string", nil, &size, nil, 0)
        if size > 0 {
            var buffer = [CChar](repeating: 0, count: size)
            sysctlbyname("machdep.cpu.brand_string", &buffer, &size, nil, 0)
            let brand = String(cString: buffer).trimmingCharacters(in: .whitespacesAndNewlines)
            if !brand.isEmpty {
                return "\(brand) (\(ProcessInfo.processInfo.activeProcessorCount)核)"
            }
        }
        return "Apple Silicon (\(ProcessInfo.processInfo.activeProcessorCount)核)"
    }

    func sampleRealProcesses() -> (cpuTotal: Double, processes: [[String: Any]]) {
        let proc = Process()
        proc.executableURL = URL(fileURLWithPath: "/bin/ps")
        proc.arguments = ["-Aceo", "pid,%cpu,rss,comm", "-r"]
        let pipe = Pipe()
        proc.standardOutput = pipe
        proc.standardError = Pipe()
        var list: [[String: Any]] = []
        var cpuSum = 0.0

        do {
            try proc.run()
            let data = pipe.fileHandleForReading.readDataToEndOfFile()
            proc.waitUntilExit()
            let output = String(data: data, encoding: .utf8) ?? ""
            for line in output.components(separatedBy: .newlines).dropFirst() {
                let trimmed = line.trimmingCharacters(in: .whitespaces)
                if trimmed.isEmpty { continue }
                let parts = trimmed.split(separator: " ", maxSplits: 3, omittingEmptySubsequences: true)
                if parts.count < 4 { continue }
                guard let pid = Int(parts[0]), pid > 1 else { continue }
                let cpu = Double(parts[1]) ?? 0
                let rssKb = Double(parts[2]) ?? 0
                let commRaw = String(parts[3]).trimmingCharacters(in: .whitespaces)
                if commRaw == "ps" || commRaw == "kernel_task" { continue }
                cpuSum += cpu
                if list.count < 10 {
                    var fullPath = commRaw
                    if let app = NSRunningApplication(processIdentifier: pid_t(pid)),
                       let bundleURL = app.bundleURL {
                        fullPath = bundleURL.path
                    }
                    list.append([
                        "pid": pid,
                        "name": (commRaw as NSString).lastPathComponent,
                        "cpu": cpu,
                        "memory_mb": Int(max(1.0, rssKb / 1024.0)),
                        "power": 0,
                        "pinned": false,
                        "path": fullPath
                    ])
                }
            }
        } catch {
            return (2.5, [])
        }

        let cores = Double(max(1, ProcessInfo.processInfo.activeProcessorCount))
        return (min(99.9, max(2.5, cpuSum / cores)), list)
    }

    func scanInstalledApps() -> [[String: Any]] {
        var results: [[String: Any]] = []
        let appDirs = ["/Applications", (NSHomeDirectory() as NSString).appendingPathComponent("Applications")]
        let home = NSHomeDirectory()

        for dir in appDirs {
            guard let items = try? fm.contentsOfDirectory(atPath: dir) else { continue }
            for item in items where item.hasSuffix(".app") {
                if item.contains("MoleStudio") || item == "Safari.app" { continue }
                let fullPath = (dir as NSString).appendingPathComponent(item)
                let bundle = Bundle(path: fullPath)
                let info = bundle?.infoDictionary
                let name = (info?["CFBundleDisplayName"] as? String)
                    ?? (info?["CFBundleName"] as? String)
                    ?? (item as NSString).deletingPathExtension
                let bundleId = bundle?.bundleIdentifier ?? "local.app.\(name)"
                let version = (info?["CFBundleShortVersionString"] as? String) ?? "1.0"
                let (appBytes, _) = fastDirectorySize(path: fullPath, maxFiles: 1200)
                let appGb = max(0.05, Double(appBytes) / 1_000_000_000.0)

                var subpaths: [[String: Any]] = [[
                    "path": fullPath,
                    "size": formatBytes(max(50_000_000, appBytes)),
                    "kind": "app",
                    "checked": true
                ]]
                var leftoverBytes: Int64 = 0
                let cacheCandidates = [
                    "\(home)/Library/Caches/\(bundleId)",
                    "\(home)/Library/Caches/\(name)"
                ]
                let configCandidates = [
                    "\(home)/Library/Application Support/\(name)",
                    "\(home)/Library/Application Support/\(bundleId)",
                    "\(home)/Library/Containers/\(bundleId)",
                    "\(home)/Library/Saved Application State/\(bundleId).savedState",
                    "\(home)/Library/Preferences/\(bundleId).plist"
                ]
                leftoverBytes += appendExisting(
                    cacheCandidates,
                    kind: "cache",
                    checked: true,
                    home: home,
                    into: &subpaths
                )
                leftoverBytes += appendExisting(
                    configCandidates,
                    kind: "config",
                    checked: false,
                    home: home,
                    into: &subpaths
                )

                let leftoverGb = Double(leftoverBytes) / 1_000_000_000.0
                let cacheGb = leftoverGb * 0.8
                results.append([
                    "id": bundleId,
                    "name": name,
                    "category": "Mac 应用",
                    "appPath": fullPath,
                    "bundleId": bundleId,
                    "version": version,
                    "lastUsed": "本机已安装",
                    "appSizeGb": (appGb * 100).rounded() / 100.0,
                    "leftoverSizeGb": (leftoverGb * 100).rounded() / 100.0,
                    "cacheSizeGb": (cacheGb * 100).rounded() / 100.0,
                    "configSizeGb": (max(0, leftoverGb - cacheGb) * 100).rounded() / 100.0,
                    "checked": results.isEmpty,
                    "keepPersonal": true,
                    "impact": "卸载时将把 \(fullPath) 及 \(subpaths.count - 1) 处 ~/Library 关联缓存移入废纸篓 (~/.Trash)。",
                    "subpaths": subpaths
                ])
                if results.count >= 16 { break }
            }
            if results.count >= 16 { break }
        }

        results.sort {
            let left = ($0["appSizeGb"] as? Double ?? 0) + ($0["leftoverSizeGb"] as? Double ?? 0)
            let right = ($1["appSizeGb"] as? Double ?? 0) + ($1["leftoverSizeGb"] as? Double ?? 0)
            return left > right
        }
        return results
    }

    func matchingWildcardPaths(in parentDir: String, prefix: String) -> [String] {
        let expanded = (parentDir as NSString).expandingTildeInPath
        guard let items = try? fm.contentsOfDirectory(atPath: expanded) else { return [] }
        return items
            .filter { prefix.isEmpty || $0.lowercased().hasPrefix(prefix.lowercased()) }
            .map { (expanded as NSString).appendingPathComponent($0) }
    }

    /// Names and bundle identifiers of every app currently in /Applications, used
    /// to decide whether a leftover directory still has an owner.
    func installedAppIndex(home: String) -> (names: Set<String>, bundleIds: Set<String>) {
        let appDirs = ["/Applications", (home as NSString).appendingPathComponent("Applications")]
        var installedNames = Set<String>()
        var installedBundleIds = Set<String>()

        for dir in appDirs {
            guard let items = try? fm.contentsOfDirectory(atPath: dir) else { continue }
            for item in items where item.hasSuffix(".app") {
                let fullPath = (dir as NSString).appendingPathComponent(item)
                installedNames.insert((item as NSString).deletingPathExtension.lowercased())
                guard let bundle = Bundle(path: fullPath) else { continue }
                if let bid = bundle.bundleIdentifier?.lowercased() {
                    installedBundleIds.insert(bid)
                }
                if let display = (bundle.infoDictionary?["CFBundleDisplayName"] as? String)?.lowercased() {
                    installedNames.insert(display)
                }
                if let bundleName = (bundle.infoDictionary?["CFBundleName"] as? String)?.lowercased() {
                    installedNames.insert(bundleName)
                }
            }
        }
        return (installedNames, installedBundleIds)
    }

    func scanLargeFiles() -> [[String: Any]] {
        let home = NSHomeDirectory()
        let roots = [
            "\(home)/Downloads",
            "\(home)/Documents",
            "\(home)/Desktop",
            "\(home)/Movies"
        ]
        var found: [(name: String, path: String, bytes: Int64, kind: String)] = []
        for root in roots {
            guard fm.fileExists(atPath: root),
                  let enumerator = fm.enumerator(
                    at: URL(fileURLWithPath: root),
                    includingPropertiesForKeys: [.fileSizeKey, .isRegularFileKey],
                    options: [.skipsHiddenFiles, .skipsPackageDescendants]
                  ) else { continue }
            var scanned = 0
            for case let url as URL in enumerator {
                scanned += 1
                if scanned > 1800 { break }
                if let vals = try? url.resourceValues(forKeys: [.fileSizeKey, .isRegularFileKey]),
                   vals.isRegularFile == true,
                   let size = vals.fileSize, size > 15_000_000 {
                    let ext = url.pathExtension.lowercased()
                    let kind = ["dmg", "pkg", "xip", "iso", "zip"].contains(ext)
                        ? "安装归档"
                        : (["mp4", "mov", "mkv"].contains(ext) ? "视频媒体" : "大体积文件")
                    found.append((
                        url.lastPathComponent,
                        url.path.replacingOccurrences(of: home, with: "~"),
                        Int64(size),
                        kind
                    ))
                }
            }
        }
        found.sort { $0.bytes > $1.bytes }
        return found.prefix(10).enumerated().map { idx, item in
            [
                "id": "real_lf_\(idx)",
                "name": item.name,
                "path": item.path,
                "sizeGb": max(0.02, (Double(item.bytes) / 1_000_000_000.0 * 100).rounded() / 100.0),
                "kind": item.kind
            ]
        }
    }

    func buildFullScanPayload() -> [String: Any] {
        let disk = queryDiskSpace()
        let mem = queryMemoryUsage()
        return [
            "freeDiskGb": disk.freeGb,
            "totalDiskGb": disk.totalGb,
            "memTotalGb": mem.totalGb,
            "cpuModel": queryCPUModel(),
            "appsList": scanInstalledApps(),
            "orphanLeftoversList": scanOrphanedLeftovers(),
            "largeFiles": scanLargeFiles(),
            "installerFiles": scanInstallerPackages(),
            "purgeItems": scanBuildArtifacts(),
            "cleanGroups": scanCleanGroups(),
            "aiMoonBands": scanAICaches(),
            "whitelist": loadWhitelistEntries()
        ]
    }

    func scanHomeTreemap() -> [[String: Any]] {
        let home = NSHomeDirectory()
        let names = ["Library", "Downloads", "Documents", "Desktop", "Movies", "Pictures", "Music", "Projects", "Developer"]
        var rows: [(name: String, path: String, bytes: Int64)] = []
        for name in names {
            let path = (home as NSString).appendingPathComponent(name)
            guard fm.fileExists(atPath: path) else { continue }
            let (bytes, _) = fastDirectorySize(path: path, maxFiles: 1800)
            if bytes > 0 { rows.append((name, "~/\(name)", bytes)) }
        }
        let total = rows.reduce(Int64(0)) { $0 + $1.bytes }
        guard total > 0 else { return [] }
        var x = 0.0
        return rows.sorted { $0.bytes > $1.bytes }.map { row in
            let pct = Double(row.bytes) / Double(total) * 100
            let width = max(8, pct)
            let tile: [String: Any] = [
                "name": row.name,
                "path": row.path,
                "sizeGb": gb(row.bytes),
                "pct": (pct * 10).rounded() / 10,
                "color": "linear-gradient(145deg, #3b6fd4, #254b9e)",
                "files": 0,
                "x": x, "y": 0, "w": width, "h": 100,
                "children": [] as [[String: Any]]
            ]
            x += width
            return tile
        }
    }

    func loadWhitelistEntries() -> [String] {
        let url = FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent(".config/mole/whitelist")
        guard let text = try? String(contentsOf: url, encoding: .utf8) else { return [] }
        return text.split(whereSeparator: \.isNewline).map(String.init).filter { !$0.isEmpty && !$0.hasPrefix("#") }
    }

    /// One catalogue entry's identity and copy. Grouping these keeps `catalog`
    /// under the parameter limit without turning the call sites into noise.
    struct CatalogSpec {
        let id: String
        let category: String
        let name: String
        let roots: [String]
        let tier: String
        let impact: String
        let rebuild: String
    }

    func catalog(_ spec: CatalogSpec) -> [String: Any]? {
        let home = NSHomeDirectory()
        let (id, category, name, roots) = (spec.id, spec.category, spec.name, spec.roots)
        let (tier, impact, rebuild) = (spec.tier, spec.impact, spec.rebuild)
        var subpaths: [[String: Any]] = []
        var total: Int64 = 0
        for root in roots where fm.fileExists(atPath: root) {
            let (bytes, count) = fastDirectorySize(path: root, maxFiles: 2500)
            guard bytes > 0 else { continue }
            total += bytes
            subpaths.append([
                "path": root.replacingOccurrences(of: home, with: "~"),
                "size": formatBytes(bytes),
                "sizeGb": gb(bytes),
                "checked": tier != "protected"
            ])
            _ = count
        }
        guard !subpaths.isEmpty else { return nil }
        return [
            "id": id,
            "category": category,
            "name": name,
            "path": subpaths[0]["path"] as? String ?? "",
            "sizeGb": gb(total),
            "origSizeGb": gb(total),
            "items": subpaths.count,
            "filesCount": "\(subpaths.count) 个目录",
            "lastMod": "本机扫描",
            "rebuildTime": rebuild,
            "tier": tier,
            "checked": tier != "protected",
            "cleaned": false,
            "impact": impact,
            "subpaths": subpaths
        ]
    }

    func gb(_ bytes: Int64) -> Double {
        max(0.01, (Double(bytes) / 1_000_000_000.0 * 100).rounded() / 100.0)
    }

    func ageText(_ date: Date?) -> String {
        guard let date else { return "修改时间未知" }
        let days = Int(Date().timeIntervalSince(date) / 86_400)
        if days <= 0 { return "今天修改" }
        return "\(days) 天未修改"
    }

    func movePathsToTrash(_ rawPaths: [String]) -> Int {
        var count = 0
        for raw in rawPaths {
            let clean = raw.components(separatedBy: " ").first ?? raw
            let expanded = (clean as NSString).expandingTildeInPath
            guard fm.fileExists(atPath: expanded) else { continue }
            do {
                var resulting: NSURL?
                try fm.trashItem(at: URL(fileURLWithPath: expanded), resultingItemURL: &resulting)
                count += 1
            } catch {
                let script = "tell application \"Finder\" to delete POSIX file \"\(expanded)\""
                var errDict: NSDictionary?
                NSAppleScript(source: script)?.executeAndReturnError(&errDict)
                if !fm.fileExists(atPath: expanded) { count += 1 }
            }
        }
        return count
    }

    private func appendExisting(
        _ candidates: [String],
        kind: String,
        checked: Bool,
        home: String,
        into subpaths: inout [[String: Any]]
    ) -> Int64 {
        var total: Int64 = 0
        for candidate in candidates where fm.fileExists(atPath: candidate) {
            let (bytes, _) = fastDirectorySize(path: candidate, maxFiles: 600)
            if bytes > 0 {
                total += bytes
                subpaths.append([
                    "path": candidate.replacingOccurrences(of: home, with: "~"),
                    "size": formatBytes(bytes),
                    "kind": kind,
                    "checked": checked
                ])
            }
        }
        return total
    }
}
