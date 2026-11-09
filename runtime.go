package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/egoist/mygo"

	"github.com/mymoles/mole-studio/internal/scanner"
)

func runtimeIdentity(executable string) map[string]any {
	appPath := appBundleFromExecutable(executable)
	packaged := strings.HasSuffix(strings.ToLower(appPath), ".app")
	identifier := ""
	if packaged {
		output, err := exec.Command("/usr/libexec/PlistBuddy", "-c", "Print :CFBundleIdentifier", filepath.Join(appPath, "Contents", "Info.plist")).Output()
		if err == nil {
			identifier = strings.TrimSpace(string(output))
		}
	}
	return map[string]any{
		"packaged": packaged, "appPath": appPath, "executablePath": executable, "identifier": identifier,
	}
}

// parseCodeSignature reads `codesign -d -r- --verbose=2` output. TCC keys grants
// on the designated requirement, which for an ad-hoc build is its cdhash and so
// changes with every rebuild, while a build signed with the stable release
// certificate keeps the same requirement across updates.
func parseCodeSignature(appPath, output string) (identity string, adhoc bool) {
	requirement := "unsigned"
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimPrefix(strings.TrimSpace(line), "# ") // ad-hoc output prefixes the requirement with "# "
		switch {
		case line == "Signature=adhoc":
			adhoc = true
		case strings.HasPrefix(line, "designated =>"):
			requirement = strings.TrimSpace(strings.TrimPrefix(line, "designated =>"))
		}
	}
	return appPath + "|" + requirement, adhoc
}

var codeSignature = sync.OnceValues(func() (string, bool) {
	executable, err := os.Executable()
	if err != nil {
		return "", false
	}
	appPath := appBundleFromExecutable(executable)
	if !strings.HasSuffix(strings.ToLower(appPath), ".app") {
		return "", false
	}
	output, _ := exec.Command("/usr/bin/codesign", "-d", "-r-", "--verbose=2", appPath).CombinedOutput()
	return parseCodeSignature(appPath, string(output))
})

// initCodeIdentity tells the scanner which code identity permission records belong to.
func initCodeIdentity() {
	scanner.CodeIdentity, _ = codeSignature()
}

// relaunchApp starts a detached helper that waits for this process to exit
// (releasing the single-instance lock) and opens the bundle again, then quits.
func relaunchApp() {
	executable, err := os.Executable()
	if err != nil {
		return
	}
	appPath := appBundleFromExecutable(executable)
	if !strings.HasSuffix(strings.ToLower(appPath), ".app") {
		return
	}
	script := `i=0; while kill -0 "$2" 2>/dev/null && [ $i -lt 50 ]; do sleep 0.2; i=$((i+1)); done; sleep 0.5; /usr/bin/open "$1"`
	command := exec.Command("/bin/sh", "-c", script, "sh", appPath, strconv.Itoa(os.Getpid()))
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if command.Start() != nil {
		return
	}
	_ = command.Process.Release()
	mygo.App.Quit()
}

func pushRuntimeStatus(requestID string, checkFolders bool) {
	go func() {
		executable, err := os.Executable()
		if err != nil {
			return
		}
		status := runtimeIdentity(executable)
		status["requestId"] = requestID
		status["version"] = buildVersion()
		status["menuBarRegistered"] = trayItem != nil
		status["readAccess"] = scanner.Shared.FullDiskReadCheck()
		_, status["adhoc"] = codeSignature()
		if record := scanner.ReadAppManagement(); record != nil {
			status["appManagement"] = record
		}
		if checkFolders {
			status["folderReadAccess"] = scanner.Shared.FolderReadChecks()
		}
		status["checkedAt"] = time.Now().UnixMilli()
		evaluate("window.onNativeRuntimeStatus ? window.onNativeRuntimeStatus(" + jsonOf(status) + ") : undefined")
	}()
}
