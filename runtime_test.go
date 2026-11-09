package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRuntimeIdentityDoesNotPresentGoRunAsAnApp(t *testing.T) {
	executable := "/tmp/go-build123/b001/exe/mole-studio"
	status := runtimeIdentity(executable)
	if status["packaged"] != false || status["identifier"] != "" || status["appPath"] != executable {
		t.Fatalf("temporary identity = %v", status)
	}
}

func TestRuntimeIdentityReadsTheExactBundle(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), "orbit.app")
	if err := os.MkdirAll(filepath.Join(bundle, "Contents"), 0o700); err != nil {
		t.Fatal(err)
	}
	data := `<?xml version="1.0"?><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>com.mymoles.mole-studio</string></dict></plist>`
	if err := os.WriteFile(filepath.Join(bundle, "Contents", "Info.plist"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	status := runtimeIdentity(filepath.Join(bundle, "Contents", "MacOS", "orbit"))
	if status["packaged"] != true || status["appPath"] != bundle {
		t.Fatalf("packaged identity = %v", status)
	}
	if runtime.GOOS == "darwin" && status["identifier"] != "com.mymoles.mole-studio" {
		t.Fatalf("bundle identifier = %v", status["identifier"])
	}
}

func TestParseCodeSignatureTellsAdHocFromCertificateBuilds(t *testing.T) {
	cert := "Identifier=x\nAuthority=orbit Self-Signed\ndesignated => identifier \"com.mymoles.mole-studio\" and certificate leaf = H\"aa\"\n"
	id, adhoc := parseCodeSignature("/Applications/orbit.app", cert)
	if adhoc || id != "/Applications/orbit.app|identifier \"com.mymoles.mole-studio\" and certificate leaf = H\"aa\"" {
		t.Fatalf("certificate build: %q adhoc=%v", id, adhoc)
	}
	local := "Signature=adhoc\nTeamIdentifier=not set\n# designated => cdhash H\"bb\"\n"
	id, adhoc = parseCodeSignature("/tmp/orbit.app", local)
	if !adhoc || id != "/tmp/orbit.app|cdhash H\"bb\"" {
		t.Fatalf("ad-hoc build: %q adhoc=%v", id, adhoc)
	}
	if id, _ := parseCodeSignature("/tmp/a.app", ""); id != "/tmp/a.app|unsigned" {
		t.Fatalf("no codesign output: %q", id)
	}
}
