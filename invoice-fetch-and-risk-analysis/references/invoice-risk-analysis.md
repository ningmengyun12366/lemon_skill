# 场景：invoice-risk-analysis（发票风险分析）

## 模板用途

用于基于发票清单目录下的全量发票明细数据执行税务风险分析；当发票清单目录缺失时，可回退到指定原始发票 Excel 文件或目录先生成发票清单，再将清单转换为适合 Agent 直接载入上下文的完整结构化数据，之后继续风险分析，聚焦纳税人类型差异、行业特性适配、风险识别与行动建议输出。

## 触发关键词

发票风险分析,税务风险分析,进销项风险扫描,发票风险扫描,发票异常分析,发票合规风险,综合风险等级,分角色行动清单,一般纳税人风险分析,小规模纳税人风险分析,行业适配风险分析

## 场景说明

- 业务目标：基于发票数据输出可读、可执行、可追溯的风险分析结论。
- 典型用户：老板、会计、财务负责人、税务顾问、AI Agent 编排流程。
- 使用前提：优先提供当前任务 JSON 中可复用的发票清单目录或其对应的分析版结构化数据；若发票清单缺失，则需提供单个原始 xlsx 文件路径或含多个原始 xlsx 的目录路径以生成发票清单。
- 输入来源：发票清单目录（主数据源）/ 发票清单分析版结构化数据（主载入源） + 企业画像（纳税人类型、行业） + 提示词约束；原始 xlsx 文件或目录仅作为发票清单缺失时的兜底输入来源。

## 输入与约束

| 输入项          | 是否必需 | 说明                                                           | 缺失处理                                                  |
| --------------- | -------- | -------------------------------------------------------------- | --------------------------------------------------------- |
| input_file_path | 否       | 单个待分析原始 xlsx 文件路径（仅在发票清单缺失时用于生成清单） | 与 input_dir 二选一；当发票清单缺失时，均缺失则报错       |
| input_dir       | 否       | 待分析目录（非递归扫描，仅在发票清单缺失时用于生成清单）       | 与 input_file_path 二选一；当发票清单缺失时，均缺失则报错 |
| taxpayer_type   | 是       | small_scale/general                                            | 缺失时优先自动查询，自动查询失败后再要求用户补充          |
| industry_type   | 是       | 行业类型                                                       | 缺失时优先自动查询，自动查询失败后再要求用户补充          |

说明：

- 本场景风险分析仅允许使用以下数据源：
  1. 发票清单目录中的全量清单文件
  2. 由发票清单目录转换得到的分析版结构化数据文件（如 JSON）
  3. 企业画像（`taxpayer_type`、`industry_type`）
  4. 提示词文件约束
- 若发票清单目录或其分析版结构化数据缺失，必须先生成发票清单并完成数据转换后再进入风险分析。
- 除上述四类外，风险分析阶段暂不引入其他新增数据源。
- 风险分析时必须严格遵循提示词文件：`references/发票风险分析提示词.md`。

## 执行逻辑

1. 读取输入并校验：

- 优先读取当前任务 JSON 中可复用的 `storage.invoice_list_dir`、`artifacts.invoice_list_dir`、`outputs.invoice_context_data_path`、`outputs.invoice_context_data`。
- 若发票清单缺失，且提供 input_file_path，则读取该单文件并调用 generate-invoice-list 生成发票清单。
- 若发票清单缺失，且提供 input_dir，则读取目录下所有 .xlsx 文件并调用 generate-invoice-list 批量生成发票清单。
- 若 taxpayer_type 或 industry_type 缺失，已有任务时优先重新执行 `fetch-deduction --task-file <task.json>`，由该命令使用任务快照查询并写入企业画像；仅当查询失败、无结果或结果仍不完整时，才停止流程并提示用户补充。

2. 企业画像自动补齐：

- 先检查当前任务 JSON 的 `intent.params`、`artifacts.enterprise_profile` 中是否已有 `taxpayer_type`、`industry_type`。
- 若缺少上述任一字段且当前任务已具备 `enterprise_context`，必须执行 `fetch-deduction --task-file <task.json>`，不得另行按税号查询企业列表，也不得在执行前追问地区或个人账号。
- 自动查询成功后：
  - 将返回的 `taxpayer_type` 直接写回企业画像。
  - 若 `industry_type` 缺失，则可使用查询结果中的 `business_scope` 回填到 `industry_type`，并同时保留 `business_scope` 作为辅助画像字段。
- 若自动查询失败、无结果、或查询后仍缺少 `taxpayer_type` / `industry_type`，才允许向用户追问缺失字段。

3. 会话发票清单数据优先复用（新增优先级规则）：

- 在当前任务 JSON 中读取 `outputs.invoice_context_data_path` / `outputs.invoice_context_data`（分析版主数据）与 `storage.invoice_list_dir` / `artifacts.invoice_list_dir`（文件主数据）。
- 若分析版数据与发票清单目录都存在，直接作为风险分析输入，不再要求额外数据源。
- 若仅存在发票清单目录，则必须先执行一次数据转换，生成分析版结构化数据后再分析。
- 若仅存在分析版数据而缺少原始清单目录，记录 warning 并继续，但需在结论中提示“未复核原始清单文件，仅基于已落盘分析版数据分析”。
- 若分析过程中对单条记录、字段映射、异常金额、票种状态存在疑惑，Agent 必须结合分析版中的 `file_id` 与顶层 `files` 映射回查原始清单文件，不得仅凭分析版摘要字段武断下结论。

4. 发票清单兜底生成：

- 若当前任务 JSON 缺少 `storage.invoice_list_dir` / `artifacts.invoice_list_dir`，必须先调用 generate-invoice-list 场景生成发票清单目录。
- generate-invoice-list 的执行规则、生成口径与输出结构以本工作区发票清单生成命令为准。
- 发票清单生成成功后，必须将结果写回任务 JSON 的 `storage.invoice_list_dir` 或 `artifacts.invoice_list_dir`，作为风险分析输入基础。

5. 发票清单完整数据转换（新增关键步骤）：

- 必须执行 Go 命令 `build-invoice-context-data`，读取发票清单目录下所有 .xlsx 文件，逐个加载“发票清单”sheet，按文件维度和行维度保留完整数据。
- 命令统一通过 `--task-file` 驱动执行，必须优先从任务 JSON 的 `storage.invoice_list_dir` 或 `artifacts.invoice_list_dir` 解析输入目录，并默认输出 `storage.root_dir/发票清单分析版数据.json`。
- 命令执行后，必须将结果写回任务 JSON：
  - `execution.build_invoice_context_data`
  - `outputs.invoice_context_data_path`
  - `outputs.risk_context_summary`
  - `outputs.invoice_context_data`（仅当数据量可控时写回）
  - 命令执行 warnings 保留在执行结果和分析版 JSON 的 `warnings` 中。
- 若脚本输出为 `partial_success`，允许继续进入风险分析，但必须在风险结论中显式说明受影响文件和影响范围；若脚本输出为 `failed`，必须停止风险分析流程并返回错误信息。
- 分析版结构化数据应尽量轻量，同时满足后续风险分析和按 `file_id` 回溯到源文件的要求。
- 分析版结构化数据至少必须包含：`file_id`、发票头字段以及明细数组；顶层 `files` 节点必须保留 `file_id` 到源文件路径的映射。
- 转换阶段不得只保留汇总值或抽样数据；必须保证目录内全部文件、全部有效发票和全部关键明细都可追溯到源文件。
- 若单个文件存在坏行、空列名、日期或金额无法解析等问题，不得直接丢弃对应发票；应尽量保留可用内容并在 `warnings` 中记录。
- 脚本执行完成后，Agent 在正式撰写报告前必须优先使用 `outputs.risk_context_summary` 作为主输入；仅当该摘要缺失，或某个结论需要定位原始证据时，才读取 `outputs.invoice_context_data_path` 指向的分析版 JSON，并进一步按 `file_id` 回查源清单。

6. 文件方向标记（关键规则）：

- 文件名包含“取得发票” -> 该文件内发票数据标记为进项。
- 文件名包含“开具发票” -> 该文件内发票数据标记为销项。
- 两者都不包含 -> 不标记方向，并记录 warning。

7. 风险分析数据准备：

- 以 `outputs.risk_context_summary` 作为风险报告撰写的结构化主输入；该摘要已预先压缩月度、进销项、票种、客户/供应商、红冲作废与异常指标。
- 以 `outputs.invoice_context_data` 或 `outputs.invoice_context_data_path` 指向的分析版全量结构化数据作为证据回溯输入，不得默认把全部 `records` 注入模型。
- 以发票清单目录中的原始清单文件作为复核输入；当分析版不足以解释结论时，必须结合分析版中的 `file_id` 与顶层 `files` 映射回查原始清单文件中的对应发票和明细。
- 合并企业画像 `taxpayer_type`、`industry_type` 形成风险分析输入上下文；若 `industry_type` 由 `business_scope` 回填，需沿用同一口径进入后续分析。
- 若方向标记因文件命名不可识别而为 unknown，风险分析必须在结论中显式标注该边界与影响。
- 风险分析正文中的金额、月份、对象、票种、红冲/作废结论，必须尽量绑定到完整结构化数据中的原始记录，不得只依据汇总统计推断。

8. 提示词装载与分析执行：

- 读取 `references/发票风险分析提示词.md` 全文作为分析主提示。
- 将企业基础信息、`outputs.risk_context_summary`、必要的原始文件边界说明和少量可追溯异常样本注入提示词上下文；不得默认将完整 `records` 全量注入模型。
- 明确要求模型严格按提示词中的结构与口径输出，不得省略关键章节；其中金额统一按“元”展示，保留两位小数并使用千分位分隔。
- 风险分析正文禁止输出 emoji、颜文字、装饰性图标或其他表情符号；若模型首轮结果包含上述内容，必须在进入下一步前重生成或清洗为纯文本版本。
- 风险分析正文必须使用通俗中文。脚本内部英文标识、字段名、枚举值、状态值、布尔值和缩写不得原样进入正文，必须按业务含义翻译；源发票中的企业法定名称、商品型号或必须保留的正式代码可以保留，但首次出现时必须紧跟中文说明。

9. 结果校验：

- 检查输出是否包含以下必备部分：
  - 一句话总结
  - 企业属性判断
  - 总体风险判断
  - 重点发现（5到10条）
  - 分模块详细分析（9层）
  - 分角色行动清单
  - 风险边界说明
- 检查关键结论是否可回溯到分析版数据中的 `file_id`，并进一步映射到顶层 `files` 中的源文件。
- 检查是否存在因转换丢失文件、丢失列、丢失行而导致的数据缺口；若存在，必须停止并提示补齐后重跑。
- 检查全文是否不包含 emoji、颜文字、装饰性图标或其他表情符号。
- 由 Agent 对正文中的英文内容逐项做来源与语义审阅，不得按是否包含英文字母或固定词表一刀切。确认脚本方向值、发票状态、真假值、统计字段和文件定位字段均已改写为通俗中文，同时保留有业务必要且已附中文说明的企业法定名称、商品型号、正式代码或通行缩写。
- 若缺失，触发一次补全生成。
- 若发现未翻译的内部英文标识，改写同一份报告并覆盖原 Markdown，再重新调用 `markdown-to-pdf`；这属于报告内容修正，不属于 PDF 技术失败，必须继续生成 PDF，不得只返回 Markdown。

10. 输出返回：

- 返回结构化分析结果与可读文本。
- 风险分析正文生成完成后，必须先将该正文原模原样保存为本地 Markdown 文件，再进行任何对话回显或后续格式转换。
- 本地 Markdown 文件默认保存到任务产物根目录 `storage.root_dir` 下，文件名固定为 `发票风险分析报告.md`，该文件是 PDF 输入和 PDF 失败时的兜底交付物。
- 若本次执行中生成了分析版结构化数据文件，只能作为内部风险分析上下文使用，不作为 PDF 成功后的最终用户路径暴露。
- 保存 Markdown 后，必须调用本 skill 自带且带 `--task-file` 的 `markdown-to-pdf` 命令将同一份 Markdown 原文转换为 PDF；默认 PDF 文件名固定为 `发票风险分析报告.pdf`，与 Markdown 位于同一目录。不得读取或调用通用 pdf skill，不得编写 Python、PowerShell、HTML 或其他临时转换脚本自行转 PDF。
- PDF 转换命令统一为：`.\bin\windows-amd64\invoice-assistant_windows_amd64.exe markdown-to-pdf --input "<markdown_path>" --output "<pdf_path>" --task-file "<task_json_path>"`。
- PDF 生成成功后，`markdown-to-pdf` 会在脚本内自动删除风险 Markdown、`发票数据摘要.md`、`勾选已认证数据.json` 和 `发票清单分析版数据.json`，并清理任务状态中的 `summary_markdown_path`、`deduction_data_file`、`risk_context_summary` 等中间产物引用；同时保留 `outputs.analysis_excel_path`，把完整正文和 PDF 绝对路径写入 `outputs.risk_report_body`、`outputs.risk_report_pdf_path`，并在命令结果中返回 `report_body`、`analysis_excel_path`、`report_pdf_path`、`artifact_root`。这是脚本固定清理动作，不得再向用户询问是否删除。
- 必须在对话中完整展示发票风险分析报告全文，不得仅返回摘要或下载链接。
- 对话中展示给用户的风险分析正文，必须与已保存并用于 PDF 转换的 Markdown 正文完全一致，不得在保存后再改写、补写、缩写或替换版本。
- PDF 生成成功时，最终必须原样完整返回 `report_body` 或 `outputs.risk_report_body` 对应的报告正文，并同时完整显示 `analysis_excel_path` 或 `outputs.analysis_excel_path`、`report_pdf_path` 或 `outputs.risk_report_pdf_path` 及 `artifact_root` 包含盘符和全部父目录的本地绝对路径文字。若使用 `MEDIA:` 交付文件，Excel 和 PDF 必须分别输出完整路径，不得只给 PDF。链接不是必需；若使用链接，链接显示文本必须是同一条完整绝对路径，不得使用文件名或“报告 PDF”等简称隐藏真实路径。不得缩写正文、裁剪路径或创建、附加摘要 Markdown。成功路径不再给出风险 Markdown 或分析版上下文 JSON 路径。
- 最终展示路径时必须把 Windows 反斜杠 `\` 转换为正斜杠 `/`，且只转换展示文本，不改变脚本内部读写路径。纯文本、Markdown 链接和 `MEDIA:` 使用同一条规范；以通用隐藏目录 `.hidden-workspace` 为例，Excel 使用 `MEDIA:C:/Users/<用户名>/.hidden-workspace/.../发票统计分析.xlsx`，PDF 使用 `MEDIA:C:/Users/<用户名>/.hidden-workspace/.../发票风险分析报告.pdf`。
- 只有字体、文件读取、渲染或写入等真正的 PDF 技术故障在重试后仍失败，才不调用 finalizer、不删除 Markdown 或分析版上下文 JSON，并最终返回 Markdown 路径和报告正文，说明 PDF 未生成及失败原因；未翻译内部英文不适用此兜底，必须改写后重新生成 PDF。
- 返回 warnings（如方向无法标记、字段缺失、转换过程数据异常、仅基于结构化副本分析等）。

## 执行规则（建议）

1. 当发票清单目录缺失时，input_file_path 与 input_dir 必须二选一，且不能同时为空。
2. 纳税人类型和行业类型为强制输入，但若缺失必须先通过 `fetch-deduction --task-file <task.json>` 使用任务企业快照自动查询；只有查询失败、无结果或字段仍不完整时，才允许停止并追问用户。
3. 风险分析数据来源优先级固定为：压缩风险上下文 `outputs.risk_context_summary` > 分析版结构化数据 `outputs.invoice_context_data` / `outputs.invoice_context_data_path` > 发票清单目录 `storage.invoice_list_dir` / `artifacts.invoice_list_dir`。
4. 风险分析阶段仅允许使用压缩风险上下文、发票清单全量数据、发票清单分析版结构化数据、企业画像与提示词约束。
5. 当进入发票清单兜底生成或数据转换流程时，仅处理 .xlsx 文件，忽略临时文件（如 ~$ 开头）和其他扩展名。
6. 任何“方向无法标记”的文件不得被丢弃，需保留并标注 unknown。
7. 必须区分一般纳税人与小规模纳税人分析逻辑，不得混用口径。
8. 必须结合行业特性输出分析，不可脱离行业背景机械判断。
9. 不得武断定性违法，只能输出风险、疑点与复核建议。
10. 风险分析正文禁止包含 emoji、颜文字、装饰性图标或其他表情符号。
11. 风险报告中的脚本内部英文标识必须翻译为通俗中文；不得把所有英文一律判错。发现未翻译内容时必须改写同一份报告并重新生成 PDF，不得只交付 Markdown。
12. 风险分析结果必须先落盘为本地 Markdown 文件，再调用本 skill 自带且带 `--task-file` 的 `markdown-to-pdf` 转换为 PDF；不得读取或调用通用 pdf skill，不得编写 Python、PowerShell、HTML 或其他临时转换脚本自行转 PDF。PDF 成功后由该命令自动清理中间 Markdown、勾选已认证 JSON、上下文 JSON 和 `risk_context_summary`，并持久化、返回完整报告正文与 PDF 绝对路径；不得另行向用户确认删除，并在对话中完整展示同一份报告正文和 PDF 本地绝对路径文字。链接不是必需；若使用链接，链接显示文本必须是同一条完整绝对路径。不得另建摘要 Markdown。
13. 生成的本地 Markdown 与 PDF 应默认放置于任务产物根目录 `storage.root_dir`，文件名分别为 `发票风险分析报告.md`、`发票风险分析报告.pdf`。
14. 发票清单分析版结构化数据必须默认放置于任务产物根目录 `storage.root_dir`，文件名固定为 `发票清单分析版数据.json`；PDF 成功后作为内部上下文删除，不作为最终用户交付物。
15. 结构化数据转换必须保留全部源文件、全部有效发票与全部关键明细，不得仅保留汇总字段或截断数据集。

## 发票清单与数据转换脚本

发票清单生成命令：

- `generate-invoice-list`

发票清单完整数据转换命令：

- `build-invoice-context-data`

统一入口命令（生成发票清单）：

```bash
.\bin\windows-amd64\invoice-assistant_windows_amd64.exe generate-invoice-list \
  --task-file "<workspace>/invoice-fetch-and-risk-analysis/tasks/<任务文件名>.json"
```

说明：该步骤负责把原始发票 Excel 生成到“发票清单”目录，作为风险分析的原始主数据输入。

## build_invoice_context_data 执行逻辑

- 执行命令：`build-invoice-context-data`
- 首选执行方式：

```bash
.\bin\windows-amd64\invoice-assistant_windows_amd64.exe build-invoice-context-data \
  --task-file "<workspace>/invoice-fetch-and-risk-analysis/tasks/<任务文件名>.json"
```

- 脚本执行步骤必须固定如下：
  1. 解析 `--task-file`，优先从任务 JSON 读取 `storage.invoice_list_dir` / `artifacts.invoice_list_dir` 和默认输出路径。
  2. 扫描发票清单目录下全部 `.xlsx` 文件，忽略 `~$` 临时文件和其他扩展名。
  3. 对每个文件读取“发票清单”sheet，并逐行提取完整记录。
  4. 根据文件名执行方向标记：文件名含“取得发票”标记为 `input`，含“开具发票”标记为 `output`，否则标记为 `unknown` 并记录 warning。
  5. 生成分析版记录：默认供 Agent 风险分析使用，尽量减少重复字段。
  6. 汇总生成 `发票清单分析版数据.json`，其中包含完整 `records` 与顶层 `risk_summary`。
  7. 将顶层 `risk_summary` 写回任务 JSON 的 `outputs.risk_context_summary`，供 Agent 作为报告撰写主上下文。
  8. 若分析版记录量较小，可附带写回 `outputs.invoice_context_data`；若记录量较大，则仅保留 `outputs.invoice_context_data_path`，避免任务 JSON 过大。

- 脚本执行结果必须至少包含：
  - `status`
  - `input_dir`
  - `analysis_output_json`
  - `file_count`
  - `record_count`
  - `risk_summary`
  - `warning_count`
  - `error_count`
  - `warnings`
  - `errors`

- 风险分析阶段读取规则：
  - 必须优先使用任务 JSON 的 `outputs.risk_context_summary` 作为报告撰写上下文。
  - 当需要补充原始证据或摘要缺失时，必须优先读取 `outputs.invoice_context_data_path` 指向的分析版本地 JSON 文件正文。
  - 仅当需要补充原始证据、`outputs.risk_context_summary` 缺失，或本地 JSON 路径缺失且 `outputs.invoice_context_data` 已写回时，才允许回退读取完整结构化数据。
  - 若分析过程中对字段、结论依据、定位结果存在疑惑，必须结合分析版中的 `file_id` 与顶层 `files` 映射回查原始发票清单文件。
  - 不允许跳过该步骤直接基于发票清单目录做不完整抽样分析，也不允许在存疑时跳过源文件回查直接猜测。

任务文件驱动示例：

```bash
.\bin\windows-amd64\invoice-assistant_windows_amd64.exe build-invoice-context-data \
  --task-file "<workspace>/invoice-fetch-and-risk-analysis/tasks/<任务文件名>.json"
```

说明：转换结果必须覆盖目录下全部清单文件；分析版用于本次风险分析直接输入，并通过 `file_id` 与顶层 `files` 映射回溯到源文件。若会话内已存在同源分析版数据，可直接复用而无需重复转换。

## 与提示词文件的绑定规则

- 提示词文件路径固定：`references/发票风险分析提示词.md`。
- 本场景中，该提示词文件优先级高于通用分析模板。
- 若场景文件与提示词文件存在冲突，以提示词文件为准。

## 中间反馈模板（可选）

### 参数确认

```text
已进入“发票风险分析”场景
输入来源：{single_or_batch}
纳税人类型：{taxpayer_type}
行业类型：{industry_type}
```

### 处理中反馈

```text
正在执行发票风险分析...
已完成：{completed_count}/{total_count}
当前文件：{current_file}
```

## 交付要求

### 用户可见结果

- 输出内容：风险结论、关键风险点、风险等级、分角色行动建议、边界说明。
- 输出形式：文本分析报告 + 本地 PDF 文件 + 可选结构化字段；PDF 转换失败时才以本地 Markdown 文件作为兜底交付物。
- 可追溯性：可定位到 source_files、row_index、direction_tags、warnings。

### 成功模板

```text
发票风险分析已完成

综合风险等级：{risk_level}
一句话总结：{one_line_summary}
最重要问题：
{top_findings}

分角色行动清单：
{role_actions}
```

### 失败模板

```text
发票风险分析未完成：{error_message}
影响范围：{impact_scope}
建议：检查输入文件可读性、关键字段完整性后重试。
```

### 部分成功模板

```text
发票风险分析部分完成

已完成：{done_part}
未完成：{undone_part}
原因：{partial_reason}
建议：补充缺失数据后重新分析。
```

## 质量标准

- 完整性：输出必须覆盖提示词规定的全部关键章节。
- 准确性：一般纳税人与小规模分析逻辑明显区分。
- 可解释性：先结论后原因，给出可执行下一步动作。
- 可行动性：每个风险点均包含建议动作。
- 可读性：脚本内部英文标识均已翻译为通俗中文，必须保留的源业务英文首次出现时附有中文说明。
