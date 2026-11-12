package scanner

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestBuildConfigRejectsNonregularPathsWithoutBlocking(t *testing.T) {
	for _, kind := range []string{"fifo", "directory", "fifo symlink", "directory symlink"} {
		t.Run(kind, func(t *testing.T) {
			home := fixtureHome(t)
			path := filepath.Join(home, ".config/mole/purge_paths")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "directory":
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			case "fifo symlink":
				target := filepath.Join(home, "path-pipe")
				if err := syscall.Mkfifo(target, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "directory symlink":
				target := filepath.Join(home, "path-directory")
				if err := os.Mkdir(target, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			default:
				if err := syscall.Mkfifo(path, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			start := time.Now()
			snapshot := (&Scanner{home: home}).PurgeRoots()
			if time.Since(start) > 2*purgeConfigReadBudget {
				t.Fatal("a special path config blocked beyond the read budget")
			}
			if !snapshot.Custom || snapshot.Error == "" || len(snapshot.AbsolutePaths()) != 0 || len(snapshot.Warnings) != 1 {
				t.Fatalf("a nonregular saved config must fail closed: %+v", snapshot)
			}
			if snapshot.Warnings[0]["kind"] != "config" || !strings.Contains(snapshot.Warnings[0]["reason"].(string), "普通文件") {
				t.Fatalf("the special file must be rejected without waiting for a writer: %+v", snapshot)
			}
			if _, err := os.Lstat(path); err != nil {
				t.Fatalf("reading the saved configuration changed its file: %v", err)
			}
		})
	}
}

func TestPurgeConfigReadAllowsRegularFileSymlinks(t *testing.T) {
	home := fixtureHome(t)
	target := filepath.Join(home, "saved-purge-paths")
	writeFile(t, target, "~/Work\n")
	path := filepath.Join(home, ".config/mole/purge_paths")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	content, present, err := readPurgePathConfig(path)
	if err != nil || !present || content != "~/Work\n" {
		t.Fatalf("a saved scope linked to a regular file must remain readable: %q, %v, %v", content, present, err)
	}
}

func TestPurgeConfigReadTimeoutReusesPendingOperationAndReloadsAfterCompletion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "purge_paths")
	read := &purgeConfigRead{done: make(chan struct{}), deadline: time.Now().Add(-time.Second)}
	purgeConfigReads.Lock()
	purgeConfigReads.pending[path] = read
	purgeConfigReads.Unlock()
	t.Cleanup(func() { purgeConfigReads.Lock(); delete(purgeConfigReads.pending, path); purgeConfigReads.Unlock() })
	for i := 0; i < 3; i++ {
		content, present, err := readPurgePathConfig(path)
		if !errors.Is(err, context.DeadlineExceeded) || !present || content != "" {
			t.Fatalf("an unavailable pending config must stay an empty saved scope: %q, %v, %v", content, present, err)
		}
		purgeConfigReads.Lock()
		sameOperation := purgeConfigReads.pending[path] == read
		purgeConfigReads.Unlock()
		if !sameOperation {
			t.Fatal("retry started another blocked filesystem operation")
		}
	}
	read.content, read.present = "~/Old\n", true
	close(read.done)
	content, present, err := readPurgePathConfig(path)
	if err != nil || !present || content != "~/Old\n" {
		t.Fatalf("completed operation was not consumed: %q, %v, %v", content, present, err)
	}
	for _, selected := range []string{"~/New", "~/ChangedAgain"} {
		writeFile(t, path, selected+"\n")
		content, present, err = readPurgePathConfig(path)
		if err != nil || !present || content != selected+"\n" {
			t.Fatalf("a later scan reused a completed saved scope: %q, %v, %v", content, present, err)
		}
		purgeConfigReads.Lock()
		retained := purgeConfigReads.pending[path] != nil
		purgeConfigReads.Unlock()
		if retained {
			t.Fatal("a completed config read retained its pending operation")
		}
	}
}

func TestPurgeConfigReadCancellationDoesNotStartAnOperation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "purge_paths")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	content, present, err := readPurgePathConfigContext(ctx, path)
	if !errors.Is(err, context.Canceled) || !present || content != "" {
		t.Fatalf("a canceled saved-scope read must fail closed: %q, %v, %v", content, present, err)
	}
	purgeConfigReads.Lock()
	started := purgeConfigReads.pending[path] != nil
	purgeConfigReads.Unlock()
	if started {
		t.Fatal("a canceled caller started filesystem work")
	}
}

func TestPurgeConfigReadKeepsOneMegabyteLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "purge_paths")
	for _, size := range []int{1 << 20, (1 << 20) + 1} {
		if err := os.WriteFile(path, bytes.Repeat([]byte(" "), size), 0o600); err != nil {
			t.Fatal(err)
		}
		content, present, err := readPurgePathConfig(path)
		if !present {
			t.Fatalf("bounded empty config = %d bytes, %v, %v", len(content), present, err)
		}
		if size == 1<<20 && (err != nil || len(content) != size) {
			t.Fatalf("a config at the byte limit must be accepted: %d bytes, %v", len(content), err)
		}
		if size > 1<<20 && (content != "" || err == nil || !strings.Contains(err.Error(), "1 MB")) {
			t.Fatalf("oversized config must fail closed: %v", err)
		}
	}
}
