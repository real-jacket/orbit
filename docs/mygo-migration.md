# MyGo 架构迁移方案

> **状态**：本文是迁移**开工前**的框架分析与实施计划。迁移已经完成，`Sources/` 下的 Swift 实现已删除。
> 实际落地的文件级对照、与 Swift 版的差异和验证方式在 [`go-port.md`](go-port.md)；
> 下文 §1 描述的是迁移**前**的项目、§4 的阶段划分与代码片段是当时的计划。
> 计划与最终实现的出入集中在 **§4 末尾的「计划 vs 实际」对照表**，读 §3.3 时请以该表为准。

## 1. 当前项目架构概览

> 本节记录的是迁移前的 Swift 实现，仅作对照用。


### 1.1 技术栈
- **语言**: Swift 5.10+
- **框架**: SwiftUI (仅用于部分视图) + AppKit + WKWebView + IOKit
- **UI 结构**: AppKit 窗口 + WKWebView + 单一 3830 行 `Resources/src-ui/index.html` (内联 CSS/JS, 无构建步骤)
- **通信**: `postMessage` / `evaluateJavaScript` 双向桥接 (`window.webkit.messageHandlers.moleNative`)
- **后端依赖**: 运行时解析 PATH 中的 `mo` CLI (`brew install mole`, 版本 1.56.1)
- **打包**: `swift build` → `.app` → DMG; macOS 13+
- **许可证**: GPL-3.0

### 1.2 核心模块
| 模块 | Swift 文件 | LOC | 功能 |
|------|-----------|-----|------|
| 应用外壳 | `MoleStudioApp.swift` | 497 | 主窗口/菜单栏托盘/弹出层/主题同步/原生菜单 |
| 扫描引擎 | `RealMacSystemScanner.swift` | 469 | 目录大小统计/已安装应用扫描/大文件扫描/垃圾回收 |
| 分类清理 | `CatalogScanning.swift` | 140 | 4 类用户缓存 +3 类 AI 缓存 + 安装包 + 构建产物 |
| 孤立残留 | `OrphanScanning.swift` | 129 | Docker/Android Studio/JetBrains/Parallels签名匹配 |
| 静态数据 | `OrphanSignatures.swift` | 86 | 4 种软件残留模式定义 |
| CLI 桥接 | `MoleAnalyzeBridge.swift` | 114 | `mo analyze --json` 执行与树图布局计算 |
| 系统状态 | `PowerStatus.swift` | 112 | 电池/内存压力/热状态 |
| 测试 | 3 个测试集 | 2535 | 模型/Safety/布局不变性验证 |

### 1.3 前端 - 原生桥接契约
**Outbound (frontend→native)** — 20 种操作:
```text
addWhitelist, copyText, executeOptimize, killProcess, 
menubarOpenClean, menubarOpenDoctor, menubarQuit, menubarShowMain, 
requestDiskAnalyze, requestRealSystemScan, requestRealTelemetry, 
restoreTrashSnapshot, revealInFinder, runDoctorReCheck, setFanMode, 
setTheme, startWindowDrag, toggleCaffeinate, toggleWindowZoom, 
trashRealPaths
```
Inbound (native→frontend) — 4 个全局回调:
```javascript
window.onNativeRealScanComplete(payload)
window.onNativeRealTelemetry(payload)
window.onNativeDiskAnalyze(payload)
window.stepLiveTelemetry() // 每 1.5s 间隔调用
```
旗帜变量控制真实数据开关: `window.isNativeMacApp`, `window.isMenuBarPopover`

---

## 2. MyGo 框架分析

### 2.1 MyGo 是什么？
- **仓库**: https://github.com/egoist/mygo (MIT, 3192 stars, v0.6.4, 活跃更新至 2026-10-26)
- **定位**: Pure Go (无 cgo), 将网页嵌入系统 WebView (macOS=WKWebView/Windows=WebView2/Linux=WebKitGTK)
- **输出**: 单二进制几 MB, 可打包为 `.app` + DMG
- **两种 UI**:
  - (a) **Web 模式**: 任意前端工具链构建 HTML/CSS/JS, Go 作为后端通过 TypeScript 客户端调用
  - (b) **Native UI**: `ui` 包绘制 GPU 加速的原生界面 (本项目不使用)

### 2.2 核心能力验证

#### ✅ 前端嵌入机制
```go
mygo.SetFrontend(fsys fs.FS)   // 服务在 mygo://localhost/
// mygo build 自动生成 embed 代码
```
- extensionless 路径未命中 → index.html (支持 SPA 路由)
- `prelodeScript` 选项: 在 `window.mygo` 之后、页面脚本之前注入 JS → **这是让 index.html 保持不变的关键钩子**

#### ✅ Bridge Transport
- 同当前 Swift 的 `webkit.messageHandlers` 机制 (`moleNative` ↔ `mygo`)
- PreloadScript 注入后创建 `window.webkit.messageHandlers.mygo` 转发器
- 消息结构 `{t:"call", id:s, k:n, m:method, a:[...]}` + secret
- TS 客户端通过 `mygo generate` 生成，调用 `call("Service.Method", args...)`

#### ✅ Bindings 方法命名
- `mygo.Bind(svc)` 暴露 `TypeName.methodName` (**保持原始大小写**, 非小驼峰)
- 首参数可选 `context.Context` (页面关闭时取消), 其余参 JSON 编码
- 返回类型: nothing/value/error/value+error; 每调用走独立 goroutine

#### ✅ Events Go→Page
- `mygo.NewEvent[T]("progress")` 定义事件
- `Emit(w *Window)`, `EmitPage(p *Page)`, `Broadcast()` 三种推送
- "mygo:" 前缀保留 (避免冲突); DOMContentLoaded 前的事件延迟到后发送
- Frontend: `events.progress.on(p => ...)`

#### ✅ Window API
- `NewWindow(WindowOptions{Title,URL,Width,Height,Frameless,TitleBarStyle="hidden",BackgroundColor:"light-dark(...)"})`
- Drag region via CSS `--app-region: drag` → 自动调用 `StartDrag()`
- `OnWindowAllClosed(fn)` 监听: 不注册则最后窗口关闭时退出应用 (**需空监听以匹配 Swift 行为**)
- `SetBackgroundColor()` 支持 `light-dark()` CSS 语法

#### ✅ Flyout = NSPopover
- `NewFlyout(FlyoutOptions{Tray:*Tray,Anchor,Placement,BottomStart,...,Width:200,Height:200})`
- Tray anchor → "浮于其他应用上方" → 完全等价 `NSPopover.behavior=.transient`
- 强制 `UseContentSize=true`, `Frameless=true`; macOS 底层用 `AppKit NSPopover` (带箭头/毛玻璃/动画)

#### ✅ Tray Menu Bar Item
- `NewTray(TrayOptions{ID:"orbit",Icon:[]byte PNG(16x16pt),IconIsTemplate:true,ToolTip:"..."})`
- **必须**在 `App.WhenReady` 中调用 (否则 panic)
- 方法: `SetIcon/SetTitle/SetToolTip/SetMenu/OnClick/OnRight/OnAppearanceChange`
- `IconIsTemplate=true` → macOS 自动 tint 为黑/透明 (与当前 Swift 绘制的品牌 mark 一致)

#### ✅ Theme API
- `Theme.SetSource(ThemeDark/ThemeLight/ThemeSystem)`
- `IsDark()`, `OnUpdated(fn)`, `Preferences()`
- 替换 `NSApp.appearance = .darkAqua`

#### ✅ ShellModule
- `Shell.OpenExternal(url)`, `Shell.OpenPath(path)`, `Shell.ShowItemInFolder(path)`
- `Shell.TrashItem(path)` → `NSFileManager.trashItemAtURL` → Finder AppleScript fallback
- `Clipboard.WriteText(text)/ReadText()` → `NSPasteboard`

#### ✅ Paths
- `PathHome`, `PathAppData` (`~/Library/Application Support`), `PathCache`, `PathLogs`, `PathUserData`, `PathTemp`

#### ✅ Distribution
- `mygo build` → 签名的 `.app` + `<slug>-<version>-<arch>.dmg`
- Config: `mygo.json` (纯 JSON) OR `mygo.config.ts` (Bun) — **只能二选一**
- `macos.minimumSystemVersion` defaults `"12.0"`; 本项目需设置为 `"13.0"`
- `macos.signingIdentity="-"` (ad hoc), notarize 可选

---

## 3. 架构映射：Swift → Go

### 3.1 整体拓扑

```
┌───────────────────────────────────────┐
│         Resources/src-ui/index.html   │  ← 保持不变!
│    (3830 lines inline CSS/JS)          │
└──────────────────┬────────────────────┘
                   │ postMessage / evaluateJavaScript
                   │ window.webkit.messageHandlers.moleNative shim
                   ▼
┌───────────────────────────────────────┐
│            Preload Script             │  ← 新:转发 webkit.messageHandlers
└──────────────────┬────────────────────┘
                   │
                   ▼
┌───────────────────────────────────────┐
│        main.go + internal/*.go        │
│  ┌─────────────┬──────────────────┐   │
│  │ app shell   │ scan engine      │   │
│  │ • tray      │ • RealMacSystem  │   │
│  │ • flyout    │ • catalog        │   │
│  │ • menus     │ • orphan sig     │   │
│  │ • theme     │ • mo analyze     │   │
│  │ • telemetry │ • power status   │   │
│  └─────────────┴──────────────────┘   │
└──────────────────┬────────────────────┘
                   │ mo --json
                   ▼
         /opt/homebrew/bin/mo (CLI runtime)
```

### 3.2 模块对应表

| Swift | Go package | Notes |
|-------|------------|-------|
| `MoleStudioApp.swift` | `main.go` + `internal/shell.go` | 主入口 + 窗口/托盘/弹窗/菜单/主题 |
| `Models/MoleModels.swift` | `internal/model/models.go` + `internal/model/viewmodel.go` | 显示模型 + VM 逻辑 (不变) |
| `Engine/RealMacSystemScanner.swift` | `internal/scanner/scanner.go` | 目录统计/已装应用/大文件/树图 |
| `Engine/CatalogScanning.swift` | `internal/scanner/catalog.go` | 扩展方法 → `catalogGroups()` |
| `Engine/OrphanScanning.swift` | `internal/scanner/orphan.go` | 签名匹配 + 孤立容器 |
| `Engine/OrphanSignatures.swift` | `internal/scanner/signatures.go` | 静态数据 verbatim port |
| `Engine/MoleAnalyzeBridge.swift` | `internal/analyze/bridge.go` | `mo analyze` 执行 + 布局 |
| `Engine/PowerStatus.swift` | `internal/power/status.go` | ioreg/sysctl → payload |
| Tests/*Tests.swift | `*_test.go` | 3 套 test suites port |

### 3.3 关键技术决策

#### 3.3.1 **前端保持不变**
- 直接复用 `Resources/src-ui/index.html`
- 通过 `mygo.SetFrontend(fs)` embedding + PreloadScript 伪造 `window.webkit.messageHandlers.moleNative`
- 不需要任何前端构建工具；dev 模式可通过 `MYGO_FRONTEND_DIST` 从磁盘 serving

#### 3.3.2 **Bridge Shim** (preload script)
```javascript
// preload.js — injected after window.mygo exists
(function() {
  const mygoHandler = window.webkit?.messageHandlers?.mygo;
  if (!mygoHandler) return;
  
  const moleNative = {
    postMessage(msg) {
      mygoHandler.postMessage({
        t: 'call',
        id: crypto.randomUUID(),
        k: 'NativeBridge',
        m: msg.action,
        a: [msg]
      });
    }
  };
  Object.defineProperty(window, 'webkit', {
    value: { ...window.webkit, messageHandlers: { ...window.webkit?.messageHandlers, moleNative } },
    writable: false,
    configurable: true
  });
})();
```
- 转发 `postMessage({action, path,...})` → `mygo.NativeBridge.action(args)`
- 前端无需修改; 完全兼容现有 bridge contract

#### 3.3.3 **NativeBridge Go 服务**
```go
type NativeBridge struct{}

func (b *NativeBridge) StartWindowDrag() error
func (b *NativeBridge) ToggleWindowZoom() error
func (b *NativeBridge) RevealInFinder(path string) error
func (b *NativeBridge) CopyText(text string) error
func (b *NativeBridge) TrashRealPaths(paths []string) (int, error)
func (b *NativeBridge) KillProcess(pid int) error
func (b *NativeBridge) RequestRealSystemScan() error
func (b *NativeBridge) RequestDiskAnalyze(path string) error
func (b *NativeBridge) RequestRealTelemetry() error
func (b *NativeBridge) ExecuteOptimize() error
func (b *NativeBridge) ToggleCaffeinate() error
func (b *NativeBridge) MenubarOpenClean() error
func (b *NativeBridge) MenubarOpenDoctor() error
func (b *NativeBridge) MenubarShowMain() error
func (b *NativeBridge) MenubarQuit() error
func (b *NativeBridge) SetTheme(theme string, remember bool) error
```
- 所有方法通过 `mygo.Bind(NativeBridge{})` 暴露为 `NativeBridge.action`
- `context.Context` 第一参数用于页面关闭时 cancel long-running calls

#### 3.3.4 **事件推送** (Go→frontend)

> 计划：用 `mygo.NewEvent[T]` + `EmitPage`。**实际没用**，改成 `Page.Eval`，原因见 §4 末尾对照表。

```go
var TelemetryEvent = mygo.NewEvent[TelemetryPayload]("telemetry")
var ScanCompleteEvent = mygo.NewEvent[ScanPayload]("scan-complete")
var DiskAnalyzeEvent = mygo.NewEvent[DiskAnalyzePayload]("disk-analyze")

// 在 go routine 里每 1.5s push
go func() {
  ticker := time.NewTicker(1500 * time.Millisecond)
  for range ticker.C {
    payload := buildTelemetryPayload()
    TelemetryEvent.EmitPage(mainPage, payload)
  }
}()
```
- Frontend 接收: `window.mygo.events.telemetry.on(p => window.onNativeRealTelemetry(p))`
- 或 generated client: `events.telemetry.on(payload => window.onNativeRealTelemetry(payload))`

#### 3.3.5 **电源状态端口策略**
- **Battery**: 通过 `exec.Command("ioreg", "-rn", "AppleSmartBattery")` 解析 XML-like output (verified working)
- **Memory pressure**: `syscall.Sysctl("kern.memorystatus_vm_pressure_level")` → integer 4=critical,2=warn,else normal
- **Thermal**: Go 没有 `ProcessInfo.thermalState`，但可以用 purego 调 `objc_msgSend` 问 Foundation 要同一个值（纯 Go 二进制要先 `Dlopen` Foundation）；`sysctl kern.thermal_pressure_level` 只作兜底
- **CPU model**: `sysctlbyname("machdep.cpu.brand_string")` + cores count

#### 3.3.6 **托盘图标生成**

> 两个方案都没用：既没有 resize `AppIcon-256.png`，也不是「简化版」，而是**按 Swift 的几何算式**在纯 Go 里
> 重绘（`internal/trayicon`），见 §4 末尾对照表。

- 当前 Swift 是程序化 draw (64-unit grid sphere+orbit ring)
- MyGo 需要 `Icon []byte PNG` (16x16/32x32 ideal)
- 方案 A: 提取 `Resources/AppIcon-256.png` resize down to 32x32, 使用 existing asset
- 方案 B: 在 Go 内使用 `image/png` + canvas 重绘简化版 (推荐 A: simpler, maintain parity)
- `IconIsTemplate=true` → 自动 tint black

---

## 4. 实现路径（计划）

下面 9 个 Phase 是开工时的排序计划，实际执行顺序基本如此，但有若干处做法改了。逐条对照见本节末尾的
**「计划 vs 实际」**表 —— 以那张表为准，本节的勾选项只代表「计划里已经想清楚了」。

### Phase 1: 脚手架 (Day 1)
- ✅ `go mod init github.com/mymoles/mole-studio`
- ✅ `mygo.json`: name="MoleStudio", slug="mole-studio", identifier="com.mymoles.mole-studio", version="0.1.0", macos.minimumSystemVersion="13.0"
- ⚠️ 前端嵌入：实际没有手写 `mygo.SetFrontend`，MyGo 按 `frontendDist` 生成 `mygo_frontend_gen.go` 并自动调用
- ✅ Basic main: `App.Run()`, `NewWindow({URL:"/", Title:"orbit"}), OnWindowAllClosed(empty fn)`
- ✅ Verify dev mode works from disk (`MYGO_FRONTEND_DIST=Resources/src-ui`)

### Phase 2: Bridge shim (Day 1-2)
- preload 脚本注入 `window.webkit.messageHandlers.moleNative`（实际是根目录 `preload.go` 里返回 JS 字符串，
  没有单独的 `internal/bridge/preload.js`）
- 实现 `NativeBridge` 服务（实际是**一个** `Dispatch(payload)`，不是 14 个方法）
- Connect outbound (frontend→Go) via bindings
- Test: 页面发一条 `{action:"setTheme", theme:"dark"}`，观察 app 外观变化

### Phase 3: Telemetry push (Day 2)
- Port `PowerStatus.snapshot()` → `internal/power/status.go`
- Port `queryDiskSpace/queryMemoryUsage/sampleRealProcesses` → real system stats
- `Ticker(1.5s)` 推送（实际是 goroutine 里调 `pushRealTelemetryNow()`，走 `Page.Eval`）
- Verify frontend receives `onNativeRealTelemetry(payload)`

### Phase 4: Scanner engine (Day 3-5)
- Port `RealMacSystemScanner` → `internal/scanner/scanner.go`:
  - `fastDirectorySize(path,maxFiles)`
  - `formatBytes(bytes)`
  - `queryDiskSpace/queryMemoryUsage/queryCPUModel()`
  - `sampleRealProcesses()`
  - `scanInstalledApps()` → apps list
  - `scanLargeFiles()` → large files list
  - `buildFullScanPayload()` → full payload
- Port `catalog` groups, `aiMoon bands`, `installer packages`, `build artifacts`
- Port `scanOrphanedLeftovers()` with 4 signature patterns
- Verify against current output structure (keys preserved)

### Phase 5: Analyze bridge (Day 6)
- Port `MoleAnalyzeBridge` → `internal/analyze/bridge.go`
- `runMoAnalyze(path?)`: spawn `mo analyze --json [path]`, env PATH=/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin TERM=dumb
- `parseJSON(stdout)` → treemap2D layout (keep exact gb()/color logic)
- Verify tree map rendering unchanged

### Phase 6: Flyout & menus (Day 7)
- Add `NewTray({...Icon:PNG...})` in `WhenReady`
- Build native menu: "关于","隐藏","退出"; edit; window
- 实际：`NewFlyout({Tray:tray, Placement:PlacementBottom, Width:356, Height:480, Popover:true, Hidden:true, CornerRadius:12, URL:"/?mode=menubar"})`
  —— `Popover:true` 才是原生 NSPopover（材料、圆角、箭头、失焦自动关），`Hidden:true` 避免首帧尺寸闪动
- Verify popover shows same module view as Swift

### Phase 7: Theme sync (Day 8)
- Connect `Theme.OnUpdated(fn)` to frontend theme update
- `mygo.Theme` **不做持久化**，所以自己存：`PathUserData/preferences.json` 的 `theme` 字段
- Verify dark/light/system modes propagate correctly

### Phase 8: Tests (Day 9)
- Port `SafetyModelTests.swift` → `internal/model/model_test.go`（9 条逐条对应）
- Port `ScannerRefactorTests.swift` → `internal/scanner/scanner_refactor_test.go`（3 条）
- `TreemapLayoutTests.swift` → **没有对应**：布局算法换成了 `internal/analyze` 的顺序排布，见 go-port.md §3.4
- 实际还多出：`internal/scanner/system_test.go`（13 条，`ps`/`vm_stat`/`sysctl` 解析）、`internal/trayicon/trayicon_test.go`（12 条，几何与反锯齿）、`internal/analyze/bridge_test.go`（7 条）、`internal/power/status_test.go`（6 条）

### Phase 9: Polish & bundle (Day 10)
- `go vet ./...` → fix warnings
- `mygo generate` → generate TS client
- `mygo build --skip-dmg` → 产物在 `build/<goos>-<goarch>/MoleStudio.app`（不是 `-o dist/`）
- Optional notarization (if account available)
- Update README: "Install Go 1.27+, `mygo dev` to run, `mygo build` to package"

### 计划 vs 实际

| 项目 | 计划里写的 | 实际做法 | 为什么改 |
| --- | --- | --- | --- |
| Go→前端推送 | `mygo.NewEvent[T]` + `EmitPage` | `Page.Eval("window.onNativeRealTelemetry(...)")` | 页面本来就有 `window.onNative*` 全局函数，用 `Eval` 不需要页面加载 MyGo 的 TS runtime，`mygo.ts` 可以只留一个 `Dispatch` |
| 窗口拖动 | 桥里调 `performDrag` | CSS `--app-region: drag` + preload 注入 | MyGo 自带的 bridge.js 监听 `--app-region` 并调原生 `StartDrag()`，比每次 mousedown 过一次 IPC 更省 |
| Bridge 服务 | 14 个独立方法（`SetTheme`/`TrashPaths`…） | 单个 `Dispatch(payload map[string]any)` | 页面用的是 `{action: "xxx"}` 单一通道，服务侧保持同构，改一个 switch 就对上，不用维护方法名映射 |
| 托盘图标 | resize `AppIcon-256.png`（方案 A） | `internal/trayicon` 按 64 格几何在纯 Go 里栅格化 | 两者图形不同：app 图标是满版圆角方形，菜单栏是「星球+环」细线标记；resize app 图标会丢形状，且 MyGo 会把 PNG 缩到 18pt |
| 主题持久化 | `PathUserData/theme` | `PathUserData/preferences.json` 里的 `theme` 字段 | 后面还要存别的东西，单开一个文件更省事 |
| 前端嵌入 | 手写 `mygo.SetFrontend(embed.FS())` | 交给 `mygo build` 生成的 `mygo_frontend_gen.go` | MyGo 自己会按 `frontendDist` 生成并调用，手写反而会覆盖它 |
| 事件/推送落点 | `EmitPage(mainPage)` | 主窗口 + （可见时的）popover 两处都要 | Swift 的 `evaluate` 就是两个 webView 都发，popover 打开着必须同步 |
| Swift 代码 | 未提 | 整个 `Sources/`、`Tests/`、`Package.swift` 删除 | 迁移完成后没有第二份实现要维护；历史留在 git（`adea952`） |

---

## 5. 边界条件与风险

| 风险 | 影响 | 缓解策略 |
|------|------|---------|
| Thermal state not accessible | PowerStatus 缺失 thermal 字段 | `NSProcessInfo.thermalState`（purego/objc），再退 `sysctl`，最后 `nominal` |
| Battery ioreg output format changes | Parsing brittle | Use regex parsing with fallback; monitor Apple doc changes |
| Preload injection order issues | bridge shim breaks | Inject after `window.mygo` per docs; debug via `__MYGO_CONFIG__` presence |
| Icon template tint mismatch | Visual drift | Test `IconIsTemplate=true` vs false; use PNG without alpha if needed |
| Menu bar icon position persistence | User preferences lost | Use stable `Tray.ID="orbit"` across launches |
| Go binary size blowup | >10MB unusual | Profile: pure Go + no stdlib bloat expected; ~5-8MB target |
| DMG signing failure (notarization) | App Store rejection optional | Ad hoc sign with `-` acceptable for personal use; notarize if required |

**实际结果**（迁移完成后回填）：
- Thermal：已按 Swift 原路径修好——`internal/power/thermal_darwin.go` 用 purego 调 `-[NSProcessInfo thermalState]`（先 `Dlopen` Foundation），`sysctl kern.thermal_pressure_level` 只作兜底（本机是 unknown oid）；见 go-port.md §3.2。memory pressure 正常（`kern.memorystatus_vm_pressure_level` = 1 → `normal`）。
- Battery：`ioreg -rn AppleSmartBattery` 扁平键解析够用（有 fixture 测试）；`AdapterDetails` 里的 `Watts` 是嵌套字典， disconnected 写成一行，所以额外切一层把它取出来（本机 65 W）。
- Preload 注入顺序：不是问题。`window.webkit` 不可重定义（会抛 `configurable attribute of unconfigurable property`），
  但可以往 `messageHandlers` 上加属性，所以 shim 直接挂在 preload 里即可，页面 3830 行一行没改。
- 托盘图标：`IconIsTemplate=true` 正确，模板图全黑 + alpha 即可，不需要去 alpha。
- 托盘位置：`Tray.ID="orbit"` 稳定，重启后位置由系统记忆。
- 体积：Go 二进制 14.1 MB（未 strip）、`.app` 10.4 MB —— 比计划的 5–8 MB 大，主要是 WebKit/purego 的 objc 绑定与 `image`/`png`。
- 签名：ad-hoc `-` 正常，`mygo build` 自带 codesign。

---

## 6. 结论

**可行性**: ⭐⭐⭐⭐⭐ (5/5)
- Bridge transport identical mechanism (webkit.messageHandlers)
- PreloadScript provides shim hook without modifying frontend
- All backend logic (scanners, analyzers, power status) portable to Go via subprocess + sysctl + exec
- Window/flyout/tray APIs map 1:1 to AppKit semantics
- No cgo needed → clean distribution, tiny binary

**工作量**: ~10 development days (single developer, sequential phases)
- Core bridge + telemetry: 2 days
- Scanner engine: 3 days
- Analyze bridge + power: 1 day
- Windows/menus/flyout: 1 day
- Tests: 1 day
- Polish/bundle: 2 days

**收益**:
- Single language stack (Go)
- Cross-platform potential (Linux/Windows via MyGo)
- Simpler toolchain (no Xcode, no Swiftlint)
- Better integration with Go ecosystem if future needs (e.g., cloud services)

**下一步**：迁移已完成（分支 `feat/mygo-port`）。现在的入口是 `make dev` 看效果、`make check` 过门禁、
`make app-nodmg` 出包；实现细节与差异清单在 [`go-port.md`](go-port.md)。

---

*Document created: 2026-10-27*  
*Based on analysis of MoleStudio-Native-macOS @ adea952 and MyGo v0.6.4 at /tmp/mygo-probe/repo*
