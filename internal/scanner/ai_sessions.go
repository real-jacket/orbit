package scanner

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Expired agent conversations are the one place session history may be removed:
// only whole units that are older than the window, are not among the newest few
// of their project, and are not open. Everything else under these roots stays
// protected.
const (
	aiSessionRetention  = 90 * 24 * time.Hour
	aiSessionKeepNewest = 5
	aiSessionGroupCap   = 40
	aiSessionTotalCap   = 300
	aiSessionWalkCap    = 5000
)

// aiUnitCacheTTL is a var so tests that edit files between calls can turn it off.
var aiUnitCacheTTL = 10 * time.Second

var (
	aiUUIDName     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	aiCodexRollout = regexp.MustCompile(`^rollout-.+\.jsonl(\.zst)?$`)
	aiYearName     = regexp.MustCompile(`^\d{4}$`)
	aiTwoDigitName = regexp.MustCompile(`^\d{2}$`)
)

// aiSessionFlatRoots hold one disposable entry per file or folder; no project ranking applies.
var aiSessionFlatRoots = []struct{ tool, root, label string }{
	{"Claude Code", ".claude/shell-snapshots", "Shell 快照"},
	{"Claude Code", ".claude/paste-cache", "粘贴缓存"},
	{"Claude Code", ".claude/todos", "任务清单"},
	{"Codex", ".codex/shell_snapshots", "Shell 快照"},
}

type aiSessionUnit struct {
	tool, group string
	paths       []string
	newest      time.Time
}

// aiSessionShape is the path-only test for "could be an expired unit member".
// It never permits a path by itself; aiSessionCleanupReason decides.
func aiSessionShape(path, home string) bool {
	rel := func(root string) []string {
		tail, ok := strings.CutPrefix(path, filepath.Join(home, root)+"/")
		if !ok || tail == "" {
			return nil
		}
		return strings.Split(tail, "/")
	}
	if p := rel(".codex/sessions"); len(p) == 4 {
		return aiYearName.MatchString(p[0]) && aiTwoDigitName.MatchString(p[1]) && aiTwoDigitName.MatchString(p[2]) && aiCodexRollout.MatchString(p[3])
	}
	if p := rel(".codex/archived_sessions"); len(p) == 1 {
		return aiCodexRollout.MatchString(p[0])
	}
	if p := rel(".claude/projects"); len(p) == 2 {
		return !isHidden(p[0]) && aiUUIDName.MatchString(strings.TrimSuffix(p[1], ".jsonl"))
	}
	if p := rel(".claude/file-history"); len(p) == 1 {
		return aiUUIDName.MatchString(p[0])
	}
	for _, flat := range aiSessionFlatRoots {
		if p := rel(flat.root); len(p) == 1 && !isHidden(p[0]) {
			return true
		}
	}
	return false
}

// aiNewestModified is the newest mtime in a unit; a tree too large or unreadable to verify is "now".
func aiNewestModified(path string) time.Time {
	info, err := os.Lstat(path)
	if err != nil {
		recordReadError(path, err)
		return time.Now()
	}
	newest, count := info.ModTime(), 0
	if info.IsDir() {
		err = filepath.WalkDir(path, func(current string, entry fs.DirEntry, err error) error {
			if err != nil {
				recordReadError(current, err)
				return err
			}
			if count++; count > aiSessionWalkCap {
				return fs.SkipAll
			}
			info, err := entry.Info()
			if err != nil {
				recordReadError(current, err)
				return err
			}
			if info.ModTime().After(newest) {
				newest = info.ModTime()
			}
			return nil
		})
		if err != nil || count > aiSessionWalkCap {
			return time.Now()
		}
	}
	return newest
}

func aiRealEntry(entry os.DirEntry) bool {
	return entry.Type()&os.ModeSymlink == 0 && !isHidden(entry.Name())
}

// aiSessionUnits lists every expired unit, uncapped. Many callers ask per path
// (analyze tiles, trash), so the enumeration is reused briefly; it only answers
// membership and ranking, and aiSessionCleanupReason re-stats the unit itself.
func (s *Scanner) aiSessionUnits() []aiSessionUnit {
	if s.aiUnits != nil {
		return s.aiUnits
	}
	s.unitMu.Lock()
	defer s.unitMu.Unlock()
	if s.unitCache == nil || time.Since(s.unitAt) > aiUnitCacheTTL {
		s.unitCache, s.unitAt = s.enumerateAISessionUnits(), time.Now()
	}
	return s.unitCache
}

func (s *Scanner) enumerateAISessionUnits() []aiSessionUnit {
	cutoff := time.Now().Add(-aiSessionRetention)
	units := []aiSessionUnit{}
	// expire keeps the newest few per ranking set, whatever the install detection says.
	expire := func(set []aiSessionUnit) {
		sort.Slice(set, func(i, j int) bool { return set[i].newest.After(set[j].newest) })
		for i, unit := range set {
			if i >= aiSessionKeepNewest && unit.newest.Before(cutoff) {
				units = append(units, unit)
			}
		}
	}
	// Codex: rollouts are named by session; the project (cwd) lives only inside the file, which is never read.
	var codex []aiSessionUnit
	addRollout := func(path string, entry os.DirEntry) {
		if info, err := entry.Info(); err == nil && info.Mode().IsRegular() && aiCodexRollout.MatchString(entry.Name()) && aiRealEntry(entry) {
			codex = append(codex, aiSessionUnit{tool: "Codex", paths: []string{path}, newest: info.ModTime()})
		}
	}
	archived := filepath.Join(s.home, ".codex/archived_sessions")
	entries, _ := readDirEntries(archived)
	for _, entry := range entries {
		addRollout(filepath.Join(archived, entry.Name()), entry)
	}
	dated := filepath.Join(s.home, ".codex/sessions")
	for _, year := range aiSubdirs(dated) {
		for _, month := range aiSubdirs(year) {
			for _, day := range aiSubdirs(month) {
				entries, _ := readDirEntries(day)
				for _, entry := range entries {
					addRollout(filepath.Join(day, entry.Name()), entry)
				}
			}
		}
	}
	for i := range codex {
		codex[i].group = codex[i].newest.Format("2006-01")
	}
	// Months are ranking-independent groups, but the newest-N rule is per tool.
	expire(codex)

	// Claude Code: a session is <project>/<uuid>.jsonl plus <project>/<uuid>/ and file-history/<uuid>/.
	history := map[string]string{}
	historyRoot := filepath.Join(s.home, ".claude/file-history")
	entries, _ = readDirEntries(historyRoot)
	for _, entry := range entries {
		if entry.IsDir() && aiRealEntry(entry) && aiUUIDName.MatchString(entry.Name()) {
			history[entry.Name()] = filepath.Join(historyRoot, entry.Name())
		}
	}
	projectsRoot := filepath.Join(s.home, ".claude/projects")
	for _, project := range aiSubdirs(projectsRoot) {
		bySession := map[string]*aiSessionUnit{}
		entries, _ := readDirEntries(project)
		for _, entry := range entries {
			sid := strings.TrimSuffix(entry.Name(), ".jsonl")
			isFile, isDirectory := entry.Name() == sid+".jsonl", entry.Name() == sid && entry.IsDir()
			if !aiRealEntry(entry) || !aiUUIDName.MatchString(sid) || !isFile && !isDirectory {
				continue
			}
			path := filepath.Join(project, entry.Name())
			unit := bySession[sid]
			if unit == nil {
				unit = &aiSessionUnit{tool: "Claude Code", group: filepath.Base(project)}
				bySession[sid] = unit
			}
			unit.paths = append(unit.paths, path)
			if mod := aiNewestModified(path); mod.After(unit.newest) {
				unit.newest = mod
			}
		}
		var set []aiSessionUnit
		for sid, unit := range bySession {
			if path, ok := history[sid]; ok {
				unit.paths = append(unit.paths, path)
				if mod := aiNewestModified(path); mod.After(unit.newest) {
					unit.newest = mod
				}
				delete(history, sid)
			}
			set = append(set, *unit)
		}
		expire(set)
	}
	for _, path := range history { // history of a session whose transcript is gone
		if newest := aiNewestModified(path); newest.Before(cutoff) {
			units = append(units, aiSessionUnit{tool: "Claude Code", group: "文件历史", paths: []string{path}, newest: newest})
		}
	}
	for _, flat := range aiSessionFlatRoots {
		root := filepath.Join(s.home, flat.root)
		entries, _ := readDirEntries(root)
		for _, entry := range entries {
			if !aiRealEntry(entry) {
				continue
			}
			path := filepath.Join(root, entry.Name())
			if newest := aiNewestModified(path); newest.Before(cutoff) {
				units = append(units, aiSessionUnit{tool: flat.tool, group: flat.label, paths: []string{path}, newest: newest})
			}
		}
	}
	sort.Slice(units, func(i, j int) bool {
		if units[i].tool != units[j].tool || units[i].group != units[j].group {
			return units[i].tool+"\x00"+units[i].group < units[j].tool+"\x00"+units[j].group
		}
		return units[i].newest.Before(units[j].newest)
	})
	return units
}

func aiSubdirs(root string) []string {
	var dirs []string
	entries, _ := readDirEntries(root)
	for _, entry := range entries {
		if entry.IsDir() && aiRealEntry(entry) {
			dirs = append(dirs, filepath.Join(root, entry.Name()))
		}
	}
	return dirs
}

// aiSessionCleanupReason re-verifies age, ranking and open state at move time.
// known=false means the path is not shaped like a session unit member.
// Display callers (live=false) skip the per-path process table and lsof, which
// cost hundreds of forks over an analyze tree; a snapshot handleProbe still applies.
func (s *Scanner) aiSessionCleanupReason(path string, live bool) (string, bool) {
	if !aiSessionShape(path, s.home) {
		return "", false
	}
	if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSymlink != 0 {
		return "会话路径无法核验，保持原状", true
	}
	var found *aiSessionUnit
	units := s.aiSessionUnits()
	for i := range units {
		for _, member := range units[i].paths {
			if member == path {
				found = &units[i]
			}
		}
	}
	if found == nil {
		return "只有超过 90 天未修改、且不属于项目最新会话的单元才能处理；近期会话、凭据与配置保持原状", true
	}
	for _, member := range found.paths { // the cached list may be seconds old; the files decide
		if !aiNewestModified(member).Before(time.Now().Add(-aiSessionRetention)) {
			return "会话在列出之后又有修改，保持原状", true
		}
	}
	if !live && s.handleProbe == nil {
		return "", true
	}
	if live {
		processes, err := s.cleanupProcesses()
		if err != nil || strings.TrimSpace(processes) == "" {
			return "进程状态未知，不能证明会话闲置，保持原状", true
		}
	}
	open, err := s.aiVersionHasOpenHandle(path)
	if err != nil {
		return "无法确认会话是否仍被打开，保持原状", true
	}
	if open {
		return "仍有进程打开这份会话数据，保持原状", true
	}
	return "", true
}

// scanAISessions reports expired units as review rows, one per tool and project or month.
func (s *Scanner) scanAISessions(claims *catalogClaims) []map[string]any {
	units := s.aiSessionUnits()
	if len(units) == 0 {
		return nil
	}
	// One process table and one open-file table serve every row of this scan;
	// the move boundary still asks per path.
	view := &Scanner{home: s.home, processProbe: s.processProbe, handleProbe: s.handleProbe, installProbe: s.installProbe, aiUnits: units}
	if view.processProbe == nil {
		output, err := s.processSnapshot(false)
		view.processProbe = func() (string, error) { return output, err }
	}
	if view.handleProbe == nil {
		view.handleProbe = s.openSnapshot
	}
	if view.handleProbe == nil {
		view.handleProbe = openFileSnapshot()
	}
	var rows []map[string]any
	listed := 0
	for start := 0; start < len(units); {
		end := start
		for end < len(units) && units[end].tool == units[start].tool && units[end].group == units[start].group {
			end++
		}
		group := units[start:end]
		start = end
		tool, label := group[0].tool, group[0].group
		hidden := 0
		if len(group) > aiSessionGroupCap || listed+len(group) > aiSessionTotalCap {
			keep := min(aiSessionGroupCap, aiSessionTotalCap-listed)
			hidden, group = len(group)-max(keep, 0), group[:max(keep, 0)]
		}
		if len(group) == 0 {
			recordReadWarning(filepath.Dir(units[start-1].paths[0]), "过期会话较多，已达到列出上限；其余未计入本次清理候选")
			continue
		}
		listed += len(group)
		var roots []string
		for _, unit := range group {
			roots = append(roots, unit.paths...)
		}
		oldest, newest := group[0].newest, group[len(group)-1].newest
		impact := fmt.Sprintf("这些是超过 90 天未修改的 %s 对话历史与资源（%d 个会话单元），不能重建；移除后无法继续或回看对应对话。已保留近 90 天内修改的内容、被进程打开的文件，以及凭据、配置、历史索引、模型与工作树。会移入废纸篓，可在“历史”中还原；默认不选。", tool, len(group))
		impact += " 每个项目仍保留最新 5 个会话。"
		if tool == "Codex" {
			impact += " 移走后 Codex 自己的会话列表或索引可能仍显示这些条目，在从“历史”还原之前无法继续这些会话。"
		}
		if hidden > 0 {
			impact += fmt.Sprintf(" 另有 %d 个过期单元未列出，不计入本次候选。", hidden)
			recordReadWarning(filepath.Dir(units[start-1].paths[0]), "过期会话较多，只列出上限内的单元；其余未计入本次清理候选")
		}
		row := view.catalog(CatalogSpec{
			ID: "ai_sessions:" + tool + ":" + label, Category: tool, Name: tool + " · 过期会话 · " + label, Roots: roots, Tier: "review",
			Reason: "超过 90 天未修改的对话历史，不能重建；移除后无法继续或回看这些对话，可在“历史”中还原",
			Impact: impact, Rebuild: "对话历史不可重建；废纸篓中可还原",
		}, claims, nil)
		if row != nil {
			now := time.Now()
			row["band"], row["tool"] = "过期会话", tool
			row["ageText"] = fmt.Sprintf("%d–%d 天前最后修改", int(now.Sub(newest).Hours()/24), int(now.Sub(oldest).Hours()/24))
			rows = append(rows, row)
		}
	}
	return rows
}

// openFileSnapshot runs one system-wide lsof and answers per-path open checks from it.
func openFileSnapshot() func(string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "/usr/sbin/lsof", "-nP", "-Fn").Output()
	if _, partial := err.(*exec.ExitError); partial && len(output) > 0 {
		err = nil // lsof exits 1 when it could not inspect some process
	}
	var names []string
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "n/") {
			names = append(names, line[1:])
		}
	}
	if len(names) == 0 {
		err = fmt.Errorf("lsof 没有返回已打开文件")
	}
	return func(path string) (bool, error) {
		if err != nil {
			return true, err
		}
		for _, name := range names {
			if name == path || strings.HasPrefix(name, path+"/") {
				return true, nil
			}
		}
		return false, nil
	}
}

// Installed means a launcher on PATH or in a known install folder, or an app bundle.
type aiInstall struct{ commands, apps []string }

var aiInstallSpecs = map[string]aiInstall{
	"Codex":                 {[]string{"codex"}, []string{"Codex"}},
	"Claude Code":           {[]string{"claude"}, []string{"Claude"}},
	"Claude Desktop":        {nil, []string{"Claude"}},
	"Cursor / Cursor Agent": {[]string{"cursor-agent", "cursor"}, []string{"Cursor"}},
	"Cursor Agent":          {[]string{"cursor-agent", "cursor"}, []string{"Cursor"}},
	"Cursor":                {[]string{"cursor-agent", "cursor"}, []string{"Cursor"}},
	"Windsurf":              {[]string{"windsurf"}, []string{"Windsurf"}},
	"ChatGPT":               {nil, []string{"ChatGPT"}},
	"LM Studio":             {[]string{"lms"}, []string{"LM Studio"}},
	"GitHub Copilot CLI":    {[]string{"copilot"}, nil},
	"Gemini CLI":            {[]string{"gemini"}, nil},
	"OpenCode":              {[]string{"opencode"}, nil},
}

// aiLauncherDirs are home-relative globs, or absolute folders.
var aiLauncherDirs = []string{
	".local/bin", ".volta/bin", ".bun/bin", ".npm-global/bin", ".claude/local", "Library/pnpm",
	".local/share/mise/shims", ".asdf/shims", ".nvm/versions/node/*/bin",
	".fnm/node-versions/*/installation/bin", ".local/share/fnm/node-versions/*/installation/bin",
	"Library/Application Support/fnm/node-versions/*/installation/bin",
	"/opt/homebrew/bin", "/usr/local/bin",
}

// aiToolUninstalled is true only for a known tool with no launcher and no app; unknown tools count as installed.
func (s *Scanner) aiToolUninstalled(tool string) bool {
	spec, known := aiInstallSpecs[tool]
	if !known {
		return false
	}
	if s.installProbe != nil {
		return !s.installProbe(spec)
	}
	for _, command := range spec.commands {
		if _, err := exec.LookPath(command); err == nil {
			return false
		}
	}
	return s.ownerAbsent(personalOwner{apps: spec.apps, cmds: spec.commands}, nil)
}
