package fsread

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestBlockedReadCanBeCanceledAndConsumedLater(t *testing.T) {
	path := t.TempDir()
	read := &request{done: make(chan struct{})}
	reads.Lock()
	reads.pending[path] = read
	reads.Unlock()
	t.Cleanup(func() {
		reads.Lock()
		delete(reads.pending, path)
		reads.Unlock()
	})
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(10*time.Millisecond, cancel)
	if _, err := ReadDir(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled read = %v", err)
	}
	read.deadline = time.Now().Add(-time.Second)
	if _, err := ReadDir(context.Background(), path); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timed-out read = %v", err)
	}
	reads.Lock()
	if reads.pending[path] != read {
		t.Fatal("retry started a duplicate filesystem call")
	}
	reads.Unlock()
	read.entries, read.err = os.ReadDir(path)
	close(read.done)
	if entries, err := ReadDir(context.Background(), path); err != nil || len(entries) != 0 {
		t.Fatalf("finished read = %v, %v", entries, err)
	}
}
