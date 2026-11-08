// Package scanner measures the disk: how large directories are, which apps
// are installed, which files are big, and what a clean, purge or uninstall
// run would touch. It is a port of the app's Swift RealMacSystemScanner, and
// its payloads keep the exact shape the frontend already reads.
package scanner

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Scanner measures the disk. Use Shared.
type Scanner struct {
	home         string
	processProbe func() (string, error)
	handleProbe  func(string) (bool, error)
	installProbe func(aiInstall) bool
	builderProbe func() ([]builderProc, error)
	aiUnits      []aiSessionUnit // set only on a scan-scoped view, never on Shared

	// processTTL is how long display-time checks may reuse one `ps` table; zero
	// means always ask. The move boundary (live) never reads the cached table.
	processTTL time.Duration
	psMu       sync.Mutex
	psOutput   string
	psAt       time.Time

	builderMu    sync.Mutex // display-time cache of the builder attribution snapshot
	builderProcs []builderProc
	builderAt    time.Time

	// openSnapshot answers display-time open-file checks from one system-wide
	// lsof taken on first use; set only on a scan-scoped view.
	openSnapshot func(string) (bool, error)

	unitMu    sync.Mutex // guards the short-lived session enumeration cache
	unitCache []aiSessionUnit
	unitAt    time.Time

	// Telemetry is sampled on every tick, but not everything has to be measured
	// that often: the process table costs a `ps` fork (~15 ms), so it is reused
	// for a few seconds, and a CPU share needs the previous kernel tick counts.
	sample telemetryState
}

// Shared is the app's scanner.
var Shared = &Scanner{home: homeDir(), processTTL: 2 * time.Second}

// homeDir returns the user's home directory.
func homeDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "/"
}

// Home returns the home directory the scanner measures relative to.
func (s *Scanner) Home() string { return s.home }

// expand replaces a leading ~ with the home directory.
func (s *Scanner) expand(path string) string {
	if path == "~" {
		return s.home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(s.home, path[2:])
	}
	return path
}

// relativize replaces the home directory at the start of path with ~, which is
// what the frontend shows.
func (s *Scanner) relativize(path string) string {
	if path == s.home {
		return "~"
	}
	if strings.HasPrefix(path, s.home+"/") {
		return "~" + strings.TrimPrefix(path, s.home)
	}
	return path
}

// round2 rounds v to two decimal places.
func round2(v float64) float64 {
	if v >= 0 {
		return float64(int64(v*100+0.5)) / 100
	}
	return float64(int64(v*100-0.5)) / 100
}

// round1 rounds v to one decimal place.
func round1(v float64) float64 {
	if v >= 0 {
		return float64(int64(v*10+0.5)) / 10
	}
	return float64(int64(v*10-0.5)) / 10
}

// roundPtr rounds a pointer'd value and keeps nil meaning "this machine does
// not report it", so the frontend can tell that apart from zero.
func roundPtr(v *float64, places int) *float64 {
	if v == nil {
		return nil
	}
	var rounded float64
	if places == 1 {
		rounded = round1(*v)
	} else {
		rounded = round2(*v)
	}
	return &rounded
}

// Keep measured sizes exact; round only when displaying them.
func gb(bytes int64) float64 { return plainGB(bytes) }

func plainGB(bytes int64) float64 {
	return float64(bytes) / 1_000_000_000
}

// formatBytes renders bytes the way the app's lists show them.
func formatBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	if bytes < 1000 {
		return itoa64(bytes) + " B"
	}
	if bytes < 1_000_000 {
		return formatFloat(float64(bytes)/1000, 1) + " KB"
	}
	value := float64(bytes) / 1_000_000_000
	if value >= 0.1 {
		return formatFloat(value, 2) + " GB"
	}
	mb := float64(bytes) / 1_000_000
	if mb < 1 {
		mb = 1
	}
	return formatFloat(mb, 0) + " MB"
}

// formatFloat formats v with the given number of decimals, as printf's %.*f
// does.
func formatFloat(v float64, decimals int) string {
	return trimFloat(v, decimals)
}

// trimFloat is formatFloat without a leading sign split.
func trimFloat(v float64, decimals int) string {
	mult := 1.0
	for i := 0; i < decimals; i++ {
		mult *= 10
	}
	scaled := v * mult
	var rounded float64
	if scaled >= 0 {
		rounded = float64(int64(scaled+0.5)) / mult
	} else {
		rounded = float64(int64(scaled-0.5)) / mult
	}
	return fixedFloat(rounded, decimals)
}

// fixedFloat renders v with exactly decimals digits after the point.
func fixedFloat(v float64, decimals int) string {
	mult := 1.0
	for i := 0; i < decimals; i++ {
		mult *= 10
	}
	scaled := v * mult
	var n int64
	if scaled >= 0 {
		n = int64(scaled + 0.5)
	} else {
		n = int64(scaled - 0.5)
	}
	if decimals == 0 {
		return itoa64(n)
	}
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	digits := itoa64(n)
	for len(digits) <= decimals {
		digits = "0" + digits
	}
	whole := digits[:len(digits)-decimals]
	frac := digits[len(digits)-decimals:]
	return sign + whole + "." + frac
}

// itoa64 renders n in base 10.
func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// exists reports whether path exists.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// nowUnix is the current time in seconds since the epoch.
func nowUnix() int64 {
	return time.Now().Unix()
}

// isDir reports whether path is a directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// dirSize is a directory's size in bytes, how many entries were visited, and
// how many of them were regular files. The two counts differ because the caps
// in the scan functions bound files, not the directory entries that lead to
// them: a tree with thousands of empty folders used to exhaust the budget
// before a single byte was counted, which is how a 2.1 GB cache directory was
// reported as 0.89 GB.
type dirSize struct {
	bytes     int64
	count     int
	files     int
	modified  int64
	partial   bool
	timedOut  bool // the walk budget ran out; partial is set too
	directory bool
	deadline  time.Time
	linked    map[[2]uint64]bool
}

// walkSlots bounds how many directory walks run at once; each one fans out over
// the shared walkWorkers.
var walkSlots = make(chan struct{}, 2)

// walkWorkers is the process-wide pool of extra goroutines walks may spawn; a
// walk that finds it empty descends inline, so it never waits on it.
var walkWorkers = make(chan struct{}, 8)

// scanWalkBudget is how long one measured root may be walked. A full scan has
// no file cap: a big cache is measured completely unless this runs out.
const scanWalkBudget = 15 * time.Second

// fastDirectorySize measures how large a directory is within scanWalkBudget,
// walking at most maxFiles regular files (0 means no cap). A path that is not a
// directory reports its own size and one entry, which is what the Swift app did.
func (s *Scanner) fastDirectorySize(path string, maxFiles int) dirSize {
	return s.measureDirectory(path, maxFiles, scanWalkBudget)
}

// measureDirectory is fastDirectorySize with the walk budget as a parameter.
// A root that comes back partial leaves exactly one read warning, on the root.
func (s *Scanner) measureDirectory(path string, maxFiles int, budget time.Duration) dirSize {
	path = s.expand(path)
	info, err := os.Lstat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			recordReadError(path, err)
			return dirSize{partial: true}
		}
		return dirSize{}
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return dirSize{}
	}
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return dirSize{}
		}
		return dirSize{bytes: allocatedSize(info), count: 1, files: 1, modified: info.ModTime().Unix()}
	}
	// The budget starts once a slot is free, so a full scan running its
	// sections side by side spends it walking, not queueing.
	walkSlots <- struct{}{}
	defer func() { <-walkSlots }()
	total := dirSize{modified: info.ModTime().Unix(), deadline: time.Now().Add(budget), directory: true}
	readErr := walkDir(path, maxFiles, &total)
	switch {
	case readErr != nil:
		recordReadError(path, readErr)
	case total.timedOut:
		recordReadWarning(path, "目录测量超时；只计入已读取部分")
	case total.partial:
		recordReadWarning(path, "达到文件测量上限；只计入已读取部分")
	}
	return total
}

// walkDir accumulates the sizes of the regular files under dir into total, and
// returns the first directory read error. A zero total.deadline means no budget.
func walkDir(dir string, maxFiles int, total *dirSize) error {
	run := &walkRun{total: total, maxFiles: maxFiles}
	run.visit(dir, true)
	run.wg.Wait()
	return run.err
}

// walkRun is one parallel walk of a directory tree into total.
type walkRun struct {
	mu       sync.Mutex // guards total, err
	total    *dirSize
	maxFiles int
	stop     atomic.Bool // the cap or the deadline was hit
	err      error       // the first directory that could not be read
	wg       sync.WaitGroup
}

// visit adds the regular files of dir to the total and descends into its
// subdirectories, in parallel while walkWorkers has room. The cap counts regular
// files; directories cost nothing and are still visited while the budget lasts.
func (r *walkRun) visit(dir string, root bool) {
	if r.stop.Load() {
		return
	}
	if !r.total.deadline.IsZero() && time.Now().After(r.total.deadline) {
		r.mu.Lock()
		r.total.partial, r.total.timedOut = true, true
		r.mu.Unlock()
		r.stop.Store(true)
		return
	}
	list := listDirItems
	if root {
		list = listDirGuarded
	}
	items, err := list(dir)
	r.mu.Lock()
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		r.total.partial = true
		if r.err == nil {
			r.err = err
		}
	}
	var subdirs []string
	for _, item := range items {
		if r.maxFiles > 0 && r.total.files >= r.maxFiles {
			r.total.partial = true
			r.stop.Store(true)
			break
		}
		r.total.count++
		switch item.kind {
		case itemDir:
			subdirs = append(subdirs, dir+"/"+item.name)
		case itemFile:
			r.addFile(item)
		}
	}
	r.mu.Unlock()
	for _, sub := range subdirs {
		select {
		case walkWorkers <- struct{}{}:
			r.wg.Add(1)
			go func() {
				defer func() { <-walkWorkers; r.wg.Done() }()
				r.visit(sub, false)
			}()
		default:
			r.visit(sub, false)
		}
	}
}

// addFile counts one regular file; r.mu is held.
func (r *walkRun) addFile(item dirItem) {
	total := r.total
	total.files++
	total.modified = max(total.modified, item.mtime)
	if item.nlink > 1 {
		if total.linked == nil {
			total.linked = make(map[[2]uint64]bool)
		}
		key := [2]uint64{item.dev, item.ino}
		if total.linked[key] {
			return
		}
		total.linked[key] = true
	}
	total.bytes += item.bytes
}

// scanProtection reports how the whitelist covers path. LoadWhitelist stores
// mole's legacy composite "a & b" lines as a single pattern; the app shows them
// split, so protection decisions split them too, otherwise a line such as
// "~/Library/Caches & ~/Library/Logs" would protect nothing.
func (s *Scanner) scanProtection(path string) (Protection, bool) {
	if protection, protected := builtinCleanupProtection(path, s.home); protected {
		return protection, true
	}
	if protection, protected := runtimeCleanupProtection(path); protected {
		return protection, true
	}
	policy := s.whitelist()
	if protection, protected := policy.Protection(path); protected {
		return protection, true
	}
	parts := splitCompositeWhitelistPatterns(policy.Patterns, s.home)
	if len(parts) == 0 {
		return Protection{}, false
	}
	return Whitelist{Patterns: parts}.Protection(path)
}

// splitCompositeWhitelistPatterns returns the individual absolute paths of the
// legacy composite entries, so the rest of the scanner can treat each one as
// the protection it was meant to be.
func splitCompositeWhitelistPatterns(patterns []string, home string) []string {
	var parts []string
	for _, pattern := range patterns {
		if !strings.Contains(pattern, " & ") {
			continue
		}
		for _, part := range strings.Split(pattern, " & ") {
			// LoadWhitelist only expands the first token of a composite line,
			// so "~/Library/Logs" is still abbreviated here.
			part = expandWhitelistPattern(strings.TrimSpace(part), home)
			if !strings.HasPrefix(part, "/") {
				continue
			}
			if !containsString(parts, part) {
				parts = append(parts, part)
			}
		}
	}
	return parts
}

// scanMayOffer reports whether path may be offered for deletion: it is not
// protected at all, or only because a protected child lives under it, in which
// case the caller measures its unprotected children instead of the folder as a
// whole. Anything else is left out entirely, so a checked row is always a row
// the trash implementation will accept.
func (s *Scanner) scanMayOffer(path string) (expandChildren bool, ok bool) {
	protection, protected := s.scanProtection(path)
	if !protected {
		return false, true
	}
	return protection.Descendant, protection.Descendant
}

// allocatedSize is a file's size on disk, blocks times block size, which is
// what FileManager's totalFileAllocatedSize reported. Zero blocks are valid
// for a sparse file, not a reason to substitute its logical length.
func allocatedSize(info os.FileInfo) int64 {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return max(0, int64(stat.Blocks)*512)
	}
	if size := info.Size(); size > 0 {
		return size
	}
	return 0
}
