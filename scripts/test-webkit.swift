import AppKit
import CryptoKit
import WebKit

// Isolated page checks: no visible window, user browser profile, or native file actions.

struct Scenario: Decodable {
    let name: String
    let width: Double
    let height: Double
    let script: String
}
final class Suite: NSObject, WKNavigationDelegate, WKScriptMessageHandler {
    let web: WKWebView
    let window: NSWindow
    let output: URL
    let setup: String
    let scenarios: [Scenario]
    let sourceHash: String
    var index = 0
    var reports = [[String: Any]]()
    init(html: URL, directory: URL) throws {
        output = directory
        sourceHash = SHA256.hash(data: try Data(contentsOf: html)).map { String(format: "%02x", $0) }.joined()
        setup = try String(contentsOf: directory.appendingPathComponent("fixtures.js"), encoding: .utf8)
        scenarios = try JSONDecoder().decode([Scenario].self, from: Data(contentsOf: directory.appendingPathComponent("suite.json")))
        let config = WKWebViewConfiguration()
        config.websiteDataStore = .nonPersistent()
        let bootstrap = "window.__qa={actions:[],errors:[]}; window.addEventListener('error',e=>__qa.errors.push(e.message)); window.addEventListener('unhandledrejection',e=>__qa.errors.push(String(e.reason))); const originalError=console.error; console.error=(...args)=>{__qa.errors.push(args.map(String).join(' ')); originalError(...args);};"
        config.userContentController.addUserScript(WKUserScript(source: bootstrap, injectionTime: .atDocumentStart, forMainFrameOnly: true))
        web = WKWebView(frame: NSRect(x: 0, y: 0, width: 1380, height: 860), configuration: config)
        window = NSWindow(contentRect: web.frame, styleMask: [.borderless], backing: .buffered, defer: false)
        super.init()
        config.userContentController.add(self, name: "moleNative")
        window.contentView = web
        web.navigationDelegate = self
        web.loadFileURL(html, allowingReadAccessTo: html.deletingLastPathComponent())
    }
    func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage) {}
    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        web.evaluateJavaScript(setup + ";null") { _, error in
            if let error = error { print(error); exit(1) }
            self.next()
        }
    }
    func next() {
        guard index < scenarios.count else {
            do {
                let data = try JSONSerialization.data(withJSONObject: reports, options: [.prettyPrinted, .sortedKeys])
                try data.write(to: output.appendingPathComponent("webkit-report.json"))
            } catch { print(error); exit(1) }
            let failures = reports.filter { $0["error"] != nil || ($0["pageErrors"] as? [String] ?? []).count > 0 || ($0["overflow"] as? [String] ?? []).count > 0 }
            print("Scenarios: \(reports.count), failures: \(failures.count)")
            exit(failures.isEmpty ? 0 : 1)
        }
        let scenario = scenarios[index]
        window.setContentSize(NSSize(width: scenario.width, height: scenario.height))
        web.setFrameSize(NSSize(width: scenario.width, height: scenario.height))
        let check = """
        await new Promise(r=>setTimeout(r,100));
        const overflow=[];
        for(const el of document.querySelectorAll('.top-header,.summary-banner,.panel-card,.treemap-split,.btn,.pref-btn')){
          const r=el.getBoundingClientRect();
          if(r.width===0||r.height===0)continue;
          if(r.x < -2 || r.right > innerWidth+2)overflow.push(el.className+': viewport');
          if(el.matches('.btn,.pref-btn') && el.scrollWidth > el.clientWidth+3)overflow.push(el.className+': label');
        }
        const content=document.getElementById('moduleStage').textContent;
        if(STATE.module==='ai_moon')qaAssert(document.getElementById('heroDesc').textContent.includes('不按天数'),'AI cleanup cannot claim unsupported age filtering');
        if(/undefined|NaN|\\[object Object\\]/.test(content))throw new Error('Invalid visible values');
        qaAssert([...document.querySelectorAll('input[type=checkbox]')].every(i=>i.getAttribute('aria-label')),'checkboxes have accessible names');
        qaAssert(document.querySelectorAll('.nav-item.active').length===1 && document.querySelector('.nav-item.active').dataset.mod===STATE.module,'sidebar selection matches current module');
        const report={width:innerWidth,height:innerHeight,pageErrors:__qa.errors.slice(),overflow,primary:document.getElementById('primaryBtnText').textContent,disabled:document.getElementById('primaryBtnText').closest('button').disabled,module:STATE.module,theme:document.documentElement.dataset.theme};
        document.getElementById('scanSourceBadge').textContent='Isolated fixture data / not a live scan';
        return JSON.stringify(report);
        """
        web.callAsyncJavaScript(scenario.script + "\n" + check, arguments: [:], in: nil, in: .page) { result in
            var report: [String: Any] = ["name": scenario.name, "sourceHash": self.sourceHash]
            switch result {
            case .failure(let error): report["error"] = String(describing: error)
            case .success(let result):
                if let json = result as? String, let data = json.data(using: .utf8), let fields = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
                    report.merge(fields) { _, next in next }
                } else { report["error"] = "Result is not JSON" }
            }
            let configuration = WKSnapshotConfiguration()
            configuration.afterScreenUpdates = true
            self.web.takeSnapshot(with: configuration) { image, error in
                if let image = image, let tiff = image.tiffRepresentation, let bitmap = NSBitmapImageRep(data: tiff), let png = bitmap.representation(using: .png, properties: [:]) {
                    do { try png.write(to: self.output.appendingPathComponent(scenario.name + ".png")) }
                    catch { report["error"] = String(describing: error) }
                } else { report["error"] = String(describing: error) }
                print(scenario.name + ": " + (report["error"] as? String ?? "rendered"))
                self.reports.append(report)
                self.index += 1
                self.next()
            }
        }
    }
    func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) { print(error); exit(1) }
}
let app = NSApplication.shared
app.setActivationPolicy(.prohibited)
let suite = try Suite(html: URL(fileURLWithPath: CommandLine.arguments[1]), directory: URL(fileURLWithPath: CommandLine.arguments[2]))
Timer.scheduledTimer(withTimeInterval: 180, repeats: false) { _ in print("suite timeout"); exit(1) }
app.run()
