package scanner

import (
	"context"
	"os"
	"syscall"
	"time"
)

// What a listed directory entry is. Symlinks and special files are itemOther:
// they are never followed and never measured.
const (
	itemOther uint8 = iota
	itemFile
	itemDir
)

// dirItem is one directory entry with the attributes a measuring walk needs, so
// the walk never has to stat each file again.
type dirItem struct {
	name     string
	kind     uint8
	bytes    int64 // allocated size on disk; regular files only
	nlink    uint32
	dev, ino uint64 // identify a hard-linked file; filled for regular files
	mtime    int64  // regular files only
}

// listDirStd lists dir with ReadDir and one lstat per file. It is the portable
// path and the fallback for volumes without getattrlistbulk support.
func listDirStd(dir string) ([]dirItem, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	items := make([]dirItem, 0, len(entries))
	for _, entry := range entries {
		item := dirItem{name: entry.Name()}
		if entry.IsDir() {
			item.kind = itemDir
			items = append(items, item)
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			items = append(items, item)
			continue
		}
		item.kind, item.bytes, item.mtime = itemFile, allocatedSize(info), info.ModTime().Unix()
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			item.nlink, item.dev, item.ino = uint32(stat.Nlink), uint64(stat.Dev), stat.Ino
		}
		items = append(items, item)
	}
	return items, nil
}

// listDirGuarded is listDirItems for a walk root: opening a folder macOS has not
// authorized can block on a prompt, so give up on it after a moment. The
// abandoned read finishes on its own.
func listDirGuarded(dir string) ([]dirItem, error) {
	type result struct {
		items []dirItem
		err   error
	}
	done := make(chan result, 1)
	go func() {
		items, err := listDirItems(dir)
		done <- result{items, err}
	}()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case r := <-done:
		return r.items, r.err
	case <-timer.C:
		return nil, context.DeadlineExceeded
	}
}
