package scanner

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSafariCacheUsesBrowserActivityInsteadOfStandaloneExtensions(t *testing.T) {
	home := fixtureHome(t)
	app := filepath.Join(home, "Applications/Safari.app")
	writeFile(t, filepath.Join(app, "Contents/Info.plist"), `<plist><dict><key>CFBundleName</key><string>Safari</string><key>CFBundleIdentifier</key><string>com.apple.Safari</string></dict></plist>`)
	cache := fixtureDir(t, home, "Library/Caches/com.apple.Safari", 4096)
	extension := filepath.Join(app, "Contents/Extensions/SafariWidgetExtension.appex")
	writeFile(t, filepath.Join(extension, "Contents/Info.plist"), `<plist><dict><key>CFBundleName</key><string>SafariWidgetExtension</string><key>CFBundleIdentifier</key><string>com.apple.Safari.SafariWidgetExtension</string></dict></plist>`)
	extensionCache := fixtureDir(t, home, "Library/Caches/com.apple.Safari.SafariWidgetExtension", 4096)
	widget := extension + "/Contents/MacOS/SafariWidgetExtension\n"
	cases := []struct {
		name, processes string
		busy            bool
	}{
		{"widget", widget, false},
		{"multiple widget processes", widget + widget, false},
		{"link extension", app + "/Contents/Extensions/SafariLinkExtension.appex/Contents/MacOS/SafariLinkExtension\n", false},
		{"launch agent", "/System/Cryptexes/App/usr/libexec/SafariLaunchAgent\n", false},
		{"bookmark sync", "/System/Cryptexes/App/usr/libexec/SafariBookmarksSyncAgent\n", false},
		{"safe browsing", "/System/Library/PrivateFrameworks/SafariSafeBrowsing.framework/com.apple.Safari.SafeBrowsing.Service\n", false},
		{"WebKit", "/System/Library/Frameworks/WebKit.framework/Versions/A/XPCServices/com.apple.WebKit.Networking.xpc/Contents/MacOS/com.apple.WebKit.Networking\n", false},
		{"Safari main", widget + app + "/Contents/MacOS/Safari\n", true},
		{"Safari helper", app + "/Contents/Frameworks/Browser Helper.app/Contents/MacOS/Browser Helper\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := policyScanner(home)
			s.processProbe = func() (string, error) { return tc.processes, nil }
			for _, live := range []bool{false, true} {
				for _, path := range []string{cache, filepath.Join(cache, "WebKit/NetworkCache")} {
					reason := s.cleanupReason(path, live)
					if tc.busy && !strings.Contains(reason, "正在运行") || !tc.busy && reason != "" {
						t.Errorf("live=%v path=%s reason=%q, busy=%v", live, path, reason, tc.busy)
					}
				}
			}
		})
	}
	s := policyScanner(home)
	s.processProbe = func() (string, error) { return widget, nil }
	if reason := s.cleanupReason(extensionCache, true); !strings.Contains(reason, "SafariWidgetExtension 正在运行") {
		t.Fatalf("extension's own cache lost its protection: %q", reason)
	}
	if running, _ := appRunning(parseProcessLines(widget), app); !running {
		t.Fatal("bundle removal must still count a running extension")
	}
	for _, state := range []struct {
		open bool
		err  error
		want string
	}{
		{open: true, want: "仍有进程打开"},
		{err: errors.New("lsof unavailable"), want: "无法确认"},
	} {
		s.handleProbe = func(string) (bool, error) { return state.open, state.err }
		if reason := s.cleanupReason(cache, true); !strings.Contains(reason, state.want) {
			t.Errorf("extension-only activity bypassed the open-file check: %q, want %q", reason, state.want)
		}
	}
}

func TestBrowserCacheProtectionIsPerPath(t *testing.T) {
	home := fixtureHome(t)
	safari := filepath.Join(home, "Applications/Safari.app")
	chrome := filepath.Join(home, "Applications/Google Chrome.app")
	writeFile(t, filepath.Join(safari, "Contents/Info.plist"), `<plist><dict><key>CFBundleName</key><string>Safari</string><key>CFBundleIdentifier</key><string>com.apple.Safari</string></dict></plist>`)
	writeFile(t, filepath.Join(chrome, "Contents/Info.plist"), `<plist><dict><key>CFBundleName</key><string>Google Chrome</string><key>CFBundleIdentifier</key><string>com.google.Chrome</string></dict></plist>`)
	fixtureDir(t, home, "Library/Caches/com.apple.Safari", 4096)
	fixtureDir(t, home, "Library/Caches/Google/Chrome", 4096)
	s := policyScanner(home)
	s.processProbe = func() (string, error) {
		return safari + "/Contents/Extensions/SafariLinkExtension.appex/Contents/MacOS/SafariLinkExtension\n" + chrome + "/Contents/MacOS/Google Chrome\n", nil
	}
	row := s.catalog(cleanGroupSpecs()[1], &catalogClaims{}, nil)
	if row == nil || row["tier"] != "safe" || row["checked"] != true {
		t.Fatalf("Safari extension or active Chrome blocked the whole browser group: %+v", row)
	}
	for _, path := range row["subpaths"].([]map[string]any) {
		if strings.Contains(path["path"].(string), "Safari") {
			if path["tier"] != "safe" || path["checked"] != true {
				t.Errorf("idle Safari cache remained protected: %+v", path)
			}
		} else if path["tier"] != "protected" || path["checked"] != false || !strings.Contains(path["reason"].(string), "Google Chrome 正在运行") {
			t.Errorf("active Chrome cache lost its own reason: %+v", path)
		}
	}
}

func TestSafariCacheRechecksBrowserAtMoveBoundary(t *testing.T) {
	home := fixtureHome(t)
	app := filepath.Join(home, "Applications/Safari.app")
	writeFile(t, filepath.Join(app, "Contents/Info.plist"), `<plist><dict><key>CFBundleName</key><string>Safari</string><key>CFBundleIdentifier</key><string>com.apple.Safari</string></dict></plist>`)
	cache := fixtureDir(t, home, "Library/Caches/com.apple.Safari", 4096)
	processes := app + "/Contents/Extensions/SafariWidgetExtension.appex/Contents/MacOS/SafariWidgetExtension\n"
	s := policyScanner(home)
	s.processTTL = time.Minute
	s.processProbe = func() (string, error) { return processes, nil }
	if reason := s.CleanupReason(cache); reason != "" {
		t.Fatalf("standalone widget blocked the preview: %q", reason)
	}
	processes += app + "/Contents/MacOS/Safari\n"
	if reason := s.cleanupReason(cache, true); !strings.Contains(reason, "Safari 正在运行") {
		t.Fatalf("move boundary reused the idle preview: %q", reason)
	}
}
