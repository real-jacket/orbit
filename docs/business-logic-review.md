# 业务逻辑审查与修复记录

迁移到 Go + MyGo 之后（见 [`go-port.md`](go-port.md)）做的一次**业务逻辑**级审查：不看框架用法（MyGo 的用法已经在 `mygo-migration.md` 里论证过），只看「这台机器上的真实行为」——数据是不是真的、删的东西是不是用户勾的东西、失败了会不会假装成功、界面上的数字有没有出处。

结论先说：**架构和窗口行为没问题，业务流程里有一批"假成功 / 假数据 / 静默失败"**。P0 六条会让用户真的丢东西或真的关不掉应用，已经全部修掉；P1 是数据真实性问题，修了大半；剩下的是需要新采样源才能做的（网络速率、磁盘 I/O、SMC 温度、风扇控制），这次**明确不做**，界面改成显示「本机不提供」。

## 1. 审查方法

四路并行、互不重叠的只读审查，各自落到一份带 `file:line` 和复现证据的报告，再由 Lead 汇总成计划、逐条实施并回验：

| 审查面 | 关注点 |
| --- | --- |
| 扫描层 `internal/scanner` | 体积统计、去重、白名单、删除范围、预勾选、orphan 匹配 |
| 运行时 `internal/{analyze,power}` + 桥 `bridge.go`/`push.go`/`app.go` | 子进程环境、超时、单飞、假数据、`sync.Once` 冻结、废纸篓语义 |
| 前端破坏性流程 | 清理/还原/卸载/白名单的请求→回执链路、0 选中、`homePath` 来源 |
| 前端展示区 | 卡片字段 ↔ payload 契约、编造的数字、硬编码项、单位与截断 |

复现优先用**真实数据**而不是读代码推断，例如：

- 用真实的 `mo analyze --json` 输出核对体积统计规则；
- 从 `ls -1` 的真实路径集里数出 80 条待删路径、其中 32 条含空格；
- 用 `ioreg -rn AppleSmartBattery` 的原始输出核对电池字段位置；
- 用 `/tmp/ui-harness`（见 `go-port.md`）驱动真实窗口，在页面侧对每个 action 打点，确认后端根本没收到某个 action。

## 2. 关键问题与根因

### 2.1 P0：会让用户丢东西或丢信任

**① 含空格的路径全被跳过，界面却报成功。**
`internal/scanner/fs.go` 里 `movePathsToTrash` 为了从「`<路径> <大小>`」记录里切出路径，写了 `strings.Index(record, " ")` 截断。真实缓存路径大量含空格（`~/Library/Application Support/...`、`/Applications/Visual Studio Code.app`），于是一半请求在 Go 侧被静默丢掉，前端仍按「全部成功」写历史、弹「已成功清理」。同一个截断还复制到了 `bridge.go` 的 `revealInFinder`。
**修法**：删掉截断逻辑本身，而不是加转义——路径是结构化字段（`paths` 数组），从来不需要从字符串里解析。

**② 删除没有回执，成功/失败由前端猜。**
`trashRealPaths` 是 fire-and-forget：Go 移完就结束，不告诉页面移了几条、哪几条失败。页面因此在**发请求后立刻**写「可还原快照」、写本地黑名单、弹成功 toast。
**修法**：`scanner.TrashPaths(rawPaths)` 返回 `TrashOutcome{Requested, Moved, Skipped, Failed}`，Go 用它回推 `onNativeTrashResult`；页面只有收到回执才改状态，`skipped`/`failed` 逐条展示原因，失败项不再进黑名单永久隐藏（黑名单只保留会话级）。

**③「从废纸篓原路还原」是空的。**
Swift 版就没做过 manifest：丢进废纸篓时把文件平铺改名成 `原文件名`，还原时前端只是把快照从 `localStorage` 里删掉，然后提示「已原路还原」。Go 迁移把这个空壳照搬了。
**修法**：新增 `internal/scanner/trash.go`：删除时写 manifest（`原绝对路径 ↔ 废纸篓内实际路径`，版本化、上限 1000 条、原子替换），还原按 manifest 反查并用 `NSFileManager` 的 `moveItemAtPath` 搬回；目标已存在时按 Finder 习惯改名 `name 2.ext`。没有 manifest 的旧条目退化为「按文件名在 `~/.Trash` 里找」。

**④ 空间分析的删除永远失败。**
`index.html` 的 `absolutePath()` 依赖 `STATE.homePath`，而这个字段**从未被赋值**——所有相对路径展开成 `undefined/...`，`trashRealPaths` 每条都被 Go 判为不存在。
**修法**：`FullScanPayload` 增加 `homePath`（Go 侧 `os.UserHomeDir()`），页面收到扫描结果后写入 `STATE`；`absolutePath()` 不再猜。

**⑤「仅清理缓存」模式会把应用本体一起删。**
卸载流程构造删除集时无条件把 `a.appPath` 塞进去，`uninstallMode === 'cache'` 分支只影响文案。
**修法**：删除集按模式构造，`cache` 模式只取缓存子路径，`freedGb` 也按实际删除集累加。

**⑥ ⌘Q 关不掉应用。**
`app.go` 的 `OnBeforeQuit` 无条件 `e.PreventDefault()`（本意是拦截未完成的清理），结果没有任何路径能退出。
**修法**：加 `quitting` 标记——第一次拦截（有未完成清理时确认），确认或再次请求即放行；`e.PreventDefault()` 只在真的要拦时调用。

### 2.2 P1：数字没有出处

**白名单只是展示品。** 手写的 `~/.config/mole/whitelist` 会被读进 payload 显示，但扫描层根本不看它，用户保护过的路径照样出现在清理列表里。**修法**：把 mole 的 `load_mole_whitelist`/`is_path_whitelisted` 语义**逐条移植**（含 `*` 跨 `/` 匹配、`~` 展开、`a & b` 复合行、`FINDER_METADATA` 哨兵、`/`、`/System` 等危险根拒绝），在 `catalog`/`apps`/`orphans`/`installers` 出结果时过滤，并在 `TrashPaths` 里做**二次拒绝**（前端可能传任意路径）。白名单是"永久不出现、不可删"的唯一机制。

**体积被放大到 29 倍。** `walkDir` 把**目录项本身的大小**也算进文件总量，并对命中上限的目录取上限值。实测 GarageBand 显示 1.126 GB，真实占用 0.039 GB。**修法**：只累加常规文件的大小（与 `du` 语义一致），目录只递归不计数。

**跨组重复计数。** `~/Library/Caches` 出现在「用户缓存」组，它的子目录 `~/Library/Caches/{pip,Homebrew}` 又出现在「包管理器缓存」组；两组同时勾选时同一个字节算两次，且先删父目录会让后面的子路径全部「不存在」。**修法**：`CatalogSpec` 支持 `exclude`（默认排除已归属其它组的已知子根），选择器在 `claimed` 集合上做去重。

**档位与预勾选错配。** review 档（`review` 级别的清理项）默认被勾上，orphan 容器（可能是系统同步数据）默认被勾上并从目录名猜"应用已卸载"，用户自己的 zip 被当成安装包。**修法**：只有 `safe` 档预勾选；orphan 只有签名命中才预勾选；zip 收紧为安装包命名模式且默认不勾。

**展示区的编造值。** 状态页的网络上下行（4.82/0.64 MB/s）、磁盘 I/O（14.2/28.6 MB/s）、CPU 温度（由占用率公式反推）、风扇 RPM（写死 3200）、GPU 曲线；体检一律 7/7 通过、健康分 `96 + recheckCount` 自增、复诊「耗时 142ms」「NVMe SMART 通过」；`keepPersonal` 的 `|| 0.25` GB。**修法**：这四类没有采样源的项一律不显示数值或显示「本机不提供」，体检项由真实 payload 生成（完全磁盘访问、内存压力、磁盘剩余、电池健康、SIP、热压力、进程数），无数据显示「未知」，健康分只对真实项加权。

**电池健康/温度永远是空的。** 真实 ioreg 把 `DesignCapacity=6075`、`NominalChargeCapacity=4942`、`AppleRawMaxCapacity=4792` 放在**嵌套的 `BatteryData` 字典**里，且这台机器**没有 `Temperature` 键**；旧代码只读顶层，于是 `healthPct`/`tempC` 恒为 nil 而界面显示 `--`。**修法**：`batteryNumber` 从 `BatteryData` 取容量算健康度（4942/6075 → 81%），温度键不存在就如实返回 nil（界面显示「本机不提供」），**不猜**。

**压力/热状态被 `sync.Once` 冻在第一帧**，运行中插拔电源、过热都不会更新；**analyze 没有超时和单飞**，连点会堆 5 个 `mo analyze`，卡住的进程永远不回收，stderr 还被丢弃；**`layout()` 在基准为 0 时返回 `nil`**；**`size:0 && scan_status!=complete` 的条目被过滤**，用户看不到"没权限，读不到"；**`MoBinaryPath` 在 `HOME` 未设置时会返回相对路径**，可能执行当前目录同名文件；**托盘 Popover 没有 `Focusable: true`**，点外面关不掉；**`caffeinate` 状态只活在页面里**，窗口和托盘面板会互相覆盖，`-t 7200` 到点退出后开关还亮着。

### 2.3 一个"隐形" P0

`index.html` 一直在调用 `healthScoreNow()` / `healthScoreText()`，但**这两个函数从未定义**。`onNativeRealTelemetry` 每 1.5 秒推一次，每次都在页面里抛 `ReferenceError`；dashboard hero 同样。因为异常发生在事件回调里、不冒泡到 UI，所以**看起来一切正常**，只有打开 DevTools 才看得到。修复前每次真机运行页面都攒 33 条错误（见 `go-port.md` §验证），这属于"没崩但功能一直是坏的"。

## 3. 落地清单

### 3.1 Go 侧

| 文件 | 改动 |
| --- | --- |
| `internal/scanner/whitelist.go`（新） | mole 白名单语义的 Go 移植：`LoadWhitelist`/`ValidateWhitelistEntry`/`(Whitelist).Protects`/`UserspacePatterns`/`ProtectsFinderMetadata`，含 bash 兼容的 `matchMoleGlob`（`*` 跨 `/`）与危险根拒绝 |
| `internal/scanner/whitelist_state.go`（新） | 每 home 缓存策略（RWMutex，可刷新，不用 `sync.Once`）、`AddWhitelistEntry`/`RemoveWhitelistEntry`（处理 `a & b` 复合行）、`ensureSafetyWhitelist` |
| `internal/scanner/trash.go`（新） | manifest（版本 1、上限 1000、原子写）、`TrashPaths` 返回 `TrashOutcome`、`RestoreTrash` 返回 `RestoreOutcome`、`uniqueSibling`、`isTrashRoot`、`humanizeTrashError`（EPERM → 需要完全磁盘访问权限） |
| `internal/scanner/trash_darwin.go` / `trash_other.go` | `moveToTrash(path) (string, error)` 走 `NSFileManager`，非 darwin 明确报「仅支持 macOS」 |
| `internal/scanner/{fs,api,payload}.go` | 删掉空格截断的 `movePathsToTrash`；`FullScanPayload` 增加 `homePath`/`whitelist`/`whitelistWarnings`；遥测电池改为 30s 缓存（ioreg 是每 tick 最贵的一步） |
| `internal/scanner/{catalog,apps,orphans,signatures}.go` | 白名单过滤、safe/review 档预勾选、跨组去重、orphan 签名优先、zip 收紧、`walkDir` 只计文件体积、`appSize` 不再有 50 MB 地板、`cacheSizeGb/configSizeGb` 改成实测子路径求和 |
| `internal/scanner/system.go` | 删掉两处编造：`vm_stat` 失败时的 `totalGB*0.55`、进程采样失败/低负载时的 `2.5%` 地板（改为真实 0，宁可显示 0 也不显示假值） |
| `internal/scanner/doctor.go`（新） | 真实体检项：完全磁盘访问（读 `~/Library/Mail`/`~/Library/Safari` 的 `os.IsPermission`）、热压力、内存压力、磁盘剩余（15%/8% 两档）、电池健康（读不到就不出这一行）、SIP（`csrutil status`）；`passed` 三态（true/false/nil＝未知） |
| `internal/power/status.go` | 去掉内存压力/热压力的 `sync.Once`；`parseIoreg` 只读命中节点自身的属性（遇到 `+-o` 子节点即停）；电池健康度从嵌套 `BatteryData` 计算；电池 30s 缓存；`take` 的 thermal 仍优先 Foundation |
| `internal/analyze/bridge.go` | `Payload.Error`、90s `context.WithTimeout`、stderr 透出、`mo` 缺失/超时/非零退出的可读原因、`absolutize`/`firstLine`、`Path` 统一绝对路径、`layout()` 返回 `[]Tile{}`、保留不可读条目 |
| `bridge.go` | `runDoctorReCheck` → `DoctorChecks` + `pushDoctorReport`；`trashRealPaths`/`executeClean`/`trashPath` → `TrashPaths` + 回执；`restoreTrashSnapshot` → `RestoreTrash` + 回执；`addWhitelist`/`removeWhitelist`；`revealInFinder` 去掉截断；`setCaffeinate` 广播 `onNativeCaffeinate` 并监听 `caffeinate` 进程退出 |
| `push.go` | `pushDoctorReport`；`pushTrashResult`/`pushRestoreResult`/`pushWhitelistChanged`/`pushCaffeinateState`（全部用表达式形式 eval，避免跨源 "Script error."）、analyze 单飞 + 最新优先 |
| `app.go` | `quitting` 标记放行退出、托盘 Popover `Focusable: true`、无可见窗口时暂停 1.5s 遥测 ticker（回到前台立即补一帧） |

### 3.2 页面侧（`Resources/src-ui/index.html`）

- 清理/还原改成**回执驱动**：`onNativeTrashResult` / `onNativeRestoreResult` 到达才更新「已清理」、写历史、弹 toast；失败/跳过逐条标红；`localStorage` 黑名单缩到会话级并提供「显示已隐藏项」。
- 体检消费 `onNativeDoctorReport`：列表由 Go 回执填充，回执未到时按钮进「等待回执」状态而不是弹「本机不提供」；健康分只对真实项加权。
- 路径比较统一走 `normalizeComparePath()`（两边都展开 `~`）：Go 一律回绝对路径，页面在 `homePath` 到手前可能还是 `~/…`，不归一化就会把成功判成失败。
- 白名单真的生效后清理列表可能为空：此时 `#navMetaClean` 显示「白名单已保护」，副标题说明「N 条路径已在你的 mole 白名单里，已从清理列表排除」，避免被误读成扫描失败。
- 白名单接入删除链路：`onNativeWhitelistChanged` 重绘列表并触发重扫；删除前不再由前端判断白名单（服务端二次拒绝，回执里给原因）。
- 补上 `healthScoreNow()` / `healthScoreText()`（磁盘/内存/热压力/内存压力加权，无数据返回「未知」）。
- 假数据下线：状态卡片（网络/磁盘 I/O/温度/风扇/GPU 曲线）、体检 7/7、健康分自增、复诊耗时、`|| 0.25`、`leftover*0.8`。
- 去掉 treemap 的 `treemap2D[0..3]` 硬编码加减，清理/还原后重新 `requestDiskAnalyze` 拉真实尺寸。
- 0 选中守卫、`uninstallMode === 'cache'` 只删缓存、面包屑不按空格截断、磁盘条用真实占用率、`fmtNum` 兜底防 `NaN%`/`undefined`、搜索切模块清空。

## 4. 明确不做（及原因）

| 项 | 原因 |
| --- | --- |
| 网络上下行速率、磁盘 I/O 吞吐、GPU 占用/曲线 | 需要新采样源（`netstat -ib` 差分、`iostat`、IOKit GPU 统计）；属于"新增采集能力"，不在业务逻辑修复范围。界面显示「本机不提供」 |
| SMC 真实 CPU 温度、风扇转速控制 | 需要读 SMC（未公开接口，需 IOKit `AppleSMC` 私有 key），风险高；Swift 版也是写死的。显示「本机不提供」 |
| 系统应用（`/System/Applications`）卸载 | 产品上不应支持；`installedAppIndex` 只加名字用于避免误判「应用已卸载」 |
| 整根 `~/Library/Caches` 改成逐子目录 | 行为变更太大（mole 本身就是整根清理），保留整根，但界面加「运行中的应用可能异常」提示 |
| 让 `mo` 的输出参与 UI 主流程 | `mo analyze --json` 是单目录、可慢到 30s；现有超时 + 单飞 + 错误透出已够，不引入长驻进程 |

## 5. 验证

1. **单测**：`internal/scanner` 的 trash/whitelist 新增 14 条（真删真还原、白名单二次拒绝、占用时改名、manifest 缺失退化、复合行编辑、危险条目拒绝），`internal/power` 增 ioreg 真实输出 fixture 与嵌套 `BatteryData` 断言；`go test ./... -count=1` 全绿。
2. **真机闭环**（e2e）：在应用里对 `~/Library/Caches` 下的子目录执行清理 → 检查 `~/Library/Application Support/orbit/trash-manifest.json` 有记录、界面 toast 的条数与 manifest 一致 → 点还原 → 文件回到原位且内容一致；含空格的路径（`~/Library/Application Support/...`）单独验一遍。
3. **白名单**：`~/.config/mole/whitelist` 写入一条路径 → 重扫后该路径不出现在清理列表 → 直接构造 `trashRealPaths` 请求也会被回执拒绝（`skipped:[{reason:"白名单保护"}]`）。
4. **退出**：⌘Q、菜单「退出 orbit」、托盘「退出」三条路径都能退，且清理进行中会先确认。
5. **体检/状态**：无数据字段显示「未知」/「本机不提供」，DevTools 无 `ReferenceError`，遥测每秒一次不再累积错误。
6. **性能**：`/tmp/ui-harness` 驱动真实窗口，记录首屏、扫描、每次交互的耗时，确认改动没有引入 jank（方法见 `go-port.md`）。

### 5.1 回执链路已经在真机应用里跑通（但删除本身被测试环境的沙箱挡住）

用一个最小页面（`/tmp/bridge-test/`）替换 `MYGO_DEV_URL` 指向的页面，在真实应用进程里发一次 `trashRealPaths`，验证的是"页面 → Go → 页面回执"这条链，而不是 UI：

```json
{"requestId":"trash-1","requested":2,"movedCount":0,"moved":[],"skipped":[],
 "failed":[{"path":"/tmp/bridge-test/victim/test file.txt","error":"权限不足，可能需要在系统设置里授予完全磁盘访问权限"},
           {"path":"/tmp/bridge-test/victim/plain.txt","error":"权限不足，可能需要在系统设置里授予完全磁盘访问权限"}]}
```

两件事同时被证明：**含空格的路径完整到达 Go**（失败项里是 `test file.txt` 全名，不是被截断的半截），以及**失败如实回报**——旧实现在这里会弹「已成功清理」。失败原因是当前开发沙箱禁止写 `~/.Trash`（`mv x ~/.Trash/x` 也是 `Operation not permitted`），不是代码缺陷：`NSFileManager` 的废纸篓调用只能在正常登录会话里完成，所以"真删→真还原"的闭环需要在用户机器上的人工验收里跑（`internal/scanner/trash_test.go` 用 `MOLE_STUDIO_TRASH_DIR` 指向临时目录，已覆盖移动/manifest/还原/冲突改名/白名单拒绝的逻辑）。

### 5.2 一个必须知道的副作用：白名单现在真的会"清空"清理列表

本机 `~/.config/mole/whitelist` 里有 `~/Library/Caches`、`~/Library/Application Support`、`~/Library/Logs`、`~/Library/Developer/Xcode/DerivedData` 这几条（复合行形式 `A & B`）。白名单生效后，"智能深度清理"里这些路径**不再出现**——包括之前显示为 1.3 GB 的那些。这是正确行为（mole CLI 同样会跳过），但界面上"什么都没了"容易被误解成功能坏了，所以空状态需要说明"这些目录已在你自己的 mole 白名单里"。
