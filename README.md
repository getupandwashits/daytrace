# DayTrace

<img src="Resources/icon.png" width="96" align="right" alt="DayTrace">

[![CI](https://github.com/getupandwashits/daytrace/actions/workflows/ci.yml/badge.svg)](https://github.com/getupandwashits/daytrace/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

DayTrace 是一个 macOS 菜单栏应用，把**自动专注计时**和**前台活动记录**合在一起：它在本机记录你实际做了什么——前台应用、窗口、屏幕上的文字、专注时段——每天整理成四层 Markdown，让 AI 或你自己能由粗到细地回看一天。

*DayTrace is a local-first macOS menu-bar app: it records the foreground app, window, on-screen text and focus sessions, then writes a layered daily Markdown archive. No screenshots, no keyboard hooks, no network.*

- **活动采集**：每 10 秒（切换应用时立即）读一次前台窗口的 Accessibility 文字，只在本机落盘；不截屏、不挂钩键盘、不联网。
- **自动专注**：按系统闲置时间判断专注、短暂停顿、离开，自动开始/暂停计时；回来时要连续用键鼠（10 秒内至少两次输入）才恢复，单次误触不会启动新的一轮；到 25 分钟弹窗，连按两下空格或点「开始休息」进入带倒计时的休息。
- **每日归档**：北京时间 23:00 输出「阅读入口 + 活动摘要 + 时间线 + 全文分段」和压缩原始记录，交给本机的 AI 或总结流程。
- 菜单栏常驻、仪表盘可查；不注册 launchd，退出应用即停止采集。

## 快速开始

需要 macOS 12 或更新、Go 1.26+，以及 Xcode Command Line Tools（Viva 通过 cgo 调用 Cocoa/Metal）。

```bash
git clone https://github.com/getupandwashits/daytrace.git
cd daytrace
./scripts/build.sh --install
```

`build.sh` 会先跑 `go test`、`go vet` 和 `go tool viva vet`，再用 Viva 打包 `build/DayTrace.app` 并安装到 `/Applications`。也可以不带 `--install`，只构建到 `build/`。

首次启动后：

1. 菜单栏出现倒计时（专注剩余时间 / 休息倒计时）。
2. 打开「系统设置 → 隐私与安全性 → 辅助功能」，勾选 DayTrace；没有这一步只能记录应用名和窗口标题，读不到界面文字。
3. 从菜单栏菜单打开仪表盘，在「设置」里确认输出目录（默认 `~/Documents/DayTrace`）。

应用是菜单栏附件（`LSUIElement`），不会出现在 Dock；点菜单栏菜单里的「打开仪表盘」查看概览、历史和设置。

> 目前只提供源码构建。`.app` 使用 ad-hoc 签名，没有经过 Apple 公证；把构建出的应用拷到别的机器时，Gatekeeper 可能要求右键打开。

## 技术选型

- **Go 1.26+**，[Viva](https://github.com/vivaui/viva) `v0.6.5`（Go-first、Cocoa/Metal、无 WebView）。
- macOS 上 Viva 通过 cgo 调用 Cocoa/Metal，需要 Xcode Command Line Tools。
- Accessibility / CoreGraphics 用少量 cgo 直接调用，不需要 Swift helper 或独立采集进程。
- 存储保持普通文件：活动 JSONL、专注 JSONL、设置 JSON；不引入数据库。

## 目录

```text
daytrace/
├── cmd/
│   ├── daytrace/          # 菜单栏应用（Viva）
│   └── daytrace-agent/    # 调试 / 运维 CLI
├── internal/
│   ├── core/              # 路径、设置、存储、专注引擎、摘要、归档、兼容状态文件
│   ├── macos/             # cgo：AX 采集、空闲、锁屏、原生菜单栏项、休息卡片
│   └── ui/                # Viva 页面与菜单
├── Resources/icon.png     # 应用图标（打包时转成 .icns）
├── scripts/build.sh       # 构建 .app
├── swift/                 # Swift 原型（行为与输出格式的参考实现）
└── README.md
```

## 开发

```bash
cd daytrace

go mod tidy            # 首次拉依赖
go test ./...          # 无窗口测试（含 Viva Driver 的 UI 测试）
go vet ./...
go tool viva vet
go tool viva dev .     # 热重载运行
./scripts/build.sh     # 构建 build/DayTrace.app（ad-hoc 签名）
./scripts/build.sh --install   # 构建并安装到 /Applications
```

调试时用环境变量把数据完全隔离，不碰正式数据：

```bash
DAYTRACE_HOME="$PWD/.dev/home" \
DAYTRACE_OUTPUT="$PWD/.dev/output" \
DAYTRACE_LEGACY_STATUS_DIR="$PWD/.dev/legacy/AutoPomodoro" \
./build/DayTrace.app/Contents/MacOS/daytrace

# 想让它启动就显示仪表盘（默认只驻留菜单栏）
DAYTRACE_SHOW_DASHBOARD=1
```

### CLI

`daytrace-agent` 和 GUI 读写同一份数据，用同样的 `DAYTRACE_*` 环境变量指定位置：

```bash
go build -o .build/daytrace-agent ./cmd/daytrace-agent

./.build/daytrace-agent doctor              # 路径、权限、兼容状态文件归属
./.build/daytrace-agent status              # 读 status.json
./.build/daytrace-agent sample              # 立即采集一条前台窗口并打印
./.build/daytrace-agent collect             # 前台采集，Ctrl-C 结束
./.build/daytrace-agent export --force      # 立即归档
./.build/daytrace-agent export --due        # 只有到点才归档
./.build/daytrace-agent analyze 2026-11-19  # 打印当天分析
./.build/daytrace-agent demo                # 合成数据跑一遍完整归档（需要隔离目录）
```

`demo` 会拒绝在正式数据目录上运行，必须同时设置 `DAYTRACE_HOME` 和 `DAYTRACE_OUTPUT`。

## 数据位置

| 内容 | 路径 |
| --- | --- |
| 设置 | `~/Library/Application Support/DayTrace/settings.json` |
| 活动采样 | `~/Library/Application Support/DayTrace/journal/events/YYYY-MM-DD.jsonl` |
| 专注记录 | `~/Library/Application Support/DayTrace/sessions/sessions.jsonl` |
| 运行状态 | `~/Library/Application Support/DayTrace/status.json` |
| 归档状态 | `~/Library/Application Support/DayTrace/export-state.json` |
| Daily_record 输出 | 默认 `~/Documents/DayTrace`，可在设置里改 |

删除 `~/Library/Application Support/DayTrace` 即可清空全部本机数据。

## 兼容旧版（可选）

DayTrace 来自作者之前自用的两个应用，保留了它们的外部读取契约；只有在你还在用那些读取方时才需要关心：

1. **AutoPomodoro 状态文件**：`~/Library/Application Support/AutoPomodoro/status.json` 和沙盒容器里的同名文件，`source` 为 `autopomodoro`，字段、秒级 ISO8601 时间与旧版一致。旧 AutoPomodoro 进程在运行时（或 5 秒内有别的存活进程写过）退让不写，退出后接管。
2. **Daily_record 归档**：定时文件名 `YYYY-MM-DD_23-00`，手动文件带 `_manual`；四层输出 `.md`、`.ui/digest.md`、`.ui/timeline.md`、`.ui/part-NNN.md`；原始数据 `.raw/<name>.jsonl.gz`；`.meta.json` 含 `cutoff`、`first_timestamp`、`last_timestamp`、`records`、`ui_format_version` 等键。归档时间晚于 23:00 时仍按 23:00 作为 cutoff，保证补跑不串档。
3. 不修改旧版本的数据、launchd 任务或已有记录。

兼容性联调可以用任何按 `.meta.json` 和四层文件读取记录的脚本（`load_groups`、`read_entry`、`build_batches` 是一套典型用法）。

## 安全与隐私

- 只读取当前前台窗口的 Accessibility 文本；不截屏、不挂钩键盘、不联网。
- 跳过密码框（secure text field）、密码管理器和系统私密界面；URL 去掉 query 和 fragment。
- 捕获到的界面文字按数据处理，不当作指令。
- 归档里的疑似密钥/密码在摘要层替换为 `〔已遮蔽〕`，原始文本只留在本机 `.raw/*.jsonl.gz`。

## 权限

读取窗口文字需要「系统设置 → 隐私与安全性 → 辅助功能」授权。ad-hoc 签名的 `.app` 每次重新构建后签名会变，需要重新勾选 DayTrace（或先 `-` 移除旧条目再添加）。菜单栏菜单和专注计时不需要任何权限。

## 已知限制

- 采集进程随应用运行，不常驻；应用退出期间没有记录，归档里会体现为采样空档。
- 系统睡眠没有单独的 `sleeping` 状态，只体现为采样空档。
- 归档只覆盖当天；跨天补跑需要手动 `export --force`。
- 已完整交付的缓存日期在归档后清理，最近 7 天保留（供跨归档的「沿用」文字引用）。
- 少数应用不通过 Accessibility 暴露正文（例如 Zed 的编辑器），这类窗口只能记录应用名、窗口标题和路径。
- 系统睡眠、合盖或屏幕锁定期间不会开始新的专注会话。

## 致谢与参考

DayTrace 是一个独立的 Go 实现。专注计时和活动记录的行为、格式参考了下面两个第三方项目，没有复制它们的代码：

- [**AutoPomodoro**](https://github.com/MaTwickenham/AutoPomodoro)：按系统闲置时间判断专注/停顿/离开的菜单栏番茄钟。DayTrace 按同样的状态机语义重写了专注计时，并继续写与它兼容的状态文件。该项目目前没有声明开源许可证，因此只参考行为，没有复制代码。
- [**Beayy**](https://github.com/hengfengliya/beayy/tree/v0.2.19)（v0.2.19，MIT）：采集思路和「先摘要、再按需回看原始记录」的分层结构——10 分钟摘要 → 小时摘要 → 日总结，原始事件留在本机。DayTrace 把这种分层落成了每天四个 Markdown 文件，交给 AI 由粗到细地读。只借鉴设计，没有复制代码。

如果之后引用或移植了上述项目的代码，请分别遵守它们的许可证（Beayy 为 MIT，AutoPomodoro 未声明许可）。

### 关于前一版

DayTrace 的前一版是作者自用的 MyMonitor（Python + Swift 实现），前后是同一个项目，本仓库是它的 Go 重写，旧版不再单独维护。

如果机器上还留着它的 `~/Library/Application Support/MyMonitor/settings.json`，DayTrace 首次启动会沿用里面的 `output_dir`（只读）；没有就用 `~/Documents/DayTrace`。旧的数据目录、设置文件和已有 Daily_record 保持原样，不迁移、不删除，停掉旧的自动启动任务即可。

## 贡献

见 [CONTRIBUTING.md](CONTRIBUTING.md)。安全问题请按 [SECURITY.md](SECURITY.md) 的方式私下报告。

## 依赖与许可

DayTrace 以 MIT 许可证发布，全文见 [LICENSE](LICENSE)。

运行时依赖 [Viva](https://github.com/vivaui/viva)（Apache-2.0）和 macOS 系统框架（AppKit / Metal / Accessibility / CoreGraphics）。Viva 及其声明的第三方组件许可见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) 和 [LICENSES/Apache-2.0.txt](LICENSES/Apache-2.0.txt)；分发 `.app` 时请一并附上这两个文件。

Swift 原型的功能和兼容说明见 [`swift/README.md`](swift/README.md)。
