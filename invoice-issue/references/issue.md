# 开票检查输入字段

本表用于准备 `issue-batch-invoice-info-check --input <input.json>` 的输入。自然语言映射、票种、折扣及煤炭规则见 [field-mapping.md](field-mapping.md)，金额冲突处理见 [amount-calculation.md](amount-calculation.md)。不要自行构造正式 HTTP 开票请求。

## 根对象和每张发票

根对象包含本次销售方 `uscc` 和 `invoices[]`；单张也必须放入数组。下表除 `uscc` 外均相对于单个 `invoices[]`。

| 字段 | 输入要求 | 含义 |
| --- | --- | --- |
| 根对象 `uscc` | 必填 | 本次用户明确提供或选择的销售方税号。 |
| 根对象 `taxpayerType` | 身份缺失时按需提供 | 仅在本次企业纳税人身份缺失或非法时，由用户明确确认一般纳税人或小规模纳税人；不据此推断票种。 |
| `invoiceType` | 逐张必填 | 普通发票或增值税专用发票；接受的简称见字段映射，不按纳税人身份猜测。 |
| `giveUpReason` | 可选 | 用户明确提供的放弃减税优惠原因。 |
| `sellerInfo.name` | 可选 | 销方名称；未提供时使用本次已选择企业的名称。 |
| `sellerInfo.address/phone/bank/bankAccount` | 可选 | 销方地址、电话、开户银行和账号；有值传、无值省略。 |
| `purchaserInfo.name` | 必填 | 购方名称。 |
| `purchaserInfo.isNaturePerson` | 由 Agent 判断 | 自然人填写 `true`，企业/单位填写 `false`。 |
| `purchaserInfo.uscc` | 专票必填 | 购方税号；企业购方未提供时先执行企业搜索，自然人不搜索。 |
| `purchaserInfo.address/phone/bank/bankAccount` | 可选 | 用户明确提供或从材料中无歧义识别的购方信息。 |
| `purchaserInfo.identificationTypeCode/identificationNumber/nationCode` | 按实际要求填写 | 证件类型、号码、国籍或地区；普通发票自然人购方不默认要求证件。 |
| `details[]` | 至少一项 | 本票的商品或服务明细，见下表。 |
| `invoiceDetail.amount/tax/total` | 可选 | 用户明确提供的整票不含税金额、税额和价税合计；交给检查命令与明细核对，缺失时不自行猜填。 |
| `noteInfo.note/payee/reviewer` | 可选 | 备注、收款人、复核人；备注不得改写为 `remark` 等其他键名。 |
| `personInfo.name/nation/certificateType/certificateCode/uscc` | 可选 | 经办人姓名、国籍、证件类型、证件号码及自然人纳税人识别号。 |
| `paymentInfo[].payChannelCode/transactionOrderNo` | 可选 | 支付渠道代码及交易单号。 |

`areaCode`、`personalAccount`、`loginType` 由命令处理，不要求用户补填或从旧任务抄入。证件、国籍和支付渠道编码按 [common-code-mappings.md](common-code-mappings.md) 处理；没有明确输入依据的可选字段保持缺失。

## 每条明细

下表字段相对于 `invoices[].details[]`。

| 字段 | 输入要求 | 含义 |
| --- | --- | --- |
| `projectName` | 必填 | 商品或服务名称。 |
| `spbm` | 可选 | 已知商品编码；未提供时先执行检查，依据检查结果处理。 |
| `size` | 可选 | 规格型号。 |
| `unit` | 通常可选，煤炭必填 | 煤炭只允许 `吨`、`千克（公斤）`。 |
| `taxInclusiveAmount` | 与 `amount` 至少提供一种 | 含税金额；未说明税口径的“金额”默认填这里。 |
| `amount` | 与 `taxInclusiveAmount` 至少提供一种 | 用户明确提供的不含税金额。 |
| `taxInclusivePrice` | 有单价时填写 | 含税单价；未说明税口径的“单价”默认填这里。 |
| `price` | 有不含税单价时填写 | 不含税单价；与 `quantity` 同时存在或同时不存在。只提供含税单价时按金额规则完成检查。 |
| `quantity` | 有单价时使用 | 用户未提供数量时补 `quantity=1` 并传入正式请求；只有数量没有任何单价时必须补单价或删除数量。 |
| `taxRate` | 明确提供时填写 | 税率、征收率、`0%`、`免税`或`不征税`；不把后三者混用，不猜测缺失税率。 |
| `tax` | 明确提供时填写 | 税额，保留用户输入并交给检查命令核对。 |
| `discountInfo.discount` | 可选 | 百分比数值，例如 `50% -> 50`、`88折 -> 88`；语义不清先确认。 |
| `discountInfo.discountAmount` | 可选 | 明确提供的折扣金额；与折扣比例的处理规则见字段映射。 |
| `xsyhzcbz/zzstsgl` | 可选 | 明确提供的优惠政策标志和增值税特殊管理说明；不因零税率自行补造。 |
| `mtzlDm` | 煤炭必填 | 煤炭种类代码；须在输入前明确品种和合法单位。 |
| `sptm` | 可选 | 商品条码。 |

金额、税额保留两位小数，单价最多十三位小数。纯总额模式不生成数量或单价；不得从总额反推用户未提供的单价。含税金额很小导致税额为零时，零税额可以是合法结果。

## 检查后的操作

1. 调用检查命令，读取本次返回的 `check_result`；按发票及明细序号一次性说明缺失项或冲突。
2. 校验通过且预览已生成后，读取预览 Markdown 并完整原样展示，再等待用户明确回复“确认开票”。
3. 正式提交和结果处理按 [workflow.md](workflow.md) 执行；输入发生变化时重新检查、重新展示预览并确认。
