package scanner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// App slimming: copy a Developer ID signed app, drop the Intel code from the
// copy, verify the copy, then swap it in and move the original to the Trash.
// Nothing is changed in place and nothing is ever deleted outright.

// slimTools are the system probes behind slimming. Tests replace them so a
// fixture bundle can be slimmed without a Developer ID certificate.
type slimTools struct {
	roots         []string                                    // nil: /Applications and ~/Applications
	appleSilicon  func() bool                                 // is this Mac an Apple Silicon Mac
	team          func(app string) string                     // Developer ID team, "" when not Developer ID signed
	verify        func(app string) (detritus bool, err error) // codesign --verify --deep --strict
	gatekeeper    func(app string) (code int)                 // spctl --assess exit code, 3 is an explicit rejection
	rosetta       func() map[string]bool                      // bundle ids set to open using Rosetta
	copyApp       func(src, dst string) error                 // clone of the bundle, keeping its metadata
	stripDetritus func(app string)                            // remove Finder xattrs that fail --strict
}

var realSlimTools = slimTools{}

func (s *Scanner) slimToolset() slimTools {
	t := realSlimTools
	if s.slimProbe != nil {
		t = *s.slimProbe
	}
	if t.appleSilicon == nil {
		// Ported from tw93/Mole V1.70.0 bin/slim.sh slim_host_is_apple_silicon
		// uname -m says x86_64 inside a Rosetta shell; the sysctl does not.
		t.appleSilicon = func() bool {
			out, code := slimRun(5*time.Second, "/usr/sbin/sysctl", "-n", "hw.optional.arm64")
			return code == 0 && strings.TrimSpace(out) == "1"
		}
	}
	if t.team == nil {
		t.team = developerIDTeam
	}
	if t.verify == nil {
		t.verify = func(app string) (bool, error) {
			out, code := slimRun(180*time.Second, "/usr/bin/codesign", "--verify", "--deep", "--strict", app)
			if code == 0 {
				return false, nil
			}
			return strings.Contains(out, "detritus not allowed"), errors.New("code signature invalid")
		}
	}
	if t.gatekeeper == nil {
		t.gatekeeper = func(app string) int {
			_, code := slimRun(20*time.Second, "/usr/sbin/spctl", "--assess", "--type", "execute", app)
			return code
		}
	}
	if t.rosetta == nil {
		home := s.home
		t.rosetta = func() map[string]bool { return rosettaBundleIDs(home) }
	}
	if t.copyApp == nil {
		t.copyApp = func(src, dst string) error {
			if _, code := slimRun(10*time.Minute, "/usr/bin/ditto", "--clone", src, dst); code != 0 {
				return errors.New("copy failed")
			}
			return nil
		}
	}
	if t.stripDetritus == nil {
		t.stripDetritus = func(app string) {
			slimRun(2*time.Minute, "/usr/bin/xattr", "-rsd", "com.apple.FinderInfo", app)
			slimRun(2*time.Minute, "/usr/bin/xattr", "-rsd", "com.apple.ResourceFork", app)
		}
	}
	return t
}

// slimRun runs one system tool with a deadline and returns its combined output
// and exit code; -1 means it did not start, timed out or was killed.
func slimRun(timeout time.Duration, name string, args ...string) (string, int) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return string(out), exit.ExitCode()
	}
	return string(out), -1
}

var teamIDPattern = regexp.MustCompile(`^[A-Z0-9]{10}$`)

// Ported from tw93/Mole V1.70.0 bin/slim.sh slim_signing_team
// The Team ID when the bundle is signed with a Developer ID certificate; App
// Store, Apple, ad-hoc and unsigned bundles give "".
func developerIDTeam(app string) string {
	out, code := slimRun(30*time.Second, "/usr/bin/codesign", "-dv", "--verbose=2", app)
	if code != 0 {
		return ""
	}
	developerID, team := false, ""
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Authority=Developer ID Application: ") {
			developerID = true
		}
		if value, ok := strings.CutPrefix(line, "TeamIdentifier="); ok && team == "" {
			team = value
		}
	}
	if !developerID || !teamIDPattern.MatchString(team) {
		return ""
	}
	return team
}

// Ported from tw93/Mole V1.70.0 bin/slim.sh slim_app_prefers_rosetta
// "Open using Rosetta" in Finder's Get Info is stored per bundle id.
func rosettaBundleIDs(home string) map[string]bool {
	root, ok := plistRoot(filepath.Join(home, "Library/Preferences/com.apple.LaunchServices/com.apple.LaunchServices.plist"))
	if !ok {
		return nil
	}
	ids := map[string]bool{}
	dict, _ := root["LSArchitecturesForX86_64"].(map[string]any)
	for id := range dict {
		ids[id] = true
	}
	return ids
}

// Ported from tw93/Mole V1.70.0 lib/core/app_protection_data.sh OFFICIAL_UNINSTALLER_RULES
// Vendor-managed security and MDM apps leave daemons and system extensions
// that only the vendor's own uninstaller removes. Shape: vendor, bundle id
// prefixes, name fragments.
var officialUninstallerRules = []struct {
	vendor    string
	prefixes  []string
	fragments []string
}{
	{"ESET", []string{"com.eset."}, []string{"eset management agent", "eset remote administrator agent", "eset endpoint security", "eset endpoint antivirus"}},
	{"Jamf", []string{"com.jamf.", "com.jamfsoftware."}, []string{"jamf connect", "jamf protect", "jamf self service"}},
	{"CrowdStrike", []string{"com.crowdstrike."}, []string{"crowdstrike", "falcon"}},
	{"SentinelOne", []string{"com.sentinelone.", "com.sentinel-labs."}, []string{"sentinelone", "sentinel agent"}},
	{"GlobalProtect", []string{"com.paloaltonetworks."}, []string{"globalprotect"}},
	{"Cisco", []string{"com.cisco.anyconnect", "com.cisco.secureclient"}, []string{"cisco secure client", "cisco anyconnect"}},
	{"Alibaba Cloud Security", []string{"com.alibaba.security."}, nil},
	{"Adobe Genuine Service", []string{"com.adobe.gcclient", "gocart."}, []string{"adobe genuine service"}},
	{"Avast", []string{"com.avast.aafm", "com.avast.avast", "com.avast.hub"}, nil},
	{"AVG", []string{"com.avg.hub"}, []string{"avgantivirus", "avg antivirus"}},
	{"Malwarebytes", []string{"com.malwarebytes.mbam"}, nil},
	{"TeamViewer", []string{"com.teamviewer."}, nil},
	{"Bitdefender", []string{"com.bitdefender.antivirusformac"}, nil},
	{"Intego", []string{"com.intego.virusbarrier.", "com.intego.virusbarrier-", "com.intego.app.netbarrier"}, nil},
	{"Kaspersky", []string{"com.kaspersky.kav"}, nil},
	{"Norton", []string{"com.symantec.nfm"}, nil},
	{"Trend Micro", []string{"com.trendmicro.tmsm", "com.trendmicro.icore"}, nil},
}

// Ported from tw93/Mole V1.70.0 lib/core/app_protection.sh official_uninstaller_vendor
func officialUninstallerVendor(bundleID, displayName, appPath string) string {
	id, name := strings.ToLower(bundleID), strings.ToLower(displayName)
	file := strings.ToLower(strings.TrimSuffix(filepath.Base(appPath), filepath.Ext(appPath)))
	for _, rule := range officialUninstallerRules {
		for _, prefix := range rule.prefixes {
			if strings.HasPrefix(id, prefix) {
				return rule.vendor
			}
		}
		for _, fragment := range rule.fragments {
			if strings.Contains(name, fragment) || strings.Contains(file, fragment) {
				return rule.vendor
			}
		}
	}
	return ""
}

// slimIdentity pins the app a scan listed: a replaced or updated bundle gets a
// different identity, and slimming then refuses it as changed since the scan.
func slimIdentity(app string) string {
	info, err := os.Lstat(app)
	if err != nil {
		return ""
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	identity := fmt.Sprintf("%d:%d:%d", stat.Dev, stat.Ino, info.ModTime().UnixNano())
	for _, rel := range []string{"Contents", "Contents/Info.plist"} {
		if sub, err := os.Stat(filepath.Join(app, rel)); err == nil {
			identity += fmt.Sprintf(":%d:%d", sub.Size(), sub.ModTime().UnixNano())
		}
	}
	return identity
}

// Ported from tw93/Mole V1.70.0 bin/slim.sh slim_skip_reason
// The reason an app is out of scope, and what to do about it, or "" when it
// may be slimmed. The order is Mole's.
func (s *Scanner) slimSkipReason(app, name string, tools slimTools, rosetta map[string]bool, procs []runningProc) (reason, unlock string) {
	if _, protected := s.scanProtection(app); protected {
		return "在白名单中，保持原状", ""
	}
	if info, err := os.Stat(filepath.Join(app, "Contents", "_MASReceipt")); err == nil && info.IsDir() {
		return "App Store 应用，签名由 Apple 管理，不处理", ""
	}
	if info, err := os.Lstat(app); err != nil || !ownedByMe(info) {
		return "不属于当前用户，不处理", ""
	}
	if syscall.Access(filepath.Dir(app), 2) != nil {
		return "所在文件夹没有写入权限，不处理", ""
	}
	if hasForeignFile(app) {
		return "应用内有其他用户的文件，不处理", ""
	}
	bundleID := readBundleInfo(app).identifier
	if bundleID == "" {
		return "没有 Bundle ID，不处理", ""
	}
	if strings.HasPrefix(bundleID, "com.apple.") {
		return "苹果自带应用，不处理", ""
	}
	if vendor := officialUninstallerVendor(bundleID, name, app); vendor != "" {
		return fmt.Sprintf("%s 的应用需要使用厂商官方的卸载程序，不处理", vendor), ""
	}
	if rosetta[bundleID] {
		return "已设为使用 Rosetta 打开，保持原状", ""
	}
	if running, _ := appRunning(procs, app); running {
		return "应用正在运行", fmt.Sprintf("退出 %s 后重新扫描即可瘦身", name)
	}
	return "", ""
}

func ownedByMe(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Getuid()
}

// hasForeignFile reports whether anything inside app belongs to another user.
func hasForeignFile(app string) bool {
	uid := uint32(os.Getuid())
	foreign := false
	_ = filepath.Walk(app, func(path string, info os.FileInfo, err error) error {
		if err != nil || foreign {
			return filepath.SkipAll
		}
		if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Uid != uid {
			foreign = true
		}
		return nil
	})
	return foreign
}

// SlimApp is one row of the slimming list.
type SlimApp struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	AppPath    string  `json:"appPath"`
	Icon       string  `json:"icon"`
	BundleID   string  `json:"bundleId"`
	Version    string  `json:"version"`
	IntelBytes int64   `json:"intelBytes"`
	IntelSize  string  `json:"intelSize"`
	IntelGb    float64 `json:"intelGb"`
	Files      int     `json:"files"`
	Tier       string  `json:"tier"` // review: can be slimmed once confirmed; protected: skipped
	Reason     string  `json:"reason"`
	Unlock     string  `json:"unlock,omitempty"`
	Running    bool    `json:"running"`
	Identity   string  `json:"identity"`
}

// SlimScan is the slimming list plus what could not be listed.
type SlimScan struct {
	Apps        []SlimApp `json:"apps"`
	NoIntel     int       `json:"noIntel"`     // apps scanned that hold no Intel code to remove
	Unavailable string    `json:"unavailable"` // non-empty when slimming does not apply to this Mac
}

const slimEligibleReason = "原应用会移入废纸篓，瘦身后的副本只保留 Apple 芯片代码；应用更新会带回 Intel 代码。"

// slimAppRoots lists the app folders Mole scans: the roots and one level of
// plain folders such as Utilities.
func (s *Scanner) slimAppRoots(tools slimTools) []string {
	roots := tools.roots
	if roots == nil {
		roots = []string{"/Applications", filepath.Join(s.home, "Applications")}
	}
	dirs := []string{}
	for _, root := range roots {
		dirs = append(dirs, root)
		entries, err := readDirEntries(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.Type()&os.ModeSymlink == 0 && entry.IsDir() && !strings.EqualFold(filepath.Ext(entry.Name()), ".app") {
				dirs = append(dirs, filepath.Join(root, entry.Name()))
			}
		}
	}
	return dirs
}

// ScanSlim lists the apps that hold Intel code, with the size of that code and
// whether each can be slimmed. It only reads.
// shortcut: symlinked and non-bundle .app entries are not listed at all, list them if users ask why an app is missing.
func (s *Scanner) ScanSlim() SlimScan {
	tools := s.slimToolset()
	scan := SlimScan{Apps: []SlimApp{}}
	if !tools.appleSilicon() {
		scan.Unavailable = "应用瘦身只适用于 Apple 芯片的 Mac"
		return scan
	}
	rosetta := tools.rosetta()
	procs := s.runningProcesses()
	type candidate struct {
		app   SlimApp
		index int
	}
	var candidates []candidate
	seen := map[string]bool{}
	for _, dir := range s.slimAppRoots(tools) {
		entries, err := readDirEntries(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !strings.EqualFold(filepath.Ext(entry.Name()), ".app") || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if seen[path] {
				continue
			}
			seen[path] = true
			if info, err := os.Stat(filepath.Join(path, "Contents")); err != nil || !info.IsDir() {
				continue
			}
			files, intel := slimPlan(path)
			if intel == 0 {
				scan.NoIntel++
				continue
			}
			info := readBundleInfo(path)
			name := firstNonEmpty(info.displayName, info.bundleName, strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))
			app := SlimApp{
				ID: path, Name: name, AppPath: path, Icon: appIconURL(path), BundleID: info.identifier, Version: info.shortVersion,
				IntelBytes: intel, IntelSize: formatBytes(intel), IntelGb: plainGB(intel), Files: len(files), Identity: slimIdentity(path),
			}
			app.Running, _ = appRunning(procs, path)
			app.Tier = "protected"
			app.Reason, app.Unlock = s.slimSkipReason(path, name, tools, rosetta, procs)
			if app.Reason == "" {
				candidates = append(candidates, candidate{app: app, index: len(scan.Apps)})
			}
			scan.Apps = append(scan.Apps, app)
		}
	}
	// Signature and Gatekeeper probes are slow, so the survivors run a few at a time.
	var wg sync.WaitGroup
	gate := make(chan struct{}, 4)
	for _, c := range candidates {
		wg.Add(1)
		go func() {
			defer wg.Done()
			gate <- struct{}{}
			defer func() { <-gate }()
			row := &scan.Apps[c.index]
			switch {
			case tools.team(row.AppPath) == "":
				row.Reason = "不是开发者 ID 签名的应用，不处理"
			case tools.gatekeeper(row.AppPath) == 3:
				row.Reason = "系统安全检查（Gatekeeper）不接受这个应用，不处理"
			default:
				row.Tier, row.Reason = "review", slimEligibleReason
			}
		}()
	}
	wg.Wait()
	sort.SliceStable(scan.Apps, func(i, j int) bool {
		a, b := scan.Apps[i], scan.Apps[j]
		if (a.Tier == "review") != (b.Tier == "review") {
			return a.Tier == "review"
		}
		return a.IntelBytes > b.IntelBytes
	})
	return scan
}

// SlimTarget is one app the user confirmed, with the identity the scan listed.
type SlimTarget struct{ Path, Identity string }

// SlimResult is the receipt for one app.
type SlimResult struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	Slimmed    bool   `json:"slimmed"`
	Reason     string `json:"reason"`
	FreedBytes int64  `json:"freedBytes"`
	Freed      string `json:"freed"`
	Trashed    string `json:"trashed"` // where the original went
}

// SlimOutcome is the receipt the page waits for.
type SlimOutcome struct {
	Requested  int          `json:"requested"`
	Results    []SlimResult `json:"results"`
	FreedBytes int64        `json:"freedBytes"`
	Freed      string       `json:"freed"`
}

var slimMu sync.Mutex

// SlimApps slims each confirmed app on a copy and swaps the copy in only after
// it passes every check. The original goes to the Trash through TrashPaths.
func (s *Scanner) SlimApps(targets []SlimTarget) SlimOutcome {
	outcome := SlimOutcome{Requested: len(targets), Results: []SlimResult{}}
	if !slimMu.TryLock() {
		for _, target := range targets {
			outcome.Results = append(outcome.Results, SlimResult{Path: target.Path, Name: filepath.Base(target.Path), Reason: "已有瘦身任务在进行，不能重复处理"})
		}
		return outcome
	}
	defer slimMu.Unlock()
	tools := s.slimToolset()
	for _, target := range targets {
		result := s.slimOne(target, tools)
		outcome.FreedBytes += result.FreedBytes
		outcome.Results = append(outcome.Results, result)
	}
	outcome.Freed = formatBytes(outcome.FreedBytes)
	return outcome
}

// Ported from tw93/Mole V1.70.0 bin/slim.sh slim_one_app
// shortcut: an interrupted swap (crash between the Trash move and the rename) leaves the slimmed copy in a
// hidden .orbit-slim-* folder beside the app; add a startup sweep if that ever shows up.
func (s *Scanner) slimOne(target SlimTarget, tools slimTools) (result SlimResult) {
	app := filepath.Clean(target.Path)
	result = SlimResult{Path: app, Name: strings.TrimSuffix(filepath.Base(app), filepath.Ext(app))}
	fail := func(reason string) SlimResult { result.Reason = reason; return result }
	if !isAppBundle(app) || !filepath.IsAbs(app) {
		return fail("不是应用包")
	}
	if info, err := os.Lstat(app); err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fail("不是应用包")
	}
	if info := readBundleInfo(app); info.displayName != "" {
		result.Name = info.displayName
	} else if info.bundleName != "" {
		result.Name = info.bundleName
	}
	if reason, _ := s.slimSkipReason(app, result.Name, tools, tools.rosetta(), s.runningProcesses()); reason != "" {
		return fail(reason)
	}
	if target.Identity == "" || slimIdentity(app) != target.Identity {
		return fail("应用在扫描后发生了变化，请重新扫描")
	}
	team := tools.team(app)
	if team == "" {
		return fail("不是开发者 ID 签名的应用，不处理")
	}
	files, intel := slimPlan(app)
	if len(files) == 0 || intel == 0 {
		return fail("没有可以去掉的 Intel 代码")
	}
	// A signature that is already broken cannot prove the copy is intact.
	detritus, err := tools.verify(app)
	if err != nil && !detritus {
		return fail("应用的签名本身已经失效")
	}
	stage, err := os.MkdirTemp(filepath.Dir(app), ".orbit-slim-")
	if err != nil {
		return fail("无法在应用所在文件夹创建临时目录")
	}
	keepStage := false
	defer func() {
		if !keepStage {
			os.RemoveAll(stage) // SAFE: orbit's own staging folder, holding only the copy it made
		}
	}()
	copyPath := filepath.Join(stage, filepath.Base(app))
	if err := tools.copyApp(app, copyPath); err != nil {
		return fail("复制应用失败")
	}
	// Finder xattrs fail --strict but sit outside the seal: strip them on the
	// copy only, and the untouched copy then has to pass in place of the original.
	if detritus {
		tools.stripDetritus(copyPath)
		if _, err := tools.verify(copyPath); err != nil {
			return fail("应用的签名本身已经失效")
		}
	}
	for _, file := range files {
		if err := thinFile(filepath.Join(copyPath, file.Rel)); err != nil {
			return fail(fmt.Sprintf("无法去掉 %s 里的 Intel 代码", filepath.Base(file.Rel)))
		}
	}
	if reason := verifySlimCopy(copyPath, team, files, tools); reason != "" {
		return fail(reason)
	}
	// Protection or the process list may change while the copy is checked.
	if _, protected := s.scanProtection(app); protected {
		return fail("在白名单中，保持原状")
	}
	if running, _ := appRunning(s.runningProcesses(), app); running {
		return fail("应用正在运行，请先退出")
	}
	trash := s.TrashPaths([]string{app})
	if len(trash.Moved) != 1 || exists(app) {
		reason := "无法把原应用移入废纸篓"
		if len(trash.Skipped) > 0 {
			reason = trash.Skipped[0].Reason
		} else if len(trash.Failed) > 0 {
			reason = trash.Failed[0].Error
		}
		return fail(reason)
	}
	result.Trashed = trash.Moved[0].Trashed
	if err := os.Rename(copyPath, app); err != nil {
		keepStage = true
		return fail(fmt.Sprintf("原应用已在废纸篓，请从废纸篓放回 %s", filepath.Dir(app)))
	}
	result.Slimmed, result.FreedBytes, result.Freed = true, intel, formatBytes(intel)
	return result
}

// Ported from tw93/Mole V1.70.0 bin/slim.sh slim_thin_file
// Replaces file with its arm64 slice through an exclusively created temp file
// next to it, keeping the original mode.
func thinFile(path string) error {
	arm, _, ok := fileArmSlice(path)
	if !ok {
		return errors.New("no thinnable slice")
	}
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	out, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".orbit-slim-")
	if err != nil {
		return err
	}
	tmp := out.Name()
	installed := false
	defer func() {
		if !installed {
			os.Remove(tmp) // SAFE: exclusively created slice output that was not installed
		}
	}()
	written, err := io.Copy(out, io.NewSectionReader(src, int64(arm.offset), int64(arm.size)))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil || uint64(written) != arm.size || !isThinArm(tmp) {
		return errors.New("slice incomplete")
	}
	if err := os.Chmod(tmp, info.Mode().Perm()); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	installed = true
	return nil
}

// Ported from tw93/Mole V1.70.0 bin/slim.sh slim_verify_copy
// Every check runs on the slimmed copy; any failure keeps the original.
func verifySlimCopy(copyPath, team string, files []slimFile, tools slimTools) string {
	for _, file := range files {
		if !isThinArm(filepath.Join(copyPath, file.Rel)) {
			return "瘦身后缺少 Apple 芯片代码"
		}
	}
	if _, err := tools.verify(copyPath); err != nil {
		return "瘦身后的签名检查没有通过"
	}
	if tools.team(copyPath) != team {
		return "瘦身后签名团队发生了变化"
	}
	if tools.gatekeeper(copyPath) != 0 {
		return "瘦身后的系统安全检查（Gatekeeper）没有通过"
	}
	return ""
}
