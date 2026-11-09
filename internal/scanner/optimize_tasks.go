package scanner

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	savedStateAgeDays = 30        // Ported from tw93/Mole V1.70.0 lib/core/base.sh MOLE_SAVED_STATE_AGE_DAYS
	sqliteMaxSize     = 104857600 // Ported from tw93/Mole V1.70.0 lib/optimize/tasks.sh MOLE_SQLITE_MAX_SIZE
	plistLintBatch    = 256       // Ported from tw93/Mole V1.70.0 lib/optimize/maintenance.sh _repair_preference_plists_in_dir (batch lint)
	receiptDetailCap  = 20        // the page lists at most this many touched items per receipt
	dsstoreDomain     = "com.apple.desktopservices"
	spotlightDomain   = "com.apple.spotlight"
	spotlightRulesKey = "EnabledPreferenceRules"
	globalDomain      = "NSGlobalDomain"
)

func isExitFailure(err error) bool { return err != nil && strings.Contains(err.Error(), "exit status") }

func capList(list []string, n int) []string {
	if len(list) > n {
		return list[:n]
	}
	return list
}

func (s *Scanner) shortPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		out = append(out, s.displayHome(p))
	}
	return out
}

// trashStep moves paths to the Trash through the shared manifest path and turns
// the outcome into a receipt plus the record that lets the move be undone.
func (s *Scanner) trashStep(paths []string) (OptimizeReceipt, optimizeUndoRecord) {
	out := s.TrashPaths(paths)
	r := OptimizeReceipt{}
	for _, m := range out.Moved {
		r.Details = append(r.Details, s.displayHome(m.Original))
	}
	r.Details = capList(r.Details, receiptDetailCap)
	r.Failed = append(r.Failed, out.Failed...)
	for _, sk := range out.Skipped {
		r.Failed = append(r.Failed, TrashFailure{Path: sk.Path, Error: sk.Reason})
	}
	if out.RestoreError != "" {
		r.Details = append(r.Details, out.RestoreError)
	}
	switch {
	case len(out.Moved) > 0 && len(r.Failed) == 0:
		r.OK, r.Outcome, r.Summary = true, OutcomeApplied, fmt.Sprintf("已把 %d 项移入废纸篓", len(out.Moved))
	case len(out.Moved) > 0:
		r.OK, r.Outcome, r.Summary = true, OutcomeAttention, fmt.Sprintf("已把 %d 项移入废纸篓，另有 %d 项没有处理", len(out.Moved), len(r.Failed))
	default:
		r.Outcome, r.Summary, r.Error = OutcomeFailed, "没有移动任何项目", "这些项目都没能移入废纸篓"
	}
	return r, optimizeUndoRecord{Trashed: out.Moved}
}

// lintPlists returns the files plutil reports as corrupt. A file that cannot be
// read is unknown, never corrupt, and a plutil that fails to run condemns nothing.
// Ported from tw93/Mole V1.70.0 lib/optimize/maintenance.sh _repair_preference_plists_in_dir
func (s *Scanner) lintPlists(ctx context.Context, files []string) (broken []string, partial bool) {
	for i := 0; i < len(files); i += plistLintBatch {
		if ctx.Err() != nil {
			return broken, true
		}
		chunk := files[i:min(i+plistLintBatch, len(files))]
		if _, err := s.runCmd(ctx, "", "/usr/bin/plutil", append([]string{"-lint"}, chunk...)...); err == nil || !isExitFailure(err) {
			continue
		}
		for _, f := range chunk {
			if h, err := os.Open(f); err != nil {
				continue
			} else {
				h.Close()
			}
			if _, err := s.runCmd(ctx, "", "/usr/bin/plutil", "-lint", f); isExitFailure(err) && ctx.Err() == nil && !s.whitelisted(f) {
				broken = append(broken, f)
			}
		}
	}
	return broken, false
}

func clearDiag(reason string) optimizeDiag {
	return optimizeDiag{status: OptimizeClear, reason: reason}
}
func unavailDiag(reason string) optimizeDiag {
	return optimizeDiag{status: OptimizeUnavailable, reason: reason, tier: "protected"}
}

// ---- cache_refresh ----

func diagCacheRefresh(*Scanner, context.Context) optimizeDiag {
	return optimizeDiag{status: OptimizeNeeded, reason: "缩略图和图标缓存可以随时重建，刷新后预览会重新生成", tier: "safe"}
}

// Ported from tw93/Mole V1.70.0 lib/optimize/tasks.sh opt_cache_refresh
// Mole also deletes the three cache folders; orbit only asks the system to refresh them.
func runCacheRefresh(s *Scanner, ctx context.Context) (OptimizeReceipt, optimizeUndoRecord) {
	r := OptimizeReceipt{}
	_, e1 := s.runCmd(ctx, "", "/usr/bin/qlmanage", "-r", "cache")
	_, e2 := s.runCmd(ctx, "", "/usr/bin/qlmanage", "-r")
	if e1 == nil {
		r.Details = append(r.Details, "Quick Look 缩略图缓存已刷新")
	}
	if e2 == nil {
		r.Details = append(r.Details, "图标服务缓存已重建")
	}
	switch {
	case e1 == nil && e2 == nil:
		r.OK, r.Outcome, r.Summary = true, OutcomeApplied, "缩略图与图标缓存已刷新"
	case e1 == nil || e2 == nil:
		r.OK, r.Outcome, r.Summary = true, OutcomeAttention, "只刷新了一部分缓存"
	default:
		r.Outcome, r.Summary, r.Error = OutcomeFailed, "缓存没有刷新", "系统命令没有成功返回"
	}
	return r, optimizeUndoRecord{}
}

// ---- saved_state_cleanup ----

func (s *Scanner) savedStates() ([]string, error) {
	dir := filepath.Join(s.home, "Library", "Saved Application State")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	cutoff := time.Now().AddDate(0, 0, -savedStateAgeDays)
	var out []string
	for _, e := range entries {
		if !e.IsDir() || !strings.HasSuffix(e.Name(), ".savedState") {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) && !s.whitelisted(filepath.Join(dir, e.Name())) {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out, nil
}

func diagSavedState(s *Scanner, _ context.Context) optimizeDiag {
	paths, err := s.savedStates()
	switch {
	case err != nil:
		return unavailDiag("无法读取应用窗口状态目录，可能需要完全磁盘访问权限")
	case len(paths) == 0:
		return clearDiag("没有超过 30 天的应用窗口状态")
	}
	return optimizeDiag{status: OptimizeNeeded, tier: "safe", items: s.shortPaths(paths),
		reason: fmt.Sprintf("有 %d 个超过 30 天的应用窗口状态，移入废纸篓后应用会按需重新生成", len(paths))}
}

// Ported from tw93/Mole V1.70.0 lib/optimize/tasks.sh opt_saved_state_cleanup
func runSavedState(s *Scanner, _ context.Context) (OptimizeReceipt, optimizeUndoRecord) {
	paths, err := s.savedStates()
	if err != nil {
		return OptimizeReceipt{Outcome: OutcomeFailed, Summary: "没有处理", Error: "无法读取应用窗口状态目录"}, optimizeUndoRecord{}
	}
	if len(paths) == 0 {
		return OptimizeReceipt{OK: true, Outcome: OutcomeUnchanged, Summary: "没有超过 30 天的应用窗口状态"}, optimizeUndoRecord{}
	}
	return s.trashStep(paths)
}

// ---- fix_broken_configs ----

// brokenPreferences lints third-party preference plists and returns the corrupt ones.
// Ported from tw93/Mole V1.70.0 lib/optimize/maintenance.sh fix_broken_preferences
func (s *Scanner) brokenPreferences(ctx context.Context) (broken []string, partial bool, err error) {
	prefs := filepath.Join(s.home, "Library", "Preferences")
	entries, err := os.ReadDir(prefs)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var files []string
	for _, e := range entries {
		name := e.Name()
		if !e.Type().IsRegular() || !strings.HasSuffix(name, ".plist") || preferenceProtected(name, true) {
			continue
		}
		files = append(files, filepath.Join(prefs, name))
	}
	_ = filepath.WalkDir(filepath.Join(prefs, "ByHost"), func(path string, d fs.DirEntry, werr error) error {
		if werr == nil && d.Type().IsRegular() && strings.HasSuffix(d.Name(), ".plist") && !preferenceProtected(d.Name(), false) {
			files = append(files, path)
		}
		return nil
	})
	broken, partial = s.lintPlists(ctx, files)
	return broken, partial, nil
}

// Ported from tw93/Mole V1.70.0 lib/optimize/maintenance.sh _preference_plist_is_protected
func preferenceProtected(name string, protectLoginwindow bool) bool {
	switch {
	case strings.HasPrefix(name, "com.apple.") || strings.HasPrefix(name, ".GlobalPreferences"):
		return true
	case name == "loginwindow.plist":
		return protectLoginwindow
	}
	return false
}

func diagBrokenPrefs(s *Scanner, ctx context.Context) optimizeDiag {
	broken, partial, err := s.brokenPreferences(ctx)
	switch {
	case err != nil:
		return unavailDiag("无法读取偏好设置目录，可能需要完全磁盘访问权限")
	case len(broken) > 0 && partial:
		return optimizeDiag{status: OptimizeNeeded, tier: "safe", items: s.shortPaths(broken),
			reason: fmt.Sprintf("发现 %d 个无法读取的偏好设置文件，扫描达到时间上限，可能还有更多", len(broken))}
	case len(broken) > 0:
		return optimizeDiag{status: OptimizeNeeded, tier: "safe", items: s.shortPaths(broken),
			reason: fmt.Sprintf("发现 %d 个无法读取的偏好设置文件，移入废纸篓后应用会重新生成", len(broken))}
	case partial:
		return clearDiag("扫描达到时间上限，已检查的偏好设置都正常")
	}
	return clearDiag("第三方偏好设置文件都正常")
}

// Ported from tw93/Mole V1.70.0 lib/optimize/tasks.sh opt_fix_broken_configs
func runBrokenPrefs(s *Scanner, ctx context.Context) (OptimizeReceipt, optimizeUndoRecord) {
	broken, _, err := s.brokenPreferences(ctx)
	if err != nil {
		return OptimizeReceipt{Outcome: OutcomeFailed, Summary: "没有处理", Error: "无法读取偏好设置目录"}, optimizeUndoRecord{}
	}
	if len(broken) == 0 {
		return OptimizeReceipt{OK: true, Outcome: OutcomeUnchanged, Summary: "第三方偏好设置文件都正常"}, optimizeUndoRecord{}
	}
	return s.trashStep(broken)
}

// ---- sqlite_vacuum ----

type sqliteScan struct {
	needed, optimal, tooBig, unreadable []string
}

var sqliteSidecar = regexp.MustCompile(`-(wal|shm)$`)

// scanDatabases probes Mail, Messages and Safari databases without writing.
// Ported from tw93/Mole V1.70.0 lib/optimize/tasks.sh opt_sqlite_vacuum (candidate and freelist checks)
func (s *Scanner) scanDatabases(ctx context.Context) sqliteScan {
	var scan sqliteScan
	lib := filepath.Join(s.home, "Library")
	if _, err := os.ReadDir(filepath.Join(lib, "Mail")); err != nil && !os.IsNotExist(err) {
		scan.unreadable = append(scan.unreadable, filepath.Join(lib, "Mail"))
	}
	var candidates []string
	for _, pattern := range []string{"Mail/V*/MailData/Envelope Index*", "Messages/chat.db", "Safari/History.db", "Safari/TopSites.db"} {
		matches, _ := filepath.Glob(filepath.Join(lib, pattern))
		if !strings.ContainsAny(pattern, "*") {
			if _, err := os.Stat(filepath.Join(lib, pattern)); err != nil && !os.IsNotExist(err) {
				scan.unreadable = append(scan.unreadable, filepath.Join(lib, pattern))
			}
		}
		candidates = append(candidates, matches...)
	}
	for _, db := range candidates {
		if sqliteSidecar.MatchString(db) || s.whitelisted(db) {
			continue
		}
		header, err := readHeader(db, 16)
		if err != nil {
			scan.unreadable = append(scan.unreadable, db)
			continue
		}
		if header != "SQLite format 3\x00" {
			continue
		}
		if info, err := os.Stat(db); err != nil || info.Size() > sqliteMaxSize {
			if err == nil {
				scan.tooBig = append(scan.tooBig, db)
			}
			continue
		}
		out, err := s.runCmd(ctx, "", "/usr/bin/sqlite3", "-readonly", db, "PRAGMA page_count; PRAGMA freelist_count;")
		var pages, free int
		if _, serr := fmt.Sscanf(strings.TrimSpace(out), "%d\n%d", &pages, &free); err != nil || serr != nil {
			scan.unreadable = append(scan.unreadable, db)
			continue
		}
		if pages > 0 && free*100 < pages*5 {
			scan.optimal = append(scan.optimal, db)
			continue
		}
		scan.needed = append(scan.needed, db)
	}
	return scan
}

func readHeader(path string, n int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf := make([]byte, n)
	read, err := io.ReadFull(f, buf)
	if err != nil && read == 0 {
		if err == io.EOF {
			return "", nil // an empty file is not a database
		}
		return "", err
	}
	return string(buf[:read]), nil
}

var sqliteApps = []string{"Mail", "Safari", "Messages"}

// busySQLiteApps names the apps that own these databases and are running.
// known is false when the process list could not be read.
func (s *Scanner) busySQLiteApps() (busy []string, known bool) {
	procs := s.runningProcesses()
	if len(procs) == 0 {
		return nil, false
	}
	for _, app := range sqliteApps {
		for _, p := range procs {
			if filepath.Base(p.path) == app {
				busy = append(busy, app)
				break
			}
		}
	}
	return busy, true
}

func diagSQLite(s *Scanner, ctx context.Context) optimizeDiag {
	scan := s.scanDatabases(ctx)
	switch {
	case len(scan.needed) > 0:
		busy, known := s.busySQLiteApps()
		if !known {
			return unavailDiag("无法确认邮件、Safari、信息是否在运行，保持原状")
		}
		if len(busy) > 0 {
			names := strings.Join(busy, ", ")
			return optimizeDiag{status: OptimizeSkipped, tier: "protected", items: s.shortPaths(scan.needed),
				reason: fmt.Sprintf("%s 正在运行，数据库可能被占用，暂不整理", names),
				unlock: fmt.Sprintf("退出 %s 后重新扫描即可整理这些数据库", names)}
		}
		return optimizeDiag{status: OptimizeNeeded, tier: "safe", items: s.shortPaths(scan.needed),
			reason: fmt.Sprintf("有 %d 个数据库的空闲页超过 5%%，压缩后更紧凑", len(scan.needed))}
	case len(scan.unreadable) > 0:
		d := unavailDiag("读取这些数据库需要完全磁盘访问权限，orbit 现在读不了")
		d.unlock = "在系统设置里给 orbit 完全磁盘访问权限后重新扫描即可"
		return d
	case len(scan.optimal) > 0:
		return clearDiag("数据库已经很紧凑，空闲页不到 5%")
	case len(scan.tooBig) > 0:
		return clearDiag("数据库超过 100 MB 的安全上限，不处理")
	}
	return clearDiag("没有找到邮件、Safari 或信息的数据库")
}

// Ported from tw93/Mole V1.70.0 lib/optimize/tasks.sh opt_sqlite_vacuum
func runSQLite(s *Scanner, ctx context.Context) (OptimizeReceipt, optimizeUndoRecord) {
	if busy, known := s.busySQLiteApps(); !known || len(busy) > 0 {
		return OptimizeReceipt{Outcome: OutcomeSkipped, Summary: "没有处理", Error: "邮件、Safari 或信息正在运行，或无法确认它们是否在运行"}, optimizeUndoRecord{}
	}
	var done []string
	failed := 0
	for _, db := range s.scanDatabases(ctx).needed {
		if ctx.Err() != nil {
			failed++
			break
		}
		if out, err := s.runCmd(ctx, "", "/usr/bin/sqlite3", "-readonly", db, "PRAGMA integrity_check;"); err != nil || strings.TrimSpace(out) != "ok" {
			failed++ // never VACUUM a database that does not pass its own check
			continue
		}
		if _, err := s.runCmd(ctx, "", "/usr/bin/sqlite3", db, "VACUUM;"); err != nil {
			failed++
			continue
		}
		done = append(done, db)
	}
	r := OptimizeReceipt{Details: s.shortPaths(done)}
	switch {
	case len(done) > 0 && failed == 0:
		r.OK, r.Outcome, r.Summary = true, OutcomeApplied, fmt.Sprintf("已整理 %d 个数据库", len(done))
	case len(done) > 0:
		r.OK, r.Outcome, r.Summary = true, OutcomeAttention, fmt.Sprintf("已整理 %d 个数据库，另有 %d 个没有处理", len(done), failed)
	case failed > 0:
		r.Outcome, r.Summary, r.Error = OutcomeFailed, "没有整理任何数据库", "数据库没有通过完整性检查，或整理时出错，已保持原状"
	default:
		r.OK, r.Outcome, r.Summary = true, OutcomeUnchanged, "数据库已经很紧凑"
	}
	return r, optimizeUndoRecord{}
}

// ---- preference-writing tasks ----

type prefTarget struct {
	domain, key, label, plist string
}

func (s *Scanner) prefPlist(domain string) string {
	if domain == globalDomain {
		return filepath.Join(s.home, "Library", "Preferences", ".GlobalPreferences.plist")
	}
	return filepath.Join(s.home, "Library", "Preferences", domain+".plist")
}

// ---- prevent_network_dsstore ----

var dsstoreKeys = []string{"DSDontWriteNetworkStores", "DSDontWriteUSBStores"}

// dsstoreMissing lists the keys that are not yet true; err when a read failed.
func (s *Scanner) dsstoreMissing(ctx context.Context) (missing []string, err error) {
	for _, key := range dsstoreKeys {
		frag, _, gerr := s.defaultsGet(ctx, dsstoreDomain, key)
		if gerr != nil {
			return nil, gerr
		}
		if !fragTruthy(frag) {
			missing = append(missing, key)
		}
	}
	return missing, nil
}

func diagDSStore(s *Scanner, ctx context.Context) optimizeDiag {
	missing, err := s.dsstoreMissing(ctx)
	switch {
	case err != nil:
		return unavailDiag("无法读取 Finder 偏好设置，保持原状")
	case len(missing) == 0:
		return clearDiag("已经设置过，Finder 不会在网络共享和 U 盘上生成 .DS_Store")
	}
	return optimizeDiag{status: OptimizeNeeded, tier: "safe", items: missing,
		reason: "Finder 现在还会在网络共享和 U 盘上生成 .DS_Store"}
}

// Ported from tw93/Mole V1.70.0 lib/optimize/tasks.sh opt_prevent_network_dsstore
func runDSStore(s *Scanner, ctx context.Context) (OptimizeReceipt, optimizeUndoRecord) {
	missing, err := s.dsstoreMissing(ctx)
	if err != nil {
		return OptimizeReceipt{Outcome: OutcomeFailed, Summary: "没有改动", Error: "无法读取 Finder 偏好设置"}, optimizeUndoRecord{}
	}
	var undo optimizeUndoRecord
	r := OptimizeReceipt{}
	failed := 0
	for _, key := range missing {
		before, _, _ := s.defaultsGet(ctx, dsstoreDomain, key)
		if _, err := s.runCmd(ctx, "", "/usr/bin/defaults", "write", dsstoreDomain, key, "-bool", "true"); err != nil {
			failed++
			continue
		}
		undo.Defaults = append(undo.Defaults, defaultsChange{Domain: dsstoreDomain, Key: key, Before: before, After: s.afterValue(ctx, dsstoreDomain, key)})
		r.Details = append(r.Details, dsstoreDomain+" "+key)
	}
	return finishPrefReceipt(r, len(undo.Defaults), failed, "已设置 %d 项 Finder 偏好", "已设置 %d 项 Finder 偏好，另有 %d 项没有改成"), undo
}

func finishPrefReceipt(r OptimizeReceipt, changed, failed int, appliedFmt, partialFmt string) OptimizeReceipt {
	switch {
	case changed > 0 && failed == 0:
		r.OK, r.Outcome, r.Summary = true, OutcomeApplied, fmt.Sprintf(appliedFmt, changed)
	case changed > 0:
		r.OK, r.Outcome, r.Summary = true, OutcomeAttention, fmt.Sprintf(partialFmt, changed, failed)
	case failed > 0:
		r.Outcome, r.Summary, r.Error = OutcomeFailed, "没有改动", "系统没有接受这次设置"
	default:
		r.OK, r.Outcome, r.Summary = true, OutcomeUnchanged, "已经是目标状态，没有改动"
	}
	return r
}

// ---- legacy_overrides_audit ----

func (s *Scanner) legacyTargets() []prefTarget {
	targets := []prefTarget{{globalDomain, "NSAppSleepDisabled", "App Nap 被全局关闭（NSAppSleepDisabled）", s.prefPlist(globalDomain)}}
	for _, key := range []string{"skip-verify", "skip-verify-locked", "skip-verify-remote"} {
		targets = append(targets, prefTarget{"com.apple.frameworks.diskimages", key, fmt.Sprintf("磁盘映像校验被跳过（%s）", key), s.prefPlist("com.apple.frameworks.diskimages")})
	}
	return targets
}

// legacyFound returns the override keys that are set and not whitelisted.
// Ported from tw93/Mole V1.70.0 lib/optimize/tasks.sh opt_legacy_overrides_audit
func (s *Scanner) legacyFound(ctx context.Context) (found []prefTarget, err error) {
	for _, t := range s.legacyTargets() {
		frag, _, gerr := s.defaultsGet(ctx, t.domain, t.key)
		if gerr != nil {
			return nil, gerr
		}
		if fragTruthy(frag) && !s.whitelisted(t.plist) {
			found = append(found, t)
		}
	}
	return found, nil
}

func diagLegacyOverrides(s *Scanner, ctx context.Context) optimizeDiag {
	found, err := s.legacyFound(ctx)
	switch {
	case err != nil:
		return unavailDiag("无法读取这些偏好设置，保持原状")
	case len(found) == 0:
		return clearDiag("没有发现 App Nap 或磁盘映像校验的覆盖设置")
	}
	d := optimizeDiag{status: OptimizeNeeded, tier: "safe", reason: fmt.Sprintf("发现 %d 个旧版调优工具留下的覆盖设置，删除后 macOS 恢复默认行为", len(found))}
	for _, t := range found {
		d.items = append(d.items, t.label)
	}
	return d
}

func runLegacyOverrides(s *Scanner, ctx context.Context) (OptimizeReceipt, optimizeUndoRecord) {
	found, err := s.legacyFound(ctx)
	if err != nil {
		return OptimizeReceipt{Outcome: OutcomeFailed, Summary: "没有改动", Error: "无法读取这些偏好设置"}, optimizeUndoRecord{}
	}
	var undo optimizeUndoRecord
	r := OptimizeReceipt{}
	failed := 0
	for _, t := range found {
		before, _, _ := s.defaultsGet(ctx, t.domain, t.key)
		if _, err := s.runCmd(ctx, "", "/usr/bin/defaults", "delete", t.domain, t.key); err != nil {
			failed++
			continue
		}
		undo.Defaults = append(undo.Defaults, defaultsChange{Domain: t.domain, Key: t.key, Before: before, After: s.afterValue(ctx, t.domain, t.key)})
		r.Details = append(r.Details, t.label)
	}
	return finishPrefReceipt(r, len(undo.Defaults), failed, "已删除 %d 个覆盖设置", "已删除 %d 个覆盖设置，另有 %d 项没有改成"), undo
}

// ---- spotlight_orphan_rules_cleanup ----

var reverseDNS = regexp.MustCompile(`^[A-Za-z0-9][-A-Za-z0-9]*(\.[A-Za-z0-9][-A-Za-z0-9]*)+$`)
var lowerFirstLabel = regexp.MustCompile(`^[a-z0-9]+$`)

// parseStringArray reads an XML plist fragment holding an array of strings. Any
// other element makes the rule list unreadable, and nothing is rewritten then.
func parseStringArray(frag string) ([]string, error) {
	dec := xml.NewDecoder(strings.NewReader(frag))
	var out []string
	depth, inString := 0, false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch {
			case depth == 1 && t.Name.Local == "plist":
			case depth == 2 && t.Name.Local == "array":
			case depth == 3 && t.Name.Local == "string":
				inString = true
				out = append(out, "")
			default:
				return nil, fmt.Errorf("unexpected element %s", t.Name.Local)
			}
		case xml.EndElement:
			depth--
			inString = false
		case xml.CharData:
			if inString {
				out[len(out)-1] += string(t)
			}
		}
	}
}

func stringArrayFragment(items []string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><array>`)
	for _, item := range items {
		b.WriteString("<string>")
		_ = xml.EscapeText(&b, []byte(item))
		b.WriteString("</string>")
	}
	b.WriteString("</array></plist>")
	return b.String()
}

// installedBundleIndex is installedAppIndex plus the other roots Mole's resolver walks.
// Ported from tw93/Mole V1.70.0 lib/core/bundle_resolver.sh bundle_has_installed_app (filesystem scan)
func (s *Scanner) installedBundleIndex() map[string]struct{} {
	_, ids := s.installedAppIndex(s.home)
	for _, dir := range []string{"/Applications/Setapp", filepath.Join(s.home, "Library", "Application Support", "Setapp", "Applications"), "/Library/Input Methods", filepath.Join(s.home, "Library", "Input Methods")} {
		for _, path := range s.matchingWildcardPaths(dir, "") {
			if strings.HasSuffix(path, ".app") {
				if id := readBundleInfo(path).identifier; id != "" {
					ids[strings.ToLower(id)] = struct{}{}
				}
			}
		}
	}
	return ids
}

// bundleInstalled is true when the app exists or when that is uncertain.
// Ported from tw93/Mole V1.70.0 lib/core/bundle_resolver.sh bundle_has_installed_app
func (s *Scanner) bundleInstalled(ctx context.Context, id string, index map[string]struct{}) bool {
	if !reverseDNS.MatchString(id) {
		return true
	}
	lower := strings.ToLower(id)
	if _, ok := index[lower]; ok {
		return true
	}
	for _, suffix := range []string{".helper", ".daemon", ".agent", ".xpc", ".service"} {
		if strings.HasSuffix(lower, suffix) {
			if _, ok := index[strings.TrimSuffix(lower, suffix)]; ok {
				return true
			}
		}
	}
	if id == "com.microsoft.autoupdate.helper" || id == "com.microsoft.office.licensingV2.helper" {
		for _, app := range []string{"Word", "Excel", "Powerpoint", "Outlook", "OneNote"} {
			if _, ok := index["com.microsoft."+strings.ToLower(app)]; ok {
				return true
			}
		}
	}
	out, err := s.runCmd(ctx, "", "/usr/bin/mdfind", "kMDItemCFBundleIdentifier == '"+id+"'")
	if ctx.Err() != nil || err != nil || strings.TrimSpace(out) != "" {
		return true
	}
	return false
}

type spotlightState struct {
	snapshot string // the fragment as read; empty when there are no custom rules
	rules    []string
	orphans  []string
}

// spotlightScan finds rule entries that name an app which is gone.
// Ported from tw93/Mole V1.70.0 lib/optimize/tasks.sh opt_prune_spotlight_orphan_rules
func (s *Scanner) spotlightScan(ctx context.Context) (spotlightState, error) {
	frag, present, err := s.defaultsGet(ctx, spotlightDomain, spotlightRulesKey)
	if err != nil {
		return spotlightState{}, err
	}
	if !present || s.whitelisted(s.prefPlist(spotlightDomain)) {
		return spotlightState{}, nil
	}
	rules, err := parseStringArray(frag)
	if err != nil {
		return spotlightState{}, err
	}
	st := spotlightState{snapshot: frag, rules: rules}
	index := s.installedBundleIndex()
	for _, rule := range rules {
		if strings.HasPrefix(rule, "System.") || strings.HasPrefix(rule, "com.apple.") {
			continue
		}
		// Spotlight keeps its own switches here (Custom.relatedContents); only a lowercase first label can name an app.
		if !reverseDNS.MatchString(rule) || !lowerFirstLabel.MatchString(strings.SplitN(rule, ".", 2)[0]) {
			continue
		}
		if !s.bundleInstalled(ctx, rule, index) {
			st.orphans = append(st.orphans, rule)
		}
	}
	return st, nil
}

func diagSpotlightRules(s *Scanner, ctx context.Context) optimizeDiag {
	st, err := s.spotlightScan(ctx)
	switch {
	case err != nil:
		return unavailDiag("无法读取 Spotlight 搜索规则，保持原状")
	case len(st.orphans) == 0:
		return clearDiag("Spotlight 搜索规则里没有已卸载应用的残留")
	}
	return optimizeDiag{status: OptimizeNeeded, tier: "safe", items: st.orphans,
		reason: fmt.Sprintf("有 %d 条规则指向已不在本机的应用", len(st.orphans))}
}

func runSpotlightRules(s *Scanner, ctx context.Context) (OptimizeReceipt, optimizeUndoRecord) {
	st, err := s.spotlightScan(ctx)
	if err != nil {
		return OptimizeReceipt{Outcome: OutcomeFailed, Summary: "没有改动", Error: "无法读取 Spotlight 搜索规则"}, optimizeUndoRecord{}
	}
	if len(st.orphans) == 0 {
		return OptimizeReceipt{OK: true, Outcome: OutcomeUnchanged, Summary: "Spotlight 搜索规则里没有已卸载应用的残留"}, optimizeUndoRecord{}
	}
	// Rewrite through defaults (cfprefsd), and only if nothing changed since the scan.
	if now, _, err := s.defaultsGet(ctx, spotlightDomain, spotlightRulesKey); err != nil || now != st.snapshot {
		return OptimizeReceipt{Outcome: OutcomeFailed, Summary: "没有改动", Error: "搜索规则在检查之后又变了，保持原状"}, optimizeUndoRecord{}
	}
	orphan := map[string]bool{}
	for _, o := range st.orphans {
		orphan[o] = true
	}
	var keep []string
	for _, rule := range st.rules {
		if !orphan[rule] {
			keep = append(keep, rule)
		}
	}
	var werr error
	if len(keep) > 0 {
		_, werr = s.runCmd(ctx, "", "/usr/bin/defaults", "write", spotlightDomain, spotlightRulesKey, stringArrayFragment(keep))
	} else {
		_, werr = s.runCmd(ctx, "", "/usr/bin/defaults", "delete", spotlightDomain, spotlightRulesKey)
	}
	if werr != nil {
		return OptimizeReceipt{Outcome: OutcomeFailed, Summary: "没有改动", Error: "系统没有接受这次设置"}, optimizeUndoRecord{}
	}
	undo := optimizeUndoRecord{Defaults: []defaultsChange{{Domain: spotlightDomain, Key: spotlightRulesKey, Before: st.snapshot, After: s.afterValue(ctx, spotlightDomain, spotlightRulesKey)}}}
	return OptimizeReceipt{OK: true, Outcome: OutcomeApplied, Summary: fmt.Sprintf("已删除 %d 条 Spotlight 规则", len(st.orphans)), Details: capList(st.orphans, receiptDetailCap)}, undo
}

// ---- shared_file_list_repair ----

// brokenSharedLists finds unreadable .sfl2/.sfl3/.sfl4 files, never touching recent documents.
// Ported from tw93/Mole V1.70.0 lib/optimize/tasks.sh opt_shared_file_list_repair
func (s *Scanner) brokenSharedLists(ctx context.Context) (broken []string, partial bool, err error) {
	dir := filepath.Join(s.home, "Library", "Application Support", "com.apple.sharedfilelist")
	if _, err := os.ReadDir(dir); err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var files []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		if d.IsDir() && d.Name() == "ApplicationRecentDocuments" {
			return filepath.SkipDir
		}
		switch filepath.Ext(path) {
		case ".sfl2", ".sfl3", ".sfl4":
			if d.Type().IsRegular() {
				files = append(files, path)
			}
		}
		return nil
	})
	broken, partial = s.lintPlists(ctx, files)
	return broken, partial, nil
}

func diagSharedFileLists(s *Scanner, ctx context.Context) optimizeDiag {
	broken, _, err := s.brokenSharedLists(ctx)
	switch {
	case err != nil:
		return unavailDiag("无法读取共享文件列表目录，可能需要完全磁盘访问权限")
	case len(broken) == 0:
		return clearDiag("访达收藏与最近项目的列表都正常")
	}
	return optimizeDiag{status: OptimizeNeeded, tier: "safe", items: s.shortPaths(broken),
		reason: fmt.Sprintf("发现 %d 个无法读取的共享文件列表，移入废纸篓后系统会重新生成", len(broken))}
}

func runSharedFileLists(s *Scanner, ctx context.Context) (OptimizeReceipt, optimizeUndoRecord) {
	broken, _, err := s.brokenSharedLists(ctx)
	if err != nil {
		return OptimizeReceipt{Outcome: OutcomeFailed, Summary: "没有处理", Error: "无法读取共享文件列表目录"}, optimizeUndoRecord{}
	}
	if len(broken) == 0 {
		return OptimizeReceipt{OK: true, Outcome: OutcomeUnchanged, Summary: "访达收藏与最近项目的列表都正常"}, optimizeUndoRecord{}
	}
	return s.trashStep(broken)
}

// ---- launch_agents_cleanup ----

// brokenLaunchAgents lists agents whose absolute program is missing, as "label → path".
// Ported from tw93/Mole V1.70.0 lib/optimize/tasks.sh opt_launch_agents_cleanup
func (s *Scanner) brokenLaunchAgents(ctx context.Context) ([]string, error) {
	dir := filepath.Join(s.home, "Library", "LaunchAgents")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var broken []string
	for _, e := range entries {
		if ctx.Err() != nil {
			break
		}
		if !strings.HasSuffix(e.Name(), ".plist") || !e.Type().IsRegular() {
			continue
		}
		out, err := s.runCmd(ctx, "", "/usr/bin/plutil", "-convert", "json", "-o", "-", filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var agent struct {
			Label            string
			Program          string
			ProgramArguments []string
		}
		if json.Unmarshal([]byte(out), &agent) != nil {
			continue
		}
		program := agent.Program
		if program == "" && len(agent.ProgramArguments) > 0 {
			program = agent.ProgramArguments[0]
		}
		// Bare names resolve through PATH and an unplugged /Volumes disk is not a broken agent.
		if !filepath.IsAbs(program) || !launchVolumeMounted(program) {
			continue
		}
		if _, err := os.Lstat(program); os.IsNotExist(err) {
			label := agent.Label
			if label == "" {
				label = strings.TrimSuffix(e.Name(), ".plist")
			}
			broken = append(broken, label+" → "+s.displayHome(program))
		}
	}
	return broken, nil
}

// Ported from tw93/Mole V1.70.0 lib/optimize/tasks.sh launch_agent_volume_mounted
func launchVolumeMounted(path string) bool {
	rest, ok := strings.CutPrefix(path, "/Volumes/")
	if !ok {
		return true
	}
	vol, _, _ := strings.Cut(rest, "/")
	if vol == "" {
		return false
	}
	_, err := os.Stat("/Volumes/" + vol)
	return err == nil
}

func diagLaunchAgents(s *Scanner, ctx context.Context) optimizeDiag {
	broken, err := s.brokenLaunchAgents(ctx)
	switch {
	case err != nil:
		return unavailDiag("无法读取启动代理目录，可能需要完全磁盘访问权限")
	case len(broken) == 0:
		return clearDiag("启动代理指向的程序都还在")
	}
	return optimizeDiag{status: OptimizeNeeded, tier: "safe", items: broken,
		reason: fmt.Sprintf("有 %d 个启动代理指向已不存在的程序，只列出，不会改动", len(broken))}
}

func runLaunchAgents(s *Scanner, ctx context.Context) (OptimizeReceipt, optimizeUndoRecord) {
	broken, err := s.brokenLaunchAgents(ctx)
	if err != nil {
		return OptimizeReceipt{Outcome: OutcomeFailed, Summary: "没有检查", Error: "无法读取启动代理目录"}, optimizeUndoRecord{}
	}
	if len(broken) == 0 {
		return OptimizeReceipt{OK: true, Outcome: OutcomeUnchanged, Summary: "启动代理指向的程序都还在"}, optimizeUndoRecord{}
	}
	return OptimizeReceipt{OK: true, Outcome: OutcomeAttention, Details: capList(broken, receiptDetailCap),
		Summary: fmt.Sprintf("有 %d 个启动代理指向已不存在的程序，已列出，没有改动", len(broken))}, optimizeUndoRecord{}
}

// ---- login_items_audit ----

const loginItemsScript = `set tabChar to ASCII character 9
set outputLines to {}
tell application "System Events"
	repeat with loginItem in login items
		set itemName to ""
		set itemPath to ""
		try
			set itemName to name of loginItem as text
		end try
		try
			set itemPath to POSIX path of (path of loginItem as alias)
		on error
			try
				set itemPath to path of loginItem as text
			end try
		end try
		set end of outputLines to itemName & tabChar & itemPath
	end repeat
end tell
set AppleScript's text item delimiters to (ASCII character 10)
return outputLines as text`

func diagLoginItems(*Scanner, context.Context) optimizeDiag {
	// Listing login items asks System Events for permission, so the diagnosis does not run it.
	return optimizeDiag{status: OptimizeNeeded, tier: "review",
		reason: "核对登录项指向的应用是否还在；首次检查时 macOS 可能询问是否允许 orbit 控制“系统事件”"}
}

// loginItemExists is a simplified form of Mole's _login_item_app_exists: the item's own path, then the standard app folders.
func (s *Scanner) loginItemExists(name, path string) bool {
	if path != "" {
		if _, err := os.Lstat(path); err == nil {
			return true
		}
	}
	app := strings.TrimSuffix(name, ".app") + ".app"
	for _, root := range []string{"/Applications", "/Applications/Utilities", "/System/Applications", "/System/Applications/Utilities", "/Applications/Setapp",
		filepath.Join(s.home, "Applications"), filepath.Join(s.home, "Library", "Application Support", "Setapp", "Applications")} {
		if _, err := os.Stat(filepath.Join(root, app)); err == nil {
			return true
		}
	}
	return false
}

// Ported from tw93/Mole V1.70.0 lib/optimize/tasks.sh opt_login_items_audit
func runLoginItems(s *Scanner, ctx context.Context) (OptimizeReceipt, optimizeUndoRecord) {
	out, err := s.runCmd(ctx, loginItemsScript, "/usr/bin/osascript")
	if err != nil {
		return OptimizeReceipt{Outcome: OutcomeFailed, Summary: "没有检查", Error: "没能读取登录项；请在系统设置的隐私与安全性里允许 orbit 控制“系统事件”后重试"}, optimizeUndoRecord{}
	}
	checked := 0
	var broken []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		name, path, _ := strings.Cut(line, "\t")
		if strings.TrimSpace(name) == "" {
			continue
		}
		checked++
		if !s.loginItemExists(strings.TrimSpace(name), strings.TrimSpace(path)) {
			broken = append(broken, strings.TrimSpace(name))
		}
	}
	switch {
	case len(broken) > 0:
		return OptimizeReceipt{OK: true, Outcome: OutcomeAttention, Details: capList(broken, receiptDetailCap),
			Summary: fmt.Sprintf("有 %d 个登录项指向找不到的应用，请在系统设置的登录项里移除", len(broken))}, optimizeUndoRecord{}
	case checked == 0:
		return OptimizeReceipt{OK: true, Outcome: OutcomeUnchanged, Summary: "没有登录项"}, optimizeUndoRecord{}
	}
	return OptimizeReceipt{OK: true, Outcome: OutcomeUnchanged, Summary: fmt.Sprintf("检查了 %d 个登录项，都正常", checked)}, optimizeUndoRecord{}
}
