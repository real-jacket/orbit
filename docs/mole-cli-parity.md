# Mole CLI 能力对照

核对日期：2026-11-12。基准：[tw93/Mole V1.70.0](https://github.com/tw93/Mole/tree/V1.70.0)（`7ec1af8`）。

## 做法：移植规则，不调用 `mo`

orbit 不运行 `mo`，也不需要装 Mole。早期版本只调用过 `mo analyze --json`，后来换成进程内扫描
（同一目录 17 秒对 87.6 秒，而且可以取消，见 [disk-analyzer.md](disk-analyzer.md)）。不改回去调用 `mo`，原因是：

- `mo clean`、`mo purge`、`mo installer`、`mo optimize` 没有 JSON 输出，只有终端文字；`--select` / `--categories`
  已在 V1.70 移除，GUI 无法逐项勾选、预览后再执行。
- `mo clean` / `purge` / `installer` 是永久删除；orbit 的每一次移除都进废纸篓，并写还原清单。
- `mo` 会在需要时弹 sudo；orbit 是不提权的用户态应用。

所以 orbit 把 Mole 的**判断逻辑**（路径清单、可重建证据、运行中保护、白名单、年龄阈值）移植成 Go，执行仍走
orbit 自己的预览 → 确认 → 移入废纸篓 → 回执 → 还原。两个项目都是 GPL-3.0。

每条移植过来的规则上方都有一行标记：

```go
// Ported from tw93/Mole V1.70.0 lib/clean/user.sh clean_browsers
```

标记只写文件不写函数名时，表示移植的是该文件里的一组函数（例如 `lib/clean/dev.sh` 的各个 `clean_dev_*`）。

### 跟进上游

```bash
make mole-sync                 # 对比到 Mole 最新发布版本
make mole-sync MOLE_TAG=V1.71.0
```

`scripts/mole-sync.sh` 读取全部标记，只对比这些函数和变量在两个版本之间的差异（函数改了打印 diff，删掉或改名
单独提示），再列出上游改动过、但还没有任何标记引用的规则文件。照着报告改完规则后，把对应标记的版本号改成新版本。
脚本只读 Mole 仓库，不改 orbit 的任何文件。

## 命令总表

| `mo` 命令 | orbit 对应 | 状态 |
| --- | --- | --- |
| `mo clean` | 深度清理、AI 清理 | 用户级规则已移植；需要 sudo 和无法还原的部分不做（见下） |
| `mo uninstall` | 应用卸载 | 已移植：残留文件、配置引用、共享数据保护、停止用户级后台服务、LaunchServices 记录；系统设置里的登录项不做 |
| `mo optimize` | 系统加速 | 19 项全部列出：10 项可执行，9 项说明原因不执行 |
| `mo purge` | 构建清理 · 构建产物 | 已移植，扫描目录与 `mo purge` 共用 `~/.config/mole/purge_paths` |
| `mo installer` | 构建清理 · 安装包 | 已移植 |
| `mo slim` | 应用卸载 · 应用瘦身 | 已移植 |
| `mo analyze` | 空间分析 | orbit 自己的进程内扫描，不移植 |
| `mo status` | 状态监控、菜单栏 | orbit 自己的采样，不移植 |
| `mo history` | 历史与白名单 | orbit 自己的回执和还原清单，不读 Mole 的历史 |
| 白名单 `~/.config/mole/whitelist` | 历史与白名单 | 读写同一个文件 |
| `mo touchid` | — | 不做：要用 sudo 改 `/etc/pam.d/sudo_local` |
| `mo completion` / `mo update` / `mo remove` | — | 命令行专用；orbit 有自己的应用内更新 |

## 深度清理（`mo clean`）

orbit 的分级：推荐清理（默认勾选，可重建且没在用）、需确认（写明原因，默认不勾）、受保护（写明原因，不可勾选；
因为应用正在运行而受保护时，会提示「退出 X 后重新扫描即可清理」）。

| Mole 部分 | 函数 | orbit |
| --- | --- | --- |
| User essentials：用户缓存与日志 | `clean_user_essentials` | `~/Library/Caches`、`~/Library/Logs` 按子项测量和分级 |
| Browsers | `clean_browsers` | 「Chromium 系浏览器与 Firefox 缓存」：按配置文件找缓存，浏览器运行时受保护 |
| App caches / Apps & utilities / Cloud & Office | `clean_app_caches`、`app_caches.sh` 各 `clean_*`、`clean_office_apps` | 「应用内置缓存与日志」，含微信容器缓存、Xcode 工具缓存 |
| Developer tools | `dev.sh` 各 `clean_dev_*`、`clean_dev_jvm` | 「开发工具缓存与临时文件」「开发包管理器下载缓存」「Gradle 与 CocoaPods 缓存」 |
| Xcode | `clean_xcode_device_support` | 「Xcode 设备调试符号与测试设备数据」：保留最近使用的 2 个系统版本，其余推荐清理 |
| 模拟器设备数据 | `_simulator_device_leaf_targets`、`_simulator_device_is_shut_down` | 同一分组：只列出 `device.plist` 为关机状态的设备的诊断日志、缓存（推荐）与系统资产（需确认）；有 `launchd_sim` 进程即视为模拟器在运行 |
| 路径保护表 | `should_protect_path` | 已移植其中的壁纸与屏保资源、账户/邮件/日历/通讯录、Playwright/Cypress/camoufox、Google 标识队列等条目；另加 Spotlight 索引（orbit 自有） |
| macOS 用户状态 | `clean_system_state` | 「macOS 用户级缓存与状态」 |
| 设备固件 | `clean_cached_device_firmware` | 「设备固件安装包缓存」：`.ipsw` 文件，正被打开的保留 |
| Application Support 里的缓存与日志 | `clean_application_support_logs` | 「应用的浏览器内核缓存与崩溃报告」：沿用 Mole 的名称保护和白名单；应用运行时受保护，归属无法确认的交给「待核对数据」 |
| 空沙盒容器 | `clean_orphaned_container_stubs` | 「已卸载应用留下的空沙盒容器」：只处理 Mole 列出的那几类 |
| 邮件附件副本 | `clean_mail_downloads` | 「邮件附件的本地副本」：30 天以上、未改动过的副本，需确认（邮件若已从服务器删除，这可能是唯一一份） |
| 最近使用的项目 | `clean_recent_items` | 「最近使用的项目记录」：需确认，移入废纸篓可还原 |
| App leftovers | `clean_orphaned_app_data`、`scan_installed_apps`、`bundle_has_installed_app` | 应用卸载 · 待核对数据；Claude 虚拟机包按 Mole 的 `is_claude_vm_bundle_orphaned` 单独判断 |

`~/Library/Caches` 下 Mole 逐个列出的缓存（云盘、虚拟机、下载工具等）由通用的「用户缓存与诊断日志」分组覆盖，
并按 Mole 的清单判为可重建（`cache_policy.go` 的 `regenerablePrefixes`）。

orbit 自有、不来自 Mole 的规则（标注为 shortcut）：GVM 模块下载缓存、Volta 临时文件与安装包、Gradle Wrapper 发行版、
`.webpack_cache`。体积测量不跨卷：目录里挂载的其他卷（模拟器运行时、设备文件系统视图）不计入，也不能随目录移入废纸篓。

**有意不做：**

| Mole 部分 | 原因 |
| --- | --- |
| 清空废纸篓（`clean_trash`） | orbit 的还原依赖废纸篓；整桶清空会让所有回执失效 |
| System（`clean_deep_system`）、`/Library` 下的系统缓存、`clean_apple_silicon_caches` | 需要 sudo |
| 本地 APFS 快照（`clean_local_snapshots`）、Time Machine 失败备份 | 需要 sudo，且删除后无法还原 |
| 孤立的系统服务（`clean_orphaned_system_services`：LaunchDaemons、PrivilegedHelperTools） | 需要 sudo |
| Docker 的 `prune`、`brew cleanup` 等工具自带清理命令 | 由工具直接永久删除，无法先放进废纸篓 |
| 递归删除 `.DS_Store`（`clean_finder_metadata`） | 空间收益很小；系统加速里改为「禁止在网络与 U 盘上写 .DS_Store」 |
| 浏览器代码签名副本（`clean_browser_code_sign_clones`） | 位于 `/var/folders/*/X`，是应用包的 APFS 克隆，删除后几乎不释放空间；Mole 也不报告其体积 |
| 删除模拟器设备与运行时（`simctl delete`） | 由 Xcode 工具直接永久删除，无法先放进废纸篓 |
| 外接卷（`mo clean --external`） | 外接卷的废纸篓在卷上，还原路径不同，暂不支持 |

## AI 清理

Mole CLI 没有单独的 AI 清理命令；相关规则在 `mo clean` 的 `clean_dev_ai_agents` 和各应用缓存里。orbit 的 AI 清理
是自己的目录清单，与 Mole 共用同一套保护语义：

| 项目 | Mole | orbit |
| --- | --- | --- |
| Claude Code / Cursor Agent / Copilot CLI 旧版本 | 保留当前版本、一个备用版本，以及任何仍被进程打开的版本 | 相同；移动前再次核对启动器、进程和 `lsof` |
| Claude Desktop 内置的 Claude Code 与虚拟机 | 从 `.sdk-version` 判断当前版本，应用运行时跳过 | 相同 |
| ChatGPT / Claude Desktop / LM Studio 缓存 | 清理 | 清理，工具运行时受保护 |
| 会话、凭据、配置、模型、扩展、Agent 工作树 | 排除 | 只检查，不可勾选 |
| Codex 运行缓存（staging 目录） | 清理 | 只检查，尚未移植 |
| T3 Code 预览缓存（V1.70 新增） | 清理 | 尚未移植 |

## 应用卸载（`mo uninstall`）

| 能力 | Mole 函数 | orbit |
| --- | --- | --- |
| 应用与残留文件 | `scan_applications`、`remove_file_list` | 标准 / 完整 / 仅缓存三种方式，逐个路径勾选，移入废纸篓 |
| 卸载后列出仍引用该应用的配置行 | `_uninstall_scan_configuration_references` | 卸载回执里列出文件和行号，只读，不改文件 |
| 同一应用还装着另一份时保留共享数据 | `uninstall_bundles_share_remnants` | 共享的子路径标为受保护 |
| 停止应用的用户级后台服务 | `unload_launch_plist` | 勾选的 `~/Library/LaunchAgents` 项移入废纸篓前先 `launchctl bootout`；移动失败会重新加载，从废纸篓还原时也会重新加载 |
| 应用内置的登录项 helper | `discover_login_item_helper_bundle_ids`、`bootout_login_item_helpers` | 应用进入废纸篓后停止；另一份相同标识的应用仍安装时不停止（Mole 的同款保护） |
| LaunchServices 记录 | `unregister_app_bundle`、`refresh_launch_services_after_uninstall` | 对废纸篓里的应用执行 `lsregister -u`，最后执行一次 `lsregister -gc` |
| Homebrew 安装的应用 | `brew.sh` 的 cask 识别 | 只读文件判断，回执里给出 `brew uninstall --cask` 命令供复制，orbit 不运行 brew |
| 系统设置里的登录项 | `remove_login_item` | 不做：要通过 osascript 申请自动化授权，删掉后无法还原；回执提示到系统设置检查 |
| 需要管理员权限的系统路径（LaunchDaemons、PrivilegedHelperTools） | 用 sudo 删除 | 只列出，不删除 |

Mole 的应用归属检查同时参考安装清单、Bundle ID 和共享残留；目录名没匹配到应用不能证明应用已经卸载。
orbit 会检查厂商目录下子目录的精确归属，例如已安装 `com.citrolabs.ego.lite` 时，
`Application Support/Citro Labs/ego lite` 属于 ego lite，即使应用包被改名也保持归属。不会因为一个已安装子目录
就把同一厂商的所有兄弟目录归给它。仍不能确认的目录显示「待核对数据」，默认不勾选；沙盒容器、Group Containers
和配置目录属于个人数据，不整体当作缓存。

「仅缓存」沿用 Mole `clean_application_support_logs`、`clean_browsers` 的可重建目录判断，在应用的
Application Support、沙盒数据、`User Data` profiles 和 Electron `Partitions` 内寻找缓存与日志叶子目录，
不选择它们的配置父目录。应用重名、显示名称与目录名称不同（如 VS Code 的 `Code`）、应用改名及后台 helper
都通过安装元数据核对；扫描和移入废纸篓前分别检查运行状态及打开的文件。

## 系统加速（`mo optimize`）

Mole 的 19 项全部列在 orbit 里，先只读检查，再由用户勾选执行。每项执行后有回执；能撤销的（移入废纸篓的文件、
改过的偏好设置）可以一键撤销，偏好设置会恢复成执行前的值。

| Mole 任务 | orbit |
| --- | --- |
| `opt_cache_refresh` 刷新缩略图与图标缓存 | 执行 |
| `opt_saved_state_cleanup` 旧的应用窗口状态 | 执行，30 天以上的移入废纸篓 |
| `opt_fix_broken_configs` 损坏的偏好设置 | 执行，无法读取的第三方 plist 移入废纸篓 |
| `opt_sqlite_vacuum` 邮件、Safari、信息的数据库 | 执行：先完整性检查再压缩，不删内容；对应应用运行时暂不处理 |
| `opt_prevent_network_dsstore` | 执行，可撤销 |
| `opt_legacy_overrides_audit` 旧调优工具的覆盖设置 | 执行，可撤销 |
| `opt_prune_spotlight_orphan_rules` | 执行，可撤销 |
| `opt_shared_file_list_repair` 访达收藏与最近项目 | 执行，损坏文件移入废纸篓 |
| `opt_login_items_audit` 失效的登录项 | 只报告（Mole 也只报告） |
| `opt_launch_agents_cleanup` 失效的启动代理 | 只报告（Mole 也只报告） |
| `opt_network_optimization` DNS 缓存、`opt_network_stack_optimize` 路由表与 ARP、`opt_disk_permissions_repair`、`opt_spotlight_index_optimize`、`opt_periodic_maintenance` | 不执行：需要管理员权限 |
| `opt_disk_verify` | 不执行：Mole 默认也关闭，整卷校验可能让系统长时间卡住 |
| `opt_quarantine_cleanup`、`opt_notification_cleanup`、`opt_coreduet_cleanup` | 不执行：直接删除数据库记录，无法放进废纸篓还原 |

## 构建清理（`mo purge`、`mo installer`）

- **构建产物：** Mole 的产物目录名（`MOLE_PURGE_TARGETS`）、项目标识文件、跳过的目录、`CACHEDIR.TAG`、
  .NET `bin` 判断、Git 跟踪文件检查和 `node_modules` 保护都已移植。默认扫描位置沿用 Mole 的列表，但不含 Agent 工作树目录（AI 清理单独列出）和 `~/Library/CloudStorage`（还不能判断哪些文件只在云端）；在页面上添加扫描目录后
  写入 `~/.config/mole/purge_paths`，`mo purge` 也会读到同一份列表。Mole 首次运行时会自动发现家目录里的项目目录并写入配置；
  orbit 只把发现的目录显示为建议，由用户点击添加。
- **分级：** 7 天内有改动、构建工具正在运行、目录里有 Git 跟踪的文件或密钥文件时受保护；安装之后被手动改过、
  缺少锁文件或安装记录、7 到 30 天没动过的需确认；其余推荐清理。包管理器和打包工具自己会改写的状态文件
  （`.cache`、`.vite`、`.package-lock.json` 等）不算手动改动。
- **安装包：** Mole 的扫描位置（下载、桌面、文稿、共享、Homebrew、iCloud 下载、邮件附件、Telegram）、
  安装包类型和「压缩包里含有安装程序」的判断都已移植。移入废纸篓前再核对一次大小和修改时间，文件变了就跳过。
- **扫描范围与读取告警：** 扫描、页面目录列表和告警共用一次 `purge_paths` 快照；无效或不可读取的自定义配置
  不回退到默认目录。缺失目录显示为跳过，真正的拒绝、读取超时、测量上限、配置错误和系统保护分别报告。
  每个模块只展示自身范围内的告警；完整证据仍在运行时诊断中。完全磁盘访问权限不能绕过目录 ACL 等访问控制。
  配置允许的外部项目目录仍接受构建工具运行保护，移动前重新核验。

## 应用瘦身（`mo slim`）

已移植 Mole 的通用二进制解析、可瘦身判断和签名检查（`slim_plan_app` 等）。orbit 先瘦身一份副本，检查代码签名、
签名团队和 Gatekeeper 都通过后才替换；原应用移入废纸篓，从废纸篓放回即可还原。App Store 应用、苹果自带应用、
设为用 Rosetta 打开的应用、正在运行的应用和需要厂商卸载程序的应用都不处理，并写明原因。
