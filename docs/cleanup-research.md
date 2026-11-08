# 空间分析与清理边界

研究日期：2026-10-30。实现与验收状态在本文末尾更新。

## 上游能力核对

- 本机 `mo --version` 为 1.56.1；仅执行版本和帮助查询，没有运行清理、更新或安装脚本。
- 官方最新 macOS release 为 [V1.67.0](https://github.com/tw93/Mole/releases/tag/V1.67.0)，
  发布时间为 2026-10-31 02:17 UTC。研究源码固定在
  [85162c24](https://github.com/tw93/Mole/tree/85162c24071d98560749e5c0a192876a252d7733)，不是未标版本的第三方方案。
- [官方 SYNC](https://github.com/tw93/Mole/blob/98b3e873396164fbb740c472de3187c8938c3b39/SYNC.md)
  明确：CLI 不提供独立 AI 数据清理界面；`mo clean` 包含可重建缓存和旧程序版本，
  会话、历史、凭据、模型和 Agent 工作树不属于该清理范围。桌面应用的 AI 功能不能当作 CLI flag。
- [版本清理实现](https://github.com/tw93/Mole/blob/85162c24071d98560749e5c0a192876a252d7733/lib/clean/dev.sh)
  同时保留启动器指向的版本和回滚版本，并在删除前重新核验；V1.67 还会用 `lsof` 保留任何进程仍打开的版本。
- [安全说明](https://github.com/tw93/Mole/blob/98b3e873396164fbb740c472de3187c8938c3b39/SECURITY_AUDIT.md)
  要求预览、保护路径、活跃状态检查、未知状态保留和操作回执。
  `clean/purge/installer` 的直接删除语义不适合替代 orbit 已有的精确路径预览与废纸篓还原链路。
- [Hugging Face 缓存文档](https://huggingface.co/docs/huggingface_hub/main/guides/manage-cache)
  说明模型缓存在多个库和修订间共享，不能因为路径包含 cache 就推断整目录可删。
- [Git worktree 文档](https://git-scm.com/docs/git-worktree)区分主工作区、关联工作区、锁定和移除；
  干净的工作区不代表分支已合并、提交已推送或 Agent 已结束使用。

## 成熟空间分析器

- [DaisyDisk](https://web.daisydiskapp.com/guide/4/en/UnderstandingSunburst) 用磁盘图、精确侧栏和可展开的
  “smaller objects”承担同一条任务链；受限项和未测量的隐藏空间都有明确状态，删除前进入 Collector，
  并允许 5 秒撤销。
- [GrandPerspective](https://grandperspective.sourceforge.net/HelpDocumentation/Views.html) 用树图和便捷
  导航定位文件；[Full Disk Access](https://grandperspective.sourceforge.net/HelpDocumentation/FullDiskAccess.html)
  说明未授权区域必须被视为测量边界，而不是不存在的 0 字节。
- [SpaceSniffer](https://github.com/redtrillix/SpaceSniffer) 的成熟能力包括可过滤的树图、目录层级缩放下钻和
  按名称、大小、时间缩小结果集。
- [Apple Storage](https://support.apple.com/en-us/102624) 把总容量、分类、建议动作和每种资料自己的管理入口分开，
  避免把“系统建议”误当成任意路径都可删除。

据此，orbit 不增加第二套图形库，而是在现有树图+列表上补：
当前层搜索、最多 5 个折叠小项的可检查样本、文件数/修改时间、范围外空间提示，以及窄窗地图优先。
折叠样本有明确上限，不会为了“全部展开”在内存里保留几十万个小文件。

## 最小完整需求

| 模块 | 应修正的事实与行为 | 保留边界 |
| --- | --- | --- |
| 空间分析 | 显式重扫读取磁盘；下钻统计、大文件和受限清单属于当前目录；迟到回执不能替换新请求 | 家目录不是整机占用；allocated 不是删除后保证释放的空间 |
| 深度清理 | 缓存/日志按子路径审查；测量上限、超时、文件数和修改时间真实披露；不虚增小文件体积 | 不把目录存在当成可删除证明；受限和活跃路径默认保留 |
| AI 清理 | 旧程序版本逐项判断；当前版本、回滚版本、活跃程序与未知状态保留 | 扩展、模型、工作树和会话资料只检查，不自动移除 |
| 共享执行 | 前端选择不包含保护项；执行时重新读白名单、版本和进程状态 | 白名单读取失败停止；不执行真实用户文件清理以验证功能 |

## UI 审计

在本轮修改前，用实际页面的隔离 WebKit 验收重新捕获界面，数据明确标为 fixture。
截图保存在 `.review/cleanup-logic-2026-10-30/before/`，没有使用旧截图充当本轮证据。

1. 深度清理：布局可用，但整组目录的文件数和时间缺少真实来源；默认安全说明过于宽泛。
2. AI 清理：布局可用，但工作树与普通缓存共用可勾选语义，且没有当前版本或保留原因。
3. 空间分析：布局可用，但家目录标为全机概览，下钻事实条缺失，大文件与当前目录边界不清楚。

继续使用已有暖炭/陶土配色、字号、图标和主从布局，不新增设计系统、运行时依赖或自动删除功能。

## 验收

- `make check`：Go vet、全部 Go 测试与前端回归通过。
- `MOLE_TRASH_E2E=1 go test -race ./...`：236 个 Go 测试/子测试通过，无竞态报告。
- `make test-ui-webkit`：81 个隔离场景、双主题、1380 x 860 与 880 x 620 全部通过；
  报告中的页面 SHA-256 与当前 `Resources/src-ui/index.html` 一致，失败数为 0。
- 新增回归覆盖显式重扫、迟到回执、目录级大文件、聚合面积、受保护子路径、旧版本启动器切换、
  活跃缓存、工作树只读检查、白名单读取失败和小文件真实体积。
- 研究阶段只执行 `mo --version` / `mo --help` 与各子命令帮助查询，没有运行 `mo clean`、
  `mo analyze` 或任何清理命令。没有为了验证功能而移动、卸载或还原用户真实文件。
- MyGo `v0.6.4` 构建的 `.app` 通过 `codesign --verify --deep --strict` 和 `plutil -lint`；
  bundle 内 `src-ui/index.html` 与当前源码 SHA-256 完全一致。
