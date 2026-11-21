# DayTrace（Swift 原型）

> 这是重构时的 Swift 原型，**仅作行为、界面和输出格式的参考**；主实现是仓库根目录的 Go 版，功能和设置以根目录 [README](../README.md) 为准。原型里保留的 45 分钟默认值、传统番茄模式和 `~/Documents/MyMonitor` 默认目录都是历史行为，Go 版已不再使用。

把 **AutoPomodoro（自动专注计时）** 和 **MyMonitor（前台活动采集 + 每日 Markdown 归档）** 合并成一个菜单栏应用的本地重构版。

当前状态是**可运行的 Swift 原型**：功能已经打通，并兼容旧应用的两个外部读取契约（AutoPomodoro 状态文件、MyMonitor/Daily_record 归档目录）。数据写在独立目录，不导入旧数据，也不依赖旧应用的 launchd 任务。

## 已实现

### 专注计时（原 AutoPomodoro）

- 两种模式：
  - **自动专注**：默认 45 分钟目标，达到目标后提醒；无键鼠输入时暂停，超过重置阈值后回到休息。
  - **传统番茄钟**：25 分钟专注 + 5 分钟短休息，每 4 轮 15 分钟长休息。
- 状态机：`away` / `active` / `paused` / `reminding` / `stopped`。
- 菜单栏实时显示剩余时间，下拉菜单可暂停、继续、强制休息、重新开始。
- 提醒浮窗（`NSPanel`），应用退到后台也能显示。
- 睡眠、锁屏、显示器休眠时自动挂起计时。
- 每完成一段有效专注写入 `sessions.jsonl`，仪表盘统计今日/近 7 天专注时长。

### 活动采集（原 MyMonitor）

- 每 10 秒通过 macOS Accessibility API 读取前台应用、窗口标题、界面可见文字。
- 不抓屏幕、不装键盘钩子、不发网络请求。
- 界面文字做 SHA256 上下文去重；同一天内重复内容只记录一次。
- 跳过密码管理器、安全输入框；URL 去掉查询参数和片段。
- 空闲、锁屏、睡眠、前台应用切换都有状态标记。

### 每日归档（原 MyMonitor exporter）

- 默认每天北京时间 23:00 检查并归档（应用运行期间）。
- 四层输出：
  1. `YYYY-MM-DD_HH-mm_manual.md`：入口文件，概况、应用用时、逐小时活动。
  2. `.ui/digest.md`：按「小时｜应用｜窗口」整理的界面文字摘要。
  3. `.ui/part-NNN.md`：按段号拆分的去重原文。
  4. `.ui/timeline.md`：窗口切换时间线。
- 另存 `.meta.json` 和 `.raw/*.jsonl.gz`，方便核对。
- 摘要规则：去掉流式草稿、重复数字/空格变体、标题回声、装饰线；合并碎片；按活跃时长分配摘录预算；遮蔽常见密钥格式。
- 归档水位记录在 `export-state.json`，可强制重新生成当天记录。

## 构建与运行

要求 macOS 15+，Xcode 15.3+（需要 Xcode 的 SwiftUI 宏插件，不能只用 Command Line Tools）。

```bash
cd ~/code/apps/daytrace

# 构建 .build/DayTrace.app 和 CLI
./scripts/build.sh

# 开发模式：数据和输出都放在项目内 .dev/
./scripts/run-dev.sh
```

`run-dev.sh` 会设置：

```bash
DAYTRACE_HOME=.../daytrace/.dev/home
DAYTRACE_OUTPUT=.../daytrace/.dev/output
```

`DAYTRACE_OUTPUT` 会覆盖设置里的输出目录，确保调试数据不会写到真实归档目录。`DAYTRACE_LEGACY_STATUS_DIR` 把兼容状态文件也放到 `.dev/legacy/AutoPomodoro/`，避免和正在运行的旧 AutoPomodoro 抢文件。

正常安装运行时（不设置环境变量）：

- 数据：`~/Library/Application Support/DayTrace/`
- 归档：优先读 `~/Library/Application Support/MyMonitor/settings.json` 的 `output_dir`（可自行指定）；没有配置时用 `~/Documents/MyMonitor/`

## 外部写入兼容

### 1. AutoPomodoro 状态文件

DayTrace 会按原 schema 写入：

```text
~/Library/Application Support/AutoPomodoro/status.json
~/Library/Containers/twickenhamma.AutoPomodoro/Data/Library/Application Support/AutoPomodoro/status.json
```

字段与旧版一致：`schemaVersion`、`source: "autopomodoro"`、`processID`、`updatedAt`、`state`、`stateLabel`、专注/休息秒数、`remainingFraction`、`trackingSuspended`、`mode`、`phase`、`completedPomodoros`、`pomodorosPerCycle`。

- 旧 AutoPomodoro 还在运行时，DayTrace 会检测到它，**不写**这两个文件，避免两边每秒互相覆盖。
- 退出旧 AutoPomodoro 后，DayTrace 会在 5 秒内接管；`scripts/read-status.py --watch` 和键盘点阵屏服务不需要改动。
- 检查当前占用情况：

```bash
daytrace-agent legacy
```

### 2. MyMonitor / Daily_record 归档

默认输出目录直接复用 MyMonitor 的 `output_dir`，并保持它的文件契约：

- 定时文件名：`YYYY-MM-DD_23-00.md`，即使 23:00 的检查迟到也保持这个文件名和截止时间；23:00 之后的尾部进入下一天。
- 手动文件名：`YYYY-MM-DD_HH-mm_manual.md`。
- 目录结构：`.ui/digest.md`、`.ui/timeline.md`、`.ui/part-NNN.md`、`.raw/<name>.jsonl.gz`。
- `.meta.json` 同时包含 Daily_record 读取的键：`cutoff`、`first_timestamp`、`last_timestamp`、`records`、`ui_format_version`，以及 DayTrace 自己的统计字段。

已用一套按 `.meta.json` 读取记录的脚本（`load_groups`、`pick_for_date`、`read_entry`、`build_batches`）对一个临时输出目录做过联调，能识别定时记录并生成阅读批次。

## 调试命令

```bash
cd ~/code/apps/daytrace
export DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer
export DAYTRACE_HOME="$PWD/.dev/home"
export DAYTRACE_OUTPUT="$PWD/.dev/output"

# 路径、权限、样本数、专注段数、最近归档
.build/DayTrace.app/Contents/Helpers/daytrace-agent doctor

# 生成一段演示采样并立即归档（不改真实数据，前提是使用 .dev 路径）
.build/DayTrace.app/Contents/Helpers/daytrace-agent demo --minutes 360 --reset

# 立即归档（--force 重新生成今天）
.build/DayTrace.app/Contents/Helpers/daytrace-agent export --force

# 执行一次到点检查
.build/DayTrace.app/Contents/Helpers/daytrace-agent due

# 采集一次当前前台窗口并打印 JSON
.build/DayTrace.app/Contents/Helpers/daytrace-agent sample

# 检查 AutoPomodoro 兼容状态文件的路径、占用者和内容
.build/DayTrace.app/Contents/Helpers/daytrace-agent legacy

# 打印当前设置
.build/DayTrace.app/Contents/Helpers/daytrace-agent settings
```

也可以直接跑 SPM：

```bash
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer swift run daytrace-agent doctor
```

## 测试

```bash
cd ~/code/apps/daytrace
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer swift test
```

当前 15 项测试覆盖：

- 路径稳定性与调试目录隔离
- 自动专注的启动、空闲暂停、长空闲重置
- 传统番茄钟完成、短休息切换
- 手动停止/继续时保留进度
- 睡眠/锁屏挂起与恢复
- 短专注不写记录
- 摘要去噪、密钥遮蔽、碎片合并
- 归档四层文件与 meta 输出
- 专注记录 JSONL 持久化
- 23:00 之后启动时仍生成 `_23-00` 定时文件，并以 23:00 为截止
- AutoPomodoro 兼容状态文件的 schema、`phase` 和旧写入者占用时的退让

## 目录结构

```text
daytrace/
├── Package.swift
├── Sources/
│   ├── DayTraceCore/        # 不依赖 UI 的核心库
│   │   ├── FocusEngine.swift
│   │   ├── ActivityCollector.swift
│   │   ├── ActivityAnalyzer.swift
│   │   ├── Digest.swift
│   │   ├── Exporter.swift
│   │   ├── Storage.swift
│   │   ├── Presence.swift
│   │   ├── Settings.swift
│   │   ├── LegacyStatus.swift
│   │   ├── Models.swift
│   │   └── Paths.swift
│   ├── DayTraceApp/         # SwiftUI 菜单栏应用
│   │   ├── DayTraceApp.swift
│   │   ├── AppModel.swift
│   │   ├── MenuBarView.swift
│   │   ├── TodayView.swift
│   │   ├── HistoryView.swift
│   │   ├── SettingsView.swift
│   │   └── ReminderPanel.swift
│   └── DayTraceAgent/       # 本地调试 CLI
├── Tests/DayTraceCoreTests/
└── scripts/
    ├── build.sh
    └── run-dev.sh
```

核心库只依赖 Foundation / AppKit / ApplicationServices / CryptoKit，不依赖 SwiftUI，方便单独测试，也方便以后给 CLI 或 launchd helper 复用。

## 与旧应用的关系

- 新 Bundle ID：`io.github.getupandwashits.daytrace`，与 `AutoPomodoro.app`、`MyMonitor.app` 不冲突。
- 新数据目录：`Application Support/DayTrace`；调试时是 `daytrace/.dev/home`。
- 不修改旧应用的容器、launchd 任务和已有记录；只读取 MyMonitor 的 `settings.json` 来复用输出目录。
- 旧 AutoPomodoro 运行时，DayTrace 不会覆盖它的状态文件；退出旧应用后自动接管。
- 旧应用可以继续照常使用，适合并行验证。

## 已知限制 / 暂缓项

- 活动采集目前由应用进程内定时器执行；**退出应用就停止采集**。原 MyMonitor 的 launchd 常驻采集尚未迁移。
- 23:00 自动归档需要应用在运行；没有后台 agent 时不会补跑。
- 归档按水位切分：如果应用连续多天没有运行，补归档会把跨天数据合并到一个记录文件；文件头会写明实际时间范围。
- 迟到的定时归档仍以 23:00 为截止，23:00 之后的尾部会进入下一天的记录（与原 MyMonitor 一致）。
- 不导入旧数据；新归档兼容 Daily_record 的读取契约，但不追求与旧 MyMonitor 字节级一致。
- `.raw` 的 7 天自动清理尚未单独实现；Daily_record 的总结/清理流程会整组删除。
- 没有接日历同步、通知中心提醒、应用图标、签名/公证。
- 仪表盘每 15 秒重新读取当天 JSONL；数据量大时可以改成增量缓存。
- 首次运行需要在「系统设置 → 隐私与安全性 → 辅助功能」里授权。开发模式重新构建后如果权限失效，重新添加 `.build/DayTrace.app`。

## 后续（Go 版）

Swift 版作为行为和格式参考保留。Go 版目标：

1. 用 Go + Viva 重写菜单栏应用和仪表盘。
2. 用 cgo 直接调用 Accessibility API 和 CoreGraphics，不依赖 Swift helper。
3. 保持本文件描述的外部写入契约。
4. 增加应用图标、签名和安装脚本。
5. 观察一周真实使用后，再停用旧应用。
