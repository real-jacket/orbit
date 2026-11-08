package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMeasurePathFullyScope(t *testing.T) {
	home := fixtureHome(t)
	outside := fixtureDir(t, t.TempDir(), "elsewhere", 4096)
	ok := fixtureDir(t, home, "Library/Application Support/DingTalkMac", 4096)
	writeTestFile(t, filepath.Join(ok, "deep/second.bin"), 8192)
	link := filepath.Join(home, "alias")
	if err := os.Symlink(ok, link); err != nil {
		t.Fatal(err)
	}
	s := &Scanner{home: home}
	for _, tc := range []struct{ name, path, want string }{
		{"outside home", outside, "家目录"},
		{"home itself", home, "家目录"},
		{"relative", "Library/x", "家目录"},
		{"dot dot escape", home + "/../" + filepath.Base(outside), "家目录"},
		{"protected core folder", filepath.Join(home, "Library"), "不能整体移除"},
		{"protected credential dir", fixtureDir(t, home, ".ssh", 10), "凭据"},
		{"missing", filepath.Join(home, "nope"), "不存在"},
		{"symlink", link, "符号链接"},
	} {
		got := s.measureFully(tc.path, time.Minute)
		if got.OK || !strings.Contains(got.Reason, tc.want) || got.Bytes != 0 {
			t.Errorf("%s: %+v, want a refusal containing %q", tc.name, got, tc.want)
		}
	}
	got := s.measureFully("~/Library/Application Support/DingTalkMac", time.Minute)
	if !got.OK || got.Partial || got.Reason != "" || got.Files != 2 || got.Bytes <= 0 || got.SizeGb != plainGB(got.Bytes) {
		t.Fatalf("offerable path was not measured completely: %+v", got)
	}
}

func TestMeasurePathFullyIgnoresTheScanFileCap(t *testing.T) {
	home := fixtureHome(t)
	dir := filepath.Join(home, "Library/Application Support/Big")
	const scanCap = 10
	for i := 0; i < scanCap+5; i++ {
		writeTestFile(t, filepath.Join(dir, "d", strings.Repeat("a", 1)+itoa64(int64(i))), 0)
	}
	s := &Scanner{home: home}
	if capped := s.fastDirectorySize(dir, scanCap); !capped.partial {
		t.Fatalf("fixture must exceed the scan cap: %+v", capped)
	}
	if got := s.measureFully(dir, time.Minute); !got.OK || got.Partial || got.Files != scanCap+5 {
		t.Fatalf("full measurement stayed capped: %+v", got)
	}
}

func TestMeasurePathFullyReportsAnExpiredBudget(t *testing.T) {
	home := fixtureHome(t)
	dir := fixtureDir(t, home, "Library/Application Support/Slow", 4096)
	got := (&Scanner{home: home}).measureFully(dir, time.Nanosecond)
	if !got.OK || !got.Partial || !strings.Contains(got.Reason, "超过") {
		t.Fatalf("an expired budget must be reported as partial with a reason: %+v", got)
	}
}
