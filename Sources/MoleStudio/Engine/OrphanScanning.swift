import Foundation

/// Leftover detection: which support directories under ~/Library belong to no
/// installed app. Split from `RealMacSystemScanner` so both files stay inside the
/// repo's file-length budget; the scanning primitives live on the main class.
extension RealMacSystemScanner {
    func scanOrphanedLeftovers() -> [[String: Any]] {
        let home = NSHomeDirectory()
        let (installedNames, installedBundleIds) = installedAppIndex(home: home)
        var orphans: [[String: Any]] = []
        var claimedPaths = Set<String>()
        let signatures = OrphanSignature.all(home: home)

        for signature in signatures {
            var subpaths: [[String: Any]] = []
            var totalBytes: Int64 = 0
            var candidates = signature.fixedPaths
            for wildcard in signature.wildcardParents {
                for matched in matchingWildcardPaths(in: wildcard.parent, prefix: wildcard.prefix) {
                    candidates.append((matched, wildcard.kind))
                }
            }

            for candidate in candidates {
                if claimedPaths.contains(where: { candidate.path.hasPrefix($0 + "/") || $0.hasPrefix(candidate.path + "/") }) {
                    continue
                }
                guard fm.fileExists(atPath: candidate.path) else { continue }
                let (bytes, _) = fastDirectorySize(path: candidate.path, maxFiles: 3500)
                guard bytes > 0 else { continue }
                claimedPaths.insert(candidate.path)
                totalBytes += bytes
                subpaths.append([
                    "path": candidate.path.replacingOccurrences(of: home, with: "~"),
                    "kind": candidate.kind,
                    "size": formatBytes(bytes),
                    "checked": true
                ])
            }

            if !subpaths.isEmpty {
                let stillInstalled = signature.appKeywords.contains { keyword in
                    installedNames.contains { $0.contains(keyword) }
                } || installedBundleIds.contains(signature.bundleId)
                orphans.append([
                    "id": signature.id,
                    "name": signature.name,
                    "category": signature.category,
                    "bundleId": signature.bundleId,
                    "statusText": stillInstalled ? "深层容器与 SDK 残留" : "主程序已卸载 · 孤立残留",
                    "leftoverSizeGb": max(0.01, (Double(totalBytes) / 1_000_000_000.0 * 100).rounded() / 100.0),
                    "checked": !stillInstalled,
                    "impact": signature.impact,
                    "subpaths": subpaths
                ])
            }
        }

        let containerOrphans = scanUnclaimedContainers(
            home: home,
            claimedPaths: claimedPaths,
            installedNames: installedNames,
            installedBundleIds: installedBundleIds
        )
        orphans.append(contentsOf: containerOrphans.prefix(18))

        orphans.sort { ($0["leftoverSizeGb"] as? Double ?? 0) > ($1["leftoverSizeGb"] as? Double ?? 0) }
        return orphans
    }

    /// Directories under ~/Library that belong to no installed app. These are the
    /// leftovers of a normal uninstall: the bundle is gone, its support files are not.
    private func scanUnclaimedContainers(
        home: String,
        claimedPaths: Set<String>,
        installedNames: Set<String>,
        installedBundleIds: Set<String>
    ) -> [[String: Any]] {
        let scanTargets = [
            ("\(home)/Library/Containers", "cache", "孤立沙盒容器残留"),
            ("\(home)/Library/Group Containers", "cache", "孤立沙盒容器残留"),
            ("\(home)/Library/Application Support", "config", "已卸载应用支持残留"),
            ("\(home)/Library/Caches", "cache", "已卸载应用支持残留")
        ]
        let ignoredPrefixes = [
            "com.apple.", "group.com.apple.", "apple", "cloudkit", "knowledge", "caches",
            "crashreporter", "geoservices", "addressbook", "facetime", "homekit", "safari",
            "spotlight", "google", "jetbrains", "mozilla", "microsoft"
        ]

        var found: [[String: Any]] = []
        var claimed = claimedPaths
        for target in scanTargets {
            guard let items = try? fm.contentsOfDirectory(atPath: target.0) else { continue }
            for item in items where !item.hasPrefix(".") {
                let lower = item.lowercased()
                if ignoredPrefixes.contains(where: { lower == $0 || lower.hasPrefix($0) }) { continue }
                let fullPath = (target.0 as NSString).appendingPathComponent(item)
                if claimed.contains(fullPath) { continue }
                let matchesInstalled = installedBundleIds.contains(lower)
                    || installedBundleIds.contains { lower.contains($0) || $0.contains(lower) }
                    || installedNames.contains { $0.count >= 3 && (lower.contains($0) || $0.contains(lower)) }
                if matchesInstalled { continue }

                let (bytes, _) = fastDirectorySize(path: fullPath, maxFiles: 1200)
                guard bytes >= 15_000_000 else { continue }
                claimed.insert(fullPath)
                found.append([
                    "id": "orphan_\(item)",
                    "name": item,
                    "category": target.2,
                    "bundleId": item,
                    "statusText": "主程序已卸载 · 孤立残留",
                    "leftoverSizeGb": max(0.02, (Double(bytes) / 1_000_000_000.0 * 100).rounded() / 100.0),
                    "checked": true,
                    "impact": "经与 /Applications 已安装列表比对，该目录对应的主程序已不存在，属于卸载后遗留的无主残留目录。",
                    "subpaths": [[
                        "path": fullPath.replacingOccurrences(of: home, with: "~"),
                        "kind": target.1,
                        "size": formatBytes(bytes),
                        "checked": true
                    ]]
                ])
                if found.count >= 18 { break }
            }
        }
        return found
    }
}
