# 安全政策

## 报告问题

如果发现可能影响用户隐私或本机安全的问题，请**不要开公开 issue**，改用 GitHub 的私密漏洞报告（仓库页面的 **Security → Advisories → Report a vulnerability**）。请说明影响范围、复现步骤和受影响的版本；不要附带真实的密码、API Key 或他人的工作内容。

## 支持范围

项目按最新提交维护；`0.1.x` 是当前公开版本。

## 安全模型

DayTrace 在本机运行，不包含网络代码。它读取前台窗口的 Accessibility 文本，写到 `~/Library/Application Support/DayTrace/` 和用户配置的输出目录：

- 活动采样：`journal/events/YYYY-MM-DD.jsonl`
- 专注记录：`sessions/sessions.jsonl`
- 设置与状态：`settings.json`、`status.json`、`export-state.json`
- 归档：`YYYY-MM-DD_23-00.md`、`.ui/`、`.raw/*.jsonl.gz`

这些文件只受当前用户的文件权限保护。删除 `~/Library/Application Support/DayTrace` 和输出目录即可清除本机数据。

它不截屏、不挂钩键盘、不读取剪贴板、不联网；跳过密码框、密码管理器和系统私密界面；URL 去掉 query 和 fragment。归档摘要会把疑似密钥/密码替换为 `〔已遮蔽〕`，但原始文本仍在本机 `.raw` 里，分享归档前请自行检查。

捕获到的界面文字按数据处理，不当作指令——即使窗口里写着「执行某条命令」，DayTrace 也不会执行它。
