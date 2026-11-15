package scanner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type aiVersionSpec struct {
	root, launcher, marker, tool, processName string
}

var aiVersionSpecs = []aiVersionSpec{
	{root: ".local/share/claude/versions", launcher: ".local/bin/claude", tool: "Claude Code"},
	{root: ".local/share/cursor-agent/versions", launcher: ".local/bin/cursor-agent", tool: "Cursor Agent"},
	{root: ".copilot/pkg/universal", launcher: ".local/bin/copilot", tool: "GitHub Copilot CLI"},
	{
		root:   "Library/Application Support/Claude/claude-code",
		marker: "Library/Application Support/Claude/claude-code-vm/.sdk-version",
		tool:   "Claude Desktop", processName: "Claude",
	},
	{
		root:   "Library/Application Support/Claude/claude-code-vm",
		marker: "Library/Application Support/Claude/claude-code-vm/.sdk-version",
		tool:   "Claude Desktop", processName: "Claude",
	},
}

type aiVersion struct {
	path, reason string
	modified     int64
}

// Match the launcher, not the newest mtime: an updater can pre-download its next version.
func (s *Scanner) versionInventory(spec aiVersionSpec, live bool) []aiVersion {
	root := filepath.Join(s.home, spec.root)
	entries, err := readDirEntries(root)
	if err != nil {
		return nil
	}
	active, activeErr := s.activeAIVersion(spec)
	processes, processErr := s.processSnapshot(live)
	var versions []aiVersion
	current := ""
	for _, entry := range entries {
		if !validAIVersionName(entry.Name()) {
			continue
		}
		path := filepath.Join(root, entry.Name())
		info, err := entry.Info()
		recordReadError(path, err)
		version := aiVersion{path: path}
		if err != nil {
			version.reason = "版本元数据无法读取，保持原状"
		} else {
			version.modified = info.ModTime().Unix()
			if info.Mode()&os.ModeSymlink != 0 {
				version.reason = "不能证明是可替换的旧程序版本，保持原状"
			}
		}
		resolved, resolveErr := filepath.EvalSymlinks(path)
		if activeErr == nil && resolveErr == nil && withinPath(active, resolved) {
			current = path
			version.reason = "当前启动器指向的版本，保持原状"
		}
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool {
		if versions[i].modified != versions[j].modified {
			return versions[i].modified > versions[j].modified
		}
		return versions[i].path < versions[j].path
	})
	verified, verifyErr := s.activeAIVersion(spec)
	kept := false
	for i := range versions {
		version := &versions[i]
		if current == "" || activeErr != nil || verifyErr != nil || verified != active {
			version.reason = "当前启动版本无法稳定核验，所有版本保持原状"
			continue
		}
		if processErr != nil || strings.TrimSpace(processes) == "" {
			version.reason = "进程状态未知，不能证明旧版本闲置，保持原状"
			continue
		}
		if version.path == current || version.reason != "" {
			continue
		}
		if !kept {
			version.reason = "保留最近的非当前版本作为备用，不清理"
			kept = true
			continue
		}
		resolved, _ := filepath.EvalSymlinks(version.path)
		for _, command := range strings.Split(processes, "\n") {
			command = strings.TrimSpace(command)
			if withinPath(command, version.path) || resolved != "" && withinPath(command, resolved) ||
				spec.launcher != "" && filepath.Base(command) == filepath.Base(spec.launcher) ||
				spec.processName != "" && (strings.EqualFold(filepath.Base(command), spec.processName) || strings.Contains(command, "/"+spec.processName+".app/")) {
				version.reason = "程序仍在运行或版本占用不明，保持原状"
				break
			}
		}
	}
	return versions
}

func (s *Scanner) activeAIVersion(spec aiVersionSpec) (string, error) {
	if spec.marker != "" {
		marker := filepath.Join(s.home, spec.marker)
		data, err := os.ReadFile(marker)
		if err != nil {
			recordReadError(marker, err)
			return "", err
		}
		name := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
		if !validAIVersionName(name) {
			return "", os.ErrInvalid
		}
		root := filepath.Join(s.home, spec.root)
		path := filepath.Join(root, name)
		if !exists(path) {
			return "", os.ErrNotExist
		}
		return filepath.EvalSymlinks(path)
	}
	if spec.launcher == "" {
		return "", os.ErrNotExist
	}
	launcher := filepath.Join(s.home, spec.launcher)
	active, err := filepath.EvalSymlinks(launcher)
	recordReadError(launcher, err)
	return active, err
}

func validAIVersionName(name string) bool {
	return name != "" && name[0] >= '0' && name[0] <= '9' &&
		name != "." && name != ".." && !strings.ContainsAny(name, "/\n\r\t") && !strings.Contains(name, "..")
}

func (s *Scanner) scanAIVersions(claims *catalogClaims) []map[string]any {
	var rows []map[string]any
	for _, spec := range aiVersionSpecs {
		for _, version := range s.versionInventory(spec, false) {
			tier, impact := "review", "已核验不是当前或备用版本。删除前会再次检查启动器和活跃进程；旧版本不会自动勾选。"
			if version.reason != "" {
				tier, impact = "protected", version.reason
			}
			row := s.catalog(CatalogSpec{
				ID: "ai_version:" + version.path, Category: spec.tool,
				Name: spec.tool + " · " + filepath.Base(version.path), Roots: []string{version.path},
				Tier: tier, Impact: impact, Rebuild: "仅移除完整旧版本；必要时重新安装",
			}, claims, nil)
			if row != nil {
				row["band"], row["tool"] = "程序版本", spec.tool
				rows = append(rows, row)
			}
		}
	}
	return rows
}

func (s *Scanner) scanAIWorktrees(claims *catalogClaims) []map[string]any {
	var rows []map[string]any
	for _, tree := range s.aiWorktreeInventory() {
		path := tree.path
		// Offer artifacts separately; moving the whole checkout includes them too.
		if artifacts := worktreeArtifacts(path); len(artifacts) > 0 {
			row := s.catalog(CatalogSpec{
				ID: "ai_worktree_artifacts:" + path, Category: tree.tool, Name: filepath.Base(path) + " · 依赖与构建产物",
				Roots: artifacts, Tier: "review", Classify: (*Scanner).worktreeArtifactVerdict,
				Impact:  "智能体工作树里的依赖目录与构建产物，可以按需重新生成；工作树里的源码和提交不在此列。",
				Rebuild: "需要时重新安装依赖或重新编译",
			}, claims, nil)
			if row != nil {
				row["band"], row["tool"] = "依赖与构建产物", tree.tool
				row["sourceProject"], row["worktreePath"], row["branch"] = s.relativize(tree.source), s.relativize(path), tree.branch
				rows = append(rows, row)
			}
		}
		state := worktreeState(path)
		impact := worktreeKept(state)
		tier := "review"
		if tree.reason != "" {
			tier, impact = "protected", tree.reason
		}
		protection, protected := s.scanProtection(path)
		if protected {
			tier, impact = "protected", protection.Reason
			if impact == "" {
				impact = "白名单排除：" + s.relativize(protection.Pattern)
			}
		}
		row := s.catalog(CatalogSpec{
			ID: "ai_worktree:" + path, Category: tree.tool, Name: filepath.Base(path),
			Roots: []string{path}, Tier: tier, Reason: impact, Impact: "整个工作树移入废纸篓；源仓库、分支与提交保留，历史记录可原路还原。请先确认任务已结束。",
			Rebuild: "从废纸篓还原，或从源仓库重新检出",
			Classify: func(s *Scanner, p string, size dirSize) verdict {
				if tier == "protected" {
					return verdict{tier: tier, reason: impact}
				}
				return worktreeVerdict(state, impact)
			},
		}, &catalogClaims{}, nil)
		if row != nil {
			row["band"], row["gitState"], row["tool"] = "关联工作树", state, tree.tool
			row["sourceProject"], row["worktreePath"], row["branch"] = s.relativize(tree.source), s.relativize(path), tree.branch
			row["wholeWorktree"] = true
			if protected && protection.Descendant {
				row["partial"] = true
			}
			rows = append(rows, row)
		}
	}
	return rows
}

// A clean worktree still needs the user's confirmation that its task has ended.
func worktreeVerdict(state, impact string) verdict {
	if strings.HasPrefix(state, "工作区干净") {
		return verdict{tier: "review", reason: "工作区干净；确认任务已结束、不再需要此工作树后，可移入废纸篓。源仓库与提交保留"}
	}
	return verdict{tier: "protected", reason: impact}
}

func worktreeState(path string) string {
	if !exists(filepath.Join(path, ".git")) {
		return "Git 状态无法核验，保留全部内容"
	}
	// Git worktree repair does not relocate submodule Git pointers.
	if exists(filepath.Join(path, ".gitmodules")) {
		return "工作树含有子模块配置，需保留其 Git 关联，保持原状"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	binary, why := gitBinary(ctx)
	if why == "missing" {
		return "未找到 Git，保留全部内容"
	}
	if binary == "" {
		return "Git 开发工具未就绪，保留全部内容"
	}
	cmd := exec.CommandContext(ctx, binary, "-c", "core.fsmonitor=false", "-C", path, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=none")
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	output, err := cmd.Output()
	if err != nil {
		return "Git 状态读取失败或超时，保留全部内容"
	}
	if len(output) > 0 {
		return "存在未提交或未跟踪内容，保留工作成果"
	}
	return "工作区干净；未核验提交与会话状态"
}
