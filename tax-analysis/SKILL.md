---
name: "tax-analysis"
display_name: "柠檬云企业税务风险分析"
display_name_en: "tax-analysis"
description: "根据企业名称和分析期间获取财税及工商信息，生成内容一致、目录可跳转的 Word 和 PDF 企业税务风险分析报告。"
description_zh: "同步企业税务及工商数据，生成内容一致、目录可跳转的 DOCX 和 PDF 企业税务风险分析报告。"
description_en: "Synchronizes enterprise tax and business registry data and generates matching navigable DOCX and PDF tax risk reports."
version: "0.1.1"
---

# 税务分析

根据企业名称和分析期间获取财税及工商信息，生成内容一致、目录可跳转的 Word 和 PDF 企业税务风险分析报告。数据含义及使用范围见[数据说明](references/structured-data.md)。

## 安装、版本检测与自动更新

首次加载本Skill时完整原样展示[README.md](README.md)，不概括或删减。安装、重装按`{{INSTALL_DOC_URL}}`执行。主程序下载后先验证SHA-256，再允许执行；assets 全部从安装文档的服务器地址下载；按 references/asset-download-manifest.json 选择共用资源及当前平台 Node，逐项验证 SHA-256 后使用，复用已校验的本地文件。

本 skill 会自动检测版本，版本不符合要求时会阻断执行。若命令返回包含“当前 skill 版本过低，无法执行”的报错，从报错提供的安装文档链接获取最新教程，按教程备份、更新并重装skill；验证成功后删除备份并重试原业务。失败恢复备份并说明实际原因。该规则优先于任何普通错误重试、降级或继续执行规则；不重复请求已获得的更新授权。

### PowerShell UTF-8 初始化（Windows 强制）

Windows每个新PowerShell进程在调用主程序前先初始化，命令必须在同一个 PowerShell 进程执行：

```powershell
$Utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[Console]::InputEncoding = $Utf8NoBom
[Console]::OutputEncoding = $Utf8NoBom
$OutputEncoding = $Utf8NoBom
```

输入JSON/文本使用 UTF-8 无 BOM 文件，写入方式为`[IO.File]::WriteAllText($path, $content, $Utf8NoBom)`。

## 企业、期间与登录

按[企业和期间规则](references/periods.md)使用用户已明确的企业与起止月份。企业须与绑定列表精确匹配；唯一匹配直接继续，不再要求用户确认一次。未指定企业时才执行：

```text
tax-analysis enterprises --api-key <key> --pretty
```

完整展示返回的企业名称和USCC供用户选择；同名须使用USCC，不做模糊选择或自动绑定。已选择的企业可沿用，后续命令仍核验归属。

`run/create/report-prepare`自动执行登录状态、快速登录预检和登录后复查。不能继续时展示返回的完整手动登录网页入口，等用户完成后执行原命令。登录请求失败不等于登录过期，详见[异常处理](references/error-handling.md)。`query`不重新登录；本地校验与渲染不创建任务。

## 取数与任务状态

接口字段及恢复机制见[API契约](references/api.md)。普通取数使用：

```text
tax-analysis run --company <完整企业名称> --start-month YYYY-MM --end-month YYYY-MM --api-key <key> --pretty
tax-analysis query --uscc <已确认USCC> --task-id <taskId> --api-key <key> --pretty
```

公司与USCC参数二选一。首尾月份包含、最早2017-01、最多36个月；脚本统一把末月规范化为月末日期。不得因3997或其他创建失败擅自缩短期间。

运行中继续查询同一taskId。可恢复的interrupted继续等待；不可恢复时保留原因和部分数据。所有异常按[状态处理表](references/error-handling.md)执行。只有用户明确要求全部重新查询才加`--force-refresh`。

同一环境和API Key的业务请求间隔至少5秒，默认查询轮询间隔为5秒。使用命令提供的等待机制，不要另起进程或密集执行命令绕过限频；离线分析不用额外等待。

普通取数只说明状态、范围和返回事实。最终风险结论只通过下面的报告流程生成。

## 生成Word和PDF

报告规则`{{REPORT_RULE_VERSION}}`，AI契约`{{AI_CONTRACT_VERSION}}`。运行资料分工：

- [报告工作流](references/report.md)：命令和工作区文件。
- [数据契约](references/report-data-contract.md)、[字段口径](references/fields.md)：取数及缺失含义。
- [规则说明](references/report-rules.md)：已确认的计算、风险条件和适用范围。
- [输出规则](references/report-output-v4.md)、[报告范围](references/report-scope.md)：客户可见内容、条数和禁用内容。
- [逐节分析写作](references/report-analysis.md)：按指标解释业务含义、分别填写正文和引用。
- [Word章节映射](references/word-report-contract.md)：核对标题、表头、图表和显示条件时查阅。

先读取工作流、输出规则及[AI 读写字段与指标口径](references/report-data-contract.md#ai-读写字段)，明确单位、状态和引用含义后开始正文。其余资料按当前章节和问题查阅；直接核对原始响应时使用字段口径及结构化数据说明，不反复加载完整原始数据。

```text
tax-analysis report-prepare --company <完整企业名称> --start-month YYYY-MM --end-month YYYY-MM --api-key <key> --workspace <dir> --pretty
```

准备完成后先读`analysis-task.md`中的通用要求，再执行以下本地命令。用户明确要报告时直接prepare，无需另行create。按批阅读完整章节及比较事项；业务品名的全部明细可按需查询。

```text
tax-analysis report-read --workspace <dir> --next --pretty
```

先读返回的成组关键事实和主体核验范围，连同期间/基期、缺失状态及准确引用ID按目标写分析。关键事实与详细指标共用同一来源，不需重算。正文选关键金额或差额说明发现，不用“较大金额”“见证据”代替已给出的数值。不编写临时抽取、拆文件或Python填充程序；无需全文加载analysis-data.json或analysis-sections。需补查时使用`report-refs --workspace <dir> --section <semanticId> --name <中文指标名>`，可加`--subject/--period/--base-period/--id`；多个匹配自行按真实主体和口径选择，不猜ID或默认取第一条。结果有nextOffset时可继续查本地下一页。

每批保存UTF-8正文片段JSON：复制report-read的analysisDataDigest，sections只包含本批章节，每节items填写text/evidenceRefs；不用复制整个analysis-output.json。通过脚本写入：

```text
tax-analysis report-write --workspace <dir> --input <本批正文.json> --pretty
```

成功后按progress继续report-read --next。失败只修返回的具体字段，原文件保持不变；同节重交为整体替换。所有章节完成后，next按“全部已确认风险及动作、数据限制、关键事实”的顺序返回综合结论材料，不重复附上各节正文。对照一次后用同一命令提交conclusion.items；完整事实仍可用report-refs查询。断点续写直接读next，不重新取数。完整JSON例子与查询匹配方式见[报告工作流](references/report.md)。

guidance提供本节选题方向，不新增必填项。不可用指标不能作为有效分析依据。
`text`填写简洁易懂的自然语言分析，`evidenceRefs`独立记录实际使用的来源ID；正文不写占位符，不凑引用或复述所有数据。按提供的计算结果量化重点，不编造数字、原因、行业基准或整改目标。

具体数字口径、分析边界和中文标点统一遵循已读取的analysis-task.md通用指引。本节写作要求与企业事实分区展示；同口径比较连同对应的调节结果一起阅读，发票分类与财报收入使用各自明确的对象和分母。

```text
tax-analysis report-validate --workspace <dir> --pretty
```

按missingPaths、invalidReferences、contentErrors和issues修正结构或引用错误。valid=true后直接渲染，无需单独语义审读文件。校验只确认结构、来源ID与冻结摘要，不核实自然语言分析。正文或引用变化后重新校验。

```text
tax-analysis report-render --workspace <dir> --output-dir <dir> --pretty
```

只有最终`valid=true`才渲染，`report`是`report-render`的别名。完整返回保存为工作区render-result.json，后续查看它即可，不重跑渲染来获取被head/tail截掉的结果。相同输入和渲染版本且两文件SHA-256一致时返回reused=true；否则重新生成。Word/PDF必须同时成功且内容一致，目录可跳转，文件名冲突不覆盖旧报告。任一失败不交付单格式或半成品。

## 交付边界

报告正文不附原始数据、来源ID或计算底稿。引用其他章节时使用章节标题；填写正文JSON时使用返回的sectionId，不用报告显示编号替代。

封面无额外单位句；2.1/2.2/2.3/3.1不追加读表说明；企业变更全部记录只展示表格；税费月度汇总仅实缴数字。敏感服务统一使用进项购进，表头为“发票类别｜本次查询期间金额｜关注事项”，期间由实际起止月份生成，关注事项沿用对应服务的核查内容。其余具体要求见输出规则。

实际风险由脚本决定，只有确认风险才给对应动作。缺失不填零，成功空记录不等于全面正常；不展示API Key、Cookie、密码、sessionId、accountId、接口路径和内部日志。
