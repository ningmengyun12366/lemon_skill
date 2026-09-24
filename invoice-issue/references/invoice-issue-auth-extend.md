# 开票权限认证

适用于开票命令提示“请先扫码验证获取开票权限”，或用户主动要求延长开票权限。认证完成不等于开票成功，仍须按开票结果判断。

## 获取二维码

已有本次开票任务时执行：

```powershell
& ".\bin\windows-amd64\invoice-assistant_windows_amd64.exe" issue-auth-workflow --action start --pretty
```

独立认证且尚无可复用任务时，准备只包含本次已选择 `uscc` 的 UTF-8 JSON，用 `--action start --input <input.json> --pretty` 初始化。不要要求用户填写登录账号或地区代码；命令提示企业绑定或配置不完整时按报告处理。

- 使用本次已选择的企业，不使用上次任务的企业信息；可用 `--task-file <task.json>` 明确指定当前任务。
- 命令返回登录问题时，原样展示原因和安全登录地址，一律称为“手动登录网页入口”；不将其解释为二维码或权限认证链接。
- 只有结果同时提供非空 `issue_auth_result.qrcode_markdown_path`、`issue_auth_result.qrcode_list` 和 `issue_auth_result.qrcode_image_paths`，才读取并展示二维码 Markdown。
- Agent 必须直接解析 stdout JSON，按明确路径读取；不得因没有捕获到输出而扫描目录。
- 每个平台下保留“二维码图片路径：<绝对 PNG 路径>”，明确对应“国家网络身份认证APP”或“电子税局APP”，任选一个扫码即可，并提示 5 分钟失效。
- 不得输出 `MEDIA:`、二维码原文、Base64、`rzid` 或认证编号。不把二维码图片交给第三方服务。

## 校验认证

用户明确表示已完成扫码后，使用同一任务执行：

```powershell
& ".\bin\windows-amd64\invoice-assistant_windows_amd64.exe" issue-auth-workflow --action verify --pretty
```

读取 `issue_auth_result.verify_markdown_path` 并原样展示。按当前返回状态处理：

| 返回状态 | 下一步 |
| --- | --- |
| 二维码状态 `1` | 待扫码，等待用户完成，不自动轮询。 |
| 二维码状态 `3` | 已失效，提示重新获取。 |
| 二维码状态 `2` | 已扫码，继续根据命令返回的认证结果判断；不能直接认定认证成功。 |
| 认证 `status=1` 且 `isAuth=true` | 认证通过，可继续原开票任务。 |
| 认证 `status=0/2` | 仍在处理，稍后再次校验。 |
| 认证 `status=-1` | 认证失败，按实际原因处理。 |
| `login_required` 或 `code=2006` | 原样展示原因和手动登录网页入口，用户完成登录后用同一任务再次 `verify`，不要求重新填写开票内容。 |

## 返回开票流程

当前预览已由用户明确确认时，执行 `issue-from-output --confirm-preview --pretty`。旧任务明确因 `code=2015`、`data.status=-1` 失败、`issue_completed=false` 且无已开票产物时，按命令提示恢复一次；不追加 `--confirm-duplicate`，不手工删除或改写任务号。结果未知或再次被阻断时停止，不反复提交。

正式开票操作见 [workflow.md](workflow.md)，错误处理见 [error-handling.md](error-handling.md)。
