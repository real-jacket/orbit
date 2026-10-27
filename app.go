package main

// This file builds the app: the main window, the menu-bar item, the panel it
// opens, and the menu bar. It is the Go form of the Swift app's AppDelegate
// applicationDidFinishLaunching, minus the parts that belong to the page.

import (
	"time"

	"github.com/egoist/mygo"

	"github.com/mymoles/mole-studio/internal/trayicon"
)

// The windows and the menu-bar item the bridge and the push helpers talk to.
var (
	mainWindow   *mygo.Window
	flyoutWindow *mygo.Window
	trayItem     *mygo.Tray
)

// appName is what the app calls itself in the menu bar, the About panel and the
// window title. It is the name the Swift app used.
const appName = "orbit"

// run wires the app up and starts its event loop.
func run() {
	mygo.Bind(&NativeBridge{})

	mygo.App.SetName(appName)
	mygo.App.SetVersion("0.1.0")

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

	mygo.App.WhenReady(buildUI)
	if err := mygo.App.Run(); err != nil {
		panic(err)
	}
}

// buildUI creates the windows and the menu bar. It runs once, on the main
// thread, right before the event loop starts.
func buildUI() {
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
		e.PreventDefault()
		mainWindow.Hide()
	})

	// The page asks for a scan and for telemetry as soon as it loads, and the
	// Swift app answered the same way once its web view finished loading.
	mainWindow.Page().OnDidFinishLoad(func() {
		pushRealTelemetryNow()
		pushRealSystemScanNow()
	})

	// The page's first paint should already sit on the right background,
	// which is why the theme is applied before the window is shown.
	applyTheme(loadSavedTheme(), false)

	mygo.App.SetMenu(buildApplicationMenu())

	mainWindow.Show()
	mygo.App.Focus()

	startTelemetryTimer()
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
		ToolTip:        "orbit · 点击展开系统状态栏",
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
	w := flyoutWindow
	if w != nil && !w.IsDestroyed() {
		if w.IsVisible() {
			w.Hide()
			return
		}
	}
	w = ensureFlyout()
	if w == nil {
		return
	}
	w.Show()
	// The Swift app refreshed both payloads when its popover opened, since
	// the panel is a snapshot of the moment it was opened.
	pushRealTelemetryNow()
	pushRealSystemScanNow()
}

// ensureFlyout returns the menu-bar panel, creating it the first time. A
// popover that the system closed may take its window with it, so a destroyed
// panel is replaced rather than reused.
func ensureFlyout() *mygo.Window {
	if flyoutWindow != nil && !flyoutWindow.IsDestroyed() {
		return flyoutWindow
	}
	if trayItem == nil {
		return nil
	}
	panel := mygo.NewFlyout(mygo.FlyoutOptions{
		Tray:         trayItem,
		URL:          "/?mode=menubar",
		Width:        356,
		Height:       480,
		Placement:    mygo.PlacementBottom,
		Popover:      true,
		Hidden:       true,
		CornerRadius: 12,
		Page: mygo.PageOptions{
			PreloadScript: preloadScript(true),
		},
	})
	panel.Page().OnDidFinishLoad(func() {
		pushRealTelemetryNow()
		pushRealSystemScanNow()
	})
	panel.OnClosed(func() {
		if flyoutWindow == panel {
			flyoutWindow = nil
		}
	})
	flyoutWindow = panel
	return panel
}

// startTelemetryTimer refreshes the live numbers every 1.5 seconds, the
// interval the Swift app's timer used.
func startTelemetryTimer() {
	go func() {
		ticker := time.NewTicker(1500 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			if activeMainWindow() == nil {
				continue
			}
			pushRealTelemetryNow()
		}
	}()
}

// buildApplicationMenu builds the menu bar the Swift app built by hand. Without
// one, the standard editing shortcuts and ⌘Q, ⌘W, ⌘M, ⌘H do nothing.
func buildApplicationMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{
			Role: mygo.RoleAppMenu,
			Submenu: []*mygo.MenuItem{
				{Role: mygo.RoleAbout, Label: "关于 " + appName},
				mygo.Separator(),
				{Role: mygo.RoleHide, Label: "隐藏 " + appName},
				{Role: mygo.RoleHideOthers, Label: "隐藏其他"},
				{Role: mygo.RoleUnhide, Label: "全部显示"},
				mygo.Separator(),
				{Role: mygo.RoleQuit, Label: "退出 " + appName},
			},
		},
		{
			Role:  mygo.RoleEditMenu,
			Label: "编辑",
			Submenu: []*mygo.MenuItem{
				{Role: mygo.RoleUndo, Label: "撤销"},
				{Role: mygo.RoleRedo, Label: "重做"},
				mygo.Separator(),
				{Role: mygo.RoleCut, Label: "剪切"},
				{Role: mygo.RoleCopy, Label: "拷贝"},
				{Role: mygo.RolePaste, Label: "粘贴"},
				{Role: mygo.RoleSelectAll, Label: "全选"},
			},
		},
		{
			Role:  mygo.RoleWindowMenu,
			Label: "窗口",
			Submenu: []*mygo.MenuItem{
				{Role: mygo.RoleMinimize, Label: "最小化"},
				{Role: mygo.RoleZoom, Label: "缩放"},
				{Role: mygo.RoleClose, Label: "关闭"},
			},
		},
	})
}
