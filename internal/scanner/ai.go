package scanner

import (
	"context"
	"fmt"
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
	for _, spec := range agentWorktreeRoots {
		root := filepath.Join(s.home, spec.root)
		for _, entry := range readDir(root) {
			if !entry.isDir || isHidden(entry.name) {
				continue
			}
			container := filepath.Join(root, entry.name)
			paths := []string{container}
			if !exists(filepath.Join(container, ".git")) {
				var repos []string
				for _, child := range readDir(container) {
					path := filepath.Join(container, child.name)
					if child.isDir && exists(filepath.Join(path, ".git")) {
						repos = append(repos, path)
					}
				}
				if len(repos) > 0 {
					paths = repos
				}
			}
			for _, path := range paths {
				// Dependencies and build output first: they are regenerable and
				// are measured out of the worktree row below.
				if artifacts := worktreeArtifacts(path); len(artifacts) > 0 {
					row := s.catalog(CatalogSpec{
						ID: "ai_worktree_artifacts:" + path, Category: spec.tool, Name: spec.tool + " · " + filepath.Base(path) + " · 依赖与构建产物",
						Roots: artifacts, Tier: "review", Classify: (*Scanner).worktreeArtifactVerdict,
						Impact:  "智能体工作树里的依赖目录与构建产物，可以按需重新生成；工作树里的源码和提交不在此列。",
						Rebuild: "需要时重新安装依赖或重新编译",
					}, claims, nil)
					if row != nil {
						row["band"], row["tool"] = "依赖与构建产物", spec.tool
						rows = append(rows, row)
					}
				}
				state := worktreeState(path)
				impact := worktreeKept(state)
				row := s.catalog(CatalogSpec{
					ID: "ai_worktree:" + path, Category: spec.tool, Name: spec.tool + " · " + filepath.Base(path),
					Roots: []string{path}, Tier: "protected", Reason: impact, Impact: impact,
					Rebuild: "工作成果不能按可重建缓存处理",
					Classify: func(s *Scanner, p string, size dirSize) verdict {
						return worktreeVerdict(state, impact, size.modified)
					},
				}, claims, nil)
				if row != nil {
					row["band"], row["gitState"], row["tool"] = "工作成果（仅检查）", state, spec.tool
					rows = append(rows, row)
				}
			}
		}
	}
	return rows
}

// worktreeVerdict keeps a worktree inspect-only unless git verified a clean
// tree and nothing changed for a month; even then it is review, because a
// clean tree does not prove the commits were pushed.
func worktreeVerdict(state, impact string, modified int64) verdict {
	if strings.HasPrefix(state, "工作区干净") && modified > 0 && idleDays(modified) >= 30 {
		return verdict{tier: "review", reason: fmt.Sprintf("工作区干净且 %d 天未修改，但未核验提交是否已推送或分支是否已合并；确认不再需要后再清理", idleDays(modified))}
	}
	return verdict{tier: "protected", reason: impact}
}

func worktreeState(path string) string {
	if !exists(filepath.Join(path, ".git")) {
		return "Git 状态无法核验，保留全部内容"
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
	cmd := exec.CommandContext(ctx, binary, "-c", "core.fsmonitor=false", "-C", path, "status", "--porcelain=v1", "-z", "--untracked-files=normal", "--ignore-submodules=all")
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
