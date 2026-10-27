package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// The whitelist is scanner input, not a UI decoration: every place that decides
// what may be deleted asks for it here. It is cached between scans because the
// payload builders walk thousands of paths, and refreshed whenever the user
// edits it in the app.

// whitelistMu guards the cached policy; Shared is used from several goroutines
// (the window, the tray flyout, the 1.5s ticker).
var whitelistMu sync.RWMutex

// cachedWhitelist holds the last loaded policy per scanner home directory.
var cachedWhitelist = map[string]Whitelist{}

// whitelist returns the protection policy for this scanner's home.
func (s *Scanner) whitelist() Whitelist {
	whitelistMu.RLock()
	policy, ok := cachedWhitelist[s.home]
	whitelistMu.RUnlock()
	if ok {
		return policy
	}
	return s.refreshWhitelist()
}

// refreshWhitelist reloads the policy from disk and caches it.
func (s *Scanner) refreshWhitelist() Whitelist {
	policy := LoadWhitelist(s.home)
	whitelistMu.Lock()
	cachedWhitelist[s.home] = policy
	whitelistMu.Unlock()
	return policy
}

// Whitelisted reports whether path is protected by the current policy.
func (s *Scanner) Whitelisted(path string) bool {
	return s.whitelist().Protects(path)
}

// UserWhitelistPaths lists the entries the user can edit: the FINDER_METADATA
// sentinel is an instruction rather than a path, and composite "a & b" lines
// are split so the app can show and remove each path on its own.
func (s *Scanner) UserWhitelistPaths() []string {
	policy := s.whitelist()
	paths := []string{}
	for _, pattern := range policy.UserspacePatterns() {
		for _, part := range strings.Split(pattern, " & ") {
			part = strings.TrimSpace(part)
			if part != "" {
				paths = append(paths, part)
			}
		}
	}
	return paths
}

// AddWhitelistEntry validates path and appends it to mole's whitelist file, so
// both the app and `mo` stop offering it as cleanable. The file keeps mole's
// one-path-per-line format.
func (s *Scanner) AddWhitelistEntry(raw string) (Whitelist, error) {
	entry, err := ValidateWhitelistEntry(raw, s.home)
	if err != nil {
		return s.whitelist(), err
	}
	if entry == finderMetadataSentinel {
		if err := s.ensureSafetyWhitelist(); err != nil {
			return s.whitelist(), err
		}
		return s.refreshWhitelist(), nil
	}
	path := WhitelistPath(s.home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return s.whitelist(), err
	}
	data, readErr := os.ReadFile(path)
	content := string(data)
	if readErr != nil && !os.IsNotExist(readErr) {
		return s.whitelist(), readErr
	}
	for _, line := range strings.Split(content, "\n") {
		candidate, _, ok := normalizeWhitelistEntry(line, s.home)
		if ok && candidate == entry {
			return s.refreshWhitelist(), nil
		}
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += entry + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return s.whitelist(), err
	}
	return s.refreshWhitelist(), nil
}

// RemoveWhitelistEntry deletes path from the whitelist file, including when it
// is one part of a composite "a & b" line.
func (s *Scanner) RemoveWhitelistEntry(raw string) (Whitelist, error) {
	entry, err := ValidateWhitelistEntry(raw, s.home)
	if err != nil {
		return s.whitelist(), err
	}
	path := WhitelistPath(s.home)
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return s.refreshWhitelist(), nil
		}
		return s.whitelist(), readErr
	}
	lines := strings.Split(string(data), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			kept = append(kept, line)
			continue
		}
		parts := strings.Split(trimmed, " & ")
		remaining := make([]string, 0, len(parts))
		for _, part := range parts {
			candidate, _, ok := normalizeWhitelistEntry(part, s.home)
			if ok && candidate == entry {
				continue
			}
			remaining = append(remaining, strings.TrimSpace(part))
		}
		if len(remaining) == 0 {
			continue // the whole line went away
		}
		kept = append(kept, strings.Join(remaining, " & "))
	}
	content := strings.Join(kept, "\n")
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return s.whitelist(), err
	}
	return s.refreshWhitelist(), nil
}

// ensureSafetyWhitelist writes the always-on safety entries into the file, so
// they survive a user who replaced the file with their own list.
func (s *Scanner) ensureSafetyWhitelist() error {
	path := WhitelistPath(s.home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	content := string(data)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	added := false
	for _, pattern := range whitelistSafetyPatterns {
		expanded := expandWhitelistPattern(pattern, s.home)
		found := false
		for _, line := range strings.Split(content, "\n") {
			candidate, _, ok := normalizeWhitelistEntry(line, s.home)
			if ok && candidate == expanded {
				found = true
				break
			}
		}
		if !found {
			content += expanded + "\n"
			added = true
		}
	}
	if !added {
		return nil
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
