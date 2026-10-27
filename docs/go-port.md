# Go 迁移记录

Swift（AppKit + WKWebView）→ Go（MyGo）迁移的落地记录。框架选型的论证、MyGo 的能力清单和各 Phase 的实施顺序在 [`mygo-migration.md`](mygo-migration.md) 里，本文不重复，只记录**实际怎么迁的、和 Swift 版差在哪、怎么验**。

> 下面表里的 `Sources/...` 是**迁移来源**，不是现存文件。Swift 实现已在迁移提交里删除；要对照原文件请走 git 历史，例如 `git show adea952:Sources/MoleStudio/MoleStudioApp.swift`。

## 1. 迁移对照表

### 1.1 领域层：`internal/`

| Swift | Go | 说明 |
| --- | --- | --- |
| `Sources/MoleStudio/Models/MoleModels.swift` | `internal/model/model.go` | `MoleModule` → `Module`（`clean`/`ai_moon`/… 原始值不变）；`.protectedSystem` → `TierProtectedSystem = "protected"` |
| `Sources/MoleStudio/Engine/MoleViewModel.swift` | `internal/model/viewmodel.go` | `NewViewModel()` 的默认值同 Swift：`clean` 模块、中文、暗色、`DryRunMode = true` |
| `Sources/MoleStudio/Engine/RealMacSystemScanner.swift` | `internal/scanner/scanner.go`<br>`internal/scanner/system.go`<br>`internal/scanner/apps.go`<br>`internal/scanner/largefiles.go`<br>`internal/scanner/payload.go`<br>`internal/scanner/plist.go`<br>`internal/scanner/trash_darwin.go` | 按职责拆开：目录体积/格式化 → `scanner.go`；磁盘/内存/CPU/进程表 → `system.go`；应用与残留匹配 → `apps.go`；大文件 → `largefiles.go`；`buildFullScanPayload`/遥测 → `payload.go`；Info.plist → `plist.go`；废纸篓 → `trash_darwin.go` |
| `Sources/MoleStudio/Engine/CatalogScanning.swift` | `internal/scanner/catalog.go` | `scanCleanGroups`/`scanAICaches`/`scanInstallerPackages`/`scanBuildArtifacts` 全在，`CatalogSpec` 取代 Swift 的局部 spec 结构体 |
| `Sources/MoleStudio/Engine/OrphanScanning.swift` | `internal/scanner/orphans.go` | 含 `scanUnclaimedContainers` |
| `Sources/MoleStudio/Engine/OrphanSignatures.swift` | `internal/scanner/signatures.go` | `struct OrphanSignature` 数据原样搬过来，`AllSignatures()` |
| `Sources/MoleStudio/Engine/MoleAnalyzeBridge.swift` | `internal/analyze/bridge.go` | `overview()`/`children(path:)` → `Overview()`/`Children(path)`；仍走 `/usr/bin/env mo analyze --json`，`PATH` 与 `TERM=dumb` 和 Swift 完全一致 |
| `Sources/MoleStudio/Engine/MoleCLIRunner.swift` | `internal/analyze/bridge.go` `MoBinaryPath()` | **只迁移了可执行文件解析**；探测顺序相同（`/opt/homebrew/bin/mo`、`/usr/local/bin/mo`、`~/.local/bin/mo`、`~/.mole/mo`，落到 `/opt/homebrew/bin/mo`）。Swift 里多探的两条 `Bundle/*/resources/mole-cli/mo` 没迁——本仓库不附带 `mo` |
| `Sources/MoleStudio/PowerStatus.swift` | `internal/power/status.go` | IOKit → `ioreg` 文本解析，见 §2.4 |
| `Sources/MoleStudio/MoleStudioApp.swift` `configureStatusItem()` | `internal/trayicon/trayicon.go` | AppKit 运行时绘制 → 解析式 SDF 算 template PNG，见 §2.5 |
| `Sources/MoleStudio/Engine/NativeSystemMonitor.swift` | 未迁移 | 死代码，见 §3.3 |
| `Sources/MoleStudio/Views/VisualEffectAndPlanetView.swift` | 未迁移 | 死代码，见 §3.3 |

### 1.2 应用外壳：仓库根 `package main`

MyGo 要求入口在模块根目录，所以外壳是根下几个文件，而不是一个 `internal/app` 包。

| Swift | Go | 说明 |
| --- | --- | --- |
| `@main enum MoleStudioMain` | `main.go` | 17 行，`func main() { run() }`，别的什么也没有 |
| `MoleStudioApp.swift` `AppDelegate`（窗口、托盘、flyout、菜单、遥测定时器、`applicationShouldHandleReopen`） | `app.go` | `run()`/`buildUI()`/`createTray()`/`ensureFlyout()`/`startTelemetryTimer()`/`buildApplicationMenu()` |
| `MoleStudioApp.swift` `UILocator`（三种启动模式） | `app.go` | 改由 MyGo 的 `NewWindow`（主窗）+ `NewFlyout`（`Popover: true`）表达，不再手写 locator |
| `MoleStudioApp.swift` `NativeBridgeController` + `userContentController(_:didReceive:)` | `bridge.go` | 一个 `NativeBridge.Dispatch`，见 §2.2 |
| `MoleStudioApp.swift` 的 WKWebView 配置（`WKUserContentController`、注入的 flag、拖拽区域） | `preload.go` | `PageOptions.PreloadScript`，见 §2.1 |
| `MoleStudioApp.swift` 里所有 `evaluateJavaScript` 调用点 | `push.go` | 序列化 payload + 调页面全局函数，见 §2.3 |
| `MoleStudioApp.swift` `applyTheme(_:remember:)` + `UserDefaults` | `theme.go` | `mygo.Theme` + 用户数据目录里的 `preferences.json`，见 §2.6 |

### 1.3 测试

| Swift | Go | 说明 |
| --- | --- | --- |
| `Tests/MoleStudioTests/SafetyModelTests.swift` (9) | `internal/model/model_test.go` (9) | 逐条对应，含 `TestProtectedCleanItemsAreNotSelectable`、`TestDryRunIsOnByDefault` |
| `Tests/MoleStudioTests/ScannerRefactorTests.swift` (3) | `internal/scanner/scanner_refactor_test.go` (3) | 签名表非空且唯一、payload 键不变、残留行字段对齐 UI |
| `Tests/MoleStudioTests/TreemapLayoutTests.swift` (8) | 未迁移 | 测的是死代码 `computeTreemapRects`，实际在跑的布局另有测试覆盖，见 §3.5 |
| — | `internal/analyze/bridge_test.go` (7)、`internal/power/status_test.go` (6)、`internal/scanner/scanner_numeric_test.go` (10)、`internal/trayicon/trayicon_test.go` (12) | 新增：Swift 版没有覆盖解析器、数值助手和托盘图 |

Go 侧共 48 个测试：`analyze` 7、`model` 9、`power` 6、`scanner` 14、`trayicon` 12。

## 2. 关键决策

### 2.1 前端 `index.html` 一行不改

`Resources/src-ui/index.html`（单文件，内联 CSS/JS）只认一个入口——`sendNativeBridge()`：

```javascript
if (window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.moleNative) {
  try { window.webkit.messageHandlers.moleNative.postMessage(payload); } catch (e) {}
}
```

它没有 import `mygo-runtime`（`grep mygo-runtime` 在 index.html 里 0 命中），所以 MyGo 生成的 `mygo.ts` 和 `call()` 对页面**不可见**：页面不会主动换成 `call("NativeBridge.X")`。

决策：用 MyGo 的 `PageOptions.PreloadScript`（"injected into every page before the page's own scripts, after `window.mygo` is available"）注入兼容层，把 `window.webkit.messageHandlers.moleNative.postMessage` 接到 `NativeBridge.Dispatch`。页面侧一行不动，`hasBridge()` 照样为真，原生专属 UI 照常出现。

两个细节：

- `window.webkit` 已存在且不可配置，所以在它上面**新挂**一个 `messageHandlers` 属性，而不是替换 `window.webkit`。
- MyGo 自己占用了 `"mygo"` 这个 message handler 名，不能复用。

同一份 preload 还负责 `window.isNativeMacApp`、`window.isMenuBarPopover`（flyout 里）、`menubar-mode` classbody，以及主窗隐藏红绿灯、`.window-bar`/`.top-header` 的拖拽区域 CSS——这些在 Swift 版里散在 `MoleStudioApp.swift` 各处。

### 2.2 一个 `NativeBridge.Dispatch` 复刻 Swift 的 action switch

Swift 在 `MoleStudioApp.swift:46` 取 `dict["action"] as? String`，然后一个 16 个 `case` 分支、覆盖 18 个 action 字符串的 `switch`。Go 侧保留同样的"一个入口 + 按 action 分发"形状（而不是给每个 action 一个绑定方法），因为前端的 contract 就是 `{action, ...}` 这种无类型字典。

`bridge.go` 的 `Dispatch(payload map[string]any)` 分支与 Swift 逐条对齐，其中 `case "trashRealPaths", "executeClean", "trashPath":` 三个字符串共用一个分支（`executeClean`/`trashPath` 在 `index.html` 里没有调用方）。`default` 记下页面仍会发的 4 个 action——`addWhitelist`、`restoreTrashSnapshot`、`runDoctorReCheck`、`setFanMode`——Swift 版同样没有处理，状态由页面自己持有。

真正的实现（不是占位）：Finder 显示、剪贴板、`SIGTERM` → 400ms 后 `SIGKILL`、移到废纸篓、`qlmanage -r cache`、`caffeinate -d -i -t 7200`（持有一个 `*exec.Cmd`，再次调用时 `SIGTERM` 掉）、主题持久化、窗口缩放/拖拽、用 `Page().Eval` 执行 `switchModule('clean'); openConfirmSheet();` 这类菜单命令。

### 2.3 Go → 页面：事件必须手动转发到 `window.onNative*`

MyGo 的事件不走 `evaluateJavaScript`：它把消息 `eval` 成 `__mygo.receive([...])`，而 `packages/bridge/src/runtime.ts` 只把 `msg.t === "event"` 交给 `listeners.get(msg.n)`——只有 import 了 `mygo-runtime` 并调过 `runtime().on(name, fn)` 的页面才收得到。从未 import 的 index.html 收不到。

所以 `push.go` 不用事件，直接序列化后调全局函数：

```
scan-complete  →  window.onNativeRealScanComplete(payload)
telemetry      →  window.onNativeRealTelemetry(payload)
disk-analyze   →  window.onNativeDiskAnalyze(payload)
```

`evaluate()` 会同时打到主窗和（可见的）flyout，因为两边是独立的页面实例。三个 `Request*` action 都落到这里：`RequestRealTelemetry` → `scanner.BuildTelemetry()`，`RequestRealSystemScan` → `scanner.BuildFullScanPayload()`，`RequestDiskAnalyze` → `analyze.Overview()`/`analyze.Children()`。跑在 goroutine 里，因为读盘会阻塞 UI。

### 2.4 电池走 `ioreg`，不用 IOKit

Swift 用 `IOServiceGetMatchingService` + `IORegistryEntryCreateCFProperties` 读 `AppleSmartBattery`。Go 侧用 `purego` 绑 IOKit 的收益不抵风险，改成 `ioreg -rn AppleSmartBattery` 抓文本再解析。字段映射保持 Swift 的语义：

| 输出 | ioreg 键 |
| --- | --- |
| `percent` | `CurrentCapacity` / `MaxCapacity`（>100 截到 100） |
| `healthPct` | `NominalChargeCapacity`（缺失时退 `AppleRawMaxCapacity`）/ `DesignCapacity` |
| `minutes` | `TimeRemaining`（`<= 0` 或 `>= 6000` 视为"还在算"，省略） |
| `adapterWatts` | `AdapterDetails` 里的 `Watts`（嵌套字典，从 ioreg 文本里切出来；少数机型有扁平 `Watts`，先试扁平） |
| `charging` / `plugged` / `full` | `IsCharging` / `ExternalConnected` / `FullyCharged`（`"Yes"` 判定） |

没有电池的机器返回 `Battery == nil`，JSON 里 `battery` 整个键消失（`omitempty`），与 Swift 的 `result["battery"] = nil` 等价。

### 2.5 托盘图标：Go 侧现算，不用 PNG

Swift 在 `MoleStudioApp.swift:252` 用 CoreGraphics 现画一枚 18pt 单色 template 图（把 app icon 的球/轨道/伴星压到 64 单位网格、用留白描边切开）。Go 侧没有 AppKit，`internal/trayicon` 把同一套几何写成解析式：球/环/伴星用 SDF 求值、边缘做抗锯齿，输出 36×36（18pt @2x）的 template PNG，每像素纯黑、形状全在 alpha 里。

`PNG(36)` 在启动时算一次喂给 `mygo.NewTray`（`IconIsTemplate: true`）。这样菜单栏标记和 app icon 一样仍是代码生成物，不需要往仓库里塞第二份图片；`assets/icon.png` 只用于 bundle icon。12 个测试锁住几何（中心不透明、四角透明、环被球切掉、右边缘不该有环等）。

### 2.6 主题用 `mygo.Theme`

`setTheme` 的三个值直接映射：

| 前端 | Swift | Go |
| --- | --- | --- |
| `dark` | `NSApp.appearance = NSAppearance(named: .darkAqua)` | `mygo.Theme.SetSource(mygo.ThemeDark)` |
| `light` | `.aqua` | `mygo.Theme.SetSource(mygo.ThemeLight)` |
| `system` | `nil` | `mygo.Theme.SetSource(mygo.ThemeSystem)` |

`UserDefaults` 换成一个 JSON 文件：`mygo.App.Path(mygo.PathUserData)` 下的 `preferences.json`，只存 `{"theme": "dark"}`；选 `system` 就删掉文件。窗口背景色跟着切（`#111110` / `#faf8f5`，取自 `index.html` 的 `--bg-window`），这样页面加载期间原生底色不跳。

## 3. 与 Swift 版的已知差异

### 3.1 系统要求不变

`mygo.json` 里 `macos.minimumSystemVersion` 和 `infoPlist.LSMinimumSystemVersion` 都写 `13.0`，`Package.swift` 的 `.macOS(.v13)` 语义保留。

### 3.2 thermal 走 Foundation 的 `NSProcessInfo.thermalState`，和 Swift 同源

Swift 用 `ProcessInfo.processInfo.thermalState`（`.fair`/`.serious`/`.critical`/default → `nominal`）。
Go 标准库没有等价 API，但**不需要**退化成假数据：`internal/power/thermal_darwin.go` 用 purego 直接调
Objective-C 运行时问 Foundation 要同一个值：

```go
purego.Dlopen("/System/Library/Frameworks/Foundation.framework/Foundation", RTLD_GLOBAL|RTLD_NOW)
info := objc.ID(objc.GetClass("NSProcessInfo")).Send(objc.RegisterName("processInfo"))
state := int(objc.Send[uintptr](info, objc.RegisterName("thermalState")))  // 0 nominal / 1 fair / 2 serious / 3 critical
```

关键点：纯 Go 二进制不 link Foundation，`objc.GetClass("NSProcessInfo")` 会返回 0 —— 必须先 `Dlopen`
把 framework 装进进程。这也是 MyGo 自己的 darwin 后端（`internal/darwin/objc.go`）做的事。

取不到时按 `sysctl -n kern.thermal_pressure_level`（1/2/3 → fair/serious/critical）兜底，再取不到才
`nominal`。**注意兜底那条在本机是无效的**：

```
$ sysctl -n kern.thermal_pressure_level
sysctl: unknown oid 'kern.thermal_pressure_level'
```

`sysctl -a | grep -i thermal` 也搜不到这个 OID，`pmset -g therm` 返回 "No thermal warning level has been
recorded"。也就是说在 macOS 27.2 / MacBookPro18,4 上真正生效的只有 NSProcessInfo 这一条，
`TestAppKitThermalStateIsReachable`（`internal/power/thermal_darwin_test.go`）守着它不被悄悄降级。

`kern.memorystatus_vm_pressure_level` 正常（本机返回 `1` → `normal`），内存压力这条没退化。

### 3.3 进程 `path` 是可执行文件路径，不是 app bundle 路径

Swift 拿 `NSRunningApplication` 把属于某个 `.app` 的进程 `path` 替换成 bundle 路径，所以"在 Finder 中显示"打开的是应用本身。

MyGo 没有 `NSRunningApplication` 等价物，Go 侧直接用 `/bin/ps -Aceo pid,%cpu,rss,comm -r` 给出的 `comm`，也就是**可执行文件**的路径（如 `/Applications/Xcode.app/Contents/MacOS/Xcode`）。同一个文件，只是指向可执行文件而非 `.app` 目录；`revealInFinder` 的兜底逻辑（路径不存在就显示父目录）能正常落到 `Contents/MacOS` 或 `.app` 上。要完全还原就得调 `NSWorkspace`/`NSRunningApplication` 或自己从路径里截 `*.app/` 前缀。

### 3.4 未迁移的死代码（在原项目里就没有调用方）

| 文件 | 内容 | 判定依据 |
| --- | --- | --- |
| `Sources/MoleStudio/Views/VisualEffectAndPlanetView.swift` | `VisualEffectView`、`PlanetCanvasView` | 全仓库 grep `VisualEffectView\|PlanetCanvasView\|PlanetView` 除定义文件外 0 命中 |
| `Sources/MoleStudio/Engine/NativeSystemMonitor.swift` | `NativeHardwareMetrics`、`NativeSystemMonitor.sampleMetrics()`、`CleanScreenShieldController` | grep 四个符号除定义文件外 0 命中 |
| `Sources/MoleStudio/Engine/MoleCLIRunner.swift` | `execute(_:requiresAdmin:)`、`computeTreemapRects(entries:)`、`setKeepScreenOn(seconds:)`，以及 `saveWhitelist(_:)` | `execute` 唯一调用方是 `MoleViewModel.runCLISubcommand`（`MoleViewModel` 自身只有测试在构造）；`computeTreemapRects` 只被 `Tests/MoleStudioTests/TreemapLayoutTests.swift` 调用；`setKeepScreenOn` 无调用方 |

`MoleViewModel` 本身同样只有测试调用，但因为它是模型层的门面，仍然按职责迁成了 `internal/model/viewmodel.go`。上表三类没有迁——`internal/scanner` 里没有任何对应物。

顺带一个 Swift 版就存在的悬空承诺：`MoleCLIRunner.swift:31` 定义了 `~/Library/Logs/mole/operations.log` 并建目录，但全仓库没有任何地方往里写。Go 版不再造这个文件，清理历史只认 `mo history`。

### 3.5 树图布局：真正在跑的路径一模一样，差别全在死代码上

界面吃的 `treemap2D` 一直是 `MoleAnalyzeBridge.layout()` 产的**横条布局**，Go `analyze.layout`
（`internal/analyze/bridge.go:151`）逐行照搬：过滤 `name`/`path` 为空和 `size <= 0` 的条目、`basis` 取
`total_size`（≤0 时退化成累加）、`w = max(6, pct)` 后 `x` 累加、`y = 0`、`h = 100`、按
`cleanable → isDir → 其它` 三档取同一串渐变色、路径把 home 换成 `~`、`pct` 四舍五入到 0.1。唯一可察觉的
实现差异是路径处理：Swift 用 `replacingOccurrences(of: home, with: "~")`（任意位置替换），Go 用 `relativize`
只替换前缀 —— 实际路径都以 home 开头，输出一致。

没迁的是 `MoleCLIRunner.computeTreemapRects`（`MoleCLIRunner.swift:144`）那套**交替方向递归切分**（按 size
降序、`clampedRatio` 夹在 0.22–0.75、`verticalSplit` 逐层翻转、6 色轮转、0–1 归一化矩形）。它是死代码：
唯一调用方是 `TreemapLayoutTests`，`MoleCLIRunner.execute` 那条链路从来没人用。所以 `TreemapLayoutTests`
的 8 条不变性断言（铺满、不重叠、最大项拿最大块、百分比求和≈100、零值不产生 NaN）没有对应物，Go 侧只留
`TestLayoutAddsUpPositions`（断言每块从上一块结束处开始）覆盖实际在跑的那条路径。

结论：**观感没有变化**。如果哪天想换成「铺满方框的树图」，`computeTreemapRects` 还在 git 历史里
（`git show adea952:Sources/MoleStudio/Engine/MoleCLIRunner.swift`）。

### 3.6 二进制 plist 的 Info.plist 读不出内容

Swift 用 `Bundle.infoDictionary`，两种 plist 格式都吃。Go 的 `parsePlist`（`internal/scanner/plist.go`）只处理 XML：开头不是 `<?xml` 或 `<plist` 就直接返回空 map。

实测本机 `/Applications` 下 64 个 `.app` 里有 8 个是二进制 plist（GarageBand、Keynote、Muse、Numbers、Pages、Xcode、iMovie、wpsoffice）。这些应用在卸载列表里会退化成用 `.app` 文件名当显示名、用 `local.app.<name>` 当 bundle id，于是 `~/Library` 里按真实 bundle id 命名的缓存目录匹配不到——**卸载器的残留体积会偏小**。要修就得加一个二进制 plist 解析器（bplist00 格式）或调 `plutil -convert xml1 -o -`。

### 3.7 `.dmg` 在受限环境里生成不了

`mygo build` 打完 `.app` 之后调 `hdiutil create -fs HFS+ -format UDRW`。在没有卷管理权限的沙箱里：

```
error: hdiutil create ... : exit status 1
hdiutil: create failed - 操作不被允许
```

这是环境限制而非仓库问题（同一台机器上手工 `hdiutil create` 也失败）。`make app-nodmg` 可以绕开它。

## 4. 验证步骤

所有命令都要先给 Go 一个可写的 module/build cache（默认的 `$HOME/go/pkg/mod` 在沙箱里通常只读）：

```bash
export GOPATH=/tmp/gopath GOMODCACHE=/tmp/gopath/pkg/mod GOCACHE=/tmp/gocache
export PATH="/tmp/gopath/bin:$PATH"   # go install 出来的 mygo CLI 落在这
```

`mygo` CLI 不随仓库提供，版本对齐 `go.mod`：

```bash
go install github.com/egoist/mygo/cmd/mygo@v0.6.4
```

### 4.1 静态检查与测试

```bash
make vet     # go vet ./...，预期无输出、退出码 0
make test    # go test ./... -count=1
```

`make test` 预期：

```
?   github.com/mymoles/mole-studio        [no test files]
ok  github.com/mymoles/mole-studio/internal/analyze
ok  github.com/mymoles/mole-studio/internal/model
ok  github.com/mymoles/mole-studio/internal/power
ok  github.com/mymoles/mole-studio/internal/scanner
ok  github.com/mymoles/mole-studio/internal/trayicon
```

`make check` 就是这两条串起来，CI 该跑的就是它。

### 4.2 开发热重载

```bash
make dev     # 需要 mygo CLI，见上文 go install
```

它把前端和 Go 代码打进一个临时 bundle `.mygo/dev/darwin-arm64/MoleStudio Dev.app` 并启动，然后监听整棵树：保存 `Resources/src-ui/index.html`（或任意 `.go` 文件）会在约 5 秒内重建并重启应用，日志里是

```
○ Rebuilding MoleStudio Dev (resources/src-ui/index.html changed)
✓ Rebuilt MoleStudio Dev (resources/src-ui/index.html changed)  5.3s
```

这条路径**不需要** `MYGO_FRONTEND_DIST`：`mygo dev` 自己生成内嵌前端，`.mygo/` 已在 `.gitignore` 里。

### 4.3 本地可执行文件

```bash
make run     # go run .，等价于 MYGO_FRONTEND_DIST=Resources/src-ui go run .
```

`make run` 会开一个 1380×860 主窗口、一个菜单栏托盘和 356×480 的 flyout，页面通过 preload 的 shim 拿到原生数据。

**为什么需要 `MYGO_FRONTEND_DIST`。** 前端正常是 `mygo build` 时生成 `mygo_frontend_gen.go` 再 `mygo.SetFrontend` 内嵌进二进制的，而这个生成文件是 gitignore 的，所以纯 `go run` / `go build` 里没有任何前端资源，`mygo://localhost/` 会返回 MyGo 自带的 `No frontend` 页面。MyGo 留了 dev 回退：`IsDev()`（默认成立，除非 `MYGO_ENV=production` 或 `MYGO_PRODUCTION=1`）且没有内嵌 FS 时，用 `MYGO_FRONTEND_DIST` 指向的目录当 `os.DirFS`。Makefile 的 `run` 目标已经带上了这个变量。

```bash
make build   # go build -v -o mole-studio .
MYGO_FRONTEND_DIST=Resources/src-ui ./mole-studio   # 不带变量就只能看到 No frontend 页
```

产物是仓库根目录下的 `mole-studio`（约 14 MB）。

### 4.4 打包

```bash
make doctor      # 工具链自检，预期 go / bun / 平台三项 ✓；codesign、makensis、osslsign 找不到是正常的
make app-nodmg   # 只出 .app
make app         # .app + .dmg
```

产物在 **`build/darwin-arm64/`**（MyGo 按 `<goos>-<goarch>` 分子目录，不是 `build/` 根下）：

```
build/darwin-arm64/MoleStudio.app
build/darwin-arm64/MoleStudio 0.1.0 arm64.dmg   # 仅 make app
```

两个坑：

1. **`mygo.json` 的 `resources` 不能重复列图标。** 同时配了 `icon` 又在 `resources` 里列 `AppIcon-256.png`/`AppIcon.icns` 会直接失败：
   - 同名撞车：`error: resources ... and ... would both be installed as AppIcon-256.png`（macOS 大小写不敏感）
   - 保留名撞车：`error: resource ... would replace the app's own AppIcon.icns`（`AppIcon.icns` 由 MyGo 从 `icon` 现场生成）

   正解是删掉整个 `resources` 数组；前端已由 `frontendDist` 内嵌，图标已由 `icon` 生成。
2. `make app` 的 dmg 步骤依赖 `hdiutil`，受限环境会失败，见 §3.7。

### 4.5 生成物

```bash
make generate    # 重新生成 mygo.ts（绑定变更后跑；mygo build 也会顺手跑一次）
```

预期刷新仓库根目录的 `mygo.ts`，抬头是 `// Code generated by \`mygo generate\`. DO NOT EDIT.`。绑定变更后若不提交它，CI 的 `git diff --exit-code -- mygo.ts` 会失败。

## 5. 实机联调发现并修掉的三处偏差

静态检查和单元测试都过不了这三关，它们只在真机上跑起来才暴露：单元测试的 fixture 是手写的，而真实工具的输出自带格式细节。

### 5.1 电池永远缺失：`ioreg` 的输出是树，属性行前面有 `|`

`parseIoreg` 原来要求去掉空白后的行以 `"` 开头，但真机的 `ioreg -rn AppleSmartBattery` 是这样：

```
  |   "CycleCount" = 266
```

开头是 `|`，于是每一行都被跳过，`batteryInfo()` 返回 nil，状态页的电池卡片永远不显示。修法：先剥掉行首的 `|`，再用 `" = ` 定位 key/value 的边界（`strings.Index(line, `" = `)`），这样 key 内部和 value 里的引号都不用猜。嵌套字典（`AdapterDetails`）本来就跟 key 同一行，`nestedNumber` 照常工作。

教训：fixture 必须从真机输出原样拷贝，手写的「看起来一样」的样本会把这类 bug 遮住。§2.4 的 `adapterWatts` 也是同一个坑——Swift 读的是嵌套的 `AdapterDetails.Watts`。

### 5.2 `mo analyze` 立刻失败：清空环境变量把 `HOME` 也清掉了

`runAnalyze` 原来用 `command.Env = []string{"PATH=…", "TERM=dumb"}` **替换**整个环境。`mo` 是 bash 脚本且开了 `set -u`，没有 `HOME` 时第一句就死：

```
/opt/homebrew/Cellar/mole/1.56.1/libexec/lib/core/base.sh: line 168: HOME: unbound variable
```

进程 26 毫秒退出，`exec.Command.Output()` 返回 err，代码把 err 吞成 `source: "mo analyze unavailable"`，界面上只是磁盘分析一直空着。Swift 版是 `var env = ProcessInfo.processInfo.environment; env["PATH"] = …; env["TERM"] = "dumb"; proc.environment = env`，即**继承再覆盖**。修法：`mergedEnv(os.Environ(), "PATH=…", "TERM=dumb")`，保留继承语义，只替换这两个 key。

### 5.3 每 1.5 秒一条页面 `Script error.`：`Page.Eval` 收的是表达式

MyGo 的 `Page.Eval` 先按**表达式**编译（可以取返回值），语法错误才退回函数体再试一次。`if(window.onNativeTelemetry) window.onNativeTelemetry(…);` 是语句，第一次必然语法失败，WebKit 把每次失败都记成一条来自 `:0:0` 的跨域 `Script error.`——正好跟着 1.5 秒的遥测定时器走，一次推送两条求值。改成条件表达式后一次编译成功：

```js
window.onNativeRealTelemetry ? window.onNativeRealTelemetry(<json>) : undefined
```

实测页面错误从 33 条降到 0 条。另外 `evaluate()` 现在会在 `MOLE_DEBUG=1` 时把 `Eval` 返回的错误打到 stderr——这类失败以前是完全静默的（5.2 就是靠它一句话定位的）。

## 6. 界面联调记录

在没有「辅助功能」权限的机器上也能驱动真机界面：让真实二进制去加载一份带探针的页面副本即可。`MYGO_DEV=1` 时 MyGo 的 `devURL()` 返回 `MYGO_DEV_URL`（`mygo@v0.6.4/frontend.go:46`），于是 Go 侧一行不改，页面、桥、数据全是真的：

```bash
MYGO_DEV=1 MYGO_DEV_URL=http://127.0.0.1:8765 ./build/darwin-arm64/MoleStudio.app/Contents/MacOS/MoleStudio
```

（`mygo dev` 自己会设这两个变量；直接跑打包件时也要设 `MYGO_DEV=1`，否则 `IsDev()` 为假、窗口仍走 `mygo://`。）

### 6.1 24 步交互结果

| 检查项 | 结果 |
| --- | --- |
| 8 个模块切换 | `module` 与侧栏 `navActive` 全部一致 |
| 主题三连（浅/深/跟随系统） | light → dark → light → dark，无残留 |
| 清理勾选 | 4 项 1.30 GB → 3 项 0.38 GB → 3 项 1.20 GB |
| 「仅看可清理」筛选 | 分组卡 4 → 1 |
| 二次确认弹层 / 系统联检弹层 / 命令面板 | 三个都正常弹出、内容正确 |
| 搜索过滤 | 卡片 22 → 21，正文 1086 → 967 字 |
| 状态页刷新 | 内存 66.4% ↔ 66.7%、已用 21.2 ↔ 21.3 GB 每 1.5 s 跳动 |
| 残缺文本扫描 | 每一步的 `badText` 都为空（无 `undefined`/`NaN`/`[object Object]`） |
| 页面错误 | 0 条 |

截图存档（`/build/` 被 gitignore）：`build/ui-verify/01..25-*.png` 与四张拼版 `v1..v4.png`。

### 6.2 性能

| 指标 | 实测 |
| --- | --- |
| `DOMContentLoaded` / `FCP` | 182 ms / 171 ms |
| 首次遥测到达 | 238 ms |
| 首次全盘扫描 | 3.6–6.8 s（冷启动时更长） |
| `mo analyze --json` | 30.4 s，异步，不挡主线程 |
| 主线程长任务（>50 ms） | 0 个 |
| 滚动帧率 | 62 fps（窗口可见时） |
| 驻留内存 | 主进程 140 MB + WebContent 75 MB + GPU 47 MB + Networking 9 MB ≈ 271 MB |
| 1.5 s 遥测一跳的代价 | `ioreg` 18.1 ms + `vm_stat` 7.0 ms + `sysctl` 6.1 ms ≈ 32 ms（约 2% 单核），与 Swift 版同源同频 |

沙箱里 `/bin/ps` 和 `/usr/bin/top` 都被拒（`Operation not permitted`），所以进程列表为空、也拿不到 `ps` 口径的 CPU 占比；进程号与驻留内存改用 `proc_pid_rusage` 读（见下），子进程的真实耗时是直接 `time` 出来的。

```bash
# 这台机器上 proc_pid_rusage 是唯一能拿到单进程数据的口径
swift rusage.swift <pid> [样本数] [间隔ms]
```

### 6.3 两个不算 bug 但值得知道的界面状态

- 空间分析在 `mo analyze` 返回前一直显示「正在扫描」骨架屏，本机约 30 秒。Swift 版走同一个 `mo` 调用，等待时间一样长。
- 构建产物页在扫描结果为 0 项时，右侧面板仍会画出占位骨架行，读起来像还在加载。这是前端原有的空态画法，迁移没有改动。
