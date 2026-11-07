package analyze

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeAnalysisFailureReturnsDirectly(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	payload := PayloadFor(missing, true)
	if payload.Error == "" || payload.ScanStatus != "error" || strings.Contains(payload.Error, "安装") {
		t.Fatalf("native failure must return directly, not an install hint: %+v", payload)
	}
}

// A drill-down inside the running scan's root must not cancel it: Tree waits for
// that scan and reuses its tree, while cancelling would restart the same walk.
func TestCancelUnlessCoveringKeepsScanThatCoversTheRequest(t *testing.T) {
	canceled := 0
	a := &Analyzer{running: true, rootPath: "/Users/me", cancel: func() { canceled++ }}
	for _, c := range []struct {
		path    string
		refresh bool
		want    bool
	}{
		{"/Users/me/Library", false, false}, // inside the running root: keep
		{"/Users/me", false, false},         // the root itself: keep
		{"/Users/me/Library", true, true},   // refresh forces a new walk
		{"/Volumes/Backup", false, true},    // outside the root: cancel
		{"/Users/meow", false, true},        // a sibling that shares a prefix
	} {
		before := canceled
		if got := a.CancelUnlessCovering(c.path, c.refresh); got != c.want || (canceled > before) != c.want {
			t.Errorf("CancelUnlessCovering(%q, %v) = %v (cancels %d), want %v", c.path, c.refresh, got, canceled-before, c.want)
		}
	}
	if idle := (&Analyzer{}); idle.CancelUnlessCovering("/x", false) {
		t.Error("nothing running, nothing to cancel")
	}
}
