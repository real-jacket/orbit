package scanner

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mymoles/mole-studio/internal/fsread"
)

const (
	purgeConfigReadBudget = time.Second
	purgeConfigMaxBytes   = 1 << 20
	maxPurgeRoots         = 100
)

type purgeConfigRead struct {
	done     chan struct{}
	deadline time.Time
	content  string
	present  bool
	err      error
}

var purgeConfigReads = struct {
	sync.Mutex
	pending map[string]*purgeConfigRead
}{pending: map[string]*purgeConfigRead{}}

type PurgeScanRoot struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}

func readPurgePathConfig(path string) (string, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), purgeConfigReadBudget)
	defer cancel()
	return readPurgePathConfigContext(ctx, path)
}

// Like fsread.ReadDir, a canceled caller does not leave another syscall behind
// on retry. Metadata or reads on unavailable storage can block in the kernel,
// so share the one pending operation until it finishes. Completed operations
// are discarded immediately: a later scan must reload the saved scope.
func readPurgePathConfigContext(ctx context.Context, path string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", true, err
	}
	path = filepath.Clean(path)
	purgeConfigReads.Lock()
	read := purgeConfigReads.pending[path]
	if read == nil {
		deadline, _ := ctx.Deadline()
		read = &purgeConfigRead{done: make(chan struct{}), deadline: deadline}
		purgeConfigReads.pending[path] = read
		go func() {
			read.content, read.present, read.err = readRegularPurgePathConfig(path)
			purgeConfigReads.Lock()
			if purgeConfigReads.pending[path] == read {
				delete(purgeConfigReads.pending, path)
			}
			purgeConfigReads.Unlock()
			close(read.done)
		}()
	}
	purgeConfigReads.Unlock()
	select {
	case <-read.done:
	default:
		var timeout <-chan time.Time
		if !read.deadline.IsZero() {
			timer := time.NewTimer(time.Until(read.deadline))
			defer timer.Stop()
			timeout = timer.C
		}
		select {
		case <-read.done:
		case <-ctx.Done():
			return "", true, ctx.Err()
		case <-timeout:
			return "", true, context.DeadlineExceeded
		}
	}
	purgeConfigReads.Lock()
	if purgeConfigReads.pending[path] == read {
		delete(purgeConfigReads.pending, path)
	}
	purgeConfigReads.Unlock()
	return read.content, read.present, read.err
}

func readRegularPurgePathConfig(path string) (string, bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", true, err
	}
	if !info.Mode().IsRegular() {
		return "", true, fmt.Errorf("扫描路径配置必须是普通文件")
	}
	// The path could be replaced by a FIFO after Stat. O_NONBLOCK prevents open
	// from waiting for a writer; inspect the actual descriptor before reading.
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", true, err
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return "", true, err
	}
	if !info.Mode().IsRegular() {
		return "", true, fmt.Errorf("扫描路径配置必须是普通文件")
	}
	data, err := io.ReadAll(io.LimitReader(file, purgeConfigMaxBytes+1))
	if err != nil {
		return "", true, err
	}
	if len(data) > purgeConfigMaxBytes {
		return "", true, fmt.Errorf("扫描路径配置超过 1 MB")
	}
	return string(data), true, nil
}

func (s *Scanner) normalizeBuildScanRoot(raw string) (string, string) {
	if s.home == "" || s.home == "/" {
		return "", "无法确定用户家目录，未启用构建扫描"
	}
	path := filepath.Clean(s.expand(raw))
	if !filepath.IsAbs(path) || strings.ContainsAny(raw, "\x00\r\n") {
		return "", "扫描目录必须是绝对路径或以 ~/ 开头的目录"
	}
	path = resolvePathCase(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
		// /var and /private/var (and a symlinked home) are the same home.
		// Keep the scanner's home spelling so path display and live builder
		// attribution do not lose their common prefix after resolution.
		if physicalHome, err := filepath.EvalSymlinks(s.home); err == nil {
			if relative, err := filepath.Rel(physicalHome, path); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				path = filepath.Join(s.home, relative)
			}
		}
	}
	for _, root := range []string{"/", "/Users", "/Library", "/Applications", "/private", "/var", "/tmp", "/private/tmp", "/private/var", "/opt", "/Volumes", s.home, filepath.Join(s.home, "Library"), filepath.Join(s.home, "Applications")} {
		if strings.EqualFold(path, root) {
			return "", "扫描目录范围过大，请指定存放项目的子目录"
		}
	}
	for _, root := range []string{"/System", "/usr", "/bin", "/sbin", "/etc", "/dev", "/private/var/db", "/Applications"} {
		if withinPath(path, root) {
			return "", "系统与应用目录不作为构建项目扫描根目录"
		}
	}
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		if isBundleDir(part) {
			return "", "应用包与资源包不作为构建项目扫描根目录"
		}
	}
	return path, ""
}

func inspectBuildScanRoot(path string) PurgeScanRoot {
	root := PurgeScanRoot{Path: path, Status: "unknown"}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := fsread.ReadDir(ctx, path)
	switch {
	case err == nil:
		root.Status = "readable"
	case os.IsNotExist(err):
		root.Status, root.Reason = "missing", "目录不存在，已跳过"
	case os.IsPermission(err):
		root.Status, root.Reason = "denied", "目录读取被拒绝，请检查目录的访问控制"
	default:
		root.Reason = "目录读取未完成：" + err.Error()
	}
	return root
}
