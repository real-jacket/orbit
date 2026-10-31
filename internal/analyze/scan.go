package analyze

// This file is the in-process replacement for `mo analyze`: one walk over a
// directory tree, an in-memory tree the UI can drill into without re-scanning,
// and the bookkeeping (restricted paths, aggregation, progress, cancellation)
// that a treemap needs.
//
// It deliberately sticks to the standard library. getattrlistbulk would be
// faster, but it is error-prone with bundles and volume boundaries, and the
// first version of the analyzer has to be exact about what it counts before it
// is fast. os.ReadDir plus one Lstat per entry is the expensive part, so the
// walk is parallelized across directories: 10 filesystem threads statting
// 2.3M inodes one at a time is dominated by syscall latency, and the kernel is
// happy to run many of them at once.
//
// # Concurrency model
//
// Directories are the unit of work. A fixed pool of workers pops directory
// jobs from a queue, reads one directory, counts its files inline (a file's
// lstat is cheap next to shipping it to another goroutine) and pushes child
// directories back onto the queue. There is no goroutine waiting for each child directory:
// a 2.3M-file home directory has hundreds of thousands of directories, and a
// goroutine each would cost more memory than the tree itself.
//
// Every directory owns a dirRecord with a pending counter: one for its own
// ReadDir plus one per child directory it queued. A worker subtracts its own
// unit when it finishes the directory; the worker that takes the counter to
// zero finalizes that record - folds small children into "…其它 N 项" - and
// then subtracts one from its parent, possibly cascading. The last one out
// assembles the root, so no goroutine ever waits for a child.
//
// Shared state (hard-link table, tallies, progress throttle, large-file list)
// is behind mutexes or atomics. Ordering is fixed afterwards, never by arrival
// order: children sort by DiskBytes desc, then Name asc, then Path asc, so two
// scans of the same tree serialize to identical JSON.
//
// The counting rules callers rely on:
//
//   - DiskBytes is allocated size, st_blocks*512; LogBytes is st_size. Sparse
//     files therefore report little DiskBytes and their full LogBytes; APFS
//     clones/compression can make DiskBytes larger than LogBytes.
//   - Symbolic links are not followed and not counted at all (a symlink loop
//     therefore cannot hang the walk).
//   - Hard-linked regular files are counted once per (dev, ino); repeats only
//     bump ScanStats.HardLinkedDeduped.
//   - A directory whose subtree contains an unreadable directory is Partial,
//     the unreadable path lands in ScanStats.Restricted, and the walk keeps
//     going.
//   - Directory inodes' own blocks are not counted; only their contents are.
//     That is a few KB per directory away from `du -sk`, which the UI does not
//     show anyway.

import (
	"context"
	"errors"
	"fmt"
	"github.com/mymoles/mole-studio/internal/fsread"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	// defaultMinKeepBytes folds children smaller than 1 MB into "…其它 N 项"
	// when the caller does not pick a threshold.
	defaultMinKeepBytes = 1 << 20

	// maxChildren caps how many children one directory keeps. A folder with
	// thousands of small children would otherwise dominate the treemap and the
	// payload that crosses the bridge.
	maxChildren = 60

	// progressEveryFiles and progressEvery are the two triggers for the
	// Progress callback: every 2000 files or every 200 ms, whichever comes
	// first. The caller owns the real throttle; this only stops us from
	// calling back once per file.
	progressEveryFiles = 2000
	progressEvery      = 200 * time.Millisecond

	// largeFileThreshold and maxLargeFiles define the Top 50 large-file list.
	// The threshold is applied to allocated size so it matches DiskBytes.
	largeFileThreshold = 100 << 20
	maxLargeFiles      = 50

	// Worker pool sizing. Lstat on a warm APFS volume is fast but has
	// syscall latency, so oversubscribing the filesystem threads helps more
	// than it hurts; above 32 workers the volume is the bottleneck and memory
	// pressure starts to show.
	workersPerCPU = 4
	minWorkers    = 4
	maxWorkers    = 32

	// cancelCheckEvery is how many entries a directory worker processes
	// between ctx checks. Checking ctx.Err() per entry would add a lock to
	// every lstat's critical path.
	cancelCheckEvery = 64
)

// systemVolumesPath is the APFS volume-group root. Scanning it from "/" would
// count the Data volume twice, once through its firmlinks and once through
// this path, so it is skipped and reported as excluded.
const systemVolumesPath = "/System/Volumes"

// systemExcludedDirs are macOS bookkeeping directories that are not user data.
// Scanning them would only make the analyzer blame the OS for its own records.
var systemExcludedDirs = map[string]string{
	".Spotlight-V100":         "Spotlight 索引目录",
	".fseventsd":              "文件系统事件目录",
	".Trashes":                "废纸篓目录",
	".DocumentRevisions-V100": "文稿版本目录",
}

// Node is one item in the scanned tree.
type Node struct {
	Name      string
	Path      string // absolute path; empty for the synthetic "…其它" node
	DiskBytes int64  // allocated, st_blocks*512; for directories, the subtree sum
	LogBytes  int64  // logical, st_size; for directories, the subtree sum
	FileCount int64  // regular files in the subtree
	Dir       bool
	Partial   bool    // some directory in the subtree could not be read
	Children  []*Node // directories only: aggregated, sorted by DiskBytes desc
}

// LargeFile is one entry of the Top 50 large-file list. SizeBytes uses the
// allocated size, the same measure as Node.DiskBytes.
type LargeFile struct {
	Name      string
	Path      string
	SizeBytes int64
}

// Restriction records a path the scan did not read or did not visit, and why.
type Restriction struct {
	Path   string
	Reason string
}

// ScanStats is everything about a scan that is not the tree itself.
type ScanStats struct {
	Files, Dirs       int64 // Dirs 含扫描根本身
	HardLinkedDeduped int64 // hard links skipped after the first one
	Restricted        []Restriction
	Excluded          []Restriction
	LargeFiles        []LargeFile
	Partial           bool // at least one Restricted entry
	Elapsed           time.Duration
	Canceled          bool // ctx was canceled; the tree is a prefix, not a total
}

// ScanOptions configures one scan.
type ScanOptions struct {
	Root         string // absolute path, must be a directory
	MinKeepBytes int64  // children below this fold into "…其它 N 项"; <=0 means 1 MB
	// Progress is called at most every 200 ms or every 2000 files. Calls are
	// serialized, so the callback may touch state without extra locking. The
	// caller throttles further if it needs to; it may be nil.
	Progress    func(scannedFiles int64, currentPath string)
	CrossDevice bool // false (default) stops at a volume boundary
}

// ScanTree walks opts.Root once and returns the tree plus stats.
//
// The walk is parallel across directories but deterministic: the tree, the
// aggregated children and ScanStats are ordered by size then name/path, never
// by goroutine arrival order.
//
// Cancellation is not an error: when ctx is canceled, ScanTree returns the
// partial tree, stats.Canceled is true, and err is nil. The only errors are
// unusable options (missing root, root is not a directory).
func ScanTree(ctx context.Context, opts ScanOptions) (*Node, *ScanStats, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()

	if strings.TrimSpace(opts.Root) == "" {
		return nil, nil, errors.New("analyze: 扫描根目录为空")
	}
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, nil, fmt.Errorf("analyze: 解析扫描根目录 %q: %w", opts.Root, err)
	}
	root = filepath.Clean(root)
	info, err := os.Stat(root) // follow a symlinked root once, then stop following
	if err != nil {
		return nil, nil, fmt.Errorf("analyze: 扫描根目录 %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("analyze: 扫描根目录 %s 不是目录", root)
	}
	if opts.MinKeepBytes <= 0 {
		opts.MinKeepBytes = defaultMinKeepBytes
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, nil, fmt.Errorf("analyze: 无法读取 %s 的设备号", root)
	}

	scanner := &treeScanner{
		ctx:          ctx,
		opts:         opts,
		rootDev:      uint64(stat.Dev),
		restricted:   make(map[string]string),
		excluded:     make(map[string]string),
		lastReportAt: start,
	}
	// The first callback tells the UI which root it is about to watch; it is a
	// single synchronous call, so it cannot race with the workers.
	if opts.Progress != nil {
		opts.Progress(0, root)
	}

	rootRecord := &dirRecord{
		node: &Node{Name: rootName(root), Path: root, Dir: true},
	}
	queue := newDirQueue()
	queue.push(dirJob{path: root, record: rootRecord})

	workers := workerCount()
	var group sync.WaitGroup
	group.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer group.Done()
			scanner.worker(queue)
		}()
	}
	group.Wait()

	// The walk only collects; attribution and aggregation happen here, after
	// every path is known, so the result does not depend on goroutine timing.
	scanner.resolveHardLinks()
	node := scanner.finalizeDir(rootRecord)
	return node, scanner.buildStats(ctx, start), nil
}

// Find returns the node whose Path is path, or nil. Paths folded into the
// synthetic "…其它 N 项" node have no path of their own and are never returned;
// callers drill into nodes the UI showed them, so that is not a problem.
func Find(root *Node, path string) *Node {
	if root == nil || path == "" {
		return nil
	}
	target := filepath.Clean(path)
	if root.Path != "" && filepath.Clean(root.Path) == target {
		return root
	}
	for _, child := range root.Children {
		if child == nil || child.Path == "" {
			continue
		}
		if filepath.Clean(child.Path) == target {
			return child
		}
		if child.Dir && pathIsUnder(target, child.Path) {
			if found := Find(child, target); found != nil {
				return found
			}
		}
	}
	return nil
}

// pathIsUnder reports whether target is strictly inside parent.
func pathIsUnder(target, parent string) bool {
	if parent == "/" {
		return len(target) > 1 && target[0] == '/'
	}
	return strings.HasPrefix(target, parent+string(filepath.Separator))
}

// fileKey identifies an inode on one device, for hard-link de-duplication.
type fileKey struct {
	dev uint64
	ino uint64
}

// dirJob is one directory handed to a worker.
type dirJob struct {
	path   string
	record *dirRecord
}

// dirRecord is the per-directory accumulator. Exactly one worker owns a record
// while it reads the directory, so its fields need no lock: children simply
// push their records into childRecords and every sum is computed after the walk
// is over. Keeping finalization out of the walk is what makes hard-link
// attribution deterministic: the winner of a (dev, ino) group can only be
// decided once every path is known.
type dirRecord struct {
	node         *Node
	parent       *dirRecord
	childRecords []*dirRecord // subdirectories queued by the owning worker

	disk       int64
	log        int64
	files      int64
	smallDisk  int64 // files below MinKeepBytes, folded into "…其它"
	smallLog   int64
	smallCount int64
	childNodes []*Node // file nodes that survived the threshold
	partial    bool
	scanned    bool // false when cancellation dropped this directory's job
}

// hardOccurrence is one path of a hard-linked inode. They are collected during
// the walk and resolved afterwards so the counted path does not depend on which
// worker happened to see it first.
type hardOccurrence struct {
	key    fileKey
	path   string
	parent *dirRecord
	disk   int64
	log    int64
}

// dirQueue is an unbounded directory work queue with a live-job counter. It is
// unbounded on purpose: pushes happen from workers, and a bounded channel
// would let all workers block on send with nobody left to receive.
type dirQueue struct {
	mu     sync.Mutex
	cond   *sync.Cond
	jobs   []dirJob
	live   int // queued plus in-flight
	closed bool
}

func newDirQueue() *dirQueue {
	queue := &dirQueue{}
	queue.cond = sync.NewCond(&queue.mu)
	return queue
}

func (q *dirQueue) push(job dirJob) {
	q.mu.Lock()
	if q.closed { // cancellation already drained the queue; drop late children
		q.mu.Unlock()
		return
	}
	q.live++
	q.jobs = append(q.jobs, job)
	q.cond.Signal()
	q.mu.Unlock()
}

// pop blocks until a job is available; ok is false when the queue is drained
// and no job can appear any more.
func (q *dirQueue) pop() (dirJob, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.jobs) == 0 {
		if q.closed {
			return dirJob{}, false
		}
		q.cond.Wait()
	}
	job := q.jobs[len(q.jobs)-1]
	q.jobs = q.jobs[:len(q.jobs)-1]
	return job, true
}

// finish marks one popped job as accounted for and closes the queue when the
// last job is done.
func (q *dirQueue) finish() {
	q.mu.Lock()
	q.live--
	if q.live <= 0 {
		q.closed = true
		q.cond.Broadcast()
	}
	q.mu.Unlock()
}

// shutdown drops queued jobs so workers exit promptly on cancellation.
func (q *dirQueue) shutdown() {
	q.mu.Lock()
	q.closed = true
	q.jobs = nil
	q.cond.Broadcast()
	q.mu.Unlock()
}

// treeScanner holds the state shared by all workers.
type treeScanner struct {
	ctx     context.Context
	opts    ScanOptions
	rootDev uint64

	files      atomic.Int64
	dirs       atomic.Int64
	hardLinked atomic.Int64
	canceled   atomic.Bool

	hardMu    sync.Mutex
	hardLinks []hardOccurrence

	tallyMu    sync.Mutex
	restricted map[string]string
	excluded   map[string]string

	largeMu sync.Mutex
	large   []LargeFile

	progressMu      sync.Mutex
	lastReportFiles int64
	lastReportAt    time.Time
}

// workerCount oversubscribes the CPUs a little: the workers spend most of
// their time waiting on the filesystem, not on the CPU.
func workerCount() int {
	workers := runtime.GOMAXPROCS(0) * workersPerCPU
	if workers < minWorkers {
		workers = minWorkers
	}
	if workers > maxWorkers {
		workers = maxWorkers
	}
	return workers
}

// worker drains the queue until it is closed.
func (s *treeScanner) worker(queue *dirQueue) {
	for {
		job, ok := queue.pop()
		if !ok {
			return
		}
		if s.ctx.Err() != nil {
			s.markCanceled()
			job.record.partial = true
			queue.shutdown()
			queue.finish()
			continue
		}
		s.scanDir(job, queue)
		queue.finish()
	}
}

// scanDir reads one directory, counts its files inline, queues its
// subdirectories and hands the finished record to the pending-counter
// protocol.
func (s *treeScanner) scanDir(job dirJob, queue *dirQueue) {
	s.dirs.Add(1) // 含根目录；ReadDir 失败也计入，因为它确实被访问过
	record := job.record
	record.scanned = true
	entries, err := fsread.ReadDir(s.ctx, job.path)
	if err != nil {
		if s.ctx.Err() != nil {
			s.markCanceled()
			record.partial = true
			queue.shutdown()
		} else {
			s.restrict(job.path, err)
			record.partial = true
		}
		return
	}

	var disk, log, files int64
	var smallDisk, smallLog, smallCount int64
	for index, entry := range entries {
		if index%cancelCheckEvery == 0 && s.ctx.Err() != nil {
			s.markCanceled()
			record.partial = true
			queue.shutdown()
			break
		}
		childName := entry.Name()
		childPath := filepath.Join(job.path, childName)

		if reason, skip := systemExcludedDirs[childName]; skip {
			s.exclude(childPath, reason)
			continue
		}
		if childPath == systemVolumesPath {
			s.exclude(childPath, "APFS 系统卷组（避免与 Data 卷重复计数）")
			continue
		}

		info, err := entry.Info() // Lstat: a symlink reports itself, never its target
		if err != nil {
			if s.ctx.Err() != nil {
				s.markCanceled()
				queue.shutdown()
				break
			}
			s.restrict(childPath, err)
			record.partial = true
			continue
		}
		mode := info.Mode()
		if mode&fs.ModeSymlink != 0 {
			continue // not followed, not counted: the target may be counted elsewhere
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			continue
		}
		dev := uint64(stat.Dev)
		if !s.opts.CrossDevice && dev != s.rootDev {
			if mode.IsDir() {
				s.exclude(childPath, "跨卷（st_dev 与扫描根不同）")
			}
			continue
		}

		switch {
		case mode.IsDir():
			child := &dirRecord{
				node:   &Node{Name: childName, Path: childPath, Dir: true},
				parent: record,
			}
			record.childRecords = append(record.childRecords, child)
			queue.push(dirJob{path: childPath, record: child})
			s.reportProgress(childPath)

		case mode.IsRegular():
			diskBytes := allocatedBytes(stat)
			logBytes := info.Size()
			if uint64(stat.Nlink) > 1 {
				// The same inode may show up under several paths. Defer it so
				// the counted path is chosen by path order, not by which
				// worker ran first.
				s.addHardLink(hardOccurrence{
					key:    fileKey{dev: dev, ino: uint64(stat.Ino)},
					path:   childPath,
					parent: record,
					disk:   diskBytes,
					log:    logBytes,
				})
				s.reportProgress(childPath)
				continue
			}
			s.files.Add(1)
			disk += diskBytes
			log += logBytes
			files++
			s.considerLarge(childName, childPath, diskBytes)
			// A child below the threshold can never survive aggregation, so it
			// is summed here instead of allocated as a node. On a home
			// directory that keeps the tree at a few tens of MB instead of
			// hundreds.
			if diskBytes >= s.opts.MinKeepBytes {
				record.childNodes = append(record.childNodes, &Node{
					Name:      childName,
					Path:      childPath,
					DiskBytes: diskBytes,
					LogBytes:  logBytes,
					FileCount: 1,
				})
			} else {
				smallDisk += diskBytes
				smallLog += logBytes
				smallCount++
			}
			s.reportProgress(childPath)

		default:
			// Sockets, FIFOs and device nodes hold no user data and can vanish
			// between readdir and lstat; counting them would only add noise.
		}
	}

	record.disk = disk
	record.log = log
	record.files = files
	record.smallDisk = smallDisk
	record.smallLog = smallLog
	record.smallCount = smallCount
}

// addHardLink records one path of a hard-linked inode for the post-walk
// resolution. Workers append concurrently, so the slice has its own mutex.
func (s *treeScanner) addHardLink(occurrence hardOccurrence) {
	s.hardMu.Lock()
	s.hardLinks = append(s.hardLinks, occurrence)
	s.hardMu.Unlock()
}

// resolveHardLinks counts every hard-linked inode exactly once. Paths are
// sorted first and the smallest path wins, so the counted directory does not
// depend on traversal order; the losers only bump HardLinkedDeduped.
//
// Called after all workers are done, i.e. single-threaded.
func (s *treeScanner) resolveHardLinks() {
	if len(s.hardLinks) == 0 {
		return
	}
	sort.Slice(s.hardLinks, func(i, j int) bool {
		if s.hardLinks[i].key != s.hardLinks[j].key {
			return s.hardLinks[i].key.dev < s.hardLinks[j].key.dev ||
				(s.hardLinks[i].key.dev == s.hardLinks[j].key.dev && s.hardLinks[i].key.ino < s.hardLinks[j].key.ino)
		}
		return s.hardLinks[i].path < s.hardLinks[j].path
	})

	var total int64
	var previous fileKey
	first := true
	for _, occurrence := range s.hardLinks {
		if !first && occurrence.key == previous {
			total++
			continue
		}
		first = false
		previous = occurrence.key
		s.attributeHardLink(occurrence)
	}
	if total > 0 {
		s.hardLinked.Add(total)
	}
}

// attributeHardLink adds a hard-linked file to its directory record after the
// walk. The record still holds its sums and childNodes; finalization happens
// later, so the numbers stay consistent.
func (s *treeScanner) attributeHardLink(occurrence hardOccurrence) {
	s.files.Add(1)
	record := occurrence.parent
	record.disk += occurrence.disk
	record.log += occurrence.log
	record.files++
	s.considerLarge(filepath.Base(occurrence.path), occurrence.path, occurrence.disk)
	if occurrence.disk >= s.opts.MinKeepBytes {
		record.childNodes = append(record.childNodes, &Node{
			Name:      filepath.Base(occurrence.path),
			Path:      occurrence.path,
			DiskBytes: occurrence.disk,
			LogBytes:  occurrence.log,
			FileCount: 1,
		})
		return
	}
	record.smallDisk += occurrence.disk
	record.smallLog += occurrence.log
	record.smallCount++
}

// finalizeDir turns a record into a Node: recurse into the subdirectories first,
// aggregate and sort this directory's children, and return the node. Running it
// once, after the walk, is what keeps the result deterministic.
func (s *treeScanner) finalizeDir(record *dirRecord) *Node {
	node := record.node
	disk, log, files := record.disk, record.log, record.files
	children := record.childNodes // file nodes: this worker's, plus hard-link winners
	partial := record.partial

	for _, childRecord := range record.childRecords {
		if !childRecord.scanned {
			partial = true
			continue // cancellation dropped this directory's job
		}
		child := s.finalizeDir(childRecord)
		disk += child.DiskBytes
		log += child.LogBytes
		files += child.FileCount
		if child.Partial {
			partial = true
		}
		children = append(children, child)
	}

	node.DiskBytes = disk
	node.LogBytes = log
	node.FileCount = files
	node.Partial = partial
	node.Children = aggregateChildren(children, s.opts.MinKeepBytes, record.smallDisk, record.smallLog, record.smallCount)
	return node
}

// aggregateChildren keeps the big children (at most maxChildren, largest
// first) and folds everything else into one synthetic node so the treemap and
// the bridge payload stay small.
//
// The comparator is a total order (DiskBytes desc, Name asc, Path asc) because
// the input order depends on which goroutine saw which directory first: without
// the name tie-break, two scans of the same tree could produce different JSON.
func aggregateChildren(children []*Node, minKeep, smallDisk, smallLog, smallCount int64) []*Node {
	if len(children) == 0 && smallCount == 0 {
		return nil
	}
	sort.Slice(children, func(i, j int) bool {
		left, right := children[i], children[j]
		if left.DiskBytes != right.DiskBytes {
			return left.DiskBytes > right.DiskBytes
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.Path < right.Path
	})

	kept := make([]*Node, 0, len(children)+1)
	folded := smallCount
	othersDisk, othersLog, othersCount := smallDisk, smallLog, smallCount
	for _, child := range children {
		if len(kept) < maxChildren && child.DiskBytes >= minKeep {
			kept = append(kept, child)
			continue
		}
		othersDisk += child.DiskBytes
		othersLog += child.LogBytes
		othersCount += child.FileCount
		folded++
	}
	if folded > 0 {
		kept = append(kept, &Node{
			Name:      fmt.Sprintf("…其它 %d 项", folded),
			DiskBytes: othersDisk,
			LogBytes:  othersLog,
			FileCount: othersCount,
		})
	}
	return kept
}

// reportProgress is called once per counted entry; the throttle decides
// whether the caller hears about it. The callback runs under progressMu so two
// workers can never enter it at the same time.
func (s *treeScanner) reportProgress(path string) {
	if s.opts.Progress == nil {
		return
	}
	files := s.files.Load()
	now := time.Now()
	s.progressMu.Lock()
	defer s.progressMu.Unlock()
	if files-s.lastReportFiles < progressEveryFiles && now.Sub(s.lastReportAt) < progressEvery {
		return
	}
	s.lastReportFiles = files
	s.lastReportAt = now
	s.opts.Progress(files, path)
}

// considerLarge remembers every file above the threshold; the list is sorted
// deterministically and cut to maxLargeFiles when the scan ends. Keeping the
// full tail is cheap (hundreds of paths) and avoids tie-break drift at the
// 50-entry boundary.
func (s *treeScanner) considerLarge(name, path string, diskBytes int64) {
	if diskBytes <= largeFileThreshold {
		return
	}
	s.largeMu.Lock()
	s.large = append(s.large, LargeFile{Name: name, Path: path, SizeBytes: diskBytes})
	s.largeMu.Unlock()
}

func (s *treeScanner) markCanceled() {
	s.canceled.Store(true)
}

func (s *treeScanner) restrict(path string, err error) {
	reason := scanReason(err)
	s.tallyMu.Lock()
	if _, exists := s.restricted[path]; !exists {
		s.restricted[path] = reason
	}
	s.tallyMu.Unlock()
}

func (s *treeScanner) exclude(path, reason string) {
	s.tallyMu.Lock()
	if _, exists := s.excluded[path]; !exists {
		s.excluded[path] = reason
	}
	s.tallyMu.Unlock()
}

// buildStats snapshots the shared counters into the public result.
func (s *treeScanner) buildStats(ctx context.Context, start time.Time) *ScanStats {
	stats := &ScanStats{
		Files:             s.files.Load(),
		Dirs:              s.dirs.Load(),
		HardLinkedDeduped: s.hardLinked.Load(),
		Elapsed:           time.Since(start),
		Canceled:          s.canceled.Load() || ctx.Err() != nil,
	}
	stats.Restricted = s.tallySnapshot(s.restricted)
	stats.Excluded = s.tallySnapshot(s.excluded)
	stats.Partial = len(stats.Restricted) > 0
	stats.LargeFiles = s.largeSnapshot()
	return stats
}

func (s *treeScanner) tallySnapshot(tally map[string]string) []Restriction {
	s.tallyMu.Lock()
	entries := make([]Restriction, 0, len(tally))
	for path, reason := range tally {
		entries = append(entries, Restriction{Path: path, Reason: reason})
	}
	s.tallyMu.Unlock()
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries
}

func (s *treeScanner) largeSnapshot() []LargeFile {
	s.largeMu.Lock()
	files := append([]LargeFile(nil), s.large...)
	s.largeMu.Unlock()
	if len(files) == 0 {
		return nil
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].SizeBytes != files[j].SizeBytes {
			return files[i].SizeBytes > files[j].SizeBytes
		}
		return files[i].Path < files[j].Path
	})
	return files
}

// scanReason turns a filesystem error into one short line the UI can show.
func scanReason(err error) string {
	if err == nil {
		return "未知错误"
	}
	if errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.EPERM) {
		return "权限不足"
	}
	reason := strings.TrimSpace(err.Error())
	if index := strings.IndexByte(reason, '\n'); index >= 0 {
		reason = reason[:index]
	}
	return reason
}

// allocatedBytes returns the space a file actually occupies, st_blocks*512.
// Sparse files and clones report less here than their st_size, which is what
// the analyzer wants: the question is disk usage, not logical length.
func allocatedBytes(stat *syscall.Stat_t) int64 {
	if stat.Blocks <= 0 {
		return 0
	}
	return int64(stat.Blocks) * 512
}

// rootName is the display name for the scanned root ("/" stays "/").
func rootName(root string) string {
	if root == "/" {
		return "/"
	}
	return filepath.Base(root)
}
