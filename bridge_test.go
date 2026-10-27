package main

import (
	"strings"
	"testing"
)

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
