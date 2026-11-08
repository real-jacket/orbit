package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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
		if checkFolders {
			status["folderReadAccess"] = scanner.Shared.FolderReadChecks()
		}
		status["checkedAt"] = time.Now().UnixMilli()
		evaluate("window.onNativeRuntimeStatus ? window.onNativeRuntimeStatus(" + jsonOf(status) + ") : undefined")
	}()
}
