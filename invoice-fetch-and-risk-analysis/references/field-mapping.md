# 发票获取及风险分析字段映射

本文件说明用户自然语言如何落位到任务输入 JSON、任务 JSON 和 CLI 参数。字段缺失、冲突或含糊时，先一次性追问关键项，不用历史取票结果替代本轮输入。

## 根输入结构

`init-task --input <input.json>` 的输入应是单个 JSON 对象，根对象必须包含非空 `params` 对象；否则 Go 端会返回 `missing required non-empty params object`。本次意图参数全部放在 `params` 内：

```json
{
  "params": {
    "uscc": "企业税号",
    "invoiceStartTime": "2026-01-01",
    "invoiceEndTime": "2026-03-31",
    "outputRequirement": "只获取",
    "needAttachments": false
  }
}
```

- `uscc`、`invoiceStartTime`、`invoiceEndTime`、`outputRequirement` 和 `needAttachments` 是创建任务前的关键字段。即使 API Key 只绑定一家企业，`uscc` 也不能省略。
- `areaCode`、`personalAccount`、`loginType` 由命令处理；Agent 不要求用户补填，也不使用旧任务中的这些值。
- 后续命令统一使用本次 `--task-file <task.json>`，不手工改写任务企业信息。

## 企业与账号字段

| 用户说法 | 落位字段 | 处理规则 |
| --- | --- | --- |
| 税号、统一社会信用代码、纳税人识别号 | `uscc` | 保留原值；明显缺失或格式不完整时追问。 |
| 深圳、广东、地区编码 | `enterprise_context.areaCode` | 不作为 Agent 输入；由本次任务提供，不从地名或历史信息猜测。 |
| 个人账号、手机号、办税员账号 | `enterprise_context.personalAccount` | 不作为 Agent 输入；由服务端绑定记录提供，输出时不得展示完整敏感账号。 |
| 企业名称 | `enterpriseName` | 可用于 `task-preflight-check --enterprise-name`；不能替代 `uscc` 完成风险画像检索。 |

## 日期范围

- “今年”“本季度”“上个月”等相对日期必须换算为明确起止日期后回显。
- 开始日期写入 `invoiceStartTime`，结束日期写入 `invoiceEndTime`。
- 日期格式使用 `yyyy-MM-dd`。
- 开始日期晚于结束日期时必须追问修正。
- 时间间隔超过 2 年时，提示数据量可能导致获取时间过长，取得用户二次确认后继续。
- 日期范围缺失时不能启动 `init-task`。

## 输出要求

| 用户说法 | `outputRequirement` | 后续步骤 |
| --- | --- | --- |
| 获取、拉取、同步、查询发票 | `只获取` | 取票、摘要、勾选分支、发票清单。 |
| 统计、汇总、趋势、占比 | `获取+统计` | 在取票和清单后执行 `analyze-invoice-data`。 |
| 分析、风险分析、风险扫描、税务风险 | `获取+分析` | 统计、企业画像、结构化数据、风险报告和 PDF。 |

- 同一句同时出现“只获取”和“风险分析”等冲突意图时，必须追问确认。
- 用户语义已经明确命中时，不再重复询问三选一，只在参数摘要中回显归类结果。
- 发票清单不属于可选输出参数；`只获取`、`获取+统计`、`获取+分析` 均必须在取票成功后生成发票清单。不得询问用户是否生成清单，也不得在取票完成后把生成清单作为可选后续操作。

## 附件需求

- “下载发票”“下载发票原件”“下载附件”“带附件”“发票附件一起取”“PDF/OFD 一起下载”映射为 `needAttachments: true`。
- “只取数据”“不要附件”“不下载发票原件”映射为 `needAttachments: false`。
- 新任务中用户未明确说明是否下载发票原件/附件时，必须在参数确认前追问；不得静默默认 `needAttachments: false`。
- 附件远程链接仅内部下载使用，最终输出必须转成本地附件目录或本地文件结果。

## 取票范围

- 主取票任务固定使用服务端当前支持的整批同步范围。
- 参数确认阶段不得询问发票方向、票种或状态，不得创建相应筛选字段。
- 用户主动提出上述筛选条件时，明确说明当前取票任务不支持按这些条件筛选，不得承诺筛选后再创建任务。
- 取回后的统计或风险分析可以根据实际数据识别方向、票种和状态；该分析分类不属于取票参数。

## 企业画像字段

风险分析需要企业画像：

- `taxpayer_type`：只允许 `一般纳税人`、`小规模纳税人`、`待人工核验`。
- `industry_type`：行业或经营范围口径。
- `business_scope`：可作为行业补充信息。

处理规则：

1. 已有任务时必须执行 `fetch-deduction --task-file <task.json>`；读取本次返回的 `artifacts.enterprise_profile`，不使用其他企业或历史查询结果替代。
2. `query-enterprise-profile-by-uscc --uscc <税号>` 只用于不存在任务文件的独立查询，返回纳税人类型和经营范围，不得在已有任务流程中替代 `fetch-deduction`。
3. 查询失败、无结果或字段不完整时，才向用户追问纳税人类型、行业或经营范围。
4. 所有取票任务均执行 `fetch-deduction`；一般纳税人须取得勾选结果，其他纳税人须取得明确 `skipped`。小规模纳税人不得套用一般纳税人的抵扣判断。

## Windows UTF-8 JSON 输入

- Windows 下包含中文的输入优先写入 UTF-8 JSON 文件，再通过 `--input <file>.json` 调用 `init-task`。
- PowerShell 中优先用对象加 `ConvertTo-Json` 生成 JSON，再用 .NET UTF-8 无 BOM 写入。
- 不优先使用 stdin 管道直传复杂 JSON。
- 控制台显示乱码时，先用 `Get-Content -Raw -Encoding UTF8 <input.json> | ConvertFrom-Json` 做 round-trip 验证。
- `\uXXXX` 是合法 JSON 转义，不等于编码损坏。

## 缺失与冲突处理

- 缺少必填字段时，一次性列出缺失项和可选项，不创建任务 JSON。
- 参数冲突时，说明冲突字段并要求用户选择一个口径。
- 用户要求风险分析但拒绝提供或无法自动取得企业画像时，停止风险分析，可继续交付已完成的取票或统计结果。
- 不用旧任务 JSON、旧 Excel、旧附件目录或旧报告自动补齐本轮字段。
