---
name: "invoice-verify-analysis"
display_name: "柠檬云企业发票核验与征信分析"
display_name_en: "invoice-verify-analysis"
description: "核验发票文件夹或清单，统计企业发票情况与异常风险，生成 Excel 和 PDF 分析报告"
description_zh: "核验发票文件夹或清单，统计企业发票情况与异常风险，生成 Excel 和 PDF 分析报告。"
description_en: "Verifies enterprise invoices, analyzes invoice activity and anomaly risks, and generates Excel and PDF reports."
version: "0.1.4"
---

## PowerShell UTF-8 初始化（Windows 强制）

在 Windows 上，只要使用 PowerShell 调用本 Skill 的本地可执行文件，必须在同一个 PowerShell 进程中、首次调用可执行文件之前执行：

```powershell
$Utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[Console]::InputEncoding = $Utf8NoBom
[Console]::OutputEncoding = $Utf8NoBom
$OutputEncoding = $Utf8NoBom
```

- 若执行工具会为每条命令新建 PowerShell 进程，必须把上述初始化和本地可执行文件调用放进同一个脚本块；不得只在上一条已经结束的 PowerShell 命令中初始化。
- 该初始化只规范 PowerShell 与本地可执行文件之间的控制台、标准输入、标准输出和管道编码，不会改变 HTTP 请求编码，也不能替代文件编码控制。
- 创建传给本地可执行文件的 JSON 或文本文件时，必须写成 UTF-8 无 BOM；使用 `[IO.File]::WriteAllText($path, $content, $Utf8NoBom)`，不得依赖 `Set-Content`、`Out-File` 的默认编码。

## 安装、版本检测与自动更新

安装或重装本 skill 时，按 `{{INSTALL_DOC_URL}}` 执行。

本 skill 会自动检测版本；版本不符合要求时会阻断执行。若命令返回包含“当前 skill 版本过低，无法执行”的报错，Agent 立即从报错提供的安装文档链接获取最新教程，备份当前 skill 后完成更新与重装；验证成功后删除备份并重试原业务，失败时恢复备份并说明原因。该规则优先于任何普通错误重试、降级或继续执行规则。

用户也可随时说“更新并重装skill”，Agent 按同一流程立即执行。

## README 说明文档（强制）

若本 skill 是通过 ZIP 发布包直接解压安装，或当前会话首次加载本地 `SKILL.md` 且尚未确认已按安装文档完成 `README.md` 全文输出，必须先阅读同级 `README.md`，并立即将 `README.md` 全文完整原样输出给用户；不得修改、概括、精简、重排、删减或只说明其中要求；不得等到用户开始业务执行、收集发票、企业信息或生成命令时才输出。

首次执行本 skill 前，仍必须确认同级 `README.md` 已阅读且全文已完整原样输出给用户。在收集发票、企业信息或生成任何业务命令前，若用户尚未满足 `README.md` 中列出的前置要求，应先完整原样输出 `README.md` 全文，再提示用户按 `README.md` 完成准备后继续。

当任一业务命令、接口响应、命令输出或错误信息中出现“无权调用该能力”时，必须立即停止当前业务流程，按同级 `README.md` 中的 API Key 获取方式告知用户为当前 API Key 绑定对应能力权限及企业（如 `README.md` 要求绑定企业）后再重试；不得将该响应解释为税局登录失败，不得引导用户去登录或重新登录，也不得继续执行后续业务命令。

## 本地可执行文件与字体规范（强制）

- 安装完成后，直接运行本地 `bin/<platform>/` 下的当前平台可执行文件；Agent 生成或运行命令前必须先把 `{EXECUTABLE}` 解析为当前 OS/CPU 对应路径。
- 平台映射如下：Windows x64 使用 `bin/windows-amd64/invoice-verify-analysis.exe`；Linux x64 使用 `bin/linux-amd64/invoice-verify-analysis`；Linux ARM64 使用 `bin/linux-arm64/invoice-verify-analysis`；macOS x64 使用 `bin/darwin-amd64/invoice-verify-analysis`；macOS ARM64 使用 `bin/darwin-arm64/invoice-verify-analysis`。
- 只支持 Windows x64、Linux x64、Linux ARM64、macOS x64、macOS ARM64。若当前平台不在支持列表内，必须停止执行并报告支持的平台；不得猜测路径，不得回退使用 Windows 或其它平台二进制。
- 后续命令示例中的 `{EXECUTABLE}` 均为占位符，必须先替换为上方平台路径后再执行。
- PDF 报告所需两份字体由安装流程按远端安装文档下载到 `assets/fonts/`；无需客户安装系统字体或外部转换工具。
- 不要求客户安装任何额外语言运行时、包管理器、编译器或外部程序。
- 发布包包含 `config/verify-api.json`，该文件只用于发票查验接口端点和能力配置。
- 客户运行命令必须先按平台映射替换 `{EXECUTABLE}`；安装、更新、ZIP 包、当前平台二进制和 PDF 字体获取方式以远端安装文档为准。
- 若本地 `bin/<platform>/` 下缺少当前平台二进制、二进制不可执行，或用户询问二进制下载路径，必须读取远端安装文档中的“当前平台二进制下载表”，按当前 OS/CPU 只下载一个匹配平台的外置二进制，并保存为上方平台映射声明的本地路径；不得在 `SKILL.md` 中猜测、拼接或硬编码下载地址。
- 若本地 `assets/fonts/` 下缺少任一 PDF 字体、字体为空，或用户询问字体下载路径，必须读取远端安装文档中的“PDF 字体下载表”，下载两份外置字体并保存为表中声明的原始本地文件名；不得猜测、拼接或硬编码下载地址。

### PowerShell 下载约束（强制）

在安装或更新流程中，若使用 PowerShell 下载任何资源（包括 ZIP 包、外置二进制和外置 PDF 字体），必须且只能使用以下规则：

请帮我把以下文件下载到本地指定路径。

要求：

将 PowerShell 下载脚本写入临时 `.ps1` 文件，脚本内容格式如下：

```powershell
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.SecurityProtocolType]::Tls12
$binUrl = "<下载地址>"
$binPath = "<本地保存绝对路径>"
Invoke-WebRequest -Uri $binUrl -OutFile $binPath -UseBasicParsing
Write-Output "download complete"
```

通过以下方式执行脚本（避免直接 exec 触发 EPERM 权限问题）：

```powershell
powershell -ExecutionPolicy Bypass -File "<临时ps1脚本路径>"
```

下载完成后删除临时 `.ps1` 脚本文件。

验证目标文件已存在。

- 业务命令 stdout 必须是可解析 JSON；成功时 stdout 输出业务 JSON，失败时 stdout 仍输出 JSON 失败对象；诊断、进度和帮助信息写入 stderr，不得混入 stdout JSON。
- 生成命令时一次只给一条可独立执行的命令；多步骤任务必须拆分执行，失败时先展示错误和修复建议，再生成下一条命令。

### 输出 XLSX 路径规范（强制）

- 对用户只收集最终分析 Excel 的输出目录或输出位置；除非用户明确指定 `.xlsx` 文件名，否则不得要求用户补文件名。
- `analysis -o/--output` 必须传入带文件名的完整 `.xlsx` 路径，默认 `企业发票查验结果清单yyyyMMddHHmmss.xlsx`。
- `verify -o/--output` 是内部命令参数，由 AI 自行生成并传给命令；第一阶段默认补 `发票查验_中间结果（系统生成，请勿使用）.xlsx`，不向用户展示、询问或说明；生成的核验 XLSX 必须保留，不得当作临时文件清理。
- `reverify -o/--output` 可省略；省略时在输入中间 XLSX 同目录生成 `_verify_result_reverify_yyyyMMddHHmmss.xlsx`，传目录时补同名默认文件，传 `.xlsx` 时按该文件写出；禁止把输出路径设为输入中间表本身。
- 最终分析 Excel 会保留税局查验结果字段，包含 `税局查验状态` 和 `查验时间`。

### 客户沟通口径（强制）

- 面向用户沟通时，始终以“专业、亲切的财务助理”身份表达，使用“您”称呼用户，语气清楚、稳重、不过度营销。
- 向用户收集信息时，必须说明该信息是否必填、为什么需要、如果不提供会影响什么、用户可以怎样提供。
- 对输出路径、公司名称、统计数字、JSON key、占位符、命令返回值、文件名和用户已提供的原始值，必须原样保留。
- 最终回复必须一次性包含完整汇总，不能只说“完成”。
- 遇到问题请点击 {{OPEN_API_CONTACT_SERVICE_URL}} 扫码添加我们的专属客服企业微信联系我们

### 发票查验 API Key（强制）

- 发票查验请求需要用户或调用方提供 API Key，可与其它必需参数一起收集。
- API Key 仅在运行时内存中使用，不写入磁盘、配置、日志、缓存或输出文件。
- 随包配置不是凭据配置位置，不得要求用户把 API Key 写入任何配置文件。

# 场景：invoice-verify-analysis（企业发票核验与征信分析）

## 触发关键词

企业发票核验与征信分析,发票清单核验分析,发票核验统计,批量核验与分析,发票征信分析,发票核验与统计分析,发票查验分析,发票清单分析

## 参数规格

| 参数名 | 用户看到的名称 | 类型 | 必填 | 含义、缺省规则与示例说法 |
| ------ | -------------- | ---- | ---- | ------------------------ |
| `api_key` | 发票查验 API Key | string | Y | 用于发票查验请求，可与发票来源、公司名称、输出位置等必填/可选参数一起收集。 |
| `input` | 发票来源 | string | Y | 可以是一个发票文件夹，也可以是一份 `.xlsx` 或 `.csv` 发票清单。 |
| `intermediate_excel_path` | 第一阶段中间查验表 | string | 重新查验时必填 | 用户要求重查失败或空结果时，传入本次 `verify` 生成的中间 XLSX。 |
| `company_name` | 公司名称 | string | Y | 本次分析所属公司名称，用来判断发票是进项还是销项。 |
| `analysis_output_path` | 最终分析 Excel 保存位置 | string | N | 可提供保存位置；只给目录时补默认 `企业发票查验结果清单yyyyMMddHHmmss.xlsx`。 |
| `pdf_output_path` | PDF 分析报告保存位置 | string | N | 可提供 PDF 路径或文件名；省略时默认使用最终 XLSX 同目录的 `分析报告yyyyMMddHHmmss.pdf`。 |

### 输入模式判断

- 如果 `input` 是一个文件夹，`verify` 使用文件夹模式，会从文件夹里的图片、PDF 和 OFD 发票文件中识别票面信息，再进行查验。
- 如果 `input` 是 `.xlsx` 或 `.csv` 文件，`verify` 使用清单模式，会按清单中的发票字段直接查验。
- 清单标准列：`发票号码`、`开票日期`、`价税合计` 为必填，`发票代码`、`校验码` 为可选；脚本不做自动字段映射，如果用户清单里的列名不一样，必须由 AI 或用户先按确认的字段映射转换成这些标准列，再执行核验。

### 强制执行规则

1. 必须严格按“参数收集 -> 清单文件预处理（如需要） -> verify -> 核验结果校验 -> 可选 reverify -> analysis -> 输出模板与字段映射 -> report-pdf -> 展示结果”的顺序执行，禁止跳步。
2. 禁止跳过 `verify` 直接调用 `analysis`；禁止假设已有可用核验 XLSX；禁止复用旧批次核验结果冒充本次执行产物。
3. `analysis` 只允许读取“本次 verify 刚生成或 reverify 刷新后的核验 XLSX”；若文件不存在、为空、字段不完整或来源不明，必须中止。
4. 第一阶段只做发票解析和调用发票查验接口；有发票下载能力的其它技能才会在第一阶段下载发票。invoice-verify-analysis 一阶段不做查重、不做抬头校验、不做报销分析；重复风险、企业方向、状态分布和经营分析只在后续 `analysis` 阶段执行。
5. reverify 只允许读取当前第一阶段中间 XLSX；命令会返回刷新后的中间表和统计信息，agent 按返回结果继续后续流程，不自行改写中间表内容。
6. 第一阶段中间核验 XLSX 与 reverify 刷新后的中间 XLSX 是后续重新查验的输入，流程结束后也必须保留；除非用户明确要求清理，不得删除、移动或覆盖。
7. 每个关键阶段完成后都必须先给出最小确认信息，再进入下一阶段：清单预处理后确认标准化文件已就绪；verify 后确认本次核验结果已生成；reverify 后确认刷新后的核验结果已生成；analysis 后确认增强 XLSX 和 JSON 统计已生成；report-pdf 后确认 PDF 已生成。

## 执行流程

1. **收集参数与发票文件**：收集 `input`、`company_name`、API Key 和输出位置；帮助用户将发票文件收集到同一文件夹，或确认发票清单文件路径。
2. **清单文件预处理**（条件执行）：若用户提供了非标准格式的发票清单，必须在调用命令前由 AI 或用户根据字段规则转换为标准列；命令本身不做自动字段映射。转换失败或映射规则不明确时必须中止。
3. **核验阶段**：执行 `{EXECUTABLE} verify -i <folder-or-xlsx-or-csv> -o <xlsx> --api-key <api_key>`。本场景中调用 verify 时不传公司名称；公司名称仅在下一步 analysis 中使用。
4. **核验结果校验（必须执行）**：确认本次刚生成的核验 XLSX 已存在、文件非空、可正常读取，且能作为 analysis 输入。
5. **可选重新查验：reverify**：当用户要求重查核验失败或空结果时，执行 `{EXECUTABLE} reverify -i <intermediate.xlsx> [-o <xlsx-or-dir>] --api-key <api_key>`；省略 `-o` 时默认输出 `_verify_result_reverify_yyyyMMddHHmmss.xlsx`，显式传目录时在该目录补默认文件名，显式传 `.xlsx` 时写入该文件。
6. **重新查验产物校验（条件执行）**：确认 reverify 返回的 `intermediate_excel_path` 已存在、文件非空、可正常读取；后续 `analysis` 必须使用刷新后的中间 XLSX，不再使用旧中间表。
7. **分析阶段**：执行 `{EXECUTABLE} analysis -i <xlsx> -c <company> [-o <xlsx-or-dir>]`。该步骤只输出 JSON 统计数据和增强 XLSX，不写 PDF。
8. **PDF 后处理阶段**：AI 根据 analysis 输出 JSON 生成最终统计文本，`【输出文件】` 中必须同时包含最终 XLSX 路径和 `PDF 分析报告：{PDF_OUTPUT_FILE}` 占位行；将完整文本写入 UTF-8 `.txt` 文件，再执行 `{EXECUTABLE} report-pdf --input-text-file <utf8-txt> --output <pdf>`。
9. **展示结果**：PDF 后处理完成后，最终用户可见文本必须使用转换器返回的 `final_report_text`，逐字原样输出；AI 不得在 PDF 转换后重新生成、改写或重新排版统计文本。

## 命令调用说明

以下只保留本场景特有约束；通用随包可执行文件规则见文首。

**核验阶段：verify**
- 命令：`{EXECUTABLE} verify -i <输入路径> -o <中间XLSX完整路径> --api-key <API Key>`
- 关键参数：
  - `-i/--input`：文件夹路径，或标准化后的 `.xlsx` / `.csv` 清单路径。
  - `-o/--output`：核验 XLSX 完整输出路径；该路径由 AI 内部生成并传给后续 `analysis -i/--input`，不向用户展示或询问，但文件必须保留。
  - `--api-key`：发票查验 API Key，仅在运行时内存中使用，不写入磁盘、配置、日志、缓存或输出文件。
- 约束：
  - 本场景不传公司名称。
  - 公司名称仅在分析阶段使用。
  - 输出基础票面 XLSX，作为本次 `analysis` 与后续重新查验的输入，流程结束后也必须保留。
  - 支持输入：文件夹模式支持直接扫描图片发票文件、PDF 和 OFD；清单模式仅支持 `.xlsx` / `.csv`。

**重新查验阶段：reverify**
- 命令：`{EXECUTABLE} reverify -i <intermediate.xlsx> [-o <xlsx-or-dir>] --api-key <api_key>`
- 公开命令模板：`invoice-verify-analysis reverify -i <intermediate.xlsx> [-o <xlsx-or-dir>] --api-key <api_key>`
- 关键参数：
  - `-i/--input`：本次 `verify` 刚生成的核验 XLSX。
  - `-o/--output`：刷新后的核验 XLSX 输出路径；省略时在输入中间表同目录生成 `_verify_result_reverify_yyyyMMddHHmmss.xlsx`，传目录时补默认文件名，传 `.xlsx` 时按该文件写出。
  - `--api-key`：发票查验 API Key，仅在运行时内存中使用，不写入磁盘、配置、日志、缓存或输出文件。
- 约束：
  - reverify 只重查“税局查验状态”为空或不等于“查验成功”的记录；已经查验成功的记录原样保留。
  - reverify 使用当前第一阶段生成的中间表；命令会返回刷新后的中间表和统计信息，agent 按返回结果继续后续流程。
  - reverify 成功后，将返回 JSON 中的 `intermediate_excel_path` 视为当前第一阶段输出，并继续执行 `analysis`。

**分析阶段：analysis**
- 命令：`{EXECUTABLE} analysis -i <核验XLSX路径> -c <公司名称> [-o <分析XLSX完整路径>]`
- 关键参数：
  - `-i/--input`：本次 `verify` 刚生成或 reverify 刷新后的核验 XLSX。
  - `-c/--company`：公司名称，必填。
  - `-o/--output`：分析结果完整输出路径，可选；用户只给目录或输出位置时，由 AI 补默认文件名 `企业发票查验结果清单yyyyMMddHHmmss.xlsx`。
- 约束：
  - 只能读取本次 `verify` 刚生成的核验 XLSX。
  - 用户若明确传入 `.xlsx` 文件名，则使用该文件名；否则使用默认文件名。
  - 会补充“是否重复风险”列、移除“商品明细”列，并输出 JSON 统计结果。
  - “格式不支持”等不会进入最终清单的记录不参与 `valid_invoice_count`、`verify_success`、`verify_failed` 统计。
  - JSON 结构：`{input_file, output_file, company_name, record_count, analysis: {verify_stats, status_distribution, overview, time_stats, product_stats, counterparty_stats, risk_alerts}}`。
  - `analysis` 保持 JSON/XLSX 输出职责，不输出 PDF，也不提供 PDF 参数。

**PDF 后处理阶段：report-pdf**
- 命令：`{EXECUTABLE} report-pdf --input-text-file <utf8-txt> --output <pdf>`
- 输入文本：AI 已按 JSON 和字段映射填好的最终统计文本，必须是 UTF-8 `.txt` 临时文件内容。
- PDF 路径：默认使用最终 XLSX `output_file` 的父目录，加文件名 `分析报告yyyyMMddHHmmss.pdf`；若用户明确指定 PDF 路径或文件名，则使用用户指定路径。
- 占位符：最终统计文本中的 `{PDF_OUTPUT_FILE}` 必须保留到传入 `report-pdf` 前，由转换器替换为实际 PDF 路径。
- 输出 JSON：转换器返回 JSON，至少包含 `pdf_output_file` 和 `final_report_text`。
- 展示规则：最终用户可见文本必须使用转换器返回的 `final_report_text`，逐字原样输出；AI 不得在 PDF 转换后重新生成、改写或重新排版。

## 统计分析维度说明

`analysis` 命令输出统计分析 JSON，AI 必须按以下七个维度生成最终报告，不得压缩成简短摘要。

### 1. 核验统计（analysis.verify_stats）
- 总记录数、有效发票文件数、查验成功数、查验失败数；这些核验统计均按最终清单实际保留记录计算，不包含已过滤的格式不支持记录。
- 有效发票文件数（`valid_invoice_count`）：最终清单中 `发票号码` 非空的记录数。
- 查验失败明细列表（`verify_failed_details`）：每项含文件名（`file_name`）和失败原因（`reason`）。
- 查验失败原因汇总（`verify_failed_reason_summary`）：每项含失败原因（`reason`）和数量（`count`）。

### 2. 发票状态分布（analysis.status_distribution）
- 正常发票数量（`normal_count`）。
- 红冲发票数量（`revoked_count`）：红冲通常指已被冲销或用于冲抵的发票。
- 作废发票数量（`cancelled_count`）：作废通常指已失效、不能再按正常发票使用的发票。
- 重要：红冲发票参与后续统计维度的计算；只有作废发票不参与后续统计，仅在此处计数。

### 3. 总体统计（analysis.overview）
- 参与统计的发票张数，仅已确定进项/销项方向的非作废发票，包含红冲。
- 总金额（不含税）、总税额、价税合计，每项均拆分进项/销项，进项 + 销项 = 总金额。
- 进项数量、销项数量。进项通常指公司采购、取得的发票；销项通常指公司销售、开出的发票。
- 未知方向数量及其金额（不含税、税额、价税合计）：未知方向指发票抬头与提供的公司名称不匹配，暂时无法判断属于进项还是销项；这部分单独列出，不计入总金额。
- 发票类型分布：每种类型的数量和不含税金额，仅含已确定方向的非作废发票，包含红冲。
- 所有金额字段均由命令直接格式化为带千分位的数字字符串，保留 2 位小数；单位由标题、表头或说明文本提供。

### 4. 时间统计（analysis.time_stats）
- 月度发票数量变化：进项张数、销项张数、总张数、环比变化（总张数）。
- 月度金额变化：进项金额、销项金额、总金额、环比变化（总金额），金额字段为命令直接返回的带千分位数字字符串，使用不含税金额。

### 5. 商品明细统计（analysis.product_stats）
- 商品总数量。
- 进项商品 Top5（按数量占比）。
- 销项商品 Top5（按数量占比）。
- 数量来源：商品明细 JSON 中 `MxzbList` 数组对象的 `Spsl` 字段。

### 6. 客户/供应商统计（analysis.counterparty_stats）
- 前五大客户（销项，按不含税金额），每条含：排名、名称、金额（元）、发票张数、占总额比例，附合计行。
- 前五大供应商（进项，按不含税金额），每条含：排名、名称、金额（元）、发票张数、占总额比例，附合计行。
- 这里的“发票张数”是发票数量，展示单位必须为“张”，不得写成“个”。
- 集中度分析：前 5 大占比、前 10 大占比、集中度评价（极低/低/中/高/极高）。集中度用于说明交易金额是否集中在少数客户或供应商手中。
  - 评价阈值：<10% 极低、10-30% 低、30-50% 中、50-70% 高、≥70% 极高。
- 所有金额和占比均基于不含税金额计算。

### 7. 风险提示（analysis.risk_alerts）
- 重复风险发票数量：提示同一批次中可能重复出现的发票。
- 状态异常发票数量（红冲/作废）：提示已红冲或已作废的发票。
- 未知方向发票数量：提示暂时无法判断进项或销项的发票。
- 具体风险描述列表。

## 输出模板

注意：`analysis` 只输出 JSON 统计数据，不输出自然语言文本。AI 需根据 JSON 输出，按以下模板格式化为自然语言展示给用户。表格中的 `|` 应尽量对齐，使输出整齐美观。输出模板中的所有表格表头必须逐字原样保留，禁止漏字、加字、换词、调换顺序或改成近义词；尤其是表头 `数值` 必须保持为 `数值`，不得改成 `数量` 或其它词。所有金额字段均由命令直接返回为带千分位的数字字符串，保留 2 位小数，不带单位；单位由标题、表头或说明文本提供。所有“发票张数”列的单位必须是“张”，单元格必须展示为 `XX 张`，禁止写成 `XX 个`。AI 只做字段填充，不做二次换算、格式转换或表头改写。面向用户展示时，术语说明要简短清楚：进项通常指公司采购、取得的发票；销项通常指公司销售、开出的发票；红冲通常指已冲销或冲抵的发票；作废通常指已失效的发票；未知方向指暂时无法判断进项或销项；集中度用于说明交易金额是否集中在少数客户或供应商手中。

```
# 企业发票核验分析报告

企业名称：{company_name}

## 处理统计
- 待处理文件总数：XX 个
- 有效发票文件：XX 个
- 查验成功：XX 张
- 查验失败：XX 张
- 查验失败原因统计：
  - {reason_1}：{count_1}个
  - {reason_2}：{count_2}个

## 输出文件
- 发票查验清单：{output_file}
- PDF 分析报告：{PDF_OUTPUT_FILE}

## 发票统计分析

### 发票状态分布
- 正常：XX 张
- 红冲：XX 张
- 作废：XX 张
（红冲通常指已冲销或冲抵的发票，会参与以下统计；作废通常指已失效的发票，只有作废发票不参与以下统计）

### 核验统计
| 核验发票总数 | 查验成功 | 查验失败 |
|--------------|----------|----------|
| XX 张        | XX 张    | XX 张    |

### 总体统计（仅已确定进项或销项方向的非作废发票，包含红冲）
| 统计项目             | 数值             | 备注                                      |
|----------------------|------------------|-------------------------------------------|
| 发票总张数           | XX 张            | 进项 XX 张 + 销项 XX 张                   |
| 总金额（不含税，元） | X,XXX,XXX.XX 元  | 进项 XXX.XX 元 + 销项 X,XXX,XXX.XX 元     |
| 总税额（元）         | XXX,XXX.XX 元    | 进项 XXX.XX 元 + 销项 XXX,XXX.XX 元       |
| 价税合计（元）       | X,XXX,XXX.XX 元  | 进项 X,XXX.XX 元 + 销项 X,XXX,XXX.XX 元   |

- 未知方向（发票抬头与提供的公司名称不符，暂时无法判断进项或销项方向）：XX 张（金额单位：元；不含税 X,XXX.XX，税额 XXX.XX，价税合计 X,XXX.XX）
- 发票类型分布：
  - 类型A：XX 张，不含税金额（元）X,XXX.XX
  - 类型B：XX 张，不含税金额（元）X,XXX,XXX.XX

### 时间统计：数量
| 月份    | 进项张数 | 销项张数 | 总张数 | 环比变化 |
|---------|----------|----------|--------|----------|
| YYYY-MM | XX 张    | XX 张    | XX 张  | XX.X%    |

### 时间统计：金额（元，不含税）
| 月份    | 进项金额（元） | 销项金额（元） | 总金额（元） | 环比变化 |
|---------|----------------|----------------|--------------|----------|
| YYYY-MM | XXX.XX         | X,XXX,XXX.XX   | X,XXX,XXX.XX | XX.X%    |

### 商品明细统计
#### 前五销售商品明细及占比

| 项目          | 商品名称 | 占比   |
|---------------|----------|--------|
| Top1 商品明细 | 商品名   | XX.XX% |
| Top2 商品明细 | 商品名   | XX.XX% |
| Top3 商品明细 | 商品名   | XX.XX% |
| Top4 商品明细 | 商品名   | XX.XX% |
| Top5 商品明细 | 商品名   | XX.XX% |

#### 前五采购商品明细及占比

| 项目          | 商品名称 | 占比   |
|---------------|----------|--------|
| Top1 商品明细 | 商品名   | XX.XX% |
| Top2 商品明细 | 商品名   | XX.XX% |
| Top3 商品明细 | 商品名   | XX.XX% |
| Top4 商品明细 | 商品名   | XX.XX% |
| Top5 商品明细 | 商品名   | XX.XX% |

### 客户/供应商统计
#### 前五大客户（销项）

| 排名 | 客户名称 | 销售金额（元） | 发票张数 | 占总销售额比例 |
|------|----------|----------------|----------|----------------|
| 1    | 客户名称 | X,XXX,XXX.XX   | XX 张    | XX.XX%         |
| 2    | 客户名称 | XX,XXX.XX      | XX 张    | XX.XX%         |
| 3    | 客户名称 | X,XXX.XX       | XX 张    | XX.XX%         |
| 4    | 客户名称 | X,XXX.XX       | XX 张    | XX.XX%         |
| 5    | 客户名称 | XXX.XX         | XX 张    | XX.XX%         |
| 合计 | -        | X,XXX,XXX.XX   | XX 张    | XX.XX%         |

#### 前五大供应商（进项）

| 排名 | 供应商名称 | 采购金额（元） | 发票张数 | 占总采购额比例 |
|------|------------|----------------|----------|----------------|
| 1    | 供应商名称 | X,XXX.XX       | XX 张    | XX.XX%         |
| 2    | 供应商名称 | XXX.XX         | XX 张    | XX.XX%         |
| 3    | 供应商名称 | XXX.XX         | XX 张    | XX.XX%         |
| 4    | 供应商名称 | XXX.XX         | XX 张    | XX.XX%         |
| 5    | 供应商名称 | XXX.XX         | XX 张    | XX.XX%         |
| 合计 | —          | X,XXX.XX       | XX 张    | XX.XX%         |

#### 客户与供应商集中度

| 统计维度     | 前 5 大占比 | 前 10 大占比 | 集中度评价        |
|--------------|-------------|--------------|-------------------|
| 客户集中度   | XX.XX%      | XX.XX%       | 极低/低/中/高/极高 |
| 供应商集中度 | XX.XX%      | XX.XX%       | 极低/低/中/高/极高 |

### 风险提示
- 发现 XX 张重复风险发票
- 发现 XX 张状态异常发票（红冲/作废）
- 发现 XX 张无法判断进项/销项方向的发票
```

### 字段映射说明（JSON -> 输出）

- 处理统计
  - 待处理文件总数 = `record_count`
  - 有效发票文件 = `analysis.verify_stats.valid_invoice_count`
  - 查验成功 = `analysis.verify_stats.verify_success`
  - 查验失败 = `analysis.verify_stats.verify_failed`
  - 查验失败原因统计 = `analysis.verify_stats.verify_failed_reason_summary`，按 `- {reason}：{count}个` 输出；为空时输出“无”。
- 所有金额字段：直接使用命令返回的格式化字符串；禁止再做万元换算、二次四舍五入或追加千分位；单位由标题、表头或说明文本提供。
- 输出文件
  - 发票查验清单 = `output_file`
  - PDF 分析报告 = `{PDF_OUTPUT_FILE}` 占位符，由 `report-pdf` 转换器替换为实际 PDF 路径。
- 发票状态分布
  - 正常 = `analysis.status_distribution.normal_count`
  - 红冲 = `analysis.status_distribution.revoked_count`
  - 作废 = `analysis.status_distribution.cancelled_count`
- 总体统计
  - 发票总张数 = `analysis.overview.invoice_count`
  - 进项/销项张数 = `analysis.overview.input_count` / `analysis.overview.output_count`
  - 总金额（不含税） = `analysis.overview.total_net_amount`
  - 总税额 = `analysis.overview.total_tax_amount`
  - 价税合计 = `analysis.overview.total_amount`
  - 进项金额拆分 = `analysis.overview.input_net_amount` / `analysis.overview.input_tax_amount` / `analysis.overview.input_amount`
  - 销项金额拆分 = `analysis.overview.output_net_amount` / `analysis.overview.output_tax_amount` / `analysis.overview.output_amount`
  - 未知方向 = `analysis.overview.unknown_count` / `analysis.overview.unknown_net_amount` / `analysis.overview.unknown_tax_amount` / `analysis.overview.unknown_amount`
  - 发票类型分布 = `analysis.overview.type_distribution`
- 时间统计
  - 数量表 = `analysis.time_stats.monthly_count`
  - 金额表 = `analysis.time_stats.monthly_amount`
- 商品明细统计
  - 商品总数量 = `analysis.product_stats.total_quantity`
  - 前五采购商品 = `analysis.product_stats.top5_purchase`
  - 前五销售商品 = `analysis.product_stats.top5_sales`
- 客户/供应商统计
  - 前五大客户 = `analysis.counterparty_stats.top10_customers` 前 5 条；`top10_customers` 为 JSON 来源数组，明细表仅展示前 5 条。
  - 客户合计 = `analysis.counterparty_stats.top10_customer_total`
  - 前五大供应商 = `analysis.counterparty_stats.top10_suppliers` 前 5 条；`top10_suppliers` 为 JSON 来源数组，明细表仅展示前 5 条。
  - 供应商合计 = `analysis.counterparty_stats.top10_supplier_total`
  - “发票张数”列必须使用“张”作单位，格式为 `N 张`；不得写成 `N 个`。
  - 客户集中度 = `analysis.counterparty_stats.customer_concentration_top5` / `analysis.counterparty_stats.customer_concentration_top10` / `analysis.counterparty_stats.customer_evaluation`
  - 供应商集中度 = `analysis.counterparty_stats.supplier_concentration_top5` / `analysis.counterparty_stats.supplier_concentration_top10` / `analysis.counterparty_stats.supplier_evaluation`
  - 明细表标题必须使用“前五大客户（销项）”和“前五大供应商（进项）”；不得将明细表标为“前十大客户”或“前十大供应商”。仅集中度表可保留“前 10 大占比”。
- 风险提示
  - 重复风险发票 = `analysis.risk_alerts.duplicate_count`
  - 状态异常发票 = `analysis.risk_alerts.abnormal_status_count`
  - 未知方向发票 = `analysis.risk_alerts.unknown_direction_count`
  - 风险描述列表 = `analysis.risk_alerts.details`

### PDF 后处理说明

1. AI 先根据 `analysis` 返回的 JSON 和上方字段映射，格式化完整最终统计文本；`## 输出文件` 段落中必须包含 `发票查验清单：{output_file}` 和 `PDF 分析报告：{PDF_OUTPUT_FILE}`。`analysis` 本身仍只输出 JSON/XLSX，不直接输出 PDF。
2. AI 将这份已经完成字段填充的 Markdown 文本原样写入临时 UTF-8 `.txt` 文件，不得把 `final_report_text` 当作 `analysis` 的 JSON 字段读取。
3. PDF 输出路径默认取最终 XLSX `output_file` 的父目录，并使用 `分析报告yyyyMMddHHmmss.pdf`。
4. 若用户明确指定 PDF 路径或文件名，PDF 后处理命令的 `--output` 必须使用用户指定路径；该 PDF 输出路径的父目录必须已存在，转换器不会自动创建父目录。
5. 运行命令：`{EXECUTABLE} report-pdf --input-text-file <utf8-txt> --output <pdf>`。
6. `--input-text-file` 必须是有效 UTF-8 `.txt` 文件；当前 CLI 只接受文件输入，不通过 stdin 直接传入正文。
7. 转换器仅保证块级 Markdown 基础版式：标题、列表、表格、分隔线、代码块会转换为对应版式；行内强调、行内代码、链接、图片仅保证纯文本降级，不保证保留原始格式或渲染为图片/可点击链接。最终回复仍以转换器返回的 `final_report_text` 为准。
8. 转换器返回 JSON，包含 `pdf_output_file` 和 `final_report_text`；`pdf_output_file` 为请求的 PDF 路径，`final_report_text` 已包含该 PDF 路径。
9. `final_report_text` 仍然是原始 Markdown 文本，仅将 `{PDF_OUTPUT_FILE}` 替换为实际 PDF 路径；PDF 渲染过程不会对这段 stdout 文本做改写或二次排版。
10. 最终回复用户时，必须将转换器返回的 `final_report_text` 原样输出；AI 不得在 PDF 转换后重新生成、改写、删减或重新排版。
