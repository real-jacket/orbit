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
