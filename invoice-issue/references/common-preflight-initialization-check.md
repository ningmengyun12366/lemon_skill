# 场景：preflight-initialization-check（任务执行前初始化检测）

## 模板用途

用于在开票前检查 API Key、本地工作区和本次绑定企业。预检通过只表示可以继续草稿检查，不证明税局已登录。


## 场景说明

- 业务目标：在任务开始前确认 API Key、工作区和企业上下文可用；`invoice_issue` 不为预览和检查提前触发登录。
- 当前入口：统一执行预检脚本。
- 失败处理：若预检失败，必须直接读取命令生成的 Markdown 报告正文，并原样渲染回对话中；不得因脱敏需求，对md内容进行二次修改、总结；不得根据接口响应、或其他中间结果自行补充解释或猜测处理办法。如果问题涉及工作区目录不可用或需要重新指定目录，应通过 `workspace-config-writer` 更新配置后再次执行预检。
- API Key 权限处理：若预检报告、接口响应或命令输出中出现“无权调用该能力”，必须视为当前 API Key 未绑定本 skill 所需能力权限或企业，不得引导用户登录或重新登录。Agent 必须在原样渲染预检 Markdown 报告后，按同级 `README.md` 中的 API Key 获取方式告知用户为当前 API Key 绑定对应能力权限及企业后再重新预检。
- 每个新任务必须显式提供本次用户选择的销售方 `uscc`，不能默认沿用上次企业。

## 执行步骤

1. 根据用户输入判断是否已提供企业名称或税号，如果有执行：
   
```powershell
.\bin\windows-amd64\invoice-assistant_windows_amd64.exe task-preflight-check --enterprise-name "企业名称或税号"
```
否则执行

```powershell
.\bin\windows-amd64\invoice-assistant_windows_amd64.exe task-preflight-check
```

1. 命令执行结束后，按照以下规则，处理预检结果：

- `preflight_passed=true`：预检通过，可以继续后续业务步骤。对 `invoice_issue`，该结果只证明企业和本地配置就绪，不代表税局已登录。
- 开票预检返回 `login_status.status=deferred` 是正常状态；不要额外索取账号密码或在生成预览前强行执行登录。
- `preflight_passed=false`：预检未通过，立即中止后续流程，不输入与后续流程相关的内容；必须直接读取并原样渲染 Markdown 报告正文给用户，**禁止因任何原因进行二次改动或补充描述**。
- `stopped_at=apikey_confirmation` 或问题码为 `apikey_confirmation_required`：说明本地已存在 API Key。必须先询问用户是沿用当前 API Key 还是使用新的 API Key；用户确认沿用后，重新执行 `task-preflight-check --use-config-apikey`，若同时指定企业则保留 `--enterprise-name`；用户选择新 API Key 时，先执行 `workspace-config-writer --apikey "<新 API Key>"` 后再重新预检。
- 问题码为 `enterprise_binding_selection_required`：说明当前 API Key 绑定多个企业且本次未明确企业。必须展示候选企业并等待用户选择；不得使用本地配置中的上次 `uscc` 直接继续。
- 若 Markdown 报告正文出现“无权调用该能力”，原样渲染报告后还必须按同级 `README.md` 中的 API Key 获取方式告知用户配置对应能力权限；不得改为提示用户去税局登录、重新登录或继续快速登录。
- 只有重新执行预检并确认通过后，才能继续后续业务场景。

## 配置更新方式

- 预检脚本读取的配置文件是 `.invoice-config/workspace-config.json`；若文件不存在，会在首次读取时自动创建默认配置。
- 如果 Agent 需要补写或更新配置，必须调用工作区配置写入脚本，不能直接手改 `workspace-config.json`。
- 配置更新命令入口：`.\bin\windows-amd64\invoice-assistant_windows_amd64.exe workspace-config-writer`

### 常用写法

1. 更新工作区目录：

```powershell
.\bin\windows-amd64\invoice-assistant_windows_amd64.exe workspace-config-writer --workspace-dir "D:\\your-workspace"
```

2. 更新 apikey：

```powershell
.\bin\windows-amd64\invoice-assistant_windows_amd64.exe workspace-config-writer --apikey "your_apikey"
```
