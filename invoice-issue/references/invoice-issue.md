# 开票操作示例

本文件展示如何把用户请求整理为检查输入并交付结果。完整流程见 [workflow.md](workflow.md)，字段表见 [issue.md](issue.md)，自然语言映射见 [field-mapping.md](field-mapping.md)。

## 准备本次输入

先完成预检并确认本次销售方和每张发票的票种。图片、PDF、Word、Excel 等材料先提取成可核对的文字字段；识别不清或多张发票边界不明确时，先向用户确认。

以下仅展示结构，执行时必须替换为用户真实提供或明确选择的数据：

```json
{
  "uscc": "<本次销售方税号>",
  "invoices": [
    {
      "invoiceType": "普通发票",
      "purchaserInfo": { "name": "<购方名称>", "isNaturePerson": false },
      "details": [
        { "projectName": "<商品或服务名称>", "taxInclusiveAmount": 100 }
      ]
    }
  ]
}
```

- 即使只有一张发票，也使用 `invoices[]`。编号、换行或分号分隔的多组完整开票内容，应按实际张数分别写入。
- 企业购方缺少税号时先执行 `issue-search-enterprise --query "<购方名称>"`；多个候选等待用户选择，自然人购方不搜索。
- 用户未说明税口径时，金额使用 `taxInclusiveAmount`，单价使用 `taxInclusivePrice`。不自行猜税率、生成单价或用零填补缺失字段。
- 每张发票必须明确提供 `invoiceType`，缺少票种时先确认；增值税专用发票缺少购方税号时不能继续。
- 备注写入对应发票的 `noteInfo.note`。已识别的可选字段应按字段表填写；煤炭类先明确 `mtzlDm` 和合法单位。

## 检查与预览

Windows 使用 UTF-8 JSON 文件，并保留 stdout 与 stderr 分离：

```powershell
& ".\bin\windows-amd64\invoice-assistant_windows_amd64.exe" issue-batch-invoice-info-check --input <input.json> --pretty
```

只读取本次命令返回的任务和结果。`validation_passed=false` 时按 [error-handling.md](error-handling.md) 处理；校验通过后读取 `check_result.invoice_preview_markdown_path`，完整原样展示预览及所有裸文本链接。

不能以“已生成”“请看文件”代替正文；展示应包含本次发票张数、全部明细和预览票样链接。展示后提示用户回复“确认开票”，并等待明确确认。

## 提交与交付

仅在用户确认当前已展示预览后执行：

```powershell
& ".\bin\windows-amd64\invoice-assistant_windows_amd64.exe" issue-from-output --confirm-preview --pretty
```

- 结果仍在处理时保留同一任务并续查，不重新创建发票。提交结果未知且没有可查询任务号时停止，先核实是否已开票。
- 命令返回手动登录网页入口时，原样展示实际原因和该地址；用户完成登录后复用同一任务处理。
- 需要开票权限认证时，按 [invoice-issue-auth-extend.md](invoice-issue-auth-extend.md) 完成；不要把普通登录网页描述为二维码认证。
- 重复开票提示必须先说明风险；只有用户明确二次确认后才能使用 `--confirm-duplicate`，不能默认携带。
- 读取 `issue_result.issue_result_markdown_path` 并原样展示结果、PDF 原件及附件路径。未达到最终成功状态时，不得宣称已开票成功。
