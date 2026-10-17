import Foundation

/// Catalogue-driven scans: clean groups, AI caches, installer packages and build
/// artifacts. Every one of these is a table of known locations plus a shared
/// `catalog` builder, so they belong together and away from the telemetry code.
extension RealMacSystemScanner {
    func scanCleanGroups() -> [[String: Any]] {
        let home = NSHomeDirectory()
        let groups: [(id: String, category: String, name: String, roots: [String], tier: String, impact: String, rebuild: String)] = [
            ("user_caches", "应用缓存与日志", "用户缓存与诊断日志",
             ["\(home)/Library/Caches", "\(home)/Library/Logs"],
             "safe", "只统计本机实际存在的缓存和日志目录。", "应用下次启动时按需重建"),
            ("browser_caches", "浏览器缓存", "Safari / Chrome / Firefox 缓存",
             ["\(home)/Library/Caches/com.apple.Safari", "\(home)/Library/Caches/Google"],
             "safe", "浏览器缓存可以重建，不影响书签和登录状态。", "下次打开网页时重新生成"),
            ("xcode_derived", "开发缓存", "Xcode DerivedData 与模拟器缓存",
             ["\(home)/Library/Developer/Xcode/DerivedData", "\(home)/Library/Developer/CoreSimulator/Caches"],
             "safe", "编译中间产物可以删除，下次编译会重新生成。", "下次编译重新生成"),
            ("package_caches", "包管理器缓存", "npm / pip / Homebrew 下载缓存",
             ["\(home)/.npm/_cacache", "\(home)/Library/Caches/pip", "\(home)/Library/Caches/Homebrew"],
             "review", "包管理器缓存可以重新下载。", "下次安装时重新下载")
        ]
        return groups.compactMap { group in
            catalog(CatalogSpec(id: group.id, category: group.category, name: group.name,
                                roots: group.roots, tier: group.tier,
                                impact: group.impact, rebuild: group.rebuild))
        }
    }
    func scanAICaches() -> [[String: Any]] {
        let home = NSHomeDirectory()
        let bands: [(id: String, band: String, name: String, roots: [String], tier: String, impact: String)] = [
            ("ai_versions", "可重建程序包", "Claude / Cursor 旧版本与扩展缓存",
             ["\(home)/.local/share/claude/versions", "\(home)/.cursor/extensions"],
             "safe", "旧版本程序包和扩展缓存可以重新下载。"),
            ("ai_runtime_cache", "运行缓存", "Cursor / Claude 运行缓存与日志",
             ["\(home)/Library/Caches/Cursor", "\(home)/Library/Logs/Claude"],
             "safe", "运行缓存和日志不影响对话记录。"),
            ("ai_worktrees", "临时工作树", "Codex 临时工作树与模型下载缓存",
             ["\(home)/.codex/worktrees", "\(home)/.cache/huggingface"],
             "review", "删除前确认没有未提交的工作树。")
        ]
        return bands.compactMap { band in
            guard var item = catalog(CatalogSpec(
                id: band.id, category: band.band, name: band.name, roots: band.roots,
                tier: band.tier, impact: band.impact, rebuild: "可重新下载或重新生成"
            )) else { return nil }
            item["band"] = band.band
            item["filesCount"] = "\(item["items"] as? Int ?? 0) 个目录"
            item["lastMod"] = "本机扫描"
            return item
        }
    }

    func scanInstallerPackages() -> [[String: Any]] {
        let home = NSHomeDirectory()
        let roots = ["\(home)/Downloads", "\(home)/Library/Caches/Homebrew/downloads"]
        let extensions = ["dmg", "pkg", "xip", "iso", "zip"]
        var found: [(name: String, path: String, bytes: Int64, age: String)] = []
        for root in roots {
            guard fm.fileExists(atPath: root),
                  let enumerator = fm.enumerator(
                    at: URL(fileURLWithPath: root),
                    includingPropertiesForKeys: [.fileSizeKey, .isRegularFileKey, .contentModificationDateKey],
                    options: [.skipsHiddenFiles, .skipsPackageDescendants]
                  ) else { continue }
            var scanned = 0
            for case let url as URL in enumerator {
                scanned += 1
                if scanned > 2500 { break }
                guard extensions.contains(url.pathExtension.lowercased()),
                      let vals = try? url.resourceValues(forKeys: [.fileSizeKey, .isRegularFileKey, .contentModificationDateKey]),
                      vals.isRegularFile == true,
                      let size = vals.fileSize, size > 20_000_000 else { continue }
                found.append((
                    url.lastPathComponent,
                    url.path.replacingOccurrences(of: home, with: "~"),
                    Int64(size),
                    ageText(vals.contentModificationDate)
                ))
            }
        }
        found.sort { $0.bytes > $1.bytes }
        return found.prefix(20).enumerated().map { idx, item in
            [
                "id": "installer_\(idx)",
                "name": item.name,
                "location": (item.path as NSString).deletingLastPathComponent,
                "path": item.path,
                "sizeGb": gb(item.bytes),
                "age": item.age,
                "checked": true
            ]
        }
    }

    func scanBuildArtifacts() -> [[String: Any]] {
        let home = NSHomeDirectory()
        let roots = ["\(home)/Projects", "\(home)/GitHub", "\(home)/Developer", "\(home)/code", "\(home)/src"]
        let names = ["node_modules": "前端依赖目录", "target": "Rust / Java 构建产物",
                     "DerivedData": "Xcode 构建缓存", ".venv": "Python 虚拟环境",
                     "venv": "Python 虚拟环境", "dist": "前端构建产物", "build": "构建产物"]
        var found: [(name: String, path: String, bytes: Int64, age: String)] = []
        for root in roots where fm.fileExists(atPath: root) {
            guard let enumerator = fm.enumerator(
                at: URL(fileURLWithPath: root),
                includingPropertiesForKeys: [.isDirectoryKey, .contentModificationDateKey],
                options: [.skipsHiddenFiles]
            ) else { continue }
            var scanned = 0
            for case let url as URL in enumerator {
                scanned += 1
                if scanned > 4000 { break }
                let name = url.lastPathComponent
                guard names[name] != nil else { continue }
                enumerator.skipDescendants()
                let (bytes, _) = fastDirectorySize(path: url.path, maxFiles: 1500)
                guard bytes >= 20_000_000 else { continue }
                let modified = (try? url.resourceValues(forKeys: [.contentModificationDateKey]))?.contentModificationDate
                found.append((name, url.path.replacingOccurrences(of: home, with: "~"), bytes, ageText(modified)))
            }
        }
        found.sort { $0.bytes > $1.bytes }
        return found.prefix(20).enumerated().map { idx, item in
            let project = (item.path as NSString).deletingLastPathComponent
            return [
                "id": "artifact_\(idx)",
                "project": project,
                "artifact": item.name,
                "path": item.path,
                "kind": names[item.name] ?? "构建产物",
                "sizeGb": gb(item.bytes),
                "origSizeGb": gb(item.bytes),
                "age": item.age,
                "rebuildNote": "删除后可重新安装或重新编译",
                "gitClean": "本机扫描",
                "checked": true
            ]
        }
    }
}
