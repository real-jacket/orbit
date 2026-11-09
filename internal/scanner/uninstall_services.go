package scanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// An uninstall used to only move files to the Trash, so an app's LaunchAgent kept
// running until logout. This file stops the user-level background services of the
// apps being removed. It never touches the system domain, LaunchDaemons or
// anything that needs sudo, and it records what it stopped so a restore can
// start the same services again.

// LaunchUnload is one launchd job in gui/<uid> that was stopped just before its
// plist moved to the Trash. The plist path is the entry's Original.
type LaunchUnload struct {
	Label string `json:"label"`
}

// StoppedService is one background service the uninstall stopped.
type StoppedService struct {
	Label string `json:"label"`
	Kind  string `json:"kind"` // "agent" (a plist that went to the Trash) or "helper" (a login item helper inside the app)
}

// ServiceFailure is one background service that could not be stopped or started again.
type ServiceFailure struct {
	Label  string `json:"label"`
	Error  string `json:"error"`
	Detail string `json:"detail,omitempty"` // launchctl's own words, shown verbatim
}

// CaskNote says Homebrew still records a removed app and how the user can clear it.
type CaskNote struct {
	App     string `json:"app"`
	Token   string `json:"token"`
	Command string `json:"command"`
}

// UninstallReceipt is what the uninstall did besides moving files.
type UninstallReceipt struct {
	Stopped  []StoppedService `json:"stopped"`
	Reloaded []string         `json:"reloaded"` // stopped, then started again because the Trash move failed
	Failed   []ServiceFailure `json:"failed"`
	Casks    []CaskNote       `json:"casks"`
	// LoginItemsManual is true when an app was removed: its entry in System
	// Settings > Login Items is not touched, see the note on removeLoginItem.
	LoginItemsManual bool `json:"loginItemsManual"`
}

// trashHooks lets an uninstall act around each move without TrashPaths knowing
// about launchd: before runs after every check passed, failed when the move
// did not happen, moved when it did.
type trashHooks struct {
	before func(path string) []LaunchUnload
	failed func(path string, unloaded []LaunchUnload)
	moved  func(path, trashed string)
}

const launchctlTimeout = 10 * time.Second

// caskrooms and lsregisterPath are variables so tests never look at, or call, the real ones.
var (
	caskrooms      = []string{"/opt/homebrew/Caskroom", "/usr/local/Caskroom"}
	lsregisterPath = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
)

// launchLabelOK accepts only labels that are safe to hand to launchctl. A
// third-party plist could claim an Apple label; that namespace is never touched.
func launchLabelOK(label string) bool {
	return reverseDNS.MatchString(label) && !strings.HasPrefix(strings.ToLower(label), "com.apple.")
}

type uninstallApp struct {
	path, name, bundleID string
	helpers              []string
}

type uninstallPlan struct {
	s       *Scanner
	uid     string
	apps    map[string]uninstallApp // by app path
	agents  map[string]string       // owned user LaunchAgent plist path -> label
	receipt UninstallReceipt
	removed int // app bundles that really moved
}

// TrashUninstall is TrashPathsChecked for an uninstall: the user-level launchd
// jobs of the apps in paths are stopped around the Trash move, in this order:
//  1. an owned LaunchAgent is booted out right before its own plist moves, and
//     started again if that move fails;
//  2. an app's login item helpers and its LaunchServices record go after the app
//     really is in the Trash, because a helper cannot be started again without
//     its app, so there is nothing to undo.
func (s *Scanner) TrashUninstall(paths []string, identities map[string]FileIdentity) (TrashOutcome, UninstallReceipt) {
	plan := s.planUninstall(paths)
	outcome := s.trashPaths(paths, identities, &trashHooks{before: plan.before, failed: plan.failed, moved: plan.moved})
	if plan.removed > 0 {
		plan.refreshLaunchServices()
	}
	return outcome, plan.finish()
}

func (s *Scanner) planUninstall(rawPaths []string) *uninstallPlan {
	plan := &uninstallPlan{s: s, uid: strconv.Itoa(os.Getuid()), apps: map[string]uninstallApp{}, agents: map[string]string{}}
	var plists []string
	userAgents := filepath.Join(s.home, "Library/LaunchAgents")
	for _, raw := range rawPaths {
		path := filepath.Clean(s.expand(raw))
		if !filepath.IsAbs(path) {
			continue
		}
		switch {
		case isAppBundle(path):
			if info, err := os.Lstat(path); err == nil && info.IsDir() {
				plan.addApp(path)
			}
		case filepath.Dir(path) == userAgents && strings.HasSuffix(path, ".plist"):
			plists = append(plists, path)
		}
	}
	if len(plan.apps) == 0 || len(plists) == 0 {
		return plan
	}
	items := s.loadLaunchItems()
	for _, path := range plists {
		if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() {
			continue
		}
		for _, item := range items {
			if item.path != path {
				continue
			}
			label := item.label
			if label == "" {
				label = strings.TrimSuffix(filepath.Base(path), ".plist")
			}
			for _, app := range plan.apps {
				if launchLabelOK(label) && item.belongsTo(app.path, app.bundleID) {
					plan.agents[path] = label
				}
			}
		}
	}
	return plan
}

func (p *uninstallPlan) addApp(path string) {
	info := readBundleInfo(path)
	name := info.displayName
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	p.apps[path] = uninstallApp{path: path, name: name, bundleID: info.identifier, helpers: loginItemHelperIDs(path)}
	// Read before the move: afterwards the bundle's link into the Caskroom is gone.
	if token := caskToken(path); token != "" {
		p.receipt.Casks = append(p.receipt.Casks, CaskNote{App: filepath.Base(path), Token: token, Command: "brew uninstall --cask " + token})
	}
}

// Ported from tw93/Mole V1.70.0 lib/uninstall/batch.sh discover_login_item_helper_bundle_ids
func loginItemHelperIDs(appPath string) []string {
	root := filepath.Join(appPath, "Contents/Library/LoginItems")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var ids []string
	for _, entry := range entries {
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".app") {
			continue
		}
		if id := readBundleInfo(filepath.Join(root, entry.Name())).identifier; reverseDNS.MatchString(id) {
			ids = append(ids, id)
		}
	}
	return ids
}

// caskToken says which Homebrew cask installed the app, from the files only:
// brew itself is never run. Ported from tw93/Mole V1.70.0 lib/uninstall/brew.sh
// _detect_cask_via_resolved_path and _detect_cask_via_caskroom_search. It is
// stricter than Mole: the app must resolve into a Caskroom, or exactly one cask
// must hold a link to this very bundle.
func caskToken(appPath string) string {
	if resolved, err := filepath.EvalSymlinks(appPath); err == nil && filepath.Base(resolved) == filepath.Base(appPath) {
		for _, room := range caskrooms {
			if rel, ok := strings.CutPrefix(resolved, room+"/"); ok {
				return caskTokenOK(strings.SplitN(rel, "/", 2)[0])
			}
		}
	}
	want, err := os.Stat(appPath)
	if err != nil {
		return ""
	}
	tokens := map[string]bool{}
	for _, room := range caskrooms {
		for _, link := range globDepth(room, 2, filepath.Base(appPath)) {
			if info, err := os.Stat(link); err == nil && os.SameFile(info, want) {
				tokens[strings.SplitN(strings.TrimPrefix(link, room+"/"), "/", 2)[0]] = true
			}
		}
	}
	if len(tokens) != 1 {
		return ""
	}
	for token := range tokens {
		return caskTokenOK(token)
	}
	return ""
}

var caskTokenPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// caskTokenOK keeps a token that is safe to print inside a shell command.
func caskTokenOK(token string) string {
	if caskTokenPattern.MatchString(token) {
		return token
	}
	return ""
}

// globDepth lists room/<token>/<version>/name entries (two directory levels below room).
func globDepth(room string, levels int, name string) []string {
	pattern := room
	for i := 0; i < levels; i++ {
		pattern = filepath.Join(pattern, "*")
	}
	matches, _ := filepath.Glob(filepath.Join(pattern, name))
	sort.Strings(matches)
	return matches
}

func (p *uninstallPlan) launchctl(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), launchctlTimeout)
	defer cancel()
	return p.s.runCmd(ctx, "", "launchctl", args...)
}

// loaded asks launchd, read only, whether the job is in the user's domain now.
func (p *uninstallPlan) loaded(label string) bool {
	_, err := p.launchctl("print", "gui/"+p.uid+"/"+label)
	return err == nil
}

func (p *uninstallPlan) fail(label, message string, err error) {
	p.receipt.Failed = append(p.receipt.Failed, ServiceFailure{Label: label, Error: message, Detail: errDetail(err)})
}

func errDetail(err error) string {
	if err == nil {
		return ""
	}
	if _, detail, ok := strings.Cut(err.Error(), ": "); ok && strings.TrimSpace(detail) != "" {
		return strings.TrimSpace(detail)
	}
	return strings.TrimRight(strings.TrimSpace(err.Error()), ":")
}

// Ported from tw93/Mole V1.70.0 lib/uninstall/batch.sh unload_launch_plist
// (Mole calls `launchctl unload`; this uses the current `bootout` on gui/<uid>,
// and only for a job that is loaded, so a restore never starts one that was off).
func (p *uninstallPlan) before(path string) []LaunchUnload {
	label, ok := p.agents[path]
	if !ok || !p.loaded(label) {
		return nil
	}
	if out, err := p.launchctl("bootout", "gui/"+p.uid+"/"+label); err != nil {
		p.fail(label, "launchctl 未能停止这个后台服务", fmt.Errorf("%w: %s", err, strings.TrimSpace(out)))
		return nil
	}
	p.receipt.Stopped = append(p.receipt.Stopped, StoppedService{Label: label, Kind: "agent"})
	return []LaunchUnload{{Label: label}}
}

func (p *uninstallPlan) failed(path string, unloaded []LaunchUnload) {
	for _, job := range unloaded {
		p.dropStopped(job.Label)
		if _, err := p.launchctl("bootstrap", "gui/"+p.uid, path); err != nil {
			p.fail(job.Label, "移入废纸篓失败，且未能重新加载这个后台服务，注销后重新登录即可恢复", err)
			continue
		}
		p.receipt.Reloaded = append(p.receipt.Reloaded, job.Label)
	}
}

func (p *uninstallPlan) dropStopped(label string) {
	kept := p.receipt.Stopped[:0]
	for _, svc := range p.receipt.Stopped {
		if svc.Label != label {
			kept = append(kept, svc)
		}
	}
	p.receipt.Stopped = kept
}

// siblingInstalled says whether another app bundle outside this uninstall still
// shares data with app, looking in the folders scanInstalledApps lists. Read only.
// Ported from tw93/Mole V1.70.0 lib/uninstall/batch.sh (the same-bundle-id sibling
// guard before bootout_login_item_helpers): helper ids are identical across
// siblings, so booting them out would stop the surviving install's helper.
func (p *uninstallPlan) siblingInstalled(app uninstallApp) bool {
	for _, dir := range p.s.defaultAppDirs() {
		entries, err := readDirEntries(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			other := filepath.Join(dir, entry.Name())
			if _, selected := p.apps[other]; selected || !isAppBundle(other) || other == app.path {
				continue
			}
			if bundlesShareRemnants(app.bundleID, readBundleInfo(other).identifier, app.path, other) {
				return true
			}
		}
	}
	return false
}

// moved runs after an app bundle is in the Trash, at trashed.
func (p *uninstallPlan) moved(path, trashed string) {
	app, ok := p.apps[path]
	if !ok {
		return
	}
	p.removed++
	helpers := app.helpers
	if p.siblingInstalled(app) {
		helpers = nil
	}
	// Ported from tw93/Mole V1.70.0 lib/uninstall/batch.sh bootout_login_item_helpers
	for _, id := range helpers {
		if !launchLabelOK(id) || !p.loaded(id) {
			continue
		}
		if out, err := p.launchctl("bootout", "gui/"+p.uid+"/"+id); err != nil {
			p.fail(id, "launchctl 未能停止这个后台服务", fmt.Errorf("%w: %s", err, strings.TrimSpace(out)))
			continue
		}
		p.receipt.Stopped = append(p.receipt.Stopped, StoppedService{Label: id, Kind: "helper"})
	}
	// Ported from tw93/Mole V1.70.0 lib/uninstall/batch.sh unregister_app_bundle
	// (best effort and not reported). Mole only unregisters a bundle that exists, so
	// this targets the copy now in the Trash, not the original path that is gone.
	ctx, cancel := context.WithTimeout(context.Background(), launchctlTimeout)
	defer cancel()
	_, _ = p.s.runCmd(ctx, "", lsregisterPath, "-u", trashed)
}

// Ported from tw93/Mole V1.70.0 lib/uninstall/batch.sh refresh_launch_services_after_uninstall
func (p *uninstallPlan) refreshLaunchServices() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, _ = p.s.runCmd(ctx, "", lsregisterPath, "-gc")
}

// removeLoginItem is deliberately not ported. Mole's remove_login_item runs
// osascript against System Events, which raises macOS's Automation prompt, and a
// deleted login item cannot be put back by a restore. The receipt tells the user
// to check System Settings instead.
func (p *uninstallPlan) finish() UninstallReceipt {
	r := p.receipt
	r.LoginItemsManual = p.removed > 0
	if r.Stopped == nil {
		r.Stopped = []StoppedService{}
	}
	if r.Reloaded == nil {
		r.Reloaded = []string{}
	}
	if r.Failed == nil {
		r.Failed = []ServiceFailure{}
	}
	if r.Casks == nil {
		r.Casks = []CaskNote{}
	}
	return r
}

// restartServices starts the jobs a restored LaunchAgent plist had before the
// uninstall stopped them. It only acts on a plist directly inside the user's
// LaunchAgents folder, whatever the manifest says.
func (s *Scanner) restartServices(entry TrashEntry) (started []string, failed []ServiceFailure) {
	if len(entry.Unloaded) == 0 || filepath.Dir(entry.Original) != filepath.Join(s.home, "Library/LaunchAgents") || !strings.HasSuffix(entry.Original, ".plist") {
		return nil, nil
	}
	p := &uninstallPlan{s: s, uid: strconv.Itoa(os.Getuid())}
	for _, job := range entry.Unloaded {
		if !launchLabelOK(job.Label) {
			continue
		}
		if _, err := p.launchctl("bootstrap", "gui/"+p.uid, entry.Original); err != nil {
			failed = append(failed, ServiceFailure{Label: job.Label, Error: "文件已还原，但未能重新加载这个后台服务，注销后重新登录即可恢复", Detail: errDetail(err)})
			continue
		}
		started = append(started, job.Label)
	}
	return started, failed
}
