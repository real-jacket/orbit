import Cocoa
import WebKit
import Darwin

final class StatusPopoverViewController: NSViewController {
    var popoverWebView: WKWebView?
    let bridge: NativeBridgeController

    init(bridge: NativeBridgeController) {
        self.bridge = bridge
        super.init(nibName: nil, bundle: nil)
    }

    required init?(coder: NSCoder) { fatalError("init(coder:) has not been implemented") }

    override func loadView() {
        let rect = NSRect(x: 0, y: 0, width: 356, height: 480)
        let config = WKWebViewConfiguration()
        let controller = WKUserContentController()
        controller.add(bridge, name: "moleNative")
        let js = """
        window.isNativeMacApp = true;
        window.isMenuBarPopover = true;
        document.addEventListener('DOMContentLoaded', function() {
          document.body.classList.add('menubar-mode');
        });
        """
        controller.addUserScript(WKUserScript(source: js, injectionTime: .atDocumentStart, forMainFrameOnly: true))
        config.userContentController = controller

        let webView = WKWebView(frame: rect, configuration: config)
        webView.setValue(false, forKey: "drawsBackground")
        popoverWebView = webView
        view = webView

        if let htmlURL = UILocator.resolve(), let html = try? String(contentsOf: htmlURL, encoding: .utf8) {
            webView.loadHTMLString(html, baseURL: htmlURL.deletingLastPathComponent())
        }
    }
}

final class NativeBridgeController: NSObject, WKScriptMessageHandler {
    weak var appDelegate: AppDelegate?
    private var caffeinateProc: Process?

    func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage) {
        guard let dict = message.body as? [String: Any],
              let action = dict["action"] as? String else { return }

        switch action {
        case "startWindowDrag":
            if let event = NSApp.currentEvent, let win = appDelegate?.window {
                win.performDrag(with: event)
            }
        case "toggleWindowZoom":
            appDelegate?.window?.zoom(nil)
        case "revealInFinder":
            if let raw = dict["path"] as? String {
                reveal(raw)
            }
        case "copyText":
            if let txt = dict["text"] as? String {
                NSPasteboard.general.clearContents()
                NSPasteboard.general.setString(txt, forType: .string)
            }
        case "trashRealPaths", "executeClean", "trashPath":
            let paths = (dict["paths"] as? [String]) ?? [dict["path"] as? String].compactMap { $0 }
            DispatchQueue.global(qos: .userInitiated).async { [weak self] in
                _ = RealMacSystemScanner.shared.movePathsToTrash(paths)
                self?.appDelegate?.pushRealTelemetryNow()
                self?.appDelegate?.pushRealSystemScanNow()
            }
        case "killProcess":
            if let pid = intValue(dict["pid"]), pid > 1 {
                NSRunningApplication(processIdentifier: pid_t(pid))?.terminate()
                kill(pid_t(pid), SIGTERM)
                DispatchQueue.global().asyncAfter(deadline: .now() + 0.4) { [weak self] in
                    if NSRunningApplication(processIdentifier: pid_t(pid)) != nil {
                        kill(pid_t(pid), SIGKILL)
                    }
                    self?.appDelegate?.pushRealTelemetryNow()
                }
            }
        case "requestRealSystemScan":
            appDelegate?.pushRealSystemScanNow()
        case "requestDiskAnalyze":
            let path = dict["path"] as? String
            appDelegate?.pushDiskAnalyze(path: path)
        case "requestRealTelemetry":
            appDelegate?.pushRealTelemetryNow()
        case "executeOptimize":
            DispatchQueue.global(qos: .userInitiated).async {
                let task = Process()
                task.executableURL = URL(fileURLWithPath: "/usr/bin/qlmanage")
                task.arguments = ["-r", "cache"]
                try? task.run()
            }
        case "toggleCaffeinate":
            caffeinateProc?.terminate()
            caffeinateProc = nil
            if let active = dict["active"] as? Bool, active {
                let proc = Process()
                proc.executableURL = URL(fileURLWithPath: "/usr/bin/caffeinate")
                proc.arguments = ["-d", "-i", "-t", "7200"]
                try? proc.run()
                caffeinateProc = proc
            }
        case "menubarOpenClean":
            showMainWindow("switchModule('clean'); openConfirmSheet();")
        case "menubarOpenDoctor":
            showMainWindow("openDoctorModal(true);")
        case "menubarShowMain":
            showMainWindow(nil)
        case "menubarQuit":
            NSApp.terminate(nil)
        case "setTheme":
            // "dark" / "light" are an explicit choice in the web UI; "system" means follow macOS.
            if let theme = dict["theme"] as? String, ["dark", "light", "system"].contains(theme) {
                appDelegate?.applyTheme(theme, remember: true)
            }
        default:
            break
        }
    }

    private func reveal(_ raw: String) {
        let clean = raw.components(separatedBy: " ").first ?? raw
        let expanded = (clean as NSString).expandingTildeInPath
        if FileManager.default.fileExists(atPath: expanded) {
            NSWorkspace.shared.selectFile(nil, inFileViewerRootedAtPath: expanded)
        } else {
            let parent = (expanded as NSString).deletingLastPathComponent
            NSWorkspace.shared.selectFile(nil, inFileViewerRootedAtPath: parent.isEmpty ? NSHomeDirectory() : parent)
        }
    }

    private func showMainWindow(_ script: String?) {
        appDelegate?.popover.performClose(nil)
        appDelegate?.window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
        if let script {
            appDelegate?.webView.evaluateJavaScript(script, completionHandler: nil)
        }
    }

    private func intValue(_ value: Any?) -> Int? {
        if let number = value as? Int { return number }
        if let number = value as? Int32 { return Int(number) }
        if let number = value as? Double { return Int(number) }
        return nil
    }
}

/// Locates `Resources/src-ui/index.html` for every supported launch mode: a packaged `.app`
/// bundle, `swift run` straight out of a checkout, or an explicit `MOLE_UI_HTML` override.
enum UILocator {
    static let overrideEnvKey = "MOLE_UI_HTML"
    static let relativePath = "Resources/src-ui/index.html"

    static func resolve() -> URL? {
        var candidates: [URL] = []

        if let override = ProcessInfo.processInfo.environment[overrideEnvKey], !override.isEmpty {
            candidates.append(URL(fileURLWithPath: override))
        }
        if let resources = Bundle.main.resourceURL {
            candidates.append(resources.appendingPathComponent("src-ui/index.html"))
        }
        // Walk up from the executable so a `swift run` build finds the checkout's Resources/.
        var dir = URL(fileURLWithPath: CommandLine.arguments[0])
            .resolvingSymlinksInPath()
            .deletingLastPathComponent()
        for _ in 0..<6 {
            candidates.append(dir.appendingPathComponent(relativePath))
            let parent = dir.deletingLastPathComponent()
            if parent.path == dir.path { break }
            dir = parent
        }

        return candidates.first { FileManager.default.isReadableFile(atPath: $0.path) }
    }
}

final class AppDelegate: NSObject, NSApplicationDelegate, WKNavigationDelegate {
    var window: NSWindow!
    var webView: WKWebView!
    var statusItem: NSStatusItem!
    let popover = NSPopover()
    lazy var popoverVC = StatusPopoverViewController(bridge: bridge)
    let bridge = NativeBridgeController()
    var telemetryTimer: Timer?

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.regular)
        bridge.appDelegate = self

        let rect = NSRect(x: 0, y: 0, width: 1380, height: 860)
        window = NSWindow(
            contentRect: rect,
            styleMask: [.titled, .closable, .miniaturizable, .resizable, .fullSizeContentView],
            backing: .buffered,
            defer: false
        )
        window.title = "orbit"
        // The delegate owns the window strongly; without this, closing it (red button, ⌘W)
        // releases it a second time and reopening from the menu-bar item crashes.
        window.isReleasedWhenClosed = false
        window.titlebarAppearsTransparent = true
        window.titleVisibility = .hidden
        window.isMovableByWindowBackground = true
        window.minSize = NSSize(width: 880, height: 620)
        window.backgroundColor = AppDelegate.darkBackground
        if !window.setFrameUsingName("orbit.main") { window.center() }
        window.setFrameAutosaveName("orbit.main")

        let config = WKWebViewConfiguration()
        let ucc = WKUserContentController()
        ucc.add(bridge, name: "moleNative")
        let js = "window.isNativeMacApp = true; document.addEventListener('DOMContentLoaded', function(){ var tl = document.querySelector('.traffic-lights'); if(tl) tl.style.visibility='hidden'; });"
        ucc.addUserScript(WKUserScript(source: js, injectionTime: .atDocumentStart, forMainFrameOnly: true))
        config.userContentController = ucc

        webView = WKWebView(frame: rect, configuration: config)
        webView.autoresizingMask = [.width, .height]
        webView.navigationDelegate = self
        webView.underPageBackgroundColor = window.backgroundColor ?? .black

        if let htmlURL = UILocator.resolve(), let html = try? String(contentsOf: htmlURL, encoding: .utf8) {
            webView.loadHTMLString(html, baseURL: htmlURL.deletingLastPathComponent())
        } else {
            NSLog("[MoleStudio] Could not locate \(UILocator.relativePath). Set \(UILocator.overrideEnvKey) to an absolute path.")
        }

        window.contentView = webView
        applyTheme(UserDefaults.standard.string(forKey: AppDelegate.themeKey) ?? "system", remember: false)
        buildMainMenu()
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)

        configureStatusItem()

        popover.contentSize = NSSize(width: 356, height: 480)
        popover.behavior = .transient
        popover.contentViewController = popoverVC

        telemetryTimer = Timer.scheduledTimer(withTimeInterval: 1.5, repeats: true) { [weak self] _ in
            self?.pushRealTelemetryNow()
        }
    }

    /// Menu-bar item: a monochrome template mark, so AppKit tints it for light and dark menu bars.
    private func configureStatusItem() {
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
        if let button = statusItem.button {
            // The menu-bar mark carries the same three elements as the app icon —
            // sphere, tilted orbit, companion — reduced to a monochrome template so
            // AppKit can tint it for light and dark menu bars. It is drawn from the
            // brand's 64-unit grid rather than hand-placed, so its proportions stay
            // identical to the Dock icon and the sidebar SVG.
            //
            // A template image is one flat colour, so the orbit cannot be separated
            // from the sphere by value the way it is in the app icon. The separation
            // is cut instead: a clear stroke around the sphere removes the ring
            // where it meets the silhouette, which is what makes the mark read as a
            // planet inside an orbit rather than a solid blob at 18pt.
            let orbitImage = NSImage(size: NSSize(width: 18, height: 18), flipped: false) { _ in
                let unit = 18.0 / 64.0
                func point(_ x: CGFloat, _ y: CGFloat) -> NSPoint {
                    NSPoint(x: x * unit, y: 18 - y * unit)
                }
                NSColor.black.setStroke()
                NSColor.black.setFill()

                let sphereRadius = 13.5 * unit
                let centre = point(32, 32)
                let ringWidth = 4.2 * unit

                // Orbit: rx 26 / ry 10.5, tilted -26° in the SVG's y-down space,
                // which is +26° once this flipped:false view maps it y-up.
                let transform = NSAffineTransform()
                transform.translateX(by: centre.x, yBy: centre.y)
                transform.rotate(byDegrees: 26)
                transform.translateX(by: -centre.x, yBy: -centre.y)
                let ring = NSBezierPath(ovalIn: NSRect(x: centre.x - 26 * unit,
                                                       y: centre.y - 10.5 * unit,
                                                       width: 52 * unit, height: 21 * unit))
                ring.lineWidth = ringWidth
                ring.transform(using: transform as AffineTransform)

                ring.stroke()

                // Cut the seam around the sphere, then fill the sphere so only the
                // outer half of the gap survives as a visible separation.
                if let context = NSGraphicsContext.current {
                    context.saveGraphicsState()
                    context.compositingOperation = .clear
                    let gap = NSBezierPath(ovalIn: NSRect(x: centre.x - sphereRadius,
                                                          y: centre.y - sphereRadius,
                                                          width: sphereRadius * 2,
                                                          height: sphereRadius * 2))
                    gap.lineWidth = ringWidth * 1.15
                    gap.stroke()
                    context.restoreGraphicsState()
                }
                NSBezierPath(ovalIn: NSRect(x: centre.x - sphereRadius, y: centre.y - sphereRadius,
                                            width: sphereRadius * 2,
                                            height: sphereRadius * 2)).fill()

                // Companion: r 4.4 at (50, 21), filled so it survives the tint.
                let companionRadius = 4.4 * unit
                let companion = point(50, 21)
                NSBezierPath(ovalIn: NSRect(x: companion.x - companionRadius,
                                            y: companion.y - companionRadius,
                                            width: companionRadius * 2,
                                            height: companionRadius * 2)).fill()
                return true
            }
            orbitImage.isTemplate = true
            orbitImage.accessibilityDescription = "orbit"
            button.image = orbitImage
            button.toolTip = "orbit · 点击展开系统状态栏"
            button.target = self
            button.action = #selector(toggleStatusPopover(_:))
        }
    }

    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        pushRealTelemetryNow()
        pushRealSystemScanNow()
    }

    func pushRealTelemetryNow() {
        DispatchQueue.global(qos: .userInitiated).async { [weak self] in
            let disk = RealMacSystemScanner.shared.queryDiskSpace()
            let mem = RealMacSystemScanner.shared.queryMemoryUsage()
            let procSnap = RealMacSystemScanner.shared.sampleRealProcesses()
            let power = PowerStatus.snapshot()
            let payload: [String: Any] = [
                "cpu": procSnap.cpuTotal,
                "freeDiskGb": disk.freeGb,
                "memUsedGb": mem.usedGb,
                "memTotalGb": mem.totalGb,
                "processes": procSnap.processes,
                "power": power
            ]
            guard let jsonData = try? JSONSerialization.data(withJSONObject: payload),
                  let json = String(data: jsonData, encoding: .utf8) else { return }
            let script = "if(window.onNativeRealTelemetry) window.onNativeRealTelemetry(\(json));"
            DispatchQueue.main.async {
                guard let self else { return }
                self.statusItem.button?.toolTip = String(
                    format: "orbit · CPU %.1f%% · 剩余 %.1f GB",
                    procSnap.cpuTotal,
                    disk.freeGb
                )
                self.evaluate(script)
            }
        }
    }

    func pushDiskAnalyze(path: String?) {
        DispatchQueue.global(qos: .userInitiated).async { [weak self] in
            let expanded = path.map { ($0 as NSString).expandingTildeInPath }
            var payload = expanded.map { MoleAnalyzeBridge.children(path: $0) } ?? MoleAnalyzeBridge.overview()
            payload["path"] = expanded ?? ""
            guard let jsonData = try? JSONSerialization.data(withJSONObject: payload),
                  let json = String(data: jsonData, encoding: .utf8) else { return }
            let script = "if(window.onNativeDiskAnalyze) window.onNativeDiskAnalyze(\(json));"
            DispatchQueue.main.async {
                self?.evaluate(script)
            }
        }
    }

    func pushRealSystemScanNow() {
        DispatchQueue.global(qos: .userInitiated).async { [weak self] in
            let scanPayload = RealMacSystemScanner.shared.buildFullScanPayload()
            guard let jsonData = try? JSONSerialization.data(withJSONObject: scanPayload),
                  let json = String(data: jsonData, encoding: .utf8) else { return }
            let script = "if(window.onNativeRealScanComplete) window.onNativeRealScanComplete(\(json));"
            DispatchQueue.main.async {
                self?.evaluate(script)
            }
        }
    }

    private func evaluate(_ script: String) {
        webView?.evaluateJavaScript(script, completionHandler: nil)
        if popover.isShown {
            popoverVC.popoverWebView?.evaluateJavaScript(script, completionHandler: nil)
        }
    }

    @objc func toggleStatusPopover(_ sender: AnyObject?) {
        if popover.isShown {
            popover.performClose(sender)
        } else if let button = statusItem.button {
            popover.show(relativeTo: button.bounds, of: button, preferredEdge: .minY)
            pushRealTelemetryNow()
            pushRealSystemScanNow()
        }
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        return false
    }

    /// Clicking the Dock icon after the window was closed brings it back.
    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        if !flag { window.makeKeyAndOrderFront(nil) }
        return true
    }

    // MARK: - Appearance

    static let themeKey = "orbit.theme"
    // Same values as --bg-window in index.html (dark and light).
    static let darkBackground = NSColor(calibratedRed: 0.067, green: 0.067, blue: 0.063, alpha: 1.0)
    static let lightBackground = NSColor(calibratedRed: 0.980, green: 0.973, blue: 0.961, alpha: 1.0)

    /// Keeps native chrome (window background, overscroll colour, system controls) in step with the
    /// web UI. "system" clears the override so macOS drives both.
    func applyTheme(_ theme: String, remember: Bool) {
        switch theme {
        case "dark": NSApp.appearance = NSAppearance(named: .darkAqua)
        case "light": NSApp.appearance = NSAppearance(named: .aqua)
        default: NSApp.appearance = nil
        }
        if remember {
            if theme == "system" {
                UserDefaults.standard.removeObject(forKey: AppDelegate.themeKey)
            } else {
                UserDefaults.standard.set(theme, forKey: AppDelegate.themeKey)
            }
        }
        let isDark = NSApp.effectiveAppearance.bestMatch(from: [.darkAqua, .aqua]) == .darkAqua
        window.backgroundColor = isDark ? AppDelegate.darkBackground : AppDelegate.lightBackground
        webView.underPageBackgroundColor = window.backgroundColor ?? .black
    }

    // MARK: - Main menu
    // Without a main menu, ⌘C / ⌘V / ⌘A do nothing in the web view's search and command-palette
    // fields, and ⌘Q, ⌘W, ⌘M and ⌘H are dead.

    private func buildMainMenu() {
        let mainMenu = NSMenu()

        let appItem = NSMenuItem()
        mainMenu.addItem(appItem)
        let appMenu = NSMenu()
        appItem.submenu = appMenu
        appMenu.addItem(withTitle: "关于 orbit", action: #selector(NSApplication.orderFrontStandardAboutPanel(_:)), keyEquivalent: "")
        appMenu.addItem(.separator())
        appMenu.addItem(withTitle: "隐藏 orbit", action: #selector(NSApplication.hide(_:)), keyEquivalent: "h")
        let hideOthers = appMenu.addItem(withTitle: "隐藏其他", action: #selector(NSApplication.hideOtherApplications(_:)), keyEquivalent: "h")
        hideOthers.keyEquivalentModifierMask = [.command, .option]
        appMenu.addItem(withTitle: "全部显示", action: #selector(NSApplication.unhideAllApplications(_:)), keyEquivalent: "")
        appMenu.addItem(.separator())
        appMenu.addItem(withTitle: "退出 orbit", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")

        let editItem = NSMenuItem()
        mainMenu.addItem(editItem)
        let editMenu = NSMenu(title: "编辑")
        editItem.submenu = editMenu
        editMenu.addItem(withTitle: "撤销", action: Selector(("undo:")), keyEquivalent: "z")
        let redo = editMenu.addItem(withTitle: "重做", action: Selector(("redo:")), keyEquivalent: "z")
        redo.keyEquivalentModifierMask = [.command, .shift]
        editMenu.addItem(.separator())
        editMenu.addItem(withTitle: "剪切", action: #selector(NSText.cut(_:)), keyEquivalent: "x")
        editMenu.addItem(withTitle: "拷贝", action: #selector(NSText.copy(_:)), keyEquivalent: "c")
        editMenu.addItem(withTitle: "粘贴", action: #selector(NSText.paste(_:)), keyEquivalent: "v")
        editMenu.addItem(withTitle: "全选", action: #selector(NSText.selectAll(_:)), keyEquivalent: "a")

        let windowItem = NSMenuItem()
        mainMenu.addItem(windowItem)
        let windowMenu = NSMenu(title: "窗口")
        windowItem.submenu = windowMenu
        windowMenu.addItem(withTitle: "最小化", action: #selector(NSWindow.performMiniaturize(_:)), keyEquivalent: "m")
        windowMenu.addItem(withTitle: "缩放", action: #selector(NSWindow.performZoom(_:)), keyEquivalent: "")
        windowMenu.addItem(withTitle: "关闭", action: #selector(NSWindow.performClose(_:)), keyEquivalent: "w")

        NSApp.mainMenu = mainMenu
        NSApp.windowsMenu = windowMenu
    }
}

@main
enum MoleStudioMain {
    /// Held for the lifetime of the process because `NSApplication.delegate` is a weak reference.
    static let delegate = AppDelegate()

    static func main() {
        let app = NSApplication.shared
        app.delegate = delegate
        app.run()
    }
}
