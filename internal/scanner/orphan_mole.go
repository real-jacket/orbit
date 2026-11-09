package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// appScanDepth matches Mole's `find -maxdepth 3` over the application folders.
const appScanDepth = 3

// findAppBundles lists every .app under dirs, up to appScanDepth levels, without
// entering a bundle (so an iOS Wrapper/*.app is never counted twice).
// Ported from tw93/Mole V1.70.0 lib/clean/apps.sh scan_installed_apps
func findAppBundles(dirs []string) []string {
	var found []string
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			path := filepath.Join(dir, entry.Name())
			if strings.EqualFold(filepath.Ext(entry.Name()), ".app") {
				found = append(found, path)
			} else if entry.IsDir() && depth < appScanDepth {
				walk(path, depth+1)
			}
		}
	}
	for _, dir := range dirs {
		walk(dir, 1)
	}
	return found
}

// orphanNeverDelete names credential and key stores that are never offered as
// leftovers, even when no app claims them.
// Ported from tw93/Mole V1.70.0 lib/clean/apps.sh ORPHAN_NEVER_DELETE_PATTERNS
var orphanNeverDelete = []string{
	"1password", "keychain", "bitwarden", "lastpass", "keepass", "dashlane", "enpass", "ssh", "gpg", "gnupg",
}

func isSensitiveOrphan(lower string) bool {
	for _, word := range orphanNeverDelete {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

const (
	claudeSupportDir   = "Library/Application Support/Claude"
	claudeVMMinAgeDays = 7
	claudeBundleID     = "com.anthropic.claudefordesktop"
)

// isClaudeVMBundle is a path-only test for a Claude workspace VM bundle: a
// *.bundle directory up to three levels under Claude's support folder.
// Ported from tw93/Mole V1.70.0 lib/clean/apps.sh clean_orphaned_app_data (Claude VM bundles)
func isClaudeVMBundle(path, home string) bool {
	tail, ok := strings.CutPrefix(path, filepath.Join(home, claudeSupportDir)+"/")
	if !ok || !strings.HasSuffix(path, ".bundle") {
		return false
	}
	if strings.Count(tail, "/") > 2 {
		return false
	}
	for _, spec := range aiVersionSpecs {
		if withinPath(path, filepath.Join(home, spec.root)) {
			return false
		}
	}
	return true
}

// claudeVMReason is empty only when Claude Desktop is gone, not running and the
// bundle has sat untouched for a week.
// Ported from tw93/Mole V1.70.0 lib/clean/apps.sh is_claude_vm_bundle_orphaned
func (s *Scanner) claudeVMReason(path string, live bool) string {
	processes, err := s.processSnapshot(live)
	if err != nil || strings.TrimSpace(processes) == "" {
		return "无法确认 Claude 是否运行，保持原状"
	}
	for _, command := range strings.Split(processes, "\n") {
		if base := filepath.Base(strings.TrimSpace(command)); base == "Claude" || strings.Contains(command, "/Claude.app/") {
			return "Claude 正在运行，暂缓清理"
		}
	}
	// By bundle id only: a `claude` CLI on the PATH is not the desktop app.
	_, ids := s.installedAppIndex(s.home)
	if _, installed := ids[claudeBundleID]; installed {
		return "Claude 桌面版仍已安装，这个虚拟机文件保持原状"
	}
	info, err := os.Stat(path)
	if err != nil {
		return "路径状态无法核验，保持原状"
	}
	if time.Since(info.ModTime()) < claudeVMMinAgeDays*24*time.Hour {
		return "最近 7 天内有变动，保持原状"
	}
	return ""
}

// scanClaudeVMOrphans offers leftover Claude workspace VM bundles once the app is gone.
// Ported from tw93/Mole V1.70.0 lib/clean/apps.sh clean_orphaned_app_data (Claude VM bundles)
func (s *Scanner) scanClaudeVMOrphans(usedIDs map[string]struct{}) []map[string]any {
	var rows []map[string]any
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			path := filepath.Join(dir, entry.Name())
			if !entry.IsDir() {
				continue
			}
			if !isClaudeVMBundle(path, s.home) {
				if depth < 3 {
					walk(path, depth+1)
				}
				continue
			}
			if s.cleanupReason(path, false) != "" {
				continue
			}
			size := s.fastDirectorySize(path, orphanFileCap)
			if size.bytes <= 0 && !size.partial {
				continue
			}
			rows = append(rows, map[string]any{
				"id":             uniqueOrphanID(usedIDs, "orphan_claude_vm_"+entry.Name()),
				"name":           "Claude 工作区虚拟机",
				"category":       "已卸载应用支持残留",
				"bundleId":       claudeBundleID,
				"statusText":     "未发现对应主程序 · 待确认",
				"leftoverSizeGb": plainGB(size.bytes),
				"checked":        false,
				"impact":         "Claude 桌面版已不在本机，这个工作区虚拟机文件可以移除；重新安装后会再次生成。",
				"partial":        size.partial,
				"subpaths": []map[string]any{{
					"path": s.relativize(path), "kind": "cache", "size": formatBytes(size.bytes),
					"sizeGb": plainGB(size.bytes), "checked": false, "partial": size.partial,
				}},
			})
		}
	}
	walk(filepath.Join(s.home, claudeSupportDir), 1)
	return rows
}
