# 取票与分析操作示例

按 [workflow.md](workflow.md) 完成预检和参数确认后执行下列步骤。命令参数见 [api-contracts.md](api-contracts.md)，输入字段见 [field-mapping.md](field-mapping.md)。

## 确认范围

- 使用本次用户明确选择的企业税号及日期范围，不使用上次企业或历史产物。
- 明确是只获取、获取+统计还是获取+分析，并单独确认是否下载发票原件/附件。
- 发票清单：每次取票成功后必生成，不作为可选参数。
- 主取票不询问或收集发票方向、票种和状态筛选；取得数据后再按真实信息进行统计。

## 创建和取票

将已确认的输入写入 UTF-8 JSON 文件，使用当前平台 `invoice-assistant` 执行：

```text
init-task --input <input.json>
fetch-invoices --task-file <本次任务.json>
```

用户确认需要附件时再执行 `fetch-attachments --task-file <本次任务.json>`。取票完成后执行 `summarize --task-file <本次任务.json>`，完整展示摘要后再继续。

命令仍在处理、超时或中断时，保留原任务文件并再次执行同一命令续查；不要删除任务号、新建任务或默认添加 `--confirm-duplicate`。只有用户明确放弃续查并重新发起时才使用该参数。提交结果未知时先核实，不自动重复创建。

## 勾选与清单

```text
fetch-deduction --task-file <本次任务.json>
generate-invoice-list --task-file <本次任务.json>
```

所有取票任务都要执行 `fetch-deduction`。一般纳税人须得到 `success`，其他纳税人须得到明确 `skipped` 后才生成清单；不能用独立企业查询代替。若重新获取勾选数据，后续清单、统计和风险报告均要重新生成。

## 统计和风险分析

需要统计或风险分析时执行 `analyze-invoice-data --task-file <本次任务.json>`，交付 `outputs.analysis_excel_path` 对应的 `发票统计分析.xlsx`。

需要风险分析时继续：

```text
build-invoice-context-data --task-file <本次任务.json>
```

用 `outputs.risk_context_summary` 及企业画像生成报告，仅在核对具体证据时读取全量分析资料。按 [invoice-risk-analysis.md](invoice-risk-analysis.md) 完成中文正文后，保存同一版 Markdown 并转换：

```text
markdown-to-pdf --input <报告.md> --output <报告.pdf> --task-file <本次任务.json>
```

PDF 成功后从 stdout 的 `report_body`、`analysis_excel_path`、`report_pdf_path`、`artifact_root`，或任务的 `outputs.risk_report_body`、`outputs.analysis_excel_path`、`outputs.risk_report_pdf_path` 读取交付内容。完整展示同一版报告正文，同时交付 Excel 与 PDF；不得另写摘要 Markdown 代替正文。

本地路径文字必须完整显示，包含盘符、父目录和文件名。链接不是必需；使用链接时，链接显示文本必须是同一条完整绝对路径。展示前将 Windows 反斜杠转换为正斜杠，例如 `MEDIA:C:/Users/<用户名>/.hidden-workspace/.../发票风险分析报告.pdf`，不改变实际文件路径。

## 失败处理

预检、权限或取票失败时停止依赖步骤，原样展示实际原因。部分附件或资料缺失须说明覆盖范围。只有真正的 PDF 技术故障重试后仍失败，才保留并完整展示原 Markdown 正文及路径，明确 PDF 未生成；未翻译的程序字段不适用此兜底，应改写正文并重新生成 PDF。
