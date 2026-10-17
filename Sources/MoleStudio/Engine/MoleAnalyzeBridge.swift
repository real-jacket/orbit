import Foundation

/// Maps `mo analyze --json` onto the Astrolabe treemap.
enum MoleAnalyzeBridge {
    static func overview() -> [String: Any] {
        payload(from: runAnalyze(path: nil).stdout, fallbackSource: "mo analyze --json")
    }

    static func children(path: String) -> [String: Any] {
        payload(from: runAnalyze(path: path).stdout, fallbackSource: "mo analyze --json \(path)")
    }

    private static func runAnalyze(path: String?) -> (stdout: String, source: String) {
        let bin = MoleCLIRunner.shared.resolveMoBinaryPath()
        let proc = Process()
        proc.executableURL = URL(fileURLWithPath: "/usr/bin/env")
        proc.arguments = path.map { [bin, "analyze", "--json", $0] } ?? [bin, "analyze", "--json"]
        var env = ProcessInfo.processInfo.environment
        env["PATH"] = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
        env["TERM"] = "dumb"
        proc.environment = env
        let out = Pipe()
        let err = Pipe()
        proc.standardOutput = out
        proc.standardError = err
        do {
            try proc.run()
            let data = out.fileHandleForReading.readDataToEndOfFile()
            proc.waitUntilExit()
            return (String(data: data, encoding: .utf8) ?? "", "mo analyze --json")
        } catch {
            return ("", "mo analyze unavailable")
        }
    }

    private static func payload(from stdout: String, fallbackSource: String) -> [String: Any] {
        guard let data = stdout.data(using: .utf8),
              let root = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let entries = root["entries"] as? [[String: Any]] else {
            return ["treemap2D": [], "insights": [], "source": fallbackSource, "totalGb": 0]
        }
        let overview = root["overview"] as? Bool ?? false
        let partitions = entries.filter { ($0["insight"] as? Bool) != true }
        let insights = entries.filter { $0["insight"] as? Bool == true }
        let total = int64(root["total_size"])
        return [
            "treemap2D": layout(partitions, total: total > 0 ? total : sum(partitions)),
            "insights": layout(insights, total: sum(insights)),
            "source": fallbackSource,
            "overview": overview,
            "scanStatus": root["scan_status"] as? String ?? "",
            "totalGb": gb(total)
        ]
    }

    private static func layout(_ entries: [[String: Any]], total: Int64) -> [[String: Any]] {
        let home = NSHomeDirectory()
        let parsed: [(name: String, path: String, bytes: Int64, isDir: Bool, status: String, cleanable: Bool)] = entries.compactMap { entry in
            guard let name = entry["name"] as? String,
                  let path = entry["path"] as? String else { return nil }
            let bytes = int64(entry["size"])
            guard bytes > 0 else { return nil }
            return (
                name,
                path,
                bytes,
                entry["is_dir"] as? Bool ?? false,
                entry["scan_status"] as? String ?? "",
                entry["cleanable"] as? Bool ?? false
            )
        }
        let basis = total > 0 ? total : parsed.reduce(Int64(0)) { $0 + $1.bytes }
        guard basis > 0 else { return [] }
        var x = 0.0
        return parsed.map { item in
            let pct = Double(item.bytes) / Double(basis) * 100
            let tile: [String: Any] = [
                "name": item.name,
                "path": item.path.replacingOccurrences(of: home, with: "~"),
                "sizeGb": gb(item.bytes),
                "pct": (pct * 10).rounded() / 10,
                "color": item.cleanable
                    ? "linear-gradient(145deg, #059669, #064e3b)"
                    : (item.isDir
                        ? "linear-gradient(145deg, #d97706, #78350f)"
                        : "linear-gradient(145deg, #0284c7, #0c4a6e)"),
                "files": 0,
                "isDir": item.isDir,
                "scanStatus": item.status,
                "cleanable": item.cleanable,
                "x": x, "y": 0, "w": max(6, pct), "h": 100,
                "children": [] as [[String: Any]]
            ]
            x += max(6, pct)
            return tile
        }
    }

    private static func sum(_ entries: [[String: Any]]) -> Int64 {
        entries.reduce(0) { $0 + int64($1["size"]) }
    }

    private static func gb(_ bytes: Int64) -> Double {
        (Double(bytes) / 1_000_000_000 * 100).rounded() / 100
    }

    private static func int64(_ value: Any?) -> Int64 {
        if let number = value as? Int64 { return number }
        if let number = value as? Int { return Int64(number) }
        if let number = value as? Double { return Int64(number) }
        if let number = value as? NSNumber { return number.int64Value }
        return 0
    }
}
