# Third-Party Notices

本项目包含或构建时使用第三方组件。MIT License 只覆盖本项目有权授权的自有代码、文档和资源文件；第三方组件继续适用各自许可证。

## Node.js

- `archiver` 8.0.0 — MIT License。完整依赖树及锁定版本见 `package-lock.json`。

## Go

生产二进制链接 `go-script/go.mod` 和 `go-script/go.sum` 中声明的 Go 模块，包括：

- `github.com/emersion/go-imap/v2`
- `github.com/emersion/go-message`
- `github.com/gpdf-dev/gpdf`
- `github.com/klippa-app/go-pdfium`
- `github.com/makiuchi-d/gozxing`
- `github.com/shopspring/decimal`
- `github.com/signintech/gopdf`
- `github.com/tetratelabs/wazero`
- `github.com/xuri/excelize/v2`
- `github.com/yuin/goldmark`
- `golang.org/x/image`
- `golang.org/x/sys`
- `golang.org/x/text`

各模块的精确版本和间接依赖以 `go-script/go.mod`、`go-script/go.sum` 为准；再分发者应同时遵守相应上游许可证。

本仓库发布的是源码快照，不包含 `node_modules`、`dist` 或预编译二进制。若分发构建后的可执行文件或安装归档，必须额外随包提供实际链接依赖对应的完整许可证和 NOTICE 文本；本摘要不能替代二进制分发所需的完整材料。

`tax-analysis` 的安装归档会额外生成 `licenses/`，包含实际随包 Node.js、JS/WASM 图表组件和报告 Go 二进制链接依赖的完整许可证；具体文件及哈希见 `tax-analysis/references/third-party-license-manifest.json`。Node.js 只作为固定版本便携运行文件随当前平台资产分发，不使用 Bun。

## Fonts

Noto Sans SC 字体使用 SIL Open Font License 1.1。许可证原文随字体资源保存在：

- `go-script/lib/invoice-assistant-go/assets/fonts/OFL.txt`
- `go-script/lib/invoice-verify-common/assets/fonts/OFL.txt`
- `tax-analysis/assets/fonts/OFL.txt`

`tax-analysis` 使用 Noto Sans CJK 2.004 的官方 Simplified Chinese Variable TTF，固定 `wght=400/700` 生成 Regular/Bold 静态实例并规范字体元数据。安装后的 `assets/fonts/OFL.txt` 保留许可证原文，字体和许可证按发布清单作为外置资产分发；具体来源、派生方式及哈希见 `tax-analysis/references/font-manifest.json`。
