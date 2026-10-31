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

type aiVersionSpec struct{ root, launcher, tool string }

var aiVersionSpecs = []aiVersionSpec{
	{".local/share/claude/versions", ".local/bin/claude", "Claude Code"},
	{".local/share/cursor-agent/versions", ".local/bin/cursor-agent", "Cursor Agent"},
}

type aiVersion struct {
	path, reason string
	modified     int64
}

// Match the launcher, not the newest mtime: an updater can pre-download its next version.
func (s *Scanner) versionInventory(spec aiVersionSpec) []aiVersion {
	root := filepath.Join(s.home, spec.root)
	entries, err := readDirEntries(root)
	if err != nil {
		return nil
	}
	launcher := filepath.Join(s.home, spec.launcher)
	active, activeErr := filepath.EvalSymlinks(launcher)
	processes, processErr := s.cleanupProcesses()
	var versions []aiVersion
	current := ""
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		info, err := entry.Info()
		version := aiVersion{path: path}
		if err != nil {
			version.reason = "版本元数据无法读取，保持原状"
		} else {
			version.modified = info.ModTime().Unix()
			if info.Mode()&os.ModeSymlink != 0 || entry.Name() == "" || entry.Name()[0] < '0' || entry.Name()[0] > '9' || strings.ContainsAny(entry.Name(), "\n\r\t") {
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
	verified, verifyErr := filepath.EvalSymlinks(launcher)
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
			if withinPath(command, version.path) || resolved != "" && withinPath(command, resolved) || filepath.Base(command) == filepath.Base(spec.launcher) {
				version.reason = "程序仍在运行或版本占用不明，保持原状"
				break
			}
		}
	}
	return versions
}

func (s *Scanner) scanAIVersions(claims *catalogClaims) []map[string]any {
	var rows []map[string]any
	for _, spec := range aiVersionSpecs {
		for _, version := range s.versionInventory(spec) {
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
	for _, spec := range []struct{ root, tool string }{{".codex/worktrees", "Codex"}, {".cursor/worktrees", "Cursor"}} {
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
				state := worktreeState(path)
				row := s.catalog(CatalogSpec{
					ID: "ai_worktree:" + path, Category: spec.tool, Name: spec.tool + " · " + filepath.Base(path),
					Roots: []string{path}, Tier: "protected", Impact: state + "。工作区干净也不能证明提交已推送、分支已合并或会话已结束，不在这里移除。",
					Rebuild: "工作成果不能按可重建缓存处理",
				}, claims, nil)
				if row != nil {
					row["band"], row["gitState"] = "工作成果（仅检查）", state
					rows = append(rows, row)
				}
			}
		}
	}
	return rows
}

func worktreeState(path string) string {
	if !exists(filepath.Join(path, ".git")) {
		return "Git 状态无法核验，保留全部内容"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	binary, err := exec.LookPath("git")
	if err != nil {
		return "未找到 Git，保留全部内容"
	}
	if binary == "/usr/bin/git" {
		// Do not launch Apple's CLT installer shim during a read-only scan.
		developer, err := exec.CommandContext(ctx, "/usr/bin/xcode-select", "-p").Output()
		if err != nil {
			return "Git 开发工具未就绪，保留全部内容"
		}
		binary = filepath.Join(strings.TrimSpace(string(developer)), "usr/bin/git")
		if !exists(binary) {
			return "Git 开发工具未就绪，保留全部内容"
		}
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
