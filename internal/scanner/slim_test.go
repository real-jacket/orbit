package scanner

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	cpuARM64  = 16777228
	cpuX86_64 = 16777223
)

// fatBytes builds a universal Mach-O file image: a 32-bit fat header and one
// 64-byte slice per cpu type, each starting with that cpu's thin magic.
func fatBytes(cpus ...[2]uint32) []byte {
	out := make([]byte, 8+20*len(cpus))
	binary.BigEndian.PutUint32(out[0:], 0xCAFEBABE)
	binary.BigEndian.PutUint32(out[4:], uint32(len(cpus)))
	for i, c := range cpus {
		off := uint32(len(out))
		entry := out[8+20*i:]
		binary.BigEndian.PutUint32(entry[0:], c[0])
		binary.BigEndian.PutUint32(entry[4:], c[1])
		binary.BigEndian.PutUint32(entry[8:], off)
		binary.BigEndian.PutUint32(entry[12:], 64)
		slice := make([]byte, 64)
		copy(slice, []byte{0xCF, 0xFA, 0xED, 0xFE})
		binary.LittleEndian.PutUint32(slice[4:], c[0])
		out = append(out, slice...)
	}
	return out
}

var universal = fatBytes([2]uint32{cpuX86_64, 3}, [2]uint32{cpuARM64, 0})

func writeBytes(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestReadFatSlices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fat")
	writeBytes(t, path, universal)
	f, _ := os.Open(path)
	defer f.Close()
	slices, ok := readFatSlices(f, int64(len(universal)))
	if !ok || len(slices) != 2 || slices[0].arch != "x86_64" || slices[1].arch != "arm64" || slices[1].size != 64 {
		t.Fatalf("slices = %+v ok=%v", slices, ok)
	}
	arm, intel, ok := armSlice(slices)
	if !ok || arm.offset != slices[1].offset || intel != 64 {
		t.Fatalf("armSlice = %+v %d %v", arm, intel, ok)
	}
	// A slice running past the end of the file is not a fat file.
	if _, ok := readFatSlices(f, int64(len(universal))-1); ok {
		t.Fatal("slice past end of file accepted")
	}
	// 64-bit fat header: one arm64e slice.
	head := make([]byte, 8+32)
	binary.BigEndian.PutUint32(head[0:], 0xCAFEBABF)
	binary.BigEndian.PutUint32(head[4:], 1)
	binary.BigEndian.PutUint32(head[8:], cpuARM64)
	binary.BigEndian.PutUint32(head[12:], 2)
	binary.BigEndian.PutUint64(head[16:], 48)
	binary.BigEndian.PutUint64(head[24:], 16)
	path64 := filepath.Join(t.TempDir(), "fat64")
	writeBytes(t, path64, append(head, make([]byte, 16)...))
	f64, _ := os.Open(path64)
	defer f64.Close()
	slices, ok = readFatSlices(f64, 64)
	if !ok || slices[0].arch != "arm64e" {
		t.Fatalf("fat64 slices = %+v ok=%v", slices, ok)
	}
	// Bad counts and a Java class magic are rejected.
	for _, count := range []uint32{0, 17} {
		bad := append([]byte(nil), universal...)
		binary.BigEndian.PutUint32(bad[4:], count)
		badPath := filepath.Join(t.TempDir(), "bad")
		writeBytes(t, badPath, bad)
		fb, _ := os.Open(badPath)
		if _, ok := readFatSlices(fb, int64(len(bad))); ok {
			t.Fatalf("count %d accepted", count)
		}
		fb.Close()
	}
}

func TestArmSliceRules(t *testing.T) {
	arm := fatSlice{arch: "arm64", size: 10}
	intel := fatSlice{arch: "x86_64", size: 7}
	cases := []struct {
		name   string
		slices []fatSlice
		want   bool
		bytes  int64
	}{
		{"arm64 and intel", []fatSlice{intel, arm}, true, 7},
		{"two intel slices add up", []fatSlice{intel, {arch: "x86_64h", size: 5}, arm}, true, 12},
		{"arm64e stays", []fatSlice{intel, {arch: "arm64e", size: 10}}, false, 0},
		{"two arm64 slices", []fatSlice{arm, arm, intel}, false, 0},
		{"no intel", []fatSlice{arm}, false, 0},
		{"unknown arch", []fatSlice{intel, arm, {arch: "other", size: 1}}, false, 0},
	}
	for _, c := range cases {
		_, got, ok := armSlice(c.slices)
		if ok != c.want || got != c.bytes {
			t.Errorf("%s: ok=%v bytes=%d, want %v %d", c.name, ok, got, c.want, c.bytes)
		}
	}
}

func TestOfficialUninstallerVendor(t *testing.T) {
	cases := []struct{ id, name, path, want string }{
		{"com.crowdstrike.falcon.UI", "", "/Applications/Falcon.app", "CrowdStrike"},
		{"com.Jamf.Connect", "", "/Applications/X.app", "Jamf"},
		{"com.example.agent", "ESET Endpoint Security", "/Applications/X.app", "ESET"},
		{"com.example.agent", "", "/Applications/GlobalProtect.app", "GlobalProtect"},
		{"com.example.notes", "Notes", "/Applications/Notes.app", ""},
	}
	for _, c := range cases {
		if got := officialUninstallerVendor(c.id, c.name, c.path); got != c.want {
			t.Errorf("%s %q: got %q want %q", c.id, c.name, got, c.want)
		}
	}
}

const infoPlist = `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>CFBundleExecutable</key><string>%s</string><key>CFBundleIdentifier</key><string>%s</string><key>CFBundleName</key><string>%s</string></dict></plist>`

// codeResources builds a minimal seal: files2 keys with a cdhash ("c") or a plain hash ("h").
func codeResources(keys map[string]string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>files</key><dict/><key>files2</key><dict>`)
	for key, kind := range keys {
		switch kind {
		case "c":
			fmt.Fprintf(&b, "<key>%s</key><dict><key>cdhash</key><data>AAAA</data><key>requirement</key><string>x</string></dict>", key)
		case "h":
			fmt.Fprintf(&b, "<key>%s</key><dict><key>hash2</key><data>AAAA</data></dict>", key)
		default:
			fmt.Fprintf(&b, "<key>%s</key><dict/>", key)
		}
	}
	b.WriteString(`</dict></dict></plist>`)
	return b.String()
}

// fakeApp lays out a bundle with an Info.plist and no real signature.
func fakeApp(t *testing.T, dir, name, id string) string {
	t.Helper()
	app := filepath.Join(dir, name+".app")
	writeFile(t, filepath.Join(app, "Contents/Info.plist"), fmt.Sprintf(infoPlist, name, id, name))
	return app
}

func TestSlimPlanFollowsSeals(t *testing.T) {
	app := fakeApp(t, t.TempDir(), "Seal", "com.example.seal")
	writeBytes(t, filepath.Join(app, "Contents/MacOS/Seal"), universal)
	// Nested code with its own signature: listed by cdhash in the app's seal.
	writeBytes(t, filepath.Join(app, "Contents/Helpers/tool"), universal)
	// A plain file the app seals by hash: thinning it would break the seal.
	writeBytes(t, filepath.Join(app, "Contents/Frameworks/plain.dylib"), universal)
	// A plugin with its own seal whose main executable the app also seals by plain hash (the ScreenFlow case).
	writeBytes(t, filepath.Join(app, "Contents/PlugIns/Q.qlgenerator/Contents/MacOS/Q"), universal)
	writeFile(t, filepath.Join(app, "Contents/PlugIns/Q.qlgenerator/Contents/Info.plist"), fmt.Sprintf(infoPlist, "Q", "com.example.q", "Q"))
	writeFile(t, filepath.Join(app, "Contents/PlugIns/Q.qlgenerator/Contents/_CodeSignature/CodeResources"), codeResources(nil))
	writeFile(t, filepath.Join(app, "Contents/_CodeSignature/CodeResources"), codeResources(map[string]string{
		"Helpers/tool":                           "c",
		"Frameworks/plain.dylib":                 "h",
		"PlugIns/Q.qlgenerator/Contents/MacOS/Q": "h",
		"Resources/empty":                        "",
	}))
	// Under Resources and already thin files are never planned.
	writeBytes(t, filepath.Join(app, "Contents/Resources/inside"), universal)
	writeBytes(t, filepath.Join(app, "Contents/Helpers/thin"), fatBytes([2]uint32{cpuARM64, 0}))

	files, total := slimPlan(app)
	var got []string
	for _, f := range files {
		got = append(got, f.Rel)
	}
	want := map[string]bool{"Contents/MacOS/Seal": true, "Contents/Helpers/tool": true}
	if len(got) != len(want) || total != 128 {
		t.Fatalf("plan = %v total=%d", got, total)
	}
	for _, rel := range got {
		if !want[rel] {
			t.Fatalf("unexpected file in plan: %s (plan %v)", rel, got)
		}
	}
}

func slimTestScanner(t *testing.T, roots ...string) (*Scanner, slimTools) {
	s := newTestScanner(t)
	s.processProbe = func() (string, error) { return "/usr/bin/other\n", nil }
	tools := slimTools{
		roots:        roots,
		appleSilicon: func() bool { return true },
		team:         func(string) string { return "ABCDE12345" },
		gatekeeper:   func(string) int { return 0 },
		rosetta:      func() map[string]bool { return nil },
	}
	s.slimProbe = &tools
	return s, tools
}

func TestSlimSkipReasons(t *testing.T) {
	root := t.TempDir()
	s, tools := slimTestScanner(t, root)
	name := func(app string) string { return strings.TrimSuffix(filepath.Base(app), ".app") }
	reason := func(app string, rosetta map[string]bool, procs []runningProc) (string, string) {
		return s.slimSkipReason(app, name(app), tools, rosetta, procs)
	}
	plain := fakeApp(t, root, "Plain", "com.example.plain")
	if r, _ := reason(plain, nil, nil); r != "" {
		t.Fatalf("plain app refused: %q", r)
	}
	mas := fakeApp(t, root, "Store", "com.example.store")
	writeFile(t, filepath.Join(mas, "Contents/_MASReceipt/receipt"), "x")
	apple := fakeApp(t, root, "Apple", "com.apple.thing")
	noID := fakeApp(t, root, "NoID", "")
	vendor := fakeApp(t, root, "Agent", "com.crowdstrike.agent")
	rosetta := fakeApp(t, root, "Old", "com.example.old")
	running := fakeApp(t, root, "Busy", "com.example.busy")
	for app, want := range map[string]string{
		mas: "App Store", apple: "苹果", noID: "Bundle ID", vendor: "CrowdStrike", rosetta: "Rosetta",
	} {
		if r, _ := reason(app, map[string]bool{"com.example.old": true}, nil); !strings.Contains(r, want) {
			t.Errorf("%s: reason %q lacks %q", filepath.Base(app), r, want)
		}
	}
	r, unlock := reason(running, nil, []runningProc{{pid: 7, path: filepath.Join(running, "Contents/MacOS/Busy")}})
	if r == "" || !strings.Contains(unlock, "Busy") {
		t.Errorf("running app: reason %q unlock %q", r, unlock)
	}
	// An unwritable folder keeps the app alone.
	locked := filepath.Join(root, "locked")
	lockedApp := fakeApp(t, locked, "Locked", "com.example.locked")
	if err := os.Chmod(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o755)
	if os.Getuid() != 0 {
		if r, _ := reason(lockedApp, nil, nil); !strings.Contains(r, "写入权限") {
			t.Errorf("unwritable folder: %q", r)
		}
	}
}

// realUniversalApp builds a tiny universal, ad-hoc signed app with clang.
func realUniversalApp(t *testing.T, dir string) string {
	t.Helper()
	for _, tool := range []string{"clang", "codesign", "ditto"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not available", tool)
		}
	}
	app := fakeApp(t, dir, "Fix", "com.example.fix")
	src := filepath.Join(t.TempDir(), "main.c")
	writeFile(t, src, "int main(void) { return 0; }\n")
	exe := filepath.Join(app, "Contents/MacOS/Fix")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("clang", "-arch", "arm64", "-arch", "x86_64", "-o", exe, src).CombinedOutput(); err != nil {
		t.Skipf("cannot build a universal binary here: %v %s", err, out)
	}
	if out, err := exec.Command("codesign", "--force", "--sign", "-", app).CombinedOutput(); err != nil {
		t.Fatalf("codesign: %v %s", err, out)
	}
	return app
}

func TestSlimAppsEndToEnd(t *testing.T) {
	root := t.TempDir()
	app := realUniversalApp(t, root)
	s, _ := slimTestScanner(t, root)

	scan := s.ScanSlim()
	if len(scan.Apps) != 1 || scan.Apps[0].Tier != "review" || scan.Apps[0].IntelBytes <= 0 || scan.Apps[0].Reason == "" {
		t.Fatalf("scan = %+v", scan)
	}
	row := scan.Apps[0]
	exe := filepath.Join(app, "Contents/MacOS/Fix")
	before, _ := os.Stat(exe)

	// An app that changed after the scan is refused and left alone.
	if res := s.SlimApps([]SlimTarget{{Path: app, Identity: row.Identity + "x"}}); res.Results[0].Slimmed || !strings.Contains(res.Results[0].Reason, "变化") {
		t.Fatalf("changed app was slimmed: %+v", res)
	}
	if !exists(exe) || isThinArm(exe) {
		t.Fatal("refused app was modified")
	}

	// A running app is refused.
	s.processProbe = func() (string, error) { return filepath.Join(app, "Contents/MacOS/Fix") + "\n", nil }
	if res := s.SlimApps([]SlimTarget{{Path: app, Identity: row.Identity}}); res.Results[0].Slimmed {
		t.Fatalf("running app was slimmed: %+v", res)
	}
	s.processProbe = func() (string, error) { return "/usr/bin/other\n", nil }

	res := s.SlimApps([]SlimTarget{{Path: app, Identity: row.Identity}})
	r := res.Results[0]
	if !r.Slimmed || r.FreedBytes != row.IntelBytes || r.Trashed == "" {
		t.Fatalf("result = %+v", r)
	}
	if !isThinArm(exe) {
		t.Fatal("installed app still holds Intel code")
	}
	if after, _ := os.Stat(exe); after.Size() >= before.Size() {
		t.Fatalf("app did not shrink: %d >= %d", after.Size(), before.Size())
	}
	if out, err := exec.Command("codesign", "--verify", "--deep", "--strict", app).CombinedOutput(); err != nil {
		t.Fatalf("slimmed app fails codesign: %v %s", err, out)
	}
	trashedExe := filepath.Join(r.Trashed, "Contents/MacOS/Fix")
	if isThinArm(trashedExe) || !exists(trashedExe) {
		t.Fatal("the original in the Trash is not the untouched universal app")
	}
	if entries, _ := filepath.Glob(filepath.Join(root, ".orbit-slim-*")); len(entries) != 0 {
		t.Fatalf("staging folder left behind: %v", entries)
	}
	if again := s.ScanSlim(); len(again.Apps) != 0 || again.NoIntel != 1 {
		t.Fatalf("slimmed app listed again: %+v", again)
	}
}

func TestSlimKeepsOriginalWhenCopyFailsVerification(t *testing.T) {
	root := t.TempDir()
	app := realUniversalApp(t, root)
	s, tools := slimTestScanner(t, root)
	tools.gatekeeper = func(path string) int {
		if strings.Contains(path, ".orbit-slim-") {
			return 1 // the copy fails Gatekeeper
		}
		return 0
	}
	s.slimProbe = &tools
	row := s.ScanSlim().Apps[0]
	res := s.SlimApps([]SlimTarget{{Path: app, Identity: row.Identity}})
	if res.Results[0].Slimmed || !strings.Contains(res.Results[0].Reason, "Gatekeeper") {
		t.Fatalf("result = %+v", res.Results[0])
	}
	if isThinArm(filepath.Join(app, "Contents/MacOS/Fix")) {
		t.Fatal("original app was modified")
	}
	if entries, _ := os.ReadDir(filepath.Join(s.home, ".Trash")); len(entries) != 0 {
		t.Fatalf("the original was trashed anyway: %v", entries)
	}
	if entries, _ := filepath.Glob(filepath.Join(root, ".orbit-slim-*")); len(entries) != 0 {
		t.Fatalf("staging folder left behind: %v", entries)
	}
}

func TestScanSlimNeedsAppleSilicon(t *testing.T) {
	s, tools := slimTestScanner(t, t.TempDir())
	tools.appleSilicon = func() bool { return false }
	s.slimProbe = &tools
	if scan := s.ScanSlim(); scan.Unavailable == "" || len(scan.Apps) != 0 {
		t.Fatalf("scan = %+v", scan)
	}
}
