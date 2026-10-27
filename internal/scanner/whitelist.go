package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The clean whitelist lives in the same file mole reads, so an entry the user
// adds in either place protects the path everywhere. The rules below mirror
// mole's lib/core/base.sh (load_mole_whitelist) and
// lib/core/app_protection.sh (is_path_whitelisted) exactly: the app moved
// deletion from `mo` into its own trash implementation, so it has to enforce
// the same protections itself or a whitelisted path would be deleted anyway.

// finderMetadataSentinel stands for "never touch .DS_Store files". mole stores
// it as a bare word rather than a path, see FINDER_METADATA_SENTINEL.
const finderMetadataSentinel = "FINDER_METADATA"

// protectedWhitelistRoots are rejected with "Protected system path".
var protectedWhitelistRoots = []string{
	"/", "/System", "/bin", "/sbin", "/usr/bin", "/usr/sbin", "/etc", "/var/db",
}

// whitelistDefaultPatterns mirrors DEFAULT_WHITELIST_PATTERNS: protections that
// apply while the user has no whitelist file of their own.
var whitelistDefaultPatterns = []string{
	"~/Library/Caches/ms-playwright*",
	"~/.gradle/caches/*",
	"~/.gradle/daemon/*",
	"~/.ollama/models/*",
	"~/Library/Caches/com.nssurge.surge-mac/*",
	"~/Library/Application Support/com.nssurge.surge-mac/*",
	"~/Library/Caches/org.R-project.R/R/renv/*",
	"~/Library/Caches/JetBrains*",
	"~/Library/Caches/com.jetbrains.toolbox*",
	"~/Library/Caches/tealdeer/tldr-pages",
	"~/Library/Application Support/JetBrains*",
	"~/Library/Caches/com.apple.finder",
	"~/Library/Mobile Documents*",
	finderMetadataSentinel,
}

// whitelistSafetyPatterns mirrors SAFETY_WHITELIST_PATTERNS: merged into every
// whitelist because removing these breaks macOS itself rather than costing a
// rebuild.
var whitelistSafetyPatterns = []string{
	finderMetadataSentinel,
	"~/Library/Caches/com.apple.FontRegistry*",
	"~/Library/Caches/com.apple.spotlight*",
	"~/Library/Caches/com.apple.Spotlight*",
	"~/Library/Caches/CloudKit*",
	"~/Library/Caches/pypoetry/virtualenvs*",
}

// WhitelistPath is the file mole and this app share.
func WhitelistPath(home string) string {
	return filepath.Join(home, ".config", "mole", "whitelist")
}

// Whitelist is the set of protection patterns in effect, plus whatever mole
// refused to load so the UI can show it.
type Whitelist struct {
	Patterns []string
	Warnings []string
}

// ProtectsFinderMetadata reports whether .DS_Store cleanup must be skipped.
func (w Whitelist) ProtectsFinderMetadata() bool {
	for _, pattern := range w.Patterns {
		if pattern == finderMetadataSentinel {
			return true
		}
	}
	return false
}

// UserspacePatterns is the list to show in the UI: the sentinel is an
// instruction, not a path, so it is reported separately.
func (w Whitelist) UserspacePatterns() []string {
	out := make([]string, 0, len(w.Patterns))
	for _, pattern := range w.Patterns {
		if pattern == finderMetadataSentinel {
			continue
		}
		out = append(out, pattern)
	}
	return out
}

// LoadWhitelist reads home's whitelist with mole's validation rules. A missing
// file falls back to the defaults; the safety patterns are always merged.
func LoadWhitelist(home string) Whitelist {
	whitelist := Whitelist{}
	file, err := os.ReadFile(WhitelistPath(home))
	if err != nil {
		for _, pattern := range whitelistDefaultPatterns {
			whitelist.Patterns = append(whitelist.Patterns, expandWhitelistPattern(pattern, home))
		}
	} else {
		for _, line := range strings.Split(string(file), "\n") {
			pattern, warning, ok := normalizeWhitelistEntry(line, home)
			if !ok {
				if warning != "" {
					whitelist.Warnings = append(whitelist.Warnings, warning)
				}
				continue
			}
			if containsString(whitelist.Patterns, pattern) {
				continue
			}
			whitelist.Patterns = append(whitelist.Patterns, pattern)
		}
	}
	for _, pattern := range whitelistSafetyPatterns {
		expanded := expandWhitelistPattern(pattern, home)
		if !containsString(whitelist.Patterns, expanded) {
			whitelist.Patterns = append(whitelist.Patterns, expanded)
		}
	}
	return whitelist
}

// ValidateWhitelistEntry checks one entry the way LoadWhitelist would, so the
// app never writes a line mole would reject.
func ValidateWhitelistEntry(raw, home string) (string, error) {
	pattern, warning, ok := normalizeWhitelistEntry(raw, home)
	if !ok {
		if warning == "" {
			warning = "条目为空"
		}
		return "", fmt.Errorf("%s", warning)
	}
	return pattern, nil
}

// normalizeWhitelistEntry applies mole's per-line rules. ok is false for
// comments and blank lines (warning is empty then) and for rejected entries.
func normalizeWhitelistEntry(line, home string) (pattern, warning string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = expandWhitelistPattern(line, home)
	if strings.Contains(line, "..") {
		return "", "路径不能包含 ..：" + line, false
	}
	if line != finderMetadataSentinel {
		if strings.ContainsFunc(line, isControlRune) {
			return "", "路径格式无效：" + line, false
		}
		if !strings.HasPrefix(line, "/") {
			return "", "必须是绝对路径：" + line, false
		}
	}
	if strings.Contains(line, "//") {
		return "", "路径包含连续斜杠：" + line, false
	}
	if isProtectedWhitelistRoot(line) {
		return "", "受保护的系统路径：" + line, false
	}
	return line, "", true
}

// expandWhitelistPattern replaces a leading ~ and $HOME with home.
func expandWhitelistPattern(pattern, home string) string {
	if strings.HasPrefix(pattern, "~") {
		pattern = home + strings.TrimPrefix(pattern, "~")
	}
	pattern = strings.ReplaceAll(pattern, "${HOME}", home)
	pattern = strings.ReplaceAll(pattern, "$HOME", home)
	return pattern
}

// isProtectedWhitelistRoot reports whether path is one of the roots mole refuses
// to whitelist, or a child of one.
func isProtectedWhitelistRoot(path string) bool {
	for _, root := range protectedWhitelistRoots {
		if path == root {
			return true
		}
		if root != "/" && strings.HasPrefix(path, root+"/") {
			return true
		}
	}
	return false
}

// isControlRune reports whether r is a control character.
func isControlRune(r rune) bool {
	return r < 0x20 || r == 0x7f
}

// containsString reports whether values holds value.
func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

// Protection explains why a path is protected.
type Protection struct {
	// Pattern is the whitelist entry that covers the path.
	Pattern string
	// Descendant is true when the path is only protected because one of its
	// children is: the path itself may still be cleaned, one child at a time,
	// but it may never be moved as a whole.
	Descendant bool
}

// Protection reports whether path is covered by a pattern and how. Callers that
// delete need this distinction: "protected because a child is" means the folder
// must be cleaned by contents rather than moved as a whole.
func (w Whitelist) Protection(path string) (Protection, bool) {
	if path == "" {
		return Protection{}, false
	}
	target := normalizeWhitelistTarget(path)
	for _, pattern := range w.Patterns {
		if pattern == finderMetadataSentinel {
			continue
		}
		candidate := normalizeWhitelistTarget(pattern)
		hasGlob := strings.ContainsAny(candidate, "*?[")
		if target == candidate || matchMoleGlob(candidate, target) {
			return Protection{Pattern: pattern}, true
		}
		// The target is a parent of a protected path: it may not be moved
		// whole, because that would take the protected child with it.
		if strings.HasPrefix(candidate, target+"/") {
			return Protection{Pattern: pattern, Descendant: true}, true
		}
		if !hasGlob && strings.HasPrefix(target, candidate+"/") {
			return Protection{Pattern: pattern}, true
		}
	}
	return Protection{}, false
}

// Protects reports whether path is covered by one of the patterns. It follows
// mole: exact match, glob match, a path that is the parent of a pattern, and a
// path under a pattern that has no glob.
func (w Whitelist) Protects(path string) bool {
	_, protected := w.Protection(path)
	return protected
}

// AllowsWhole reports whether path may be cleaned as a whole: it is not
// protected, and no protected child lives under it either.
func (w Whitelist) AllowsWhole(path string) bool {
	protection, protected := w.Protection(path)
	return !protected || protection.Descendant == false
}

// normalizeWhitelistTarget trims a trailing slash and collapses runs of
// slashes, exactly as mole does before comparing.
func normalizeWhitelistTarget(path string) string {
	path = strings.TrimSuffix(path, "/")
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	return path
}

// matchMoleGlob matches name against a shell-style pattern where * also matches
// slashes, which is what bash's [[ $name == $pattern ]] does in mole.
func matchMoleGlob(pattern, name string) bool {
	if pattern == "" {
		return name == ""
	}
	switch pattern[0] {
	case '*':
		for i := 0; i <= len(name); i++ {
			if matchMoleGlob(pattern[1:], name[i:]) {
				return true
			}
		}
		return false
	case '?':
		if name == "" {
			return false
		}
		return matchMoleGlob(pattern[1:], name[1:])
	case '[':
		closing := strings.IndexByte(pattern[1:], ']')
		if closing < 0 {
			return name != "" && name[0] == '[' && matchMoleGlob(pattern[1:], name[1:])
		}
		class := pattern[1 : 1+closing]
		if name == "" || !matchGlobClass(class, name[0]) {
			return false
		}
		return matchMoleGlob(pattern[2+closing:], name[1:])
	default:
		if name == "" || name[0] != pattern[0] {
			return false
		}
		return matchMoleGlob(pattern[1:], name[1:])
	}
}

// matchGlobClass matches one byte against a [..] character class.
func matchGlobClass(class string, value byte) bool {
	negated := false
	if strings.HasPrefix(class, "!") || strings.HasPrefix(class, "^") {
		negated = true
		class = class[1:]
	}
	matched := false
	for i := 0; i < len(class); i++ {
		if i+2 < len(class) && class[i+1] == '-' {
			if value >= class[i] && value <= class[i+2] {
				matched = true
			}
			i += 2
			continue
		}
		if class[i] == value {
			matched = true
		}
	}
	return matched != negated
}
