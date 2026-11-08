package scanner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// worktreeArtifactNames are the regenerable directories inside an agent
// worktree. Source files, .git and everything else stay inspect-only.
var worktreeArtifactNames = map[string]string{
	"node_modules": "依赖目录", ".next": "Next.js 构建缓存", ".nuxt": "Nuxt 构建缓存", ".turbo": "Turborepo 缓存",
	".parcel-cache": "Parcel 缓存", "dist": "构建产物", "build": "构建产物", "target": "Rust / Java 构建产物",
	".venv": "Python 虚拟环境", "venv": "Python 虚拟环境",
}

var agentWorktreeRoots = []struct{ root, tool string }{{".codex/worktrees", "Codex"}, {".cursor/worktrees", "Cursor"}}

// ambiguousArtifactName names directories that can also be tracked source, so
// they are only artifacts when git ignores them.
func ambiguousArtifactName(name string) bool {
	switch name {
	case "dist", "build", "target":
		return true
	}
	return false
}

func isArtifactName(name string) bool {
	_, ok := artifactKinds[name]
	_, wt := worktreeArtifactNames[name]
	return ok || wt
}

// isAgentWorktreeArtifact is a path-only test: path is exactly an artifact
// directory inside an agent worktree, not nested in another artifact or .git.
func isAgentWorktreeArtifact(path, home string) bool {
	if _, ok := worktreeArtifactNames[filepath.Base(path)]; !ok {
		return false
	}
	for _, wt := range agentWorktreeRoots {
		tail, ok := strings.CutPrefix(path, filepath.Join(home, wt.root)+"/")
		if !ok {
			continue
		}
		parts := strings.Split(tail, "/")
		if len(parts) < 2 || len(parts) > 6 {
			return false
		}
		for _, part := range parts[:len(parts)-1] {
			if _, nested := worktreeArtifactNames[part]; nested || part == ".git" {
				return false
			}
		}
		return true
	}
	return false
}

// isAgentWorktreeRoot is a path-only test: path is a worktree checkout, one or
// two levels below the agent's worktrees folder, with a .git entry.
func isAgentWorktreeRoot(path, home string) bool {
	for _, wt := range agentWorktreeRoots {
		tail, ok := strings.CutPrefix(path, filepath.Join(home, wt.root)+"/")
		if !ok {
			continue
		}
		n := len(strings.Split(tail, "/"))
		return (n == 1 || n == 2) && exists(filepath.Join(path, ".git"))
	}
	return false
}

// worktreeKept says why an unfinished or unverified worktree is not removed.
func worktreeKept(state string) string {
	return state + "。工作区干净也不能证明提交已推送、分支已合并或会话已结束，不在这里移除。"
}

// gitBinary finds a usable git without waking Apple's installer shim; why is "missing" or "notready" when it returns "".
func gitBinary(ctx context.Context) (string, string) {
	binary, err := exec.LookPath("git")
	if err != nil {
		return "", "missing"
	}
	if binary == "/usr/bin/git" {
		developer, err := exec.CommandContext(ctx, "/usr/bin/xcode-select", "-p").Output()
		if err != nil {
			return "", "notready"
		}
		binary = filepath.Join(strings.TrimSpace(string(developer)), "usr/bin/git")
		if !exists(binary) {
			return "", "notready"
		}
	}
	return binary, ""
}

// runGit runs a read-only git command in dir with a short timeout.
func runGit(dir string, args ...string) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	binary, _ := gitBinary(ctx)
	if binary == "" {
		return nil, -1, os.ErrNotExist
	}
	cmd := exec.CommandContext(ctx, binary, append([]string{"-c", "core.fsmonitor=false", "-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	out, err := cmd.Output()
	if exit, ok := err.(*exec.ExitError); ok {
		return out, exit.ExitCode(), err
	}
	return out, 0, err
}

// gitIgnored reports whether git ignores path (exit 0). Any doubt is "no".
func gitIgnored(path string) bool {
	_, code, err := runGit(filepath.Dir(path), "check-ignore", "-q", "--", path)
	return err == nil && code == 0
}

// gitCleanState describes the tracked-file state of the repository around
// project, for the build cleanup row.
func gitCleanState(project string) string {
	if !exists(filepath.Join(project, ".git")) {
		return "非 Git 项目"
	}
	out, _, err := runGit(project, "status", "--porcelain=v1", "-uno", "--ignore-submodules=all")
	if err != nil {
		return "版本库状态无法读取"
	}
	if len(strings.TrimSpace(string(out))) > 0 {
		return "有未提交改动"
	}
	return "工作区干净"
}
