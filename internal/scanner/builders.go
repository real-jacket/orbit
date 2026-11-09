package scanner

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// builderNames are the command names that can hold a project's build artifacts open.
var builderNames = map[string]bool{
	"node": true, "npm": true, "pnpm": true, "yarn": true, "bun": true, "cargo": true, "rustc": true,
	"java": true, "gradle": true, "python": true, "python3": true, "uv": true, "xcodebuild": true, "swift-frontend": true,
	"xcode": true, "sourcekitservice": true, "xcbbuildservice": true,
	"ruby": true, "npx": true,
}

// isBuilderName also admits versioned interpreters such as python3.12.
func isBuilderName(name string) bool {
	return builderNames[name] || strings.HasPrefix(name, "python")
}

// cacheOwnerRules say which running processes hold a cache open. owners match
// the executable name; cli matches a tool that runs as a script under a generic
// interpreter (npm, yarn and pip are `node`/`python` processes), so a bare
// node or python process never blocks a cache.
var cacheOwnerRules = []struct {
	roots, owners []string
	cli           string
}{
	{[]string{"Library/Caches/com.apple.Safari"}, []string{"Safari"}, ""},
	{[]string{"Library/Caches/Google"}, []string{"Google Chrome"}, ""},
	{[]string{"Library/Caches/Firefox"}, []string{"Firefox"}, ""},
	{[]string{"Library/Developer/Xcode/DerivedData"}, []string{"Xcode", "xcodebuild", "swift-frontend"}, ""},
	{[]string{"Library/Developer/CoreSimulator/Caches"}, []string{"Simulator", "CoreSimulatorService"}, ""},
	{[]string{"Library/Caches/Cursor"}, []string{"Cursor", "cursor-agent"}, ""},
	{[]string{"Library/Logs/Claude"}, []string{"Claude"}, ""},
	{[]string{"Library/Caches/com.openai.chat"}, []string{"ChatGPT"}, ""},
	{[]string{"Library/Caches/com.anthropic.claudefordesktop"}, []string{"Claude"}, ""},
	{[]string{"Library/Caches/com.lmstudio.lmstudio"}, []string{"LM Studio", "LMStudio"}, ""},
	{[]string{".npm/_cacache"}, nil, "npm"},
	{[]string{"Library/Caches/pnpm"}, nil, "pnpm"},
	{[]string{"Library/Caches/Yarn"}, nil, "yarn"},
	{[]string{".bun/install/cache"}, []string{"bun"}, ""},
	{[]string{"Library/Caches/pip"}, nil, "pip"},
	{[]string{".cache/uv"}, []string{"uv"}, ""},
	{[]string{"go/pkg/mod/cache/download", "Library/Caches/go-build"}, []string{"go"}, ""},
	{[]string{".cargo/registry/cache"}, []string{"cargo", "rustc"}, ""},
	{[]string{"Library/Caches/Homebrew"}, []string{"brew"}, "brew"},
}

// installingSubcommands are the package-manager commands that read or write
// the download cache; `npm run dev` or `pnpm dev` do not.
var installingSubcommands = map[string]bool{
	"install": true, "i": true, "add": true, "ci": true, "update": true, "up": true, "upgrade": true,
	"dlx": true, "exec": true, "x": true, "fetch": true, "cache": true, "dedupe": true, "rebuild": true,
	"link": true, "create": true, "import": true,
}

// cliRunning reports whether a running process is tool's own CLI doing work
// that touches its cache. The command line decides, never the interpreter.
func cliRunning(procs []builderProc, tool string) bool {
	for _, p := range procs {
		fields := strings.Fields(p.args)
		for i, field := range fields {
			base := filepath.Base(field)
			var markers []string
			bareInstalls := false // `yarn` / `pnpm` alone is an install
			always := false       // every invocation uses the cache
			switch tool {
			case "npm":
				markers = []string{"npm-cli.js", "npm", "npx-cli.js", "npx"}
			case "pnpm":
				markers, bareInstalls = []string{"pnpm", "pnpm.cjs", "pnpm.mjs"}, true
			case "yarn":
				markers, bareInstalls = []string{"yarn", "yarn.js", "yarn.cjs", "yarnpkg"}, true
			case "pip":
				markers, always = []string{"pip", "pip3"}, true
				if base == "pip" && i > 0 && fields[i-1] == "-m" {
					return true
				}
			case "brew":
				markers, always = []string{"brew", "brew.rb", "brew.sh"}, true
			case "gradle":
				if strings.Contains(field, "org.gradle.") { // the daemon is `java ... org.gradle.launcher...`
					return true
				}
				markers, always = []string{"gradle", "gradlew"}, true
			}
			if !containsString(markers, base) {
				continue
			}
			if always || base == "npx" || base == "npx-cli.js" {
				return true
			}
			sub := ""
			for _, next := range fields[i+1:] {
				if !strings.HasPrefix(next, "-") {
					sub = next
					break
				}
			}
			if installingSubcommands[sub] || sub == "" && bareInstalls {
				return true
			}
		}
	}
	return false
}

// xcodeNames are the only processes that can be using Xcode's DerivedData: it
// belongs to no project folder, so node, python or java say nothing about it.
var xcodeNames = map[string]bool{"xcodebuild": true, "xcode": true, "swift-frontend": true, "sourcekitservice": true, "xcbbuildservice": true}

// inDerivedData reports whether path is Xcode's DerivedData or anything below it.
func inDerivedData(path string) bool {
	for _, part := range strings.Split(path, "/") {
		if part == "DerivedData" {
			return true
		}
	}
	return false
}

// xcodeUsing reports the first running Xcode-family process.
func xcodeUsing(procs []builderProc) (string, bool) {
	for _, p := range procs {
		if xcodeNames[p.name] {
			return p.name, true
		}
	}
	return "", false
}

// builderProc is one running builder with where it works. cwd is empty when the
// system would not tell us (another user's process): only args can attribute it.
type builderProc struct {
	pid  int
	name string
	cwd  string
	args string
}

// builderSnapshot is the attribution table for the artifact check. Display
// callers may reuse one up to processTTL old; the move boundary (live) always
// takes a fresh one. A failure is never reused, and callers must treat it as busy.
func (s *Scanner) builderSnapshot(live bool) ([]builderProc, error) {
	if live || s.processTTL <= 0 {
		return s.listBuilders()
	}
	s.builderMu.Lock()
	defer s.builderMu.Unlock()
	if s.builderAt.IsZero() || time.Since(s.builderAt) > s.processTTL {
		procs, err := s.listBuilders()
		if err != nil {
			return nil, err
		}
		s.builderProcs, s.builderAt = procs, time.Now()
	}
	return s.builderProcs, nil
}

func (s *Scanner) listBuilders() ([]builderProc, error) {
	if s.builderProbe != nil {
		return s.builderProbe()
	}
	if s.processProbe != nil { // test seam: an injected process table has no cwd or args to attribute
		return nil, nil
	}
	return systemBuilders()
}

func runTool(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Output()
}

// systemBuilders runs, in order:
//
//	ps -axo pid=,comm=            which pids are builders
//	ps -axo pid=,args=            their full command lines
//	lsof -a -d cwd -Fpn -p <pids> their working directories
//
// Any failure of the first two is an error. lsof exiting 1 with nothing printed
// means no cwd was readable (or the builders just exited); the builders are then
// attributed by args only.
func systemBuilders() ([]builderProc, error) {
	comm, err := runTool("/bin/ps", "-axo", "pid=,comm=")
	if err != nil || strings.TrimSpace(string(comm)) == "" {
		return nil, errors.New("进程列表读取失败")
	}
	var procs []builderProc
	index := map[int]int{}
	for _, p := range parseProcessLines(string(comm)) {
		if name := strings.ToLower(filepath.Base(p.path)); p.pid > 0 && isBuilderName(name) {
			index[p.pid] = len(procs)
			procs = append(procs, builderProc{pid: p.pid, name: name})
		}
	}
	if len(procs) == 0 {
		return nil, nil
	}
	args, err := runTool("/bin/ps", "-axo", "pid=,args=")
	if err != nil {
		return nil, err
	}
	for _, p := range parseProcessLines(string(args)) {
		if i, ok := index[p.pid]; ok {
			procs[i].args = p.path
		}
	}
	pids := make([]string, 0, len(procs))
	for _, p := range procs {
		pids = append(pids, strconv.Itoa(p.pid))
	}
	out, err := runTool("/usr/sbin/lsof", "-a", "-d", "cwd", "-Fpn", "-p", strings.Join(pids, ","))
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return nil, err
		}
	}
	current := -1
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "p"):
			current = -1
			if pid, err := strconv.Atoi(line[1:]); err == nil {
				if i, ok := index[pid]; ok {
					current = i
				}
			}
		case strings.HasPrefix(line, "n") && current >= 0:
			procs[current].cwd = line[1:]
		}
	}
	return procs, nil
}

// builderUsing reports the first builder that can be working on the project at
// root: its cwd is inside the project or the project is inside its cwd, or its
// command line names a path inside the project. A cwd of "/" is what launchd
// children report and says nothing about any project.
func builderUsing(procs []builderProc, root string) (string, bool) {
	for _, p := range procs {
		if p.cwd != "" && p.cwd != "/" && (withinPath(p.cwd, root) || withinPath(root, p.cwd)) {
			return p.name, true
		}
		if mentionsPath(p.args, root) {
			return p.name, true
		}
	}
	return "", false
}

// mentionsPath reports whether text contains root or a path below it, at a path
// boundary, so /work/app does not match /work/app-two.
func mentionsPath(text, root string) bool {
	text, root = strings.ToLower(text), strings.ToLower(root)
	for from := 0; ; {
		i := strings.Index(text[from:], root)
		if i < 0 {
			return false
		}
		end := from + i + len(root)
		if end == len(text) || text[end] == '/' || unicode.IsSpace(rune(text[end])) || strings.ContainsRune(`"'=:,;`, rune(text[end])) {
			return true
		}
		from = end
	}
}
