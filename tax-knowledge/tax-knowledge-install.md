# tax-knowledge 安装与更新说明

当用户要求安装、更新或重装 `tax-knowledge` skill 时，agent 必须先完整阅读本文件，并按本文档执行。ZIP 包只包含 skill 说明文件、references 和资源文件，不包含 `bin/**` 二进制文件；当前平台二进制必须从本文档列出的外置地址下载。安装或更新完成并验证生效后，必须立即阅读包内 `README.md`，并将 `README.md` 全文完整原样输出给用户；不得修改、概括、精简、重排、删减或只输出其中要求；不得等到后续业务执行时才输出。

## 当前发布信息

- version: 0.1.4
- skill 目录名：`tax-knowledge`
- 发布包文件名：`tax-knowledge-0.1.4.zip`
- 安装文档地址：{{INSTALL_DOC_URL}}
- 发布包地址：{{ZIP_URL}}
- 运行时能力码：`tax_knowledge`
- 公共命令名：`tax-knowledge`

## 版本检测与自动更新

1. 本 skill 会自动检测版本，版本不符合要求时会阻断执行。
2. 若命令返回包含“当前 skill 版本过低，无法执行”的报错，Agent 立即从报错提供的安装文档链接获取最新教程，备份当前 skill 后按教程更新与重装，无需等待用户再次确认。
3. 用户说“更新并重装skill”时，Agent 按同一流程立即执行。
4. 验证成功后删除备份并重试原业务；验证或安装失败时恢复备份并说明原因。

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
| Windows amd64 | {{BIN_URL_WINDOWS_AMD64}} | `bin/windows-amd64/tax-knowledge.exe` | `.\bin\windows-amd64\tax-knowledge.exe --help` |
| Linux amd64 | {{BIN_URL_LINUX_AMD64}} | `bin/linux-amd64/tax-knowledge` | `./bin/linux-amd64/tax-knowledge --help` |
| Linux arm64 | {{BIN_URL_LINUX_ARM64}} | `bin/linux-arm64/tax-knowledge` | `./bin/linux-arm64/tax-knowledge --help` |
| macOS amd64 | {{BIN_URL_DARWIN_AMD64}} | `bin/darwin-amd64/tax-knowledge` | `./bin/darwin-amd64/tax-knowledge --help` |
| macOS arm64 | {{BIN_URL_DARWIN_ARM64}} | `bin/darwin-arm64/tax-knowledge` | `./bin/darwin-arm64/tax-knowledge --help` |

## 发布包目录结构

发布包解压后根目录必须是 `tax-knowledge/`。该目录是完整的 skill 发布包；二进制不在 ZIP 中，必须在安装时根据当前 OS/CPU 只下载当前平台二进制，并保存到上表对应的本地路径。

必须存在以下路径：

- `tax-knowledge/SKILL.md`
- `tax-knowledge/README.md`
- `tax-knowledge/tax-knowledge-install.md`
- `tax-knowledge/references/retrieval.md`
- 当前平台对应的 `tax-knowledge/bin/<platform>/<binary>`

## 安装原则

1. 安装目标必须是 agent 实际会加载的 skill 安装根目录，而不是当前业务仓库目录。
2. 安装时必须先识别当前 OS/CPU，只下载当前平台对应的一个外置二进制文件；不得下载其它平台二进制。
3. 下载的外置二进制文件名带版本号；保存到本地 skill 目录时必须改为 `SKILL.md` 中声明的原始运行文件名。
4. 安装、重装、修复安装或安装最新版时，不比较版本号，直接用当前 ZIP 包和当前平台二进制替换已安装的 `tax-knowledge` 目录。
5. 仅当用户明确要求更新/升级时，才读取已安装 `tax-knowledge/SKILL.md` 的 version 并与本文档版本比较。
6. 不得把 API Key 写入发布包、配置样例、安装文档或 evidence。API Key 应由用户在运行时提供，或由本地安全配置提供。
7. 若当前环境支持多个 skill 目录，必须以当前 agent 实际加载的目录为准；不得把发布包解压到当前业务仓库中冒充安装完成。

## 安装流程

1. 识别当前 agent 的 skill 安装根目录。
2. 下载或获取 `tax-knowledge-0.1.4.zip`。
3. 若已存在 `<skill 安装根>/tax-knowledge`，先备份或移除旧目录。
4. 将 ZIP 包解压到 skill 安装根目录，确保最终路径为 `<skill 安装根>/tax-knowledge/SKILL.md`，而不是 `<skill 安装根>/tax-knowledge/tax-knowledge/SKILL.md`。
5. 根据“当前平台二进制下载表”识别当前 OS/CPU，只下载当前平台外置二进制，并保存为表中对应的本地路径；不得下载其它平台二进制。
6. 确认“发布包目录结构”中列出的必须路径全部存在。
7. 在当前操作系统下执行二进制可用性检查；Linux 或 macOS 环境如遇到执行权限不足，先对对应二进制执行 `chmod +x`，再重新检查。
8. 若环境支持刷新、重载或重新索引 skill，执行对应动作，确保 agent 后续读取的是已安装目录。
9. 安装完成并验证生效后，必须立即阅读已安装 skill 的本地 `README.md`，将 `README.md` 全文完整原样输出给用户；不得修改、概括、精简、重排、删减或只输出其中要求；若用户后续还要执行业务，再回到已安装 skill 的本地 `SKILL.md` 继续执行。

## 更新流程

1. 只有用户明确要求更新/升级时，才读取已安装 `tax-knowledge/SKILL.md` 的 version。
2. 若版本与本文档一致，告知用户当前已是最新版本；如用户仍要求重装，则按安装流程覆盖安装。
3. 若版本落后或无法读取版本，备份已安装的 `tax-knowledge` 目录。
4. 下载或获取当前 ZIP 包，并覆盖安装到 `<skill 安装根>/tax-knowledge`。
5. 根据“当前平台二进制下载表”重新下载当前平台外置二进制，并覆盖保存为对应本地路径；不得下载其它平台二进制。
6. 覆盖后重新确认目录结构、`SKILL.md` version 和当前平台二进制可用性。
7. 仅当上述检查全部成功后，删除本次更新创建的备份目录。
8. 更新完成并验证生效后，必须立即阅读已安装 skill 的本地 `README.md`，将 `README.md` 全文完整原样输出给用户；不得修改、概括、精简、重排、删减或只输出其中要求；若用户后续还要执行业务，再回到本地 `SKILL.md` 继续执行。

## 业务命令契约

运行时命令：

```powershell
tax-knowledge retrieve --query "<query>" --api-key "<api key>" --pretty
```

每个用户问题先把相关关键词组合为一个 query 并执行一次主检索；用户提出多个问题时，每个问题分别执行一次主检索。只有某个问题的主检索 `record_count=0` 或全部返回内容明显无关时，才允许按 `SKILL.md` 与 `references/retrieval.md` 将该问题拆成更小的关键词查询进行补查。不得把多个独立问题合并为一个 query，也不得在已有相关内容时仅因缺少唯一法规而判定无结果。

也支持 UTF-8 JSON 输入文件：

```powershell
tax-knowledge retrieve --input "<input.json>" --pretty
```

`<input.json>` 至少可包含：

```json
{
  "query": "<query>",
  "api_key": "<api key>"
}
```

若同时传入 `--query` 或 `--api-key`，命令行参数会覆盖 JSON 文件中的对应值。

固定接口契约：

- 接口：`POST /retrieval`，基于 OpenAPI skill base URL。
- 请求头：`Authorization: apikey=<api key>; capabilityCode=tax_knowledge`
- 请求体必须且只能包含 `query`：

```json
{
  "query": "<单个问题的关键词组合查询或补充查询>"
}
```

API Key 只通过 `Authorization` 请求头发送；CLI 输入 JSON 中的 `api_key` 不会进入接口请求体。

成功 stdout JSON 至少包含 `ok`、`status`、`message`、`query`、`capability_code`、`record_count`、`records`、`answer_context`、`allow_external_search`，上游返回追踪标识时还包含 `traceId`。每条 `records[]` 包含 `index`、`title`、`content`、`score`，以及由上游 `metadata` 展开的 `doc_type`、`document_id`、`document_no`、`invalid_type`、`invalid_type_label`、`invalid_prompt`、`source_publish_date`、`source_url`、`attachment_refs`。失败 stdout JSON 包含 `ok=false`、`status`、`message`、`record_count`、`records`、`answer_context`、`allow_external_search`，并可能包含 `error` 对象（`http_status`、`code`、`message`、`traceId`）。

## 失败处理

- 如果 ZIP 包地址、当前平台二进制地址未配置、无法访问或下载失败，向用户说明无法完成安装或更新。
- 如果当前 OS/CPU 不在支持列表内，停止安装并报告支持的平台。
- 如果解压后目录层级错误，移动或重新解压，使最终根目录保持为 `<skill 安装根>/tax-knowledge/`。
- 如果目录结构、版本校验或当前平台二进制检查失败，恢复备份目录，并告知用户失败原因；失败时不要删除备份目录。

## 完成后动作

1. 若用户的请求只是安装、重装、修复安装或更新，到这里结束前，必须已经完成本地 `README.md` 读取和全文完整原样输出，并在汇报结果中体现。
2. 若用户后续还要执行业务，安装或更新完成且已完成 `README.md` 全文完整原样输出后，回到已安装 `tax-knowledge/SKILL.md` 继续执行。
3. 对于全新安装、重装、修复安装或安装最新版，不附带版本比较结论。
