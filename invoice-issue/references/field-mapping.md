# 发票开具字段映射

本文件说明如何把用户输入写入 `issue-batch-invoice-info-check --input` 的 JSON。完整输入字段表见 [issue.md](issue.md)。

## 根结构

原始 input JSON 必须是单个对象：

```json
{
  "uscc": "销售方税号",
  "invoices": [
    {
      "invoiceType": "普通发票",
      "purchaserInfo": {},
      "details": []
    }
  ]
}
```

- 根对象必须使用 `invoices[]`，即使只有一张发票。
- 每张票的购方写入 `invoices[].purchaserInfo`。
- 每张票的明细写入 `invoices[].details[]`。
- 不写 `params.info[]`、顶层 `info[]`、`invoice_payload_json.info[]`、`invoiceDetail.data[]`、`items[]`、`invoiceDetails[]`、`buyerName`、`extra`、`metadata` 等检查命令未声明支持的结构。
- 顶层必须写入本次销售方 `uscc`；`areaCode`、`personalAccount` 由命令处理，不要求用户补填或使用旧任务信息。

## 地区信息

销售方地区以本次绑定企业信息为准。命令提示企业地区缺失时，按预检报告完善平台企业配置，不从购方地址、备注或上次任务猜测地区。

## 票种

- “普票”“普通票”统一为“普通发票”。
- “专票”“增值税专票”统一为“增值税专用发票”。
- 每张发票必须显式提供票种。支持“普票、普通票、普通发票”映射为“普通发票”，支持“专票、增值税专票、增值税专用发票”映射为“增值税专用发票”；缺失或非法时整批检查失败。
- 增值税专用发票要求购方名称和购方税号，缺少时必须阻断并追问。

## 自然人与企业搜索

AI/Agent 必须先判断购方是自然人还是企业/单位，并把判断结果传给脚本：自然人写入 `purchaserInfo.isNaturePerson: true`，企业/单位写入 `purchaserInfo.isNaturePerson: false` 或保留企业购方字段。不要用名称猜测覆盖用户已明确的购方类型。

自然人、个人、消费者等购方开具普通发票时：

- 写入 `purchaserInfo.isNaturePerson: true`。
- 写入 `purchaserInfo.name`。
- 默认不要求 `purchaserInfo.identificationTypeCode`。
- 默认不要求 `purchaserInfo.identificationNumber`。
- 不要求购方税号或统一社会信用代码。
- 不调用企业搜索。

只有后端检查结果、地区规则或用户选择的票种明确把自然人证件信息列为阻断缺失项时，才一次性追问证件类型和证件号码。不得伪造身份证号、证件类型或税号。

企业购方规则：

- 购买方明确是企业/单位，且缺少统一社会信用代码或纳税人识别号时，必须先主动执行 `issue-search-enterprise --query "<购方名称>"` 使用企业搜索；不得跳过查询直接进入预览。
- 唯一高置信候选可补齐名称和税号。
- 企业搜索仅作参考，不是普通发票草稿的前置门槛。
- 多个候选必须展示为可选参考项；用户可选择其中一个写回，也可以不选，不能默认取首条。
- 普通发票企业搜索无候选、失败、超时或用户暂不选择时，可继续生成不带购方税号的草稿和预览，不阻止草稿生成。
- 增值税专用发票缺购方税号时必须追问，不能进入预览或正式提交。
- 用户已经明确为自然人时，即使命名像企业名，也不能把姓名送去企业搜索。

## 金额口径

- 用户只说“金额”“开 1000 元”“合计 1000”时，默认按含税金额写入 `taxInclusiveAmount`。
- 用户明确说“不含税金额”时，才写入 `amount`。
- 用户说“单价”时，默认按含税单价写入 `taxInclusivePrice`。
- 用户明确说“不含税单价”时，才写入 `price`。
- 用户可同时提供含税/不含税总额和单价；检查命令保留用户值、补齐缺项并校验。完整矩阵和报错调整见 [amount-calculation.md](amount-calculation.md)，不要在编排层自行覆盖冲突值。
- `price`、`taxInclusivePrice` 最多保留13位小数；`amount`、`taxInclusiveAmount`、`tax` 保留2位。用户提供任一种单价但未提供数量时，检查命令补 `quantity=1`，预览和正式请求都传1。
- 用户提供 `quantity` 时还必须提供 `price` 或 `taxInclusivePrice` 中的任意一种；系统补齐另一种单价。没有任何单价时不得根据总额反推单价，纯总额模式也不要生成 `quantity`。完整规则见 [amount-calculation.md](amount-calculation.md)。
- 金额和单价允许用户明确提供不含税值，按对应字段填写；票面仍按含税口径预览，不擅自切换开票模式。
- 含税金额很小导致按税率计算或四舍五入后的税额为 0 时，税额 0 是合法计算结果；继续生成草稿和预览，不得仅因税额为 0 判定金额税额关系不一致。

## 税率

- 用户明确给出 `13%`、`6%`、`1%`、`0%` 或 `0税率` 时，写入 `taxRate`。
- 用户未明确税率时，不猜测常见税率；保持缺失。仅当任务企业 `nsrzg` 合法或用户提供合法人工 `taxpayerType` 时，检查命令才在商品编码确定后按“纳税人身份 + 商品字典”匹配最低参考税率。
- 用户说“免税”时，原样写入 `taxRate: "免税"`，由检查命令标准化。
- 用户说“不征税”时，原样写入 `taxRate: "不征税"`，由检查命令标准化。
- `0税率`、`0%` 与“免税”“不征税”不是同一语义，不能自动补特殊政策字段。
- 用户同时给出数值税率和“免税/不征税”时，视为冲突，必须追问确认。

## 折扣

- 明确给出折扣比例时，先归一化为百分比数值写入 `discountInfo.discount`，例如 `50% -> 50`、`88折 -> 88`、`12.5% -> 12.5`。
- “0.5 折扣”语义不清时必须追问，不能自行解释为五折或 0.5%。
- 明确折扣金额或优惠金额时，写入 `discountInfo.discountAmount`。
- 用户只说“有折扣”“打了折”但没有比例或金额时，必须追问。
- 不主动写 `discountInfo.discountMode`、`discountInfo.discountTax`；这些由检查命令标准化。

## 备注和可选字段

- 用户明确提供备注时，只能写入对应发票的 `noteInfo.note`。
- 不写 `remark`、`remarks`、`memo` 或批量顶层备注。
- 用户或文件中已无歧义识别出的购方地址、电话、开户行账号、备注、税额、折扣等可选字段，应按受支持键名写入。
- 销方地址、电话、开户银行和银行账号也是可选字段；用户提供时按 `sellerInfo.address`、`sellerInfo.phone`、`sellerInfo.bank`、`sellerInfo.bankAccount` 写入，未提供时省略。销方名称以用户输入优先，否则只使用任务企业本地 `enterpriseName`。
- 未提供且无法稳定识别的可选字段保持缺失，不要补 `0`、`0.00`、空占位或示例值。

## 纳税人身份

- 只读取任务 `enterprise_context.nsrzg`；数值或字符串 `0` 映射为一般纳税人，数值或字符串 `1` 映射为小规模纳税人。
- 纳税人身份仅用于税率参考，不用于推断 `invoiceType`；不得用独立企业画像或上次企业信息代替。
- `nsrzg` 缺失或非法时，合法人工 `taxpayerType` 可作为人工确认；否则输出 `taxpayer_identity_missing`，保留用户显式票种和税率，并跳过依赖身份的自动票种、税率判断。

## 煤炭类

煤炭相关明细必须在 input JSON 阶段确认 `mtzlDm` 和 `unit`。

| 标准名称 | mtzlDm |
| --- | --- |
| 政府保供煤 | 0100 |
| 市场煤 | 0300 |
| 长协煤-协议期不足半年 | 0201 |
| 长协煤-协议期在半年至一年之间 | 0202 |
| 长协煤-协议期在一年至两年之间 | 0203 |
| 长协煤-协议期在两年以上 | 0204 |

- 单位只允许 `吨`、`千克（公斤）`。
- 用户只说“长协煤”“普通煤”“常见煤种”等无法唯一映射时，必须追问。
- 不能把模糊煤种延后到正式提交阶段再处理。

## 文件输入

图片、截图、PDF、Word、Excel、压缩包清单等文件输入必须先提取为文字版开票字段，再落位到 `invoices[]`。

- 文件中无歧义识别出的字段，不论必填还是可选，只要有受支持落位就写入 input JSON。
- 票与票边界、购方、商品、金额、税率等核心字段不清时，先向用户补问。
- 必要时建议用户改用文字输入或手动补充缺失关键信息。
- 不能把原文件内容未经提取直接当作结构化字段。

## Windows JSON 编码

- Windows 下包含中文的输入应写入 UTF-8 JSON 文件，再通过 `--input` 传给命令。
- PowerShell 中优先用对象加 `ConvertTo-Json` 生成 JSON，再用 .NET UTF-8 无 BOM 写文件。
- 不优先用 stdin 管道直传 JSON，不手写复杂 here-string。
- 控制台显示乱码时，优先用 `Get-Content -Raw -Encoding UTF8 <input.json> | ConvertFrom-Json` 做 round-trip 验证。
- `\uXXXX` 转义是合法 JSON，不等于编码损坏。
