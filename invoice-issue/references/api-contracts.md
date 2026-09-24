# 发票开具命令与返回结果

本文件说明开票命令的参数、前置条件、返回结果和后续操作。业务请求由已安装的 CLI 执行，Agent 不自行拼接 HTTP 请求。

## 运行时事实

- 能力码：`invoice_issue`。
- 默认开放平台门户：`https://www.nmy.cn`。
- CLI 策略：外置二进制下载，ZIP 包不包含 `bin/**` 或 `scripts/**`。
- Windows 示例二进制：`.\bin\windows-amd64\invoice-assistant_windows_amd64.exe`。
- Linux x64：`./bin/linux-amd64/invoice-assistant_linux_amd64`。
- Linux ARM64：`./bin/linux-arm64/invoice-assistant_linux_arm64`。
- macOS x64：`./bin/darwin-amd64/invoice-assistant_darwin_amd64`。
- macOS ARM64：`./bin/darwin-arm64/invoice-assistant_darwin_arm64`。
- 外置二进制下载地址必须从安装文档读取，不能在参考文档中硬编码或猜测。
- Windows PowerShell 调用带路径的二进制时使用调用运算符 `&`；不得用点号 `.` 对可执行文件做 dot-source，也不得追加 `2>&1` 合并 stdout 与 stderr。

## CLI 命令清单

| 命令 | 用途 | 正常入口 |
| --- | --- | --- |
| `--help` | 检查二进制可执行 | 首次会话环境检测 |
| `task-preflight-check [--use-config-apikey]` | API Key、企业绑定、workspace 和配置预检；`invoice_issue` 不预检税局登录 | 每次业务执行前 |
| `workspace-config-writer` | 写入 API Key、workspace_dir 等配置 | 预检提示配置缺失或用户要求更新时 |
| `issue-batch-invoice-info-check --input <input.json> --pretty` | 创建新任务、检查 `invoices[]`、补全字段、生成预览 Markdown | 正常开票草稿入口 |
| `issue-search-enterprise --query "<名称或税号>"` | 企业购方候选参考查询 | 企业/单位购方缺少购方税号时必须先主动调用；不作为普通发票草稿阻断条件 |
| `query-enterprise-profile-by-uscc` | 独立查询企业画像、经营范围 | 独立画像/风险分析场景；不是开票检查、预览或提交依赖 |
| `issue-from-output --confirm-preview --pretty` | 将用户对当前预览的明确确认写入任务状态并正式提交 | 已完整展示当前预览且用户明确回复“确认开票”后 |
| `issue-auth-workflow --action start --pretty` | 生成开票权限认证本地二维码 PNG | 权限认证分支 |
| `issue-auth-workflow --action verify --pretty` | 校验扫码和认证状态 | 用户完成扫码后 |



## 预检与 workspace 配置

预检命令：

```powershell
& ".\bin\windows-amd64\invoice-assistant_windows_amd64.exe" task-preflight-check
```

指定企业名称或税号：

```powershell
& ".\bin\windows-amd64\invoice-assistant_windows_amd64.exe" task-preflight-check --enterprise-name "企业名称或税号"
```

用户确认沿用本地已配置 API Key 后：

```powershell
& ".\bin\windows-amd64\invoice-assistant_windows_amd64.exe" task-preflight-check --use-config-apikey
```

指定销售方并确认沿用本地已配置 API Key 后：

```powershell
& ".\bin\windows-amd64\invoice-assistant_windows_amd64.exe" task-preflight-check --enterprise-name "企业名称或税号" --use-config-apikey
```

配置写入：

```powershell
& ".\bin\windows-amd64\invoice-assistant_windows_amd64.exe" workspace-config-writer --workspace-dir "D:\your-workspace"
& ".\bin\windows-amd64\invoice-assistant_windows_amd64.exe" workspace-config-writer --apikey "<API Key>"
```

- 配置文件为 `.invoice-config/workspace-config.json`，首次读取可自动创建。
- Agent 需要更新配置时必须使用 `workspace-config-writer`。
- 开票预检帮助选择本次已绑定企业；预检通过不代表税局已登录。每个新任务仍须显式提供本次销售方 `uscc`。
- 开票纳税人身份只取任务 `enterprise_context.nsrzg`：`0` 为一般纳税人，`1` 为小规模纳税人；不得使用上次企业的信息或独立画像查询结果代替。
- 默认执行 `task-preflight-check` 时，如果本地已存在 API Key，会返回 `apikey_confirmation_required` 要求先确认；用户确认沿用后才追加 `--use-config-apikey` 重新预检，用户选择更换时先用 `workspace-config-writer --apikey "<新 API Key>"` 写入。
- 新任务未指定销售方且当前 API Key 绑定多个企业时，预检会返回 `enterprise_binding_selection_required`；Agent 必须展示候选并等待用户选择，不得使用本地配置中的上次 `uscc` 直接继续。
- “无权调用该能力”表示当前 API Key 未绑定本 skill 能力或企业，不是税局登录失败。
- `40001 请求校验失败` 不能直接等同为 API Key 未绑定；应先检查 CLI、配置、销售方企业、workspace 和 `invoices[]` 结构。

## 企业购方搜索契约

```powershell
& ".\bin\windows-amd64\invoice-assistant_windows_amd64.exe" issue-search-enterprise --query "<购买方名称或统一社会信用代码>"
```

该命令使用本地配置中的 API Key。缺少配置时先通过 `workspace-config-writer` 写入，不把凭据拼进 URL 或正常输出。

- 企业/单位购方缺少税号时，必须先执行搜索；自然人购方不搜索。
- 只有一个候选时可采用并在预览中展示；多个候选必须展示并等待用户选择，不默认取第一个。
- 用户可不选择候选并继续普通发票草稿。无结果、普通失败或超时也不阻止普通发票草稿；专票缺少购方税号时仍须补齐。
- 出现版本过低提示时，先按安装说明更新，不按普通搜索失败继续。

## issue-batch-invoice-info-check 契约

输入必须是 [field-mapping.md](field-mapping.md) 定义的原始 `invoices[]` JSON 文件。

```powershell
& ".\bin\windows-amd64\invoice-assistant_windows_amd64.exe" issue-batch-invoice-info-check --input <input.json> --pretty
```

执行后使用本次命令返回的任务和 `check_result`。命令会检查输入、补充可确定的字段并生成预览；不能把检查通过当作正式开票成功。销方地址、电话和银行信息可选；企业购方缺少税号时仍须先主动搜索。

关键 `check_result` 字段：

- `validation_passed`
- `missing_fields_by_module`
- `validation_errors`（保持字符串数组；金额错误使用稳定错误码，并直接包含发票/明细序号、输入来源、公式、期望值、实际值、差额、`0.01` 容差和调整建议）
- `purchaser_autofill.needs_user_selection`（表示存在多个购方候选可供选择；普通发票可不选并继续草稿）
- `purchaser_autofill.candidates`
- `invoice_preview_markdown_generated`
- `invoice_preview_markdown_path`
- `invoice_preview_error`

`invoice_preview_markdown_path` 为空且 `validation_passed=false` 是预期结果；应先处理缺失或错误并重跑检查。

金额错误码包括 `LINE_AMOUNT_TAX_RELATION_MISMATCH`、`LINE_UNIT_PRICE_TAX_RELATION_MISMATCH`、`LINE_TAX_INCLUSIVE_TOTAL_MISMATCH`、`LINE_TAX_EXCLUSIVE_TOTAL_MISMATCH`、`LINE_AMOUNT_BASIS_MISSING`、`LINE_PRICE_QUANTITY_PAIR_MISMATCH`。Agent 必须按 [amount-calculation.md](amount-calculation.md) 处理，不得自行覆盖冲突值。

金额、单价的含税与不含税含义见 [field-mapping.md](field-mapping.md)。单价最多十三位小数；有单价无数量时安全补 `quantity=1`，有数量无单价时返回 `check_failed`，必须补单价或删除数量后重新检查。

## issue-from-output 提交与续查

```powershell
& ".\bin\windows-amd64\invoice-assistant_windows_amd64.exe" issue-from-output --confirm-preview --pretty
```

- `--confirm-preview` 表示用户已经明确确认当前完整展示的预览；生成预览的同一轮、未完整展示或模糊同意时禁止携带。不得手工编辑任务状态 JSON 代替确认。
- 使用当前检查通过的任务。用户修改开票内容后，先重新检查并展示新预览，再取得确认。
- 命令提示登录问题时，原样展示实际原因和返回的手动登录网页入口；用户处理后复用同一任务。未知状态、超时、地区限制或未分类错误不自动解释为登录失效，不自行拼接登录地址或自动重试。
- 命中 5 分钟窗口内同内容开票提示时停止提交；用户明确二次确认仍要重复开票后，才允许追加 `--confirm-duplicate`。
- 已有 `taskID` 时，使用同一任务再次运行本命令查询已有结果，不创建新任务。返回仍在处理中时保留任务、说明状态；不能根据单票暂时的失败文案提前判断整个任务终态。
- 旧任务明确为 `code=2015`、`data.status=-1`，且 `issue_completed=false`、没有已开票产物时，完成权限认证后可按命令提示复用原任务恢复一次；不追加 `--confirm-duplicate`，不手工清除任务号。
- 提交结果未知且没有可查询任务号时停止自动动作，先核实是否已开票。恢复失败或结果仍不确定时，不反复提交。
- 读取 `issue_result.issue_result_markdown_path` 并完整原样展示。只有最终成功才能报告开票完成，部分失败须说明实际影响。

## 结果状态

- `data.status=1` 表示任务成功，仍须结合各张发票结果交付。
- `data.status=-1` 表示任务终态失败，按实际原因处理。
- 其他状态继续通过原任务查询。命令返回轮询达到上限时，说明仍在处理中；等待后使用同一任务续查，不另发起开票。

## 权限认证状态

先执行 `issue-auth-workflow --action start --pretty`，用户表示扫码完成后再执行 `issue-auth-workflow --action verify --pretty`。两次均使用当前开票任务。

只有当前返回结果同时包含非空 `issue_auth_result.qrcode_markdown_path`、`issue_auth_result.qrcode_list` 和 `issue_auth_result.qrcode_image_paths`，才展示二维码认证说明。stdout 只返回 Markdown 和 PNG 绝对路径等可展示结果，Agent 按明确路径读取，不扫描目录找文件。

二维码 Markdown 须按平台标明“二维码图片路径：<绝对 PNG 路径>”，保留“国家网络身份认证APP”“电子税局APP”、任选一个扫码和 5 分钟失效提示；不得输出 `MEDIA:`、二维码原文、Base64 或认证编号。

没有上述二维码文件而返回登录地址时，一律称为“手动登录网页入口”，不解释为扫码认证。`verify` 返回 `login_required` 或 `code=2006` 时，原样展示原因及登录地址，用户完成登录后复用同一任务再次校验。

认证结果读取 `issue_auth_result.verify_markdown_path` 并原样展示；只有认证已通过才返回开票流程。详见 [invoice-issue-auth-extend.md](invoice-issue-auth-extend.md)。

## stdout 与 stderr

- CLI stdout 可能包含 JSON 摘要、Markdown 路径或执行状态；stdout 成功不等于用户已经看见结果。
- 调用 CLI 时保持 stdout 与 stderr 分离；Windows PowerShell 命令不追加 `2>&1`，不得将业务 JSON、诊断信息和 PowerShell ErrorRecord 混合后再解析。
- 用户应看到的预检报告、预览、认证、开票结果必须从 Markdown 文件读取并原样输出。
- stderr 或命令输出出现接口异常、DNS、网络、权限、UTF-8 编码或 JSON 解析错误时，按 [error-handling.md](error-handling.md) 分类处理。
- 不向用户展示内部任务状态 JSON 路径、标准 payload 全量内容或调试字段，除非用户明确要求排查。

## 深层参考

- 购方搜索见上方“企业购方搜索契约”；独立企业画像查询见 [common-query-enterprise-profile-by-uscc.md](./common-query-enterprise-profile-by-uscc.md)。
- 批量检查输入与预览操作见 [batch-invoice-info-check.md](batch-invoice-info-check.md)。
- 开票权限延长运行手册见 [invoice-issue-auth-extend.md](invoice-issue-auth-extend.md)。
- 开票检查输入字段表见 [issue.md](issue.md)。
