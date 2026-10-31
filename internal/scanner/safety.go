package scanner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func withinPath(path, root string) bool {
	// Case-insensitive APFS aliases must not bypass a protection boundary.
	path, root = strings.ToLower(path), strings.ToLower(root)
	if root == "/" {
		return strings.HasPrefix(path, "/")
	}
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

func builtinCleanupProtection(path, home string) (Protection, bool) {
	for _, root := range []string{"/System", "/usr", "/bin", "/sbin", "/etc", "/dev", "/private/var/db"} {
		if withinPath(path, root) {
			return Protection{Pattern: root, Reason: "系统文件不能清理"}, true
		}
	}
	for _, root := range []string{"/", "/Library", "/Applications", "/Users", "/private", "/var", "/opt", "/Volumes", home} {
		if strings.EqualFold(path, root) {
			return Protection{Pattern: root, Reason: "核心目录不能整体移除，请进入后检查子项"}, true
		}
	}
	if strings.EqualFold(filepath.Dir(path), "/Volumes") {
		return Protection{Pattern: path, Reason: "卷根目录不能整体移除，请进入后检查子项"}, true
	}
	for _, name := range []string{"Library", "Documents", "Desktop", "Downloads", "Pictures", "Movies", "Music", "Public"} {
		if strings.EqualFold(path, filepath.Join(home, name)) {
			return Protection{Pattern: path, Reason: "用户核心目录不能整体移除"}, true
		}
	}
	for _, rule := range []struct{ path, reason string }{
		{".ssh", "密钥与凭据保持原状"}, {".gnupg", "密钥与凭据保持原状"},
		{".config", "个人配置与凭据保持原状"}, {".aws", "云服务凭据保持原状"},
		{"Library/Keychains", "系统钥匙串保持原状"}, {"Library/Mobile Documents", "iCloud 同步资料保持原状"},
		{".codex", "Codex 工作成果、会话、配置与凭据只检查，不清理"},
		{".claude", "Claude 会话、项目记录、配置与凭据保持原状"},
		{".cursor", "Cursor 扩展、工作树和个人配置保持原状"},
		{".cache/huggingface", "共享模型缓存只检查，请使用模型管理工具按修订处理"},
		{".ollama", "本地模型与运行资料保持原状"},
		{"Library/Application Support/Codex", "Codex 会话与运行资料保持原状"},
		{"Library/Application Support/com.openai.codex", "Codex 会话与运行资料保持原状"},
		{"Library/Caches/com.openai.codex", "Codex 运行缓存可能包含活跃状态，保持原状"},
		{"Library/Application Support/Claude", "Claude 会话与个人资料保持原状"},
		{"Library/Application Support/Cursor", "Cursor 项目与个人资料保持原状"},
		{"Library/Application Support/ChatGPT", "对话与个人资料保持原状"},
	} {
		root := filepath.Join(home, rule.path)
		if withinPath(path, root) || withinPath(root, path) {
			return Protection{Pattern: root, Descendant: path != root && withinPath(root, path), Reason: rule.reason}, true
		}
	}
	for _, spec := range aiVersionSpecs {
		root := filepath.Join(home, spec.root)
		if strings.EqualFold(path, root) || withinPath(root, path) {
			return Protection{Pattern: root, Descendant: path != root, Reason: "版本库不能整体清理，必须保留当前和回滚版本"}, true
		}
	}
	return Protection{}, false
}

func (s *Scanner) cleanupProcesses() (string, error) {
	if s.processProbe != nil {
		return s.processProbe()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "/bin/ps", "-axo", "comm=").Output()
	return string(output), err
}

// CleanupReason is shared by candidate inspection and the final move boundary.
// A cached scan never grants permission to remove newly active or protected data.
func (s *Scanner) CleanupReason(raw string) string {
	if s.home == "" || s.home == "/" {
		return "无法确定用户家目录，停止清理"
	}
	path := filepath.Clean(s.expand(raw))
	if !filepath.IsAbs(path) {
		return "不是绝对路径"
	}
	s.refreshWhitelist()
	if protection, protected := s.scanProtection(path); protected {
		if protection.Reason != "" {
			return protection.Reason
		}
		return "白名单保护"
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil && !os.IsNotExist(err) {
		return "路径状态无法核验，保持原状：" + err.Error()
	}
	if err == nil && resolved != path {
		home, _ := filepath.EvalSymlinks(s.home)
		if protection, protected := builtinCleanupProtection(resolved, home); protected {
			return protection.Reason
		}
		if s.whitelist().Protects(resolved) {
			return "白名单保护"
		}
		// macOS's /var and /tmp aliases are normal; arbitrary symlink parents are not cleanup roots.
		if resolved != "/private"+path || !(withinPath(path, "/var") || withinPath(path, "/tmp")) {
			return "路径经过符号链接，不能证明清理边界；请在访达中检查原始路径"
		}
	}
	for _, spec := range aiVersionSpecs {
		root := filepath.Join(s.home, spec.root)
		if !withinPath(path, root) {
			continue
		}
		if filepath.Dir(path) != root {
			return "仅按完整旧版本处理，不单独移除版本内部文件"
		}
		for _, version := range s.versionInventory(spec) {
			if version.path == path {
				return version.reason
			}
		}
		return "版本状态无法核验，保持原状"
	}
	for _, rule := range []struct{ roots, owners []string }{
		{[]string{"Library/Caches/com.apple.Safari"}, []string{"Safari"}},
		{[]string{"Library/Caches/Google"}, []string{"Google Chrome"}},
		{[]string{"Library/Developer/Xcode/DerivedData"}, []string{"Xcode", "xcodebuild", "swift-frontend"}},
		{[]string{"Library/Developer/CoreSimulator/Caches"}, []string{"Simulator", "CoreSimulatorService"}},
		{[]string{"Library/Caches/Cursor"}, []string{"Cursor", "cursor-agent"}},
		{[]string{"Library/Logs/Claude"}, []string{"Claude", "claude"}},
		{[]string{".npm/_cacache"}, []string{"npm", "pnpm", "node"}},
		{[]string{"Library/Caches/pip"}, []string{"pip", "python", "python3"}},
		{[]string{"Library/Caches/Homebrew"}, []string{"brew", "ruby"}},
	} {
		for _, root := range rule.roots {
			if !withinPath(path, filepath.Join(s.home, root)) {
				continue
			}
			processes, err := s.cleanupProcesses()
			if err != nil || strings.TrimSpace(processes) == "" {
				return "进程状态读取失败，不能证明缓存闲置，保持原状"
			}
			for _, command := range strings.Split(processes, "\n") {
				command = strings.TrimSpace(command)
				for _, owner := range rule.owners {
					if filepath.Base(command) == owner || strings.HasPrefix(filepath.Base(command), owner+" ") || strings.HasPrefix(filepath.Base(command), owner+".") || strings.Contains(command, "/"+owner+".app/") {
						return owner + " 正在运行，暂缓清理"
					}
				}
			}
		}
	}
	return ""
}

func (s *Scanner) applyCleanupProtection(row map[string]any) {
	paths, _ := row["subpaths"].([]map[string]any)
	selectable, checked := 0, false
	for _, path := range paths {
		reason := s.CleanupReason(path["path"].(string))
		if path["partial"] == true {
			reason = "测量未完成，体积只是已读取部分；重新扫描完整后再处理"
		}
		if reason != "" || row["tier"] == "protected" {
			path["tier"], path["checked"], path["reason"] = "protected", false, reason
			if reason != "" && row["safetyReason"] == nil {
				row["safetyReason"] = reason
			}
			continue
		}
		selectable++
		checked = checked || path["checked"] == true
	}
	row["checked"] = checked
	if selectable == 0 {
		row["tier"] = "protected"
	}
}
