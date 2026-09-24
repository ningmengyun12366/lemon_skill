# 发票明细金额计算与报错调整

仅在 `validation_errors` 出现下列金额错误码时读取本文。登录、企业信息、税率缺失、API 异常等问题继续按 [error-handling.md](error-handling.md) 处理。

## 字段与精度

- A=`amount`：不含税总额；AT=`taxInclusiveAmount`：含税总额；P=`price`：不含税单价；PT=`taxInclusivePrice`：含税单价。
- Q=`quantity`。存在P或PT但用户未提供Q时，系统补 `Q=1`，预览和正式请求都传1；P、PT都不存在时不生成Q。R=`taxRate`；T=`tax`。
- P、PT四舍五入到最多13位小数；A、AT、T四舍五入到2位。中间计算保持十进制高精度。
- 用户值只做上述精度标准化，不被补算值覆盖；所有适用关系的绝对差额不超过 `0.01` 才能通过。
- 自然语言只说“金额”或“项目金额”而未说明税口径时，输入映射阶段默认写入AT；只有明确说“不含税金额”“税前金额”或直接使用正式字段 `amount` 时才写入A。
- 自然语言只说“单价”而未说明税口径时，输入映射阶段默认写入PT；只有明确说“不含税单价”“税前单价”或直接使用正式字段 `price` 时才写入P。

## 补算矩阵

| 总额状态 | 无单价 | 只提供PT | 只提供P | 同时提供P、PT |
|---|---|---|---|---|
| A、AT都缺失 | 报 `LINE_AMOUNT_BASIS_MISSING` | 补P；`AT=round2(PT×Q)`；`A=round2(P×Q)` | 补PT；`A=round2(P×Q)`；`AT=round2(PT×Q)` | 分别用P、PT和Q补A、AT |
| 只提供AT | 保留AT；补`A=round2(AT÷(1+R))`；不生成单价 | 保留AT、PT；补A、P | 保留AT、P；补A、PT | 保留AT、P、PT；只补A |
| 只提供A | 保留A；补`AT=round2(A×(1+R))`；不生成单价 | 保留A、PT；补AT、P | 保留A、P；补AT、PT | 保留A、P、PT；只补AT |
| A、AT都提供 | 保留两个总额；不生成单价 | 保留总额、PT；只补P | 保留总额、P；只补PT | 全部保留，只校验 |

单价换算：`P=round13(PT÷(1+R))`，`PT=round13(P×(1+R))`。P、PT都缺失时不得根据总额反推单价。

## 单价与数量成对规则

| 用户单价状态 | 用户提供Q | 用户未提供Q |
|---|---|---|
| 无P、无PT | 报 `LINE_PRICE_QUANTITY_PAIR_MISMATCH`，不反推单价 | P、PT、Q均不生成，按总额模式处理 |
| 只提供PT | 补P，保留Q | 补P，并生成`quantity=1` |
| 只提供P | 补PT，保留Q | 补PT，并生成`quantity=1` |
| 同时提供P、PT | 保留两个单价和Q | 保留两个单价，并生成`quantity=1` |

单价模式的最终明细必须同时存在P、PT、Q；总额模式的最终明细不得生成P、PT、Q。用户给Q但没有单价时，要求用户补P或PT中的任意一种，或者删除Q；不能用总额反推单价。系统默认的Q=1参与全部乘数量校验，错误中显示“系统默认；用户未提供，最终请求将传1”。

## 校验和处理

| 错误码 | 校验 | Agent调整动作 |
|---|---|---|
| `LINE_AMOUNT_TAX_RELATION_MISMATCH` | `abs(round2(A×(1+R))-AT) <= 0.01` | 让用户确认以A还是AT为准 |
| `LINE_UNIT_PRICE_TAX_RELATION_MISMATCH` | `abs(round13(P×(1+R))-PT) <= 0.01` | 让用户确认以P还是PT为准 |
| `LINE_TAX_INCLUSIVE_TOTAL_MISMATCH` | `abs(round2(PT×Q)-AT) <= 0.01` | 核对PT、Q、AT |
| `LINE_TAX_EXCLUSIVE_TOTAL_MISMATCH` | `abs(round2(P×Q)-A) <= 0.01` | 核对P、Q、A |
| `LINE_AMOUNT_BASIS_MISSING` | A、AT、P、PT全缺失 | 要求补充任意一种总额或单价 |
| `LINE_PRICE_QUANTITY_PAIR_MISMATCH` | 用户提供Q，但P、PT都未提供 | 补P或PT中的任意一种，或者删除Q |

直接读取错误中的发票序号、明细序号、用户值、系统补算值、期望值、差额和调整建议，一次性说明全部冲突。不要静默覆盖用户值，也不要替用户决定以哪个值为准。用户确认后更新原始 `invoices[].details[]`，重新执行检查并重新生成预览；未重新检查通过前不得正式开票。
