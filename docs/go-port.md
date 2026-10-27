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
| `Tests/MoleStudioTests/TreemapLayoutTests.swift` (8) | 未迁移 | 布局算法换了，见 §3.4 |
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
| `adapterWatts` | `Watts` |
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

### 3.2 thermal 状态换了数据源，且在本机拿不到值

Swift 用 `ProcessInfo.processInfo.thermalState`（`.fair`/`.serious`/`.critical`/default → `nominal`）。Go 没有等价 API，改读 `sysctl -n kern.thermal_pressure_level`，1/2/3 映射 fair/serious/critical、其余 nominal。

**实测问题**：在 macOS 27.2 / MacBookPro18,4 上

```
$ sysctl -n kern.thermal_pressure_level
sysctl: unknown oid 'kern.thermal_pressure_level'
```

`sysctl -a | grep -i thermal` 也搜不到这个 OID。命令失败时 `thermalLevel` 保持 0，`thermalLabel()` 恒返回 `"nominal"`——也就是说**在 Apple Silicon 上这会一直显示 nominal**，而 Swift 版能反映真实热压力。要修就得回到 IOKit（`IOHIDEventSystemClient`/私有 API，风险高）或解析 `pmset -g therm`（本机输出 "No thermal warning level has been recorded"，同样无值）。目前选择保留降级：宁可为 null 语义，不要假数据。

`kern.memorystatus_vm_pressure_level` 是正常的（本机返回 `1` → `normal`），内存压力这条没退化。

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

### 3.5 树图布局算法不同

Swift `computeTreemapRects`（`MoleCLIRunner.swift:144`）是**交替方向递归切分**：先按 `size` 降序，每次把首项切出来，`clampedRatio` 夹在 0.22–0.75，`verticalSplit` 逐层翻转，6 色调色板轮转，坐标是 0–1 归一化矩形。

Go `analyze.layout`（`internal/analyze/bridge.go`）是**从左到右的等比条带**：按 `mo` 返回的原序，`w = max(pct, 6)`，`x` 累加，`y = 0`，`h = 100`，颜色只按 cleanable/isDir 分三档渐变。

两者都能被 UI 吃下去（`index.html` 用 `left:${r.x}%;top:${r.y}%;width:${r.w}%;height:${r.h}%` 定位），但观感从"铺满方框的树图"变成了"一排横条"。`TreemapLayoutTests` 的 8 条不变性断言（铺满、不重叠、最大项拿最大块、百分比求和≈100、零值不产生 NaN）因此没有对应物；Go 侧只保留了 `TestLayoutAddsUpPositions`（断言每块从上一块结束处开始）。

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

### 4.2 本地可执行文件

```bash
make build   # go build -v -o mole-studio .
./mole-studio
```

预期产出仓库根目录下的 `mole-studio`（约 14 MB）。它会开一个 1380×860 主窗口、一个菜单栏托盘和 356×480 的 flyout，页面通过 preload 的 shim 拿到原生数据。

### 4.3 打包

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

### 4.4 生成物

```bash
make generate    # 重新生成 mygo.ts（绑定变更后跑；mygo build 也会顺手跑一次）
```

预期刷新仓库根目录的 `mygo.ts`，抬头是 `// Code generated by \`mygo generate\`. DO NOT EDIT.`。绑定变更后若不提交它，CI 的 `git diff --exit-code -- mygo.ts` 会失败。
