package scanner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

type aiDataSpec struct {
	spec     CatalogSpec
	tool     string
	band     string
	owners   []string
	logFiles bool
}

func aiCacheSpecs() []aiDataSpec {
	var specs []aiDataSpec
	for _, tool := range []struct {
		id, name, support, cache string
		owners                   []string
	}{
		{"codex", "Codex", "Codex", "", []string{"Codex", "codex", "ChatGPT"}},
		{"claude", "Claude Desktop", "Claude", "com.anthropic.claudefordesktop", []string{"Claude", "claude"}},
		{"cursor", "Cursor / Cursor Agent", "Cursor", "Cursor", []string{"Cursor", "cursor", "cursor-agent"}},
		{"windsurf", "Windsurf", "Windsurf", "Windsurf", []string{"Windsurf", "windsurf"}},
		{"chatgpt", "ChatGPT", "ChatGPT", "com.openai.chat", []string{"ChatGPT"}},
		{"lmstudio", "LM Studio", "LM Studio", "com.lmstudio.lmstudio", []string{"LM Studio", "LMStudio", "lm-studio"}},
	} {
		var roots []string
		if tool.cache != "" {
			roots = append(roots, "~/Library/Caches/"+tool.cache)
		}
		for _, name := range []string{"Cache", "Code Cache", "GPUCache", "GraphiteDawnCache", "GPUPersistentCache"} {
			roots = append(roots, "~/Library/Application Support/"+tool.support+"/"+name)
		}
		if tool.id == "codex" {
			for _, name := range []string{"Cache", "Code Cache", "GPUCache"} {
				roots = append(roots, "~/Library/Application Support/Codex/Default/"+name)
			}
		}
		specs = append(specs, aiDataSpec{
			spec: CatalogSpec{ID: "ai_" + tool.id + "_cache", Category: tool.name, Name: tool.name + " · 可重建缓存", Roots: roots, Tier: "review",
				Impact:  "只检查已识别的程序缓存、代码缓存与 GPU 缓存，不包含 Cookies、IndexedDB、Local Storage、会话或模型。工具运行、文件仍被打开或状态不明时保留；处理后可能需要重新下载并暂时变慢。",
				Rebuild: "退出工具并核对用途后选择；下次运行按需重建"},
			tool: tool.name, band: "可重建缓存", owners: tool.owners,
		})
	}
	for _, tool := range []struct {
		id, name string
		roots    []string
		owners   []string
	}{
		{"codex", "Codex", []string{"~/.codex/log"}, []string{"Codex", "codex", "ChatGPT"}},
		{"claude_code", "Claude Code", []string{"~/.claude/debug"}, []string{"Claude", "claude"}},
		{"copilot", "GitHub Copilot CLI", []string{"~/.copilot/logs"}, []string{"copilot", "github-copilot"}},
		{"opencode", "OpenCode", []string{"~/.local/share/opencode/log"}, []string{"opencode", "OpenCode"}},
		{"claude_desktop", "Claude Desktop", []string{"~/Library/Logs/Claude"}, []string{"Claude", "claude"}},
	} {
		specs = append(specs, aiDataSpec{
			spec: CatalogSpec{ID: "ai_" + tool.id + "_logs", Category: tool.name, Name: tool.name + " · 诊断日志", Roots: tool.roots, Tier: "review",
				Impact:  "只列出日志目录内最多 40 个已识别普通文件；保留最近 30 天、最新一份、latest 指向的文件及运行中或被打开的日志。不会读取日志内容；移除旧日志会失去相应排障记录，默认不选。",
				Rebuild: "新日志会重新生成；旧诊断内容不可重建"},
			tool: tool.name, band: "诊断日志", owners: tool.owners, logFiles: true,
		})
	}
	for _, tool := range []struct {
		id, name, band string
		roots          []string
	}{
		{"codex_runtime", "Codex", "运行资料（仅检查）", []string{"~/Library/Caches/com.openai.codex"}},
		{"codex_sessions", "Codex", "会话与附件（仅检查）", []string{"~/.codex/sessions", "~/.codex/archived_sessions", "~/.codex/attachments"}},
		{"claude_sessions", "Claude Code", "会话与文件历史（仅检查）", []string{"~/.claude/projects", "~/.claude/file-history", "~/.claude/paste-cache"}},
		{"claude_desktop_sessions", "Claude Desktop", "会话与配置（仅检查）", []string{"~/Library/Application Support/Claude/claude-code-sessions", "~/Library/Application Support/Claude/IndexedDB", "~/Library/Application Support/Claude/Local Storage"}},
		{"cursor_extensions", "Cursor / Cursor Agent", "已安装扩展（仅检查）", []string{"~/.cursor/extensions"}},
		{"copilot_sessions", "GitHub Copilot CLI", "会话与配置（仅检查）", []string{"~/.copilot/session-state", "~/.copilot/command-history-state", "~/.copilot/session-store.db"}},
		{"opencode_sessions", "OpenCode", "会话与快照（仅检查）", []string{"~/.local/share/opencode/opencode.db", "~/.local/share/opencode/opencode.db-wal", "~/.local/share/opencode/snapshot"}},
		{"gemini_sessions", "Gemini CLI", "会话与配置（仅检查）", []string{"~/.gemini/tmp", "~/.gemini/history", "~/.gemini/antigravity"}},
		{"windsurf_sessions", "Windsurf", "会话与配置（仅检查）", []string{"~/.codeium/windsurf"}},
		{"continue_sessions", "Continue", "会话与索引（仅检查）", []string{"~/.continue/sessions", "~/Library/Application Support/Code/User/globalStorage/continue.continue"}},
		{"cline_sessions", "Cline", "任务与检查点（仅检查）", []string{"~/Library/Application Support/Code/User/globalStorage/saoudrizwan.claude-dev"}},
		{"roo_sessions", "Roo Code", "任务与检查点（仅检查）", []string{"~/Library/Application Support/Code/User/globalStorage/rooveterinaryinc.roo-cline"}},
		{"copilot_chat_sessions", "GitHub Copilot Chat", "会话与索引（仅检查）", []string{"~/Library/Application Support/Code/User/globalStorage/github.copilot-chat"}},
		{"huggingface_models", "Hugging Face", "模型资源（仅检查）", []string{"~/.cache/huggingface"}},
		{"ollama_models", "Ollama", "模型资源（仅检查）", []string{"~/.ollama/models"}},
		{"lmstudio_models", "LM Studio", "模型资源（仅检查）", []string{"~/.lmstudio/models", "~/.cache/lm-studio/models"}},
	} {
		specs = append(specs, aiDataSpec{
			spec: CatalogSpec{ID: "ai_" + tool.id, Category: tool.name, Name: tool.name + " · " + strings.TrimSuffix(tool.band, "（仅检查）"), Roots: tool.roots, Tier: "protected",
				Impact:  "这里只测量已识别路径，不读取会话或凭据内容。会话、任务历史、文件历史、快照、索引、扩展和模型可能被恢复或共享使用，不以年龄或目录名推断可删除。",
				Rebuild: "由对应工具管理；历史和工作成果不能按缓存重建"},
			tool: tool.name, band: tool.band,
		})
	}
	return specs
}

func aiCleanupData(path, home string) (aiDataSpec, string, bool) {
	for _, entry := range aiCacheSpecs() {
		if entry.spec.Tier == "protected" {
			continue
		}
		for _, raw := range entry.spec.Roots {
			root := filepath.Join(home, strings.TrimPrefix(raw, "~/"))
			if entry.logFiles {
				if filepath.Dir(path) == root && aiDiagnosticName(filepath.Base(path), entry.tool) {
					return entry, root, true
				}
			} else if withinPath(path, root) {
				return entry, root, true
			}
		}
	}
	return aiDataSpec{}, "", false
}

func aiDiagnosticName(name, tool string) bool {
	if strings.HasPrefix(name, ".") {
		return false
	}
	extension := strings.ToLower(filepath.Ext(name))
	return extension == ".log" || tool == "Claude Code" && extension == ".txt"
}

func (s *Scanner) aiDataCleanupReason(path, root string, entry aiDataSpec, live bool) string {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return "AI 数据路径无法核验，保持原状"
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return "AI 缓存或日志边界不是普通目录，保持原状"
	}
	if entry.logFiles {
		if !info.Mode().IsRegular() {
			return "只按普通诊断文件处理，不移除日志目录"
		}
		if !info.ModTime().Before(time.Now().Add(-30 * 24 * time.Hour)) {
			return "保留最近 30 天的诊断日志"
		}
		if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Nlink > 1 {
			return "诊断文件与其它路径共享硬链接，保持原状"
		}
		entries, err := readDirEntries(root)
		if err != nil || len(entries) > 1000 {
			return "无法完整核对日志目录，保持原状"
		}
		newer := false
		for _, candidate := range entries {
			if !aiDiagnosticName(candidate.Name(), entry.tool) || candidate.Type()&os.ModeSymlink != 0 {
				continue
			}
			other, err := candidate.Info()
			if err != nil {
				return "日志时间无法核验，保持原状"
			}
			if other.Mode().IsRegular() && other.ModTime().After(info.ModTime()) {
				newer = true
			}
		}
		if !newer {
			return "最新诊断日志保持原状"
		}
	}
	if entry.logFiles {
		latest := filepath.Join(root, "latest")
		if _, err := os.Lstat(latest); err == nil {
			target, err := os.Stat(latest)
			if err != nil || os.SameFile(info, target) {
				return "latest 日志引用仍需保留，保持原状"
			}
		} else if !os.IsNotExist(err) {
			return "latest 日志引用无法核验，保持原状"
		}
	}
	processes, err := s.processSnapshot(live)
	if err != nil || strings.TrimSpace(processes) == "" {
		return "进程状态未知，不能证明 AI 数据闲置，保持原状"
	}
	for _, command := range strings.Split(processes, "\n") {
		name := strings.ToLower(filepath.Base(strings.TrimSpace(command)))
		for _, owner := range entry.owners {
			owner = strings.ToLower(owner)
			if name == owner || strings.HasPrefix(name, owner+" ") || strings.HasPrefix(name, owner+".") || strings.Contains(strings.ToLower(command), "/"+owner+".app/") {
				return entry.tool + " 正在运行，暂缓清理"
			}
		}
	}
	open, err := s.aiHandleOpen(path, live)
	if err != nil {
		return "无法确认 AI 数据是否仍被打开，保持原状"
	}
	if open {
		return "仍有进程打开这份 AI 数据，保持原状"
	}
	return ""
}

func (s *Scanner) scanAIDiagnosticFiles(entry aiDataSpec, claims *catalogClaims) map[string]any {
	spec := entry.spec
	spec.Roots = nil
	for _, raw := range entry.spec.Roots {
		root := s.expand(raw)
		entries, err := readDirEntries(root)
		if err != nil {
			continue
		}
		type file struct {
			path  string
			bytes int64
		}
		var files []file
		for _, item := range entries {
			if !aiDiagnosticName(item.Name(), entry.tool) || item.Type()&os.ModeSymlink != 0 {
				continue
			}
			info, err := item.Info()
			if err == nil && info.Mode().IsRegular() {
				files = append(files, file{filepath.Join(root, item.Name()), allocatedSize(info)})
			}
		}
		sort.Slice(files, func(left, right int) bool {
			if files[left].bytes != files[right].bytes {
				return files[left].bytes > files[right].bytes
			}
			return files[left].path < files[right].path
		})
		if len(files) > 40 {
			defer recordReadWarning(root, "AI 日志只列出最多 40 个文件；其余未计入本次清理候选")
			files = files[:40]
		}
		for _, file := range files {
			spec.Roots = append(spec.Roots, file.path)
		}
	}
	row := s.catalog(spec, claims, nil)
	if row != nil {
		row["path"] = entry.spec.Roots[0]
	}
	return row
}
