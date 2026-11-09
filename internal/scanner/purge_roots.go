package scanner

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
		if exists(filepath.Join(resolved, part)) {
			for _, entry := range readDir(resolved) {
				if strings.EqualFold(entry.name, part) {
					part = entry.name
					break
				}
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
	var paths []string
	seen := map[string]bool{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == "~" || strings.HasPrefix(line, "~/") {
			line = filepath.Join(home, line[1:])
		}
		line = resolvePathCase(line)
		if !seen[line] {
			seen[line] = true
			paths = append(paths, line)
		}
	}
	return paths
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
	data, err := os.ReadFile(PurgePathsPath(s.home))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return PurgeRoots{Error: "无法读取 ~/.config/mole/purge_paths，已停止扫描构建产物：" + err.Error()}
	}
	if paths := parsePurgePaths(string(data), s.home); len(paths) > 0 {
		return PurgeRoots{Paths: paths, Custom: true}
	}
	return PurgeRoots{Paths: s.defaultPurgeRoots()}
}

// purgePathLine is how a root is written to the file: ~ form under home.
func (s *Scanner) purgePathLine(path string) string {
	return s.relativize(path)
}

// ValidatePurgeRoot turns the page's text into an existing project folder.
func (s *Scanner) ValidatePurgeRoot(raw string) (string, error) {
	path := s.expand(strings.TrimSpace(raw))
	if !filepath.IsAbs(path) {
		return "", errors.New("请输入绝对路径或以 ~ 开头的路径")
	}
	path = resolvePathCase(path)
	if path == "/" || path == s.home {
		return "", errors.New("范围太大，请选择具体的项目目录")
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return "", errors.New("不是可读取的文件夹：" + s.relativize(path))
	}
	return path, nil
}

// writePurgePaths replaces the file in one step, so an interrupted write never
// leaves a half-written scope.
func (s *Scanner) writePurgePaths(content string) error {
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
	current := s.PurgeRoots()
	if current.Error != "" {
		return current, errors.New(current.Error)
	}
	content := ""
	if data, readErr := os.ReadFile(PurgePathsPath(s.home)); readErr == nil {
		content = string(data)
	}
	if len(parsePurgePaths(content, s.home)) == 0 {
		content = purgePathsHeader
		for _, path := range current.Paths {
			if exists(path) {
				content += s.purgePathLine(path) + "\n"
			}
		}
	}
	for _, path := range parsePurgePaths(content, s.home) {
		if path == root {
			return s.PurgeRoots(), nil
		}
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return s.afterPurgeWrite(content + s.purgePathLine(root) + "\n")
}

// RemovePurgeRoot removes a folder from the scope. Removing the last one
// leaves the file with only its comments, which means the defaults again.
func (s *Scanner) RemovePurgeRoot(raw string) (PurgeRoots, error) {
	current := s.PurgeRoots()
	if current.Error != "" {
		return current, errors.New(current.Error)
	}
	data, err := os.ReadFile(PurgePathsPath(s.home))
	if err != nil {
		return current, nil // defaults are not in a file; nothing to remove
	}
	target := resolvePathCase(s.expand(strings.TrimSpace(raw)))
	var kept []string
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
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
