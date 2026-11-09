package scanner

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestParsePurgePaths(t *testing.T) {
	home := "/Users/me"
	got := parsePurgePaths("# comment\n\n  ~/Code  \n/opt/work\n~/code\n~\n", home)
	// ~/code repeats ~/Code only when the folder's case is ambiguous, so just check the rest.
	if len(got) < 3 || got[0] != "/Users/me/Code" || got[1] != "/opt/work" || got[len(got)-1] != "/Users/me" {
		t.Fatalf("parsePurgePaths = %v", got)
	}
}

func TestPurgeRootsMissingEmptyAndUnreadable(t *testing.T) {
	home := fixtureHome(t)
	s := &Scanner{home: home}
	if roots := s.PurgeRoots(); roots.Custom || roots.Error != "" || len(roots.Paths) != len(defaultPurgeRoots) {
		t.Fatalf("a missing file means the defaults: %+v", roots)
	}
	file := PurgePathsPath(home)
	writeFile(t, file, "# only comments\n")
	if roots := s.PurgeRoots(); roots.Custom || len(roots.Paths) != len(defaultPurgeRoots) {
		t.Fatalf("a file with only comments means the defaults: %+v", roots)
	}
	writeFile(t, file, "~/work\n")
	if roots := s.PurgeRoots(); !roots.Custom || !reflect.DeepEqual(roots.Paths, []string{filepath.Join(home, "work")}) {
		t.Fatalf("custom roots replace the defaults: %+v", roots)
	}
	if err := os.Chmod(file, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(file, 0o644)
	if os.Geteuid() != 0 {
		roots := s.PurgeRoots()
		if roots.Error == "" || len(roots.Paths) != 0 {
			t.Fatalf("an unreadable file must stop discovery with a reason: %+v", roots)
		}
		if got := s.scanBuildArtifacts(); len(got) != 0 {
			t.Fatalf("no roots, no rows: %v", got)
		}
	}
}

func TestAddAndRemovePurgeRoot(t *testing.T) {
	home := fixtureHome(t)
	s := &Scanner{home: home}
	writeFile(t, filepath.Join(home, "work/a.txt"), "x")
	if err := os.MkdirAll(filepath.Join(home, "Projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	roots, err := s.AddPurgeRoot("~/work")
	if err != nil || !roots.Custom {
		t.Fatalf("add: %+v %v", roots, err)
	}
	// The first edit keeps the default folders that exist, so adding does not drop them.
	if want := []string{filepath.Join(home, "Projects"), filepath.Join(home, "work")}; !reflect.DeepEqual(roots.Paths, want) {
		t.Fatalf("roots after add = %v, want %v", roots.Paths, want)
	}
	for _, bad := range []string{"relative/dir", "~", "~/missing"} {
		if _, err := s.AddPurgeRoot(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
	roots, err = s.RemovePurgeRoot("~/work")
	if err != nil || len(roots.Paths) != 1 || roots.Paths[0] != filepath.Join(home, "Projects") {
		t.Fatalf("remove: %+v %v", roots, err)
	}
}

func TestArtifactProtections(t *testing.T) {
	deadline := time.Now().Add(time.Minute)
	home := fixtureHome(t)
	guard := func(dir string) artifactCandidate {
		c := artifactCandidate{path: filepath.Join(home, dir), name: filepath.Base(dir)}
		guardArtifact(&c, deadline)
		return c
	}
	fixtureDir(t, home, "keys/target", 10)
	writeFile(t, filepath.Join(home, "keys/target/id-keypair.json"), "{}")
	if c := guard("keys/target"); c.protect == "" {
		t.Error("a *-keypair.json inside protects the folder")
	}
	fixtureDir(t, home, "nested/build/sub/.git", 10)
	if c := guard("nested/build"); c.protect == "" {
		t.Error("a nested .git protects the folder")
	}
	fixtureDir(t, home, "plain/target", 10)
	if c := guard("plain/target"); c.protect != "" || c.unverified != "" {
		t.Errorf("an ordinary artifact is not protected: %+v", c)
	}
	if _, err := exec.LookPath("git"); err == nil {
		repo := filepath.Join(home, "repo")
		writeFile(t, filepath.Join(repo, "dist/bundle.js"), "x")
		for _, args := range [][]string{{"init", "-q"}, {"add", "-f", "dist/bundle.js"}} {
			cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v %s", args, err, out)
			}
		}
		if c := guard("repo/dist"); c.protect == "" {
			t.Error("a folder with Git-tracked files is protected")
		}
	}
	// node_modules: reproducible with a lockfile and install record, protected without.
	nodeProject(t, home, "app")
	if c := guard("app/node_modules"); c.protect != "" {
		t.Errorf("a reproducible node_modules is not protected: %+v", c)
	}
	writeFile(t, filepath.Join(home, "app/node_modules/hand-edited.js"), "x")
	future := time.Now().Add(time.Hour)
	_ = os.Chtimes(filepath.Join(home, "app/node_modules/hand-edited.js"), future, future)
	if c := guard("app/node_modules"); c.protect == "" {
		t.Error("a file newer than the install record protects node_modules")
	}
	fixtureDir(t, home, "nolock/node_modules", 10)
	if c := guard("nolock/node_modules"); c.protect == "" {
		t.Error("node_modules without package.json is protected")
	}
}

func TestArtifactShape(t *testing.T) {
	home := fixtureHome(t)
	fixtureDir(t, home, "dotnet/bin/Debug", 1)
	writeFile(t, filepath.Join(home, "dotnet/App.csproj"), "<Project/>")
	fixtureDir(t, home, "other/bin/Debug", 1)
	if !artifactShape(filepath.Join(home, "dotnet/bin"), "bin") || artifactShape(filepath.Join(home, "other/bin"), "bin") {
		t.Error("bin counts only beside a .NET project file")
	}
	fixtureDir(t, home, "php/vendor", 1)
	writeFile(t, filepath.Join(home, "php/composer.json"), "{}")
	if !artifactShape(filepath.Join(home, "php/vendor"), "vendor") || artifactShape(filepath.Join(home, "other/vendor"), "vendor") {
		t.Error("vendor counts only beside composer.json")
	}
	if artifactShape(filepath.Join(home, "Library/Developer/Xcode/DerivedData"), "DerivedData") {
		t.Error("Xcode's global DerivedData is not a project artifact")
	}
}
