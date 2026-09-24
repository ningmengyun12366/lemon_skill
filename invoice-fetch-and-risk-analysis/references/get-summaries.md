# 发票文件列表与附件结果

本文件说明取票命令返回的文件信息。调用方式见 [命令与返回结果](api-contracts.md)，整体执行要求见 [SKILL.md](../SKILL.md)。Agent 不单独发送汇总查询请求。

## 获取和读取

- 使用 `fetch-invoices --task-file <task.json>` 获取发票数据；成功后从本次任务的 `artifacts.summaries_data_list` 读取文件列表。
- 使用 `summarize --task-file <task.json>` 生成摘要，并按命令返回的 Markdown 路径完整展示摘要。
- 用户确认需要发票原件或附件时，执行 `fetch-attachments --task-file <task.json>`，使用返回的本地文件或目录交付。未确认需要附件时不自行下载。

文件列表各项含义：

| 字段 | 含义及使用方式 |
| --- | --- |
| `dateRange` | 文件覆盖期间，不能把未包含的月份算作已取得。 |
| `type` | `取得发票`按进项理解，`开具发票`按销项理解；未知值不猜测。 |
| `excelFileUrl` | 命令下载文件时使用的远程地址；不在用户输出中展示签名 URL，交付采用本地路径。 |

## 状态和交付

- 发票任务完成不代表用户要求的附件、清单、统计和报告全部完成；继续按 [workflow.md](workflow.md) 执行。
- 空列表不自动代表期间没有发票，先结合命令成功状态和实际覆盖范围判断。
- 附件部分失败时保留成功文件，说明失败数量、影响范围和可重试步骤。
- 汇总获取或下载失败时保留当前任务，按 [error-handling.md](error-handling.md) 处理，不重新创建发票同步任务。
- 不把 `code=0` 或拿到下载地址当作本地文件已经下载成功，以命令结果和实际文件为准。
