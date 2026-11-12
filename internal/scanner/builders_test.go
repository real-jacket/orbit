package scanner

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuilderUsingAttributesBuildersToProjects(t *testing.T) {
	root := "/Users/me/Projects/app"
	cases := []struct {
		name string
		proc builderProc
		busy bool
	}{
		{"unrelated project", builderProc{name: "node", cwd: "/Users/me/Projects/other", args: "node /Users/me/Projects/other/server.js"}, false},
		{"sibling with shared prefix", builderProc{name: "node", cwd: "/Users/me/Projects/app-two", args: "node /Users/me/Projects/app-two/x.js"}, false},
		{"cwd at project root", builderProc{name: "node", cwd: root}, true},
		{"cwd inside project", builderProc{name: "cargo", cwd: root + "/src/bin"}, true},
		{"cwd is case aliased", builderProc{name: "node", cwd: "/users/ME/projects/APP"}, true},
		{"artifact inside cwd", builderProc{name: "python3", cwd: "/Users/me/Projects"}, true},
		{"launchd child cwd says nothing", builderProc{name: "node", cwd: "/"}, false},
		{"unknown cwd, unrelated args", builderProc{name: "java", args: "java -jar /opt/tool.jar"}, false},
		{"unknown cwd, args name a project file", builderProc{name: "java", args: "java -jar " + root + "/build/app.jar"}, true},
		{"args name the project dir at end", builderProc{name: "node", args: "node --prefix " + root}, true},
		{"args name a sibling only", builderProc{name: "node", args: "node " + root + "-two/a.js"}, false},
		{"args quoted path", builderProc{name: "node", args: `node --root="` + root + `/x"`}, true},
	}
	for _, tc := range cases {
		if _, busy := builderUsing([]builderProc{tc.proc}, root); busy != tc.busy {
			t.Errorf("%s: busy = %v, want %v", tc.name, busy, tc.busy)
		}
	}
	if _, busy := builderUsing(nil, root); busy {
		t.Error("no builders must never block")
	}
}

func TestArtifactCleanupReasonUsesBuilderAttribution(t *testing.T) {
	home := fixtureHome(t)
	fixtureDir(t, home, "Projects/old/node_modules", 4096)
	fixtureDir(t, home, "Projects/busy/node_modules", 4096)
	old := filepath.Join(home, "Projects/old/node_modules")
	busy := filepath.Join(home, "Projects/busy/node_modules")
	calls := 0
	s := &Scanner{home: home, processProbe: func() (string, error) { return "/sbin/launchd\n", nil }}
	s.builderProbe = func() ([]builderProc, error) {
		calls++
		return []builderProc{{pid: 9, name: "node", cwd: filepath.Join(home, "Projects/busy/packages/web")}}, nil
	}
	if reason := s.cleanupReason(old, true); reason != "" {
		t.Fatalf("a builder in another project held back an idle artifact: %q", reason)
	}
	if reason := s.cleanupReason(busy, true); !strings.Contains(reason, "正在运行并使用") {
		t.Fatalf("the project's own builder must hold its artifact back: %q", reason)
	}
	s.builderProbe = func() ([]builderProc, error) { return nil, errors.New("lsof unavailable") }
	if reason := s.cleanupReason(old, true); !strings.Contains(reason, "无法确认") {
		t.Fatalf("a failed attribution snapshot must keep the conservative block: %q", reason)
	}
	if calls != 2 {
		t.Fatalf("live checks must each take a fresh snapshot, got %d", calls)
	}
}

func TestExternalArtifactKeepsRunningBuilderProtection(t *testing.T) {
	home := fixtureHome(t)
	external := fixtureHome(t)
	fixtureDir(t, external, "Project/target", 21_000_000)
	project := filepath.Join(external, "Project")
	artifact := filepath.Join(project, "target")
	s := &Scanner{
		home:         home,
		processProbe: func() (string, error) { return "/sbin/launchd\n", nil },
		builderProbe: func() ([]builderProc, error) {
			return []builderProc{{pid: 9, name: "cargo", cwd: filepath.Join(project, "src")}}, nil
		},
	}
	for _, live := range []bool{false, true} {
		if reason := s.cleanupReason(artifact, live); !strings.Contains(reason, "正在运行并使用") {
			t.Errorf("external project lost builder protection at live=%v: %q", live, reason)
		}
	}
	rows := s.scanBuildArtifactsWithRoots([]string{external})
	if len(rows) != 1 || rows[0]["tier"] != "protected" || rows[0]["checked"] == true {
		t.Fatalf("external artifact must be displayed as protected: %+v", rows)
	}
	s.builderProbe = func() ([]builderProc, error) { return nil, errors.New("lsof unavailable") }
	if reason := s.cleanupReason(artifact, true); !strings.Contains(reason, "无法确认") {
		t.Errorf("a failed external-project probe must preserve data: %q", reason)
	}
	s.builderProbe = func() ([]builderProc, error) { return nil, nil }
	if reason := s.cleanupReason(artifact, true); reason != "" {
		t.Errorf("an idle external artifact should remain reviewable: %q", reason)
	}
}

func TestBuilderSnapshotCachesDisplayOnly(t *testing.T) {
	calls := 0
	s := &Scanner{processTTL: time.Hour, builderProbe: func() ([]builderProc, error) { calls++; return nil, nil }}
	s.builderSnapshot(false)
	s.builderSnapshot(false)
	if calls != 1 {
		t.Fatalf("display snapshot not reused: %d probes", calls)
	}
	s.builderSnapshot(true)
	s.builderSnapshot(true)
	if calls != 3 {
		t.Fatalf("live snapshot must always probe: %d probes", calls)
	}
	failing := &Scanner{processTTL: time.Hour, builderProbe: func() ([]builderProc, error) { calls++; return nil, errors.New("x") }}
	before := calls
	failing.builderSnapshot(false)
	failing.builderSnapshot(false)
	if calls-before != 2 {
		t.Fatal("a failed snapshot must not be reused")
	}
}

func TestDerivedDataIsHeldBackOnlyByXcodeProcesses(t *testing.T) {
	home := fixtureHome(t)
	derived := filepath.Join(home, "Library/Developer/Xcode/DerivedData")
	fixtureDir(t, home, "Library/Developer/Xcode/DerivedData/App-abc/Build", 4096)
	child := filepath.Join(derived, "App-abc")
	nested := filepath.Join(child, "Build")
	cases := []struct {
		name  string
		procs []builderProc
		busy  bool
	}{
		{"no builders", nil, false},
		{"node and python in the parent dir", []builderProc{{name: "node", cwd: filepath.Dir(derived)}, {name: "python3", cwd: "/Users"}, {name: "java", args: "java " + derived + "/x"}}, false},
		{"xcodebuild", []builderProc{{name: "xcodebuild", cwd: filepath.Join(home, "Projects/App")}}, true},
		{"Xcode app", []builderProc{{name: "xcode", cwd: "/"}}, true},
		{"swift-frontend", []builderProc{{name: "swift-frontend"}}, true},
		{"SourceKitService", []builderProc{{name: "sourcekitservice"}}, true},
		{"XCBBuildService", []builderProc{{name: "xcbbuildservice"}}, true},
	}
	for _, tc := range cases {
		procs := tc.procs
		s := &Scanner{home: home, processProbe: func() (string, error) { return "/sbin/launchd\n", nil }, builderProbe: func() ([]builderProc, error) { return procs, nil }}
		for _, path := range []string{derived, child, nested} {
			reason := s.cleanupReason(path, true)
			if busy := strings.Contains(reason, "正在运行"); busy != tc.busy {
				t.Errorf("%s: %s reason = %q, busy want %v", tc.name, path, reason, tc.busy)
			}
		}
	}
	s := &Scanner{home: home, processProbe: func() (string, error) { return "/sbin/launchd\n", nil }, builderProbe: func() ([]builderProc, error) { return nil, errors.New("ps failed") }}
	if reason := s.cleanupReason(derived, true); !strings.Contains(reason, "无法确认") {
		t.Fatalf("a failed snapshot keeps DerivedData conservative: %q", reason)
	}
}
