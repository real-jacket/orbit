package scanner

import (
	"fmt"
	"path/filepath"
)

const artifactSafeDays = 30 // untouched this long: recommended, pre-checked

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
	case ".build":
		if has("Package.swift") {
			return "swift build"
		}
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
	if scratch := swiftPMOutputRoot(path); scratch != "" {
		name, project = ".build", filepath.Dir(scratch)
	}
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
	}
	return verdict{tier: "review", reason: fmt.Sprintf("%d 天未修改；确认不再需要这些产物后，可清理并用 %s 重建", days, how)}
}
