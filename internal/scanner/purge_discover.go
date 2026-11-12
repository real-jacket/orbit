package scanner

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Ported from tw93/Mole V1.70.0 lib/clean/purge_shared.sh MOLE_PURGE_TARGETS
// The Chinese label is what the page shows as the artifact's kind.
var artifactKinds = map[string]string{
	"node_modules": "前端依赖目录", "target": "Rust / Java 构建产物", "build": "构建产物", "dist": "前端构建产物",
	"venv": "Python 虚拟环境", ".venv": "Python 虚拟环境", ".pytest_cache": "pytest 缓存", ".mypy_cache": "mypy 缓存",
	".tox": "tox 虚拟环境", ".nox": "nox 虚拟环境", ".ruff_cache": "Ruff 缓存", ".gradle": "Gradle 本地缓存",
	".terragrunt-cache": "Terragrunt 模块缓存", "__pycache__": "Python 字节码缓存", ".next": "Next.js 构建缓存",
	".nuxt": "Nuxt 构建缓存", ".output": "Nuxt 构建产物", "vendor": "PHP Composer 依赖目录", "bin": ".NET 构建输出",
	"obj": ".NET / Unity 构建输出", ".turbo": "Turborepo 缓存", ".parcel-cache": "Parcel 缓存",
	".dart_tool": "Dart / Flutter 构建缓存", ".zig-cache": "Zig 构建缓存", "zig-out": "Zig 构建产物",
	".angular": "Angular 构建缓存", ".svelte-kit": "SvelteKit 构建产物", ".astro": "Astro 构建缓存",
	"coverage": "代码覆盖率报告", "DerivedData": "Xcode 构建缓存", "Pods": "CocoaPods 依赖", ".cxx": "Android NDK 构建缓存",
	".expo": "Expo 缓存", ".build": "Swift 构建产物",
}

const cacheDirTagKind = "带 CACHEDIR.TAG 标记的缓存目录"

// Ported from tw93/Mole V1.70.0 lib/clean/project.sh PURGE_WALK_PRUNE_DIRS
var purgePruneDirs = map[string]bool{".git": true, "Library": true, ".Trash": true, "Applications": true}

// Ported from tw93/Mole V1.70.0 lib/clean/purge_shared.sh MOLE_PURGE_MONOREPO_INDICATORS and MOLE_PURGE_PROJECT_INDICATORS
var purgeProjectIndicators = []string{
	"lerna.json", "pnpm-workspace.yaml", "nx.json", "rush.json", ".git",
	"package.json", "Cargo.toml", "go.mod", "pyproject.toml", "requirements.txt", "pom.xml", "build.gradle",
	"terragrunt.hcl", "Gemfile", "composer.json", "pubspec.yaml", "Package.swift", "Makefile", "build.zig", "build.zig.zon",
}

const (
	// purgeMaxDepth is how many levels below a root an artifact may sit.
	purgeMaxDepth = 6
	// artifactDiscoveryBudget is how long discovery may walk one project root.
	artifactDiscoveryBudget = 30 * time.Second
	// purgeProbeBudget is how long all content checks of one scan may take; a
	// check that runs out leaves the artifact in review instead of guessing.
	purgeProbeBudget = 60 * time.Second
	// maxArtifactItems is how many build artifacts the purge list keeps.
	maxArtifactItems = 300
	minArtifactBytes = 20_000_000
	cacheDirTagSig   = "Signature: 8a477f597d28d172789f06886806bc55"
)

// artifactCandidate is one build artifact the scan found.
type artifactCandidate struct {
	path       string
	name       string
	kind       string
	bytes      int64
	modified   int64 // newest file inside, or the folder's own time when that is newer
	partial    bool
	protect    string // why it must stay, from a content check
	unverified string // why a content check could not finish
}

func isPurgeProjectRoot(dir string) bool {
	for _, name := range purgeProjectIndicators {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

// hasCacheDirTag reports whether dir carries a valid CACHEDIR.TAG (a regular
// file, not a link, that starts with the standard signature).
// Ported from tw93/Mole V1.70.0 lib/clean/purge_shared.sh mole_dir_has_cachedir_tag
func hasCacheDirTag(dir string) bool {
	tag := filepath.Join(dir, "CACHEDIR.TAG")
	if info, err := os.Lstat(tag); err != nil || !info.Mode().IsRegular() {
		return false
	}
	file, err := os.Open(tag)
	if err != nil {
		return false
	}
	defer file.Close()
	head := make([]byte, len(cacheDirTagSig))
	n, _ := file.Read(head)
	return string(head[:n]) == cacheDirTagSig
}

// walkArtifacts lists the build artifact directories below root, at most six
// levels deep. It never follows a link, never descends into an artifact, a
// bundle or a pruned folder, and lists a direct child of root only when root is
// itself a project. Running past the deadline leaves one warning on the root.
// Ported from tw93/Mole V1.70.0 lib/clean/project.sh scan_purge_targets
func (s *Scanner) walkArtifacts(root string, deadline time.Time, out *[]artifactCandidate) {
	if !s.walkArtifactsIn(root, root, 1, isPurgeProjectRoot(root), deadline, out) {
		recordReadWarning(root, "构建目录遍历达到扫描上限；只统计已访问路径")
	}
}

// walkArtifactsIn reports false when it stopped at the deadline. depth is the
// level of dir's children below the root.
func (s *Scanner) walkArtifactsIn(root, dir string, depth int, rootIsProject bool, deadline time.Time, out *[]artifactCandidate) bool {
	list := listDirItems
	if dir == root {
		list = listDirGuarded
	}
	items, err := list(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		recordReadError(dir, err)
	}
	for _, item := range items {
		if item.kind != itemDir || purgePruneDirs[item.name] {
			continue
		}
		if time.Now().After(deadline) {
			return false
		}
		name, path := item.name, dir+"/"+item.name
		kind, isArtifact := artifactKinds[name]
		if !isArtifact && !isBundleDir(name) && hasCacheDirTag(path) {
			kind, isArtifact = cacheDirTagKind, true
		}
		if isArtifact {
			if _, protected := s.scanProtection(path); !protected && (depth > 1 || rootIsProject) && artifactShape(path, name) {
				*out = append(*out, artifactCandidate{path: path, name: name, kind: kind})
			}
			continue
		}
		if isBundleDir(name) || depth >= purgeMaxDepth {
			continue
		}
		if !s.walkArtifactsIn(root, path, depth+1, rootIsProject, deadline, out) {
			return false
		}
	}
	return true
}

// artifactShape drops the names that are only build output in a known layout:
// bin in a .NET project, vendor in a Composer project, and not Xcode's own
// DerivedData. Any other bin, vendor or DerivedData is somebody's files.
// Ported from tw93/Mole V1.70.0 lib/clean/project.sh is_dotnet_bin_dir, is_protected_vendor_dir, is_protected_purge_artifact
func artifactShape(path, name string) bool {
	parent := filepath.Dir(path)
	switch name {
	case "bin":
		if !exists(filepath.Join(path, "Debug")) && !exists(filepath.Join(path, "Release")) {
			return false
		}
		for _, pattern := range []string{"*.csproj", "*.fsproj", "*.vbproj"} {
			if matches, _ := filepath.Glob(filepath.Join(parent, pattern)); len(matches) > 0 {
				return true
			}
		}
		return false
	case "vendor":
		return exists(filepath.Join(parent, "composer.json"))
	case "DerivedData":
		return !strings.Contains(path, "/Library/Developer/Xcode/DerivedData")
	}
	return true
}

// hasAuthoredMarker looks for a .git entry or a *-keypair.json file anywhere
// below dir, without following links. complete is false when the walk could not
// finish, which is no evidence either way.
// Ported from tw93/Mole V1.70.0 lib/clean/project.sh purge_artifact_has_authored_content
func hasAuthoredMarker(dir string, deadline time.Time) (found, complete bool) {
	if time.Now().After(deadline) {
		return false, false
	}
	items, err := listDirItems(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, false
	}
	for _, item := range items {
		if item.name == ".git" || strings.HasSuffix(item.name, "-keypair.json") {
			return true, true
		}
	}
	complete = true
	for _, item := range items {
		if item.kind != itemDir {
			continue
		}
		subFound, subComplete := hasAuthoredMarker(dir+"/"+item.name, deadline)
		if subFound {
			return true, true
		}
		complete = complete && subComplete
	}
	return false, complete
}

// gitTracksFiles asks the repository that owns path whether it tracks anything
// below it. known is false when git could not answer.
// Ported from tw93/Mole V1.70.0 lib/clean/purge_shared.sh mole_path_has_git_tracked_files
func gitTracksFiles(path string) (tracked, known bool) {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false, false
	}
	repo := ""
	for dir := real; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			repo = dir
			break
		}
	}
	if repo == "" {
		return false, true
	}
	rel := strings.TrimPrefix(strings.TrimPrefix(real, repo), "/")
	out, _, err := runGit(repo, "ls-files", "-z", "--", ":(top,literal,icase)"+rel)
	if err != nil {
		return false, false
	}
	return len(bytes.TrimSpace(out)) > 0, true
}

// nodeModulesGuard says what to check before a node_modules folder is cleaned: the
// project needs a package.json and a lockfile whose package manager record is
// inside, and nothing inside may be newer than that record. Mole holds these
// back by default, which orbit shows as review (unchecked), not protected.
// Ported from tw93/Mole V1.70.0 lib/clean/project.sh purge_node_modules_is_protected
func nodeModulesGuard(path string, deadline time.Time) (review string) {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "没能在时限内确认里面没有项目自己的文件，请确认后再清理"
	}
	project := filepath.Dir(real)
	regular := func(file string) (os.FileInfo, bool) {
		info, err := os.Lstat(file)
		return info, err == nil && info.Mode().IsRegular()
	}
	if _, ok := regular(filepath.Join(project, "package.json")); !ok {
		return "上一级目录没有 package.json，无法确认依赖能重新安装；确认项目不再需要这些依赖后再清理"
	}
	var marker os.FileInfo
	for _, pair := range [][2]string{
		{"package-lock.json", ".package-lock.json"}, {"npm-shrinkwrap.json", ".package-lock.json"},
		{"pnpm-lock.yaml", ".modules.yaml"}, {"yarn.lock", ".yarn-integrity"}, {"yarn.lock", ".yarn-state.yml"},
	} {
		if _, ok := regular(filepath.Join(project, pair[0])); !ok {
			continue
		}
		if info, ok := regular(filepath.Join(real, pair[1])); ok {
			marker = info
			break
		}
	}
	if marker == nil {
		return "没有找到锁文件或包管理器的安装记录，无法确认依赖能原样重装；确认项目还能重新安装后再清理（npm install、pnpm install 或 yarn install）"
	}
	found, complete := hasNewerFile(real, marker.ModTime().Unix(), deadline)
	if found {
		return "安装之后有文件被改动过，可能是手动打的补丁；确认不需要后再清理，清理后重新执行 npm install（或 pnpm、yarn install）即可重建"
	}
	if !complete {
		return "没能在时限内确认里面没有项目自己的文件，请确认后再清理"
	}
	return ""
}

// Tool state the package managers and bundlers rewrite on their own; it says
// nothing about hand edits. Mole skips only the top-level .cache; orbit also
// skips the other well-known cache folders and install records.
var (
	nodeToolDirs  = map[string]bool{".cache": true, ".vite": true, ".vite-temp": true, ".parcel-cache": true, ".turbo": true, ".eslintcache": true}
	nodeToolFiles = map[string]bool{
		".package-lock.json": true, ".modules.yaml": true, ".yarn-state.yml": true, ".yarn-integrity": true,
		".pnpm-workspace-state.json": true, ".pnpm-workspace-state-v1.json": true, ".eslintcache": true, ".DS_Store": true, "lock.yaml": true,
	}
)

// hasNewerFile looks for a regular file modified after mtime, skipping tool
// caches and records. complete is false when the walk could not finish.
func hasNewerFile(dir string, mtime int64, deadline time.Time) (found, complete bool) {
	if time.Now().After(deadline) {
		return false, false
	}
	items, err := listDirItems(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, false
	}
	complete = true
	for _, item := range items {
		switch {
		case item.kind == itemFile && item.mtime > mtime && !nodeToolFiles[item.name]:
			return true, true
		case item.kind == itemDir && !nodeToolDirs[item.name]:
			subFound, subComplete := hasNewerFile(dir+"/"+item.name, mtime, deadline)
			if subFound {
				return true, true
			}
			complete = complete && subComplete
		}
	}
	return false, complete
}

// guardArtifact runs the content checks on a measured candidate.
func guardArtifact(c *artifactCandidate, deadline time.Time) {
	found, complete := hasAuthoredMarker(c.path, deadline)
	switch {
	case found:
		c.protect = "里面含有 .git 仓库或 *-keypair.json 密钥文件，可能是项目自己的内容，保持原状"
		return
	case !complete:
		c.unverified = "没能在时限内确认里面没有项目自己的文件，请确认后再清理"
	}
	if tracked, known := gitTracksFiles(c.path); tracked {
		c.protect = "Git 正在跟踪里面的文件，可能是项目源码，保持原状"
		return
	} else if !known && c.unverified == "" {
		c.unverified = "没能在时限内确认里面没有项目自己的文件，请确认后再清理"
	}
	if c.name == "node_modules" {
		if review := nodeModulesGuard(c.path, deadline); review != "" {
			c.unverified = review
		}
	}
}

// discoverArtifacts finds the build artifacts under root, measures the ones that
// are big enough to list and runs the content checks on them.
func (s *Scanner) discoverArtifacts(root string, probeDeadline time.Time) []artifactCandidate {
	var found []artifactCandidate
	s.walkArtifacts(root, time.Now().Add(artifactDiscoveryBudget), &found)
	var (
		mu   sync.Mutex
		wg   sync.WaitGroup
		out  []artifactCandidate
		jobs = make(chan artifactCandidate)
	)
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range jobs {
				size := s.fastDirectorySize(c.path, 0)
				if size.bytes < minArtifactBytes && !size.partial {
					continue
				}
				c.bytes, c.partial, c.modified = size.bytes, size.partial, max(size.modified, modifiedTime(c.path))
				guardArtifact(&c, probeDeadline)
				mu.Lock()
				out = append(out, c)
				mu.Unlock()
			}
		}()
	}
	for _, c := range found {
		jobs <- c
	}
	close(jobs)
	wg.Wait()
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path }) // stable input for the size sort
	return out
}

// scanBuildArtifacts lists the build artifacts in the project folders that are
// big enough to be worth removing. An unreadable purge_paths file stops it.
func (s *Scanner) scanBuildArtifacts() []map[string]any {
	return s.scanBuildArtifactsWithRoots(s.PurgeRoots().Paths)
}

// A full scan passes its already-read project scope here so discovery and the
// displayed settings cannot use different revisions of purge_paths.
func (s *Scanner) scanBuildArtifactsWithRoots(roots []string) []map[string]any {
	items := []map[string]any{}
	probeDeadline := time.Now().Add(purgeProbeBudget)
	var candidates []artifactCandidate
	seen := map[string]bool{}
	for _, root := range roots {
		for _, c := range s.discoverArtifacts(root, probeDeadline) {
			if key := strings.ToLower(c.path); !seen[key] {
				seen[key] = true
				candidates = append(candidates, c)
			}
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].bytes > candidates[j].bytes
	})
	if len(candidates) > maxArtifactItems {
		candidates = candidates[:maxArtifactItems]
	}
	for i, c := range candidates {
		project := dirName(c.path)
		// The newest file inside says when the artifact was last used; the
		// project says whether anyone still works in it.
		active := max(c.modified, projectActivity(project))
		v := artifactVerdict(c.path, active)
		switch {
		case c.protect != "":
			v = verdict{tier: "protected", reason: c.protect}
		case v.tier != "protected" && c.unverified != "":
			v = verdict{tier: "review", reason: c.unverified}
		case v.tier == "safe" && c.partial:
			v = verdict{tier: "review", reason: "目录太大没有读完，无法确认里面最近是否改动过，请确认后再清理"}
		}
		sub := map[string]any{
			"path": s.relativize(c.path), "sizeGb": plainGB(c.bytes), "partial": c.partial,
			"tier": v.tier, "checked": v.tier == "safe", "reason": v.reason,
		}
		if v.unlock != "" {
			sub["unlock"] = v.unlock
		}
		row := map[string]any{
			"id":          "artifact_" + itoa64(int64(i)),
			"project":     project,
			"artifact":    c.name,
			"name":        c.name,
			"path":        s.relativize(c.path),
			"kind":        c.kind,
			"sizeGb":      gb(c.bytes),
			"origSizeGb":  gb(c.bytes),
			"age":         ageText(active),
			"rebuildNote": "删除后可重新安装或重新编译",
			"gitClean":    gitCleanState(project),
			"tier":        v.tier,
			"baseTier":    v.tier,
			"partial":     c.partial,
			"checked":     v.tier == "safe",
			"reason":      v.reason,
			"subpaths":    []map[string]any{sub},
		}
		s.applyCleanupProtection(row)
		items = append(items, row)
	}
	return items
}
