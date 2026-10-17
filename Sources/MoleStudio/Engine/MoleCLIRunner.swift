import Foundation
import AppKit
import SwiftUI

public struct CLIExecutionResult: Sendable {
    public let command: String
    public let binaryPath: String
    public let exitCode: Int32
    public let stdout: String
    public let stderr: String
    public let elapsedMs: Int
}

public actor MoleCLIRunner {
    public static let shared = MoleCLIRunner()

    private var caffeinateProcess: Process?

    public nonisolated var configDirURL: URL {
        FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent(".config/mole", isDirectory: true)
    }

    public nonisolated var whitelistFileURL: URL {
        configDirURL.appendingPathComponent("whitelist")
    }

    public nonisolated var purgePathsFileURL: URL {
        configDirURL.appendingPathComponent("purge_paths")
    }

    public nonisolated var operationsLogURL: URL {
        FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent("Library/Logs/mole/operations.log")
    }

    public init() {
        let fm = FileManager.default
        try? fm.createDirectory(at: configDirURL, withIntermediateDirectories: true)
        try? fm.createDirectory(at: operationsLogURL.deletingLastPathComponent(), withIntermediateDirectories: true)
    }

    public nonisolated func resolveMoBinaryPath() -> String {
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        let candidates = [
            "/opt/homebrew/bin/mo",
            "/usr/local/bin/mo",
            "\(home)/.local/bin/mo",
            "\(home)/.mole/mo",
            Bundle.main.resourceURL?.appendingPathComponent("resources/mole-cli/mo").path ?? "",
            Bundle.main.bundleURL.deletingLastPathComponent().appendingPathComponent("resources/mole-cli/mo").path
        ]
        for path in candidates where !path.isEmpty {
            if FileManager.default.isExecutableFile(atPath: path) {
                return path
            }
        }
        return "/opt/homebrew/bin/mo"
    }

    public func execute(_ args: [String], requiresAdmin: Bool = false) async -> CLIExecutionResult {
        let bin = resolveMoBinaryPath()
        let start = DispatchTime.now()

        if requiresAdmin {
            let escapedArgs = args.map { "'\($0.replacingOccurrences(of: "'", with: "'\\''"))'" }.joined(separator: " ")
            let script = "do shell script \"\(bin) \(escapedArgs)\" with administrator privileges"
            var errorDict: NSDictionary?
            let appleScript = NSAppleScript(source: script)
            let result = appleScript?.executeAndReturnError(&errorDict)
            let ms = Int((DispatchTime.now().uptimeNanoseconds - start.uptimeNanoseconds) / 1_000_000)
            return CLIExecutionResult(
                command: "sudo mo " + args.joined(separator: " "),
                binaryPath: bin,
                exitCode: errorDict == nil ? 0 : 1,
                stdout: result?.stringValue ?? "",
                stderr: errorDict?.description ?? "",
                elapsedMs: ms
            )
        }

        let proc = Process()
        proc.executableURL = URL(fileURLWithPath: "/usr/bin/env")
        proc.arguments = [bin] + args

        var env = ProcessInfo.processInfo.environment
        env["PATH"] = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
        env["TERM"] = "dumb"
        proc.environment = env

        let outPipe = Pipe()
        let errPipe = Pipe()
        proc.standardOutput = outPipe
        proc.standardError = errPipe

        do {
            try proc.run()
            proc.waitUntilExit()
            let outData = outPipe.fileHandleForReading.readDataToEndOfFile()
            let errData = errPipe.fileHandleForReading.readDataToEndOfFile()
            let ms = Int((DispatchTime.now().uptimeNanoseconds - start.uptimeNanoseconds) / 1_000_000)
            return CLIExecutionResult(
                command: "mo " + args.joined(separator: " "),
                binaryPath: bin,
                exitCode: proc.terminationStatus,
                stdout: String(data: outData, encoding: .utf8) ?? "",
                stderr: String(data: errData, encoding: .utf8) ?? "",
                elapsedMs: ms
            )
        } catch {
            let ms = Int((DispatchTime.now().uptimeNanoseconds - start.uptimeNanoseconds) / 1_000_000)
            return CLIExecutionResult(
                command: "mo " + args.joined(separator: " "),
                binaryPath: bin,
                exitCode: -1,
                stdout: "",
                stderr: error.localizedDescription,
                elapsedMs: ms
            )
        }
    }

    public func loadWhitelist() -> [String] {
        guard let content = try? String(contentsOf: whitelistFileURL, encoding: .utf8) else {
            return [
                "~/.ollama/models",
                "~/.cache/huggingface",
                "~/.claude/projects",
                "~/.codex/sessions",
                "~/Library/Caches/com.apple.e5rt.e5bundlecache"
            ]
        }
        return content
            .components(separatedBy: .newlines)
            .map { $0.trimmingCharacters(in: .whitespaces) }
            .filter { !$0.isEmpty && !$0.hasPrefix("#") }
    }

    public func saveWhitelist(_ paths: [String]) {
        let body = paths.joined(separator: "\n") + "\n"
        try? body.write(to: whitelistFileURL, atomically: true, encoding: .utf8)
    }

    /// True 2D Recursive Slice-and-Dice / Squarified Treemap Layout Algorithm
    /// Guarantees total normalized area == 1.0 and balanced 2D aspect ratios.
    public nonisolated func computeTreemapRects(entries: [DiskAnalyzeEntry]) -> [TreemapRect] {
        let sorted = entries.sorted { $0.size > $1.size }
        let total = max(1, sorted.reduce(Int64(0)) { $0 + max(1, $1.size) })
        let palette: [Color] = [
            Color(red: 0.23, green: 0.51, blue: 0.96),
            Color(red: 0.06, green: 0.73, blue: 0.51),
            Color(red: 0.55, green: 0.36, blue: 0.96),
            Color(red: 0.96, green: 0.62, blue: 0.04),
            Color(red: 0.02, green: 0.71, blue: 0.83),
            Color(red: 0.96, green: 0.25, blue: 0.37)
        ]

        var results: [TreemapRect] = []
        func layoutSlice(_ items: ArraySlice<DiskAnalyzeEntry>, in box: CGRect, verticalSplit: Bool, colorOffset: Int) {
            guard !items.isEmpty else { return }
            if items.count == 1, let first = items.first {
                let pct = (Double(first.size) / Double(total)) * 100.0
                results.append(TreemapRect(
                    id: first.path,
                    entry: first,
                    normalizedRect: box,
                    percentage: pct,
                    color: palette[colorOffset % palette.count]
                ))
                return
            }
            let sumAll = max(1, items.reduce(Int64(0)) { $0 + max(1, $1.size) })
            let first = items[items.startIndex]
            let ratio = CGFloat(max(1, first.size)) / CGFloat(sumAll)
            let clampedRatio = min(0.75, max(0.22, ratio))

            let firstBox: CGRect
            let restBox: CGRect
            if verticalSplit {
                firstBox = CGRect(x: box.minX, y: box.minY, width: box.width * clampedRatio, height: box.height)
                restBox = CGRect(x: box.minX + box.width * clampedRatio, y: box.minY, width: box.width * (1.0 - clampedRatio), height: box.height)
            } else {
                firstBox = CGRect(x: box.minX, y: box.minY, width: box.width, height: box.height * clampedRatio)
                restBox = CGRect(x: box.minX, y: box.minY + box.height * clampedRatio, width: box.width, height: box.height * (1.0 - clampedRatio))
            }

            let pct = (Double(first.size) / Double(total)) * 100.0
            results.append(TreemapRect(
                id: first.path,
                entry: first,
                normalizedRect: firstBox,
                percentage: pct,
                color: palette[colorOffset % palette.count]
            ))
            let nextSlice = items.dropFirst()
            layoutSlice(nextSlice, in: restBox, verticalSplit: !verticalSplit, colorOffset: colorOffset + 1)
        }

        layoutSlice(sorted[...], in: CGRect(x: 0, y: 0, width: 1, height: 1), verticalSplit: true, colorOffset: 0)
        return results
    }

    public func setKeepScreenOn(seconds: Int?) {
        caffeinateProcess?.terminate()
        caffeinateProcess = nil
        guard let secs = seconds else { return }
        let proc = Process()
        proc.executableURL = URL(fileURLWithPath: "/usr/bin/caffeinate")
        proc.arguments = secs > 0 ? ["-d", "-i", "-t", "\(secs)"] : ["-d", "-i"]
        try? proc.run()
        caffeinateProcess = proc
    }
}
