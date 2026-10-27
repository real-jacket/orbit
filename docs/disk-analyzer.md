# 空间分析：从 `mo analyze` 子进程改成自研扫描器

> 本文记录「星图仪」这一页的重写：为什么换、当前实现是什么、实测数字、以及还没做的部分。
> 调研底稿见 `.review/analyze-research.md`（DaisyDisk / QDirStat / WinDirStat / ncdu / gdu 等
> 十个实现的可复用点与取舍）。

## 1. 为什么换掉 `mo analyze`

旧实现每次下钻都起一个 `mo analyze --json` 子进程。本机实测（Apple M1 Max，APFS，macOS 26）：

| 场景 | `mo analyze` | 现在（自研扫描器） |
| --- | --- | --- |
| `~` 全树（2,308,149 文件 / 372,730 目录） | **87.6 s**，而超时是 90 s，只剩 2.4 s 余量 | **17.0 s**（130–144k 文件/s） |
| `/Applications`（426,446 文件） | 6.0 s | **2.6 s**（161–164k 文件/s） |
| 下钻一层（已在内存树里） | 再跑一次子进程，5–90 s | **6–57 µs**（查表） |
| 取消 | 杀进程，已扫的部分全丢 | `ctx` 取消，返回部分树 + `canceled` 状态 |

结论：`mo analyze` 既慢又不可中断，而且它把「一层目录一次进程」的结构硬编码进了交互。
自研扫描器一次走完整棵树放进内存，之后下钻是内存查表，进度可流式回报、可取消。

`mo` 仍然保留为**兜底**：扫描本身失败（例如根本打不开根目录）时才回退到 CLI，并在
`payload.error` 里说明原因。正常路径上不再依赖 Homebrew 装的 `mole`。

## 2. 实现

### 2.1 `internal/analyze/scan.go`（扫描器）

- 目录级 worker pool：`4 × GOMAXPROCS`，夹在 `[4, 32]`；LIFO 队列 + live 计数，避免 worker 全部阻塞在 send。
- walk 阶段只收集 `dirRecord`；`Wait` 之后**单线程**做硬链接解析与自底向上聚合，所以输出与 goroutine 时序无关
  （同一个目录连扫多次，序列化结果逐字节相同 —— `TestScanTreeIsDeterministic`）。
- 硬链接：walk 时记下所有 `Nlink > 1` 的路径，post-pass 按 `(dev, ino)` 分组、路径升序取最小者为计数路径，
  其余只累加 `HardLinkedDeduped`。这是并行化后仍能保证体积口径不漂移的关键。
- 双口径：`DiskBytes = st_blocks × 512`（实际占用，APFS 压缩/稀疏文件友好）、`LogBytes = st_size`（逻辑大小）。
- 符号链接不跟随、不计数；目录自身的 inode 块不计，只计内容（与 `du -sk` 差几 KB）。
- 跨卷默认不进入（`Excluded` 记录原因）；`/System/Volumes` 与 `.Spotlight-V100` 等系统目录直接排除。
- 权限失败：每条路径只记一次，按 path 排序进 `Restricted`，`Partial = true`，扫描继续。
- 聚合：每个目录只保留 `≥ 1 MB` 的子项、最多 60 个，其余折成「…其它 N 项」。
- 进度：每 2000 文件或 200 ms 回调一次；取消在取任务前与目录内每 64 项检查。
- 大文件：`> 100 MB` 取 Top 50（按实际占用）。

内存：`~` 全树聚合后 peak RSS ≈ **400 MB**（2.3M 文件）。这是目前最大的代价，见 §5。

### 2.2 `internal/analyze/analyzer.go`（内存树 + 下沉/进度）

- 一次只持有一棵树的根：`Tree(path)` 命中就查表（`Find`），不命中才扫描；换根会取消上一次扫描。
- `PayloadFor(path)` 把节点翻译成页面要的载荷；`Shared.Cancel()` 供 `cancelDiskAnalyze` 使用。
- 进度回调再节流到 200 ms（`onNativeAnalyzeProgress`），页面只更新文字，不重排 treemap。

### 2.3 载荷契约（`onNativeDiskAnalyze`）

```json
{
  "scanId": "s-7", "root": "~", "path": "~",
  "volume": { "totalGb": 994.66, "usedGb": 894.97, "freeGb": 99.69 },
  "totalSizeGb": 710.6,        // 实际占用（主显）
  "totalLogicalGb": 9507.7,    // 逻辑大小（副显；含 VM 镜像/稀疏文件时远大于实际）
  "fileCount": 2308149,
  "scanStatus": "ok | partial | canceled",
  "restricted": [{ "path": "~/Library/Mail", "reason": "权限不足" }],
  "excluded":   [{ "path": "/System/Volumes", "reason": "系统目录" }],
  "aggregated": { "count": 182, "sizeGb": 0.1 },
  "tiles": [{ "name": "Library", "path": "~/Library", "sizeGb": 44.38, "diskGb": 46.2,
              "logicalGb": 47.1, "pct": 56.1, "files": 128394, "isDir": true, "partial": true,
              "x": 0, "y": 0, "w": 56.1, "h": 100 }],
  "largeFiles": [{ "name": "x.bin", "path": "~/x.bin", "sizeGb": 1.23 }],
  "error": ""
}
```

`treemap2D` 与 `tiles` 内容相同（前者是旧字段名，页面两者都认）。所有新字段都可缺失：
页面缺 `volume` 就退回遥测里的实时可用空间并标「容量未知」，缺 `fileCount`/`aggregated`/`restricted`
就不显示对应文案，**不补 0、不猜**。

### 2.4 页面

- 扫描中：hero 显示「正在扫描… 已扫描 N 个文件 · X.Xs」，主按钮变「取消扫描」。
- 结果：小字事实条「文件 N · 占用 X GB（实际）/ Y GB（逻辑）· 合并 Z 个小项」；
  `partial` 时多一个「N 个目录读取受限」入口（前 10 条 path + reason，注明这些目录体积未计入）。
- treemap：主数字用 `diskGb`，宽度 < 3px 的矩形不产出 DOM，体积并成一个不可点的「其它」块；
  `partial` 的块右上角有「受限」角标。
- 首次打开「空间分析」才扫描（启动即扫会让开 app 的第一秒就去啃 2.3M 文件）。
- 下钻/面包屑沿用原有实现，只是数据来自内存树。

## 3. 验证

```bash
go test ./internal/analyze/ -count=1                    # 12 个测试
go test -race ./internal/analyze/ -run Scan -count=1    # 无竞争
node /tmp/test_analyze.js Resources/src-ui/index.html   # 前端 49 项断言（抽真实函数在 vm 里跑）
```

- 确定性：`/Applications` 连扫 3 次 sha256 相同；`/System/Library`（264,169 文件 / 24,832 硬链接）2 次相同。
- 口径对照：`/Applications` 并行前后逐项一致（Disk 48.48 GB / Log 54.75 GB / 25,846 硬链接）。
- 真机载荷：`~` → 60 tiles、710.6 GB（实际）/ 9507.7 GB（逻辑）、2,308,149 文件、
  `partial`（596 条受限，主要是 `~/Library` 下 Mail/Messages/Safari/Photos 等 TCC 目录）、
  50 条大文件；下钻 `~/Library` 57 µs。

## 4. 明确不做

- **不做文件级删除以外的操作**：大文件面板的「移到废纸篓」复用既有回执链路，失败会显示原因。
- **不做 APFS clone 的精确归因**：没有系统调用能查出两个文件共享同一组块，clone 会被各自计一次；
  页面用「实际占用 vs 逻辑占用」两个数把这个差额暴露出来，而不是假装精确。
- **不做本地快照条目**：`tmutil listlocalsnapshots` 的占用计入卷已用、不落在目录树里，
  差值体现在 `volume.usedGb - totalSizeGb` 上。
- **不引第三方库**：treemap 的 squarify 是页面里手写的 vanilla JS，扫描器只用标准库。

## 5. 已知代价与下一步

- 内存 400 MB（`~` 全树聚合后）。下一步可以按目录做 LRU 裁剪，或把 `FileCount` 之外的信息丢掉。
- 首次 `~` 扫描 17 s，仍是一次性成本；缓存落盘（mtime 签名）能把二次打开降到亚秒。
- 顶部 60 个子项 + 1 MB 阈值是经验值，超大目录（如 `~/Library/Developer`）可能需要按层自适应。
