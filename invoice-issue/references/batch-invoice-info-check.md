# 批量开票信息检查

使用 `issue-batch-invoice-info-check` 检查本次开票输入并生成预览。单张发票也使用同一命令。它不代表已经正式开票。

## 调用方式

```powershell
& ".\bin\windows-amd64\invoice-assistant_windows_amd64.exe" issue-batch-invoice-info-check --input <input.json> --pretty
```

Linux 和 macOS 使用安装文档中当前平台的二进制及相同参数。不要从源码另行运行，也不要合并 stdout 与 stderr。

## 输入准备

- 根对象必须含本次销售方 `uscc` 和 `invoices[]`；不得使用顶层数组或正式开票请求结构。
- 每张发票分别填写 `invoiceType`、`purchaserInfo`、`details[]`，票种必须先由用户明确；不按纳税人身份默认填普通发票。
- 自然人购方填写 `purchaserInfo.isNaturePerson=true`，不查询企业候选，普通发票不默认要求身份证号。
- 企业购方缺少税号时先执行 `issue-search-enterprise --query "<购方名称>"`。API Key 使用本地配置，缺少时通过 `workspace-config-writer` 配置。多个候选须展示并等待用户选择，不默认取首条；用户可不选并继续普通发票草稿，专票缺税号仍须补齐。
- 图片、PDF、Word、Excel 等材料先提取并核对字段；不能区分发票边界、金额或购方时先确认。
- 字段层级和可选字段见 [issue.md](issue.md)，自然语言转换见 [field-mapping.md](field-mapping.md)。用户未提供的字段保持缺失，不用零或示例值占位。
- 金额和单价默认按含税口径填写；有数量无单价时必须补一种单价或删除数量，不得从总额反推单价。
- 备注写入对应发票 `noteInfo.note`。折扣比例先确认语义再转换，煤炭类先明确品种代码和合法单位。
- 销方地址、电话、开户银行、银行账号可选，有值传、无值省略。可由检查命令补全的商品编码、税率、税额和合计，不在检查前逐项追问。

## 读取检查结果

| 字段 | 下一步 |
| --- | --- |
| `check_result.validation_passed=false` 或 `check_failed` | 按发票、明细序号一次性说明缺失或冲突，不展示可提交预览。 |
| `missing_fields_by_module` | 按所属模块向用户补问真正阻断的字段。 |
| `validation_errors` | 保留具体错误码和调整建议；金额问题参见 [amount-calculation.md](amount-calculation.md)。 |
| `purchaser_autofill.needs_user_selection`、`candidates` | 展示候选供用户选择；不因用户暂不选择而阻断普通发票草稿。 |
| `invoice_preview_markdown_generated`、`invoice_preview_markdown_path` | 检查通过后读取本次生成的预览文件，完整原样展示。 |
| `invoice_preview_error` | 预览未生成时报告实际错误；不手工拼预览或改用独立预览命令继续提交。 |

输入变化后重新执行检查。只有用户明确继续同一任务时才使用该任务，不能拿历史任务或旧预览代替本次结果。

## 预览确认

本轮必须完整展示预览正文、发票张数、所有明细和裸文本票样链接，再提示用户回复“确认开票”。不能只返回路径或以进度说明结束。

收到明确确认后，按 [workflow.md](workflow.md) 执行 `issue-from-output --confirm-preview --pretty`。模糊同意、沉默或尚未展示预览都不构成确认；不得手工修改任务状态表示已确认。
