package scanner

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// CodeIdentity names the running build's code identity (bundle path plus
// designated requirement). TCC ties every grant to it, so a record kept for one
// identity says nothing about another. Empty for a bare executable.
var CodeIdentity string

// appNeedsAdminReason is shown for bundles the current user does not own.
const appNeedsAdminReason = "这个应用由 App Store 或安装器以管理员身份安装，移动需要管理员密码；orbit 不申请管理员权限。可在访达中把它拖到废纸篓（会要求输入密码），或在启动台中删除。"

// currentUID is replaceable so tests can pretend someone else owns a bundle.
var currentUID = func() uint32 { return uint32(os.Getuid()) }

// bundleNeedsAdmin reports whether moving the bundle needs administrator rights:
// a directory owned by another user (root for App Store installs) cannot be
// renamed into the Trash by this user, and App Management cannot change that.
// shortcut: ownership only; a non-admin user in a root-owned /Applications is not detected, add an access(2) check if that is reported.
func bundleNeedsAdmin(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid != currentUID()
}

func isPermissionErr(err error) bool {
	return errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES)
}

// tccDBPath is the file every "is Full Disk Access on" check opens.
func (s *Scanner) tccDBPath() string {
	return filepath.Join(s.home, "Library", "Application Support", "com.apple.TCC", "TCC.db")
}

// fdaFromProbe maps the result of opening TCC.db read-only: success means Full
// Disk Access is on, EPERM/EACCES means it is off, anything else (file missing,
// I/O trouble) cannot decide and the caller falls back to other evidence.
func fdaFromProbe(err error) string {
	switch {
	case err == nil:
		return "granted"
	case isPermissionErr(err):
		return "denied"
	}
	return ""
}

// probeFullDisk opens TCC.db and closes it at once; no row is ever read.
func (s *Scanner) probeFullDisk() string {
	file, err := os.Open(s.tccDBPath())
	if err == nil {
		_ = file.Close()
	}
	return fdaFromProbe(err)
}

// AppManagementRecord is the outcome of the last real app-bundle move. macOS has
// no API to ask whether App Management is on, so the only honest evidence is
// what happened when orbit moved an app the current user owns.
type AppManagementRecord struct {
	Identity string `json:"identity"`
	State    string `json:"state"` // effective | denied
	App      string `json:"app"`
	At       int64  `json:"at"` // unix milliseconds
}

var permissionsMu sync.Mutex

func permissionsPath() string { return filepath.Join(StateDir(), "permissions.json") }

// ReadAppManagement returns the record for the running identity, or nil when
// there is none or it was written by a different build.
func ReadAppManagement() *AppManagementRecord {
	permissionsMu.Lock()
	defer permissionsMu.Unlock()
	data, err := os.ReadFile(permissionsPath())
	if err != nil {
		return nil
	}
	var record AppManagementRecord
	if json.Unmarshal(data, &record) != nil || record.Identity == "" || record.Identity != CodeIdentity {
		return nil
	}
	return &record
}

// recordAppManagement keeps the newest outcome; without an identity there is
// nothing a grant could be tied to, so nothing is written.
func recordAppManagement(state, appPath string, now time.Time) {
	if CodeIdentity == "" {
		return
	}
	permissionsMu.Lock()
	defer permissionsMu.Unlock()
	data, err := json.Marshal(AppManagementRecord{Identity: CodeIdentity, State: state, App: filepath.Base(appPath), At: now.UnixMilli()})
	if err != nil || os.MkdirAll(StateDir(), 0o755) != nil {
		return
	}
	tmp := permissionsPath() + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		_ = os.Rename(tmp, permissionsPath())
	}
}
