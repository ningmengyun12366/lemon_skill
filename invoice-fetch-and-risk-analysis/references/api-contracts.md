# 发票获取及风险分析命令与返回结果

本文件说明本 Skill 的命令、输入输出、任务状态和续查规则。业务操作通过 CLI 执行，不自行拼接 HTTP 请求。

## 运行时事实

- 能力码：`invoice_batch_get`。
- CLI 策略：外置二进制和 PDF 字体下载，ZIP 包不包含 `bin/**`、`scripts/**` 或 `assets/fonts/*.ttf`。
- Windows 示例二进制：`.\bin\windows-amd64\invoice-assistant_windows_amd64.exe`。
- Linux x64：`./bin/linux-amd64/invoice-assistant_linux_amd64`。
- Linux ARM64：`./bin/linux-arm64/invoice-assistant_linux_arm64`。
- macOS x64：`./bin/darwin-amd64/invoice-assistant_darwin_amd64`。
- macOS ARM64：`./bin/darwin-arm64/invoice-assistant_darwin_arm64`。
- 外置二进制和 PDF 字体下载地址必须从安装文档读取，不能在参考文档中硬编码或猜测；当前平台二进制只下载一份，两份字体必须全部下载到 `assets/fonts/` 的原始文件名。

## CLI 命令清单

| 命令 | 用途 | 正常入口 |
| --- | --- | --- |
| `--help` | 检查二进制可执行 | 首次会话环境检测 |
| `task-preflight-check [--use-config-apikey]` | API Key 确认、企业绑定、登录态、workspace 和配置预检 | 每次业务执行前 |
| `workspace-config-writer` | 写入 API Key、workspace_dir 等配置 | 预检提示配置缺失或用户要求更新时 |
| `init-task --input <input.json>` | 创建本次任务 JSON、产物目录和步骤清单 | 参数确认后 |
| `fetch-invoices --task-file <task.json> [--confirm-duplicate]` | 创建或续查发票同步任务，拉取汇总 Excel 信息；默认复用原 task ID | 必执行 |
| `fetch-attachments --task-file <task.json> [--confirm-duplicate]` | 创建或续查附件下载任务并下载本地发票原件/附件；默认复用原 task ID | 用户确认需要下载发票原件/附件时 |
| `summarize --task-file <task.json>` | 基于 `artifacts.summaries_data_list` 生成摘要 Markdown | 取票后必执行 |
| `query-enterprise-profile-by-uscc` | 独立查询纳税人类型与经营范围 | 仅用于不存在任务文件的独立查询；任务流程由 `fetch-deduction` 使用快照查询 |
| `fetch-deduction --task-file <task.json> [--confirm-duplicate]` | 查询企业画像；一般纳税人创建或续查勾选已认证数据任务，其他纳税人落盘 `skipped`；默认复用原 task ID | 摘要后必执行 |
| `generate-invoice-list --task-file <task.json>` | 使用本次勾选结果生成发票清单 | `fetch-deduction` 成功或明确跳过后 |
| `analyze-invoice-data --task-file <task.json>` | 生成 `发票统计分析.xlsx` 并写回 `outputs.analysis_excel_path`；统计-only 成功路径同时清理摘要 Markdown、勾选已认证 JSON 和统计临时 Markdown | 统计或风险分析需要时 |
| `build-invoice-context-data --task-file <task.json>` | 根据已完成勾选步骤后的清单准备风险分析资料，返回 `outputs.risk_context_summary` | 风险分析必执行 |
| `markdown-to-pdf --input <report.md> --output <report.pdf> --task-file <task.json>` | 将风险 Markdown 转 PDF；成功后自动清理风险 Markdown、发票数据摘要 Markdown、勾选已认证 JSON、上下文 JSON 和 `risk_context_summary`，保留 `outputs.analysis_excel_path`，把完整正文和 PDF 绝对路径写入 `outputs.risk_report_body`、`outputs.risk_report_pdf_path`，并在 stdout 返回 `report_body`、`analysis_excel_path`、`report_pdf_path`、`artifact_root`、`report_size_bytes` | 风险报告落盘后 |
| `finalize-risk-artifacts --task-file <task.json> --report-md <report.md> --report-pdf <report.pdf>` | PDF 成功后删除风险 Markdown、发票数据摘要 Markdown、勾选已认证 JSON 和发票清单分析版上下文 JSON，清理中间引用，并持久化完整报告正文、PDF 绝对路径和 `execution.markdown_to_pdf` 成功状态；脚本固定动作，无需用户确认 | 风险 PDF 生成成功后 |

## 预检与 workspace 配置

预检命令：

```powershell
.\bin\windows-amd64\invoice-assistant_windows_amd64.exe task-preflight-check
```

指定企业名称或税号：

```powershell
.\bin\windows-amd64\invoice-assistant_windows_amd64.exe task-preflight-check --enterprise-name "企业名称或税号"
```

确认沿用当前已配置 API Key 后重新预检：

```powershell
.\bin\windows-amd64\invoice-assistant_windows_amd64.exe task-preflight-check --use-config-apikey
```

确认沿用当前已配置 API Key 且指定企业：

```powershell
.\bin\windows-amd64\invoice-assistant_windows_amd64.exe task-preflight-check --enterprise-name "企业名称或税号" --use-config-apikey
```

配置写入：

```powershell
.\bin\windows-amd64\invoice-assistant_windows_amd64.exe workspace-config-writer --workspace-dir "<工作区目录>"
.\bin\windows-amd64\invoice-assistant_windows_amd64.exe workspace-config-writer --apikey "<API Key>"
```

- 配置文件由二进制管理；Agent 需要更新配置时必须使用 `workspace-config-writer`。
- `task-preflight-check` 检测到已配置 API Key 时，默认返回 `apikey_confirmation_required`，不得直接继续调用业务接口；用户确认沿用当前 API Key 后，才追加 `--use-config-apikey` 重新预检。用户选择更换时，先用 `workspace-config-writer --apikey "<API Key>"` 写入新值。
- 新任务必须显式提供 `params.uscc`；不得根据上次企业或“只绑定一家企业”自动选择。预检可用于展示候选，但 `init-task` 仍会重新按税号精确查询。
- 登录检测结果使用 `login_status.status_assessment` 区分 `valid`、`not_ready` 和 `unknown`；原始 `login_status=null` 时必须是 `unknown`。`cause_category` 保存失败类别，`retryable` 仅表示未来可再次尝试，`auto_retry_allowed=false` 表示 Agent 当前不得自动重试。
- 查询请求失败、快速登录预检失败、不满足快速登录条件和地区切换限制均返回 `unknown`，但 `cause_category` 必须保留具体原因。超时使用 `login_status_query_timeout`、`quick_login_task_check_timeout`、`quick_login_precheck_timeout`、`quick_login_timeout` 或 `login_status_recheck_timeout`；地区限制使用 `region_switch_rate_limited`。报告必须明确输出该原因，不得只输出“状态未知”，也不得提供“登录失效”结论。登录上下文完整时，报告同时提供执行层生成的安全登录地址作为人工尝试入口；该入口与状态判断相互独立。
- 未命中现有分类的新错误使用 `cause_category=unclassified_login_check_failure`；保留返回的 `raw_cause`、`cause_detail`，原样说明实际错误。`auto_retry_allowed=false`，由 Agent 原样展示实际原因和报告内的安全登录地址后停止；不得由 Agent 自行拼接地址，也不得因存在地址而宣称登录失效。
- `runtime-config.json` 只能保存接口基础地址、能力码等非密运行时配置。
- API Key、token、密码、cookie、签名 URL、本机绝对路径不得写入 `runtime-config.json`、任务证据或发布包。
- “无权调用该能力”表示当前 API Key 未绑定本 skill 所需能力或企业，不是税局登录失败。

## 任务结果读取

`init-task` 返回本次任务文件。后续命令使用同一个 `--task-file <task.json>`，不要手工修改执行状态或任务标识。

| 返回字段 | 使用方式 |
| --- | --- |
| `enterprise_context` | 本次任务企业信息，用于确认当前对象，不用上次企业信息替换。 |
| `intent.params` | 核对本次企业、日期、附件需求及输出要求。 |
| `execution.<step>.status` | `submitting/submission_unknown/processing` 不能当作成功；`success/partial_success/failed/skipped` 按对应步骤处理。 |
| `execution.fetch_invoices.sync_task_id`、`execution.fetch_attachments.attachment_task_id`、`execution.fetch_deduction.deduction_task_id` | 当前异步任务标识，超时或中断后保留原值并续查。 |
| `steps` | 查看步骤是否启用及完成；`pending` 或 `stale` 的结果不能作为本次已完成产物。 |
| `artifacts.summaries_data_list` | 发票文件列表及日期范围、方向等信息，见 [get-summaries.md](get-summaries.md)。 |
| `outputs` | 读取摘要、统计 Excel、风险资料、完整报告正文及 PDF 路径。 |
| `storage` | 本次产物目录，直接采用命令返回的路径，不重新拼接。 |

勾选数据变化时，重新生成清单、所需统计、风险资料和报告，不沿用旧结果。一般纳税人须完成勾选数据获取；其他纳税人也须先运行 `fetch-deduction` 并得到明确 `skipped`，不能由 Agent 自行跳过。

## 防重复与续查

- `fetch-invoices`、`fetch-attachments`、`fetch-deduction` 超时、中断或仍在处理时，用原任务文件再次执行同一命令续查。不要另建任务或删除已有任务号。
- 命令提示原任务仍在执行时等待，不能同时启动多个进程重复操作。
- `submission_unknown` 表示无法确认提交结果，必须先核实原任务，不自动追加 `--confirm-duplicate`。
- 只有用户明确放弃续查并要求重新发起时，才在被阻断命令上追加 `--confirm-duplicate`，不预先携带；同类任务重新发起一次后继续查询该次结果，不将确认当作无限重提许可。
- 轮询达到 30 分钟上限不代表任务失败或成功，保留状态和任务号，按错误提示处理或继续查询。
- 命令返回终态失败时展示实际原因并停止当前及依赖步骤，不另建任务掩盖失败。

## stdout 与用户可见输出

- CLI stdout 可能包含 JSON 摘要、执行状态、Markdown 路径或错误信息；stdout 成功不等于用户已经看到结果。
- 预检报告、摘要 Markdown、`发票统计分析.xlsx` 路径、风险报告正文、风险 PDF 路径和失败影响范围必须按业务规则读取并展示。
- 统计分析成功路径读取 `outputs.analysis_excel_path`，完整显示 workbook 本地绝对路径文字和可选简短摘要；不展示临时 Markdown 路径或正文。
- 风险 PDF 生成成功路径由 `markdown-to-pdf --task-file` 完成清理后，必须直接使用 stdout 的 `report_body`、`analysis_excel_path`、`report_pdf_path`、`artifact_root`，或回退读取任务状态中的 `outputs.risk_report_body`、`outputs.analysis_excel_path`、`outputs.risk_report_pdf_path`；完整原样展示正文，并同时完整显示 Excel 与 PDF 的本地绝对路径文字。若使用 `MEDIA:` 交付文件，Excel 和 PDF 必须各有一条完整路径，不得只交付 PDF。链接不是必需；若使用链接，链接显示文本必须是同一条完整绝对路径，不得只显示文件名或“报告 PDF”等简称。不得另建摘要 Markdown。PDF 转换失败时完整显示保留的 Markdown 绝对路径和报告正文。
- `markdown-to-pdf` 的 stdout 会把用户可见的 `analysis_excel_path`、`report_pdf_path` 与 `artifact_root` 规范为正斜杠路径。若回退读取任务状态或其他仍使用 Windows 反斜杠的路径，Agent 必须仅在展示层把 `\` 转换为 `/`；Markdown 文本、链接与 `MEDIA:` 均不得直接输出 `\.hidden-workspace` 这类点号开头目录，媒体路径必须采用 `MEDIA:C:/Users/<用户名>/.hidden-workspace/...` 形式。
- 不向用户展示内部任务 JSON 路径、完整 payload、签名 URL、调试字段或敏感配置。
- stdout/stderr 出现网络、编码、权限、JSON 解析或接口异常时，按 [error-handling.md](error-handling.md) 分类处理。

## 深层参考

- 分阶段调用示例见 [invoice-fetch-and-risk-analysis.md](invoice-fetch-and-risk-analysis.md)。
- 发票文件列表读取和下载结果说明见 [get-summaries.md](get-summaries.md)。
- 企业画像检索见 [common-query-enterprise-profile-by-uscc.md](./common-query-enterprise-profile-by-uscc.md)。
- 通用异常规则见 [common-handle-response-exception.md](./common-handle-response-exception.md)。
