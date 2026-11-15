package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type gitWorktree struct {
	path, source, branch, tool, reason string
}

func gitWorktrees(project string) ([]gitWorktree, error) {
	output, _, err := runGit(project, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	var trees []gitWorktree
	for _, field := range strings.Split(string(output), "\x00") {
		if path, ok := strings.CutPrefix(field, "worktree "); ok {
			trees = append(trees, gitWorktree{path: filepath.Clean(path)})
			continue
		}
		if len(trees) == 0 {
			continue
		}
		tree := &trees[len(trees)-1]
		switch {
		case strings.HasPrefix(field, "branch "):
			tree.branch = strings.TrimPrefix(field, "branch refs/heads/")
		case field == "detached":
			tree.branch = "分离 HEAD"
		case field == "bare":
			tree.reason = "裸仓库保持原状，不作为工作树清理"
		case field == "locked" || strings.HasPrefix(field, "locked "):
			tree.reason = "Git 工作树已锁定，保持原状"
		case field == "prunable" || strings.HasPrefix(field, "prunable "):
			tree.reason = "Git 工作树关联已失效，保持原状"
		}
	}
	if len(trees) == 0 || !filepath.IsAbs(trees[0].path) {
		return nil, fmt.Errorf("Git 工作树列表无法核验")
	}
	for i := range trees {
		trees[i].source = trees[0].path
	}
	return trees, nil
}

// Only a registered linked checkout can be moved; the source repository stays.
func gitWorktreeAt(path string) (gitWorktree, error) {
	info, err := os.Lstat(filepath.Join(path, ".git"))
	if err != nil || !info.Mode().IsRegular() {
		return gitWorktree{}, fmt.Errorf("源仓库与未核验的 Git 目录（.git）保持原状，仅清理关联工作树")
	}
	trees, err := gitWorktrees(path)
	if err != nil {
		return gitWorktree{}, fmt.Errorf("Git 工作树关联无法核验，保持原状")
	}
	info, err = os.Stat(path)
	if err == nil {
		for _, tree := range trees[1:] {
			other, err := os.Stat(tree.path)
			if err == nil && os.SameFile(info, other) {
				return tree, nil
			}
		}
	}
	return gitWorktree{}, fmt.Errorf("Git 工作树关联无法核验，保持原状")
}

// Repair both directions of Git's pointers after a Trash move or restore.
func repairGitWorktree(path string) error {
	_, _, err := runGit(path, "worktree", "repair", path)
	return err
}

func worktreeTool(path string) string {
	for _, spec := range agentWorktreeRoots {
		if strings.Contains(filepath.ToSlash(path), "/"+spec.root+"/") {
			return spec.tool
		}
	}
	return "Git 工作树"
}

func (s *Scanner) aiWorktreeInventory() []gitWorktree {
	var projects projectDiscovery
	if s.projects != nil {
		projects = s.projects()
	} else {
		projects = s.discoverProjects(s.PurgeRoots().AbsolutePaths())
	}
	var known []string
	for _, spec := range agentWorktreeRoots {
		root := filepath.Join(s.home, spec.root)
		for _, entry := range readDir(root) {
			if !entry.isDir || isHidden(entry.name) {
				continue
			}
			path := filepath.Join(root, entry.name)
			if exists(filepath.Join(path, ".git")) {
				known = append(known, path)
				continue
			}
			var repos []string
			for _, child := range readDir(path) {
				repo := filepath.Join(path, child.name)
				if child.isDir && exists(filepath.Join(repo, ".git")) {
					repos = append(repos, repo)
				}
			}
			if len(repos) == 0 {
				repos = append(repos, path)
			}
			known = append(known, repos...)
		}
	}
	var found []gitWorktree
	seen := map[string]bool{}
	for _, project := range append(known, projects.repositories...) {
		if seen[strings.ToLower(project)] {
			continue
		}
		trees, err := gitWorktrees(project)
		if err != nil || len(trees) == 1 {
			if tool := worktreeTool(project); tool != "Git 工作树" {
				found = append(found, gitWorktree{path: project, source: project, tool: tool, reason: "Git 工作树关联无法核验，保持原状"})
			}
			seen[strings.ToLower(project)] = true
			continue
		}
		for i, tree := range trees {
			key := strings.ToLower(tree.path)
			already := seen[key]
			seen[key] = true
			if i == 0 || already || !realDir(tree.path) || withinPath(tree.path, filepath.Join(s.home, ".Trash")) {
				continue
			}
			tree.tool = worktreeTool(tree.path)
			found = append(found, tree)
		}
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].source != found[j].source {
			return found[i].source < found[j].source
		}
		return found[i].path < found[j].path
	})
	return found
}
