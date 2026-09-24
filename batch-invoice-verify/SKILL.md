---
name: "batch-invoice-verify"
display_name: "柠檬云发票查验查重及台账"
display_name_en: "batch-invoice-verify"
description: "批量识别并查验文件夹内发票，校验抬头、提示重复风险，生成最终清单"
description_zh: "批量识别并查验文件夹内发票，校验抬头、提示重复风险，生成最终清单。"
description_en: "Batch-identifies and verifies invoices, checks invoice headers and duplicate risks, and generates a ledger."
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

若本 skill 是通过 ZIP 发布包直接解压安装，或当前会话首次加载本地 `SKILL.md` 且尚未确认已按安装文档完成 `README.md` 全文输出，必须先阅读同级 `README.md`，并立即将 `README.md` 全文完整原样输出给用户；不得修改、概括、精简、重排、删减或只说明其中要求；不得等到用户开始业务执行、收集发票或生成命令时才输出。

首次执行本 skill 前，仍必须确认同级 `README.md` 已阅读且全文已完整原样输出给用户。在收集发票或生成任何业务命令前，若用户尚未满足 `README.md` 中列出的前置要求，应先完整原样输出 `README.md` 全文，再提示用户按 `README.md` 完成准备后继续。

当任一业务命令、接口响应、命令输出或错误信息中出现“无权调用该能力”时，必须立即停止当前业务流程，按同级 `README.md` 中的 API Key 获取方式告知用户为当前 API Key 绑定对应能力权限及企业（如 `README.md` 要求绑定企业）后再重试；不得将该响应解释为税局登录失败，不得引导用户去登录或重新登录，也不得继续执行后续业务命令。

## 随包可执行文件规范（强制）

- 安装完成后，直接运行本地 `bin/<platform>/` 下的当前平台可执行文件；Agent 生成或运行命令前必须先把 `{EXECUTABLE}` 解析为当前 OS/CPU 对应路径。
- 平台映射如下：Windows x64 使用 `bin/windows-amd64/batch-invoice-verify.exe`；Linux x64 使用 `bin/linux-amd64/batch-invoice-verify`；Linux ARM64 使用 `bin/linux-arm64/batch-invoice-verify`；macOS x64 使用 `bin/darwin-amd64/batch-invoice-verify`；macOS ARM64 使用 `bin/darwin-arm64/batch-invoice-verify`。
- 只支持 Windows x64、Linux x64、Linux ARM64、macOS x64、macOS ARM64。若当前平台不在支持列表内，必须停止执行并报告支持的平台；不得猜测路径，不得回退使用 Windows 或其它平台二进制。
- 后续命令示例中的 `{EXECUTABLE}` 均为占位符，必须先替换为上方平台路径后再执行。
- 不要求客户安装任何额外语言运行时、包管理器、编译器或外部程序。
- 发布包包含 `config/verify-api.json`，该文件只用于发票查验接口端点和能力配置。
- 运行时必须保留发布包目录结构，不能只把平台二进制单独拷走运行。
- `config/verify-api.json` 只用于发票查验接口端点和能力配置；不得把 API Key、邮箱授权码或其它凭据写入该文件。
- 客户运行命令必须先按平台映射替换 `{EXECUTABLE}`；安装、更新、ZIP 包和当前平台二进制获取方式以远端安装文档为准。
- 若本地 `bin/<platform>/` 下缺少当前平台二进制、二进制不可执行，或用户询问二进制下载路径，必须读取远端安装文档中的“当前平台二进制下载表”，按当前 OS/CPU 只下载一个匹配平台的外置二进制，并保存为上方平台映射声明的本地路径；不得在 `SKILL.md` 中猜测、拼接或硬编码下载地址。

### PowerShell 下载约束（强制）

在安装或更新流程中，若使用 PowerShell 下载任何资源（包括 ZIP 包和外置二进制文件），必须且只能使用以下规则：

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

- 对用户只收集输出目录或输出位置；除非用户明确指定 `.xlsx` 文件名，否则不得要求用户补文件名。
- 生成命令时，`-o/--output` 必须传入带文件名的完整 `.xlsx` 路径：第一阶段默认补 `发票查验_中间结果（系统生成，请勿使用）.xlsx`，第二阶段默认补 `发票查验结果清单yyyyMMddHHmmss.xlsx`。
- 若用户明确指定 `.xlsx` 文件名，则第二阶段最终清单使用该文件名；第一阶段中间文件仍默认使用同目录下 `发票查验_中间结果（系统生成，请勿使用）.xlsx`，除非用户明确指定中间文件名。
- `reverify -o/--output` 可省略；省略时在输入中间 XLSX 同目录生成 `_verify_result_reverify_yyyyMMddHHmmss.xlsx`，传目录时补同名默认文件，传 `.xlsx` 时按该文件写出；禁止把输出路径设为输入中间表本身。
- 第二阶段最终清单会保留税局查验结果字段，包含 `税局查验状态` 和 `查验时间`。

### 客户沟通口径（强制）

- 面向用户沟通时，始终以“专业、亲切的财务助理”身份表达，使用“您”称呼用户，语气清楚、稳重、不过度营销。
- 说明参数、文件、路径、查验状态、报销状态、入账状态、统计口径等技术内容时，必须使用通俗语言解释，避免只堆叠字段名或命令术语。
- 对输出路径、公司名称、统计数字、JSON key、占位符、命令返回值、文件名和用户已提供的原始值，必须原样保留，不得改写、翻译、补全或美化。
- 邮箱授权码、密码类字段和类似敏感信息必须按敏感信息处理，不得在回复中回显完整内容；如需确认，只能做脱敏提示。
- 最终回复必须一次性包含完整汇总，不能只说“完成”。
- 遇到问题请点击 {{OPEN_API_CONTACT_SERVICE_URL}} 扫码添加我们的专属客服企业微信联系我们

### 发票查验 API Key（强制）

- 发票查验请求需要用户或调用方提供 API Key。
- 用户已明确提出发票查验/批量核验需求后，可将 API Key 与发票文件夹、输出位置等其它必需参数一起收集；生成第一阶段命令时通过 `--api-key <API Key>` 传给命令。
- API Key 仅在运行时内存中使用，不写入磁盘、配置、日志、缓存或输出文件。
- 随包配置不是凭据配置位置，不得要求用户把 API Key 写入任何配置文件。

# 场景：batch-invoice-verify（批量发票查验与防重复报销）

## 触发关键词

批量发票查验,文件夹发票处理,防重复报销,清单追加,批量验票,批量发票核验

## 两阶段执行概览

本场景固定分两阶段执行，两阶段均为必须步骤，不可跳过：

| 阶段 | 命令 | 功能 | 输出 |
| ---- | ---- | ---- | ---- |
| 第一阶段 | `{EXECUTABLE} verify` | 发票识别 + 发票查验 | 中间 XLSX（作为第二阶段和后续重新查验输入，默认不对用户展示但必须保留） |
| 第二阶段 | `{EXECUTABLE} manage` | 抬头校验 + 重复风险 + 报销/入账状态管理 + 字段标准化 | 最终交付 XLSX + JSON 统计 |

若用户要求重新查验第一阶段中的查验失败或空结果，可在第一阶段和第二阶段之间执行 `{EXECUTABLE} reverify`，并把 reverify 返回的 `intermediate_excel_path` 作为当前第一阶段输出继续传给后续 `manage`。

第一阶段只做发票解析和调用发票查验接口；有发票下载能力的其它技能才会在第一阶段下载发票。batch-invoice-verify 一阶段不做查重、不做抬头校验、不做报销分析；这些规则只在后续 `manage` 阶段执行。

### 强制执行规则

1. 必须严格按“参数收集 -> 历史清单预处理（如需要） -> verify -> 中间 XLSX 校验 -> 可选 reverify -> manage -> 展示结果”的顺序执行，禁止跳步。
2. 生成第一阶段命令时，必须已收集发票查验 API Key，并通过 `--api-key <API Key>` 传给命令；API Key 只在运行时内存中使用，不写入磁盘、配置、日志、缓存或输出文件。
3. 禁止跳过第一阶段直接调用 manage；禁止假设中间 XLSX、标准 CSV 历史清单已经存在；禁止复用旧批次文件冒充本次执行产物。
4. 若用户启用了报销状态管理或入账状态管理，但未提供 `history_list_path`，必须中止，禁止继续执行第二阶段。
5. 第二阶段只允许读取“本次第一阶段刚生成或 reverify 刷新后的中间 XLSX”和“本次已校验通过的标准 CSV 历史清单”。
6. reverify 只允许读取当前第一阶段中间 XLSX；命令会返回刷新后的中间表和统计信息，agent 按返回结果继续后续流程，不自行改写中间表内容。
7. 第一阶段中间 XLSX 与 reverify 刷新后的中间 XLSX 是后续重新查验的输入，流程结束后也必须保留；除非用户明确要求清理，不得删除、移动或覆盖。
8. 每个关键阶段完成后都必须先给出最小确认信息，再进入下一阶段：历史清单预处理后确认标准 CSV 已就绪；第一阶段后确认中间 XLSX 已生成；reverify 后确认刷新后的中间 XLSX 已生成；第二阶段后确认最终 XLSX 和 JSON 统计已生成。

## 参数规格

| 参数名 | 用户听得懂的名称 | 状态 | 说明 |
| ------ | ---------------- | ---- | ---- |
| `api_key` | 发票查验 API Key | 必填 | 用于发票查验请求；生成第一阶段命令时通过 `--api-key <API Key>` 传给命令。 |
| `invoice_folder` / `invoice_list_path` | 发票文件夹或发票清单路径 | 必填 | `verify -i` 支持发票文件夹，也支持标准 `.xlsx` / `.csv` 清单；文件夹里可以放 PDF、OFD 或图片发票。清单模式至少需要 `发票号码`、`开票日期`、`价税合计`，可选 `发票代码`、`校验码`。 |
| `intermediate_excel_path` | 第一阶段中间查验表 | 重新查验时必填 | 用户要求重查失败或空结果时，传入本次 `verify` 生成的中间 XLSX。 |
| `output_xlsx_path` | 输出目录或结果清单路径 | 可选 | 用户只需要告诉结果放在哪里；若明确提供 `.xlsx` 文件名，则作为第二阶段最终清单文件名。 |
| `company_name` | 公司名称（发票抬头） | 抬头校验必做；参数可不提供 | 用户未提供时先不传 `-c`，由第二阶段自行解析；多个抬头时返回 failed 和 `company_names`。 |
| `check_duplicate` | 重复风险提醒 | 默认开启 | 脚本默认执行重复风险提醒；用户不需要额外确认。 |
| `check_reimburse` | 报销状态核对 | 可选 | 启用后必须提供 `history_list_path`。 |
| `check_account` | 入账状态核对 | 可选 | 启用后必须提供 `history_list_path`。 |
| `history_list_path` | 历史发票清单 | 条件必填 | 用于报销状态或入账状态核对的标准 CSV 历史清单；只做重复风险历史比对时至少包含 `发票号码`，启用报销核对时还必须包含 `发票代码`、`报销状态`，启用入账核对时还必须包含 `发票代码`、`入账状态`。 |

### 多抬头 failed 处理规则

当未传 `company_name` 且第二阶段返回 failed JSON：`{status, message, company_names}` 时，agent 只能原样展示返回的 `company_names`，禁止自行猜测、补全、筛选、改写、归并或新增任何抬头；必须等待用户确认后仅重跑第二阶段。

## 执行流程

1. **收集参数与发票文件**：收集必要参数，帮助用户将发票文件收集到同一文件夹；确认或推断输出目录。
2. **历史清单预处理**（条件执行）：若用户提供了非标准格式的历史清单，必须先转换成脚本可直接读取的标准 CSV；本命令不做自动字段映射转换。若转换失败、字段映射不明确或数据验证不通过，则必须中止。
3. **第一阶段：verify**：执行 `{EXECUTABLE} verify -i <folder-or-xlsx-or-csv> [-o <xlsx-or-dir>] --api-key <api_key>`。
4. **第一阶段产物校验（必须执行）**：确认本次刚生成并在 JSON `intermediate_excel_path` 返回的中间 XLSX 已存在、文件非空、可正常读取，且能作为第二阶段输入。
5. **可选重新查验：reverify**：当用户要求重查第一阶段查验失败或空结果时，执行 `{EXECUTABLE} reverify -i <intermediate.xlsx> [-o <xlsx-or-dir>] --api-key <api_key>`；省略 `-o` 时默认输出 `_verify_result_reverify_yyyyMMddHHmmss.xlsx`，显式传目录时在该目录补默认文件名，显式传 `.xlsx` 时写入该文件。
6. **重新查验产物校验（条件执行）**：确认 reverify 返回的 `intermediate_excel_path` 已存在、文件非空、可正常读取；后续 `manage` 必须使用刷新后的中间 XLSX，不再使用旧中间表。
7. **第二阶段：manage（必须执行）**：执行 `{EXECUTABLE} manage -i <xlsx> [-o <xlsx-or-dir>] [--check-duplicate] [--history <csv>] [--check-reimburse] [--check-account] [-c <company>]`；重复风险提醒默认开启，`--check-duplicate` 仅为兼容旧命令保留，不需要额外传入。
8. **展示结果**：根据 JSON 结果按下方“输出模板”输出统计信息，最终交付文件以第二阶段处理后的 XLSX 为准。

### reverify 使用约束

- 命令：`{EXECUTABLE} reverify -i <intermediate.xlsx> [-o <xlsx-or-dir>] --api-key <api_key>`。
- 公开命令模板：`batch-invoice-verify reverify -i <intermediate.xlsx> [-o <xlsx-or-dir>] --api-key <api_key>`。
- reverify 只重查中间表中“税局查验状态”为空或不等于“查验成功”的记录；已经查验成功的记录原样保留。
- reverify 使用当前第一阶段生成的中间表；命令会返回刷新后的中间表和统计信息，agent 按返回结果继续后续流程。
- reverify 成功后，将返回 JSON 中的 `intermediate_excel_path` 视为当前第一阶段输出，并继续执行 `manage`。
- `--api-key` 只在运行时传入，文档、日志、配置和输出文件都不得写入真实 API Key。

## 输出模板

```
企业名称：XXXXXX有限公司

【处理统计】
- 待处理文件总数：XX个
- 有效发票文件：XX个
- 查验成功：XX张
- 查验失败：XX张
- 重复风险发票：XX张
- 新增清单记录：XX张

【输出文件】
- 发票处理清单：{excel_file_path}（不输出发票明细）

【异常提示】
- XX个非发票文件已过滤
- XX个发票查验接口超时，建议稍后重试
```

### 字段映射说明（JSON -> 输出）

- 待处理文件总数 = `total_file_count`
- 有效发票文件 = `valid_invoice_count`
- 查验成功 = `verify_success_count`
- 查验失败 = `verify_failed_count`
- 重复风险发票 = `duplicate_risk_count`
- 新增清单记录 = `new_list_count`
- 异常提示 = 根据 `error_alerts` 分类展示（非查验成功的状态）；`error_alerts` 是“状态 -> 文件名列表”，展示时可汇总数量并列出或概括对应文件名。
- 企业名称 = `company_name`
