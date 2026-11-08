package main

// This file builds the app: the main window, the menu-bar item, the panel it
// opens, and the menu bar. It is the Go form of the Swift app's AppDelegate
// applicationDidFinishLaunching, minus the parts that belong to the page.

import (
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"

	"github.com/mymoles/mole-studio/internal/analyze"
	"github.com/mymoles/mole-studio/internal/scanner"
	"github.com/mymoles/mole-studio/internal/trayicon"
)

// The windows and the menu-bar item the bridge and the push helpers talk to.
var (
	mainWindow   *mygo.Window
	flyoutWindow atomic.Pointer[mygo.Window]
	trayItem     *mygo.Tray
)

// appName is what the app calls itself in the menu bar, the About panel and the
// window title. It is the name the Swift app used.
const appName = "orbit"

// fillBuildInfo stamps the running build into the scanner so the 体检 panel can
// report it. The commit comes from the linker when a release build sets it;
// otherwise the executable's mtime is the honest answer.
func fillBuildInfo() {
	scanner.BuildTime = scanner.DescribeExecutable()
}

// buildVersion is the version the running build reports. `mygo build` writes
// mygo.json's version into the bundle's Info.plist and this reads it back, so
// the number in the About panel and the 体检 row is the number of the code that
// actually ran. A plain `go run` has no bundle to read, and says "dev" rather
// than a version nobody bumped. The release workflow stamps mygo.json from the
// tag before it builds (see .github/workflows/release.yml).
func buildVersion() string {
	if version := mygo.App.Version(); version != "" {
		return version
	}
	return "dev"
}

// quitting is set while a quit is in progress. The window's close handler
// normally hides the window instead of closing it, and without this flag that
// hid the window out from under App.Quit, so 退出 never quit.
var quitting atomic.Bool

// run wires the app up and starts its event loop.
func run() {
	// The update window is Sparkle-style and reads releases from the repo in
	// mygo.json's updates block. Dev builds carry no key, so it only says that
	// updates are unavailable. No fixed Language: the window follows the app's
	// locale, which initLanguage keeps on the interface language (see lang.go).
	mygo.Use(updater.New(updater.Options{}))

	mygo.Bind(&NativeBridge{})

	mygo.App.SetName(appName)

	// A second launch (a double click while the menu-bar app is running) hands
	// over to the first instance and exits, instead of showing two menu-bar
	// items that each rescan the disk. The lock key comes from the name above.
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	mygo.App.OnSecondInstance(func([]string, string) {
		if w := activeMainWindow(); w != nil {
			w.Restore()
		}
		showMainWindow("")
	})

	// App icons are served from Go instead of being inlined as base64 in the
	// scan payload. The scheme must exist before the first window is created.
	if err := mygo.Protocol.Handle(scanner.IconScheme, scanner.IconHandler()); err != nil {
		panic(err)
	}

	version := buildVersion()
	mygo.App.SetVersion(version)
	scanner.BuildVersion = version

	// Without a listener MyGo quits when the last window closes. The Swift
	// app returned false from applicationShouldTerminateAfterLastWindowClosed
	// to keep running in the menu bar, so this is that.
	mygo.App.OnWindowAllClosed(func() {})

	// Clicking the Dock icon with no window open reopens the main window:
	// the Go form of applicationShouldHandleReopen.
	mygo.App.OnActivate(func(hasVisibleWindows bool) {
		if hasVisibleWindows {
			return
		}
		if w := activeMainWindow(); w != nil {
			w.Show()
		}
	})

	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { quitting.Store(true) })

	mygo.App.WhenReady(buildUI)
	if err := mygo.App.Run(); err != nil {
		panic(err)
	}
}

// buildUI creates the windows and the menu bar. It runs once, on the main
// thread, right before the event loop starts.
func buildUI() {
	fillBuildInfo()
	initLanguage()
	createTray()

	// Main window. The Swift app used a full-size content view with a
	// transparent, hidden title bar, which TitleBarHidden is in MyGo, and
	// remembered its frame under "orbit.main", which StateKey does here.
	mainWindow = mygo.NewWindow(mygo.WindowOptions{
		Title:           appName,
		URL:             "/",
		Width:           1380,
		Height:          860,
		MinWidth:        880,
		MinHeight:       620,
		TitleBarStyle:   mygo.TitleBarHidden,
		BackgroundColor: darkBackground,
		StateKey:        "orbit.main",
		Page: mygo.PageOptions{
			PreloadScript: preloadScript(false),
		},
	})

	// Closing the window leaves it alive so the menu-bar item can bring it
	// back; destroying it would take the page's state with it, and the Swift
	// app kept it alive for exactly that reason.
	mainWindow.OnClose(func(e *mygo.CloseEvent) {
		if quitting.Load() {
			return
		}
		e.PreventDefault()
		mainWindow.Hide()
	})

	// The page asks for a scan and for telemetry as soon as it loads, and the
	// Swift app answered the same way once its web view finished loading.
	mainWindow.Page().OnDidFinishLoad(func() {
		pushRealTelemetryNow()
		pushRealSystemScanNow()
		pushRuntimeStatus("", false)
	})

	// The page's first paint should already sit on the right background,
	// which is why the theme is applied before the window is shown.
	applyTheme(loadSavedTheme(), false)

	mygo.App.SetMenu(buildApplicationMenu())

	mainWindow.Show()
	mygo.App.Focus()

	startTelemetryTimer()

	// The disk analyzer reports scan progress from its own goroutine; the page
	// turns it into the "已扫描 N 个文件" line.
	analyze.Shared.SetProgressSink(pushAnalyzeProgress)
}

// createTray puts the app's mark in the menu bar. The icon is drawn in Go as a
// template image, so the system tints it for a light or a dark menu bar.
func createTray() {
	icon, err := trayicon.PNG(trayicon.Size())
	if err != nil {
		panic(err)
	}
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:             appName,
		Icon:           icon,
		IconIsTemplate: true,
		ToolTip:        trayDefaultToolTip(),
	})
	if err != nil {
		panic(err)
	}
	tray.OnClick(toggleFlyout)
	trayItem = tray
}

// toggleFlyout shows or hides the menu-bar panel. The page opens it with
// ?mode=menubar, which is how it knows to draw the compact layout instead of
// the full window.
func toggleFlyout() {
	w := flyoutWindow.Load()
	if w != nil && !w.IsDestroyed() {
		if w.IsVisible() {
			w.Hide()
			return
		}
	}
	showFlyout()
}

func showFlyout() {
	w, fresh := ensureFlyout()
	if w == nil {
		return
	}
	w.Show()
	w.Focus()
	if fresh {
		return // the new page asks for its state in OnDidFinishLoad; pushing now would send the payload twice
	}
	pushRealTelemetryNow()
	pushCachedSystemScan(w.Page())
	pushCaffeinateState(currentCaffeinateActive())
}

// ensureFlyout returns the menu-bar panel, creating it the first time. A
// popover that the system closed may take its window with it, so a destroyed
// panel is replaced rather than reused. fresh reports a panel created by this
// call, whose page has not loaded yet.
func ensureFlyout() (panel *mygo.Window, fresh bool) {
	if w := flyoutWindow.Load(); w != nil && !w.IsDestroyed() {
		return w, false
	}
	if trayItem == nil {
		return nil, false
	}
	panel = mygo.NewFlyout(mygo.FlyoutOptions{
		Tray:         trayItem,
		URL:          "/?mode=menubar",
		Width:        380,
		Height:       588,
		Placement:    mygo.PlacementBottom,
		Popover:      true,
		Focusable:    true,
		Hidden:       true,
		CornerRadius: 12,
		Page: mygo.PageOptions{
			PreloadScript: preloadScript(true),
		},
	})
	panel.Page().OnDidFinishLoad(func() {
		pushRealTelemetryNow()
		pushCachedSystemScan(panel.Page())
		pushCaffeinateState(currentCaffeinateActive())
		pushFlyoutTheme(loadSavedTheme())
		pushFlyoutLanguage(language())
	})
	panel.OnClosed(func() {
		flyoutWindow.CompareAndSwap(panel, nil)
	})
	flyoutWindow.Store(panel)
	return panel, true
}

// startTelemetryTimer refreshes the live numbers every 1.5 seconds, the
// interval the Swift app's timer used.
func startTelemetryTimer() {
	go func() {
		ticker := time.NewTicker(1500 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			// Sampling still walks the process table and reads IOKit every tick,
			// so it only runs while a window actually shows the numbers.
			if !appVisible() {
				continue
			}
			pushRealTelemetry(false)
		}
	}()
}

// appVisible reports whether something of the app is on screen and therefore
// worth refreshing.
func appVisible() bool {
	if w := activeMainWindow(); w != nil && w.IsVisible() {
		return true
	}
	if w := flyoutWindow.Load(); w != nil && !w.IsDestroyed() && w.IsVisible() {
		return true
	}
	return false
}

// buildApplicationMenu builds the menu bar the Swift app built by hand. Without
// one, the standard editing shortcuts and ⌘Q, ⌘W, ⌘M, ⌘H do nothing.
func buildApplicationMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{
			Role: mygo.RoleAppMenu,
			Submenu: []*mygo.MenuItem{
				{Role: mygo.RoleAbout, Label: L("关于 ", "About ") + appName},
				updater.MenuItem(),
				mygo.Separator(),
				{Role: mygo.RoleHide, Label: L("隐藏 ", "Hide ") + appName},
				{Role: mygo.RoleHideOthers, Label: L("隐藏其他", "Hide others")},
				{Role: mygo.RoleUnhide, Label: L("全部显示", "Show all")},
				mygo.Separator(),
				{Role: mygo.RoleQuit, Label: L("退出 ", "Quit ") + appName},
			},
		},
		{
			Role:  mygo.RoleEditMenu,
			Label: L("编辑", "Edit"),
			Submenu: []*mygo.MenuItem{
				{Role: mygo.RoleUndo, Label: L("撤销", "Undo")},
				{Role: mygo.RoleRedo, Label: L("重做", "Redo")},
				mygo.Separator(),
				{Role: mygo.RoleCut, Label: L("剪切", "Cut")},
				{Role: mygo.RoleCopy, Label: L("拷贝", "Copy")},
				{Role: mygo.RolePaste, Label: L("粘贴", "Paste")},
				{Role: mygo.RoleSelectAll, Label: L("全选", "Select all")},
			},
		},
		{
			Label: L("工具", "Tools"),
			Submenu: []*mygo.MenuItem{
				{Label: L("菜单栏工具", "Menu bar panel"), Click: func(*mygo.MenuItem, *mygo.Window) { showFlyout() }},
				{Label: L("运行与权限", "Runtime and permissions"), Click: func(*mygo.MenuItem, *mygo.Window) { showMainWindow("openRuntimePanel();") }},
			},
		},
		{
			Role:  mygo.RoleWindowMenu,
			Label: L("窗口", "Window"),
			Submenu: []*mygo.MenuItem{
				{Role: mygo.RoleMinimize, Label: L("最小化", "Minimize")},
				{Role: mygo.RoleZoom, Label: L("缩放", "Zoom")},
				{Role: mygo.RoleClose, Label: L("关闭", "Close")},
			},
		},
	})
}
