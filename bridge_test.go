package main

import (
	"errors"
	"strings"
	"testing"
)

func TestOptimizeOutcomeReportsActualCommandResult(t *testing.T) {
	success := optimizeOutcome("opt-1", []byte("cache reset"), nil)
	if success["ok"] != true || success["requestId"] != "opt-1" {
		t.Fatalf("success = %v", success)
	}
	failure := optimizeOutcome("opt-2", []byte("Permission denied"), errors.New("exit status 1"))
	if failure["ok"] != false || !strings.Contains(failure["error"].(string), "Permission denied") {
		t.Fatalf("failure = %v", failure)
	}
}

func TestPrivacyPaneURLCoversBothPermissions(t *testing.T) {
	if got := privacyPaneURL("appManagement"); !strings.Contains(got, "Privacy_AppBundles") {
		t.Fatalf("appManagement URL = %q", got)
	}
	if got := privacyPaneURL("fullDiskAccess"); !strings.Contains(got, "Privacy_AllFiles") {
		t.Fatalf("fullDiskAccess URL = %q", got)
	}
	// A receipt without a pane (or with an unknown one) must still open
	// something useful: the permission a clean needs.
	if got := privacyPaneURL(""); !strings.Contains(got, "Privacy_AllFiles") {
		t.Fatalf("default URL = %q", got)
	}
}

func TestPrivacyPaneURLKnowsFilesAndFolders(t *testing.T) {
	if got := privacyPaneURL("filesAndFolders"); !strings.Contains(got, "Privacy_FilesAndFolders") {
		t.Fatalf("filesAndFolders URL = %q", got)
	}
}
