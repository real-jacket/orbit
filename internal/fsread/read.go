package fsread

import (
	"context"
	"os"
	"sync"
	"time"
)

type request struct {
	done     chan struct{}
	deadline time.Time
	entries  []os.DirEntry
	err      error
}

var reads = struct {
	sync.Mutex
	pending map[string]*request
}{pending: map[string]*request{}}

// ReadDir lets canceled scans finish even when TCC blocks open(2).
// shortcut: the syscall itself cannot be canceled; share one per path until it returns.
func ReadDir(ctx context.Context, path string) ([]os.DirEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reads.Lock()
	read := reads.pending[path]
	if read == nil {
		deadline, _ := ctx.Deadline()
		read = &request{done: make(chan struct{}), deadline: deadline}
		reads.pending[path] = read
		go func() {
			read.entries, read.err = os.ReadDir(path)
			close(read.done)
		}()
	}
	reads.Unlock()
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
			return nil, ctx.Err()
		case <-timeout:
			return nil, context.DeadlineExceeded
		}
	}
	reads.Lock()
	if reads.pending[path] == read {
		delete(reads.pending, path)
	}
	reads.Unlock()
	return read.entries, read.err
}
