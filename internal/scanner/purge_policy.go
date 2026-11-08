package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	artifactSafeDays   = 30 // untouched this long: recommended, pre-checked
	artifactReviewDays = 7  // untouched this long: offered, needs a look
	worktreeSafeDays   = 7  // agent worktree artifacts go idle sooner
)

// rebuildCommand names the real command that brings back artifact name in
// project, read from the lockfiles and manifests beside it. "" when unknown.
func rebuildCommand(project, name string) string {
	has := func(files ...string) bool {
		for _, file := range files {
			if exists(filepath.Join(project, file)) {
				return true
			}
		}
		return false
	}
	switch name {
	case "node_modules":
		switch {
		case has("pnpm-lock.yaml"):
			return "pnpm install"
		case has("yarn.lock"):
			return "yarn install"
		case has("bun.lockb", "bun.lock"):
			return "bun install"
		case has("package-lock.json", "npm-shrinkwrap.json", "package.json"):
			return "npm install"
		}
	case ".venv", "venv":
		switch {
		case has("uv.lock"):
			return "uv sync"
		case has("poetry.lock"):
			return "poetry install"
		case has("Pipfile.lock", "Pipfile"):
			return "pipenv install"
		case has("requirements.txt"):
			return "pip install -r requirements.txt"
		}
	case "target":
		switch {
		case has("Cargo.toml"):
			return "cargo build"
		case has("pom.xml"):
			return "mvn package"
		case has("build.gradle", "build.gradle.kts"):
			return "gradle build"
		}
	case "dist", "build", ".next", ".nuxt", ".turbo", ".parcel-cache":
		switch {
		case has("pnpm-lock.yaml"):
			return "pnpm build"
		case has("yarn.lock"):
			return "yarn build"
		case has("package.json"):
			return "npm run build"
		case has("build.gradle", "build.gradle.kts"):
			return "gradle build"
		case has("pom.xml"):
			return "mvn package"
		case has("Cargo.toml"):
			return "cargo build"
		}
	}
	return ""
}

// projectActivity is the newest sign of work on project outside the artifact:
// the project folder, its manifest and the git index (commits and checkouts
// rewrite it). shortcut: edits deep in source folders change none of these, so
// the 30-day line is a heuristic, and a running builder is checked separately.
func projectActivity(project string) int64 {
	latest := modifiedTime(project)
	for _, file := range []string{"package.json", "Cargo.toml", "pyproject.toml", ".git/index", ".git/HEAD"} {
		latest = max(latest, modifiedTime(filepath.Join(project, file)))
	}
	return latest
}

// idleDays is how many whole days ago modified was; unknown times count as busy.
func idleDays(modified int64) int {
	if modified <= 0 {
		return 0
	}
	return int(max(nowUnix()-modified, 0) / 86400)
}

// artifactVerdict is the recommendation for one build artifact. The live
// builder check runs afterwards and can still demote it to protected.
func artifactVerdict(path string, modified int64) verdict {
	name, project := filepath.Base(path), filepath.Dir(path)
	days := idleDays(modified)
	how := rebuildCommand(project, name)
	if how == "" {
		how = "重新安装或重新编译"
	}
	if ambiguousArtifactName(name) && !gitIgnored(path) {
		return verdict{tier: "review", reason: "没有被 .gitignore 忽略，可能是项目自带的文件而不是构建产物，请确认后再清理"}
	}
	switch {
	case modified <= 0:
		return verdict{tier: "review", reason: "读不到修改时间，无法判断是否还在使用，请确认后再清理"}
	case days >= artifactSafeDays:
		return verdict{tier: "safe", reason: fmt.Sprintf("%d 天未修改，可用 %s 重建", days, how)}
	case days >= artifactReviewDays:
		return verdict{tier: "review", reason: fmt.Sprintf("%d 天未修改，项目可能仍在使用；清理后需用 %s 重建", days, how)}
	}
	return verdict{tier: "protected", reason: fmt.Sprintf("最近 %d 天内刚修改过，项目可能正在使用，暂缓清理", days),
		unlock: "该项目闲置一周以上后重新扫描即可清理"}
}

// worktreeArtifactVerdict is the same decision for an artifact inside an agent
// worktree: idle >= 7 days is safe, newer is review. Agents rebuild on demand.
func (s *Scanner) worktreeArtifactVerdict(path string, size dirSize) verdict {
	name := filepath.Base(path)
	kind := worktreeArtifactNames[name]
	how := rebuildCommand(filepath.Dir(path), name)
	if how == "" {
		how = "重新安装或重新编译"
	}
	if ambiguousArtifactName(name) && !gitIgnored(path) {
		return verdict{tier: "review", reason: "没有被 .gitignore 忽略，可能是工作树自带的文件而不是构建产物，请确认后再清理"}
	}
	days := idleDays(size.modified)
	if size.modified > 0 && days >= worktreeSafeDays {
		return verdict{tier: "safe", reason: fmt.Sprintf("%s，%d 天未修改，需要时可用 %s 重建", kind, days, how)}
	}
	return verdict{tier: "review", reason: fmt.Sprintf("%s，最近有改动，工作树可能仍在使用；清理后需用 %s 重建", kind, how)}
}

// worktreeArtifacts lists the regenerable directories of one agent worktree:
// at the root and one or two levels down (monorepo packages), never inside
// .git or another artifact. shortcut: bounded breadth-first listing, a deeper
// monorepo layout is missed, raise the depth when one shows up.
func worktreeArtifacts(worktree string) []string {
	var found []string
	var walk func(dir string, depth int)
	visited := 0
	walk = func(dir string, depth int) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Name() == ".git" || strings.HasPrefix(entry.Name(), ".") && worktreeArtifactNames[entry.Name()] == "" {
				continue
			}
			if visited++; visited > 400 {
				return
			}
			path := filepath.Join(dir, entry.Name())
			if worktreeArtifactNames[entry.Name()] != "" {
				found = append(found, path)
			} else if depth < 2 {
				walk(path, depth+1)
			}
		}
	}
	walk(worktree, 0)
	return found
}
