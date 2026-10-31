package analyze

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// scanTestWrite creates a file of size bytes, including parent directories.
func scanTestWrite(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), size), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

// scanTestDisk is the allocated size of one file, read the same way scan.go
// reads it.
func scanTestDisk(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat(%s): %v", path, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("Lstat(%s): 不是 syscall.Stat_t", path)
	}
	return allocatedBytes(stat)
}

func scanTestTree(t *testing.T, root string, minKeepBytes int64) (*Node, *ScanStats) {
	t.Helper()
	node, stats, err := ScanTree(context.Background(), ScanOptions{Root: root, MinKeepBytes: minKeepBytes})
	if err != nil {
		t.Fatalf("ScanTree(%q): %v", root, err)
	}
	if node == nil || stats == nil {
		t.Fatalf("ScanTree(%q) 返回 nil", root)
	}
	return node, stats
}

func scanTestChild(node *Node, name string) *Node {
	for _, child := range node.Children {
		if child.Name == name {
			return child
		}
	}
	return nil
}

// TestScanTreeCountsFilesAndSkipsSymlinks covers the basic contract: files and
// directories are counted, allocated size is st_blocks*512, logical size is
// st_size, and a symlink contributes nothing on its own.
func TestScanTreeCountsFilesAndSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	aPath := filepath.Join(root, "a.bin")
	bPath := filepath.Join(root, "sub", "b.bin")
	cPath := filepath.Join(root, "sub", "deep", "c.bin")
	scanTestWrite(t, aPath, 1000)
	scanTestWrite(t, bPath, 2000)
	scanTestWrite(t, cPath, 3000)

	if err := os.Symlink(aPath, filepath.Join(root, "link-file")); err != nil {
		t.Fatalf("Symlink(file): %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "sub"), filepath.Join(root, "link-dir")); err != nil {
		t.Fatalf("Symlink(dir): %v", err)
	}

	wantDisk := scanTestDisk(t, aPath) + scanTestDisk(t, bPath) + scanTestDisk(t, cPath)
	node, stats := scanTestTree(t, root, 1) // 1 byte keeps even the smallest files as nodes

	if stats.Files != 3 {
		t.Fatalf("Files = %d, want 3（符号链接不能计入）", stats.Files)
	}
	if stats.Dirs != 3 {
		t.Fatalf("Dirs = %d, want 3（root/sub/deep，符号链接目录不能计入）", stats.Dirs)
	}
	if node.DiskBytes != wantDisk {
		t.Fatalf("DiskBytes = %d, want %d", node.DiskBytes, wantDisk)
	}
	if node.LogBytes != 6000 {
		t.Fatalf("LogBytes = %d, want 6000", node.LogBytes)
	}
	if node.FileCount != 3 {
		t.Fatalf("FileCount = %d, want 3", node.FileCount)
	}
	if child := scanTestChild(node, "link-file"); child != nil {
		t.Fatalf("符号链接文件被计为子节点: %+v", child)
	}
	if child := scanTestChild(node, "link-dir"); child != nil {
		t.Fatalf("符号链接目录被计为子节点: %+v", child)
	}

	deep := Find(node, cPath)
	if deep == nil {
		t.Fatalf("Find(%q) = nil", cPath)
	}
	if deep.LogBytes != 3000 || deep.DiskBytes != scanTestDisk(t, cPath) {
		t.Fatalf("Find(%q) = log %d / disk %d, want 3000 / %d", cPath, deep.LogBytes, deep.DiskBytes, scanTestDisk(t, cPath))
	}
	if Find(node, root) != node {
		t.Fatalf("Find(root) 没有返回根节点")
	}
	if Find(node, filepath.Join(root, "missing")) != nil {
		t.Fatalf("Find(不存在的路径) 返回了非 nil")
	}
	if stats.Partial || node.Partial || len(stats.Restricted) != 0 {
		t.Fatalf("干净目录树不应是 Partial: stats=%+v node.Partial=%v", stats.Restricted, node.Partial)
	}
}

// TestScanTreeReportsAllocatedBytes pins the difference between DiskBytes and
// LogBytes without depending on sparse-file support: a 1-byte file still eats a
// whole filesystem block, so DiskBytes must be st_blocks*512 and not st_size.
func TestScanTreeReportsAllocatedBytes(t *testing.T) {
	root := t.TempDir()
	tinyPath := filepath.Join(root, "tiny.bin")
	scanTestWrite(t, tinyPath, 1)
	exactPath := filepath.Join(root, "exact.bin")
	scanTestWrite(t, exactPath, 5000)

	tinyDisk := scanTestDisk(t, tinyPath)
	exactDisk := scanTestDisk(t, exactPath)
	if tinyDisk <= 1 || exactDisk <= 5000 {
		t.Skipf("文件系统没有按块报告已分配大小（tiny=%d, exact=%d）", tinyDisk, exactDisk)
	}

	node, stats := scanTestTree(t, root, 1)
	if stats.Files != 2 {
		t.Fatalf("Files = %d, want 2", stats.Files)
	}
	if node.LogBytes != 5001 {
		t.Fatalf("LogBytes = %d, want 5001（st_size 之和）", node.LogBytes)
	}
	if node.DiskBytes != tinyDisk+exactDisk {
		t.Fatalf("DiskBytes = %d, want %d（st_blocks*512 之和）", node.DiskBytes, tinyDisk+exactDisk)
	}
	tiny := scanTestChild(node, "tiny.bin")
	if tiny == nil {
		t.Fatalf("tiny.bin 没有出现在子节点里")
	}
	if tiny.DiskBytes != tinyDisk || tiny.LogBytes != 1 {
		t.Fatalf("tiny.bin = disk %d / log %d, want %d / 1", tiny.DiskBytes, tiny.LogBytes, tinyDisk)
	}
	if tiny.DiskBytes <= tiny.LogBytes {
		t.Fatalf("tiny.bin DiskBytes(%d) 应大于 LogBytes(%d)，否则报的是逻辑大小", tiny.DiskBytes, tiny.LogBytes)
	}
}

// TestScanTreeUnreadableDirectoryIsRestricted makes sure a permission error
// downgrades the scan instead of aborting or panicking.
func TestScanTreeUnreadableDirectoryIsRestricted(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root 无视目录权限，构造不出不可读目录")
	}
	root := t.TempDir()
	scanTestWrite(t, filepath.Join(root, "readable.bin"), 4096)
	locked := filepath.Join(root, "locked")
	scanTestWrite(t, filepath.Join(locked, "hidden.bin"), 8192)
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatalf("Chmod(%s): %v", locked, err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	node, stats := scanTestTree(t, root, 1)

	if stats.Files != 1 {
		t.Fatalf("Files = %d, want 1（不可读目录里的文件不能计入）", stats.Files)
	}
	if len(stats.Restricted) != 1 {
		t.Fatalf("Restricted = %+v, want 1 条", stats.Restricted)
	}
	if stats.Restricted[0].Path != locked {
		t.Fatalf("Restricted[0].Path = %q, want %q", stats.Restricted[0].Path, locked)
	}
	if stats.Restricted[0].Reason != "权限不足" {
		t.Fatalf("Restricted[0].Reason = %q, want 权限不足", stats.Restricted[0].Reason)
	}
	if !stats.Partial || !node.Partial {
		t.Fatalf("Partial 未传播: stats.Partial=%v node.Partial=%v", stats.Partial, node.Partial)
	}
}

// TestScanTreeDedupesHardLinks counts an inode once even when it has several
// names, which matters for Time Machine-style trees and Homebrew.
func TestScanTreeDedupesHardLinks(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "big.bin")
	scanTestWrite(t, path, 8192)
	if err := os.Link(path, filepath.Join(root, "dup1.bin")); err != nil {
		t.Skipf("文件系统不支持硬链接: %v", err)
	}
	disk := scanTestDisk(t, path)

	node, stats := scanTestTree(t, root, 1)

	if stats.Files != 1 {
		t.Fatalf("Files = %d, want 1（同一 inode 只计一次）", stats.Files)
	}
	if stats.HardLinkedDeduped != 1 {
		t.Fatalf("HardLinkedDeduped = %d, want 1", stats.HardLinkedDeduped)
	}
	if node.DiskBytes != disk {
		t.Fatalf("DiskBytes = %d, want %d", node.DiskBytes, disk)
	}
	if node.FileCount != 1 {
		t.Fatalf("FileCount = %d, want 1", node.FileCount)
	}
	if child := scanTestChild(node, "dup1.bin"); child != nil {
		t.Fatalf("硬链接副本被计为子节点: %+v", child)
	}
}

// TestScanTreeAggregatesSmallChildren checks the "…其它 N 项" contract: one
// big child stays separate, the small ones sum into one synthetic node.
func TestScanTreeAggregatesSmallChildren(t *testing.T) {
	root := t.TempDir()
	smallPaths := []string{
		filepath.Join(root, "a.bin"),
		filepath.Join(root, "b.bin"),
		filepath.Join(root, "c.bin"),
	}
	for _, path := range smallPaths {
		scanTestWrite(t, path, 1024)
	}
	bigPath := filepath.Join(root, "big.bin")
	scanTestWrite(t, bigPath, 5<<20)

	node, stats := scanTestTree(t, root, 1<<20)

	if stats.Files != 4 {
		t.Fatalf("Files = %d, want 4", stats.Files)
	}
	if len(node.Children) != 2 {
		t.Fatalf("Children = %d, want 2（big.bin + 其它）: %+v", len(node.Children), node.Children)
	}
	if node.Children[0].Name != "big.bin" {
		t.Fatalf("Children[0].Name = %q, want big.bin（按 DiskBytes 降序）", node.Children[0].Name)
	}
	if node.Children[0].DiskBytes < 5<<20 {
		t.Fatalf("big.bin 的 DiskBytes = %d, want >= %d", node.Children[0].DiskBytes, 5<<20)
	}

	others := node.Children[1]
	if !strings.Contains(others.Name, "其它 3 项") {
		t.Fatalf("聚合节点名 = %q, want 含「其它 3 项」", others.Name)
	}
	if others.Dir {
		t.Fatalf("聚合节点必须是文件型")
	}
	if others.Path != "" {
		t.Fatalf("聚合节点不应带路径，got %q", others.Path)
	}
	if others.FileCount != 3 {
		t.Fatalf("聚合节点 FileCount = %d, want 3", others.FileCount)
	}

	wantDisk := scanTestDisk(t, bigPath) + scanTestDisk(t, smallPaths[0])*3
	if node.DiskBytes != wantDisk {
		t.Fatalf("DiskBytes = %d, want %d", node.DiskBytes, wantDisk)
	}
	if wantOthers := scanTestDisk(t, smallPaths[0]) * 3; others.DiskBytes != wantOthers {
		t.Fatalf("聚合节点 DiskBytes = %d, want %d", others.DiskBytes, wantOthers)
	}
}

// TestScanTreeFoldsSmallChildrenByDefault makes sure a zero MinKeepBytes means
// the 1 MB default rather than "keep everything".
func TestScanTreeFoldsSmallChildrenByDefault(t *testing.T) {
	root := t.TempDir()
	scanTestWrite(t, filepath.Join(root, "small.txt"), 100)

	node, _ := scanTestTree(t, root, 0)
	if len(node.Children) != 1 {
		t.Fatalf("Children = %d, want 1", len(node.Children))
	}
	if !strings.Contains(node.Children[0].Name, "其它 1 项") {
		t.Fatalf("默认阈值下小文件应被折叠，got %q", node.Children[0].Name)
	}
	if Find(node, filepath.Join(root, "small.txt")) != nil {
		t.Fatalf("被折叠的文件不应能被 Find 找到")
	}
}

// TestScanTreeCapsChildrenAtSixty covers the second aggregation reason: too
// many children, not too small children.
func TestScanTreeCapsChildrenAtSixty(t *testing.T) {
	root := t.TempDir()
	const total = 65
	for i := 0; i < total; i++ {
		scanTestWrite(t, filepath.Join(root, fmt.Sprintf("f%02d.bin", i)), 1024)
	}

	node, stats := scanTestTree(t, root, 512)

	if stats.Files != total {
		t.Fatalf("Files = %d, want %d", stats.Files, total)
	}
	if len(node.Children) != maxChildren+1 {
		t.Fatalf("Children = %d, want %d（%d 个保留 + 1 个聚合）", len(node.Children), maxChildren+1, maxChildren)
	}
	last := node.Children[len(node.Children)-1]
	if !strings.Contains(last.Name, "其它 5 项") {
		t.Fatalf("最后一个子节点 = %q, want 含「其它 5 项」", last.Name)
	}
	if last.FileCount != total-maxChildren {
		t.Fatalf("聚合节点 FileCount = %d, want %d", last.FileCount, total-maxChildren)
	}
}

// TestScanTreeExcludesSystemDirectories pins the macOS bookkeeping skip list.
func TestScanTreeExcludesSystemDirectories(t *testing.T) {
	root := t.TempDir()
	excluded := []string{".Spotlight-V100", ".fseventsd", ".Trashes", ".DocumentRevisions-V100"}
	for _, name := range excluded {
		scanTestWrite(t, filepath.Join(root, name, "junk.bin"), 2048)
	}
	keepPath := filepath.Join(root, "keep.bin")
	scanTestWrite(t, keepPath, 1024)

	node, stats := scanTestTree(t, root, 1)

	if stats.Files != 1 {
		t.Fatalf("Files = %d, want 1（系统目录里的文件不能计入）", stats.Files)
	}
	if stats.Dirs != 1 {
		t.Fatalf("Dirs = %d, want 1（只有根目录；被排除的系统目录不计）", stats.Dirs)
	}
	if len(stats.Excluded) != len(excluded) {
		t.Fatalf("Excluded = %+v, want %d 条", stats.Excluded, len(excluded))
	}
	if node.DiskBytes != scanTestDisk(t, keepPath) {
		t.Fatalf("DiskBytes = %d, want %d", node.DiskBytes, scanTestDisk(t, keepPath))
	}
	for _, name := range excluded {
		if child := scanTestChild(node, name); child != nil {
			t.Fatalf("%s 不应出现在子节点里: %+v", name, child)
		}
	}
}

// TestScanTreeCancelReturnsPartialTree covers both cancellation shapes: an
// already-canceled context, and a cancel triggered from the progress callback.
func TestScanTreeCancelReturnsPartialTree(t *testing.T) {
	t.Run("already canceled", func(t *testing.T) {
		root := t.TempDir()
		scanTestWrite(t, filepath.Join(root, "a.bin"), 1024)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		node, stats, err := ScanTree(ctx, ScanOptions{Root: root})
		if err != nil {
			t.Fatalf("ScanTree 返回错误: %v", err)
		}
		if node == nil || stats == nil {
			t.Fatalf("取消也要返回部分树")
		}
		if !stats.Canceled {
			t.Fatalf("Canceled = false, want true")
		}
		if stats.Files != 0 {
			t.Fatalf("Files = %d, want 0", stats.Files)
		}
	})

	t.Run("cancel from progress", func(t *testing.T) {
		root := t.TempDir()
		const total = 2500
		for i := 0; i < total; i++ {
			scanTestWrite(t, filepath.Join(root, fmt.Sprintf("f%04d.bin", i)), 4096)
		}

		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		node, stats, err := ScanTree(ctx, ScanOptions{
			Root: root,
			Progress: func(scanned int64, _ string) {
				calls++
				if scanned >= 2000 {
					cancel()
				}
			},
		})
		if err != nil {
			t.Fatalf("ScanTree 返回错误: %v", err)
		}
		if !stats.Canceled {
			t.Fatalf("Canceled = false, want true")
		}
		if stats.Files < 2000 || stats.Files >= total {
			t.Fatalf("Files = %d, want [2000, %d)", stats.Files, total)
		}
		if node.FileCount != stats.Files {
			t.Fatalf("node.FileCount = %d, stats.Files = %d（应一致）", node.FileCount, stats.Files)
		}
		if calls > 10 {
			t.Fatalf("进度回调被调用了 %d 次，明显不是按 2000 文件/200ms 节流", calls)
		}
	})
}

// TestScanTreeRejectsBadRoots pins the two hard errors: empty and non-directory.
func TestScanTreeRejectsBadRoots(t *testing.T) {
	if _, _, err := ScanTree(context.Background(), ScanOptions{Root: ""}); err == nil {
		t.Fatalf("空 Root 应返回错误")
	}
	root := t.TempDir()
	path := filepath.Join(root, "file.bin")
	scanTestWrite(t, path, 10)
	if _, _, err := ScanTree(context.Background(), ScanOptions{Root: path}); err == nil {
		t.Fatalf("以文件为 Root 应返回错误")
	}
}

// TestScanApplications is the real-tree benchmark: one walk over
// /Applications, the same tree the analyzer shows first. It is a -short skip so
// unit tests stay fast, but it runs by default so the timing is on record.
func TestScanApplications(t *testing.T) {
	if testing.Short() {
		t.Skip("-short: 跳过真机基准")
	}
	if _, err := os.Stat("/Applications"); err != nil {
		t.Skipf("/Applications 不可用: %v", err)
	}

	start := time.Now()
	node, stats, err := ScanTree(context.Background(), ScanOptions{Root: "/Applications"})
	wall := time.Since(start)
	if err != nil {
		t.Fatalf("ScanTree(/Applications): %v", err)
	}
	if node == nil || stats == nil {
		t.Fatalf("ScanTree(/Applications) 返回 nil")
	}
	if stats.Files < 1000 {
		t.Fatalf("Files = %d, 明显少于真实 /Applications 的文件数", stats.Files)
	}
	// The budget is machine-dependent: a GitHub macOS runner walks the same tree
	// in about 41 s on a cold cache, so only a local run with MOLE_BENCH=1 fails
	// the tight budget. CI still catches a pathological regression.
	budget := 120 * time.Second
	if os.Getenv("MOLE_BENCH") == "1" {
		budget = 15 * time.Second
	}
	if wall > budget {
		t.Fatalf("扫描 /Applications 耗时 %s，超过 %s 预算", wall, budget)
	}
	if node.DiskBytes <= 0 {
		t.Fatalf("DiskBytes = %d, want > 0", node.DiskBytes)
	}
	rate := float64(stats.Files) / wall.Seconds()
	t.Logf("扫描 /Applications: Files=%d Dirs=%d 耗时=%s (%.0f 文件/s) Disk=%.2f GB Log=%.2f GB HardLinkedDeduped=%d Restricted=%d Excluded=%d Partial=%v LargeFiles=%d",
		stats.Files, stats.Dirs, wall.Round(time.Millisecond), rate,
		float64(node.DiskBytes)/(1<<30), float64(node.LogBytes)/(1<<30),
		stats.HardLinkedDeduped, len(stats.Restricted), len(stats.Excluded), stats.Partial, len(stats.LargeFiles))

	if Find(node, "/Applications/Safari.app") == nil {
		t.Logf("提示：/Applications 中没有 Safari.app（或它被折叠），跳过抽查")
	}
}

// TestScanTreeIsDeterministic pins the ordering contract: directories are
// visited by different workers in nondeterministic order, so the tree, the
// aggregated children and the stats must be ordered by value, never by arrival
// order. Serializing twice must give identical bytes.
func TestScanTreeIsDeterministic(t *testing.T) {
	root := t.TempDir()
	// Identical sizes in every directory on purpose: only the name/path
	// tie-break can decide the order.
	for dir := 0; dir < 6; dir++ {
		for file := 0; file < 8; file++ {
			scanTestWrite(t, filepath.Join(root, fmt.Sprintf("d%d", dir), fmt.Sprintf("f%d.bin", file)), (file+1)*1024)
		}
	}
	// More children than maxChildren exercise the synthetic "…其它" node.
	many := filepath.Join(root, "many")
	for file := 0; file < 70; file++ {
		scanTestWrite(t, filepath.Join(many, fmt.Sprintf("m%03d.bin", file)), 100)
	}

	type snapshot struct {
		Tree       *Node
		Files      int64
		Dirs       int64
		Restricted []Restriction
		Excluded   []Restriction
		LargeFiles []LargeFile
	}
	scan := func() []byte {
		t.Helper()
		node, stats := scanTestTree(t, root, defaultMinKeepBytes)
		payload, err := json.Marshal(snapshot{
			Tree:       node,
			Files:      stats.Files,
			Dirs:       stats.Dirs,
			Restricted: stats.Restricted,
			Excluded:   stats.Excluded,
			LargeFiles: stats.LargeFiles,
		})
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		return payload
	}

	first := scan()
	for attempt := 1; attempt <= 3; attempt++ {
		if next := scan(); !bytes.Equal(first, next) {
			t.Fatalf("第 %d 次扫描序列化结果不同（聚合/排序不确定）:\nfirst=%s\nnext =%s", attempt, first, next)
		}
	}
}

// TestScanTreeFanOutTotals checks the pending-counter protocol across many
// sibling directories: every child's FileCount must reach the root exactly
// once, and the root totals must equal the sum of its children.
func TestScanTreeFanOutTotals(t *testing.T) {
	root := t.TempDir()
	const dirs, perDir = 12, 30
	for dir := 0; dir < dirs; dir++ {
		for file := 0; file < perDir; file++ {
			scanTestWrite(t, filepath.Join(root, fmt.Sprintf("d%02d", dir), fmt.Sprintf("f%02d.bin", file)), 1024)
		}
	}

	node, stats := scanTestTree(t, root, 1) // keep every file node
	if want := int64(dirs * perDir); stats.Files != want {
		t.Fatalf("Files = %d, want %d", stats.Files, want)
	}
	if want := int64(dirs + 1); stats.Dirs != want {
		t.Fatalf("Dirs = %d, want %d（含根目录）", stats.Dirs, want)
	}
	if node.FileCount != stats.Files {
		t.Fatalf("node.FileCount = %d, stats.Files = %d", node.FileCount, stats.Files)
	}
	var childFiles, childDisk int64
	for _, child := range node.Children {
		if len(child.Children) != perDir {
			t.Fatalf("%s 有 %d 个子项, want %d", child.Name, len(child.Children), perDir)
		}
		childFiles += child.FileCount
		childDisk += child.DiskBytes
	}
	if childFiles != stats.Files || childDisk != node.DiskBytes {
		t.Fatalf("子项合计 Files=%d Disk=%d，根 Files=%d Disk=%d", childFiles, childDisk, stats.Files, node.DiskBytes)
	}
}

// A root scan must not report a path: the page reads "has path" as "this is a
// drill-down", and a root payload that claimed "~" was thrown away, leaving the
// analyzer showing 0.00 GB while Go had measured 711 GB.
func TestPayloadForNodeLeavesTheRootPathEmpty(t *testing.T) {
	root := &Node{Path: "/Users/me", Dir: true, Partial: false}
	child := &Node{Path: "/Users/me/Documents", Dir: true, Name: "Documents"}
	root.Children = []*Node{child}
	stats := &ScanStats{}

	rootPayload := payloadForNode(root, stats, "scan-1", "/Users/me", "")
	if rootPayload.Path != "" {
		t.Fatalf("root payload path = %q, want empty", rootPayload.Path)
	}
	if rootPayload.TotalSizeGb == 0 && root.DiskBytes > 0 {
		t.Fatal("root payload lost its size")
	}

	drill := payloadForNode(child, stats, "scan-1", "/Users/me", "")
	if drill.Path == "" {
		t.Fatal("drill-down payload lost its path, so the page cannot place the tiles")
	}
}

func TestAnalyzerRefreshDoesNotReuseAnOldOrCanceledTree(t *testing.T) {
	root := t.TempDir()
	scanTestWrite(t, filepath.Join(root, "first.bin"), 4096)
	a := NewAnalyzer()
	_, _, firstID, err := a.Tree(root, false)
	if err != nil {
		t.Fatal(err)
	}
	scanTestWrite(t, filepath.Join(root, "second.bin"), 4096)
	_, _, cachedID, err := a.Tree(root, false)
	if err != nil || cachedID != firstID {
		t.Fatalf("drill lookup should reuse the tree: %s, %v", cachedID, err)
	}
	node, _, freshID, err := a.Tree(root, true)
	if err != nil || freshID == firstID || node.FileCount != 2 {
		t.Fatalf("explicit refresh must walk the disk: %s, %v, %+v", freshID, err, node)
	}
	a.stats.Canceled = true
	_, stats, retryID, err := a.Tree(root, false)
	if err != nil || retryID == freshID || stats.Canceled {
		t.Fatalf("a canceled result cannot masquerade as a complete cached scan: %s, %v", retryID, err)
	}
}

func TestAnalysisDrillScopesFactsAndLargeFiles(t *testing.T) {
	root := "/Users/me"
	child := &Node{Path: root + "/Projects", Dir: true, DiskBytes: 3_000_000_000, LogBytes: 4_000_000_000}
	stats := &ScanStats{Restricted: []Restriction{{Path: root + "/Documents", Reason: "denied"}}, LargeFiles: []LargeFile{
		{Name: "in.bin", Path: child.Path + "/in.bin", SizeBytes: 2_000_000_000},
		{Name: "outside.bin", Path: root + "/Projects-other/outside.bin", SizeBytes: 5_000_000_000},
	}}
	payload := payloadForNode(child, stats, "scan-1", root, child.Path)
	if payload.TotalSizeGb != 3 || payload.TotalLogicalGb != 4 || len(payload.LargeFiles) != 1 || len(payload.Restricted) != 0 {
		t.Fatalf("drill-down facts must describe this directory only: %+v", payload)
	}
	child.DiskBytes = 0
	if got := payloadForNode(child, stats, "scan-1", root, child.Path); got.TotalGb != 0 {
		t.Fatalf("zero allocation cannot fall back to logical bytes: %+v", got)
	}
	if !covers("/", "/Users/me") || covers(root, root+"-other") {
		t.Fatal("root and sibling boundaries are not directory-prefix comparisons")
	}
}
