package scanner

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// launchItem is one launchd plist that may belong to an installed app.
type launchItem struct {
	path, label, program string
}

// launchRoots lists where launchd jobs live. Only the first is the user's own;
// the other two are root-owned and can be listed but not removed here.
func (s *Scanner) launchRoots() []string {
	return []string{filepath.Join(s.home, "Library/LaunchAgents"), "/Library/LaunchAgents", "/Library/LaunchDaemons"}
}

// loadLaunchItems reads every launchd plist once per scan, so matching 200 apps
// does not parse 200 copies of the same directories.
func (s *Scanner) loadLaunchItems() []launchItem {
	var items []launchItem
	for _, root := range s.launchRoots() {
		entries, err := readDirEntries(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".plist") {
				continue
			}
			path := filepath.Join(root, entry.Name())
			props, err := parsePlistFile(path)
			if err != nil {
				continue
			}
			// ProgramArguments is an array; parsePlist keeps its first element,
			// which is the executable.
			program := props["Program"]
			if program == "" {
				program = props["ProgramArguments"]
			}
			items = append(items, launchItem{path: path, label: props["Label"], program: program})
		}
	}
	return items
}

// belongsTo matches by executable inside the bundle, or by Label / file name
// equal to the bundle id or one of its dotted children (never a bare prefix:
// com.tencent.Lemon must not claim com.tencent.LemonMonitor).
func (l launchItem) belongsTo(appPath, bundleID string) bool {
	if l.program != "" && withinPath(l.program, appPath) {
		return true
	}
	id := strings.ToLower(bundleID)
	if !strings.Contains(id, ".") || strings.HasPrefix(id, "local.app.") {
		return false
	}
	for _, name := range []string{l.label, strings.TrimSuffix(filepath.Base(l.path), ".plist")} {
		name = strings.ToLower(name)
		if name == id || strings.HasPrefix(name, id+".") {
			return true
		}
	}
	return false
}

func launchPathsFor(items []launchItem, appPath, bundleID string) []string {
	var paths []string
	for _, item := range items {
		if item.belongsTo(appPath, bundleID) {
			paths = append(paths, item.path)
		}
	}
	return paths
}

// runningProc is one process with its executable path.
type runningProc struct {
	pid  int
	path string
}

// runningProcesses lists processes with their pid. A probe may return bare
// paths, in which case pid is 0 and the process cannot be asked to quit.
func (s *Scanner) runningProcesses() []runningProc {
	var output string
	if s.processProbe != nil {
		output, _ = s.processProbe()
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		raw, _ := exec.CommandContext(ctx, "/bin/ps", "-axo", "pid=,comm=").Output()
		output = string(raw)
	}
	return parseProcessLines(output)
}

// parseProcessLines reads "path" or "pid path" lines.
func parseProcessLines(output string) []runningProc {
	var procs []runningProc
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		proc := runningProc{path: line}
		if head, rest, ok := strings.Cut(line, " "); ok {
			if pid, err := strconv.Atoi(head); err == nil {
				proc = runningProc{pid: pid, path: strings.TrimSpace(rest)}
			}
		}
		procs = append(procs, proc)
	}
	return procs
}

// appRunning reports whether any process executes from inside the bundle, and
// the main executable (Contents/MacOS) to ask to quit, if one is identifiable.
func appRunning(procs []runningProc, appPath string) (bool, runningProc) {
	var running bool
	var main runningProc
	for _, proc := range procs {
		if !withinPath(proc.path, appPath) {
			continue
		}
		running = true
		if proc.pid > 0 && withinPath(proc.path, filepath.Join(appPath, "Contents/MacOS")) && (main.pid == 0 || proc.pid < main.pid) {
			main = proc
		}
	}
	return running, main
}

// parseMdlsLastUsed reads `mdls -raw` output for n paths: NUL-separated values,
// "(null)" when Spotlight has none. A count mismatch (mdls aborts on a missing
// file) or an unparsable value stays unknown instead of guessing.
func parseMdlsLastUsed(output string, n int) []*time.Time {
	result := make([]*time.Time, n)
	values := strings.Split(output, "\x00")
	if len(values) != n {
		return result
	}
	for i, value := range values {
		if parsed, err := time.Parse("2006-01-02 15:04:05 -0700", strings.TrimSpace(value)); err == nil {
			result[i] = &parsed
		}
	}
	return result
}

// appLastUsed asks Spotlight for kMDItemLastUsedDate in chunks.
func appLastUsed(paths []string) []*time.Time {
	result := make([]*time.Time, 0, len(paths))
	for start := 0; start < len(paths); start += 40 {
		chunk := paths[start:min(start+40, len(paths))]
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		args := append([]string{"-name", "kMDItemLastUsedDate", "-raw"}, chunk...)
		var out bytes.Buffer
		cmd := exec.CommandContext(ctx, "/usr/bin/mdls", args...)
		cmd.Stdout = &out
		_ = cmd.Run()
		cancel()
		result = append(result, parseMdlsLastUsed(out.String(), len(chunk))...)
	}
	return result
}
