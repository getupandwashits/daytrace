# DayTrace Swift 原型

这里保存了早期的 Swift 实现，供回顾专注引擎、Accessibility 采集和 Markdown 分层设计。当前发布版的安装、数据及命令见 [DayTrace README](../README.md)。

原型使用自己的应用标识和 `~/Library/Application Support/DayTracePrototype` 数据目录。开发脚本用 `swift/.dev/` 存放测试日志和归档。

从仓库根目录运行以下命令。需安装完整的 Xcode，运行原型需要 macOS 15 或更新。

```bash
cd swift
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer swift test
./scripts/run-dev.sh
```
