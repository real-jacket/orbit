package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// personalOwner says who an AI tool or app keeps personal data for. The data
// stays protected while any owner is installed, or when that cannot be decided.
// Once no owner is installed it is leftover data the user may confirm and trash.
type personalOwner struct {
	tool string   // display name used in reasons
	apps []string // app bundle names, without ".app"
	ids  []string // bundle identifiers
	cmds []string // launcher names looked up in known install folders
	bins []string // extra home-relative launcher folders of the tool itself
	exts []string // editor extension folder prefixes
}

const (
	cursorBundleID   = "com.todesktop.230313mzl4w4u92"
	windsurfBundleID = "com.exafunction.windsurf"
)

var (
	// The codex CLI only uses ~/.codex; the app's own folders belong to the app alone. The app may ship as ChatGPT.app, so it is matched by bundle id too.
	codexAppOwner = personalOwner{tool: "Codex", apps: []string{"Codex", "ChatGPT"}, ids: []string{"com.openai.codex"}}
	codexOwner    = personalOwner{tool: "Codex", apps: codexAppOwner.apps, ids: codexAppOwner.ids, cmds: []string{"codex"}}
	cursorOwner   = personalOwner{tool: "Cursor", apps: []string{"Cursor"}, ids: []string{cursorBundleID}, cmds: []string{"cursor-agent", "cursor"}}
	windsurfOwner = personalOwner{tool: "Windsurf", apps: []string{"Windsurf"}, ids: []string{windsurfBundleID}, cmds: []string{"windsurf"}, exts: []string{"codeium.codeium", "codeium.windsurf"}}
	lmStudioOwner = personalOwner{tool: "LM Studio", apps: []string{"LM Studio"}, ids: []string{"com.lmstudio.lmstudio"}, cmds: []string{"lms"}}
	continueOwner = personalOwner{tool: "Continue", cmds: []string{"cn"}, exts: []string{"continue.continue"}}
)

// personalOwners is keyed by the home-relative rule path in builtinCleanupProtection.
// A rule that is not here stays protected for good.
var personalOwners = map[string]personalOwner{
	".codex":                            codexOwner,
	"Library/Application Support/Codex": codexAppOwner,
	"Library/Application Support/com.openai.codex":  codexAppOwner,
	"Library/Caches/com.openai.codex":               codexAppOwner,
	".claude":                                       {tool: "Claude Code", apps: []string{"Claude"}, ids: []string{claudeBundleID}, cmds: []string{"claude"}},
	"Library/Application Support/Claude":            {tool: "Claude", apps: []string{"Claude"}, ids: []string{claudeBundleID}},
	".cursor":                                       cursorOwner,
	"Library/Application Support/Cursor":            {tool: "Cursor", apps: []string{"Cursor"}, ids: []string{cursorBundleID}},
	".copilot":                                      {tool: "GitHub Copilot CLI", cmds: []string{"copilot"}},
	".gemini":                                       {tool: "Gemini CLI", apps: []string{"Antigravity"}, cmds: []string{"gemini"}},
	".continue":                                     continueOwner,
	".codeium":                                      windsurfOwner,
	"Library/Application Support/Windsurf":          {tool: "Windsurf", apps: []string{"Windsurf"}, ids: []string{windsurfBundleID}, cmds: []string{"windsurf"}},
	".local/share/opencode":                         {tool: "OpenCode", apps: []string{"OpenCode"}, cmds: []string{"opencode"}, bins: []string{".opencode/bin"}},
	".lmstudio":                                     lmStudioOwner,
	"Library/Application Support/LM Studio":         {tool: "LM Studio", apps: []string{"LM Studio"}, ids: []string{"com.lmstudio.lmstudio"}},
	"Library/Application Support/ChatGPT":           {tool: "ChatGPT", apps: []string{"ChatGPT"}, ids: []string{"com.openai.chat"}},
	globalStorageRule("rooveterinaryinc.roo-cline"): {tool: "Roo Code", exts: []string{"rooveterinaryinc.roo-cline"}},
	globalStorageRule("saoudrizwan.claude-dev"):     {tool: "Cline", exts: []string{"saoudrizwan.claude-dev"}},
	globalStorageRule("continue.continue"):          {tool: "Continue", exts: []string{"continue.continue"}},
	globalStorageRule("github.copilot-chat"):        {tool: "GitHub Copilot Chat", exts: []string{"github.copilot-chat"}},
}

func globalStorageRule(extension string) string {
	return "Library/Application Support/Code/User/globalStorage/" + extension
}

// ownerRule is the relax function a restore uses: a path that was trashed
// legitimately must be able to come back, whoever owns it now.
func ownerRule(rule string) bool { _, ok := personalOwners[rule]; return ok }

var extensionDirs = []string{".vscode/extensions", ".cursor/extensions", ".windsurf/extensions"}

// launcherDirs are the folders a launcher may live in. Absolute folders belong to
// the real machine, so a scanner with its app folders redirected (tests) skips them.
func (s *Scanner) launcherDirs(extra []string) []string {
	var dirs []string
	for _, pattern := range append(append([]string{}, aiLauncherDirs...), extra...) {
		if filepath.IsAbs(pattern) {
			if s.appDirs != nil {
				continue
			}
		} else {
			pattern = filepath.Join(s.home, pattern)
		}
		matches, _ := filepath.Glob(pattern)
		dirs = append(dirs, matches...)
	}
	return dirs
}

// present is false only when path certainly does not exist: any other error
// means the answer is unknown, and unknown counts as installed.
func present(path string) bool {
	_, err := os.Lstat(path)
	return err == nil || !os.IsNotExist(err)
}

// ownerAbsent is true only when no owner is installed. App bundles at the
// paths in except do not count: an uninstall passes the bundles it is removing.
func (s *Scanner) ownerAbsent(o personalOwner, except map[string]bool) bool {
	for _, dir := range s.defaultAppDirs() {
		for _, name := range o.apps {
			if path := filepath.Join(dir, name+".app"); !except[path] && present(path) {
				return false
			}
		}
		if len(o.ids) == 0 {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return false
		}
		for _, entry := range entries {
			path := filepath.Join(dir, entry.Name())
			if except[path] || !strings.EqualFold(filepath.Ext(path), ".app") {
				continue
			}
			id := readBundleInfo(path).identifier
			for _, want := range o.ids {
				if strings.EqualFold(id, want) {
					return false
				}
			}
		}
	}
	for _, dir := range s.launcherDirs(o.bins) {
		for _, cmd := range o.cmds {
			if present(filepath.Join(dir, cmd)) {
				return false
			}
		}
	}
	for _, rel := range extensionDirs {
		if len(o.exts) == 0 {
			break
		}
		entries, err := os.ReadDir(filepath.Join(s.home, rel))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return false
		}
		for _, entry := range entries {
			for _, prefix := range o.exts {
				if strings.HasPrefix(strings.ToLower(entry.Name()), prefix) {
					return false
				}
			}
		}
	}
	return true
}

// ownerGoneRule is the default relax function: the rule is lifted when none of its owners is installed.
func (s *Scanner) ownerGoneRule(rule string) bool {
	o, ok := personalOwners[rule]
	return ok && s.ownerAbsent(o, nil)
}

// personalRule is the owner rule whose folder holds path (or that path holds).
func (s *Scanner) personalRule(path string) (string, personalOwner, bool) {
	for rule, o := range personalOwners {
		root := filepath.Join(s.home, rule)
		if withinPath(path, root) || withinPath(root, path) {
			return rule, o, true
		}
	}
	return "", personalOwner{}, false
}

func (o personalOwner) ownedBy(app uninstallApp) bool {
	for _, name := range []string{strings.TrimSuffix(filepath.Base(app.path), ".app"), app.name} {
		for _, want := range o.apps {
			if strings.EqualFold(name, want) {
				return true
			}
		}
	}
	for _, want := range o.ids {
		if app.bundleID != "" && strings.EqualFold(app.bundleID, want) {
			return true
		}
	}
	return false
}

// ownerRelaxFor frees the personal data of the apps an uninstall removes, but
// only while no other copy or launcher of the same owner stays installed.
func (s *Scanner) ownerRelaxFor(apps ...uninstallApp) func(rule string) bool {
	except := map[string]bool{}
	for _, app := range apps {
		except[app.path] = true
	}
	return func(rule string) bool {
		o, ok := personalOwners[rule]
		if !ok {
			return false
		}
		if s.ownerAbsent(o, nil) {
			return true
		}
		for _, app := range apps {
			if o.ownedBy(app) {
				return s.ownerAbsent(o, except)
			}
		}
		return false
	}
}

// ownerRunningReason keeps a lifted rule honest: while an owner process runs,
// its data stays where it is.
func (s *Scanner) ownerRunningReason(path string, live bool) string {
	if _, lifted := builtinCleanupProtection(path, s.home, nil); !lifted {
		return "" // no rule was lifted for this path
	}
	_, o, ok := s.personalRule(path)
	if !ok {
		return ""
	}
	processes, err := s.processSnapshot(live)
	if err != nil || strings.TrimSpace(processes) == "" {
		return "无法确认 " + o.tool + " 是否在运行，保持原状"
	}
	for _, command := range strings.Split(processes, "\n") {
		base := filepath.Base(strings.TrimSpace(command))
		for _, owner := range append(append([]string{}, o.apps...), o.cmds...) {
			if base == owner || strings.HasPrefix(base, owner+" ") || strings.HasPrefix(base, owner+".") || strings.Contains(command, "/"+owner+".app/") {
				return o.tool + " 正在运行，暂缓清理"
			}
		}
	}
	return ""
}

// goneReason is the review reason for personal data whose owner is no longer installed.
func (o personalOwner) goneReason() string {
	return fmt.Sprintf("%s 已卸载，这里是它留下的会话与配置；确认不再需要后可清理，移入废纸篓后可从历史还原", o.tool)
}

// ownedReason is the review reason for the personal data of an app being uninstalled.
func (o personalOwner) ownedReason() string {
	return fmt.Sprintf("含 %s 的项目记录与聊天历史；移入废纸篓后可从历史还原", o.tool)
}

// goneLogReason is the review reason for the diagnostic logs of a tool that is no longer installed.
func (o personalOwner) goneLogReason() string {
	return fmt.Sprintf("%s 已卸载，这里是它留下的诊断日志；确认不再需要后可清理，移入废纸篓后可从历史还原", o.tool)
}
