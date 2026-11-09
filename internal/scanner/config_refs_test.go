package scanner

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLineNamesPath(t *testing.T) {
	const app = "/Applications/Foo.app"
	cases := []struct {
		line string
		want bool
	}{
		{`export PATH="/Applications/Foo.app/Contents/MacOS:$PATH"`, true},
		{`alias foo=/Applications/Foo.app`, true},
		{`"command": "/Applications/Foo.app",`, true},
		{`source /Applications/Foo.app;`, true},
		{`# see /Applications/Foo.app`, true},
		{`x=/mnt/Applications/Foo.app`, false}, // glued to a longer path on the left
		{`/Applications/Foo.app2`, false},      // glued to a longer name on the right
		{`/Applications/Foo.application`, false},
		{`/Applications/Foo.app-beta/bin`, false},
		{`-/Applications/Foo.app`, false},
		{`no mention here`, false},
		{`/Applications/Foo.app2 and then /Applications/Foo.app`, true}, // a later hit still counts
	}
	for _, c := range cases {
		if got := lineNamesPath(c.line, app); got != c.want {
			t.Errorf("%q: got %v want %v", c.line, got, c.want)
		}
	}
}

func TestConfigReferences(t *testing.T) {
	s := newTestScanner(t)
	gone := filepath.Join(s.home, "Tools/Foo")
	alive := filepath.Join(s.home, "Tools/Alive")
	if err := os.MkdirAll(alive, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(s.home, ".zshrc"), strings.Join([]string{
		"# nothing",
		`export PATH="$HOME/Tools/Foo/bin:$PATH"`,
		`alias a=~/Tools/Alive/run`, // still installed: not reported
		`source ` + gone + `/env.sh`,
		`export X=${HOME}/Tools/Foo`,
		`export Y=$HOME/Tools/FooBar`, // a different folder
	}, "\n"))
	writeFile(t, filepath.Join(s.home, ".claude/settings.json"), `{"hooks": {"command": "\/Users\/x\/y", "run": "`+strings.ReplaceAll(gone, "/", `\/`)+`/hook.sh"}}`)
	// A file with a NUL byte, an oversized file and a symlink out of the home are skipped.
	writeFile(t, filepath.Join(s.home, ".bashrc"), "export A="+gone+"\x00\n")
	writeFile(t, filepath.Join(s.home, ".profile"), strings.Repeat("x", configRefMaxBytes)+"\n"+gone+"\n")
	outside := filepath.Join(t.TempDir(), "outside")
	writeFile(t, outside, "export A="+gone+"\n")
	if err := os.Symlink(outside, filepath.Join(s.home, ".zshenv")); err != nil {
		t.Fatal(err)
	}

	got := s.ConfigReferences([]string{gone, alive, "relative/path", "/", filepath.Join(s.home, "Tools/Missing")})
	want := []ConfigRef{
		{"~/.zshrc", 2, gone}, {"~/.zshrc", 4, gone}, {"~/.zshrc", 5, gone},
		{"~/.claude/settings.json", 1, gone},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("refs = %+v\nwant %+v", got, want)
	}
	if refs := s.ConfigReferences([]string{alive}); len(refs) != 0 {
		t.Fatalf("an installed path was reported: %+v", refs)
	}
}

func TestEscapeControl(t *testing.T) {
	if got := escapeControl("a\nb\x7f"); got != `a\x0ab\x7f` {
		t.Fatalf("got %q", got)
	}
}
