# 报告生成工作流


> 当前交付约束见[输出规则](report-output-v4.md)，报告规则 `{{REPORT_RULE_VERSION}}`，AI契约 `{{AI_CONTRACT_VERSION}}`。字段及来源以[数据契约](report-data-contract.md)为准。

## 三阶段命令

企业及期间按[参数规则](periods.md)确定。用户已明确企业且绑定列表唯一精确匹配时直接继续，不重复确认；未指定或歧义时才展示列表选择。

### 1. 准备可信工作区

```text
tax-analysis report-prepare (--company <完整企业名称> | --uscc <uscc>) --start-month YYYY-MM --end-month YYYY-MM --api-key <key> --workspace <dir> [--force-refresh] [--pretty]
```

企业名称或 USCC 必须在当前 API Key 的绑定企业列表中精确匹配。命令完成数据准备后生成以下工作区文件：

- `manifest.json`：任务、企业、完整日期、`{{REPORT_RULE_VERSION}}` 规则版本、`{{AI_CONTRACT_VERSION}}` 契约版本、章节集合和输入摘要。
- `source-snapshot.json`：17 模块任务快照，仅供事实追溯。
- `report-model.json`：脚本计算的章节、事实、表格、图表、风险结果和证据引用。
- `analysis-data.json`：冻结指标底稿；`analysis-sections/<semanticId>.json`为完整分节阅读卡，供追溯和特殊核查。日常分析读取`report-read`提供的精简批次，不反复加载这些文件。未舍入输入仅保留在完整底稿；阅读值使用display。guidance是对应章节的分析方向，不是事实或新增必填项；仅需写分析的章节才提供。
- `analysis-task.md`：分析任务清单、可用引用及通用分析指引。
- `analysis-output.json`：脚本合并AI正文的固定结构（text正文与evidenceRefs来源分开）。
- `analysis-batches.json`、`analysis-batches/*.md`：批次索引和阅读材料，包含完整章节及月季比较序列。
- `prepare-result.json`：准备结果、批次数及取数/模型/写工作区耗时（毫秒）。

### 2. 填写并校验 AI 分析

Agent先读`analysis-task.md`，再`report-read --workspace <dir> --next`逐批读取、提交。额外create不是报告前置步骤；已prepare后不再调用接口重新取数。

```text
tax-analysis report-read --workspace <dir> --next [--pretty]
tax-analysis report-read --workspace <dir> --batch batch-02 [--pretty]
tax-analysis report-read --workspace <dir> --section vat.burden-trend [--pretty]
tax-analysis report-refs --workspace <dir> --section vat.burden-trend --name 税负率 [--period <完整期间>] [--base-period <完整基期>] [--subject <主体>] [--id <准确引用>] [--offset 0] [--limit 50]
tax-analysis report-write --workspace <dir> --input <本批正文.json> [--pretty]
```

read/refs/write只访问本地工作区。名称和主体为包含匹配，ID、期间及基期精确匹配；不确定期间格式时先按名称查，再复制返回的期间。所有候选均可分页获取，返回total及nextOffset；不会自动选首个同名指标。缺失或冲突指标返回status且不返回display，不能作为有效引用。

本批提交示例（占位内容应替换为read返回的真实摘要和引用）：

```json
{
  "analysisDataDigest": "sha256:从report-read复制",
  "sections": [
    {"sectionId": "vat.burden-trend", "items": [
      {"text": "依据本批真实指标撰写的分析。", "evidenceRefs": ["从本批复制准确ID"]}
    ]}
  ]
}
```

工作区已有其他章节会保留；重交同节时完整替换该节items。JSON、摘要、章节或引用不通过时原文件不变。使用上述命令提交，无需另写抽取或写入脚本；断点续写直接read --next。

全部章节完成后next返回综合结论所需已确认风险、数据限制及关键事实。提交`{"analysisDataDigest":"实际摘要","conclusion":{"items":[...]}}`；正文内容约定：

- conclusion.items：3—5条重点。首条总体风险，随后1—3项优先风险与对应动作，并保留最重要的数据限制；不足三条时只补充必要的期间或企业事实，不能凑写正常结论。
- `sectionOrder` 是工作区完整 `analysis-output.json` 中的章节顺序，由准备命令提供；不放入本批提交 JSON，也不自行改写。提交只接受 `analysisDataDigest`、`sections`、`conclusion`。
- `sections`：每个要求分析的章节恰好一项；按冻结模型中的 `narrative.minItems/maxItems` 填写 `items[].text/evidenceRefs`，不填写通用建议。
- text直接写自然语言，金额、比例、期间等依据脚本计算结果自然表达；evidenceRefs单独记录使用的来源，不在正文写占位符。
- 引用数量由实际分析需要决定，不凑数、不强制复述全部证据；objectiveIds仅为可选定位信息。
- 不得新增或隐藏章节、风险、关系、金额、阈值和等级；不得写接口英文键名或内部字段路径。
- 先读V4规则，按脚本六种主状态填写。事实说明0—2条可为空，关注事项1—3条，风险分析及对应整改各1—3条；缺数、不适用、未见异常使用脚本固定文案。风险概览不重复读图，综合结论3—5条。分析应简洁易懂、重点前置，实际使用的来源记入evidenceRefs。

完成后执行：

```text
tax-analysis report-validate --workspace <dir> [--pretty]
```

技术校验不核实正文含义，不按字数、关键词或引用在正文的位置判断质量。失败结果明确返回 `missingPaths`、`invalidReferences` 和 `contentErrors`。条目字段问题还返回issues，包含sectionId、从0开始的itemIndex、field、code及referenceId。Agent 按具体位置修正后继续重试，校验不限制修正次数；未通过时不生成 DOCX/PDF，也不回退到脚本拼接结论。`report-write.valid=true` 只表示本批提交通过；整份报告是否完成还须看 `progress` 并执行 `report-validate`。全部返回字段见[AI 读写字段](report-data-contract.md#ai-读写字段)。

### 3. 渲染最终报告

```text
tax-analysis report-render --workspace <dir> --output-dir <dir> [--pretty]
tax-analysis report --workspace <dir> --output-dir <dir> [--pretty]
```

`report` 是 `report-render` 的别名。企业、期间、源数据或正文在校验后发生变化时必须重新校验。

成功退出码为 `0`，且必须同时生成：

```text
<企业名称>_财税风险分析报告_<YYYYMMDD>.docx
<企业名称>_财税风险分析报告_<YYYYMMDD>.pdf
```

企业名称缺失时使用 USCC。文件名过滤Windows/Linux非法字符，已有同名文件时追加`(2)`、`(3)`，不覆盖旧文件。

成功后查看`render-result.json`，获取成品路径、SHA-256、版本及耗时，不为读取结果重复运行渲染命令。`reused=true`表示复用了已有有效报告。文件不存在或只存在一种格式不视为成功；耗时字段以毫秒计，不包含正文写作时间。

## 数据门禁

1. `report-prepare` 执行现有 create/query 轮询流程。报告期间不得早于 `2017-01`，首尾月份均包含且最长 36 个自然月；创建失败时按接口契约排查并保留原期间，不以缩短期间替代原报告。
2. 任务中的17项数据原则上必须全部 `success` 且没有失败明细。
3. `partial_success/failed` 复用同一任务最多补查两次；补查使用 `forceRefresh=false`。
4. 两次补查后仅允许 `arrears_and_refunds` 终态失败、房产税/城镇土地使用税精确空税源、小规模纳税人抵扣功能明确不适用三类降级；任务保留 `partial_success`，模型写入中文警告。未完整取得的数据不得解释为无异常。
5. 空数据但模块状态成功不是失败。其他模块缺失、失败、不支持或仍有失败明细时，准备命令退出码为 `1`。

## 数据与 AI 边界

- 纳税人资格读取 `taxpayerInfo/identifyInfo`，明确 `nsrzg/Nsrzg` 严格使用 `0=一般纳税人、1=小规模纳税人`。来源冲突保留可理解警告，以明确税局资格字段优先。
- 税费分析以各税种 `effectiveRecords` 与申报记录模块 `declaredRecords` 的实际已申报记录为主，税费种认定为补充；`undeclaredRecords/todoRecords` 不进入已申报统计。月报、季报、年报、按日和不定期记录都保留实际所属期；季度和不定期记录按结束月份进入月度矩阵，缺失月份显示“—”。
- 缴款优先按税款所属期归集；仅有缴款日期时明确标记为缴款日期口径。
- 分析使用返回的金额、比例、风险等级和输出状态，填写正文及综合结论；正文与来源引用分开，不自行重算或改变风险结果。
- 两种格式的标题、正文、表格、风险和图表数据必须一致。

## 报告版式

- 使用本次企业名称、报告期间和生成日期，名称完整显示，不残留示例内容。
- 两种格式均使用A4页面，目录可跳转且页码准确，页眉、页脚、标题、表格和图表样式一致。
- 版式及章节显示条件见[输出规则](report-output-v4.md)和[章节映射](word-report-contract.md)。字体、封面等资源缺失时按安装文档补齐并校验。

## 成对提交

DOCX和PDF均成功生成后才交付；任一格式失败时按错误提示处理并重新成对生成。两种格式允许分页略有差异，但可见标题、文字、数值、风险、表格和图表数据必须一致。

成功 stdout JSON：

```json
{
  "status": "success",
  "taskId": "...",
  "dataStatus": "success|partial_success",
  "ruleVersion": "{{REPORT_RULE_VERSION}}",
  "docxPath": "...",
  "pdfPath": "...",
  "includedSections": ["1", "1.1"],
  "omittedSections": ["3.6"],
  "warnings": []
}
```

`includedSections/omittedSections` 返回[章节映射](word-report-contract.md)中的章节代码；报告正文和目录中的显示编号可能因省略不适用章节而变化。

## 逐节量化与来源记录

按[逐节分析约定](report-analysis.md)使用本批指标及分析目标：text填写自然语言，evidenceRefs独立保存来源ID。提交时复制report-read返回的analysisDataDigest；技术校验通过后渲染，不另设语义审读文件。底稿与引用不进入客户文档。
