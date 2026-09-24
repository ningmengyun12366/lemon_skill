# 税务分析接口契约

> 当前交付约束见[输出规则](report-output-v4.md)，报告规则 `{{REPORT_RULE_VERSION}}`，AI契约 `{{AI_CONTRACT_VERSION}}`。字段及来源以[数据契约](report-data-contract.md)为准。

当前响应格式为 `tax-analysis.v2`，各类数据的含义见[结构化数据说明](structured-data.md)。分析使用已准备工作区中的可信数据。

## 公开路径

- `POST /api/v1/skill/tax-analysis/create-task`
- `POST /api/v1/skill/tax-analysis/query-task`

`run/create/report-prepare` 会确认目标企业已绑定且登录就绪。需要人工登录时，按 CLI 返回的登录网页入口完成操作，再重试原命令。`query-task` 只读取已有任务，不发起登录，但仍须确认目标企业属于当前 API Key 的绑定企业。

请求头：

```text
Authorization: apikey=<key>; uscc=<uscc>; capabilityCode=tax_analysis
Content-Type: application/json
```

API Key 只能放在请求头，不得写入请求体、日志或回答。

所有响应固定为：

```json
{"code": 0, "message": "成功", "data": {}}
```

`code=0` 表示本次 create/query 调用成功。常见错误码：`2005` 参数错误、`2001` 认证失败、`2003` 归属或权限不匹配、`1301` 任务不存在；其他错误为 `3997`。任务自身的 `partial_success/failed/interrupted` 位于 `data.status`，query 读取成功时外层仍为 `code=0`。

出现 `3997` 时，不能仅凭错误文案认定为税局错误或月份限制。保留原始期间、错误和响应中可用的 `traceId` 供问题反馈。create 失败时，不得根据旧 taskId 或旧结果声称新一轮采集已启动。短期间创建成功只证明该次创建被受理，不证明采集完成。保持用户原定期间，原因未解决前不连续试建其他期间；仅在用户明确改变报告范围后调整期间。

## create-task

```json
{
  "uscc": "统一社会信用代码",
  "startMonth": "2025-01",
  "endMonth": "2025-12",
  "forceRefresh": false
}
```

- 起始月份不得早于 `2017-01`；起止月份均包含，最长 36 个自然月。例如 `2017-01` 至 `2019-12` 允许，`2016-12` 起始或 `2017-01` 至 `2020-01` 不允许。
- 任务响应中的 `startTime/endTime` 分别为首月第一天和末月最后一天。
- 无公共分页参数。
- 同一范围再次发起时，以响应返回的 `taskId` 为准，不自行构造任务标识。
- 普通再次发起保留成功结果，只重试失败模块或失败明细。
- `forceRefresh=true` 仅在终态增加 `generation` 并刷新全部已支持模块。

## query-task

```json
{
  "uscc": "统一社会信用代码",
  "taskId": "<create-task 返回值>"
}
```

任务完成后的查询结果保留 1 小时，普通查询不会延长该期限。请及时保存结果或工作区；过期后需重新创建任务。

任务中断时查看 `executionError.retryable`。可恢复时继续查询等待；不可恢复时保留任务标识、已取得结果和原因，按错误提示处理，不循环创建任务掩盖失败。

## 任务响应

以下路径相对于响应的 `data` 对象。字段名用于读取真实响应，展示给报告读者时使用对应的中文业务名称。

| 字段 | 类型 | 含义及使用方式 |
| --- | --- | --- |
| `dataSchemaVersion` | 字符串 | 当前为 `tax-analysis.v2`，用于确认响应格式。 |
| `taxDocuments` | 对象 | 文档ID到税表对象的映射；模块中的 `{"$taxDocument":"文档ID"}` 引用这里的对象，结构见[税表字段说明](structured-data.md)。 |
| `taskId` | 字符串 | 任务标识，后续查询使用返回原值，不自行构造。 |
| `generation` | 整数 | 首次为1；强刷终态任务时递增，普通失败续查及中断恢复不递增。 |
| `status` | 字符串 | 任务整体状态，取值见下表；不同于外层 `code` 及各模块的状态。 |
| `uscc` | 字符串 | 当前任务对应企业的统一社会信用代码。 |
| `startTime`、`endTime` | 日期字符串 | 分析范围的起止日期，分别为首月第一天和末月最后一天；不等同于申报日期或缴款日期。 |
| `moduleResults` | 数组 | 17类数据的模块结果；每项按 `code` 识别，不能靠数组位置或中文名称猜测。 |
| `failedItems` | 数组 | 任务级失败明细；使用 `moduleCode` 定位所属模块。已有部分数据不代表失败范围已补齐。 |
| `completedCount` | 整数 | 已完成的模块数。 |
| `failedCount` | 整数 | 失败的模块数。 |
| `notSupportedCount` | 整数 | 不支持的模块数。 |
| `totalCount` | 整数 | 任务模块总数。这四项均不是业务记录条数或风险数量。 |
| `createdAt`、`updatedAt` | 时间字符串 | 任务创建及更新时间，不作为税款所属期或事件发生时间。 |
| `completedAt` | 时间字符串，可缺省 | 任务完成时间，尚未完成时不能据缺失值推断失败。 |
| `executionError` | 对象，可缺省 | 中断原因和可恢复性，字段见下表。 |

任务状态：

| `status` | 含义 | 下一步 |
| --- | --- | --- |
| `pending` | 等待执行 | 继续查询同一任务。 |
| `running` | 执行中 | 继续查询同一任务。 |
| `success` | 任务成功 | 结合模块状态、失败明细及具体字段判断数据覆盖。 |
| `partial_success` | 部分数据成功 | 保留成功结果，按限定补查及降级规则处理。 |
| `failed` | 任务失败 | 保留实际错误及可用部分结果，不按成功空记录处理。 |
| `interrupted` | 执行中断 | 查看 `executionError.retryable`；缺少恢复信息时不能假定可以恢复。 |

`moduleResults[]`：

| 字段 | 类型 | 含义及使用方式 |
| --- | --- | --- |
| `code` | 字符串 | 模块标识，与[模块字段说明](fields.md)对应。 |
| `name` | 字符串 | 模块中文名称。 |
| `status` | 字符串 | 当前模块状态，取值见下方。 |
| `data` | JSON值，可缺省 | 模块业务数据，嵌套结构因模块而异。缺失或 `null` 不表示零金额、零条记录或没有异常。 |
| `error` | 字符串，可缺省 | 模块错误原因。 |
| `stale` | 布尔值 | 为 `true` 时，当前 `data` 是此前可用结果，不能视作本次刷新成功。 |
| `refreshError` | 字符串，可缺省 | 刷新或失败续查未成功的原因，与 `stale` 一起解释。 |
| `failedItems` | 数组 | 本模块失败明细；有返回数据时也可能存在未成功部分。 |
| `updatedAt` | 时间字符串，可缺省 | 模块结果更新时间，不代替业务记录所属期。 |

模块 `status`：

- `pending`：等待当前 generation 执行。
- `running`：正在查询。
- `success`：该模块全部完成。
- `failed`：模块或部分明细失败；可能仍有可用 `data`。
- `not_supported`：该模块不受支持，不等同于查询成功但无记录。

当刷新或失败续查未成功时，旧数据会保留；`stale=true` 表示当前 `data` 来自上一次可用结果，失败原因在 `refreshError`。

任务级及模块级 `failedItems[]` 使用同一明细结构：

| 字段 | 类型 | 含义及使用方式 |
| --- | --- | --- |
| `moduleCode` | 字符串，可缺省 | 所属模块；模块内明细可由外层模块确定，任务级明细用此字段定位。 |
| `itemId` | 字符串 | 失败明细标识，保留原值，不拆分后推定业务事实。 |
| `message` | 字符串 | 该明细的失败原因。 |
| `errorCode` | 整数，可缺省 | 明细错误码，不等同于外层响应 `code`。 |
| `context` | 对象，可缺省 | 返回的定位信息，具体字段随失败项而异；只使用实际返回的键和值。 |

`executionError`：

| 字段 | 类型 | 含义及使用方式 |
| --- | --- | --- |
| `code` | 字符串 | 中断原因代码；不同于外层整数业务码。 |
| `message` | 字符串 | 可读的中断原因。 |
| `occurredAt` | 时间字符串 | 中断发生时间。 |
| `moduleCode` | 字符串，可缺省 | 与中断有关的模块标识。 |
| `retryable` | 布尔值 | 是否仍可自动恢复；`true` 不表示已经恢复，也不保证后续成功。 |

数组为空或 `null` 只能说明该字段当前未给出条目。判断“查询成功但无记录”还须结合对应状态和覆盖证据；不同字段中的 `code`、`status` 不可混用。

## CLI

先列出当前 API Key 的全部绑定企业：

```text
tax-analysis enterprises --api-key <key> [--pretty]
```

stdout 只返回供用户选择的 `enterpriseName/uscc`。Agent 必须完整展示列表并等待用户明确选择；该命令不检查电子税局登录状态、不调用 `create-task/query-task`。用户选择后，后续命令还会重新查询绑定列表并精确校验。

准备可信分析工作区：

```text
tax-analysis report-prepare (--company <完整企业名称> | --uscc <uscc>) --start-month YYYY-MM --end-month YYYY-MM --api-key <key> --workspace <dir> [--force-refresh] [--pretty]
```

`report-prepare` 首次运行后若数据不完整，会复用同一任务补查两次，补查不自动 `forceRefresh`。补查后只允许三类明确降级：`arrears_and_refunds` 终态失败、房产税/城镇土地使用税精确空税源、小规模纳税人抵扣功能明确不适用；其余失败仍阻断工作区并输出中文错误。

`interrupted` 不按业务失败补查或空数据处理。`retryable=true` 时 `run/report-prepare` 继续查询等待恢复，不重复 create；明确不可恢复或响应缺少恢复信息时停止并保留原 taskId、部分结果与原因，不生成完整报告。`create/query` 即时输出当前结果：可恢复中断退出码为 0，不可恢复中断为 1；`run` 返回不可恢复中断时退出码为 1。

读取本批写作材料、按需查询引用并提交正文：

```text
tax-analysis report-read --workspace <dir> --next [--pretty]
tax-analysis report-read --workspace <dir> --batch <batchId> [--pretty]
tax-analysis report-read --workspace <dir> --section <sectionId> [--pretty]
tax-analysis report-refs --workspace <dir> [--section <sectionId>] [--name <指标名>] [--subject <主体>] [--period <完整期间>] [--base-period <完整基期>] [--id <引用ID>] [--offset 0] [--limit 50] [--pretty]
tax-analysis report-write --workspace <dir> --input <本批正文.json> [--pretty]
```

这些命令只读取或更新本地工作区。`report-read` 的 `content` 是 Markdown 阅读材料，`analysisDataDigest` 须原样带入提交；根据 `progress.pendingSectionIds/conclusionPending` 判断剩余工作。`report-refs` 的名称和主体为包含匹配，章节、ID、期间和基期为精确匹配；`offset` 不小于0，`limit` 为1—200。多项匹配须核对主体和期间，有 `nextOffset` 时可继续分页。只有 `matches[].status=present` 的引用可作为有效事实；命令整体 `status=success` 不代表每条指标可用。

`report-write` 输入顶层只接受 `analysisDataDigest`、`sections`、`conclusion`，至少提交章节或结论之一。具体字段、单位、状态及返回结构见[AI 读写字段](report-data-contract.md#ai-读写字段)，填写示例见[报告工作流](report.md)。本批写入成功后继续 `report-read --next`；完成全部章节和结论后执行完整校验。

校验 Agent 分析并渲染同内容 DOCX/PDF：

```text
tax-analysis report-validate --workspace <dir> [--pretty]
tax-analysis report-render --workspace <dir> --output-dir <dir> [--pretty]
tax-analysis report --workspace <dir> --output-dir <dir> [--pretty]
```

`report` 是 `report-render` 的别名。两条渲染命令都只能读取已准备、已填写并通过校验的工作区，不重新采集数据。

完整运行：

```text
tax-analysis run (--company <完整企业名称> | --uscc <uscc>) --start-month YYYY-MM --end-month YYYY-MM --api-key <key> [--force-refresh] [--pretty]
```

只发起：

```text
tax-analysis create (--company <完整企业名称> | --uscc <uscc>) --start-month YYYY-MM --end-month YYYY-MM --api-key <key>
```

只查询：

```text
tax-analysis query (--company <完整企业名称> | --uscc <uscc>) --task-id <taskId> --api-key <key>
```

`--company` 与 `--uscc` 二选一。两种输入都会先请求绑定企业列表，并只使用匹配记录中服务端返回的 USCC；名称必须完全一致，同名多条时必须改用 USCC，未绑定企业直接拒绝。`run` 永远先调用 create，再使用返回的 `taskId` 轮询 query。`partial_success` 是有部分可用数据的终态，不应丢弃成功模块。

## 工作区版本

若命令提示工作区版本不匹配，应重新执行 `report-prepare`。不要手工改写版本或摘要以继续生成报告。
