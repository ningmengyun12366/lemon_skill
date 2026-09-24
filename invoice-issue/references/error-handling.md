# 发票开具错误处理

本文件定义预检、检查、企业候选、权限认证、正式提交和结果轮询的错误处理。通用接口异常文案映射见 [common-handle-response-exception.md](./common-handle-response-exception.md)。

## 预检失败

- 预检失败时，立即停止后续开票流程。
- 必须读取预检命令生成的 Markdown 报告正文并原样展示。
- 不能摘要、脱敏、重排、补充猜测或自行改写报告。
- 预检问题码为 `apikey_confirmation_required` 时，询问用户沿用当前已配置 API Key 还是更换新 API Key；确认沿用后用 `--use-config-apikey` 重新预检，更换时先通过 `workspace-config-writer --apikey "<新 API Key>"` 更新。
- 预检问题码为 `enterprise_binding_selection_required` 时，展示候选销售方企业并等待用户选择；不得使用本地配置中的上次 `uscc` 直接继续。
- 只有重新执行预检并确认通过后，才能继续开票。
- 报告指向 workspace 不可用或需要重新指定目录时，用 `workspace-config-writer` 更新后重新预检。
- 开票预检通过只证明企业绑定和本地配置可用，不证明税局已登录；提交时仍应按命令返回结果处理登录问题。

## 后置登录失败

- `issue-from-output` 返回登录失败且命令错误中包含安全登录地址时，必须把该地址按原始裸文本完整展示给用户，并一律称为“手动登录网页入口”；只提示用户打开网页并按页面指引完成登录，停止正式提交并等待用户完成登录，不得只回复“登录失败”或省略地址。
- 只要当前结果未同时具备非空的 `issue_auth_result.qrcode_markdown_path`、`issue_auth_result.qrcode_list` 和 `issue_auth_result.qrcode_image_paths`，禁止使用“二维码”“扫码”“两个平台”“5 分钟有效”等开票权限认证表述，不得把手动登录网页入口解释为二维码、扫码链接或权限认证入口。
- 用户确认已完成登录后，复用同一任务重新执行 `issue-from-output --confirm-preview --pretty`；不得创建新任务或绕过登录校验。
- 原样展示命令返回的安全登录地址；未返回地址时说明实际错误并停止，不自行拼接或猜测地址。
- `login_status=null` 或 `status_assessment=unknown` 只表示没有确认到登录状态，不表示登录失效。仍必须按标题和 `cause_category` 输出具体原因；超时分别使用 `login_status_query_timeout`、`quick_login_task_check_timeout`、`quick_login_precheck_timeout`、`quick_login_timeout` 或 `login_status_recheck_timeout`。其它请求失败和 `quick_login_not_eligible` 也必须保留实际原因；只能把登录链接表述为可手动尝试的兜底入口，不能表述为必须重新登录。
- 上述未知状态仍须展示命令返回的安全登录地址，作为用户手动尝试登录的兜底入口；地址存在不代表已确认登录失效，Agent 不得自行拼接地址。
- `region_switch_rate_limited` 表示地区切换触发安全限制；必须停止自动重试和跨地区切换，不能把它改写为登录失效；同时保留手动登录兜底入口供用户自行决定是否尝试。
- 未命中已知分类的新错误使用 `unclassified_login_check_failure`，并保留 `raw_cause` 和 `cause_detail`；开票 Agent 原样展示原因和命令返回的安全登录地址后停止，不得补充登录失效结论。
- `retryable=true` 只表示未来可以再次发起，不能覆盖 `auto_retry_allowed=false`；开票 Agent 当前必须停止，等待用户稍后重新发起。

## API Key 与权限错误

- 预检报告、接口响应、stdout 或 stderr 出现“无权调用该能力”时，按 API Key 能力或企业绑定问题处理。
- 本地已存在 API Key 时不能静默沿用；必须通过预检的 `apikey_confirmation_required` 门禁取得用户确认，或让用户写入新的 API Key。
- 原样展示预检 Markdown 后，提示用户按 README 的 API Key 获取方式为当前 API Key 绑定发票开具能力及企业。
- 对“无权调用该能力”不得引导用户去税局登录、重新登录或继续快速登录；它不是登录错误。
- `40001 请求校验失败` 先检查 CLI、API Key 本地配置、企业名称/税号、workspace 配置和 `invoices[]` 结构；只有明确命中无权限、未绑定企业、未绑定能力或企业税务信息未配置，才提示 API Key 绑定问题。
- `issue-search-enterprise` 提示缺少 API Key 时，先用 `workspace-config-writer --apikey "<新 API Key>"` 安全写入后重试，不自行发送搜索请求。

## 接口响应异常

接口响应或 CLI 内部接口返回异常时：

1. 检查响应对象是否存在 `data.code !== 200` 且 `message` 非空，或现有场景定义的异常状态。
2. 命中异常后，按 [common-handle-response-exception.md](./common-handle-response-exception.md) 的表格匹配用户提示。
3. 匹配不到时展示原始 `message`。
4. 展示后立即终止当前流程。

不得追加重试建议、改写文案、包装成新模板或继续后续业务步骤。

## 检查失败或字段缺失

`issue-batch-invoice-info-check` 返回 `validation_passed=false` 或任务状态为 `check_failed` 时：

- 不进入预览确认或正式提交。
- 不调用 `issue-invoice-preview-generator` 试图补生成 Markdown。
- 根据 `check_result.missing_fields_by_module`、`validation_errors`、`tax_rate_validation`、`purchaser_autofill` 一次性向用户补问。
- 批量缺失项必须保留“第几张发票”的索引。
- 原始 input JSON 阶段允许缺失的可选字段，只有被检查结果明确列为阻断项时才追问。
- 税率缺失时，先确认商品编码和任务 `enterprise_context.nsrzg`。`nsrzg` 缺失或非法时，合法人工 `taxpayerType` 可作为人工确认；否则提示 `taxpayer_identity_missing`。发票类型始终逐张必填，不受该身份判断影响。

算术错误码以 `LINE_` 开头时，必须先读取 [amount-calculation.md](amount-calculation.md)：

- 原样保留错误中的发票/明细序号、用户值、系统补算值、公式、期望值、差额和调整建议，一次性向用户说明全部冲突。
- 只建议调整用户明确提供的字段；两个用户字段冲突时展示错误中给出的两种调整方向，不替用户选择。
- 存在单价但数量未提供时，系统默认 `quantity=1` 且正式请求传1；若实际数量不是1，提示用户提供正确的 `quantity`。
- `LINE_PRICE_QUANTITY_PAIR_MISMATCH` 表示用户给了数量但没有任何单价；要求用户补 `price` 或 `taxInclusivePrice` 中的任意一种，或者删除 `quantity`。不得根据总额反推单价。
- 用户确认调整项后更新原始 `invoices[].details[]` 并重新检查、重新生成预览；不得静默修正、绕过检查或沿用旧预览。

示例结构：

```text
已完成开票信息检查，但仍有字段需要补充：

【第1张发票 / 发票明细】
- 纳税人身份缺失；请人工确认一般纳税人/小规模纳税人，或直接提供票种和税率

【第2张发票 / 购方信息】
- 购方企业搜索返回多个参考候选，可选择其一写回；如不选择且当前为普通发票，可继续生成草稿
```

## 购方候选参考与选择

当 `check_result.purchaser_autofill.needs_user_selection=true`：

- 展示候选企业名称、统一社会信用代码和必要区分信息，说明企业搜索仅作参考。
- 用户可以选择正确候选；选择后，把对应 `name`、`uscc` 写回原始 input JSON，并重新执行检查命令。
- 用户也可以不选择；只要当前不是增值税专用发票缺购方税号等阻断场景，就继续普通发票草稿和预览。
- 不能默认选择第一条。
- 若当前发票 `purchaserInfo.isNaturePerson=true`，该企业候选与自然人路径冲突；应重新按自然人购方规则检查，不让用户选择企业候选。

## 预览生成失败

- `validation_passed=true` 但 `invoice_preview_markdown_generated=false` 时，优先查看 `check_result.invoice_preview_error` 并重新执行检查命令。
- `invoice_preview_markdown_path` 为空时，先确认是否检查未通过或预览生成失败。
- 不能手工拼装预览 Markdown。
- 不能用 HTML 预览生成器替代正常 Markdown 预览。

## 缺少确认

- 未收到明确“确认开票”前，停留在预览阶段。
- “可以”“继续”“没问题”“确认”若无法确认是针对当前预览，应追问用户是否确认开票。
- 用户在确认时又提供新的开票要素，按新任务处理，重新检查和预览。
- 用户对当前已完整展示预览明确回复“确认开票”后，执行 `issue-from-output --confirm-preview --pretty`；不得手工修改 `invoice_preview_confirmed`。
- 若已取得明确确认却返回“发票预览尚未确认”，先检查本次命令是否漏了 `--confirm-preview`；该错误是预览状态门禁，不得当作中文编码问题反复执行同一条缺参命令。

## 正式提交状态不确定

- 已有 `taskID` 时使用同一任务继续运行 `issue-from-output --confirm-preview --pretty` 查询结果；不得另建任务再次提交。
- 旧任务明确返回 `code=2015`、`data.status=-1`，且 `issue_completed=false`、没有已开票产物、权限认证已通过时，按命令提示复用原任务恢复一次；不追加 `--confirm-duplicate`，不手工删除任务号。
- 提交或权限恢复结果不确定、没有可查询任务号时，停止自动动作，提示用户先核实税局或平台记录，防止重复开票。
- 命令提示同内容重复开票时，先说明风险；仅在用户明确二次确认后追加 `--confirm-duplicate`，不得默认携带。
- 登录状态查询失败、未知状态、地区限制或未分类异常时，原样展示实际原因及命令返回的手动登录网页入口，停止自动动作；用户处理后复用同一任务。
- CLI 成功返回但结果 Markdown 缺失时，核对本次返回路径，不能用摘要替代正文或声称完成。

## 查询超时或未终态

- 命令已结束但返回仍在处理中时，说明当前状态，等待后使用同一任务续查，不重新创建。
- `data.status` 不是 `-1` 时，不能仅因单票结果中的失败文案判断整个任务已失败。
- 明确 `data.status=-1` 时展示实际失败原因；成功或失败结果均读取 `issue_result.issue_result_markdown_path` 并原样展示。
- 网络、DNS、超时或进程中断不证明已失败；无法确认是否提交成功时停止自动提交。

## 权限认证错误

二维码和认证状态按以下规则处理：

- `start` 返回登录问题时，原样展示原因和手动登录网页入口；没有二维码文件时不提供扫码说明。
- 只有当前命令结果同时包含非空的 `issue_auth_result.qrcode_markdown_path`、`issue_auth_result.qrcode_list` 和 `issue_auth_result.qrcode_image_paths`，才允许使用二维码、扫码、两个平台和 5 分钟有效等认证表述。
- 每个二维码必须展示平台名称；“国家网络身份认证APP”和“电子税局APP”任选一个扫码成功即可，不要求两个都扫。
- 二维码状态 `1`：待扫码，按 Markdown 中的“二维码图片路径：<绝对 PNG 路径>”告知用户对应二维码的本地文件路径，并提示二维码 5 分钟失效、用户扫码后再回复。
- 二维码状态 `3`：二维码已失效，提示重新获取二维码。
- 二维码状态 `2`：继续查询认证状态。
- 认证状态 `status=1` 且 `isAuth=true`：认证通过，返回当前开票流程。
- 认证状态 `status=0/2`：处理中，提示稍后再次校验。
- 认证状态 `status=-1`：认证失败，提示重新发起认证。
- 扫码成功后认证状态接口返回 `code=2006`：保留扫码成功信息，按 `login_required` 提示原样展示实际原因和安全登录入口；用户登录后复用同一任务再次执行 `verify`，不得要求重新填写开票内容。

认证 Markdown 必须从 stdout JSON 给出的 `issue_auth_result.qrcode_markdown_path` 或 `issue_auth_result.verify_markdown_path` 读取并原样展示；二维码路径必须以“二维码图片路径：<绝对 PNG 路径>”明确标注。不得扫描认证目录，不得输出 `MEDIA:`、`data:image/png;base64,...`、二维码原文、Base64、`rzid` 或内部认证编号。

## 编码和命令错误

- PowerShell 写入 JSON 时优先使用对象、`ConvertTo-Json` 和 .NET UTF-8 无 BOM。
- 中文显示乱码时，用 `Get-Content -Raw -Encoding UTF8 <input.json> | ConvertFrom-Json` 做 round-trip 验证。
- `Unexpected UTF-8 BOM` 表示编码兼容问题，重写为 UTF-8 无 BOM 或使用兼容读取。
- JSON parse error 先检查文件编码、转义、根对象是否为 `invoices[]` 结构。
- DNS、TLS、代理或网络错误不应转化成业务字段错误；报告网络/环境问题并停止当前自动推进。

## 敏感输出

- 不主动展示任务状态 JSON 路径、预览路径、正式 payload、API Key、token、密码、短信验证码、二维码截图或人脸信息。
- 二进制生成的 Markdown 原文可以展示；内部路径和调试 JSON 不作为正常用户输出。

## 联系客服

```text
遇到问题请点击 {{OPEN_API_CONTACT_SERVICE_URL}} 扫码添加我们的专属客服企业微信联系我们
```
