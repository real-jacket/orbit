# Mole CLI 能力对照

核对日期：2026-10-31。

## 基准

- 本机 CLI：Mole `1.56.1`。只读取 `--version` 与各命令的 `--help`。
- 上游源码：[tw93/Mole@85162c24](https://github.com/tw93/Mole/tree/85162c24071d98560749e5c0a192876a252d7733)，
  对应 2026-10-31 发布的 V1.67.0。
- 最新 AI 边界：[SYNC.md](https://github.com/tw93/Mole/blob/85162c24071d98560749e5c0a192876a252d7733/SYNC.md)
  仍明确：CLI 没有独立 AI 清理命令；会话、凭据、模型和 Agent 工作树不属于 `mo clean`。
- V1.67 的旧版本保护还要求检查进程是否仍打开该版本，而不只检查启动器或进程名：
  [clean_dev_ai_agents](https://github.com/tw93/Mole/blob/85162c24071d98560749e5c0a192876a252d7733/lib/clean/dev.sh)。

orbit 不追求逐项复制 Mole 的数百个应用规则。目标是复用同一安全语义：真实路径、可重建证据、
活跃状态检查、精确预览、可还原回执和明确拒绝原因。

## 深度清理

| 能力 | Mole CLI | orbit 当前 | 结论 |
| --- | --- | --- | --- |
| 用户缓存与日志 | `mo clean` | `~/Library/Caches`、`~/Library/Logs` 按直接子路径审查 | 已实现；未知项默认不勾选 |
| Safari / Chrome / Firefox 缓存 | 支持，并在浏览器运行时跳过 | 三个已知根路径，执行时再次检查进程 | 已实现 |
| Xcode DerivedData / Simulator 缓存 | 支持并有工具进程保护 | 支持，Xcode/CoreSimulator 运行时保留 | 已实现 |
| npm / Yarn / Bun / pip / uv / Go / Cargo / Homebrew 下载缓存 | 支持其中多类，部分使用工具自身命令 | 已知下载缓存路径；工具运行时保留，默认需确认 | 部分实现；不执行包管理器自己的 prune 命令 |
| 具体应用缓存规则 | 大量精选路径与工具专属逻辑 | 通用缓存子路径审查 | 安全边界相近，覆盖面少于 Mole |
| 废纸篓清空 | `mo clean` 默认处理 | 不整桶清空 | 有意不实现；避免与可还原回执混淆 |
| 系统缓存 / LaunchServices / 本地快照 / sudo 清理 | 支持 | 不实现 | 有意不实现；当前是 ad-hoc 签名用户态应用 |
| 外接卷元数据清理 | `mo clean --external PATH` | 不支持 | 未实现 |
| 白名单 | `~/.config/mole/whitelist` | 读写同一文件；读取失败即停止 | 已实现 |
| 预览与审计 | `--dry-run`、`mo history` | 精确路径确认、废纸篓回执、本地历史与还原 | 行为等价，输出形式不同 |

## AI 清理

| 能力 | Mole CLI V1.67 | orbit 当前 | 结论 |
| --- | --- | --- | --- |
| Claude Code 旧版本 | 保留当前、一个备用版本、启动器变化及任何打开中的版本 | 相同；最终移动前重新核验启动器、进程和 `lsof` | 已实现 |
| Cursor Agent 旧版本 | 同上 | 同上 | 已实现 |
| GitHub Copilot CLI 旧版本 | 同上 | 同上 | 已实现 |
| Claude Desktop bundled Claude Code / VM | 从 `.sdk-version` 解析当前版本，应用运行时跳过 | 支持两个版本根和同一 marker/进程规则 | 已实现 |
| Cursor 扩展 | Mole CLI 不清理 | 仅检查，不可勾选 | 有意只读 |
| Hugging Face / Ollama 模型 | Mole CLI 不清理 | 仅检查，不可勾选 | 有意只读 |
| Codex / Cursor 工作树 | Mole CLI 排除 | 读取 Git 状态，只检查，不可勾选 | 有意只读 |
| 会话、凭据、配置 | Mole CLI 排除 | 内置保护 | 已实现 |
| ChatGPT / Claude Desktop / LM Studio 已知缓存 | `mo clean` 支持其中多类 | 已知缓存路径，工具运行时保留，默认安全但执行前复核 | 已实现 |
| Codex 运行缓存 | Mole CLI 有专项处理 | 仅检查 | 有意保守；尚未复制其运行态判断 |

## 不在本轮范围

- 不调用 `mo clean` / `mo purge` / `mo uninstall` 代替 orbit 自己的精确路径预览和废纸篓回执。
- 不自动执行 `sudo`、系统快照删除、外接卷清理或应用专属维护命令。
- 不把扩展、模型、工作树、会话或凭据加入自动清理候选。
