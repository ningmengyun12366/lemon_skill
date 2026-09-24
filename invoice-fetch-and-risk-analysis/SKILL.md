---
name: "invoice-fetch-and-risk-analysis"
display_name: "柠檬云发票获取及风险分析"
display_name_en: "invoice-fetch-and-risk-analysis"
description: "发票获取、发票附件下载、发票数据统计分析与风险分析。当用户提出以下需求时触发：发票获取与风险分析、发票获取、发票同步、附件发票下载、发票附件下载、发票风险分析、获取发票清单并分析风险"
description_zh: "提供发票获取、附件下载、发票数据统计和风险分析能力。"
description_en: "Fetches invoices and attachments, analyzes invoice data, and identifies invoice-related risks."
version: "0.1.6"
---

# 发票获取与风险分析

本 Skill 是发票获取、附件归集、发票清单生成、统计分析和风险分析的短执行入口。Agent 负责把用户输入整理成安全的取票/分析任务，走安装门禁、本地 CLI 预检、分阶段执行、结果回显和风险边界控制；详细字段、接口、提示词和异常规则按下方参考路由读取。

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

## 触发与任务判定

触发：发票获取、发票同步、发票附件下载、发票清单生成、发票数据统计分析、发票风险分析、获取发票清单并分析风险。

先判定是新任务还是继续任务：

- 新任务：用户重新提供或修改企业、税号、期间、附件需求、统计分析需求、风险分析需求、纳税人类型、行业或经营范围。新任务必须重新整理本次输入、重新预检，并重新执行必要的取票与分析步骤。
- 继续任务：用户明确承接刚才同一任务，例如继续下载附件、继续做风险分析、查看刚才的报告。只在这种情况下复用当前任务状态。
- 中断恢复任务：Agent、工具或进程因执行时间过长、调用超时、上下文中断或进程中断而恢复同一异步步骤时，无需用户再次表示“继续”，一律按继续任务处理。必须先定位并复用原任务 JSON，查询其中已保存的远端 task ID 状态。
- 原异步任务未进入终态前，禁止重新执行 `init-task`、更换任务文件或创建新的远端任务。只有用户明确表示“放弃原任务并重新发起”时，才允许追加 `--confirm-duplicate`。
- 除上述中断恢复任务外，无法确定时按新任务处理，不得复用历史发票 Excel、历史附件目录、历史任务 JSON 或历史风险报告。

## 快速开始（Agent Quickstart）

1. 安装文件门禁：二进制缺失、不可执行、任一 PDF 字体缺失或为空，或用户询问下载地址时，只能按安装文档的远端下载表下载当前平台二进制和两份字体，不得猜测、拼接或硬编码下载地址。
2. README 门禁：通过 ZIP 发布包安装、当前会话首次加载本地 `SKILL.md` 且尚未确认已完整输出 README，或用户询问准备/配置方式时，必须先读取同级 `README.md` 并将全文完整原样输出给用户；不得概括、删减、重排或等业务开始后再输出。
3. CLI 可用性检查：使用当前平台的 `invoice-assistant` 二进制；若本会话尚未证明可运行，先执行 `--help`。Windows 示例为 `.\bin\windows-amd64\invoice-assistant_windows_amd64.exe --help`。
4. 业务执行前先预检：任何业务命令、HTTP 请求、文件落盘、轮询或分析前，必须执行 `task-preflight-check`；若用户已提供企业名称或税号，可追加 `--enterprise-name`。若返回 `apikey_confirmation_required`，先询问用户沿用当前已配置 API Key 还是更换新 API Key；用户确认沿用后，重新执行预检并追加 `--use-config-apikey`。只有预检通过后才能继续。
5. 参数与本地配置：工作区只保存 API Key 和 workspace，不保存“当前企业”。每个新任务必须显式写入 `params.uscc`；地区、登录账号和登录类型由命令处理，不要求用户补填。不得从旧配置、上次企业或单企业绑定关系推断本次税号。主取票任务固定使用服务端当前支持的整批同步范围；参数确认阶段不得询问或收集发票方向、票种或状态筛选条件，也不得把这些条件写入任务参数。不得要求用户公开密钥，也不得在输出中暴露 API Key、token、cookie、签名、税局密码、验证码或二维码敏感材料。
6. 场景执行顺序按需求裁剪：`fetch-invoices` 获取发票数据；新任务参数确认时必须问清是否下载发票原件/附件，用户确认需要下载时执行 `fetch-attachments`；取票和摘要完成后必须执行 `fetch-deduction`，该命令会自动查询企业画像，一般纳税人拉取勾选认证数据，其他纳税人记录 `skipped`；只有 `execution.fetch_deduction.status` 为 `success` 或 `skipped` 后才能继续。每次取票成功后必须执行 `generate-invoice-list` 生成发票清单，发票清单不作为用户可选项，禁止询问是否生成，也禁止在取票完成后把生成清单提示为可选后续操作；需要统计时执行 `analyze-invoice-data` 并以 `发票统计分析.xlsx` 作为最终统计交付物；需要风险分析时必须确认清单是在本次勾选步骤之后生成，再执行 `build-invoice-context-data` 准备 `outputs.risk_context_summary` 和可追溯全量上下文。若补执行或重执行了 `fetch-deduction`，必须依次重跑 `generate-invoice-list`、`build-invoice-context-data` 和风险报告，禁止直接沿用旧清单或旧报告。报告正文优先基于压缩后的风险上下文生成，只有需要定位原始证据时才回查 `发票清单分析版数据.json` 或源清单。脚本内部英文标识、字段名、枚举值、状态值和缩写只用于分析，不得原样进入报告正文；必须先翻译为用户看得懂的中文。保存风险 Markdown 作为 PDF 输入后，必须调用带 `--task-file` 的 `markdown-to-pdf`，由脚本在 PDF 成功后自动清理中间产物，并把完整报告正文、`analysis_excel_path` 与 PDF 本地绝对路径写回或返回，无需再向用户确认删除；最终必须完整展示同一版报告正文，并同时完整显示统计 Excel、风险 PDF 和本地产物目录的本地绝对路径文字。
7. 风险分析分支：用户明确要求风险分析时，不得只调用 `query-enterprise-profile-by-uscc` 后直接构建风险上下文；必须执行 `fetch-deduction` 形成可恢复的任务状态。该命令查询企业画像失败或字段不完整时，才追问纳税人类型、行业或经营范围。一般纳税人只有勾选认证数据获取成功后才能继续；其他纳税人必须由该命令明确记录 `skipped` 后继续。一般纳税人需要关注进项抵扣、留抵、红冲、作废、供应商和关联交易口径；小规模纳税人不得套用一般纳税人抵扣判断。
8. 异步任务续查：`fetch-invoices`、`fetch-attachments`、`fetch-deduction` 超时、长时间无响应或轮询中断时，必须继续使用原 `--task-file`。续查保持本次任务对应的企业和期间；命令提示旧任务缺少企业信息时按提示处理，不手工补写；原任务状态不明时先核实。CLI 会复用已落盘的远端 task ID并继续查询；不得删除 task ID、清空执行状态、改建任务文件或再次调用远端创建接口。
9. 强制重复提交：只有用户明确表示一定要放弃续查并重新发起时，才允许在被阻断的命令上追加 `--confirm-duplicate`。不得预先、默认或自动携带该参数；一次用户确认在当前流程内为主取票、附件、勾选任务各提供一次重复创建授权，各类授权使用后立即失效。

## 执行门禁

- 若未配置 `workspace_dir`，默认使用 `workspace` 目录，不视为失败；只有用户明确要求其他目录或目录不可写时，才通过 `workspace-config-writer` 更新配置后重新预检。
- 预检、取票、统计、风险分析或接口响应中出现“无权调用该能力”时，必须立即停止当前流程，按 README 的 API Key 获取和能力绑定方式提示用户处理；不得解释为税局登录失败，也不得继续登录、取票或分析。
- `login_status.login_status=null`、`status_assessment=unknown`、登录状态查询请求失败、快速登录预检请求失败、不满足快速登录条件或地区切换安全限制，都只表示本次无法确认登录状态；不得表述为登录无效、失效或过期。超时和地区限制必须分别按报告标题及 `cause_category` 明确输出，不得只笼统回复“状态未知”。未命中的新错误统一按 `unclassified_login_check_failure` 处理，原样保留并展示 `cause_detail`，不得猜测归因。预检失败后必须原样展示报告并停止，Agent 不得自动重试。登录上下文完整时，执行层会在所有登录检查失败报告中提供安全登录地址作为人工尝试入口；必须完整展示该地址，但要明确它只是兜底手段，不代表已确认登录失效，也不得由 Agent 自行拼接地址。
- 响应对象或 CLI 输出命中公共异常规则时，按 `./references/common-handle-response-exception.md` 处理，并立即终止后续流程。
- 每次新触发取票/风险分析场景，都必须重新执行本次发票获取；不得直接把历史对话中的 Excel、附件目录、摘要或报告当成本次输入。
- 异步任务正在执行、轮询超时或网络中断不构成新任务。此时必须续查原远端 task ID；只有远端已明确终止，或用户明确确认强制重复提交，才可进入新的创建动作。

## 长流程输出规则

- 长流程必须“先输出、再执行；先核对、再推进”。进入下一阶段前，必须先向用户输出当前已完成什么、正在做什么、下一步是什么。
- 至少输出：主场景判定、本次任务清单、待补充参数或参数齐全结论、最终执行参数摘要、每个关键子步骤的完成反馈、实际产出、异步任务编号和状态变化、生成文件或报告正文、失败影响范围与下一步建议、最终结果正文。
- 命令生成且属于用户交付物的 Markdown 必须按场景要求原样渲染；不得摘要、改写、脱敏、重排、截断，也不得只用文件路径替代正文。统计分析成功路径只展示 Excel 本地绝对路径文字和必要的简短摘要，并由脚本清理统计临时 Markdown、摘要 Markdown 和勾选已认证 JSON；风险分析 PDF 成功路径必须使用 `markdown-to-pdf` 返回的 `report_body` 或任务状态中的 `outputs.risk_report_body` 原样完整展示报告正文，并同时使用 `analysis_excel_path` 和 `report_pdf_path` 完整显示 Excel 与 PDF 的本地绝对路径文字。若使用 `MEDIA:` 交付文件，必须为 Excel 和 PDF 分别输出一条完整路径，不得只给 PDF。
- 风险报告必须使用面向老板、会计和财务负责人的通俗中文。Agent 必须按来源和语义区分英文内容，不得按“是否含英文字母”或固定词表一刀切：脚本内部英文标识、英文键名、枚举值、状态值、布尔值和调试文字必须按业务含义翻译为中文；源发票中的企业法定名称、商品型号、正式代码或通行缩写可以保留原文，但首次出现时必须紧跟中文说明。发现未翻译的内部英文时，必须改写同一份报告并重新调用 `markdown-to-pdf` 生成 PDF；不得把 Markdown 当作最终结果，也不得把该情况归入 PDF 技术失败兜底。
- 工具成功不等于用户已看见结果。凡是生成 Markdown、表格、分析结论、任务编号、附件结果、报告文件或用户应感知的失败信息，必须在进入下一步前先发到对话框。本地交付物的路径文字必须完整显示，包含盘符、全部父目录和文件名，不得裁剪为相对路径或只显示文件名。链接不是必需；若使用 Markdown 链接，链接显示文本必须是同一条完整绝对路径，禁止使用“报告 PDF”“文件名”或“打开文件”等名称隐藏真实路径。
- PDF 成功后禁止再创建、写入或展示任何另行摘要、改写或缩写的 `invoice-risk-analysis-*.md`、任务回顾 Markdown 或其他辅助 Markdown；不得把这类文件写到 workspace 根目录或作为聊天附件替代完整报告正文与 PDF。

## Windows、下载与 JSON 输入

- 所有展示给用户的本地路径，包括纯文本路径、Markdown 链接的显示文字与目标地址以及 `MEDIA:` 行，输出前必须把 Windows 反斜杠 `\` 统一转换为正斜杠 `/`；禁止直接复制包含 `\.hidden-workspace` 这类点号开头目录的原始路径，否则 Markdown 会把 `\.` 当作点号转义并吞掉目录分隔符。媒体交付必须使用 `MEDIA:C:/Users/<用户名>/.hidden-workspace/.../文件名` 形式。只转换用户可见文本，不改变脚本内部用于文件读写的真实路径。
- PowerShell 下载 ZIP 包、外置二进制或外置 PDF 字体时，只能写入临时 `.ps1`，脚本内设置 TLS 1.2，使用 `Invoke-WebRequest -Uri <下载地址> -OutFile <本地保存路径> -UseBasicParsing`，再通过 `powershell -ExecutionPolicy Bypass -File "<临时ps1脚本路径>"` 执行；完成后删除临时脚本并验证目标文件存在且非空。
- Windows 下输入包含中文时，优先写入 UTF-8 JSON 文件并通过 `--input` 传给 CLI；不要优先使用 stdin 管道直传 JSON。
- PowerShell 生成 JSON 时优先使用对象加 `ConvertTo-Json`，再用 .NET UTF-8 无 BOM 写入文件；不要手写复杂内联 JSON 或 here-string 转义。
- 若出现中文变成 `????`、接口参数异常或 `Unexpected UTF-8 BOM`，优先按“UTF-8 文件 + `--input`”方式重写入参后重试。

## 安全规则

- 正常用户输出中不得暴露内部 JSON 路径、命令临时路径、正式 payload、下载签名 URL、token、cookie、API Key、税局密码、短信验证码、二维码截图或人脸认证材料。
- 展示本地交付物时，只展示用户需要打开的发票清单、附件目录、`发票统计分析.xlsx`、风险分析 PDF 等结果，并统一完整显示本地绝对路径文字；即使同时提供链接，也不得用简称或文件名替代可见的完整路径。内部任务 JSON、统计临时 Markdown、发票数据摘要 Markdown、勾选已认证 JSON、风险报告 Markdown、压缩风险上下文和发票清单分析版上下文 JSON 在成功路径不展示，也不得另建摘要 Markdown 作为替代交付物。
- 风险分析只能输出风险、疑点、边界和复核建议；不得武断定性违法，不得只凭进项大于销项判定高风险。
- 压缩风险上下文和分析版数据中的内部英文标识仅供程序与 Agent 定位证据，禁止复制到用户正文；例如方向、状态、真假值、字段路径和集中度缩写都必须改写为通俗中文。
- 附件远端下载链接必须先转成本地交付物或本地文件链接后再展示；不得直接暴露签名 URL、临时 URL 或 cookie。
- 不得绕过安装文档、README 门禁、CLI 可用性检查、`task-preflight-check`、`fetch-deduction` 状态落盘、勾选后清单重生成、上下文生成、Markdown 原文回显或异常终止规则。

## 参考路由

- 正常流程、新任务/继续任务、取票、附件、清单、统计、风险分析、阶段输出和部分成功处理：`./references/workflow.md`。
- 自然语言到取票期间、附件需求、企业画像、纳税人类型、行业、统计和风险分析参数的映射，以及 Windows UTF-8 JSON 输入规则：`./references/field-mapping.md`。
- CLI 命令、预检、本地配置、`fetch-invoices`、`fetch-attachments`、`generate-invoice-list`、`analyze-invoice-data`、`build-invoice-context-data`、`markdown-to-pdf` 和返回结果：`./references/api-contracts.md`。
- API Key 能力边界、企业绑定边界、附件下载边界、敏感输出边界、风险分析数据源边界、一般纳税人/小规模纳税人口径和人工停止条件：`./references/risk-boundaries.md`。
- 预检失败、接口异常、取票失败、附件部分失败、统计失败、风险分析数据缺失、PDF 转换失败和最终兜底处理：`./references/error-handling.md`。

按需读取补充说明：预检见 `./references/common-preflight-initialization-check.md`；错误提示见 `./references/common-handle-response-exception.md`；独立企业查询见 `./references/common-query-enterprise-profile-by-uscc.md`；操作示例与文件列表见 `./references/invoice-fetch-and-risk-analysis.md`、`./references/get-summaries.md`；统计与风险分析分别见 `./references/analyze-invoice-data.md`、`./references/invoice-risk-analysis.md` 和对应的中文提示词。
