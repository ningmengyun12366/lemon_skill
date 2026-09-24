# 发票获取及风险分析流程

本文件说明 `invoice-fetch-and-risk-analysis` 的正常执行路线。普通任务必须先完成安装与 README 门禁、CLI 可用性检测、预检、场景识别和参数确认，再按用户需求裁剪执行取票、附件、统计和风险分析步骤。

## 新任务与继续任务

- 用户本轮重新提供或修改企业、税号、地区、个人账号、日期范围、附件需求、统计需求、风险分析需求、纳税人类型、行业或经营范围时，判定为新任务。
- 每次新触发都必须重新执行本次发票获取，不得直接复用历史对话中的 Excel、附件目录、任务 JSON、摘要、统计结果或风险报告。
- 只有用户明确表达“继续刚才的附件下载”“继续刚才的风险分析”“查看刚才的报告”等同一任务后续动作时，才允许复用当前任务 JSON 中已成功落盘的上游产物。
- 异步取票命令超时、长时间未返回、轮询中断或网络错误时仍属于原任务；必须使用原任务 JSON 续查已落盘的远端 task ID，不得判定为新任务或再次创建远端任务。
- 命令返回可继续查询的任务时，复用其结果继续；不得用旧 Excel、附件或报告代替本次产物。
- 无法判断时按新任务处理。

## 门禁顺序

1. 首次安装、更新、修复、二进制缺失、任一 PDF 字体缺失或用户询问下载方式时，先按安装文档读取远程下载表；不得猜测二进制或字体 URL。当前平台二进制只下载一份，两份字体则不区分平台且必须全部下载。
2. 通过 ZIP 包安装且当前会话尚未完整输出同级 `README.md` 时，先读取并完整输出 README 正文。
3. 检查当前平台 `invoice-assistant` 可执行性，至少运行 `--help`。
4. 每次业务执行前运行 `task-preflight-check`；用户已提供企业名称或税号时追加 `--enterprise-name`。若预检返回 `apikey_confirmation_required`，先询问用户沿用当前已配置 API Key 还是更换新 API Key；确认沿用后用 `--use-config-apikey` 重新预检，选择更换时先通过 `workspace-config-writer --apikey <新 API Key>` 更新后再预检。
5. 新任务必须由用户明确提供或选择 `uscc`；不得使用本地配置、上次企业或单企业绑定自动补齐。预检只查询候选，不保存企业状态。
6. 预检未通过时按 [error-handling.md](error-handling.md) 停止，不进入任何取票或分析步骤。

## 正常执行顺序

1. 识别场景和输出要求：只获取、获取+统计、获取+分析；发票原件/附件下载需求必须单独识别。
2. 按 [field-mapping.md](field-mapping.md) 整理显式 `uscc`、日期范围、发票原件/附件下载需求和输出要求。`areaCode`、`personalAccount`、`loginType` 由命令处理，不要求用户补填，也不从旧任务复制。主取票任务固定使用服务端当前支持的整批同步范围；不得询问或收集发票方向、票种、状态筛选条件。
3. 新任务进入 `init-task` 前，必须把“是否下载发票原件/附件”作为用户选择项问清。发票清单为取票成功后的固定必做项，不得询问是否生成；若同时询问是否统计、是否风险分析，必须把附件下载选项放在同一组问题中，不得只问统计/风险而静默默认不下载。
4. 参数齐全后，先向用户回显本次执行摘要：工作区、日期范围、输出要求、发票清单固定生成步骤、发票原件/附件下载分支、统计分支、风险分析分支、会跳过的步骤。
5. 调用 `init-task --input <input.json>` 生成任务 JSON，文件位于 `<workspace>/invoice-fetch-and-risk-analysis/tasks/`。
6. 调用 `fetch-invoices --task-file <task.json>` 获取本次发票数据，读取命令结果和任务状态后继续。
7. 用户确认需要下载发票原件/附件时，调用 `fetch-attachments --task-file <task.json>`，按返回的本地附件目录交付；用户确认不下载时跳过。
8. 调用 `summarize --task-file <task.json>` 生成摘要 Markdown。摘要正文必须先完整回显给用户，再进入后续步骤。
9. 必须调用 `fetch-deduction --task-file <task.json>`。该命令会先查询企业画像：一般纳税人创建或续查勾选任务并拉取勾选认证数据；其他纳税人写入 `execution.fetch_deduction.status=skipped` 和跳过原因。不得只调用 `query-enterprise-profile-by-uscc` 后自行推断可以跳过 `fetch-deduction`。
10. 只有 `execution.fetch_deduction.status=success` 或 `skipped` 后，才调用 `generate-invoice-list --task-file <task.json>`。每次取票成功后都必须生成发票清单，不得询问用户是否生成，也不得将其留作可选后续操作。一般纳税人的清单必须基于本次 `outputs.deduction_data_file` 生成并包含可匹配到的 `勾选认证日期`。
11. 若在发票清单、统计、风险上下文或风险报告生成后补执行或重执行了 `fetch-deduction`，任务会把下游状态标记为过期；必须依次重跑 `generate-invoice-list`、`analyze-invoice-data`（本次需要统计时）、`build-invoice-context-data`（本次需要风险分析时）和风险报告，不得沿用旧清单、旧统计或旧报告。
12. 用户要求统计或风险分析时，调用 `analyze-invoice-data --task-file <task.json>`。命令必须写出 `发票统计分析.xlsx`，成功时删除脚本内部临时 Markdown；若本次输出要求为“获取+统计”且没有后续风险报告，命令还会删除发票数据摘要 Markdown 和勾选已认证 JSON，并清理对应任务状态引用。用户可见输出为 `outputs.analysis_excel_path` 对应的 workbook 本地绝对路径文字和可选的 500 字以内简短统计摘要，不展示或暴露临时 Markdown 路径。
13. 用户要求风险分析时，检查同一任务 JSON 中 `execution.fetch_deduction` 和 `execution.generate_invoice_list`：一般纳税人必须分别为 `success` 和 `success`/`partial_success`，其他纳税人的勾选步骤必须为 `skipped`；清单必须在本次勾选步骤完成后生成。任一条件不满足时先补跑对应命令，不得以“数据不足”代替执行。
14. 风险分析必须调用 `build-invoice-context-data --task-file <task.json>` 生成发票清单分析版结构化数据，并写回 `outputs.risk_context_summary`。CLI 会拒绝未执行勾选、勾选文件缺失、未在勾选后重生成清单或清单依赖已过期的任务；命中门禁时按错误提示补跑，禁止绕过。
15. 基于 `outputs.risk_context_summary`、企业画像和风险提示词生成风险报告正文；分析版全量 JSON 与发票清单源文件映射仅用于定位原始证据和复核异常样本。脚本内部英文标识、字段名、枚举值、状态值、布尔值和缩写必须先按业务含义翻译为用户看得懂的中文，不得原样复制到报告。正文先保存为 `<产物根目录>/发票风险分析报告.md` 作为 PDF 输入和失败兜底；Agent 必须保留该正文的同一版本，禁止另写摘要版或任务回顾版报告。
16. 保存风险 Markdown 前由 Agent 对正文中的英文内容逐项做来源与语义审阅，不得使用“包含英文字母即禁止”的正则或固定词表判断。来自脚本或结构化上下文的内部标识必须翻译；源发票中的企业法定名称、商品型号、正式代码或通行缩写可以保留，但首次出现时必须紧跟中文说明。
17. 若语义审阅发现未翻译的内部英文，改写同一份报告正文并覆盖原 Markdown，再重新调用 `markdown-to-pdf`；重复完成“审阅、改写、转换”，直到同版 PDF 成功。该情况属于报告内容修正，不属于 PDF 技术失败，不得只交付 Markdown，也不得声称任务完成。
18. 调用本 skill 自带的 `markdown-to-pdf --input <report.md> --output <report.pdf> --task-file <task.json>` 生成同版 PDF；PDF 成功后，命令自动删除风险 Markdown、发票数据摘要 Markdown、勾选已认证 JSON 和发票清单分析版上下文 JSON，并清理任务状态中的 `summary_markdown_path`、`deduction_data_file`、`risk_context_summary` 等中间产物引用，同时将完整正文和 PDF 绝对路径分别写入 `outputs.risk_report_body`、`outputs.risk_report_pdf_path`，保留 `outputs.analysis_excel_path`，并在命令结果返回 `report_body`、`analysis_excel_path`、`report_pdf_path`、`artifact_root`。不得读取或调用通用 pdf skill，不得编写 Python、PowerShell、HTML 或其他临时转换脚本自行转 PDF。
19. PDF 生成成功后的清理是脚本成功路径的固定动作，不得再向用户询问是否删除；最终必须从命令结果或任务状态读取完整正文与路径，原样完整展示风险报告正文，并同时完整显示统计 Excel、风险 PDF 和本地产物目录包含盘符和全部父目录的本地绝对路径文字。若使用 `MEDIA:` 交付文件，必须为 Excel 和 PDF 分别输出一条完整路径，不得只输出 PDF。链接不是必需；若使用链接，链接显示文本必须是同一条完整绝对路径，不得把路径隐藏在文件名或“打开报告”等简称后面。不得把绝对路径裁剪成任务目录相对路径，不得另建或附加摘要 Markdown。
20. 只有字体、文件读取、渲染或写入等真正的 PDF 技术故障在按错误处理规则重试后仍失败，才不调用 `finalize-risk-artifacts`，并保留、展示风险 Markdown 正文和路径，说明 PDF 未生成及失败原因。

## 异步任务防重与续查

- `fetch-invoices`、`fetch-attachments`、`fetch-deduction` 中断、超时或仍在处理时，用原 `--task-file` 再次执行同一命令续查，不创建新任务或删除任务号。
- 命令提示任务正在执行时等待，不重复启动进程。返回 `submission_unknown` 或无法确认提交结果时先核实原任务，不能冒险重提。
- 终态失败按实际原因处理，不自动创建替代任务；超时并不证明任务失败。
- 只有用户明确放弃续查并要求重新发起，才在被阻断命令追加 `--confirm-duplicate`，不得预先或自动携带；主取票、附件和勾选同类任务各仅确认重新发起一次，之后继续查询该次结果。

## 用户可见输出门禁

- 工具成功不等于用户已看到结果。摘要、统计 Excel 路径、风险报告正文、风险 PDF 路径、预检报告、失败影响范围和最终产物路径必须在进入下一阶段前展示；所有本地交付路径的文字必须完整显示，包含盘符、全部父目录和文件名。链接不是必需；若使用链接，链接显示文本必须是同一条完整绝对路径。
- 用户可见的本地绝对路径必须在展示前把反斜杠 `\` 全部转换为正斜杠 `/`，但不得修改脚本内部真实路径。纯文本、Markdown 链接和 `MEDIA:` 行都执行同一转换；以点号开头的通用隐藏目录 `.hidden-workspace` 为例，禁止输出 `MEDIA:C:\Users\<用户名>\.hidden-workspace\...`，必须输出 `MEDIA:C:/Users/<用户名>/.hidden-workspace/...`，避免 Markdown 将 `\.` 解析为点号转义并吞掉目录分隔符。
- 摘要和风险报告正文必须与落盘 Markdown、命令结果或任务 JSON 字段保持同一版本，不得摘要、截断、重排或另写一版；统计分析成功路径以 `发票统计分析.xlsx` 为最终可见产物，风险分析成功路径不得另建摘要 Markdown 或任务回顾 Markdown。
- 风险报告正文必须是用户可直接理解的中文，不得展示脚本内部英文标识或未解释的英文缩写；若源业务资料必须保留英文原文，首次出现时必须附中文说明。发现内容问题时必须由 Agent 改写并重新生成 PDF，不能把 Markdown 作为最终交付物。
- 附件远程下载链接、签名 URL、内部任务 JSON 路径、临时目录、调试 payload 不作为正常用户输出。
- 产物路径只展示用户需要打开的目录或文件，例如发票清单目录、附件目录、`发票统计分析.xlsx`、风险报告 PDF；必须直接读取任务 JSON 或 `markdown-to-pdf` 结果中的绝对路径并完整显示路径文字，不得手工缩短或重新拼接。风险 PDF 转换失败时才完整显示风险 Markdown 绝对路径。
- 风险分析成功后的最终输出必须再次列出 `analysis_excel_path` 和 `report_pdf_path`，即使 Excel 路径已在统计阶段展示过也不得省略；缺少任一路径时不得声称最终交付完整。

## 部分成功处理

- 附件下载部分失败时，保留已成功下载的本地文件，向用户说明成功数、失败数、失败影响范围和可重试建议。
- `build-invoice-context-data` 返回 `partial_success` 时可继续风险分析，但必须在风险边界说明中写明受影响文件和影响范围。
- 统计或风险报告生成失败时，不得声称整体完成；只交付已成功的取票、附件或清单产物，并按 [error-handling.md](error-handling.md) 处理失败步骤。

## 深层参考

- 分阶段调用示例见 [invoice-fetch-and-risk-analysis.md](invoice-fetch-and-risk-analysis.md)。
- 统计口径见 [analyze-invoice-data.md](analyze-invoice-data.md)。
- 风险分析细则见 [invoice-risk-analysis.md](invoice-risk-analysis.md)。
- 预检规则见 [common-preflight-initialization-check.md](./common-preflight-initialization-check.md)。
- 通用异常处理见 [common-handle-response-exception.md](./common-handle-response-exception.md)。
