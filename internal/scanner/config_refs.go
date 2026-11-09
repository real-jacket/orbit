package scanner

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ConfigRef is one line of a shell, hook or MCP config file that still names a
// path that was just removed. It carries the file and line, never the line's text.
type ConfigRef struct {
	File string `json:"file"` // "~/" plus the path under the home folder
	Line int    `json:"line"`
	Path string `json:"path"` // the removed path the line names
}

// Ported from tw93/Mole V1.70.0 lib/uninstall/config_refs.sh _uninstall_scan_configuration_references
var configRefFiles = []string{
	".zshrc", ".zprofile", ".zshenv", ".bashrc", ".bash_profile", ".profile",
	".config/fish/config.fish",
	".claude/settings.json", ".claude.json", ".codex/hooks.json", ".codex/config.toml",
	".cursor/hooks.json", ".cursor/mcp.json", ".gemini/settings.json",
}

const configRefMaxBytes = 1 << 20

// ConfigReferences lists, read only, the config lines that still name one of
// the removed paths. Only paths that are really gone count, so a path that was
// refused or restored is never reported. orbit never edits these files.
func (s *Scanner) ConfigReferences(removed []string) []ConfigRef {
	home, err := filepath.EvalSymlinks(s.home)
	if err != nil {
		return []ConfigRef{}
	}
	type needle struct{ text, path string }
	var needles []needle
	for _, path := range removed {
		if !strings.HasPrefix(path, "/") || len(path) < 2 {
			continue
		}
		if _, err := os.Lstat(path); err == nil {
			continue
		}
		needles = append(needles, needle{path, path})
		if rel, ok := strings.CutPrefix(path, s.home+"/"); ok {
			needles = append(needles, needle{"~/" + rel, path}, needle{"$HOME/" + rel, path}, needle{"${HOME}/" + rel, path})
		}
	}
	refs := []ConfigRef{}
	if len(needles) == 0 {
		return refs
	}
	for _, rel := range configRefFiles {
		text, ok := readConfigFile(filepath.Join(s.home, rel), home)
		if !ok {
			continue
		}
		display := escapeControl("~/" + rel)
		for i, line := range strings.Split(text, "\n") {
			line = strings.ReplaceAll(line, `\/`, "/")
			for _, n := range needles {
				if lineNamesPath(line, n.text) {
					refs = append(refs, ConfigRef{File: display, Line: i + 1, Path: n.path})
					break
				}
			}
		}
	}
	return refs
}

// readConfigFile reads a small regular file that stays inside the physical
// home and does not change while it is read; anything else is skipped.
func readConfigFile(path, physicalHome string) (string, bool) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() > configRefMaxBytes {
		return "", false
	}
	if resolved, err := filepath.EvalSymlinks(path); err != nil || !strings.HasPrefix(resolved, physicalHome+"/") {
		return "", false
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return "", false
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return "", false
	}
	data, err := io.ReadAll(io.LimitReader(file, configRefMaxBytes+1))
	after, statErr := file.Stat()
	if err != nil || statErr != nil || int64(len(data)) != before.Size() || bytes.IndexByte(data, 0) >= 0 ||
		after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return "", false
	}
	return string(data), true
}

// lineNamesPath is Mole's match: the path stands alone, not glued to a longer
// path or word on its left, and ends at the end of the line or a delimiter.
// Go has no lookbehind, so both sides are checked by hand.
func lineNamesPath(line, needle string) bool {
	for from := 0; from <= len(line)-len(needle); {
		i := strings.Index(line[from:], needle)
		if i < 0 {
			return false
		}
		start, end := from+i, from+i+len(needle)
		leftOK := start == 0 || !strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_./-", rune(line[start-1]))
		rightOK := end == len(line) || strings.ContainsRune("/ \t\r\n\v\f\"')],;}", rune(line[end]))
		if leftOK && rightOK {
			return true
		}
		from = start + 1
	}
	return false
}

// escapeControl renders control characters as \xNN so a hostile file name cannot reshape the output.
func escapeControl(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			fmt.Fprintf(&b, `\x%02x`, r)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
