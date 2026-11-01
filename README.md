# zrss · MyGo

一个本地优先的桌面 RSS 阅读器，用 Go 与 [MyGo](https://github.com/egoist/mygo) 的原生 UI 编写。没有 WebView、没有账号、没有云端同步：订阅、文章、已读状态和设置都在本机的一个 SQLite 文件里。

产品与交互规则见 [`docs/功能设计.md`](docs/功能设计.md)，代码分层与数据流见 [`docs/代码架构设计.md`](docs/代码架构设计.md)，版本检查、签名与发版流程见 [`docs/版本更新设计.md`](docs/版本更新设计.md)。

## 功能

- 支持 RSS 0.91 / 2.0、RSS 1.0（RDF）与 Atom 1.0 / 0.3，自动处理常见字符集、相对链接与不规范 XML。
- 三栏阅读界面：订阅侧栏、文章列表、正文阅读区；侧栏与列表宽度固定，正文随窗口伸展。
- 文章列表左对齐，每条三行（标题、摘要、时间 · 订阅名）；列表头部始终显示当前范围的文章总数，分页时补充已加载数量。
- 智能视图：全部文章、未读、星标、今天；文件夹支持三级树形结构，可逐级折叠，订阅直接显示在所属文件夹下。首次启动会创建「默认」文件夹，所有未指定文件夹的订阅都放在这里。
- 添加订阅时先尝试给出的地址，再读取网页里的 `<link rel="alternate">`，最后尝试 `/feed`、`/rss` 等常见路径；添加失败时会说明是网络/代理问题还是确实没有订阅源。
- 网络代理：优先读取 `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY` 环境变量；Windows 上还会读取系统代理设置，本机回环地址始终直连。
- 条件请求：保存 `ETag` 与 `Last-Modified`，内容未变化时服务器返回 304，不重复下载；订阅还没有文章时会强制下载全文，文章写入失败则清空 ETag，避免 304 让列表一直为空。
- 文章去重：优先用 GUID，其次用链接，最后用标题、正文与时间的摘要；重复抓取不会产生重复文章，也不会丢失已读/星标状态。
- HTML 正文转 Markdown 后阅读，图片经应用内缓存加载，链接在系统浏览器中打开。
- 全文搜索：SQLite FTS5 trigram 索引，两个字符以下的词自动退回 `LIKE`；输入停顿后即时搜索，回车保存最近 8 条搜索记录。
- OPML 导入/导出：保留多层文件夹树（zrss 最多展示三级，更深的层级会并入第三级），导入时跳过已有订阅。
- 已读、星标、全部标为已读、按订阅清理旧文章；默认每个订阅保留最近 500 篇，未读与星标文章额外保留。
- 浅色 / 深色 / 跟随系统，五种强调色，可调正文字号。
- Windows / Linux 通知区域与 macOS 菜单栏图标：刷新、全部标为已读、设置、退出；关闭窗口可留在托盘继续刷新。
- 定时刷新（手动 / 15 分钟到 6 小时）、新文章桌面通知、后台刷新失败提示。
- 应用内检查更新：启动后检查 GitHub Releases，之后每天一次；更新包经过 Ed25519 签名校验，可自动下载并在下次启动时生效。
- 设置、搜索历史、上次查看的范围与窗口状态都会恢复。

## 快捷键

| 快捷键 | 操作 |
| --- | --- |
| `Cmd/Ctrl + R` | 刷新全部订阅 |
| `Cmd/Ctrl + N` | 添加订阅 |
| `Cmd/Ctrl + F` | 聚焦搜索 |
| `Cmd/Ctrl + ,` | 打开设置 |
| `Cmd/Ctrl + O` | 在浏览器中打开原文 |
| `Cmd/Ctrl + Shift + I` / `E` | 导入 / 导出 OPML |
| `Cmd/Ctrl + 1` … `4` | 全部 / 未读 / 星标 / 今天 |
| `Cmd/Ctrl + =` / `-` | 增大 / 减小正文字号 |
| `J` / `K` 或 `↓` / `↑` | 下一篇 / 上一篇 |
| `U` | 标为已读 / 未读 |
| `S` | 加星标 / 取消星标 |
| `M` | 把当前列表全部标为已读 |
| `Esc` | 清除搜索 |

## 构建

需要 Go 1.27 或更新版本。

```sh
go tool mygo dev     # 开发模式运行
go tool mygo build   # 打包到 build/
go test ./...        # 运行测试
go vet ./...         # 静态检查
```

带更新清单的本地构建使用脚本（会读取 `.update-keys/mygo-update.key`）：

```sh
sh scripts/keygen.sh   # 第一次：生成更新签名密钥
sh scripts/build.sh    # 构建并生成 update-<platform>.json
```

脚本是 POSIX `sh`，Windows 自带 PowerShell 不能直接运行，需要用 Git for Windows 的 Git Bash，或 WSL：

```powershell
& 'C:\Program Files\Git\bin\bash.exe' -lc 'sh scripts/build.sh'
```

在 Git Bash 里也可以直接 `sh scripts/build.sh`。

`go tool mygo build` 默认只构建当前平台。发布工作流会分别构建 `darwin/arm64`、`darwin/amd64`、`windows/amd64`、`windows/arm64`、`linux/amd64` 与 `linux/arm64`。

应用图标与托盘图标由 `tools/gentray` 生成：

```sh
go run ./tools/gentray .
```

## 数据与隐私

数据库位于 MyGo 的用户数据目录：

- macOS：`~/Library/Application Support/zrss/zrss.db`
- Windows：`%AppData%\zrss\zrss.db`
- Linux：`$XDG_CONFIG_HOME/zrss/zrss.db`（默认 `~/.config/zrss/zrss.db`）

数据库使用 WAL 模式，包含 `feeds`、`folders`、`articles`、`articles_fts`、`settings`、`searches` 等表；`folders` 通过 `parent_id` 组成最多三级的树，同一父级下文件夹名不区分大小写唯一。设置窗口可以查看数据库大小、执行 `VACUUM` 整理，或通过 OPML 迁移订阅。

zrss 只在你刷新订阅、打开文章图片或加载订阅图标时访问网络；没有遥测、统计或后台账号请求。删除订阅会同时删除它的文章，取消订阅前会二次确认。

## 发布

`.github/workflows/ci.yml` 在 Ubuntu、macOS 与 Windows 上运行 `go vet`、`go test` 和 `go build`，并在 Ubuntu 上打一次 Linux 包。

`.github/workflows/release.yml` 在推送 `v*` 标签时：

1. 检查标签与 `mygo.json` 的 `version` 一致；
2. 从 `CHANGELOG.md` 取本版本说明，创建草稿 Release；
3. 在三个系统上并行打包；
4. 用 `MYGO_UPDATER_PRIVATE_KEY` 签名更新包并生成 `update-<platform>.json`；
5. 通过 `mygo build -upload` 把安装包、更新包与清单附到草稿 Release，人工检查后发布并标记 latest。

发版流程、脚本参数与故障排查见 [`docs/版本更新设计.md`](docs/版本更新设计.md)。快速命令：

```sh
sh scripts/release.sh            # 检查 + 带签名构建
sh scripts/release.sh --tag      # 打标签并触发 CI
sh scripts/release.sh --publish  # CI 完成后发布并验证更新源
```

Windows 上通过 Git Bash 执行，例如 `& 'C:\Program Files\Git\bin\bash.exe' -lc 'sh scripts/release.sh --tag'`。

默认构建是未签名的：macOS 应用会被 Gatekeeper 拦截，Windows 会显示 SmartScreen 提示。正式分发前，在 `mygo.json` 中配置 `macos.signingIdentity`、`macos.notarize` 与 `windows.certificate`，并按 [MyGo 的发布文档](https://github.com/egoist/mygo/blob/main/docs/distribution.md) 在 CI 中提供证书和公证凭据。
