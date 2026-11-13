package scanner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mymoles/mole-studio/internal/fsread"
)

// Ported from tw93/Mole V1.70.0 lib/clean/purge_shared.sh MOLE_PURGE_DEFAULT_SEARCH_PATHS
// The agent worktree roots are left out (orbit lists those worktrees on their
// own) and ~/Library/CloudStorage is left out (no online-only folder check).
// ~/src was already an orbit root.
var defaultPurgeRoots = []string{
	"~/www", "~/dev", "~/Projects", "~/GitHub", "~/Code", "~/Workspace", "~/Repos",
	"~/Development", "~/Developer", "~/src",
}

const purgePathsHeader = "# Mole Purge Paths - Auto-discovered project directories\n" +
	"# Edit this file to customize, or run: mo purge --paths\n" +
	"# Add one path per line (supports ~ for home directory)\n"

// PurgeRoots is where the build cleanup looks for projects.
type PurgeRoots struct {
	Paths []string `json:"paths"`
	// Custom is true when ~/.config/mole/purge_paths named them.
	Custom bool `json:"custom"`
	// Error is set when that file exists but cannot be read; discovery stops.
	Error string `json:"error,omitempty"`
	// Roots records missing and unreadable directories without calling every
	// unfinished measurement a Full Disk Access failure.
	Roots []PurgeScanRoot `json:"roots"`
	// Suggested are project folders in the home directory the scope leaves out.
	// They are only shown; nothing is scanned or saved until the user adds one.
	Suggested []string         `json:"suggested"`
	Warnings  []map[string]any `json:"warnings"`
}

// AbsolutePaths returns a copy of the accepted scope, including missing roots.
// An empty saved scope never activates defaults during artifact discovery.
func (roots PurgeRoots) AbsolutePaths() []string {
	return append([]string{}, roots.Paths...)
}

// PurgePathsPath is mole's purge_paths file, shared with `mo purge`.
func PurgePathsPath(home string) string {
	return filepath.Join(home, ".config", "mole", "purge_paths")
}

// resolvePathCase returns path with the on-disk spelling of every component
// that exists, so ~/code and ~/Code are one root on a case-insensitive volume.
// Ported from tw93/Mole V1.70.0 lib/clean/purge_shared.sh mole_purge_resolve_path_case
func resolvePathCase(path string) string {
	resolved := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(filepath.Clean(path), "/"), "/") {
		if part == "" {
			continue
		}
		// Case resolution is configuration parsing, not a scan. A failed or
		// unavailable parent cannot create a permission warning for every module.
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		entries, _ := fsread.ReadDir(ctx, resolved)
		cancel()
		for _, entry := range entries {
			if strings.EqualFold(entry.Name(), part) {
				part = entry.Name()
				break
			}
		}
		resolved = filepath.Join(resolved, part)
	}
	return resolved
}

// parsePurgePaths reads mole's purge_paths format: one path per line, # starts
// a comment, ~ stands for home. A path listed twice, in any letter case, once.
// Ported from tw93/Mole V1.70.0 lib/clean/purge_shared.sh mole_purge_read_paths_config
func parsePurgePaths(content, home string) []string {
	return (&Scanner{home: home}).parsePurgeRoots(content).Paths
}

// parsePurgeRoots is shared by scans and scope editing. Active but invalid
// lines still constitute a custom scope: silently falling back would scan
// folders the user never selected.
func (s *Scanner) parsePurgeRoots(content string) PurgeRoots {
	roots := PurgeRoots{Paths: []string{}, Roots: []PurgeScanRoot{}, Warnings: []map[string]any{}}
	var identities []os.FileInfo
	active := 0
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		roots.Custom = true
		if active >= maxPurgeRoots {
			roots.Warnings = append(roots.Warnings, map[string]any{"path": PurgePathsPath(s.home), "kind": warnConfig, "reason": "扫描目录配置超过 100 项；本次只采用前 100 项"})
			break
		}
		active++
		path, reason := s.normalizeBuildScanRoot(line)
		if reason != "" {
			roots.Warnings = append(roots.Warnings, map[string]any{"path": line, "kind": warnConfig, "reason": reason})
			continue
		}
		info, _ := os.Stat(path)
		duplicate := false
		for i, existing := range roots.Paths {
			if path == existing || info != nil && identities[i] != nil && os.SameFile(info, identities[i]) {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		roots.Paths = append(roots.Paths, path)
		identities = append(identities, info)
	}
	return roots
}

func (s *Scanner) defaultPurgeRoots() []string {
	var paths []string
	for _, root := range defaultPurgeRoots {
		paths = append(paths, s.expand(root))
	}
	return parsePurgePaths(strings.Join(paths, "\n"), s.home)
}

// PurgeRoots reports where the next scan will look. A missing or empty file
// means the defaults; a file that exists but cannot be read stops discovery,
// because scanning the defaults would ignore the scope the user chose.
// Ported from tw93/Mole V1.70.0 lib/clean/project.sh load_purge_config
func (s *Scanner) PurgeRoots() PurgeRoots {
	roots, _, _ := s.loadPurgeRoots()
	return roots
}

func (s *Scanner) loadPurgeRoots() (PurgeRoots, string, bool) {
	content, present, err := readPurgePathConfig(PurgePathsPath(s.home))
	if err != nil {
		reason := "无法读取 ~/.config/mole/purge_paths，已停止扫描构建产物：" + err.Error()
		return PurgeRoots{Paths: []string{}, Custom: true, Error: reason, Roots: []PurgeScanRoot{}, Suggested: []string{}, Warnings: []map[string]any{{"path": PurgePathsPath(s.home), "kind": warnConfig, "reason": reason}}}, content, present
	}
	roots := s.parsePurgeRoots(content)
	if !roots.Custom {
		roots = s.parsePurgeRoots(strings.Join(defaultPurgeRoots, "\n"))
		roots.Custom = false
	}
	for _, path := range roots.Paths {
		roots.Roots = append(roots.Roots, inspectBuildScanRoot(path))
	}
	roots.Suggested = s.suggestedPurgeRoots(roots.Paths)
	return roots, content, present
}

// purgePathLine is how a root is written to the file: ~ form under home.
func (s *Scanner) purgePathLine(path string) string {
	return s.relativize(path)
}

// ValidatePurgeRoot turns the page's text into an existing project folder.
func (s *Scanner) ValidatePurgeRoot(raw string) (string, error) {
	path, reason := s.normalizeBuildScanRoot(strings.TrimSpace(raw))
	if reason != "" {
		return "", errors.New(reason)
	}
	if root := inspectBuildScanRoot(path); root.Status != "readable" {
		return "", errors.New("不是可读取的文件夹：" + s.relativize(path) + "；" + root.Reason)
	}
	return path, nil
}

// writePurgePaths replaces the file in one step, so an interrupted write never
// leaves a half-written scope.
func (s *Scanner) writePurgePaths(content string) error {
	if len(content) > purgeConfigMaxBytes {
		return errors.New("扫描路径配置超过 1 MB")
	}
	path := PurgePathsPath(s.home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".purge_paths-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// AddPurgeRoot adds a folder to the scope. The first edit saves the defaults
// that exist next to it, so choosing one folder does not drop the others.
func (s *Scanner) AddPurgeRoot(raw string) (PurgeRoots, error) {
	root, err := s.ValidatePurgeRoot(raw)
	if err != nil {
		return s.PurgeRoots(), err
	}
	current, content, _ := s.loadPurgeRoots()
	if current.Error != "" {
		return current, errors.New(current.Error)
	}
	if !current.Custom {
		if content == "" {
			content = purgePathsHeader
		} else if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		for _, path := range current.Paths {
			if exists(path) {
				content += s.purgePathLine(path) + "\n"
			}
		}
	}
	for _, path := range current.Paths {
		if path == root {
			return s.PurgeRoots(), nil
		}
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	nextContent := content + s.purgePathLine(root) + "\n"
	for _, accepted := range s.parsePurgeRoots(nextContent).Paths {
		if accepted == root {
			return s.afterPurgeWrite(nextContent)
		}
	}
	return current, errors.New("扫描目录配置已达到 100 项，请先移除不用的目录或编辑 ~/.config/mole/purge_paths")
}

// RemovePurgeRoot removes a folder from the scope. Removing the last one
// leaves the file with only its comments, which means the defaults again.
func (s *Scanner) RemovePurgeRoot(raw string) (PurgeRoots, error) {
	current, content, present := s.loadPurgeRoots()
	if current.Error != "" {
		return current, errors.New(current.Error)
	}
	if !present {
		return current, nil // defaults are not in a file; nothing to remove
	}
	target, reason := s.normalizeBuildScanRoot(strings.TrimSpace(raw))
	if reason != "" {
		return current, errors.New(reason)
	}
	var kept []string
	for _, line := range strings.Split(strings.TrimRight(content, "\n"), "\n") {
		if paths := parsePurgePaths(line, s.home); len(paths) == 1 && paths[0] == target {
			continue
		}
		kept = append(kept, line)
	}
	return s.afterPurgeWrite(strings.Join(kept, "\n") + "\n")
}

func (s *Scanner) afterPurgeWrite(content string) (PurgeRoots, error) {
	if err := s.writePurgePaths(content); err != nil {
		return s.PurgeRoots(), err
	}
	return s.PurgeRoots(), nil
}

const maxSuggestedPurgeRoots = 5

// Folders of the home directory that are never project containers. Desktop,
// Documents and Downloads are left out too: looking inside them can raise a
// macOS permission prompt, which a suggestion is not worth.
var purgeSuggestSkip = map[string]bool{
	"Library": true, "Applications": true, "Movies": true, "Music": true, "Pictures": true, "Public": true,
	"Desktop": true, "Documents": true, "Downloads": true,
}

// suggestedPurgeRoots lists the direct children of home that hold a project
// (an indicator file at most two levels down) and are not scanned yet.
// Ported from tw93/Mole V1.70.0 lib/clean/project.sh discover_project_dirs is_project_container
func (s *Scanner) suggestedPurgeRoots(current []string) []string {
	suggested := []string{}
	if s.home == "" || s.home == "/" {
		return suggested
	}
	deadline := time.Now().Add(2 * time.Second)
	for _, entry := range readDir(s.home) {
		if len(suggested) >= maxSuggestedPurgeRoots || time.Now().After(deadline) {
			break
		}
		if !entry.isDir || isHidden(entry.name) || purgeSuggestSkip[entry.name] || artifactKinds[entry.name] != "" || isBundleDir(entry.name) {
			continue
		}
		path, reason := s.normalizeBuildScanRoot(filepath.Join(s.home, entry.name))
		if reason != "" || !withinPath(path, s.home) {
			continue
		}
		covered := false
		for _, root := range current {
			covered = covered || withinPath(path, root) || withinPath(root, path)
		}
		if covered || containsString(suggested, path) || !holdsProject(path, 2, deadline) {
			continue
		}
		suggested = append(suggested, path)
	}
	return suggested
}

// holdsProject looks for a project indicator in dir or up to depth levels below.
func holdsProject(dir string, depth int, deadline time.Time) bool {
	if isPurgeProjectRoot(dir) {
		return true
	}
	if depth == 0 || time.Now().After(deadline) {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for i, entry := range entries {
		if i >= 200 {
			break
		}
		if entry.IsDir() && !isHidden(entry.Name()) && artifactKinds[entry.Name()] == "" && holdsProject(filepath.Join(dir, entry.Name()), depth-1, deadline) {
			return true
		}
	}
	return false
}
