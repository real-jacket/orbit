import Foundation

/// One known leftover signature: the app it belongs to and the directories that
/// app leaves behind. Kept in its own file because it is static data, not logic.
struct OrphanSignature {
    let id: String
    let name: String
    let category: String
    let bundleId: String
    let appKeywords: [String]
    let fixedPaths: [(path: String, kind: String)]
    let wildcardParents: [(parent: String, prefix: String, kind: String)]
    let impact: String

    static func all(home: String) -> [OrphanSignature] {
        [
            OrphanSignature(
                id: "orphan_docker",
                name: "Docker Desktop",
                category: "虚拟化与容器残留",
                bundleId: "com.docker.docker",
                appKeywords: ["docker"],
                fixedPaths: [
                    ("\(home)/Library/Containers/com.docker.docker", "cache"),
                    ("\(home)/Library/Containers/com.docker.helper", "cache"),
                    ("\(home)/Library/Group Containers/group.com.docker", "cache"),
                    ("\(home)/.docker", "config"),
                    ("\(home)/Library/Application Support/Docker Desktop", "config"),
                    ("\(home)/Library/Caches/com.docker.docker", "cache"),
                    ("\(home)/Library/Logs/Docker Desktop", "cache"),
                    ("\(home)/Library/Preferences/com.docker.docker.plist", "config")
                ],
                wildcardParents: [],
                impact: "检测到 Docker.app 主程序已卸载，或仍留有容器虚拟磁盘、共享守护进程目录与 ~/.docker 配置。"
            ),
            OrphanSignature(
                id: "orphan_android_studio",
                name: "Android Studio & Android SDK",
                category: "移动开发与模拟器残留",
                bundleId: "com.google.android.studio",
                appKeywords: ["android studio"],
                fixedPaths: [
                    ("\(home)/Library/Android/sdk", "cache"),
                    ("\(home)/.android/avd", "cache"),
                    ("\(home)/.android", "config"),
                    ("\(home)/.gradle/caches", "cache"),
                    ("\(home)/Library/Preferences/com.google.android.studio.plist", "config")
                ],
                wildcardParents: [
                    ("\(home)/Library/Application Support/Google", "AndroidStudio", "config"),
                    ("\(home)/Library/Caches/Google", "AndroidStudio", "cache"),
                    ("\(home)/Library/Logs/Google", "AndroidStudio", "cache")
                ],
                impact: "检测到 Android Studio 遗留的 SDK、AVD 模拟器镜像、Gradle 缓存与 AndroidStudio 历史目录。"
            ),
            OrphanSignature(
                id: "orphan_jetbrains",
                name: "JetBrains IDEs",
                category: "IDE 索引与插件残留",
                bundleId: "com.jetbrains.intellij",
                appKeywords: ["intellij", "pycharm", "webstorm", "goland", "clion"],
                fixedPaths: [],
                wildcardParents: [
                    ("\(home)/Library/Caches/JetBrains", "", "cache"),
                    ("\(home)/Library/Application Support/JetBrains", "", "config"),
                    ("\(home)/Library/Logs/JetBrains", "", "cache")
                ],
                impact: "检测到 JetBrains IDE 卸载后遗留的索引缓存与历史版本插件目录。"
            ),
            OrphanSignature(
                id: "orphan_parallels",
                name: "Parallels Desktop",
                category: "虚拟机与驱动残留",
                bundleId: "com.parallels.desktop.console",
                appKeywords: ["parallels"],
                fixedPaths: [
                    ("\(home)/Library/Caches/com.parallels.desktop.console", "cache"),
                    ("\(home)/Library/Application Support/Parallels", "config"),
                    ("\(home)/Library/Preferences/com.parallels.desktop.console.plist", "config")
                ],
                wildcardParents: [],
                impact: "检测到 Parallels Desktop 卸载后遗留的缓存与应用支持目录。"
            )
        ]
    }
}
