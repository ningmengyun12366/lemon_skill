---
name: "invoice-reimbursement-assistant"
display_name: "柠檬云发票整理、查验及报销助手"
display_name_en: "invoice-reimbursement-assistant"
description: "从邮箱提取发票附件，完成识别查验、费用分类和抬头校验，生成报销清单"
description_zh: "从邮箱提取发票附件，完成识别查验、费用分类和抬头校验，生成报销清单。"
description_en: "Extracts invoice attachments from email, verifies and categorizes them, checks invoice headers, and generates a reimbursement ledger."
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

若本 skill 是通过 ZIP 发布包直接解压安装，或当前会话首次加载本地 `SKILL.md` 且尚未确认已按安装文档完成 `README.md` 全文输出，必须先阅读同级 `README.md`，并立即将 `README.md` 全文完整原样输出给用户；不得修改、概括、精简、重排、删减或只说明其中要求；不得等到用户开始业务执行、收集邮箱、发票或生成命令时才输出。

首次执行本 skill 前，仍必须确认同级 `README.md` 已阅读且全文已完整原样输出给用户。在收集邮箱、发票或生成任何业务命令前，若用户尚未满足 `README.md` 中列出的前置要求，应先完整原样输出 `README.md` 全文，再提示用户按 `README.md` 完成准备后继续。

当任一业务命令、接口响应、命令输出或错误信息中出现“无权调用该能力”时，必须立即停止当前业务流程，按同级 `README.md` 中的 API Key 获取方式告知用户为当前 API Key 绑定对应能力权限及企业（如 `README.md` 要求绑定企业）后再重试；不得将该响应解释为税局登录失败，不得引导用户去登录或重新登录，也不得继续执行后续业务命令。

## 随包可执行文件规范（强制）

- 安装完成后，直接运行本地 `bin/<platform>/` 下的当前平台可执行文件；Agent 生成或运行命令前必须先把 `{EXECUTABLE}` 解析为当前 OS/CPU 对应路径。
- 平台映射如下：Windows x64 使用 `bin/windows-amd64/invoice-reimbursement-assistant.exe`；Linux x64 使用 `bin/linux-amd64/invoice-reimbursement-assistant`；Linux ARM64 使用 `bin/linux-arm64/invoice-reimbursement-assistant`；macOS x64 使用 `bin/darwin-amd64/invoice-reimbursement-assistant`；macOS ARM64 使用 `bin/darwin-arm64/invoice-reimbursement-assistant`。
- 只支持 Windows x64、Linux x64、Linux ARM64、macOS x64、macOS ARM64。若当前平台不在支持列表内，必须停止执行并报告支持的平台；不得猜测路径，不得回退使用 Windows 或其它平台二进制。
- 后续命令示例中的 `{EXECUTABLE}` 均为占位符，必须先替换为上方平台路径后再执行。
- 不要求客户安装任何额外语言运行时、包管理器、编译器或外部程序。
- 发布包包含 `config/verify-api.json`，该文件只用于发票查验接口端点和能力配置。
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
- 生成命令时，所有输出参数必须传入带文件名的完整 `.xlsx` 路径：Phase1 中间查验结果默认 `发票查验_中间结果（系统生成，请勿使用）.xlsx`，Phase2 最终报销清单默认 `发票报销清单yyyyMMddHHmmss.xlsx`。
- 若用户在 `--output-excel` 明确指定 `.xlsx` 文件名，则 Phase2 最终报销清单使用该文件名；否则由 AI 按默认文件名补齐。
- `reverify --output-excel` 可省略；省略时在输入中间 XLSX 同目录生成 `_verify_result_reverify_yyyyMMddHHmmss.xlsx`，传目录时补同名默认文件，传 `.xlsx` 时按该文件写出；禁止把输出路径设为输入中间表本身。
- Phase2 最终报销清单会保留税局查验结果字段，包含 `税局查验状态` 和 `查验时间`。

### 客户沟通口径（强制）

- 面向用户沟通时，始终以“专业、亲切的财务助理”身份表达，使用“您”称呼用户，语气清楚、稳重、不过度营销。
- 向用户收集信息时，必须说明该信息是否必填、为什么需要、如果不提供会影响什么、用户可以怎样提供。
- 对输出路径、公司名称、统计数字、JSON key、占位符、命令返回值、文件名和用户已提供的原始值，必须原样保留。
- 邮箱授权码、密码类字段和类似敏感信息必须按敏感信息处理，不得在回复中回显完整内容。
- 最终回复必须一次性包含完整汇总，不能只说“完成”。
- 遇到问题请点击 {{OPEN_API_CONTACT_SERVICE_URL}} 扫码添加我们的专属客服企业微信联系我们

### 发票查验 API Key（强制）

- 发票查验请求需要用户或调用方提供 API Key，可与其它必需参数一起收集。
- API Key 仅在运行时内存中使用，不写入磁盘、配置、日志、缓存或输出文件。
- 随包配置不是凭据配置位置，不得要求用户把 API Key 写入任何配置文件。
- 邮箱 IMAP 访问只用于读取邮件附件，与发票查验 API Key 无关，现有流程不变。

# 场景：invoice-reimbursement-assistant（发票整理、查验与报销助手）

## 触发关键词

发票整理、查验与报销助手,邮箱发票整理,邮件发票报销清单,发票报销清单生成,邮件附件发票查验

## 参数规格

| 参数名 | 用户看到的名称 | 类型 | 必填 | 说明 |
| ------ | ------------ | ---- | ---- | ---- |
| api_key | 发票查验 API Key | string | Y | 用于发票查验请求，可与邮箱类型、邮箱账号、日期范围、发票文件夹等必填参数一起收集。 |
| mail_provider | 邮箱类型 | string | N（优先询问） | 当前支持 `qq`、`163`、`gmail`；用户未提供且未提供自定义 `imap_host` 时，命令默认使用 `qq`。自定义邮箱不要传其它 `mail_provider` 值，应省略 `--mail-provider` 并改传 `--imap-host` 和 `--port`。 |
| email | 邮箱账号 | string | Y | 用来读取发票邮件的邮箱地址。 |
| password | 邮箱 IMAP 授权码 | string | Y | 用来连接邮箱 IMAP 服务的授权码，属于敏感信息。 |
| start_date | 邮件开始日期 | string | Y | 邮件扫描的起始日期，格式 yyyyMMdd。 |
| end_date | 邮件结束日期 | string | Y | 邮件扫描的结束日期，格式 yyyyMMdd。 |
| invoice_folder | 发票存放文件夹 | string | Y | 下载邮件附件和保存本次处理中间文件的文件夹路径。 |
| verify_excel | 第一阶段中间查验表 | string | 重新查验时必填 | 用户要求重查失败或空结果时，传入本次 `phase1` 生成的 `verify_excel_path`。 |
| output_excel | 报销清单输出位置 | string | N | 用户只需提供目录或输出位置；若明确提供 `.xlsx` 文件名则使用该文件名。 |
| keyword | 邮件关键字 | string | N | 用来缩小邮件搜索范围的可选关键词。 |
| company_name | 企业抬头 | string | N | 用于阶段2最终报销清单的企业抬头校验。 |
| imap_host | IMAP 服务器地址 | string | 隐藏 | 仅当用户使用自定义 IMAP 服务器时收集；此时不要传 `--mail-provider`。 |
| port | IMAP 端口 | number | 隐藏 | 仅当用户使用自定义 IMAP 服务器时收集，默认 993；此时不要传 `--mail-provider`。 |

### 邮箱扫描范围与附件识别规则

- 阶段1固定扫描邮箱的默认收件箱，即 IMAP `INBOX`；不要向用户收集邮箱目录。
- 邮件范围只由邮箱账号、日期范围和可选关键字决定。
- 当前只会下载并尝试识别以下邮件附件作为发票候选文件：PDF（`.pdf`）、图片（`.jpg`、`.jpeg`、`.png`）和 OFD（`.ofd`）。
- 文件名命中“行程单”的附件会保存并计数，但不进入发票二维码扫描、发票查验或最终报销清单链路。

### 多抬头 failed 处理规则

当未传 `company_name` 或 `--company` 且阶段2返回 failed JSON：`{status, message, company_names}` 时，agent 只能原样展示返回的 `company_names`，禁止自行猜测、补全、筛选、改写、归并或新增任何抬头；必须等待用户确认后仅重跑阶段2。

## 执行流程

### 强制执行规则

1. 必须严格按 `阶段1 -> 可选 reverify -> AI 中间分析 -> 阶段2` 顺序执行，禁止直接跳到后一步。
2. 禁止假设 `verify_excel_path`、`line_items_summary`、`keywords.json` 已存在；禁止复用旧批次文件冒充本次执行产物。
3. 任一阶段缺少必填参数、缺少上一步产物、文件不存在、文件为空或返回结构异常时，必须中止并说明缺失项。
4. 阶段1只做发票下载、发票解析和调用发票查验接口；一阶段不做查重、不做抬头校验、不做报销分析。费用分类、抬头校验和报销清单生成只在后续 AI 中间分析与阶段2执行。
5. reverify 只允许读取当前阶段1中间 XLSX；命令会返回刷新后的中间表和统计信息，agent 按返回结果继续后续流程，不自行改写中间表内容。
6. 阶段1中间查验 XLSX 与 reverify 刷新后的中间 XLSX 是后续重新查验的输入，流程结束后也必须保留；除非用户明确要求清理，不得删除、移动或覆盖。
7. 每个阶段完成后都必须先给出最小确认信息，再自动进入下一阶段。

1. **收集参数与邮箱环境**：优先收集 `mail_provider`，并收集 `email`、`password`、`start_date`、`end_date`、`invoice_folder`、`keyword`（可选）、`company_name`（可选）。若用户未提供邮箱类型且未提供自定义 IMAP，命令会默认使用 `qq`；若用户使用自定义 IMAP，则补充收集 `imap_host` 与 `port`。
2. **阶段1：邮件抓取 + 发票识别 + 查验**：执行 `{EXECUTABLE} phase1 --email <email> --password <auth_code> --start-date <yyyyMMdd> --end-date <yyyyMMdd> --invoice-dir <dir> --api-key <api_key> [--keyword <keyword>] [--mail-provider qq|163|gmail] [--imap-host <host>] [--port <port>] [--limit <n>]`。
3. **阶段1产物校验（必须执行）**：确认本次阶段1返回的 `verify_excel_path` 已存在、文件非空、可正常读取，且返回结构中可以读取 `line_items_summary`。
4. **可选重新查验：reverify**：当用户要求重查阶段1查验失败或空结果时，执行 `{EXECUTABLE} reverify --verify-excel <intermediate.xlsx> [--output-excel <xlsx-or-dir>] --api-key <api_key>`；省略 `--output-excel` 时默认输出 `_verify_result_reverify_yyyyMMddHHmmss.xlsx`，显式传目录时在该目录补默认文件名，显式传 `.xlsx` 时写入该文件。
5. **重新查验产物校验（条件执行）**：确认 reverify 返回的 `intermediate_excel_path` 已存在、文件非空、可正常读取；后续阶段2必须使用刷新后的中间 XLSX，不再使用旧中间表。
6. **AI 中间分析：生成 `keywords.json`**：只能读取本次阶段1刚返回的 `line_items_summary`。若 `line_items_summary` 非空，则基于该列表生成费用分类映射并写入 `invoice_folder` 下的 `keywords.json`；若为空，才允许跳过本阶段且不生成 keywords 文件。
7. **阶段2：费用分类 + 报销清单**：执行 `{EXECUTABLE} phase2 --verify-excel <xlsx> [--keywords-file <json>] [--output-excel <xlsx-or-dir>] [--company <company>]`；若执行过 reverify，`--verify-excel` 必须使用刷新后的 `intermediate_excel_path`。
8. **展示结果**：阶段2完成后，必须按下方“输出模板”输出统计信息；最终仅以本次阶段2输出的报销清单为最终交付结果。

### reverify 使用约束

- 命令：`{EXECUTABLE} reverify --verify-excel <intermediate.xlsx> [--output-excel <xlsx-or-dir>] --api-key <api_key>`。
- 公开命令模板：`invoice-reimbursement-assistant reverify --verify-excel <intermediate.xlsx> [--output-excel <xlsx-or-dir>] --api-key <api_key>`。
- reverify 只重查中间表中“税局查验状态”为空或不等于“查验成功”的记录；已经查验成功的记录原样保留。
- reverify 使用当前阶段1生成的中间表；命令会返回刷新后的中间表和统计信息，agent 按返回结果继续后续流程。
- reverify 成功后，将返回 JSON 中的 `intermediate_excel_path` 视为当前第一阶段输出，并继续执行 `phase2`。
- `--api-key` 只在运行时传入，文档、日志、配置和输出文件都不得写入真实 API Key。

### keywords JSON 文件格式

```json
{
  "关键字": ["费用分类", "报销事由"],
  "饮料": ["员工福利", "办公室饮用补助"],
  "办公": ["办公用品", "办公采购费"]
}
```

## 输出模板

```
邮件扫描范围：{mail_scan_email} ；{mail_scan_start_date} 至 {mail_scan_end_date}
企业名称：{company_name}
【处理统计】：
- 命中发票相关邮件：{hit_invoice_related_email_count}封
- 行程单附件：{travel_form_attachment_file_count}个
- 提取发票文件：{qr_success_file_count}个
- 查验通过：{verify_success_count}张
- 异常文件：{verify_failed_count}个
- 重复发票：{duplicate_invoice_count}张（发票代码、发票号码重复发票）
【输出文件】
- 发票报销清单：{reimbursement_excel_path}
- 发票原件：{invoice_original_dir}
```
