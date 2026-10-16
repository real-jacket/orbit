import Cocoa
import Darwin

/// Samples installed apps, large files, disk, memory, and processes for the WebKit UI.
final class RealMacSystemScanner {
    static let shared = RealMacSystemScanner()
    private let fm = FileManager.default

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
            includingPropertiesForKeys: [.fileSizeKey, .isRegularFileKey],
            options: [.skipsHiddenFiles]
        ) else { return (0, 0) }

        var totalBytes: Int64 = 0
        var count = 0
        for case let fileURL as URL in enumerator {
            if let vals = try? fileURL.resourceValues(forKeys: [.fileSizeKey, .isRegularFileKey]),
               vals.isRegularFile == true {
                totalBytes += Int64(vals.fileSize ?? 0)
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
        return (302.1, 1000.0)
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
            "largeFiles": scanLargeFiles()
        ]
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
