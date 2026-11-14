package scanner

import "path/filepath"

type scanWarningScope struct {
	name  string
	roots []string
}

func (s *Scanner) scanWarningScopes(config PurgeRoots) []scanWarningScope {
	cleanRoots := s.specRoots(cleanGroupSpecs(), "")
	for _, group := range moleGroups {
		for _, rule := range group.rules {
			for _, root := range moleLeadingRoots(rule.paths) {
				cleanRoots = append(cleanRoots, filepath.Join(s.home, root))
			}
		}
	}
	var aiRoots []string
	for _, entry := range aiCacheSpecs() {
		aiRoots = append(aiRoots, s.specRoots([]CatalogSpec{entry.spec}, "")...)
	}
	for _, spec := range aiVersionSpecs {
		aiRoots = append(aiRoots, filepath.Join(s.home, spec.root))
	}
	for _, root := range []string{".codex", ".claude", ".cursor", ".copilot", ".gemini", ".continue", ".codeium", ".local/share/opencode"} {
		aiRoots = append(aiRoots, filepath.Join(s.home, root))
	}
	softwareRoots := []string{"/Applications", "/System/Applications", filepath.Join(s.home, "Applications")}
	appRoots := s.defaultAppDirs()
	aiRoots = append(aiRoots, appRoots...)
	softwareRoots = append(softwareRoots, appRoots...)
	softwareRoots = append(softwareRoots, s.launchRoots()...)
	softwareRoots = append(softwareRoots, "/opt/homebrew/Caskroom", "/usr/local/Caskroom", "/opt/homebrew/Cellar", "/usr/local/Cellar", "/opt/homebrew/bin", "/usr/local/bin", filepath.Join(s.home, "Library/Application Support/Setapp/Applications"))
	ownerRoots := append(append([]string{}, aiLauncherDirs...), extensionDirs...)
	ownerRoots = append(ownerRoots, ".opencode/bin")
	for _, root := range moleLeadingRoots(ownerRoots) {
		if !filepath.IsAbs(root) {
			root = filepath.Join(s.home, root)
		}
		aiRoots = append(aiRoots, root)
		softwareRoots = append(softwareRoots, root)
	}
	for _, family := range []string{"Caches", "Application Support", "Containers", "Group Containers", "Preferences", "Saved Application State", "Logs", "LaunchAgents"} {
		softwareRoots = append(softwareRoots, filepath.Join(s.home, "Library", family))
	}
	for _, signature := range AllSignatures(s.home) {
		for _, fixed := range signature.FixedPaths {
			softwareRoots = append(softwareRoots, fixed.Path)
		}
		for _, wildcard := range signature.Wildcards {
			softwareRoots = append(softwareRoots, wildcard.Parent)
		}
	}
	purgeRoots := append(config.AbsolutePaths(), filepath.Join(s.home, "Downloads"), filepath.Join(s.home, "Library/Caches/Homebrew/downloads"))
	return []scanWarningScope{
		{"clean", cleanRoots}, {"ai_moon", aiRoots}, {"software", softwareRoots}, {"purge", purgeRoots},
	}
}

func (s *Scanner) scanWarningRoots(config PurgeRoots) []string {
	roots := []string{s.home}
	for _, section := range s.scanWarningScopes(config) {
		roots = append(roots, section.roots...)
	}
	return roots
}

// A failed read belongs to every section that includes its path, even if that
// section produced no candidates. Global evidence stays available to runtime
// diagnostics; module pages must not inherit another section's read failures.
func (s *Scanner) scopedReadWarnings(config PurgeRoots) []map[string]any {
	sections := s.scanWarningScopes(config)
	// Include configured projects on external volumes as well as home reads.
	allRoots := []string{s.home}
	for _, section := range sections {
		allRoots = append(allRoots, section.roots...)
	}
	warnings := scanReadWarningsForRoots(allRoots)
	for _, warning := range warnings {
		path, _ := warning["path"].(string)
		modules := []string{}
		for _, section := range sections {
			for _, root := range section.roots {
				if withinPath(path, root) || withinPath(root, path) {
					modules = append(modules, section.name)
					break
				}
			}
		}
		warning["modules"] = modules
	}
	for _, configWarning := range config.Warnings {
		warning := make(map[string]any, len(configWarning)+1)
		for key, value := range configWarning {
			warning[key] = value
		}
		warning["modules"] = []string{"purge"}
		warnings = append(warnings, warning)
	}
	return warnings
}
