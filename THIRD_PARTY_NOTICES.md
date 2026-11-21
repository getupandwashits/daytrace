# Third-party notices

DayTrace 的发布物（源码和打包的 `DayTrace.app`）包含以下第三方软件。

## Viva

- 项目：https://github.com/vivaui/viva
- 版本：v0.6.5
- 许可证：Apache License 2.0，全文见 [LICENSES/Apache-2.0.txt](LICENSES/Apache-2.0.txt)

Viva 附带的 NOTICE：

```text
Viva
Copyright 2026 The Viva Authors

This product includes software developed by third parties:

- shadcn-svelte (MIT License)
- Lucide (ISC License; portions are derived from Feather under the MIT License)
- Unicode Character Database (Unicode License V3)
- Ghostty (MIT License)
- github.com/ebitengine/purego (Apache License 2.0)
- github.com/pelletier/go-toml/v2 (MIT License)
- golang.org/x/text (BSD 3-Clause License)
```

Viva 仓库的 `THIRD_PARTY_NOTICES.md` 包含上述组件的完整许可证文本：https://github.com/vivaui/viva/blob/v0.6.5/THIRD_PARTY_NOTICES.md

## 分发包

构建脚本会在应用包内附带 DayTrace 许可、Viva 的 Apache-2.0 文本和完整第三方声明，以及 Go 运行时许可。相关文件位于 `Contents/Resources/`。
