package scanner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// The 系统加速 catalog is a port of Mole's `mo optimize` task list. It runs in
// two phases, like Mole: Diagnose reads the machine and says per task whether
// it is needed, and RunOptimize executes only the ids the page selected, each
// with its own timeout and a receipt. Nothing here asks for administrator
// rights: tasks Mole runs with sudo are listed as unavailable and never run.
// File removals go through TrashPaths; settings changes record the previous
// value so UndoOptimize can put it back.

// Diagnosis states.
const (
	OptimizeNeeded      = "needed"      // the machine would change
	OptimizeClear       = "clear"       // nothing to do, with the reason
	OptimizeSkipped     = "skipped"     // a precondition fails right now (an app is running)
	OptimizeUnavailable = "unavailable" // orbit cannot or will not do this
)

// Receipt outcomes, after Mole's lib/optimize/outcomes.sh.
const (
	OutcomeApplied   = "applied"
	OutcomeUnchanged = "unchanged"
	OutcomeAttention = "attention"
	OutcomeSkipped   = "skipped"
	OutcomeFailed    = "failed"
)

const (
	optimizeSudoReason = "需要管理员权限，orbit 不执行"
	optimizeDiagLimit  = 25 * time.Second
	optimizeListCap    = 8
)

// OptimizeRunner runs one external command. stdin may be empty. The error
// carries the command's stderr so callers can tell "no such key" from a failure.
type OptimizeRunner func(ctx context.Context, stdin, name string, args ...string) (string, error)

func defaultOptimizeRunner(ctx context.Context, stdin, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return stdout.String(), ctx.Err()
		}
		return stdout.String(), fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func (s *Scanner) runCmd(ctx context.Context, stdin, name string, args ...string) (string, error) {
	if s.optimizeRun != nil {
		return s.optimizeRun(ctx, stdin, name, args...)
	}
	return defaultOptimizeRunner(ctx, stdin, name, args...)
}

// OptimizeItem is one row of the plan the page renders.
type OptimizeItem struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Detail   string   `json:"detail"` // what the task does
	Status   string   `json:"status"`
	Reason   string   `json:"reason"` // why it is needed, not needed, skipped or unavailable
	Tier     string   `json:"tier"`   // safe | review | protected
	Unlock   string   `json:"unlock,omitempty"`
	Undo     string   `json:"undo"` // how to undo, in one line
	Checked  bool     `json:"checked"`
	Runnable bool     `json:"runnable"`
	Items    []string `json:"items,omitempty"` // what would be touched, capped
}

// OptimizePlan is the diagnosis the page shows before anything runs.
type OptimizePlan struct {
	Items []OptimizeItem `json:"items"`
}

// OptimizeReceipt is what one executed task reports back.
type OptimizeReceipt struct {
	ID      string         `json:"id"`
	OK      bool           `json:"ok"`
	Outcome string         `json:"outcome"`
	Summary string         `json:"summary"`
	Details []string       `json:"details,omitempty"`
	Failed  []TrashFailure `json:"failed,omitempty"`
	Undo    string         `json:"undo"`
	CanUndo bool           `json:"canUndo"`
	Error   string         `json:"error,omitempty"`
}

type optimizeDiag struct {
	status, reason, tier, unlock string
	items                        []string
}

type optimizeTask struct {
	id, name, detail, undo string
	timeout                time.Duration
	diagnose               func(s *Scanner, ctx context.Context) optimizeDiag
	run                    func(s *Scanner, ctx context.Context) (OptimizeReceipt, optimizeUndoRecord) // nil: never runs
}

func unavailableTask(id, name, detail, reason string) optimizeTask {
	return optimizeTask{id: id, name: name, detail: detail, undo: "不执行，没有改动",
		diagnose: func(*Scanner, context.Context) optimizeDiag {
			return optimizeDiag{status: OptimizeUnavailable, reason: reason, tier: "protected"}
		}}
}

// optimizeCatalog lists every task of Mole's catalog.sh in its order.
var optimizeCatalog = []optimizeTask{
	{id: "cache_refresh", name: "刷新缩略图与图标缓存", detail: "重建 Quick Look 缩略图与图标服务缓存",
		undo: "无需撤销，缓存会按需重建", timeout: 40 * time.Second, diagnose: diagCacheRefresh, run: runCacheRefresh},
	{id: "saved_state_cleanup", name: "清理旧的应用窗口状态", detail: "把 30 天以上没动过的应用窗口状态移入废纸篓",
		undo: "在这里点撤销，或在访达的废纸篓里放回原处", timeout: 60 * time.Second, diagnose: diagSavedState, run: runSavedState},
	{id: "fix_broken_configs", name: "修复损坏的偏好设置", detail: "把无法读取的第三方偏好设置文件移入废纸篓，应用下次启动会重新生成",
		undo: "在这里点撤销，或在访达的废纸篓里放回原处", timeout: 60 * time.Second, diagnose: diagBrokenPrefs, run: runBrokenPrefs},
	unavailableTask("network_optimization", "刷新 DNS 缓存", "清空系统 DNS 缓存", optimizeSudoReason),
	{id: "sqlite_vacuum", name: "整理邮件、Safari 与信息的数据库", detail: "先做完整性检查，再压缩数据库文件，不删除内容",
		undo: "无需撤销，数据库内容不变", timeout: 180 * time.Second, diagnose: diagSQLite, run: runSQLite},
	{id: "prevent_network_dsstore", name: "禁止在网络与 U 盘上写 .DS_Store", detail: "设置 Finder 偏好，不再在网络共享和 U 盘上生成 .DS_Store",
		undo: "在这里点撤销，会恢复成设置前的值", timeout: 20 * time.Second, diagnose: diagDSStore, run: runDSStore},
	{id: "legacy_overrides_audit", name: "移除旧版调优工具的覆盖设置", detail: "删除 App Nap 与磁盘映像校验的隐藏覆盖项，让 macOS 恢复默认行为",
		undo: "在这里点撤销，会恢复成删除前的值", timeout: 20 * time.Second, diagnose: diagLegacyOverrides, run: runLegacyOverrides},
	unavailableTask("network_stack_optimize", "刷新路由表与 ARP 缓存", "清空路由表与 ARP 缓存来排查网络问题", optimizeSudoReason),
	unavailableTask("disk_permissions_repair", "修复用户目录权限", "修复家目录的权限问题", optimizeSudoReason),
	unavailableTask("spotlight_index_optimize", "重建 Spotlight 索引", "搜索变慢时重建索引", optimizeSudoReason),
	{id: "spotlight_orphan_rules_cleanup", name: "清理 Spotlight 里已卸载应用的规则", detail: "删除指向已不在本机的应用的 Spotlight 搜索规则",
		undo: "在这里点撤销，会恢复成清理前的规则列表", timeout: 40 * time.Second, diagnose: diagSpotlightRules, run: runSpotlightRules},
	unavailableTask("periodic_maintenance", "运行系统定期维护脚本", "运行 macOS 每日、每周、每月维护脚本", optimizeSudoReason),
	{id: "shared_file_list_repair", name: "修复损坏的访达收藏与最近项目", detail: "把无法读取的共享文件列表移入废纸篓，系统会重新生成",
		undo: "在这里点撤销，或在访达的废纸篓里放回原处", timeout: 40 * time.Second, diagnose: diagSharedFileLists, run: runSharedFileLists},
	unavailableTask("disk_verify", "校验磁盘文件系统", "检查文件系统完整性", "Mole 默认也关闭这一项：整卷校验可能让系统长时间卡住，orbit 不执行"),
	{id: "login_items_audit", name: "检查失效的登录项", detail: "核对登录项指向的应用是否还在，只报告，不修改",
		undo: "不改动任何东西，无需撤销", timeout: 30 * time.Second, diagnose: diagLoginItems, run: runLoginItems},
	unavailableTask("quarantine_cleanup", "清理下载隔离记录", "清除 Gatekeeper 的下载追踪历史", "会直接删除数据库里的记录，无法放进废纸篓还原，orbit 不执行"),
	{id: "launch_agents_cleanup", name: "检查失效的启动代理", detail: "找出程序已不存在的 LaunchAgents，只报告，不修改",
		undo: "不改动任何东西，无需撤销", timeout: 30 * time.Second, diagnose: diagLaunchAgents, run: runLaunchAgents},
	unavailableTask("notification_cleanup", "清理旧通知", "清理通知数据库里的旧记录", "会直接删除数据库里的记录，无法放进废纸篓还原，orbit 不执行"),
	unavailableTask("coreduet_cleanup", "清理使用记录", "清理系统的使用情况追踪数据", "会直接删除数据库里的记录，无法放进废纸篓还原，orbit 不执行"),
}

func optimizeTaskByID(id string) *optimizeTask {
	for i := range optimizeCatalog {
		if optimizeCatalog[i].id == id {
			return &optimizeCatalog[i]
		}
	}
	return nil
}

// DiagnoseOptimize reads the machine and reports every task. It never changes
// anything. Tasks are diagnosed concurrently, each with its own time limit.
func (s *Scanner) DiagnoseOptimize(ctx context.Context) OptimizePlan {
	plan := OptimizePlan{Items: make([]OptimizeItem, len(optimizeCatalog))}
	var wg sync.WaitGroup
	for i := range optimizeCatalog {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			plan.Items[i] = s.diagnoseTask(ctx, &optimizeCatalog[i])
		}(i)
	}
	wg.Wait()
	return plan
}

func (s *Scanner) diagnoseTask(ctx context.Context, task *optimizeTask) (item OptimizeItem) {
	item = OptimizeItem{ID: task.id, Name: task.name, Detail: task.detail, Undo: task.undo}
	defer func() {
		if r := recover(); r != nil {
			item.Status, item.Reason, item.Tier = OptimizeUnavailable, "检测时出错，保持原状", "protected"
		}
		item.Runnable = item.Status == OptimizeNeeded && task.run != nil
		item.Checked = item.Runnable && item.Tier == "safe"
	}()
	dctx, cancel := context.WithTimeout(ctx, optimizeDiagLimit)
	defer cancel()
	d := task.diagnose(s, dctx)
	if dctx.Err() != nil && d.status != OptimizeNeeded {
		d = optimizeDiag{status: OptimizeUnavailable, reason: "检测超时，保持原状", tier: "protected"}
	}
	if d.tier == "" {
		d.tier = "safe"
	}
	if d.reason == "" { // review and protected rows must always explain themselves
		d.reason = "没有更多说明"
	}
	item.Status, item.Reason, item.Tier, item.Unlock = d.status, d.reason, d.tier, d.unlock
	if len(d.items) > optimizeListCap {
		d.items = d.items[:optimizeListCap]
	}
	item.Items = d.items
	return item
}

var optimizeBusy sync.Mutex

// ErrOptimizeBusy means another run or undo has not finished.
var ErrOptimizeBusy = errors.New("已有系统加速任务正在执行，请等它结束")

// RunOptimize executes the selected ids one after another and calls emit with
// each receipt as it completes. Each task is diagnosed again right before it
// runs, so a stale page cannot make orbit act on an outdated plan.
func (s *Scanner) RunOptimize(ctx context.Context, ids []string, emit func(OptimizeReceipt)) error {
	if !optimizeBusy.TryLock() {
		return ErrOptimizeBusy
	}
	defer optimizeBusy.Unlock()
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		emit(s.runOptimizeTask(ctx, id))
	}
	return nil
}

func (s *Scanner) runOptimizeTask(ctx context.Context, id string) (receipt OptimizeReceipt) {
	task := optimizeTaskByID(id)
	if task == nil {
		return OptimizeReceipt{ID: id, Outcome: OutcomeFailed, Summary: "未执行", Error: "未知的维护项目", Undo: "不执行，没有改动"}
	}
	receipt = OptimizeReceipt{ID: id, Undo: task.undo}
	defer func() {
		if r := recover(); r != nil {
			receipt = OptimizeReceipt{ID: id, Outcome: OutcomeFailed, Summary: "执行出错", Error: "执行时出错，已停止这一项", Undo: task.undo}
		}
	}()
	if task.run == nil {
		d := s.diagnoseTask(ctx, task)
		receipt.Outcome, receipt.Summary, receipt.Error = OutcomeSkipped, "未执行", d.Reason
		return receipt
	}
	if d := s.diagnoseTask(ctx, task); !d.Runnable {
		if d.Status == OptimizeClear {
			receipt.OK, receipt.Outcome, receipt.Summary = true, OutcomeUnchanged, d.Reason
		} else {
			receipt.Outcome, receipt.Summary, receipt.Error = OutcomeSkipped, "未执行", d.Reason
		}
		return receipt
	}
	rctx, cancel := context.WithTimeout(ctx, task.timeout)
	defer cancel()
	out, undo := task.run(s, rctx)
	out.ID, out.Undo = id, task.undo
	if rctx.Err() == context.DeadlineExceeded && out.Outcome != OutcomeFailed {
		out.Outcome, out.OK, out.Error = OutcomeFailed, false, "超过时限，已停止；已完成的部分保持不变"
	}
	if len(undo.Trashed) > 0 || len(undo.Defaults) > 0 {
		if err := saveOptimizeUndo(id, undo); err != nil {
			out.Details = append(out.Details, "未能保存撤销记录："+err.Error())
		} else {
			out.CanUndo = true
		}
	}
	return out
}

// ---- undo records ----

type optimizeUndoRecord struct {
	Trashed  []TrashEntry     `json:"trashed,omitempty"`
	Defaults []defaultsChange `json:"defaults,omitempty"`
}

// defaultsChange is one preference key orbit wrote: the value before (an XML
// plist fragment, empty when the key was absent) and the value it left behind.
type defaultsChange struct {
	Domain string `json:"domain"`
	Key    string `json:"key"`
	Before string `json:"before"`
	After  string `json:"after"`
}

type optimizeStore struct {
	Version int                           `json:"version"`
	Records map[string]optimizeUndoRecord `json:"records"`
}

var optimizeStoreMu sync.Mutex

func optimizeStorePath() string { return filepath.Join(StateDir(), "optimize-undo.json") }

func readOptimizeStore() (optimizeStore, error) {
	store := optimizeStore{Version: 1, Records: map[string]optimizeUndoRecord{}}
	data, err := os.ReadFile(optimizeStorePath())
	if os.IsNotExist(err) {
		return store, nil
	}
	if err != nil {
		return store, err
	}
	if err := json.Unmarshal(data, &store); err != nil {
		return store, fmt.Errorf("撤销记录损坏：%w", err)
	}
	if store.Records == nil {
		store.Records = map[string]optimizeUndoRecord{}
	}
	return store, nil
}

func writeOptimizeStore(store optimizeStore) error {
	if err := os.MkdirAll(StateDir(), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(store)
	if err != nil {
		return err
	}
	tmp := optimizeStorePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, optimizeStorePath())
}

// saveOptimizeUndo appends to the task's record, so a second run does not make
// the first run's changes un-undoable.
func saveOptimizeUndo(id string, add optimizeUndoRecord) error {
	optimizeStoreMu.Lock()
	defer optimizeStoreMu.Unlock()
	store, err := readOptimizeStore()
	if err != nil {
		return err
	}
	rec := store.Records[id]
	rec.Trashed = append(rec.Trashed, add.Trashed...)
	rec.Defaults = append(rec.Defaults, add.Defaults...)
	store.Records[id] = rec
	return writeOptimizeStore(store)
}

// UndoOptimize reverses what the last runs of one task changed. Restores go
// through the Trash manifest, and a preference is only put back if it still
// holds the value orbit left, so a change made since is never overwritten.
func (s *Scanner) UndoOptimize(ctx context.Context, id string) (receipt OptimizeReceipt, err error) {
	if !optimizeBusy.TryLock() {
		return receipt, ErrOptimizeBusy
	}
	defer optimizeBusy.Unlock()
	optimizeStoreMu.Lock()
	defer optimizeStoreMu.Unlock()
	task := optimizeTaskByID(id)
	if task == nil {
		return receipt, errors.New("未知的维护项目")
	}
	store, err := readOptimizeStore()
	if err != nil {
		return receipt, err
	}
	rec, ok := store.Records[id]
	if !ok {
		return receipt, errors.New("没有可撤销的记录")
	}
	receipt = OptimizeReceipt{ID: id, Undo: task.undo}
	var left optimizeUndoRecord
	restored, reverted := 0, 0
	if len(rec.Trashed) > 0 {
		originals := make([]string, 0, len(rec.Trashed))
		trashed := map[string]string{}
		for _, e := range rec.Trashed {
			originals = append(originals, e.Original)
			trashed[e.Original] = e.Trashed
		}
		out := s.RestoreTrash(originals, trashed)
		restored = len(out.Restored)
		done := map[string]bool{}
		for _, p := range out.Restored {
			done[p] = true
			receipt.Details = append(receipt.Details, p)
		}
		for _, e := range rec.Trashed {
			if !done[e.Original] {
				left.Trashed = append(left.Trashed, e)
			}
		}
		receipt.Failed = out.Failed
	}
	for i := len(rec.Defaults) - 1; i >= 0; i-- {
		ch := rec.Defaults[i]
		if reason := s.revertDefaults(ctx, ch); reason != "" {
			left.Defaults = append([]defaultsChange{ch}, left.Defaults...)
			receipt.Failed = append(receipt.Failed, TrashFailure{Path: ch.Domain + " " + ch.Key, Error: reason})
			continue
		}
		reverted++
		receipt.Details = append(receipt.Details, ch.Domain+" "+ch.Key)
	}
	if len(left.Trashed)+len(left.Defaults) == 0 {
		delete(store.Records, id)
	} else {
		store.Records[id] = left
	}
	if werr := writeOptimizeStore(store); werr != nil {
		receipt.Details = append(receipt.Details, "未能更新撤销记录："+werr.Error())
	}
	receipt.CanUndo = len(left.Trashed)+len(left.Defaults) > 0
	switch {
	case len(receipt.Failed) == 0:
		receipt.OK, receipt.Outcome = true, OutcomeApplied
		receipt.Summary = fmt.Sprintf("已撤销 %d 项改动", restored+reverted)
	case restored+reverted > 0:
		receipt.OK, receipt.Outcome = true, OutcomeAttention
		receipt.Summary = fmt.Sprintf("已撤销 %d 项，其余未能撤销", restored+reverted)
	default:
		receipt.Outcome, receipt.Summary = OutcomeFailed, "没有撤销任何改动"
		receipt.Error = "改动未能撤销，保持原状"
	}
	return receipt, nil
}

// revertDefaults puts one key back, or returns why it did not.
func (s *Scanner) revertDefaults(ctx context.Context, ch defaultsChange) string {
	cur, present, err := s.defaultsGet(ctx, ch.Domain, ch.Key)
	if err != nil {
		return "无法读取当前设置，保持原状"
	}
	if (ch.After == "" && present) || (ch.After != "" && cur != ch.After) {
		return "设置在这之后又被改过，为免覆盖保持原状"
	}
	if ch.Before == "" {
		if !present {
			return ""
		}
		if _, err := s.runCmd(ctx, "", "/usr/bin/defaults", "delete", ch.Domain, ch.Key); err != nil {
			return "恢复设置失败"
		}
		return ""
	}
	if _, err := s.runCmd(ctx, "", "/usr/bin/defaults", "write", ch.Domain, ch.Key, ch.Before); err != nil {
		return "恢复设置失败"
	}
	return ""
}

// ---- preference helpers ----

// defaultsGet returns one key as an XML plist fragment. present is false when
// the key does not exist. Ported from tw93/Mole V1.70.0 lib/optimize/tasks.sh opt_prevent_network_dsstore (defaults read), through plutil so the type survives a restore.
func (s *Scanner) defaultsGet(ctx context.Context, domain, key string) (frag string, present bool, err error) {
	doc, err := s.runCmd(ctx, "", "/usr/bin/defaults", "export", domain, "-")
	if err != nil {
		return "", false, err
	}
	frag, err = s.runCmd(ctx, doc, "/usr/bin/plutil", "-extract", key, "xml1", "-o", "-", "-")
	if err != nil {
		if strings.Contains(err.Error(), "No value at that key path") {
			return "", false, nil
		}
		return "", false, err
	}
	return frag, true, nil
}

var plistInner = regexp.MustCompile(`(?s)<plist[^>]*>\s*(.*?)\s*</plist>`)

// fragTruthy is Mole's _opt_defaults_is_truthy over a plist fragment.
func fragTruthy(frag string) bool {
	m := plistInner.FindStringSubmatch(frag)
	if m == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(m[1])) {
	case "<true/>", "<integer>1</integer>", "<string>1</string>", "<string>true</string>", "<string>yes</string>":
		return true
	}
	return false
}

// recordDefaultsWrite reads the value back after a write so the undo knows what
// orbit left behind.
func (s *Scanner) afterValue(ctx context.Context, domain, key string) string {
	after, _, _ := s.defaultsGet(ctx, domain, key)
	return after
}

// whitelisted reports whether the user's whitelist protects a path.
func (s *Scanner) whitelisted(path string) bool {
	s.refreshWhitelist()
	return s.whitelist().Protects(path)
}

// displayHome shortens a path for the page.
func (s *Scanner) displayHome(path string) string {
	if s.home != "" && withinPath(path, s.home) {
		return "~" + strings.TrimPrefix(path, s.home)
	}
	return path
}
