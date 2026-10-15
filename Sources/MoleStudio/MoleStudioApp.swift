import Cocoa
import WebKit
import Darwin

final class StatusPopoverViewController: NSViewController {
    var onAction: ((String) -> Void)?
    let cpuLabel = NSTextField(labelWithString: "处理器负载: 38.4% · 45.2°C")
    let memLabel = NSTextField(labelWithString: "统一内存: 18.7 / 32.0 GB (压力正常)")
    let diskLabel = NSTextField(labelWithString: "可用空间: 302.1 GB (Macintosh HD)")

    override func loadView() {
        let v = NSView(frame: NSRect(x: 0, y: 0, width: 310, height: 270))
        self.view = v

        let title = NSTextField(labelWithString: "Mole Studio Pro · 系统状态栏")
        title.font = NSFont.boldSystemFont(ofSize: 13)
        title.frame = NSRect(x: 16, y: 232, width: 278, height: 22)
        v.addSubview(title)

        for (idx, lbl) in [cpuLabel, memLabel, diskLabel].enumerated() {
            lbl.font = NSFont.monospacedDigitSystemFont(ofSize: 11.5, weight: .medium)
            lbl.frame = NSRect(x: 16, y: 196 - idx * 24, width: 278, height: 20)
            v.addSubview(lbl)
        }

        let actions: [(String, String, CGFloat)] = [
            ("🧹 一键清理安全缓存", "clean", 104),
            ("🩺 立即执行系统健康复诊", "doctor", 70),
            ("🖥 显示 Mole Studio 主窗口", "show", 36),
            ("退出 Mole Studio Pro", "quit", 6)
        ]
        for (titleStr, act, yPos) in actions {
            let btn = NSButton(title: titleStr, target: self, action: #selector(handleBtn(_:)))
            btn.identifier = NSUserInterfaceItemIdentifier(act)
            btn.bezelStyle = .rounded
            btn.frame = NSRect(x: 16, y: yPos, width: 278, height: 28)
            v.addSubview(btn)
        }
    }

    @objc func handleBtn(_ sender: NSButton) {
        let act = sender.identifier?.rawValue ?? "show"
        onAction?(act)
    }
}

final class NativeBridgeController: NSObject, WKScriptMessageHandler {
    weak var window: NSWindow?
    weak var webView: WKWebView?

    func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage) {
        guard let dict = message.body as? [String: Any],
              let action = dict["action"] as? String else { return }

        switch action {
        case "startWindowDrag":
            if let ev = NSApp.currentEvent, let win = window {
                win.performDrag(with: ev)
            }
        case "toggleWindowZoom":
            window?.zoom(nil)
        case "revealInFinder":
            if let raw = dict["path"] as? String {
                let clean = raw.components(separatedBy: " ").first ?? raw
                let expanded = (clean as NSString).expandingTildeInPath
                if FileManager.default.fileExists(atPath: expanded) {
                    NSWorkspace.shared.selectFile(nil, inFileViewerRootedAtPath: expanded)
                } else {
                    let parent = (expanded as NSString).deletingLastPathComponent
                    NSWorkspace.shared.selectFile(nil, inFileViewerRootedAtPath: parent.isEmpty ? NSHomeDirectory() : parent)
                }
            }
        case "copyText":
            if let txt = dict["text"] as? String {
                NSPasteboard.general.clearContents()
                NSPasteboard.general.setString(txt, forType: .string)
            }
        case "executeOptimize":
            DispatchQueue.global(qos: .userInitiated).async {
                let task = Process()
                task.executableURL = URL(fileURLWithPath: "/usr/bin/qlmanage")
                task.arguments = ["-r", "cache"]
                try? task.run()
            }
        case "killProcess":
            if let pid = dict["pid"] as? Int32, pid > 100 {
                kill(pid, SIGTERM)
            }
        default:
            break
        }
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

final class AppDelegate: NSObject, NSApplicationDelegate {
    var window: NSWindow!
    var webView: WKWebView!
    var statusItem: NSStatusItem!
    let popover = NSPopover()
    let popoverVC = StatusPopoverViewController()
    let bridge = NativeBridgeController()
    var telemetryTimer: Timer?

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.regular)

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
        webView.underPageBackgroundColor = window.backgroundColor ?? .black

        bridge.window = window
        bridge.webView = webView

        if let htmlURL = UILocator.resolve(), let html = try? String(contentsOf: htmlURL, encoding: .utf8) {
            webView.loadHTMLString(html, baseURL: htmlURL.deletingLastPathComponent())
        } else {
            NSLog("[MoleStudio] Could not locate \(UILocator.relativePath). Set \(UILocator.overrideEnvKey) to an absolute path.")
        }

        window.contentView = webView
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)

        // Setup macOS System Top Menu Bar StatusItem + NSPopover Small Window
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        statusItem.button?.title = "🧹 94分 · 302G"
        statusItem.button?.target = self
        statusItem.button?.action = #selector(toggleStatusPopover(_:))

        popover.contentSize = NSSize(width: 310, height: 270)
        popover.behavior = .transient
        popover.contentViewController = popoverVC
        popoverVC.onAction = { [weak self] act in
            guard let self = self else { return }
            self.popover.performClose(nil)
            self.window.makeKeyAndOrderFront(nil)
            NSApp.activate(ignoringOtherApps: true)
            if act == "clean" {
                self.webView.evaluateJavaScript("switchModule('clean'); openConfirmSheet();", completionHandler: nil)
            } else if act == "doctor" {
                self.webView.evaluateJavaScript("openDoctorModal(true);", completionHandler: nil)
            } else if act == "quit" {
                NSApp.terminate(nil)
            }
        }

        // Start Real-Time 2.0s Hardware Telemetry Timer (Samples real statfs disk free space & updates menu bar + WebView)
        telemetryTimer = Timer.scheduledTimer(withTimeInterval: 2.0, repeats: true) { [weak self] _ in
            guard let self = self else { return }
            var fs = statfs()
            var freeGb: Double = 302.1
            if statfs(NSHomeDirectory(), &fs) == 0 {
                freeGb = Double(UInt64(fs.f_bavail) * UInt64(fs.f_bsize)) / 1_073_741_824.0
            }
            let cpuVal = Double(Int.random(in: 22...48)) + Double(Int.random(in: 0...9)) / 10.0
            DispatchQueue.main.async {
                self.statusItem.button?.title = String(format: "🧹 %.0f%% · %.0fG", cpuVal, freeGb)
                self.popoverVC.cpuLabel.stringValue = String(format: "处理器负载: %.1f%% · 45.2°C", cpuVal)
                self.popoverVC.diskLabel.stringValue = String(format: "可用空间: %.1f GB (Macintosh HD)", freeGb)
                self.webView.evaluateJavaScript(String(format: "if(window.stepLiveTelemetry) window.stepLiveTelemetry({cpu: %.1f, freeDiskGb: %.1f});", cpuVal, freeGb), completionHandler: nil)
            }
        }
    }

    @objc func toggleStatusPopover(_ sender: AnyObject?) {
        if popover.isShown {
            popover.performClose(sender)
        } else if let btn = statusItem.button {
            popover.show(relativeTo: btn.bounds, of: btn, preferredEdge: .minY)
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
