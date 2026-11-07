package scanner

import (
	"errors"
	"testing"
	"time"
)

func TestProcessSnapshotReusesDisplayReadsButNotLiveOnes(t *testing.T) {
	calls := 0
	s := &Scanner{processTTL: time.Hour, processProbe: func() (string, error) {
		calls++
		return "node\n", nil
	}}
	for i := 0; i < 3; i++ {
		if _, err := s.processSnapshot(false); err != nil {
			t.Fatalf("processSnapshot(display) failed: %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("display reads ran ps %d times, want 1 within the TTL", calls)
	}
	s.processSnapshot(true)
	s.processSnapshot(true)
	if calls != 3 {
		t.Fatalf("live reads must always run ps: calls = %d, want 3", calls)
	}
	s.psAt = time.Now().Add(-2 * time.Hour)
	s.processSnapshot(false)
	if calls != 4 {
		t.Fatalf("an expired snapshot must be refreshed: calls = %d, want 4", calls)
	}
}

func TestProcessSnapshotWithoutTTLNeverCaches(t *testing.T) {
	calls := 0
	s := &Scanner{processProbe: func() (string, error) {
		calls++
		return "", nil
	}}
	s.processSnapshot(false)
	s.processSnapshot(false)
	if calls != 2 {
		t.Fatalf("a zero TTL must not cache: calls = %d, want 2", calls)
	}
}

func TestProcessSnapshotDoesNotCacheFailures(t *testing.T) {
	fail := true
	calls := 0
	s := &Scanner{processTTL: time.Hour, processProbe: func() (string, error) {
		calls++
		if fail {
			return "", errors.New("ps failed")
		}
		return "node\n", nil
	}}
	if _, err := s.processSnapshot(false); err == nil {
		t.Fatal("processSnapshot must report a failed ps")
	}
	fail = false
	if out, err := s.processSnapshot(false); err != nil || out != "node\n" {
		t.Fatalf("processSnapshot after a failure = %q, %v; want a fresh read", out, err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestAIHandleOpenUsesTheSnapshotOnlyForDisplay(t *testing.T) {
	snapshotCalls, pathCalls := 0, 0
	s := &Scanner{openSnapshot: func(string) (bool, error) {
		snapshotCalls++
		return false, nil
	}}
	s.handleProbe = nil
	if _, err := s.aiHandleOpen("/nonexistent/orbit-handle-test", false); err != nil || snapshotCalls != 1 {
		t.Fatalf("display check: snapshotCalls = %d, err = %v; want the snapshot used once", snapshotCalls, err)
	}
	s.aiHandleOpen("/nonexistent/orbit-handle-test", true)
	if snapshotCalls != 1 {
		t.Fatalf("the live check must not read the snapshot: snapshotCalls = %d", snapshotCalls)
	}
	s.handleProbe = func(string) (bool, error) {
		pathCalls++
		return true, nil
	}
	if open, _ := s.aiHandleOpen("/x", false); !open || pathCalls != 1 || snapshotCalls != 1 {
		t.Fatalf("an injected probe wins over the snapshot: open=%v pathCalls=%d snapshotCalls=%d", open, pathCalls, snapshotCalls)
	}
	if open, _ := s.aiHandleOpen("/x", true); !open || pathCalls != 2 {
		t.Fatalf("live check must ask the probe: open=%v pathCalls=%d", open, pathCalls)
	}
}

func TestScanViewKeepsProbesAndHasItsOwnProcessCache(t *testing.T) {
	calls := 0
	s := &Scanner{home: t.TempDir(), processTTL: time.Hour, processProbe: func() (string, error) {
		calls++
		return "node\n", nil
	}}
	view := s.scanView()
	view.processSnapshot(false)
	view.processSnapshot(false)
	if calls != 1 || view.openSnapshot == nil || view.home != s.home {
		t.Fatalf("scanView: calls = %d, openSnapshot nil = %v, home = %q", calls, view.openSnapshot == nil, view.home)
	}
	if s.openSnapshot != nil {
		t.Fatal("the scanner itself must not carry a display snapshot")
	}
}
