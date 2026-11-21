# 贡献指南

感谢你愿意改进 DayTrace。这个项目是一个 macOS 菜单栏应用，目标是**在本机**记录活动并生成可读的每日归档，所以隐私边界比功能数量更重要。

## 环境

- macOS 12 或更新
- Go 1.26+，发布构建固定使用 Go 1.26.11
- Xcode Command Line Tools（`xcode-select --install`）

## 构建与测试

```bash
go mod tidy
go test ./...
go vet ./...
go tool viva vet
./scripts/build.sh          # build/DayTrace.app
./scripts/build.sh --install
```

提交前请至少跑通 `go test ./...`、`go vet ./...` 和 `go tool viva vet`。改了 UI 或原生代码时，再跑一次 `./scripts/build.sh` 确认真实应用能启动。

## 代码约定

- 用 `gofmt`；Go 代码保持现在的包结构（`internal/core` 业务、`internal/macos` cgo、`internal/ui` Viva 页面）。
- 新增行为先写或补测试；专注引擎和归档格式的测试在 `internal/core`。
- UI 页面是声明式的：每次重绘从 model 重新声明，不要保存 `ui.C` 或闭包。
- 原生菜单栏项在 `internal/macos/status_darwin.*`，不要改回 Viva 的 `AddStatusItem`（它只支持图标，没有文字）。
- `main` 第一行必须保留 `runtime.LockOSThread()`，否则从 LaunchServices 启动时窗口创建会死锁。

## 不能破坏的边界

- 采集由用户开启。读取前台窗口的 Accessibility 文本，跳过安全输入控件和已知密码管理器。
- 捕获到的界面文字作为数据处理。摘要遮蔽可识别的密钥，原文按私密数据处理。
- 自动专注需要连续输入恢复，保留专注、短暂停顿和休息的切换逻辑。
- 测试归档时使用合成采样和临时目录。归档发布失败时保留待归档日志。

## 提交与 PR

- 一个 PR 聚焦一件事；说明动机、行为变化和验证方式。
- 涉及 UI 的改动附截图或录屏（不要包含真实的工作内容、窗口标题或密钥）。
- 提交信息用祈使句，例如 `fix: keep rest countdown when the screen locks`。
- 不要提交构建产物（`build/`、`.build/`、`swift/.build/`）或本机数据。

## 问题反馈

- Bug 请附 macOS 版本、DayTrace 版本（`./.build/daytrace-agent doctor` 的输出）、复现步骤和相关日志。
- 安全相关问题不要开公开 issue，见 [SECURITY.md](SECURITY.md)。
