# 第三方软件声明

`tax-analysis` 0.1.1 使用下列第三方组件，各组件适用其对应许可证。许可证原文保存在 `licenses/` 或 `assets/fonts/OFL.txt`；组件清单和SHA-256见 `references/third-party-license-manifest.json`、`references/chart-renderer-manifest.json` 和 `references/font-manifest.json`。

## Node.js 24.8.0

- 项目与固定版本：https://github.com/nodejs/node/tree/v24.8.0
- 官方发布文件：https://nodejs.org/download/release/v24.8.0/
- 许可证：Node.js 主体采用 MIT License，同时包含其二进制所带第三方组件的许可证和声明
- 完整许可证及第三方声明：`licenses/NODE-LICENSE.txt`

## JS/WASM 图表组件

### @resvg/resvg-wasm 2.6.2

- 项目与对应源代码：https://github.com/thx/resvg-js/tree/v2.6.2
- 许可证：Mozilla Public License 2.0
- 完整许可证：`licenses/RESVG-WASM-MPL-2.0.txt`

本项目未修改上游 `@resvg/resvg-wasm` 文件。随包的 `index.mjs` 和 `index_bg.wasm` 与清单固定版本保持一致。

### opentype.js 1.3.4

- 项目：https://github.com/opentypejs/opentype.js/tree/1.3.4
- 许可证：MIT License
- 完整许可证：`licenses/OPENTYPE-JS-MIT.txt`

随包的 `opentype.js` 浏览器构建还内嵌以下上游代码，对应许可证一并保留：

- `string.prototype.codepointat` 0.2.1 — MIT License，`licenses/STRING-CODEPOINTAT-MIT.txt`
- `tiny-inflate` 1.0.3 — MIT License，`licenses/TINY-INFLATE-MIT.txt`

## 主程序第三方组件

主程序包含以下第三方组件及对应许可证：

- `github.com/signintech/gopdf` v0.36.1 — MIT License，`licenses/GOPDF-MIT.txt`
- `github.com/phpdave11/gofpdi` v1.0.14-0.20211212211723-1f10f9844311 — MIT License，`licenses/GOFPDI-MIT.txt`
- `github.com/pkg/errors` v0.8.1 — BSD 2-Clause License，`licenses/PKG-ERRORS-BSD-2-CLAUSE.txt`
- `github.com/yuin/goldmark` v1.8.2 — MIT License，`licenses/GOLDMARK-MIT.txt`
- `golang.org/x/image` v0.25.0 — BSD 3-Clause License，`licenses/GO-X-IMAGE-BSD-3-CLAUSE.txt`
- `golang.org/x/sys` v0.41.0 — BSD 3-Clause License，`licenses/GO-X-SYS-BSD-3-CLAUSE.txt`
- `golang.org/x/text` v0.34.0 — BSD 3-Clause License，`licenses/GO-X-TEXT-BSD-3-CLAUSE.txt`

## Noto Sans SC

- 上游项目：https://github.com/notofonts/noto-cjk/tree/Sans2.004
- 固定版本：Noto Sans CJK 2.004（Sans2.004）
- Copyright 2014-2021 Adobe (http://www.adobe.com/), with Reserved Font Name `Source`.
- Copyright 2014-2021 Google LLC. All Rights Reserved.
- 许可证：SIL Open Font License 1.1

报告字体 `NotoSansSC-Regular.ttf` 和 `NotoSansSC-Bold.ttf` 是从该固定版本的 Simplified Chinese Variable TTF 生成的 400/700 静态实例；派生过程没有使用保留名称 `Source`。许可证全文随字体保存在 `assets/fonts/OFL.txt`，固定来源、派生方式、文件大小和 SHA-256 见 `references/font-manifest.json`。字体仅随 Skill 和生成文档一并分发，不作为独立字体产品销售。
