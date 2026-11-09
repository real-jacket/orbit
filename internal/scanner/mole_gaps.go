package scanner

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// The last user level rules of Mole's `mo clean` that the other tables lack.
// Rules whose matches need a guard (a lock, an age, a stub shape) list their
// matches through a find hook; their paths only tie running owners to folders.

func (r moleRule) found(f func(*Scanner) []moleHit) moleRule { r.find = f; return r }

const (
	mailAgeDays = 30
	// Mole skips a Mail Downloads folder smaller than 5120 KB.
	mailDownloadsDefaultMin = 5120 * 1024
)

var mailDownloadsMinBytes int64 = mailDownloadsDefaultMin

var gapGroups = []moleGroup{
	{
		id: "device_firmware", category: "应用缓存与日志", name: "设备固件安装包缓存",
		impact:  "访达、iTunes 和 Apple Configurator 缓存的 iPhone、iPad 等设备固件（.ipsw），需要恢复设备时可重新下载。仍被打开的文件保持原状。",
		rebuild: "恢复设备时重新下载",
		rules:   []moleRule{ok("设备固件安装包，需要时可以从苹果服务器重新下载").found((*Scanner).firmwareFiles)},
	},
	{
		id: "app_support_sweep", category: "应用缓存与日志", name: "应用的浏览器内核缓存与崩溃报告",
		impact:  "Electron 和 Chromium 内核应用放在 Application Support 里的代码缓存、图形着色器缓存和已完成的崩溃报告，不含账户和文档。运行中的应用保持原状。",
		rebuild: "下次打开应用时重新生成",
		rules: []moleRule{
			sf("App Store content delivery", "log", "Library/Group Containers/group.com.apple.contentdelivery/Logs",
				"Library/Group Containers/group.com.apple.contentdelivery/Library/Logs"),
			ok("").found((*Scanner).appSupportSweep),
		},
	},
	{
		id: "container_stubs", category: "应用缓存与日志", name: "已卸载应用留下的空沙盒容器",
		impact:  "CleanMyMac 卸载后留下的空沙盒容器，里面只有系统元数据文件，不含用户数据。",
		rebuild: "无需重建",
		rules:   []moleRule{ok("已卸载应用留下的空沙盒容器，里面只有系统元数据文件，不含用户数据").found((*Scanner).containerStubs)},
	},
	{
		id: "mail_downloads", category: "应用缓存与日志", name: "邮件附件的本地副本",
		impact:  "邮件 App 打开附件时在本机留下的副本，超过 30 天且从未改动，可在邮件中重新下载。",
		rebuild: "在邮件中再次打开附件时重新下载",
		rules: []moleRule{
			rv("邮件附件的本地副本，超过 30 天且未被改动，可以在邮件中重新下载；如果邮件已从服务器删除，这可能是唯一的副本，请确认",
				"Library/Mail Downloads", containers+"com.apple.mail/Data/Library/Mail Downloads").by("Mail").found((*Scanner).mailAttachments),
		},
	},
	{
		id: "recent_items", category: "应用缓存与日志", name: "最近使用的项目记录",
		impact:  "苹果菜单里「最近使用的项目」的记录文件，移到废纸篓后菜单会清空，从废纸篓放回即可恢复。",
		rebuild: "使用应用和文件时自动重新记录",
		rules: []moleRule{
			rv("「最近使用的项目」菜单的记录，清理后菜单会清空，文件进入废纸篓，可以放回恢复",
				recentItemPaths()...),
		},
	},
}

// firmwareFiles lists the .ipsw files of Finder, iTunes and Apple Configurator.
// Ported from tw93/Mole V1.70.0 lib/clean/user.sh clean_cached_device_firmware
func (s *Scanner) firmwareFiles() []moleHit {
	var hits []moleHit
	add := func(p string) { hits = append(hits, moleHit{path: p}) }
	for _, kind := range []string{"iPhone", "iPad", "iPod", "Apple TV"} {
		dir := filepath.Join(s.home, "Library/iTunes", kind+" Software Updates")
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".ipsw") {
				add(filepath.Join(dir, entry.Name()))
			}
		}
	}
	groups, _ := filepath.Glob(filepath.Join(s.home, "Library/Group Containers/*.group.com.apple.configurator"))
	for _, group := range groups {
		if info, err := os.Lstat(group); err != nil || !info.IsDir() {
			continue
		}
		_ = filepath.WalkDir(group, func(p string, d fs.DirEntry, err error) error {
			if err == nil && d.Type().IsRegular() && strings.HasSuffix(d.Name(), ".ipsw") {
				add(p)
			}
			return nil
		})
	}
	return hits
}

// isFirmwarePath says whether path is a firmware file in one of Mole's folders.
func (s *Scanner) isFirmwarePath(p string) bool {
	if !strings.HasSuffix(p, ".ipsw") {
		return false
	}
	if withinPath(p, filepath.Join(s.home, "Library/iTunes")) {
		return true
	}
	rel, err := filepath.Rel(filepath.Join(s.home, "Library/Group Containers"), p)
	return err == nil && !strings.HasPrefix(rel, "..") && strings.HasSuffix(strings.SplitN(rel, "/", 2)[0], ".group.com.apple.configurator")
}

// appSupportInUse is Mole's app_support_dir_in_use: Chromium and Electron keep
// <userData>/SingletonLock, a link to "<host>-<pid>", while they run. An
// unreadable or unparsable lock counts as in use.
// Ported from tw93/Mole V1.70.0 lib/clean/user.sh app_support_dir_in_use
func appSupportInUse(dir string) bool {
	lock := filepath.Join(dir, "SingletonLock")
	if info, err := os.Lstat(lock); err != nil || info.Mode()&os.ModeSymlink == 0 {
		return false
	}
	target, err := os.Readlink(lock)
	if err != nil {
		return true
	}
	pid, err := strconv.Atoi(target[strings.LastIndex(target, "-")+1:])
	if err != nil || pid <= 0 {
		return true
	}
	// EPERM is a live process of another user.
	err = syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

var sweepLeaves = []string{"Code Cache", "GPUCache", "DawnCache", "GrShaderCache", "GraphiteDawnCache", "DawnGraphiteCache", "DawnWebGPUCache"}

// appSupportSweep lists the regenerable Chromium cache folders of every app
// under Application Support, after Mole's protected names and lock check.
// An app that the orphan scan would offer whole is left to that list.
// Ported from tw93/Mole V1.70.0 lib/clean/user.sh clean_application_support_logs
func (s *Scanner) appSupportSweep() []moleHit {
	base := filepath.Join(s.home, appSupport)
	var hits []moleHit
	var names, ids map[string]struct{}
	for _, entry := range readDir(base) {
		app := entry.name
		dir := filepath.Join(base, app)
		if isHidden(app) || moleProtectedName(app) || isCriticalSystemComponent(app) || !realDir(dir) {
			continue
		}
		if _, protected := s.scanProtection(dir); protected || appSupportInUse(dir) {
			continue
		}
		var found []moleHit
		marker := realDir(filepath.Join(dir, "Crashpad"))
		for _, leaf := range sweepLeaves {
			if _, err := os.Lstat(filepath.Join(dir, leaf)); err == nil {
				marker = true
			}
			if p := filepath.Join(dir, leaf); realDir(p) {
				found = append(found, moleHit{p, fmt.Sprintf(kindReason["cache"], app)})
			}
		}
		if p := filepath.Join(dir, "Crashpad/completed"); realDir(p) {
			found = append(found, moleHit{p, fmt.Sprintf(kindReason["crash"], app)})
		}
		if marker {
			for _, leaf := range []string{"Cache", "CachedData"} {
				if p := filepath.Join(dir, leaf); realDir(p) {
					found = append(found, moleHit{p, fmt.Sprintf(kindReason["cache"], app)})
				}
			}
		}
		if len(found) == 0 {
			continue
		}
		if lower := strings.ToLower(app); !ignored(lower) && !isSensitiveOrphan(lower) {
			if names == nil {
				names, ids = s.installedAppIndex(s.home)
			}
			if !s.matchesInstalled(lower, names, ids) {
				continue
			}
		}
		hits = append(hits, found...)
	}
	return hits
}

func realDir(p string) bool {
	info, err := os.Lstat(p)
	return err == nil && info.IsDir()
}

// containerStubs lists CleanMyMac's empty sandbox containers once the app is gone.
// shortcut: the move boundary does not re-check the stub shape, so a container that gains data after the scan would move whole to the Trash and could be put back; add a boundary check if more apps join the list.
// Ported from tw93/Mole V1.70.0 lib/clean/apps.sh clean_orphaned_container_stubs
func (s *Scanner) containerStubs() []moleHit {
	dir := filepath.Join(s.home, containers)
	var hits []moleHit
	var index map[string]struct{}
	for _, entry := range readDir(dir) {
		name := entry.name
		if a, _ := path.Match("com.macpaw.CleanMyMac*", name); !a {
			if b, _ := path.Match("*.com.macpaw.CleanMyMac*", name); !b {
				continue
			}
		}
		stub := filepath.Join(dir, name)
		children, err := os.ReadDir(stub)
		if !realDir(stub) || err != nil || len(children) != 1 ||
			children[0].Name() != ".com.apple.containermanagerd.metadata.plist" || !children[0].Type().IsRegular() {
			continue
		}
		installed := false
		for _, app := range []string{"/Applications/CleanMyMac X.app", filepath.Join(s.home, "Applications/CleanMyMac X.app"),
			"/Applications/Setapp/CleanMyMac X.app", filepath.Join(s.home, "Library/Application Support/Setapp/Applications/CleanMyMac X.app")} {
			if _, err := os.Lstat(app); err == nil {
				installed = true
			}
		}
		// The team id prefix is not part of the bundle id the app registers.
		id := name
		if i := strings.Index(name, ".com.macpaw."); i > 0 {
			id = name[i+1:]
		}
		if !installed {
			if index == nil {
				index = s.installedBundleIndex()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			installed = s.bundleInstalled(ctx, id, index)
			cancel()
		}
		if !installed {
			hits = append(hits, moleHit{path: stub})
		}
	}
	return hits
}

// mailAttachments lists the untouched, old attachment copies of Mail Downloads.
// Ported from tw93/Mole V1.70.0 lib/clean/user.sh _clean_mail_downloads
func (s *Scanner) mailAttachments() []moleHit {
	var hits []moleHit
	now := time.Now()
	containerRoot := filepath.Join(s.home, containers, "com.apple.mail/Data/Library/Mail Downloads")
	for _, root := range []string{filepath.Join(s.home, "Library/Mail Downloads"), containerRoot} {
		if !realDir(root) || s.fastDirectorySize(root, orphanFileCap).bytes < mailDownloadsMinBytes {
			continue
		}
		folders, _ := os.ReadDir(root)
		for _, folder := range folders {
			files, _ := os.ReadDir(filepath.Join(root, folder.Name()))
			for _, file := range files {
				p := filepath.Join(root, folder.Name(), file.Name())
				if !s.mailUntouchedCopy(root, p, now) {
					continue
				}
				// The installer list already offers installers found there.
				if info, err := file.Info(); root == containerRoot && err == nil {
					if installer, _ := installerKind(p, info.Size()); installer {
						continue
					}
				}
				hits = append(hits, moleHit{path: p})
			}
		}
	}
	return hits
}

// mailUntouchedCopy says the file is a plain copy Mail wrote and nobody touched
// since: a lone regular file of ours, never modified after it was created, in a
// folder created more than mailAgeDays ago. Without a creation time it fails.
// Ported from tw93/Mole V1.70.0 lib/clean/user.sh _mail_attachment_is_untouched_copy
func (s *Scanner) mailUntouchedCopy(root, p string, now time.Time) bool {
	rel, err := filepath.Rel(root, p)
	parts := strings.Split(rel, string(filepath.Separator))
	if err != nil || len(parts) != 2 || isHidden(parts[0]) || isHidden(parts[1]) {
		return false
	}
	for a := root; a != s.home; a = filepath.Dir(a) {
		if !strings.HasPrefix(a, s.home+"/") {
			return false
		}
		if info, err := os.Lstat(a); err != nil || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	fileInfo, err1 := os.Lstat(p)
	dirInfo, err2 := os.Lstat(filepath.Join(root, parts[0]))
	if err1 != nil || err2 != nil || !fileInfo.Mode().IsRegular() || !dirInfo.IsDir() {
		return false
	}
	st, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok || st.Nlink != 1 || int(st.Uid) != os.Getuid() {
		return false
	}
	fileBirth, ok1 := birthTime(fileInfo)
	dirBirth, ok2 := birthTime(dirInfo)
	// Writing the copy itself takes a moment, hence two seconds of slack.
	return ok1 && ok2 && fileBirth > 0 && fileInfo.ModTime().Unix() <= fileBirth+2 &&
		dirBirth > 0 && dirBirth < now.Unix()-mailAgeDays*86400 &&
		fileBirth <= dirBirth+2 && dirInfo.ModTime().Unix() <= dirBirth+2
}

// gapsCleanupReason adds the live checks of the rules above to the shared
// boundary: a running Chromium app keeps the caches of its data folder, and an
// open firmware file stays.
func (s *Scanner) gapsCleanupReason(p string, live bool) string {
	base := filepath.Join(s.home, appSupport)
	if rel, err := filepath.Rel(base, p); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		app := strings.SplitN(rel, string(filepath.Separator), 2)[0]
		if appSupportInUse(filepath.Join(base, app)) {
			return app + " 正在运行，暂缓清理"
		}
	}
	if s.isFirmwarePath(p) {
		open, err := s.aiHandleOpen(p, live)
		if err != nil {
			return "无法确认固件文件是否仍被打开，保持原状"
		}
		if open {
			return "仍有进程打开这个固件文件，保持原状"
		}
	}
	return ""
}

// recentItemPaths are Mole's recent item lists plus the legacy preferences file.
// Ported from tw93/Mole V1.70.0 lib/clean/user.sh _clean_recent_items
func recentItemPaths() []string {
	var paths []string
	for _, ext := range []string{".sfl2", ".sfl"} {
		for _, list := range []string{"Applications", "Documents", "Servers", "Hosts"} {
			paths = append(paths, appSupport+"com.apple.sharedfilelist/com.apple.LSSharedFileList.Recent"+list+ext)
		}
	}
	return append(paths, "Library/Preferences/com.apple.recentitems.plist")
}
