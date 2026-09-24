---
name: "invoice-issue"
display_name: "柠檬云全场景开票"
display_name_en: "invoice-issue"
description: "发票开具与开票权限处理。当用户提出以下需求时触发：发票开具、数电票开具、数电票开票、蓝字发票开具、单张开票、批量开票、开票二维码获取、开票二维码状态查询、开票授权、开票权限校验、开票权限延长、扫码授权开票"
description_zh: "提供单张、批量及数电票开具、开票授权和开票结果查询能力。"
description_en: "Supports single, batch, and digital invoice issuance, authorization, and issuance result queries."
version: "0.1.6"
---

# 发票开具

本 Skill 是发票开具的短执行入口。Agent 负责把用户输入整理成安全的开票任务，走本地 CLI 预检、草稿检查、预览、明确确认、正式提交、权限认证和结果查询链路；详细字段、命令、风险和错误规则按下方参考路由读取。

## PowerShell UTF-8 初始化（Windows 强制）

在 Windows 上，只要使用 PowerShell 调用本 Skill 的本地可执行文件，必须在同一个 PowerShell 进程中、首次调用可执行文件之前执行：

```powershell
$Utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[Console]::InputEncoding = $Utf8NoBom
[Console]::OutputEncoding = $Utf8NoBom
$OutputEncoding = $Utf8NoBom
```

- 若执行工具会为每条命令新建 PowerShell 进程，必须把上述初始化和本地可执行文件调用放进同一个脚本块；不得只在上一条已经结束的 PowerShell 命令中初始化。
- 该初始化只规范 PowerShell 与本地可执行文件之间的控制台、标准输入、标准输出和管道编码，不会改变 HTTP 请求编码，也不能替代文件编码控制。
- 创建传给本地可执行文件的 JSON 或文本文件时，必须写成 UTF-8 无 BOM；使用 `[IO.File]::WriteAllText($path, $content, $Utf8NoBom)`，不得依赖 `Set-Content`、`Out-File` 的默认编码。

## 安装、版本检测与自动更新

安装或重装本 skill 时，按 `{{INSTALL_DOC_URL}}` 执行。

本 skill 会自动检测版本；版本不符合要求时会阻断执行。若命令返回包含“当前 skill 版本过低，无法执行”的报错，Agent 立即从报错提供的安装文档链接获取最新教程，备份当前 skill 后完成更新与重装；验证成功后删除备份并重试原业务，失败时恢复备份并说明原因。该规则优先于任何普通错误重试、降级或继续执行规则。

用户也可随时说“更新并重装skill”，Agent 按同一流程立即执行。

## 触发与任务判定

触发：发票开具、开票、数电票开具、蓝字发票开具、单张开票、批量开票、开票权限认证、开票权限延长、开票结果查询。

先判定是新任务还是继续任务：

- 新任务：用户重新提供或修改购方、销方、票种、商品、金额、税率、备注、文件内容等任一开票要素。新任务必须重新整理 input JSON，重新执行 `issue-batch-invoice-info-check`，重新生成预览。
- 继续任务：用户明确承接刚才同一任务，例如 `确认开票`、继续刚才那张、查看刚才开票结果、我已完成扫码。只在这种情况下复用当前任务状态。
- 无法确定时按新任务处理，不得复用历史预览、历史 task JSON 或历史正式 payload。

## 快速开始（Agent Quickstart）

1. README 门禁：首次使用发布包，或用户询问如何使用、如何配置时，必须读取 `./README.md`，并保留其中面向用户的准备说明。
2. 确认 CLI 可用：使用当前平台的 `invoice-assistant` 二进制；若本会话尚未证明可运行，先执行 `--help`。若二进制缺失，按安装文档处理，不得猜测下载地址。
3. 确认本地配置：工作区只保存 API Key 和 workspace，不保存“当前企业”、`uscc` 或 `companyInfo`。不得要求用户公开任何密钥；应通过 `workspace-config-writer` 或文档中的安全配置路径引导本地写入。
4. 业务执行前先预检：执行 `task-preflight-check`；若用户已提供销售方企业名称或税号，可追加 `--enterprise-name`。开票预检帮助选择本次已绑定企业，预检通过不代表税局已登录。若返回 `apikey_confirmation_required`，先询问用户沿用当前已配置 API Key 还是更换新 API Key；用户确认沿用后，重新执行预检并追加 `--use-config-apikey`。若缺少 `workspace_dir`，除非用户要求其他可写目录，否则使用默认 `workspace` 目录。
5. 写入每张发票前，AI/Agent 必须先判断购方是自然人还是企业/单位，并把结果写入 `invoices[].purchaserInfo.isNaturePerson`。
6. 若购方被判断为企业/单位/个体工商户/其他组织，且用户未提供购方统一社会信用代码或纳税人识别号，必须先主动执行 `issue-search-enterprise --query "<购方名称>"` 作为税号参考查询；不得跳过该命令直接生成预览。该命令使用本地配置中的 API Key；缺少配置时先通过 `workspace-config-writer` 配置。查询到 1 个候选时可写回购方名称和税号；查询到多个候选时必须先展示候选让用户选择，也要明确用户可回复“不选择/继续”来生成不带购方税号的普通发票草稿；查询无结果、普通失败或超时时，说明未自动补齐税号并继续普通发票草稿。若错误包含“当前 skill 版本过低，无法执行”，必须优先执行自动更新，不得按普通查询失败继续。
7. 构造标准原始输入：根对象必须显式包含本次销售方 `uscc` 和顶层 `invoices[]`；即使只开 1 张发票，也必须是 `invoices[]` 中的 1 个元素。创建 input JSON 前必须逐张确认 `invoiceType`，只允许普通发票或增值税专用发票及文档声明的别名，Agent 不得根据纳税人身份自行推断票种。
8. 执行 `issue-batch-invoice-info-check --input <input.json> --pretty` 创建当前任务。读取本次命令返回的检查结果、任务路径和预览，不使用旧任务结果代替。
9. 若检查结果返回阻断缺失字段，必须带上发票序号一次性询问；若返回企业购方候选，企业搜索仅作参考，用户可选择其中一个写回，也可不选并继续普通发票草稿/预览，不能默认取第一条。
10. 预览 Markdown 生成后，读取预览 Markdown 文件并原样渲染。所有网页链接和本地链接都必须按文件中的原始裸文本逐行展示，不得包装成“点击查看”“打开发票”“下载 PDF”等短文本链接。执行检查后不得以“正在构造 JSON”“现在执行检查”或类似进度描述结束本轮；若 `validation_passed=true` 且已生成预览 Markdown，本轮只有在完整展示预览并提示用户回复 `确认开票` 后才能结束；模糊回复不能作为正式确认。
11. 只有在已完整展示当前预览后，收到用户明确 `确认开票` 时，才执行 `issue-from-output --confirm-preview --pretty`。`--confirm-preview` 表示 Agent 已取得用户对当前预览的明确确认；生成预览的同一轮、尚未展示完整预览或用户仅模糊同意时禁止携带该参数。命令提示重复开票或登录问题时，按实际原因停止并处理，不能把阻断当作提交成功。命令错误中的安全登录地址必须按原始裸文本完整展示，并一律称为“手动登录网页入口”，只提示用户打开网页并按页面指引完成登录；用户确认登录完成后复用同一任务重试。只要当前结果未同时具备非空的 `issue_auth_result.qrcode_markdown_path`、`issue_auth_result.qrcode_list` 和 `issue_auth_result.qrcode_image_paths`，禁止使用“二维码”“扫码”“两个平台”“5 分钟有效”等开票权限认证表述，不得把手动登录网页入口解释成二维码、扫码链接或权限认证入口。若错误属于登录状态查询失败、快速登录请求失败、`status_assessment=unknown`、`region_switch_rate_limited` 或未分类异常，仍须先原样说明实际原因，不得判断登录失效、不得自行拼接安全登录地址、不得自动重试；链接存在只表示提供人工入口。不得手工修改任务状态中的 `invoice_preview_confirmed`，也不得自行构造或提交正式开票 payload。
12. 若需要开票权限认证，执行 `issue-auth-workflow --action start --pretty`。命令返回登录问题时，原样展示实际原因和手动登录网页入口。成功时直接解析 stdout JSON；只有其中同时包含非空的 `issue_auth_result.qrcode_markdown_path`、`issue_auth_result.qrcode_list` 和 `issue_auth_result.qrcode_image_paths`，才允许读取认证 Markdown。Markdown 必须在每个平台名称下使用“二维码图片路径：<绝对 PNG 路径>”明确告知用户这是对应二维码的本地文件路径；禁止输出 `MEDIA:`、`data:image/png;base64,...`、任何二维码 Base64、二维码原文、`content`、`rzid` 或内部认证编号，也禁止在目录中自行搜索认证文件。Markdown 必须标明“国家网络身份认证APP”“电子税局APP”和 5 分钟失效提示；明确告知用户任选一个二维码扫码即可，不得要求两个都扫。等待用户说明已完成后，再执行 `issue-auth-workflow --action verify --pretty`。扫码成功后若认证状态接口返回 `code=2006`，必须保留扫码成功状态，并把命令返回的安全登录地址称为“手动登录网页入口”；用户重新登录后复用同一任务再次校验。只有认证通过后才返回开票流程。
13. 当前远端任务已明确以 `code=2015`、`data.status=-1` 失败，且本地记录 `issue_completed=false`、不存在已开票产物、当前任务权限认证已经通过时，再次执行 `issue-from-output --confirm-preview --pretty`。按命令提示恢复一次；结果未知或仍被阻断时停止，不使用 `--confirm-duplicate`，不得手工删除或改写任务号。
14. 正式提交后读取结果；仍在处理中时使用同一任务续查：使用 CLI 管理的任务状态和状态轮询结果，原样渲染生成的结果 Markdown，并按真实状态报告成功、失败或仍在处理中，不得编造结果。

## 链接展示硬规则

预检、预览、认证、开票结果、下载地址、本地文件路径等用户可见内容中，只要出现 HTTP/HTTPS 链接或本地路径，Agent 必须按原文裸文本展示：

```text
预览发票票样链接：

C:/Users/<用户名>/.qclaw/invoice-workspace/invoice-issue/<纳税人识别号>_<任务时间>/发票预览/<预览文件>.html
```

- 不得把链接改写为 `[点击查看发票票样](...)`、`[打开文件](...)`、`[发票PDF下载链接](...)` 或任何 Markdown 包装链接。
- 不得把链接改写为 HTML `<a href="...">...</a>`。
- 不得只展示“点击查看”“点击下载”“打开发票”等短文本。
- 不得重新编码、脱敏、截断、换参、改参数名、改 query 顺序或对 OSS 签名链接做任何二次处理。
- 如果需要补充说明，说明文字必须放在链接前后，链接本身仍单独一行裸文本展示。

## 标准 invoices[] 输入

正常执行写入的是原始检查入参，不是正式开票 payload：

```json
{
  "uscc": "销售方税号",
  "invoices": [
    {
      "invoiceType": "普通发票",
      "purchaserInfo": {
        "name": "购买方名称"
      },
      "details": [
        {
          "projectName": "商品或服务名称",
          "taxInclusiveAmount": 100
        }
      ]
    }
  ]
}
```

输入规则：

- 根节点必须是带 `invoices[]` 的对象，不得使用顶层数组。
- 顶层 `uscc` 代表本次销售方税号，必须由本次用户明确提供或选择；即使 API Key 只绑定一家企业也不能省略。不得直接复用上次企业。
- 纳税人身份只取任务 `enterprise_context.nsrzg`：`0` 为一般纳税人，`1` 为小规模纳税人。`nsrzg` 缺失或非法时，可接受合法人工 `taxpayerType` 作为人工确认，否则提示 `taxpayer_identity_missing`；不得用独立画像查询或上次企业信息代替。
- 每张发票都必须显式提供 `invoiceType`。缺失或非法时脚本仍创建 `check_failed` 任务并准确列出发票序号，但不生成可提交预览；小规模纳税人也不得默认补普通发票。
- 销方地址、电话、开户银行和银行账号均为可选字段：用户提供时传入，未提供时省略。销方名称以用户输入优先；未提供时只使用任务快照中的 `enterpriseName`。
- 每张发票使用 `invoices[].purchaserInfo` 和 `invoices[].details[]`。
- 每张发票的购方类型由 AI/Agent 先判断：自然人写入 `purchaserInfo.isNaturePerson: true`，企业/单位写入 `purchaserInfo.isNaturePerson: false` 或保留企业购方字段。
- 企业/单位购方缺少 `purchaserInfo.uscc` 时，必须先用 `issue-search-enterprise --query "<购方名称>"` 主动查询候选；不能因为普通发票允许无税号草稿，就省略查询步骤。
- 除非用户明确说不含税，金额和单价默认按含税口径处理。
- 用户提供任一种单价但未提供数量时，系统默认 `quantity=1` 并在正式请求中传1；用户提供数量但没有任何单价时必须停止并要求补一种单价或删除数量，不得从总额反推单价。
- 含税金额很小导致按税率计算或四舍五入后的税额为 0 时，税额 0 是合法计算结果；仍允许生成草稿和预览，不得仅因税额为 0 阻断校验。
- 不得把 `params.info[]`、顶层 `info[]`、`invoice_payload_json.info[]`、`invoiceDetail.data[]`、`goods[]`、`items[]` 或自造别名作为检查入参。
- 来自文件、PDF、Word、Excel、图片或压缩包的输入，必须先提取成明确的开票字段，再写入 `invoices[]`；识别不稳、票据边界不清或核心字段冲突时，必须先确认再继续。

## 安全规则

- 预览确认是硬门禁：先执行检查并完整渲染预览，再等待用户明确 `确认开票`，之后才允许执行 `issue-from-output --confirm-preview --pretty`。`--confirm-preview` 只能映射用户对当前已展示预览的明确确认，不得由 Agent 预先或自行断言。
- 防重复开票是 5 分钟窗口内的硬门禁：重复按规范化后的发票内容是否相同判断。同内容任务在 5 分钟内无论处理中、已提交、状态未知或已完成，都阻断新的正式提交；超过 5 分钟后不再因历史同内容任务阻断。
- 预检、预览、认证或结果命令生成的 Markdown 必须原样渲染。不得摘要、改写、脱敏、重排，也不得用文件路径替代正文；其中任何 HTTP/HTTPS 链接和本地路径都必须裸文本展示，不得包装成 Markdown/HTML 可点击短文本。
- 正常用户输出中不得暴露内部 task JSON 路径、正式 payload JSON、token、cookie、签名、API Key、税局密码、短信验证码、二维码截图或人脸认证材料。
- 输出包含 `无权调用该能力` 时，必须停止业务流程。该问题按 API Key、能力权限或企业绑定配置处理，不得解释为税局登录失败。
- 正式开票必须使用已检查、已展示预览且取得用户确认的 `issue-from-output`，不自行构造 HTTP 开票请求。
- 检查输入字段见 `./references/issue.md`，先按字段映射准备 `invoices[]`。
- 不得绕过预检、`issue-batch-invoice-info-check`、预览、明确确认、权限认证、防重复检查或结果轮询。
- `issue-from-output`、认证和状态续查只能使用任务中的 `enterprise_context`；命令提示旧任务缺少企业信息时按提示重新检查，不手工修改任务数据；已有提交结果不明时先核实状态。

## 参考路由

- 正常流程、新任务/继续任务、检查-预览-确认-提交、认证分支、续查和防重复处理：`./references/workflow.md`。
- 自然语言或文件输入到 `invoices[]` 的映射、购方规则、金额/税率/折扣/煤炭规则，以及 UTF-8 JSON 输入说明：`./references/field-mapping.md`。
- `validation_errors` 出现 `LINE_AMOUNT_*`、`LINE_UNIT_PRICE_*`、`LINE_*TOTAL_MISMATCH` 或 `LINE_PRICE_QUANTITY_PAIR_MISMATCH` 时的金额补算矩阵、成对规则、校验公式和调整方法：`./references/amount-calculation.md`。
- CLI 命令、预检/本地配置、输入输出、`issue-batch-invoice-info-check`、`issue-from-output`、`issue-auth-workflow`、任务状态和结果续查：`./references/api-contracts.md`。
- 误开票防控、重复开票阻断、敏感输出边界、API Key 边界、认证边界和人工停止条件：`./references/risk-boundaries.md`。
- 预检失败、API 异常、缺失字段、购方候选选择、认证失败、部分/未知开票状态和最终兜底处理：`./references/error-handling.md`。

按需读取补充说明：预检见 `./references/common-preflight-initialization-check.md`；通用错误提示见 `./references/common-handle-response-exception.md`；批量输入和预览见 `./references/batch-invoice-info-check.md`；权限认证见 `./references/invoice-issue-auth-extend.md`；输入字段表见 `./references/issue.md`；操作示例见 `./references/invoice-issue.md`。
