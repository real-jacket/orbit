package analyze

// This file is the seam between the in-process scanner (scan.go) and the
// treemap payload the page draws. One scan per root, kept in memory, so that
// drilling into a directory is a map lookup instead of another seven-second
// walk — the old design re-ran `mo analyze` for every level.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// scanTimeout bounds one full scan. A home directory with 700k files takes
// about twelve seconds; ten minutes only exists so that a pathological mount
// cannot leave the page "扫描中" forever.
const scanTimeout = 10 * time.Minute

// Progress is one `onNativeAnalyzeProgress` payload.
type Progress struct {
	ScanID  string  `json:"scanId"`
	Files   int64   `json:"files"`
	Seconds float64 `json:"seconds"`
	Current string  `json:"current,omitempty"`
}

// Analyzer owns the scanned tree of one root at a time.
type Analyzer struct {
	mu       sync.Mutex
	scanID   string
	rootPath string
	tree     *Node
	stats    *ScanStats
	running  bool
	cancel   context.CancelFunc
	done     chan struct{}
	started  time.Time
	seq      int64
	sink     func(Progress)
}

// Shared is the analyzer the bridge and the push layer use.
var Shared = NewAnalyzer()

// NewAnalyzer returns an analyzer with no scan in it.
func NewAnalyzer() *Analyzer { return &Analyzer{} }

// SetProgressSink installs the function that receives scan progress. Passing
// nil stops progress reporting.
func (a *Analyzer) SetProgressSink(sink func(Progress)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sink = sink
}

// Tree returns the tree for path, scanning it when the current tree does not
// cover it. When the tree already covers path, the matching node comes back
// without touching the disk. path may be empty, which means the home directory.
func (a *Analyzer) Tree(path string) (*Node, *ScanStats, string, error) {
	target := absolutize(path)
	if target == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, nil, "", fmt.Errorf("无法确定家目录路径：%w", err)
		}
		target = home
	}
	target = filepath.Clean(target)

	for {
		a.mu.Lock()
		running, done := a.running, a.done
		rootPath, tree, stats, scanID := a.rootPath, a.tree, a.stats, a.scanID
		a.mu.Unlock()

		if running && done != nil {
			<-done
			continue
		}
		if tree != nil && covers(rootPath, target) {
			if node := Find(tree, target); node != nil {
				return node, stats, scanID, nil
			}
		}
		break
	}

	return a.scan(target)
}

// scan starts one walk and waits for it (up to scanTimeout).
func (a *Analyzer) scan(target string) (*Node, *ScanStats, string, error) {
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel() // a different root was requested; the newest click wins
	}
	a.seq++
	scanID := "s-" + strconv.FormatInt(a.seq, 10)
	ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
	done := make(chan struct{})
	sink := a.sink
	a.scanID = scanID
	a.rootPath = target
	a.tree = nil
	a.stats = nil
	a.running = true
	a.cancel = cancel
	a.done = done
	a.started = time.Now()
	a.mu.Unlock()

	startedAt := time.Now()
	var progressMu sync.Mutex
	var lastProgress time.Time
	go func() {
		defer close(done)
		tree, stats, err := ScanTree(ctx, ScanOptions{
			Root:         target,
			MinKeepBytes: defaultMinKeepBytes,
			Progress: func(files int64, current string) {
				if sink == nil {
					return
				}
				// ScanTree can call back every 2000 files, which is dozens of
				// times a second; the page only needs a few updates a second.
				progressMu.Lock()
				if files > 0 && time.Since(lastProgress) < 200*time.Millisecond {
					progressMu.Unlock()
					return
				}
				lastProgress = time.Now()
				progressMu.Unlock()
				sink(Progress{
					ScanID:  scanID,
					Files:   files,
					Seconds: round2(time.Since(startedAt).Seconds()),
					Current: current,
				})
			},
		})
		_ = err

		a.mu.Lock()
		if a.scanID == scanID {
			a.tree = tree
			a.stats = stats
			a.running = false
			a.cancel = nil
		}
		a.mu.Unlock()
		cancel()
	}()

	select {
	case <-done:
	case <-time.After(scanTimeout + 5*time.Second):
		return nil, nil, scanID, fmt.Errorf("扫描超时（%s）", scanTimeout)
	}

	a.mu.Lock()
	tree, stats := a.tree, a.stats
	a.mu.Unlock()
	if tree == nil || stats == nil {
		return nil, nil, scanID, fmt.Errorf("扫描 %s 失败", target)
	}
	return tree, stats, scanID, nil
}

// Cancel stops the running scan. The partial tree stays available.
func (a *Analyzer) Cancel() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel == nil {
		return false
	}
	a.cancel()
	return true
}

// Root reports the directory the current tree was scanned from.
func (a *Analyzer) Root() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.rootPath
}

// Status reports the scan the analyzer is holding.
func (a *Analyzer) Status() (scanID string, running bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.scanID, a.running
}

// covers reports whether target is root itself or lies below it.
func covers(root, target string) bool {
	if root == "" {
		return false
	}
	if root == target {
		return true
	}
	return strings.HasPrefix(target, root+string(filepath.Separator))
}

// payloadForNode turns one node of the in-memory tree into the payload the page
// draws.
func payloadForNode(node *Node, stats *ScanStats, scanID, scanRoot, requested string) Payload {
	home, _ := os.UserHomeDir()
	total := node.DiskBytes
	if total <= 0 {
		total = node.LogBytes
	}

	tiles := make([]Tile, 0, len(node.Children))
	aggregatedCount, aggregatedBytes := 0, int64(0)
	for _, child := range node.Children {
		if child.Path == "" {
			aggregatedCount += syntheticCount(child.Name)
			aggregatedBytes += child.DiskBytes
			continue
		}
		tile := tileFor(child, total, home)
		tiles = append(tiles, tile)
	}

	// The page draws the rectangles left to right in payload order, exactly as
	// it did for `mo`'s output, so the strip layout stays where it was.
	x := 0.0
	for i := range tiles {
		width := tiles[i].Pct
		if width < 6 {
			width = 6
		}
		tiles[i].X, tiles[i].Y, tiles[i].W, tiles[i].H = round1(x), 0, round1(width), 100
		x += width
	}

	largeFiles := make([]LargeFileTile, 0, len(stats.LargeFiles))
	for _, file := range stats.LargeFiles {
		largeFiles = append(largeFiles, LargeFileTile{
			Name:   file.Name,
			Path:   relativize(file.Path, home),
			SizeGb: plainGB(file.SizeBytes),
		})
	}

	payload := Payload{
		Treemap2D:      tiles,
		Tiles:          tiles,
		Insights:       []Tile{},
		Source:         "orbit scan",
		ScanID:         scanID,
		Root:           relativize(scanRoot, home),
		Path:           relativize(node.Path, home),
		TotalGb:        plainGB(total),
		TotalSizeGb:    plainGB(total),
		TotalLogicalGb: plainGB(node.LogBytes),
		FileCount:      node.FileCount,
		ScanStatus:     scanStatus(stats),
		Partial:        node.Partial,
		Restricted:     restrictionInfos(stats.Restricted, home),
		Excluded:       restrictionInfos(stats.Excluded, home),
		Aggregated:     &Aggregated{Count: aggregatedCount, SizeGb: plainGB(aggregatedBytes)},
		LargeFiles:     largeFiles,
		Volume:         volumeInfo(node.Path),
	}
	if requested != "" {
		payload.Path = relativize(requested, home)
	}
	return payload
}

// tileFor converts one child node into a treemap rectangle.
func tileFor(node *Node, basis int64, home string) Tile {
	var pct float64
	if basis > 0 {
		pct = float64(node.DiskBytes) / float64(basis) * 100
	}
	color := "linear-gradient(145deg, #0284c7, #0c4a6e)"
	if node.Dir {
		color = "linear-gradient(145deg, #d97706, #78350f)"
	}
	return Tile{
		Name:       node.Name,
		Path:       relativize(node.Path, home),
		SizeGb:     plainGB(node.DiskBytes),
		DiskGb:     plainGB(node.DiskBytes),
		LogicalGb:  plainGB(node.LogBytes),
		Pct:        round1(pct),
		Color:      color,
		Files:      int(node.FileCount),
		IsDir:      node.Dir,
		Partial:    node.Partial,
		ScanStatus: "complete",
		Children:   []Tile{},
	}
}

// syntheticCount reads the "N" of a "…其它 N 项" node name, falling back to 1.
func syntheticCount(name string) int {
	digits := strings.Builder{}
	for _, r := range name {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	if n, err := strconv.Atoi(digits.String()); err == nil && n > 0 {
		return n
	}
	return 1
}

// restrictionInfos relativizes and shortens restriction lists for the page.
func restrictionInfos(list []Restriction, home string) []RestrictionInfo {
	if len(list) == 0 {
		return nil
	}
	out := make([]RestrictionInfo, 0, len(list))
	for _, item := range list {
		out = append(out, RestrictionInfo{
			Path:   relativize(item.Path, home),
			Reason: item.Reason,
		})
	}
	return out
}

// scanStatus collapses a scan's flags into the three states the page knows.
func scanStatus(stats *ScanStats) string {
	switch {
	case stats == nil:
		return "ok"
	case stats.Canceled:
		return "canceled"
	case stats.Partial:
		return "partial"
	default:
		return "ok"
	}
}

// volumeInfo reads the volume a path lives on.
func volumeInfo(path string) *Volume {
	if path == "" {
		return nil
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil
	}
	total := float64(stat.Blocks) * float64(stat.Bsize)
	free := float64(stat.Bavail) * float64(stat.Bsize)
	return &Volume{
		TotalGb: plainGB(int64(total)),
		FreeGb:  plainGB(int64(free)),
		UsedGb:  plainGB(int64(total - free)),
	}
}
