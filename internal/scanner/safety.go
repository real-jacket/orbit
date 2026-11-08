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

func runtimeCleanupProtection(path string) (Protection, bool) {
	roots := []string{StateDir()}
	if executable, err := os.Executable(); err == nil {
		for current := executable; current != "/" && current != "."; current = filepath.Dir(current) {
			if strings.EqualFold(filepath.Ext(current), ".app") {
				executable = current
				break
			}
		}
		roots = append(roots, executable)
	}
	for _, root := range roots {
		if withinPath(path, root) || withinPath(root, path) {
			return Protection{Pattern: root, Descendant: path != root && withinPath(root, path), Reason: "当前应用、恢复清单与运行状态保持原状"}, true
		}
	}
	return Protection{}, false
}

func builtinCleanupProtection(path, home string) (Protection, bool) {
	for _, root := range []string{"/System", "/usr", "/bin", "/sbin", "/etc", "/dev", "/private/var/db"} {
		if withinPath(path, root) {
			return Protection{Pattern: root, Reason: "系统文件不能清理"}, true
		}
	}
	for _, root := range []string{"/", "/Library", "/Applications", "/Users", "/private", "/var", "/tmp", "/private/tmp", "/private/var", "/private/var/folders", "/opt", "/Volumes", home} {
		if strings.EqualFold(path, root) {
			return Protection{Pattern: root, Reason: "核心目录不能整体移除，请进入后检查子项"}, true
		}
	}
	for _, root := range []string{"/Library/LaunchAgents", "/Library/LaunchDaemons"} {
		if withinPath(path, root) {
			return Protection{Pattern: root, Reason: "需要管理员权限，仅列出"}, true
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
	// Version entries under an AI tool's version root are individually
	// evaluated by CleanupReason. Let them through here so the active-version
	// guard, not a blanket app-data rule, decides.
	if isAIVersionEntry(path, home) {
		return Protection{}, false
	}
	if _, _, known := aiCleanupData(path, home); known || aiSessionShape(path, home) {
		return Protection{}, false
	}
	// Build artifacts inside an agent worktree are regenerable; CleanupReason
	// checks git-ignore and builders. The worktree's own source stays protected.
	if isAgentWorktreeArtifact(path, home) || isAgentWorktreeRoot(path, home) {
		return Protection{}, false
	}
	for _, entry := range aiCacheSpecs() {
		if !entry.logFiles {
			continue
		}
		for _, raw := range entry.spec.Roots {
			root := filepath.Join(home, strings.TrimPrefix(raw, "~/"))
			if withinPath(path, root) {
				return Protection{Pattern: root, Reason: "日志目录与未识别文件保持原状，只能选择核验后的旧诊断文件"}, true
			}
		}
	}
	for _, rule := range []struct{ path, reason string }{
		{".ssh", "密钥与凭据保持原状"}, {".gnupg", "密钥与凭据保持原状"},
		{".config", "个人配置与凭据保持原状"}, {".aws", "云服务凭据保持原状"},
		{"Library/Keychains", "系统钥匙串保持原状"}, {"Library/Mobile Documents", "iCloud 同步资料保持原状"},
		{".codex", "Codex 工作成果、会话、配置与凭据只检查，不清理"},
		{".claude", "Claude 会话、项目记录、配置与凭据保持原状"},
		{".cursor", "Cursor 扩展、工作树和个人配置保持原状"},
		{".copilot", "Copilot 会话、插件、配置与凭据保持原状"},
		{".gemini", "Gemini 会话、项目资料与凭据保持原状"},
		{".continue", "Continue 会话、索引与个人配置保持原状"},
		{".codeium", "Windsurf 会话、工作成果与配置保持原状"},
		{".local/share/opencode", "OpenCode 会话数据库、快照与工作成果保持原状"},
		{".lmstudio", "LM Studio 模型与个人资料保持原状"},
		{".cache/lm-studio/models", "本地模型与共享资源保持原状"},
		{".cache/huggingface", "共享模型缓存只检查，请使用模型管理工具按修订处理"},
		{".ollama", "本地模型与运行资料保持原状"},
		{"Library/Application Support/Codex", "Codex 会话与运行资料保持原状"},
		{"Library/Application Support/com.openai.codex", "Codex 会话与运行资料保持原状"},
		{"Library/Caches/com.openai.codex", "Codex 运行缓存可能包含活跃状态，保持原状"},
		{"Library/Application Support/Claude", "Claude 会话与个人资料保持原状"},
		{"Library/Application Support/Cursor", "Cursor 项目与个人资料保持原状"},
		{"Library/Application Support/Windsurf", "Windsurf 项目与个人资料保持原状"},
		{"Library/Application Support/LM Studio", "LM Studio 模型与个人资料保持原状"},
		{"Library/Application Support/Code/User/globalStorage/rooveterinaryinc.roo-cline", "Roo Code 任务与检查点保持原状"},
		{"Library/Application Support/Code/User/globalStorage/saoudrizwan.claude-dev", "Cline 任务与检查点保持原状"},
		{"Library/Application Support/Code/User/globalStorage/continue.continue", "Continue 会话与索引保持原状"},
		{"Library/Application Support/Code/User/globalStorage/github.copilot-chat", "Copilot Chat 会话与索引保持原状"},
		{"Library/Application Support/ChatGPT", "对话与个人资料保持原状"},
		{"Library/Application Support/orbit", "本应用的恢复清单与运行状态保持原状"},
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

func isAIVersionEntry(path, home string) bool {
	for _, spec := range aiVersionSpecs {
		root := filepath.Join(home, spec.root)
		if strings.EqualFold(filepath.Dir(path), root) && validAIVersionName(filepath.Base(path)) {
			return true
		}
	}
	return false
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

// processSnapshot is the process table for a safety check. Display callers may
// get one up to processTTL old; the live move boundary always runs `ps` itself.
func (s *Scanner) processSnapshot(live bool) (string, error) {
	if live || s.processTTL <= 0 {
		return s.cleanupProcesses()
	}
	s.psMu.Lock()
	defer s.psMu.Unlock()
	if s.psAt.IsZero() || time.Since(s.psAt) > s.processTTL {
		output, err := s.cleanupProcesses()
		if err != nil {
			return output, err // a failure is never reused
		}
		s.psOutput, s.psAt = output, time.Now()
	}
	return s.psOutput, nil
}

// aiHandleOpen is aiVersionHasOpenHandle for a path: display callers on a scan
// view share one system-wide lsof, the move boundary asks lsof about the path.
func (s *Scanner) aiHandleOpen(path string, live bool) (bool, error) {
	if !live && s.handleProbe == nil && s.openSnapshot != nil {
		return s.openSnapshot(path)
	}
	return s.aiVersionHasOpenHandle(path)
}

// CleanupReason is shared by candidate inspection and the final move boundary.
// A cached scan never grants permission to remove newly active or protected data.
func (s *Scanner) CleanupReason(raw string) string { return s.cleanupReason(raw, false) }

// cleanupReason with live=true is the move boundary: it adds the per-path
// process and open-file checks that display-only callers skip for session units.
func (s *Scanner) cleanupReason(raw string, live bool) string {
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
		if protection, protected := runtimeCleanupProtection(resolved); protected {
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
	if reason, known := s.aiSessionCleanupReason(path, live); known {
		return reason
	}
	if entry, root, known := aiCleanupData(path, s.home); known {
		return s.aiDataCleanupReason(path, root, entry, live)
	}
	if isAppBundle(path) {
		processes, err := s.processSnapshot(live)
		if err != nil || strings.TrimSpace(processes) == "" {
			return "无法确认应用是否运行，保持原状"
		}
		if running, _ := appRunning(parseProcessLines(processes), path); running {
			return "应用仍在运行，请先正常退出后重新扫描"
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
		for _, version := range s.versionInventory(spec, live) {
			if version.path == path {
				if version.reason != "" {
					return version.reason
				}
				open, err := s.aiHandleOpen(path, live)
				if err != nil {
					return "无法确认旧版本是否仍被打开，保持原状"
				}
				if open {
					return "仍有进程打开这个版本，保持原状"
				}
				return ""
			}
		}
		return "版本状态无法核验，保持原状"
	}
	for _, rule := range cacheOwnerRules {
		for _, root := range rule.roots {
			if !withinPath(path, filepath.Join(s.home, root)) && !withinPath(filepath.Join(s.home, root), path) {
				continue
			}
			processes, err := s.processSnapshot(live)
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
			if rule.cli != "" {
				builders, err := s.builderSnapshot(live)
				if err != nil {
					return "无法确认 " + rule.cli + " 是否在运行，保持原状"
				}
				if cliRunning(builders, rule.cli) {
					return rule.cli + " 正在运行，暂缓清理"
				}
			}
		}
	}
	if reason := s.cacheOwnerRunningReason(path, live); reason != "" {
		return reason
	}
	if isAgentWorktreeRoot(path, s.home) {
		// Only a clean, long-idle checkout nobody is working in may move.
		if state := worktreeState(path); !strings.HasPrefix(state, "工作区干净") {
			return worktreeKept(state)
		}
		if idleDays(projectActivity(path)) < 30 {
			return "工作树最近 30 天内有活动，保持原状"
		}
		builders, err := s.builderSnapshot(live)
		if err != nil {
			return "无法确认构建工具闲置，保持原状"
		}
		if builder, busy := builderUsing(builders, path); busy {
			return "构建或依赖工具（" + builder + "）正在运行并使用这个项目，暂缓清理"
		}
	}
	if isAgentWorktreeArtifact(path, s.home) && ambiguousArtifactName(filepath.Base(path)) && !gitIgnored(path) {
		return "该目录没有被 .gitignore 忽略，可能是源码的一部分，保持原状"
	}
	if derived := inDerivedData(path); withinPath(path, s.home) && (derived || isArtifactName(filepath.Base(path))) {
		builders, err := s.builderSnapshot(live)
		if err != nil {
			return "无法确认构建工具闲置，保持原状"
		}
		if derived {
			if name, busy := xcodeUsing(builders); busy {
				return "Xcode 或其构建服务（" + name + "）正在运行，暂缓清理构建缓存"
			}
		} else if builder, busy := builderUsing(builders, filepath.Dir(path)); busy {
			return "构建或依赖工具（" + builder + "）正在运行并使用这个项目，暂缓清理"
		}
	}
	return ""
}

func (s *Scanner) aiVersionHasOpenHandle(path string) (bool, error) {
	if s.handleProbe != nil {
		return s.handleProbe(path)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	args := []string{"-Fn"}
	if isDir(path) {
		args = append(args, "+D", path)
	} else {
		args = append(args, "--", path)
	}
	output, err := exec.CommandContext(ctx, "/usr/sbin/lsof", args...).CombinedOutput()
	if err == nil {
		return len(output) > 0, nil
	}
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 && len(output) == 0 {
		return false, nil
	}
	return true, err
}

// applyCleanupProtection lets the live safety check demote a subpath to
// protected. A partial measurement is only a lower bound and never changes a
// tier: removal moves the whole directory to the Trash either way.
func (s *Scanner) applyCleanupProtection(row map[string]any) {
	paths, _ := row["subpaths"].([]map[string]any)
	selectable, checked, allSafe := 0, false, true
	for _, path := range paths {
		reason := s.CleanupReason(path["path"].(string))
		verdictProtected := path["tier"] == "protected" && reason == ""
		if reason != "" || row["tier"] == "protected" || path["tier"] == "protected" {
			if reason == "" {
				reason, _ = path["reason"].(string)
			}
			if reason == "" {
				reason, _ = row["impact"].(string)
			}
			path["tier"], path["checked"], path["reason"] = "protected", false, reason
			unlock := unlockHint(reason)
			if kept, _ := path["unlock"].(string); kept != "" && unlock == "" && verdictProtected {
				unlock = kept
			}
			if unlock != "" {
				path["unlock"] = unlock
				if row["unlock"] == nil {
					row["unlock"] = unlock
				}
			}
			if reason != "" && row["safetyReason"] == nil {
				row["safetyReason"] = reason
			}
			continue
		}
		if path["tier"] == nil && row["tier"] != nil {
			path["tier"] = row["tier"]
		}
		if path["tier"] == "review" {
			allSafe = false
			if r, _ := path["reason"].(string); r == "" {
				path["reason"] = row["impact"]
			}
		}
		selectable++
		checked = checked || path["checked"] == true
	}
	row["checked"] = checked
	switch {
	case selectable == 0:
		row["tier"] = "protected"
	case row["tier"] == nil: // orphan rows carry no tier of their own
	case allSafe && row["tier"] != "protected":
		row["tier"] = "safe"
	case row["tier"] == "safe":
		row["tier"] = "review"
	}
	if row["tier"] == "review" || row["tier"] == "protected" {
		row["reason"] = rowReason(row, paths)
	}
}

// rowReason summarises why a row is not recommended: the first reason a path
// of that tier gives, else what the row says it is.
func rowReason(row map[string]any, paths []map[string]any) string {
	for _, path := range paths {
		if path["tier"] == row["tier"] {
			if reason, _ := path["reason"].(string); reason != "" {
				return reason
			}
		}
	}
	reason, _ := row["impact"].(string)
	return reason
}

// unlockHint turns a "busy" reason into what the user can do about it.
func unlockHint(reason string) string {
	switch {
	case strings.HasSuffix(reason, " 正在运行，暂缓清理"):
		name := strings.TrimSuffix(reason, " 正在运行，暂缓清理")
		if name == strings.ToLower(name) && !strings.ContainsAny(name, " ") {
			return "等 " + name + " 运行结束后重新扫描即可清理"
		}
		return "退出 " + name + " 后重新扫描即可清理"
	case reason == "应用仍在运行，请先正常退出后重新扫描":
		return "正常退出该应用后重新扫描即可处理"
	case strings.HasPrefix(reason, "构建或依赖工具（"):
		name, _, _ := strings.Cut(strings.TrimPrefix(reason, "构建或依赖工具（"), "）")
		return "等 " + name + " 运行结束或手动结束后重新扫描即可清理"
	case strings.HasPrefix(reason, "Xcode 或其构建服务"):
		return "退出 Xcode 并等构建结束后重新扫描即可清理"
	case strings.HasPrefix(reason, "仍有进程打开"):
		return "关闭正在使用它的程序后重新扫描即可处理"
	}
	return ""
}
