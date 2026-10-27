# 权限与签名：为什么「清理」和「卸载」会被 macOS 拒绝

这个应用做的是**改别人的文件**，而 macOS 对这件事有两道独立的闸门。闸门没开时，动作会被
内核直接拒绝（`EPERM`），文件一个字节都不会动。知道是哪道闸门没开，才知道去哪个开关授权——
这也是这一版把回执里的错误换成具体面板名称的原因。

## 两道闸门

| 闸门 | 管什么 | 在哪儿授权 |
|------|--------|-----------|
| **App 管理**（App Management） | 移动、删除、修改**其他应用**的 bundle（`/Applications/**.app`） | 系统设置 › 隐私与安全性 › App 管理 |
| **文件与文件夹**（Files and Folders） | 访问/删除用户自己的 `~/Desktop`、`~/Documents`、`~/Downloads` 里的文件 | 系统设置 › 隐私与安全性 › 文件与文件夹 |
| **完全磁盘访问权限**（Full Disk Access） | 读取/写入受保护目录：`~/Library/**`、`/Library/**`、`/private/**`、其他应用的容器 | 系统设置 › 隐私与安全性 › 完全磁盘访问权限 |

两条经验：

- **三种失败互不相干**：卸载应用要 App 管理，删「桌面/文稿/下载」里的大文件要文件与文件夹，
  清 `~/Library` 下的缓存要完全磁盘访问权限。只开一个，另外两个仍然失败；这也是「大文件删不掉」
  与「应用卸载不掉」同时出现的原因。应用会按路径判断失败原因（`.app` → App 管理；`Library/`、`/private/` → 完全磁盘
  访问权限），回执里带上 `pane` 字段，页面弹提示条并给「去系统设置」按钮。
- **Finder 能删不等于本应用能删**。Finder 有系统级豁免；终端里的 `mo` 之所以能删，是因为授权
  给的是终端（或它继承的父进程），而不是 `mo` 自己。

## 签名：本地 ad-hoc 签名会让授权「记不住」

`mygo build` 产出的 `.app` 默认是 **ad-hoc 签名**（`codesign -dv` 显示 `Signature=adhoc`、
`TeamIdentifier=not set`）。ad-hoc 签名没有稳定的身份（designated requirement），后果是：

1. 系统设置里勾选授权后，**重新打包一次，cdhash 变了，系统把它当成另一个应用**，授权失效；
2. 部分系统版本对 ad-hoc 应用**不弹 TCC 授权对话框**，直接静默拒绝，表现就是「点了没反应」或
   「失败了但没有提示」。

因此：本地自用最稳的是**每次重新打包后重新授权一次**；要长期免维护，需要 Developer ID 证书签名
（`mygo build` 支持签名配置，见 `mygo doctor`），签名稳定后 TCC 授权才会持久。

## 应用现在的行为

- 失败回执带 `pane`，页面底部弹出「系统拒绝了这次操作：缺少「App 管理」」提示条，一键跳到对应设置面板；
- 已经成功移入废纸篓的条目不受影响，失败项保留在列表里（不会假装成功）；
- 每一次移动都写 `~/Library/Application Support/orbit/trash-manifest.json`，所以「还原」是真的按
  原路径搬回去，而不是重新造一个同名文件。

## 自检命令

```bash
# 当前二进制是不是 ad-hoc 签名（TeamIdentifier=not set 即为 ad-hoc）
codesign -dv build/darwin-arm64/MoleStudio.app 2>&1 | grep -E "Signature|TeamIdentifier"

# 应用里的体检页也会读真实状态：完全磁盘访问权限那一行显示 false 就说明还没授权
```
