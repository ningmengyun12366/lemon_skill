# 17 类税务分析任务模块字段说明

当前响应格式为 `tax-analysis.v2`，详见[结构化数据说明](structured-data.md)。以下说明用于理解查询结果的业务范围和限制。

## 通用字段

- `recordHistory`：查询范围内全部申报版本，包含更正和作废历史。
- `effectiveRecords`：按税种、申报表和所属期分组后选出的最新非作废版本。
- 当前期申报采用完整包含：`skssqq >= startTime AND skssqz <= endTime`。VAT及企税季报的`yearToDateReference`另存年初至请求前的参考记录，仅供累计/差分计算，不进入当前期税额汇总。
- `failedItems.itemId`：失败明细标识，问题反馈时保留返回原值。
- `data` 是取数结果，不等同于税务结论。

## 1 纳税人档案 `taxpayer_profile`

`taxpayerInfo` 包含当前纳税人基础、登记、资格、主管机关等信息；`identifyInfo.taxIdentifyList` 包含税种、税目、申报期限和纳税期限；`identifyInfo.taxFinancialList` 包含财务会计制度备案。它是当前档案，不按历史月份切片。

## 2 应申报清册 `filing_obligations`

- `declaredRecords`：税局申报记录。
- `undeclaredRecords`：税局未申报查询接口返回。
- `todoRecords`：税局待办返回。
- `confirmedExceptions`：只有税局状态明确为未申报或逾期的项目。

缺少一张已申报表不能单独证明漏报。

## 3 增值税申报 `vat_returns`

从实际申报记录筛选征收项目代码 `10101`，月报或季报以税局实际记录为准。`details` 为主表/附表预览结构，可用于销售额、未开票、销项、进项、转出、留抵和应纳税额字段提取。

## 4 企业所得税预缴 `cit_quarterly_returns`

按季度预缴表代码或表名确定表型。收入、成本、利润等累计值不得直接当成单季度发生额；“本期应补（退）所得税额”及最终“实际应补（退）所得税额”属于本期税额，不能再按季度差分，不能替换为“应纳所得税额”或缴款数据。

## 5 企业所得税年度汇算 `cit_annual_returns`

返回查询范围内完整年度记录及子表，可能包含收入成本费用、纳税调整、优惠和研发加计扣除申报字段。

## 6 资产负债表 `balance_sheets`

`records[].previews` 只包含文件标签为资产负债表的税务报送财报预览。它不是企业账套，通常可解释货币资金、应收、预付、存货、固定资产、负债和所有者权益等报表项目。

## 7 利润表 `income_statements`

`records[].previews`保留利润表、现金流量表、民非业务活动表，不含费用凭证明细。民非007“收入合计”按“本年累计数→合计”取数，不能取本月数或非限定性列。资产负债表另由balance_sheets保留。

## 8 印花税 `stamp_tax`

先返回印花税申报历史和详情，再在 `taxSources` 中提供与申报关联的只读税源结果。合同或凭证未在税局数据中出现时，不得直接认定漏缴。

## 9 房产税 `property_tax`

按征收项目代码 `10110` 识别房产税申报。`sourceList` 是税源查询结果对象，包含 `code/message/status/data`；`taxableDetails` 是应税详情数组，申报详情中的 `syuuid` 用于关联税源和应税信息。可提供房产坐落、原值/租金、计税比例、减免和税额字段；装修是否计入原值需要另行核对资料。空数组须结合查询状态理解，不能直接认定没有房产。

## 10 城镇土地使用税 `urban_land_use_tax`

按征收项目代码 `10112` 识别土地使用税申报。`sourceList` 保存税源查询结果，`taxableDetails` 保存应税详情，`syuuid` 用于关联土地税源。常见字段包括 `ystdmj`（应税土地面积）、`dwse`（单位税额）、`tddjDm/tddjmc`（土地等级）和减免信息。单位税额与面积不可互代；查询失败或详情缺失不等同于无应税土地。

## 11 缴款记录 `payments`

`list` 保留 `zsxmDm/zsxmMc`、`zspmDm/zspmMc`、`skssqq/skssqz`、`skzlDm/skzlMc`、`sjje`、`jkrq` 和缴款方式等税局字段。

`paymentTypeName` 映射：`10 正税`、`20 滞纳金`、`31 涉税罚款`、`32 行为罚款`、`40 没收违法所得`、`50 利息`、`60 违约金`、`90 其他`。仅 `skzlDm=20` 时，`lateFeeAmount=sjje`。

## 12 欠税/退税 `arrears_and_refunds`

`arrears.arrearsInfo` 表示当前查询取得的欠税信息，不据此还原历史月份余额；`arrears.collectionNotice.chunks` 为催缴欠税记录，应按各记录所属期间理解。

`refunds` 包含一站式退税、增值税留抵退税（`institutional` 制度性、`periodEnd` 期末）、石脑油燃料油消费税退税、车辆购置税退税、留抵退税缴回、留抵抵欠、即征即退缴回。单个来源包含 `status/message/total/data`：`EMPTY` 是查询成功但无数据，`NOT_APPLICABLE` 是企业不适用，均不算失败；`PARTIAL/FAILED` 表示结果不完整或失败，须结合 `failedItems` 说明缺失范围。

## 13 纳税信用 `tax_credit`

`taxpayerCreditRating` 为当前纳税信用等级。`tags` 为当前纳税人标签，每项包含 `tagName`、`effectiveStartDate` 和 `effectiveEndDate`。本版不提供评价年度、评价分数、扣分指标、扣分行为、年度等级变化或信用修复记录，因此只能展示当前等级和标签，不能分析年度变化或扣分原因。

## 14 全量开具发票 `issued_invoices`

`direction=2`（数值）表示企业开具方向。记录通常包含购方、项目/编码、金额、税额、税率、状态、红字标记、发票代码/号码及红字发票明确返回的原蓝票代码/号码。该模块用于查询，不执行开票。

`3.16` 只使用明确返回的原蓝票号码建立蓝红关联，并按完整的次年1—3月观察。原蓝票号码缺失时只展示次年1—3月红字总量，不按交易对手、金额接近或日期接近推测关联关系。

## 15 全量取得发票 `received_invoices`

`direction=1`（数值）表示企业取得方向。记录通常包含销方、项目/编码、金额、税额、税率、用途、勾选和状态。

## 16 用途确认/抵扣 `deduction_confirmations`

按月份返回勾选认证汇总。来源字段 `yxdkse` 表示有效抵扣税额，`gxzt` 表示勾选状态，`gxsj` 表示勾选时间；它们不等于当前汇总响应会直接保留同名字段。当前 `deduction-summary.v1` 中，有效抵扣税额读取 `chunks[].list.selectedTax[].amount` 并核对 `complete`；认证购进金额和票面税额读取 `chunks[].list.purchases[]`；用途及状态读取 `chunks[].list.confirmations[]`，勾选与票面关联读取 `chunks[].list.relations[]`。具体字段和六类集合见[用途确认结构](structured-data.md#用途确认)。票面税额、有效抵扣税额和申报抵扣税额不可互代，差异须结合增值税附表和已有调节事实理解。

## 17 工商及公开信用信息 `qixin_external`

以下路径相对于该模块的 `data`，字段名及大小写保留响应原值。

| 字段 | 类型与业务含义 |
| --- | --- |
| `subjects` | 主体对象数组，包含企业自身及主要客户、供应商。 |
| `subjects[].name` / `subjects[].taxId` | 主体名称、税号字符串，用于和发票对手方核对身份。 |
| `subjects[].role` | 角色字符串：`企业自身`、`主要客户`、`主要供应商`。 |
| `subjects[].transactionShare` | 对应进销方向的交易占比，数值比例，`1` 表示 `100%`。这是响应中的占比字段；报告使用下述发票口径重新计算。 |
| `subjects[].identityVerified` | 布尔值，表示是否已按税号或完整名称精确核验；只有 `true` 的主体可进入工商及公开信用事实。 |
| `subjects[].detail` | 工商详情对象，包含基本登记信息及实际返回的股东/出资、人员、分支、变更、经营异常和行业分类。 |
| `subjects[].execution` / `subjects[].dishonest` | 被执行、失信被执行公开记录的集合结果，可为 `null`；有结果时保留原集合包装。 |
| `subjects[].taxViolation` / `subjects[].adminPunishment` / `subjects[].seriousIllegal` | 重大税收违法、行政处罚、严重违法失信公开记录的集合结果，可为 `null`。 |
| `coverage` | 主体覆盖统计对象；`ownEnterprise`、`importantCustomers`、`importantSuppliers`、`verifiedCounterparties` 为数量，覆盖数量不代表每项记录查询完整。 |
| `warnings` / `subjects[].warnings` | 模块及主体的提示字符串数组，用于说明未精确匹配、未取得结果等情况。 |

主体核验优先按税号，缺少税号时按完整企业名称精确匹配；不得采用模糊候选或混并同名不同税号主体。

报告中客户交易占比为本企业向该客户开具的发票净额除以本企业全部开具发票净额，供应商交易占比为本企业从该供应商取得的发票净额除以本企业全部取得发票净额。分母包含同方向全部有效金额的正负净额，包括未识别对手方及净额为负的对手方；不缩小为入选 Top5/Top10 的合计。分母不大于零时，占比不可用；`transactionShare` 不替代这一计算口径。

`report-read/report-refs` 提供的占比指标按 `display` 及 `unit` 写正文；`unit=%` 时已经是百分数，不能再次乘100，详见[写作指标口径](report-data-contract.md#指标与章节材料)。

主要客户、供应商的工商核验数量与报告对手方规则保持一致：少于5家时取全部，5—10家取Top5，超过10家且Top5交易占比低于60%时取Top10，其他情况取Top5。股东、主要人员和分支机构只展示接口实际返回的节点，不据此补推间接关系。

常用工商字段位于 `subjects[].detail`：`NewStatus` / `Status` 为登记状态，优先取 `NewStatus`；`StartDate` 为成立日期，`EndDate` 为返回的结束日期，需结合登记状态理解，缺失时不推定事件发生时间。`Partners`、`Employees`、`Branches`、`Changerecords`、`AbnormalItems` 分别保留股东、人员、分支、变更和经营异常集合。状态保留返回文字，不把缺失状态解释为正常。

金额和比例按字段分别解释：

- 股东记录的 `StockPercent`：不带百分号且在 `0—1` 内的值按比例转换为 `0%—100%`；带百分号或大于 `1` 的兼容值按百分数解释，缺失时显示未获取。
- 行政处罚记录的 `PunishAmnt` / `IllegalIncomeAmnt`：原始单位为万元，报告金额乘以 `10000` 后按元展示。
- 执行记录的 `Amount`：原始单位为元，不重复换算。
- `Content` 等决定内容不是金额字段；纯数字内容不赋予金额含义，未写明单位的文字不补单位，金额字段缺失也不补零。

分页集合中的 `Total`、`Num`、`Items` 等包装字段保留原结构。后续任一页失败时，不把首屏或已返回条数当作完整结果；结合模块状态、`failedItems` 和 `warnings` 区分查询成功无记录、身份未核验、未覆盖及查询失败。单独的 `null` 或字段缺失不能证明没有相关记录。

上下游信用表展示登记状态、当前经营异常及各类公开记录数量。只有事件日期能够落入同主体发票交易月份的公开信息，才可作为 `3.18/3.19` 的弱信号；不得仅凭成立年限、人员规模推断业务能力或认定风险。

模糊候选不得进入报告。公开信息可能存在更新时差；未返回记录不等同于绝对未发生。当前服务不包含专利、商标、行政许可、限制高消费、整体诉讼和行业专项许可，报告不得通过其他字段补推这些事实。

## 查询范围与缺失情况

下表中的模块名前缀用于定位模块，其后路径相对于该模块的 `data`；未写模块名的字段适用于返回该字段的模块。这些信息用于区分业务事实和查询覆盖情况：

| 字段或路径 | 类型与用途 |
| --- | --- |
| `filing_obligations.declarationDetails` | 已申报税费详情数组，通过原申报主键与各税种 `effectiveRecords` 关联；详情中的 `sbuuid`、`yzpzzlDm` 和子表 `code` 保留原值。 |
| `filing_obligations.sourceStatuses.undeclaredRecords` / `.todoRecords` | 两个来源各自的状态字符串，例如 `SUCCESS`；不能用已申报记录是否存在替代未申报和待办的查询状态。 |
| `listCollection.records` / `listCollection.chunks` | 记录数组及查询窗口数组；`chunks[]` 保留 `startTime`、`endTime` 和该窗口的 `list`。 |
| `queryCoverage` / `listCollection.queryCoverage` | 覆盖对象，包含 `startTime`、`endTime`、布尔值 `complete`；`endTime` 是本次固定查询日，可能晚于分析所属期终点。 |
| `listCollection.failedItems` | 查询缺段或失败明细数组，需连同模块及任务的 `failedItems` 判断缺失范围；失败不等同于查询成功无记录。 |
| `cit_annual_returns.referenceAnnualRecords` / `.referenceCoverage` | 年度参考记录数组及参考覆盖对象，和当前期记录分开；结合 `referenceCoverage.status` 理解，例如 `current-available` 表示当前年度已有可用记录，不能只据参考数组为空判断查询失败。 |
| `stamp_tax.historyCollection` | 历史查询集合对象，保留 `records`、`chunks`、`queryCoverage` 和 `failedItems`。 |
| `stamp_tax.historicalCapitalBooks` | 营业账簿历史对象，保留 `records`、`queryCoverage`、布尔值 `complete` 和 `scope`；其覆盖对象可另含 `analysisEndTime`，与实际查询的 `endTime` 区分。 |

申报事实仍按实际所属期筛选。`complete` 缺失或为 `false` 时，不得声称已完整覆盖。`failedItems.itemId` 及失败上下文保留返回原值，用于定位失败记录和子表。

财报 `records[].previews[]` 按 `code`、`label` 和 `data` 保存各张子表；`financial_detail` 失败上下文中的 `recordId` / `code` 定位失败记录及子表。记录缺失、查询成功但无记录、子表查询失败是不同情况；某张子表失败不能否定其他已成功取得的报表。

年度参考与营业账簿历史同当期记录分开。历史起点来自纳税人档案的开业日期 `Kyslrq`（基础信息中的 `taxpayerInfo.taxpayerBasicInfo[].Kyslrq`）；起点未取得或查询存在失败窗口时，`historicalCapitalBooks.complete=false`，不得称完整历史，也不得把历史与当期应纳税额累加。

报告模型的 `TaxFieldFact` 使用 `rowLabel` / `columnLabel` 保存实际中文项目及列名，`raw` 保留原值，`value`、`status`、`unit` 分别保存数值、可用状态和单位。`status` 为 `present`、`missing`、`not_applicable`、`present_text` 或 `failed`；文本、缺失和失败状态下的 `value=0` 不代表实际零值。双侧资产负债表分别识别左右项目；原始空白、上年数、本期数、累计数保持独立。
