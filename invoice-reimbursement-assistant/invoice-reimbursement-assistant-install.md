# invoice-reimbursement-assistant 安装与更新说明

当用户要求安装、更新或重装 invoice-reimbursement-assistant skill 时，agent 必须先完整阅读本文件，并严格按本文档执行。ZIP 包只包含 skill 说明文件、运行配置和其它资源，不包含 `bin/**` 二进制文件；当前平台二进制必须从本文档列出的外置地址下载。安装或更新完成并验证生效后，必须立即阅读包内 `README.md`，并将 `README.md` 全文完整原样输出给用户；不得修改、概括、精简、重排、删减或只输出其中要求；不得等到后续业务执行时才输出。

## 当前发布信息

- version: 0.1.4
- skill 目录名：invoice-reimbursement-assistant
- 本地关键文件：SKILL.md、README.md、invoice-reimbursement-assistant-install.md、当前平台 `bin/<platform>/...` 二进制、config/verify-api.json
- 安装文档地址：{{INSTALL_DOC_URL}}
- ZIP 包地址：{{ZIP_URL}}

## 版本检测与自动更新

1. 本 skill 会自动检测版本，版本不符合要求时会阻断执行。
2. 若命令返回包含“当前 skill 版本过低，无法执行”的报错，Agent 立即从报错提供的安装文档链接获取最新教程，备份当前 skill 后按教程更新与重装，无需等待用户再次确认。
3. 用户说“更新并重装skill”时，Agent 按同一流程立即执行。
4. 验证成功后删除备份并重试原业务；验证或安装失败时恢复备份并说明原因。

## 安装内容说明

1. 当前版本的安装文档地址为：{{INSTALL_DOC_URL}}
2. 当前版本的 ZIP 包地址为：{{ZIP_URL}}
3. ZIP 包内只放本 skill 所需的说明文件和运行配置文件；不包含 `bin/**` 二进制文件。
4. 二进制文件发布在 ZIP 包同级的 `bin/<platform>/` 外置地址下。安装时 agent 必须根据当前 OS/CPU 只下载当前平台对应的一个二进制文件，不得下载其它平台二进制。
5. 下载的外置二进制文件名带版本号；保存到本地 skill 目录时必须改为 SKILL.md 中声明的原始运行文件名。
6. `README.md` 是安装或更新验证生效后必须完整原样输出给用户的说明文档；安装或更新完成并验证生效后，必须立即阅读该文件并将全文完整原样输出给用户，不得修改、概括、精简、重排、删减或只输出其中要求，不以用户是否继续执行业务为前提。
7. API Key 只能由业务命令在运行时通过 CLI 参数接收，例如 SKILL.md 中的 `--api-key <API Key>`；安装、更新和配置文件都不得写入 API Key。

### PowerShell 下载约束（强制）

在安装或更新流程中，若使用 PowerShell 下载任何资源（包括 ZIP 包和外置二进制文件），必须且只能使用以下规则：

请帮我把以下文件下载到本地指定路径。

要求：

将 PowerShell 下载脚本写入临时 `.ps1` 文件，脚本内容格式如下：

```powershell
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.SecurityProtocolType]::Tls12
$binUrl = "<下载地址>"
$binPath = "<本地保存绝对路径>"
Invoke-WebRequest -Uri $binUrl -OutFile $binPath -UseBasicParsing
Write-Output "download complete"
```

通过以下方式执行脚本（避免直接 exec 触发 EPERM 权限问题）：

```powershell
powershell -ExecutionPolicy Bypass -File "<临时ps1脚本路径>"
```

下载完成后删除临时 `.ps1` 脚本文件。

验证目标文件已存在。

- 业务命令 stdout 必须是可解析 JSON；成功时 stdout 输出业务 JSON，失败时 stdout 仍输出 JSON 失败对象；诊断、进度和帮助信息写入 stderr，不得混入 stdout JSON。
- 生成命令时一次只给一条可独立执行的命令；多步骤任务必须拆分执行，失败时先展示错误和修复建议，再生成下一条命令。

## 当前平台二进制下载表

| 当前 OS/CPU | 外置二进制地址 | 本地保存路径 | 可用性检查命令 |
| ----------- | -------------- | ------------ | -------------- |
| Windows amd64 | {{BIN_URL_WINDOWS_AMD64}} | `bin/windows-amd64/invoice-reimbursement-assistant.exe` | `.\bin\windows-amd64\invoice-reimbursement-assistant.exe --help` |
| Linux amd64 | {{BIN_URL_LINUX_AMD64}} | `bin/linux-amd64/invoice-reimbursement-assistant` | `./bin/linux-amd64/invoice-reimbursement-assistant --help` |
| Linux arm64 | {{BIN_URL_LINUX_ARM64}} | `bin/linux-arm64/invoice-reimbursement-assistant` | `./bin/linux-arm64/invoice-reimbursement-assistant --help` |
| macOS amd64 | {{BIN_URL_DARWIN_AMD64}} | `bin/darwin-amd64/invoice-reimbursement-assistant` | `./bin/darwin-amd64/invoice-reimbursement-assistant --help` |
| macOS arm64 | {{BIN_URL_DARWIN_ARM64}} | `bin/darwin-arm64/invoice-reimbursement-assistant` | `./bin/darwin-arm64/invoice-reimbursement-assistant --help` |

## 运行时配置与路径约束

1. 发布包目录结构必须保持完整，不能只把某个平台的二进制文件单独拷贝出去运行。
2. 安装时必须先识别当前 OS/CPU，只下载当前平台二进制，并保存到上表对应的本地路径。若当前平台不在上表中，必须停止安装并说明支持的平台。
3. Linux 或 macOS 环境下载后必须确保本地二进制具备执行权限；如权限不足，执行 `chmod +x <本地二进制路径>` 后再检查。
4. 包内运行配置文件用于发票查验接口端点和能力配置，安装或更新时不得写入 API Key。
5. API Key 仍只能通过业务命令参数传入。

## 适用场景

- 用户说“安装 invoice-reimbursement-assistant skill”
- 用户说“更新 invoice-reimbursement-assistant skill”
- 用户说“重装 / 修复安装 / 安装最新版 / 未明确说明动作”
- 命令返回包含“当前 skill 版本过低，无法执行”的报错，需要 Agent 自动更新并重装

## 执行原则

1. 所有安装和更新动作都按本文档执行。
2. 安装目标必须是 agent 实际会加载的 skill 安装根目录，而不是当前工作目录或临时目录。
3. 先判断用户意图：仅当用户明确要求更新/升级（update/upgrade）时，才比较当前已安装版本与发布版本。
4. 安装、重新安装、修复安装、安装最新版或未明确说明动作时，不比较版本号；先卸载当前已安装的 invoice-reimbursement-assistant skill 副本，再下载并安装当前发布版本。
5. 若当前环境支持多个 skill 目录，必须以“当前 agent 实际会加载的那个目录”为准；不得把发布包解压到无关目录中冒充安装完成。

## 安装流程

1. 识别当前 agent 的 skill 安装根目录，并先判断用户意图。
2. 若用户明确要求更新/升级（update/upgrade），先检查是否存在已安装的 `invoice-reimbursement-assistant` 副本；只有在副本存在时，才读取其 `invoice-reimbursement-assistant/SKILL.md` 中的 version 并与本文档声明版本比较。
3. 若用户明确要求更新但未发现已安装副本，则无法比较本地版本，按全新安装流程继续；若用户请求安装、重新安装、修复安装、安装最新版或未明确说明动作，则不比较版本号，直接进入全新安装。
4. 在该 skill 安装根目录下定位 `invoice-reimbursement-assistant` 子目录；若不存在则创建。
5. 获取 invoice-reimbursement-assistant 的最新 ZIP 包，并将 ZIP 包解压到 agent 的 `invoice-reimbursement-assistant` skill 子目录，而不是当前工作目录或临时目录。
6. 根据“当前平台二进制下载表”识别当前 OS/CPU，只下载当前平台外置二进制，并保存为表中对应的本地路径；不得下载其它平台二进制。
7. 安装完成后，确认以下路径存在于已安装的 skill 子目录中：
    - invoice-reimbursement-assistant/SKILL.md
    - invoice-reimbursement-assistant/README.md
    - invoice-reimbursement-assistant/invoice-reimbursement-assistant-install.md
    - invoice-reimbursement-assistant/config/verify-api.json
    - 当前平台对应的 invoice-reimbursement-assistant/bin/<platform>/<binary>
8. 执行当前平台对应的可用性检查命令；Linux 或 macOS 如遇执行权限不足，先 `chmod +x` 后重试。
9. 仅当用户明确要求更新/升级（update/upgrade）且已安装副本存在时，才读取已安装 skill 子目录中的 `SKILL.md` 并确认 `version` 与本文档声明版本一致；其他动作只确认全新安装已完成。
10. 验证 skill 已生效：确认 agent 后续实际读取的是这个已安装 skill 目录；若当前环境支持刷新、重载或重新索引 skill，执行对应动作。
11. 若安装前创建了旧 skill 备份，安装成功并验证生效后必须自动删除该备份。
12. 安装完成并验证生效后，必须立即阅读已安装 skill 的本地 `README.md`，将 `README.md` 全文完整原样输出给用户；不得修改、概括、精简、重排、删减或只输出其中要求；若用户后续还要执行业务，再回到已安装 skill 的本地 `SKILL.md` 继续执行。

## 用户请求更新时的版本处理

1. 只有当用户明确要求更新/升级（update/upgrade）时，才先检查 agent 是否存在已安装的 `invoice-reimbursement-assistant` 副本；如果用户明确要求更新但未发现已安装副本，则无法比较本地版本，按全新安装流程继续。
2. 仅在已安装副本存在时，才读取 `invoice-reimbursement-assistant/SKILL.md` 中的 version，并与本文档声明的 version 比较。
3. 若版本一致且用户仅要求“更新/升级”，告知用户“当前已是最新版本”，然后继续本地 SKILL.md 的业务流程；若用户说“更新并重装skill”或明确要求重装，仍须执行下面的“更新流程”完成覆盖安装。
4. 若版本落后，执行下面的“更新流程”。

## 更新流程

1. 备份 agent 已安装的 `invoice-reimbursement-assistant` skill 子目录，备份目录放在同级临时位置，仅用于本次更新回滚。
2. 获取最新 ZIP 包，并将新版本 ZIP 包解压覆盖到 agent 已安装的 `invoice-reimbursement-assistant` skill 子目录。
3. 根据“当前平台二进制下载表”重新下载当前平台外置二进制，并覆盖保存为对应本地路径；不得下载其它平台二进制。
4. 覆盖后再次确认以下路径存在于已安装 skill 子目录中：
    - invoice-reimbursement-assistant/SKILL.md
    - invoice-reimbursement-assistant/README.md
    - invoice-reimbursement-assistant/invoice-reimbursement-assistant-install.md
    - invoice-reimbursement-assistant/config/verify-api.json
    - 当前平台对应的 invoice-reimbursement-assistant/bin/<platform>/<binary>
5. 重新读取已安装 skill 子目录中的 `SKILL.md`，确认 `version` 已更新。
6. 执行当前平台对应的可用性检查命令；Linux 或 macOS 如遇执行权限不足，先 `chmod +x` 后重试。
7. 验证 skill 已生效：确认 agent 当前读取的是更新后的安装目录；若当前环境支持刷新、重载或重新索引 skill，执行对应动作。
8. 仅当上述检查和生效验证全部成功后，删除本次更新创建的备份目录。
9. 更新完成并验证生效后，必须立即阅读已安装 skill 的本地 `README.md`，将 `README.md` 全文完整原样输出给用户；不得修改、概括、精简、重排、删减或只输出其中要求；若用户后续还要执行业务，再回到本地 `SKILL.md` 继续执行。

## 失败处理

- 如果 ZIP 包地址、当前平台二进制地址尚未配置、无法访问或下载失败，向用户明确说明无法完成安装或更新。
- 如果当前 OS/CPU 不在支持列表内，停止安装并报告支持的平台。
- 如果覆盖、版本校验、当前平台二进制可用性检查或生效验证失败，恢复备份目录，再告知用户失败原因；失败路径不得删除备份目录。
- 如果用户明确要求更新但未发现已安装副本，不视为失败；按全新安装流程继续，不做本地版本比较。

## 完成后动作

1. 若用户的请求只是安装、重新安装、修复安装、安装最新版或更新/升级，到这里结束前，必须已经完成本地 `README.md` 读取和全文完整原样输出，并在汇报结果中体现。
2. 若用户后续还要执行业务，安装或更新完成且已完成 `README.md` 全文完整原样输出后，回到已安装 skill 的本地 SKILL.md 继续执行。
3. 若本次请求属于安装、重新安装、修复安装、安装最新版或未明确说明动作，完成后按全新安装结果汇报，不附带版本比较结论。
