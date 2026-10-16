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
        let rect = NSRect(x: 0, y: 0, width: 356, height: 436)
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
        window.title = "Mole Studio Pro"
        window.titlebarAppearsTransparent = true
        window.titleVisibility = .hidden
        window.isMovableByWindowBackground = true
        window.minSize = NSSize(width: 880, height: 620)
        window.backgroundColor = NSColor(calibratedRed: 0.067, green: 0.067, blue: 0.063, alpha: 1.0)
        window.center()

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
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)

        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
        if let button = statusItem.button {
            let orbitImage = NSImage(size: NSSize(width: 18, height: 18), flipped: false) { _ in
                NSColor.black.setStroke()
                NSColor.black.setFill()
                NSBezierPath(ovalIn: NSRect(x: 7.2, y: 7.2, width: 3.6, height: 3.6)).fill()
                let transform = NSAffineTransform()
                transform.translateX(by: 9, yBy: 9)
                transform.rotate(byDegrees: 28)
                transform.translateX(by: -9, yBy: -9)
                let ring = NSBezierPath(ovalIn: NSRect(x: 1.5, y: 5.4, width: 15, height: 7.2))
                ring.lineWidth = 1.6
                ring.transform(using: transform as AffineTransform)
                ring.stroke()
                NSBezierPath(ovalIn: NSRect(x: 13.8, y: 11.2, width: 2.6, height: 2.6)).fill()
                return true
            }
            orbitImage.isTemplate = true
            button.image = orbitImage
            button.toolTip = "Mole Studio · 点击展开系统状态栏"
            button.target = self
            button.action = #selector(toggleStatusPopover(_:))
        }

        popover.contentSize = NSSize(width: 356, height: 436)
        popover.behavior = .transient
        popover.contentViewController = popoverVC

        telemetryTimer = Timer.scheduledTimer(withTimeInterval: 1.5, repeats: true) { [weak self] _ in
            self?.pushRealTelemetryNow()
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
            let payload: [String: Any] = [
                "cpu": procSnap.cpuTotal,
                "freeDiskGb": disk.freeGb,
                "memUsedGb": mem.usedGb,
                "memTotalGb": mem.totalGb,
                "processes": procSnap.processes
            ]
            guard let jsonData = try? JSONSerialization.data(withJSONObject: payload),
                  let json = String(data: jsonData, encoding: .utf8) else { return }
            let script = "if(window.onNativeRealTelemetry) window.onNativeRealTelemetry(\(json));"
            DispatchQueue.main.async {
                guard let self else { return }
                self.statusItem.button?.toolTip = String(
                    format: "Mole Studio · CPU %.1f%% · 剩余 %.1f GB",
                    procSnap.cpuTotal,
                    disk.freeGb
                )
                self.evaluate(script)
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
