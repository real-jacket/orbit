package scanner

// OrphanSignature describes an app whose leftovers the scanner looks for: the
// directories it leaves behind even after the app itself is gone.
type OrphanSignature struct {
	ID          string
	Name        string
	Category    string
	BundleID    string
	AppKeywords []string
	FixedPaths  []OrphanPath
	Wildcards   []OrphanWildcard
	Impact      string
}

// OrphanPath is one directory a signature always leaves behind.
// SharedTools names CLI tools or alternative runtimes that actively share this
// path even when the desktop app itself is uninstalled (for example ~/.docker
// with docker CLI or OrbStack). When any of those tools is present, the path
// stays untouched instead of being offered as a desktop-app leftover.
type OrphanPath struct {
	Path        string
	Kind        string
	SharedTools []string
}

// OrphanWildcard is a directory a signature leaves variants of, matched by the
// prefix of their name.
type OrphanWildcard struct {
	Parent string
	Prefix string
	Kind   string
}

// AllSignatures lists the leftovers the scanner looks for, built from the
// user's home directory.
func AllSignatures(home string) []OrphanSignature {
	return []OrphanSignature{
		{
			ID:          "orphan_docker",
			Name:        "Docker Desktop",
			Category:    "虚拟化与容器残留",
			BundleID:    "com.docker.docker",
			AppKeywords: []string{"docker"},
			FixedPaths: []OrphanPath{
				{Path: home + "/Library/Containers/com.docker.docker", Kind: "cache"},
				{Path: home + "/Library/Containers/com.docker.helper", Kind: "cache"},
				{Path: home + "/Library/Group Containers/group.com.docker", Kind: "cache"},
				{Path: home + "/.docker", Kind: "config", SharedTools: []string{"docker", "orbstack", "colima", "rancher", "podman"}},
				{Path: home + "/Library/Application Support/Docker Desktop", Kind: "config"},
				{Path: home + "/Library/Caches/com.docker.docker", Kind: "cache"},
				{Path: home + "/Library/Logs/Docker Desktop", Kind: "cache"},
				{Path: home + "/Library/Preferences/com.docker.docker.plist", Kind: "config"},
				{Path: home + "/Library/Preferences/com.electron.dockerdesktop.plist", Kind: "config"},
				{Path: home + "/Library/Saved Application State/com.electron.dockerdesktop.savedState", Kind: "cache"},
			},
			Impact: "检测到 Docker.app 主程序未安装，以下为 Docker Desktop 遗留的容器虚拟磁盘、日志与桌面端配置。",
		},
		{
			ID:          "orphan_android_studio",
			Name:        "Android Studio & Android SDK",
			Category:    "移动开发与模拟器残留",
			BundleID:    "com.google.android.studio",
			AppKeywords: []string{"android studio"},
			FixedPaths: []OrphanPath{
				{Path: home + "/Library/Android/sdk", Kind: "cache", SharedTools: []string{"adb"}},
				{Path: home + "/.android/avd", Kind: "cache", SharedTools: []string{"adb"}},
				{Path: home + "/.android", Kind: "config", SharedTools: []string{"adb"}},
				{Path: home + "/.gradle/caches", Kind: "cache", SharedTools: []string{"gradle"}},
				{Path: home + "/Library/Preferences/com.google.android.studio.plist", Kind: "config"},
			},
			Wildcards: []OrphanWildcard{
				{home + "/Library/Application Support/Google", "AndroidStudio", "config"},
				{home + "/Library/Caches/Google", "AndroidStudio", "cache"},
				{home + "/Library/Logs/Google", "AndroidStudio", "cache"},
			},
			Impact: "检测到 Android Studio 遗留的 SDK、AVD 模拟器镜像、Gradle 缓存与 AndroidStudio 历史目录。",
		},
		{
			ID:          "orphan_jetbrains",
			Name:        "JetBrains IDEs",
			Category:    "IDE 索引与插件残留",
			BundleID:    "com.jetbrains.intellij",
			AppKeywords: []string{"intellij", "pycharm", "webstorm", "goland", "clion"},
			Wildcards: []OrphanWildcard{
				{home + "/Library/Caches/JetBrains", "", "cache"},
				{home + "/Library/Application Support/JetBrains", "", "config"},
				{home + "/Library/Logs/JetBrains", "", "cache"},
			},
			Impact: "检测到 JetBrains IDE 卸载后遗留的索引缓存与历史版本插件目录。",
		},
		{
			ID:          "orphan_parallels",
			Name:        "Parallels Desktop",
			Category:    "虚拟机与驱动残留",
			BundleID:    "com.parallels.desktop.console",
			AppKeywords: []string{"parallels"},
			FixedPaths: []OrphanPath{
				{Path: home + "/Library/Caches/com.parallels.desktop.console", Kind: "cache"},
				{Path: home + "/Library/Application Support/Parallels", Kind: "config"},
				{Path: home + "/Library/Preferences/com.parallels.desktop.console.plist", Kind: "config"},
			},
			Impact: "检测到 Parallels Desktop 卸载后遗留的缓存与应用支持目录。",
		},
	}
}
