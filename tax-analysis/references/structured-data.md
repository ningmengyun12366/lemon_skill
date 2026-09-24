# 结构化数据说明

当前响应使用 `dataSchemaVersion=tax-analysis.v2`。报告规则为 `{{REPORT_RULE_VERSION}}`，AI 契约为 `{{AI_CONTRACT_VERSION}}`。分析通过 `report-read` 获取事实与引用，需要完整对象时使用 `report-refs`。下文的模块路径指响应 `moduleResults` 中对应 `code` 项的 `data`；字段名、大小写、数组顺序和格式标记均按实际响应读取，不改名或推造替代字段。

## 发票

`issued_invoices/received_invoices.data` 的 `format=invoice-summary.v1`，`chunks[]` 按查询月份提供统计结果，包含 `month/startTime/endTime/total/groups/links/checks/specificChecks`。这些是汇总与关联数据，不是完整逐票明细。下表 `groups[]` 路径均相对于单个 `chunks[]`，完整路径为 `data.chunks[].groups[]`。

| 字段 | 读取含义 |
| --- | --- |
| `data.total`、`chunks[].total` | 响应总记录数、该查询月份的记录数；不能用分组数量代替发票张数。 |
| `groups[].id/month/counterparty/counterpartyID` | 统计组标识、月份、交易对手名称及标识；主体合并和排名必须保持同一身份口径。 |
| `groups[].status/red/directAbnormal/deducted/signalType` | 发票状态、红字标记、直接异常标记、抵扣标记及返回的异常信号类型；沿用已有状态，不自行推定已申报抵扣。 |
| `groups[].count` | 该统计组代表的真实发票张数，不是统计组数量或明细行数；报告张数使用工具给出的同口径统计结果。 |
| `groups[].amount/tax/maxAbsAmount` | 不含税金额合计、税额合计、逐票绝对不含税金额最大值，单位均为元；合计保留原始正负号，红字不得再次反号。 |
| `groups[].hasAmount/hasTax/hasCategory` | 金额、税额、品类是否有可用依据。金额或税额标记为 false 时保持未知，即使对应数值字段为 0，也不能认作零额事实。 |
| `groups[].evidenceKey/evidenceAmount` | 风险线索使用的证据标识及对应金额（元）；不能把证据金额再次加进统计组汇总。 |
| `groups[].lines[]` | 品名、品类和税码统计行，字段为 `item/category/taxCode/amount/tax/hasAmount/hasTax/count`；`count` 是原明细行数，金额及税额为元，其可用性仍由标记决定。 |
| `groups[].allocations[]` | `name/amount` 分别是分摊品类名称及金额（元）。返回结果已包含逐票未分摊差额，金额比较容差为 0.005 元；保留“未识别/未分摊”，不能因缺少品类而减少发票总额。 |

`chunks[].checks` 包含 `records/identityKnown/missingItem/missingCode/missingRate/directAbnormal` 等记录数，以及以下数组：`ownIdentities[]` 每项为 `[主体标识, 主体名称, 记录数]`；`codes[]/rates[]` 每项为 `[比较对象键, 字符串值数组]`。代码和税率文本保持字符串，不把缺失税率当作零税率。`chunks[].specificChecks[]` 使用 `name/count/missing/labels` 表示特定业务检查名称、覆盖数量、缺失数量和检查标签；无覆盖不代表全部检查通过。

`chunks[].links` 按发票方向读取：销项项包含 `number/code/originalNumber/originalCode/month/amount/hasAmount/red`，分别用于当前票号及代码、原蓝票号及代码、月份、不含税金额（元）、金额可用性和红字标记；进项项包含 `number/hasCategory/restricted/directAbnormal`，用于票号、品类可用性、不得抵扣业务线索和直接异常判定。它们只保留关联必需信息，不替代完整票面。未明确返回原蓝票关联时，不按名称、金额或日期相近推测关系。

`data.yearEndObservations[]` 是跨年观察结果，包含 `baseYear/decemberBlueAmount/nextQuarterRedAmount/linkedRedAmount/amountDataComplete/linkDataComplete`：基准年、当年12月蓝字金额、次年1—3月红字金额、对应当年12月原蓝票的红字金额，以及金额和关联是否完整。金额单位为元，红字观察金额为用于计算比例的绝对金额。仅对完整覆盖当年12月及次年1—3月的窗口使用；金额不完整，或存在次年红字金额但关联不完整时，不计算对应红冲比例或补造等级。

主要客户、供应商按完整期间红蓝净额及同一主体规则排名后选前五/前十。个人保留在发票金额与排名口径中，但不作工商查询。张数及金额采用返回结果，不自行新增去重；补查后的同一月份结果替换旧结果，不把新旧分区重复相加。

## 用途确认

`deduction_confirmations.data.format=deduction-summary.v1`，`chunks[]` 使用 `month/startTime/endTime/total` 标明查询月份、起止日期和数量。认证所属月与开票月份含义不同，不能互相替代。六类集合均位于各 `chunks[]` 内，值是汇总对象，不是原始发票数组；不要再去模块根节点重复取数。

| 集合字段 | 数据范围 |
| --- | --- |
| `list` | 普通发票用途确认 |
| `abnormalInvoiceList` | 独立异常凭证查询结果 |
| `customsPaymentList` | 海关缴款书 |
| `withholdingList` | 代扣代缴凭证 |
| `agriProductAdditionalList` | 农产品加计扣除 |
| `agriProductCustomsDocsList` | 农产品海关缴款书 |

集合中的 `count` 是记录数；`normalAbnormal/flagged/selectedAbnormal/conflict` 分别保留普通列表异常状态线索、异常标记、已勾选异常记录及相反状态冲突数量。缺少 `abnormalInvoiceList` 不等于异常凭证数量为零；不能用普通列表替代独立异常凭证查询。

`chunks[].list.purchases[]` 每项严格为四元组 `[认证所属月, 原去重键, 不含税金额, 税额]`。前两项为字符串，后两项为元金额或 `null`；任一金额为 `null` 只表示该项未知，不转为零。去重以认证所属月和有效原去重键组合判断，空键或仅含冒号/零的占位键不能把不同记录合并。该数组用于认证购进金额及税额，不能再与同批原票或汇总数重复累加。

`chunks[].list.relations[]` 使用 `number/selected/embedded` 表示票号、已勾选状态及可用的嵌套票面判定。`embedded` 若存在，包含 `number/hasCategory/restricted/directAbnormal`；它不是原始 `invoiceItemVO` 明细。模块根节点的 `association` 是用途确认与进项发票的关联结果，使用 `matched/missing/restricted/selectedAbnormal` 分别表示按票号关联数量、缺少票面品名数量、餐饮/娱乐等不得抵扣业务的已勾选线索数量、已勾选异常关联数量。使用最新返回的关联结果，不能把未关联或品名缺失当作无风险，也不能仅凭已勾选认定最终已申报抵扣。

各 `chunks[]` 内上述集合对象的 `confirmations[]` 按 `period/purpose/status` 保留用途确认所属期、用途和状态，并有 `count/amount/amountKnown/attention`；例如普通列表路径为 `chunks[].list.confirmations[]`。有票号时通过 `invoiceNumber` 保留。`amount` 单位为元，`amountKnown=false` 时金额未知；`attention` 表示需核实的状态或用途。同层的 `signals[]` 使用 `counterparty/period/amount` 表示异常凭证线索涉及的主体、所属期及金额（元），例如 `chunks[].abnormalInvoiceList.signals[]`。这些汇总、关联数量和信号不能相加当作额外发票张数。

`chunks[].list.selectedTax[]` 按实际确认所属期提供 `period/amount/complete`：所属月、已勾选有效抵扣税额（元）、该月是否完整。只有该月份全部分页查询成功且明确无记录，才可将 `amount=0, complete=true` 视为零额事实；未知勾选状态、已勾选票缺有效抵扣税额或返回错期时，不能视为完整。计算进项票申差异时，模块须完整、所需月份均有唯一且明确完整的汇总，所属月也须落在对应 chunk 的起止范围内；缺失该项的空列表不代表零抵扣，不能据此形成100%差异或风险。

税局明确提示“本功能仅提供给增值税一般纳税人或辅导期一般纳税人使用。”时，该月份表示不适用，不是失败或已认证金额为零。对应 `applicability` 保留 `status=not_applicable`、`reasonCode=general_taxpayer_only` 和原始 `message`，同时保留空汇总，不进入失败项。仅在全部月份均明确不适用时，模块根节点才能同样标记整体不适用；其他月份的真实请求错误须分别保留，不能用整体不适用掩盖。

## 申报与财报

结构化税表使用 `format=tax-table.v1`，包含 `texts/rows/sourceHash/hints`。`texts` 是字符串数组；`rows[]` 每行严格为四项数组，位置不可互换：

| 数组位置（从0开始） | 含义 |
| --- | --- |
| `rows[i][0]` | 表号（整数），用于区分同一文档中的不同表。 |
| `rows[i][1]` | 原始行号（整数），保留行在原表中的位置。 |
| `rows[i][2]` | 原始单元格文本 ID 数组，对应未展开的单元格序列。 |
| `rows[i][3]` | 合并单元格展开后的列网格文本 ID 数组，用于按列定位。 |

后两项中的每个文本 ID 都是 `texts` 的零基下标，读取为 `texts[id]`；它本身不是金额、列号或业务代码。必须满足 `0 <= id < texts.length`，行缺项或文本引用越界属于无效税表。保留空字符串单元格及展开后的列位置，不删除空列或把数值向前压缩。主表、附表、子表、中文标题及文本说明都须保留；空单元格不等于零，文本说明不等于金额，不得脱离列标题解释本期、累计、上年、期初或期末数。

`sourceHash` 只标识原来源内容，不表示金额、资料完整性或税表业务类型。`hints.smallVat/hints.taxpayerForm` 是布尔提示，保留表外适用文字识别出的表式信息；缺失或非 true 不得当作已确认适用。原生 JSON 税表仍按其结构化字段读取，不强行当作 `rows` 数组。申报主键、所属期、表代码、有效版本与历史版本、前期参考及历史营业账簿集合仍在各模块元数据中。税种使用 `zsxmDm` 及必要的 `zspmDm` 判定，申报表代码不能替代征收项目。

任务响应顶层的 `taxDocuments` 是“文档 ID → 税表对象”的映射，同一文档可被多处引用。模块的 `details[].data/previews[].data` 等位置若为 `{"$taxDocument":"文档ID"}`，必须先取 `taxDocuments[文档ID]` 再读取其 `format/texts/rows`，不能把引用对象当成空税表；嵌套对象或数组中的同类引用也按此定位。引用 ID 缺失、文档不存在或无法解码属于无效响应，不能表述为查询成功但无数据。某张子表本身失败时保留该失败范围，其他已成功子表仍可使用。

## 缴款

`payments.data.list` 是唯一缴款正文。`data.total` 为返回总记录数，`data.queryAsOfDate` 为查询基准日；`data.chunks[]` 只描述查询分段，不再包含另一份缴款正文：

| 字段 | 含义 |
| --- | --- |
| `startTime/endTime` | 该分段的查询起止日期，不自动等同于每条税款所属期。 |
| `total` | 该分段返回的总记录数。 |
| `startIndex` | 该分段记录在 `data.list` 中的零基起始下标。 |
| `count` | 该分段实际放入 `data.list` 的记录数，对应 `list[startIndex:startIndex+count]`。 |

按根 `list` 读取一次，不再把 chunks 的数量或区间当作另一份金额累加。记录原顺序、真实重复业务记录、缺失和零额均保留，不自行去重；更新后的同一查询结果替换旧结果，不能把补查前后的快照叠加。实缴金额单位为元，缺失金额与明确零额分别处理，不用申报应纳税额代填。所属期与缴款日期须区分；来源引用保留 `moduleResults[code=payments]/data/list[原下标]`。

## 来源与使用范围

`source-snapshot.json` 保存本次结构化响应，`report-read` 和 `report-refs` 提供可用于写作的事实及引用。统计组 ID、数组行下标和税表中文行列引用必须能在该快照中定位；不能把汇总响应当作完整原票或原始页面。底稿用于核对来源，不能直接作为客户正文；Word/PDF 使用业务名称和已确认的展示值。

记录数量已知、查询成功但无记录、查询不完整及未取得结果是不同状态。分页或期间有缺口时，应明确限制，不能将缺失值补零，也不能把部分结果概括为全部业务。
