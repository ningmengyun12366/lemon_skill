# 发票开具流程

本文件说明发票开具的操作流程：预检、输入检查、预览、明确确认、正式提交和结果查询。业务请求统一通过 CLI 执行。

## 新任务与继续任务

- 用户本轮重新提供购方、商品、金额、税率、票种、备注、文件内容或其它开票要素时，判定为新任务。
- 新任务必须重新整理本轮输入，生成顶层 `invoices[]` input JSON，并重新执行 `issue-batch-invoice-info-check` 创建新的任务状态。
- 只有用户明确表达“确认开票”“继续刚才那张”“继续上一条预览”“查看刚才开票结果”时，才允许按继续任务处理。
- 如果同一输入既包含“继续/确认”又包含新的开票要素，优先按新任务处理。
- 无法判断时默认按新任务处理，不能使用旧任务或旧预览代替本次检查。

## 正常执行顺序

1. 完成二进制可用性检测，并按 [api-contracts.md](api-contracts.md) 的预检命令执行 `task-preflight-check`；若用户已提供销售方企业名称或税号，追加 `--enterprise-name`。预检帮助选择本次绑定企业，预检通过不代表税局已登录。
2. 预检通过后，识别单张或批量开票；即使只有 1 张票，也写入顶层 `invoices[]`。
3. AI/Agent 先判断每张购方是自然人还是企业/单位，并写入 `invoices[].purchaserInfo.isNaturePerson`。购方其它字段写入 `invoices[].purchaserInfo`，明细写入 `invoices[].details[]`；字段映射见 [field-mapping.md](field-mapping.md)。
4. 若某张发票的购方被判断为企业/单位/个体工商户/其他组织，且用户未提供购方统一社会信用代码或纳税人识别号，必须先主动执行 `issue-search-enterprise --query "<购方名称>"`。该命令使用本地配置中的 API Key，缺少时先通过 `workspace-config-writer` 配置。查询到 1 个候选时可写回 `purchaserInfo.name` 和 `purchaserInfo.uscc`；查询到多个候选时必须展示候选并等待用户选择，用户也可以明确表示不选择并继续普通发票草稿；查询无结果、普通失败或超时时，说明未自动补齐税号并继续普通发票草稿。若错误包含“当前 skill 版本过低，无法执行”，必须优先执行自动更新，不得按普通查询失败继续。自然人购方不得调用该命令。
5. 在 Windows 下用 PowerShell 生成 UTF-8 JSON 文件；顶层 `uscc` 和每张 `invoices[].invoiceType` 必须显式提供。再用 `--input <file>.json` 调用 `issue-batch-invoice-info-check --pretty`。使用命令返回的本次任务路径继续。
6. 读取本次检查命令刚创建的任务状态和 `check_result`。
7. 若 `validation_passed=false`，按 [error-handling.md](error-handling.md) 处理缺失项、候选企业或校验错误。
8. 若 `validation_passed=true` 且已生成预览 Markdown，读取 `check_result.invoice_preview_markdown_path` 指向的 Markdown 文件并原样展示；其中所有 HTTP/HTTPS 链接和本地路径都必须按裸文本原样输出，不得包装成“点击查看”“打开文件”等短文本链接。检查命令成功后不得以“正在构造 JSON”“现在执行检查”或类似进度描述结束本轮；本轮只有在完整展示预览和确认提示后才能结束。
9. 等待用户明确回复“确认开票”。模糊同意、沉默、继续执行类表述都不能替代该确认。
10. 用户对当前已展示预览明确回复“确认开票”后，调用 `issue-from-output --confirm-preview --pretty` 基于当前任务状态提交。`--confirm-preview` 只能表示已取得用户对当前预览的明确确认；生成预览的同一轮、预览未完整展示或仅有模糊同意时禁止携带。命令提示登录问题时停止开票；登录检查失败且命令返回安全登录地址时，必须按原始裸文本完整展示该地址，一律称为“手动登录网页入口”，只提示用户打开网页并按页面指引完成登录，停止正式提交，之后复用同一任务重试。只要当前结果未同时具备非空的 `issue_auth_result.qrcode_markdown_path`、`issue_auth_result.qrcode_list` 和 `issue_auth_result.qrcode_image_paths`，禁止使用“二维码”“扫码”“两个平台”“5 分钟有效”等开票权限认证表述。查询失败、状态未知、地区切换限制或未分类异常仍必须报告实际原因，不得因链接存在而判断登录失效，也不得自动重试。不得手工修改 `invoice_preview_confirmed`。
11. 提交后读取命令的最终状态；仍在处理时复用同一任务查询，不能新建任务重复提交。结果 Markdown 从 `issue_result.issue_result_markdown_path` 读取并原样展示；PDF 原件和 XML/OFD/PDF 附件等链接均保留裸文本。

## 预检与 workspace

- 每次进入业务执行前都要先跑预检；预检失败时按 [error-handling.md](error-handling.md) 原样展示报告并停止。
- 开票预检用于核对本地配置和选择绑定企业，不能据此宣称税局已登录。
- 预检返回 `apikey_confirmation_required` 时，先询问用户沿用当前已配置 API Key 还是更换新 API Key；用户确认沿用后，重新执行 `task-preflight-check --use-config-apikey`，若同时指定销售方则保留 `--enterprise-name`。
- 新开票任务必须显式提供销售方 `uscc`；不得使用上次销售方或单企业绑定自动补齐。以本次检查结果为准。
- `workspace_dir` 未配置时，不视为业务失败；默认使用 `workspace` 目录。
- 用户要求切换目录、目录不可写或预检报告指向工作区问题时，使用 `workspace-config-writer` 更新配置后重新预检。
- 不能手工编辑 `.invoice-config/workspace-config.json` 代替配置命令。

## 批量 invoices[] 流程

- 单张和批量都统一使用根对象 `invoices[]`，不得使用顶层数组、`params.info[]`、顶层 `info[]` 或正式 payload 的 `invoiceDetail.data[]` 作为检查输入。
- 用户用编号、换行或分号列出多组“购方/商品/金额/税率/票种”时，每组可独立成票的信息应拆成一张发票。
- 每张发票独立校验购方、票种、明细、金额和税率。
- 每张发票必须由用户显式确认票种；缺失或非法时任务状态为 `check_failed`，不生成预览。
- 缺失项必须保留“第几张发票”的索引，不得把多张票混成一组。

## 预览 Markdown 原样展示

- 正常流程的预览 Markdown 由 `issue-batch-invoice-info-check` 在校验通过后生成。
- 必须先读取 `check_result.invoice_preview_markdown_path` 指向的本地 Markdown 文件，再完整原样展示正文。
- 不能用“已生成预览”“详见文件路径”或摘要替代正文。
- 如果检查命令已成功但尚未产生用户可见的完整预览，重新读取当前 `invoice_preview_markdown_path` 并完整输出一次；不得重新创建任务或重复执行检查。
- 预览 Markdown 中所有 HTTP/HTTPS 链接和本地路径都必须以裸文本逐行展示，例如 `C:/Users/<用户名>/.qclaw/invoice-workspace/invoice-issue/.../发票预览/<预览文件>.html`；不得包装成 `[点击查看](...)`、`[打开文件](...)`、HTML 链接或任何短文本可点击链接。
- 展示后必须自检发票明细和“预览发票票样链接”是否完整出现；若缺失、截断或漏行，先重新完整展示。
- `issue-invoice-preview-generator` 不是正常开票后续步骤；它不写回任务状态，也不生成 `invoice_preview_markdown_path`。

## 自动补全与候选处理

检查命令提供当前企业信息、商品编码、税率和金额补全结果。使用本次返回结果生成预览，发票类型必须事先由用户明确。

- 自然人购方按 [field-mapping.md](field-mapping.md) 处理；`purchaserInfo.isNaturePerson=true` 时不走企业搜索。
- 企业购方缺少税号时必须先主动调用 `issue-search-enterprise --query "<购方名称>"`；不能跳过用户可见的候选查询，也不能跳过查询直接展示预览。
- 企业购方搜索到 1 个高置信候选时可采用并在预览中体现。
- 企业搜索仅作参考：多个候选应展示给用户作为可选项，用户可以选择其中一个写回，也可以不选；不能默认取第一个。
- 企业搜索无结果、失败、超时或用户暂不选择时，不阻止普通发票草稿和预览；增值税专用发票缺少购方税号仍按阻断缺失项处理。
- 纳税人身份只取任务 `enterprise_context.nsrzg`：数值或字符串 `0` 为一般纳税人，数值或字符串 `1` 为小规模纳税人。`nsrzg` 缺失或非法时，合法人工 `taxpayerType` 可作为人工确认；发票类型仍必须逐张显式提供。
- 税率缺失时保持缺失进入检查；身份明确且商品编码确定后，由检查命令按纳税人身份和商品字典匹配最低参考税率。
- 销方地址、电话、开户银行和银行账号均可选：有值传、无值省略。销方名称以用户输入优先；未提供时只从任务 `enterprise_context.enterpriseName` 补齐。
- 含税金额很小导致按税率计算或四舍五入后的明细税额、合计税额为 0 时，税额 0 是合法计算结果；不得仅因税额为 0 阻断草稿或预览。

## 正式提交与结果轮询

- 正式提交只使用检查通过且用户确认后的当前任务，执行 `issue-from-output --confirm-preview --pretty`。
- 命令返回登录问题时，原样展示原因和手动登录网页入口。查询失败、未知状态、地区限制或未分类异常不自动解释为登录失效；用户处理后复用同一任务。
- 5 分钟窗口内同内容开票可能被阻断。说明重复开票风险；只有用户明确二次确认仍要重复开票后，才允许追加 `--confirm-duplicate`。
- 已有 `taskID` 时，使用同一任务再次执行本命令查询结果，不另建任务。仅修改文件名、路径或时间不能作为重新开票的依据。
- 旧任务明确返回 `code=2015`、`data.status=-1`，且 `issue_completed=false`、无已开票产物、权限认证已通过时，按命令提示复用原任务恢复一次，不使用 `--confirm-duplicate`，不手工删除任务号。
- 提交结果未知且没有可查询任务号时，停止重试并先核实税局或平台记录。
- 未到终态只报告处理中；结果失败须按真实原因说明，不误报成功。

## 权限认证分支

开票失败原因包含“请先扫码验证获取开票权限”或等价语义时：

1. 停止当前正式提交路径。
2. 向用户说明需要延长开票权限。
3. 用户确认后进入 [invoice-issue-auth-extend.md](invoice-issue-auth-extend.md)。命令返回登录问题时原样展示实际原因，并把安全登录地址称为“手动登录网页入口”。
4. 只有当前命令结果同时包含非空的 `issue_auth_result.qrcode_markdown_path`、`issue_auth_result.qrcode_list` 和 `issue_auth_result.qrcode_image_paths`，才允许把用户可见内容描述为二维码认证；按 stdout JSON 给出的 Markdown 路径读取文件，使用“二维码图片路径：<绝对 PNG 路径>”明确告知用户每个平台对应的本地二维码文件。不得扫描目录，不得输出 `MEDIA:`、二维码原文、Base64、`rzid` 或内部认证编号；必须明确任选一个扫码即可，并展示 5 分钟失效提示。
5. 认证通过后返回当前本地开票任务继续，不重新解释为新任务；按前述失败状态和恢复条件继续；结果未知时停止，不反复提交。

## 深层参考

- CLI 参数、stdout 和任务状态契约见 [api-contracts.md](api-contracts.md)。
- 字段映射、自然人/企业搜索、金额口径和税率规则见 [field-mapping.md](field-mapping.md)。
- 风险、重复开票、敏感输出和 API Key 边界见 [risk-boundaries.md](risk-boundaries.md)。
- 预检、接口异常、候选企业和权限认证失败处理见 [error-handling.md](error-handling.md)。
- 完整调用示例见 [invoice-issue.md](invoice-issue.md)。
