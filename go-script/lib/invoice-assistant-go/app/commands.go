package app

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/fetchrisk"
	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/issue"
	"invoice-assistant-skill/go-script/lib/skillupdate"
)

func Run(args []string) (resultErr error) {
	defer func() {
		resultErr = skillupdate.UserFacingError(resultErr)
	}()
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		fmt.Fprintln(os.Stdout, "invoice-assistant commands:")
		/** 取票命令 */
		fmt.Fprintln(os.Stdout, "  init-task --input <input.json>")
		fmt.Fprintln(os.Stdout, "  task-preflight-check [--enterprise-name <name>] [--use-config-apikey]")
		fmt.Fprintln(os.Stdout, "  workspace-config-writer [--workspace-dir <dir>] [--apikey <key>] [--set k=v] [--extra-json <json>]")
		fmt.Fprintln(os.Stdout, "  query-enterprise-profile-by-uscc --uscc <uscc>")
		fmt.Fprintln(os.Stdout, "  download-files --input <input.json>")
		fmt.Fprintln(os.Stdout, "  fetch-invoices --task-file <task.json> [--confirm-duplicate]")
		fmt.Fprintln(os.Stdout, "  fetch-attachments --task-file <task.json> [--confirm-duplicate]")
		fmt.Fprintln(os.Stdout, "  summarize --task-file <task.json>")
		fmt.Fprintln(os.Stdout, "  fetch-deduction --task-file <task.json> [--confirm-duplicate]")
		fmt.Fprintln(os.Stdout, "  generate-invoice-list --task-file <task.json>")
		fmt.Fprintln(os.Stdout, "  analyze-invoice-data --task-file <task.json>")
		fmt.Fprintln(os.Stdout, "  build-invoice-context-data --task-file <task.json>")
		fmt.Fprintln(os.Stdout, "  markdown-to-pdf --input <report.md> --output <report.pdf> [--task-file <task.json>]")
		fmt.Fprintln(os.Stdout, "  finalize-risk-artifacts --task-file <task.json> --report-md <report.md> --report-pdf <report.pdf>")

		/** 开票命令 */
		fmt.Fprintln(os.Stdout, "  issue-search-enterprise --query <name-or-uscc>")
		// 单张开票参数校验与自动填充
		fmt.Fprintln(os.Stdout, "  issue-single-invoice-info-check [--input <input.json>] [--task-file <task.json>] [--no-generate-fplsh] [--pretty]  (single invoice check; task_type=single)")
		// 批量开票参数校验与自动填充
		fmt.Fprintln(os.Stdout, "  issue-batch-invoice-info-check [--input <input.json>] [--task-file <task.json>] [--no-generate-fplsh] [--pretty]  (batch invoice check; one invoice is allowed but returns a batch-shaped result)")
		// 发票预览生成
		fmt.Fprintln(os.Stdout, "  issue-invoice-preview-generator --input <input.json> --output <preview.html>  (HTML only; does not update task JSON or Markdown)")
		// 发票开具权限延长
		fmt.Fprintln(os.Stdout, "  issue-auth-workflow --action start|verify [--task-file <task.json>] [--input <input.json>] [--pretty]")
		// 开票
		fmt.Fprintln(os.Stdout, "  issue-from-output [--task-file <task.json>] [--output <result.md>] [--confirm-preview] [--confirm-duplicate] [--pretty]")

		return nil
	}
	switch args[0] {
	case "init-task":
		fs := flag.NewFlagSet("init-task", flag.ContinueOnError)
		input := fs.String("input", "", "input JSON file")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *input == "" {
			return fmt.Errorf("missing required --input")
		}
		_, err := fetchrisk.InitTask(*input)
		return err
	case "fetch-invoices":
		taskFile, confirmDuplicate, err := parseFetchTaskFile(args[0], args[1:])
		if err != nil {
			return err
		}
		return runWithFailureLog(fetchrisk.FetchInvoicesWithOptions(taskFile, fetchrisk.FetchCommandOptions{ConfirmDuplicate: confirmDuplicate}), "执行取票失败", ": ", "，请稍后重试")
	case "fetch-attachments":
		taskFile, confirmDuplicate, err := parseFetchTaskFile(args[0], args[1:])
		if err != nil {
			return err
		}
		return runWithFailureLog(fetchrisk.FetchAttachmentsWithOptions(taskFile, fetchrisk.FetchCommandOptions{ConfirmDuplicate: confirmDuplicate}), "执行附件下载失败", ": ", "，请稍后重试")
	case "fetch-deduction":
		taskFile, confirmDuplicate, err := parseFetchTaskFile(args[0], args[1:])
		if err != nil {
			return err
		}
		_, err = fetchrisk.ExecuteFetchDeductionWithOptions(taskFile, fetchrisk.FetchCommandOptions{ConfirmDuplicate: confirmDuplicate})
		return err
	case "markdown-to-pdf":
		return runMarkdownToPDF(args[1:])
	case "finalize-risk-artifacts":
		fs := flag.NewFlagSet("finalize-risk-artifacts", flag.ContinueOnError)
		taskFile := fs.String("task-file", "", "task JSON file")
		reportMD := fs.String("report-md", "", "risk report markdown file")
		reportPDF := fs.String("report-pdf", "", "risk report PDF file")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return fetchrisk.FinalizeRiskArtifacts(*taskFile, *reportMD, *reportPDF)
	case "summarize":
		taskFile, err := parseTaskFile(args[0], args[1:])
		if err != nil {
			return err
		}
		common.AppendMessageLog("开始生成发票摘要")
		return runWithFailureLog(fetchrisk.Summarize(taskFile), "生成发票摘要失败", ": ", "")
	case "generate-invoice-list":
		taskFile, err := parseTaskFile(args[0], args[1:])
		if err != nil {
			return err
		}
		common.AppendMessageLog("开始生成发票清单")
		return runWithFailureLog(fetchrisk.GenerateInvoiceListFromTask(taskFile), "生成发票清单失败", ": ", "")
	case "analyze-invoice-data":
		taskFile, err := parseTaskFileOptional(args[0], args[1:])
		if err != nil {
			return err
		}
		common.AppendMessageLog("开始生成统计分析报告")
		return runWithFailureLog(fetchrisk.AnalyzeInvoiceDataCommand(taskFile), "发票数据分析未完成", "：", "")
	case "build-invoice-context-data":
		taskFile, err := parseTaskFileOptional(args[0], args[1:])
		if err != nil {
			return err
		}
		common.AppendMessageLog("开始生成风险分析数据源")
		return runWithFailureLog(fetchrisk.BuildInvoiceContextDataCommand(taskFile), "生成风险分析数据源失败", "：", "")
	case "workspace-config-writer":
		return runWorkspaceConfigWriter(args[1:])
	case "query-enterprise-profile-by-uscc":
		return runQueryEnterpriseProfile(args[1:])
	case "download-files":
		return runDownloadFiles(args[1:])
	case "task-preflight-check":
		return runTaskPreflightCheck(args[1:])
	case "issue-search-enterprise":
		return runIssueSearchEnterprise(args[1:])
	case "issue-single-invoice-info-check", "single-invoice-info-check":
		return runIssueSingleInvoiceInfoCheck(args[1:])
	case "issue-batch-invoice-info-check", "batch-invoice-info-check":
		return runIssueBatchInvoiceInfoCheck(args[1:])
	case "issue-invoice-preview-generator", "invoice-preview-generator":
		return runIssueInvoicePreviewGenerator(args[1:])
	case "issue-auth-workflow":
		return runIssueAuthWorkflow(args[1:])
	case "issue-from-output":
		return runIssueFromOutput(args[1:])
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func parseTaskFile(command string, args []string) (string, error) {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	taskFile := fs.String("task-file", "", "task JSON file")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if *taskFile == "" {
		return "", fmt.Errorf("missing required --task-file")
	}
	return *taskFile, nil
}

func parseFetchTaskFile(command string, args []string) (string, bool, error) {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	taskFile := fs.String("task-file", "", "task JSON file")
	confirmDuplicate := fs.Bool("confirm-duplicate", false, "user explicitly confirmed creating a duplicate remote fetch task")
	if err := fs.Parse(args); err != nil {
		return "", false, err
	}
	if *taskFile == "" {
		return "", false, fmt.Errorf("missing required --task-file")
	}
	return *taskFile, *confirmDuplicate, nil
}

func parseTaskFileOptional(command string, args []string) (string, error) {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	taskFile := fs.String("task-file", "", "task JSON file")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	return *taskFile, nil
}

func runWorkspaceConfigWriter(args []string) error {
	fs := flag.NewFlagSet("workspace-config-writer", flag.ContinueOnError)
	workspaceDir := fs.String("workspace-dir", "", "workspace dir")
	apikey := fs.String("apikey", "", "api key")
	extraJSON := fs.String("extra-json", "", "extra JSON")
	var pairs multiValue
	fs.Var(&pairs, "set", "key=value")
	if err := fs.Parse(args); err != nil {
		return err
	}
	result, err := common.UpdateWorkspaceConfig(*workspaceDir, *apikey, pairs, *extraJSON)
	if err != nil {
		return err
	}
	return printJSON(result)
}

/**
 * runQueryEnterpriseProfile 按显式 USCC 解析服务端企业快照后查询企业画像。
 *
 * @param args query-enterprise-profile-by-uscc 的命令行参数。
 * @returns 查询和 JSON 输出成功时返回 nil，否则返回参数或接口错误。
 */
func runQueryEnterpriseProfile(args []string) error {
	fs := flag.NewFlagSet("query-enterprise-profile-by-uscc", flag.ContinueOnError)
	uscc := fs.String("uscc", "", "uscc")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*uscc) == "" {
		return fmt.Errorf("missing required --uscc")
	}
	config := common.LoadWorkspaceConfig()
	baseClient := common.NewAPIClient()
	enterpriseContext, err := common.ResolveBoundEnterprise(baseClient, common.StringValue(config[common.APIKey]), *uscc)
	if err != nil {
		return err
	}
	requestContext, err := common.EnterpriseRequestContextFromSnapshot(enterpriseContext, common.CapabilityCode)
	if err != nil {
		return err
	}
	resolvedAreaCode, err := common.NormalizeStandardAreaCode(enterpriseContext["areaCode"])
	if err != nil {
		return err
	}
	client := common.NewEnterpriseAPIClient(baseClient, config, requestContext)
	result, err := fetchrisk.QueryEnterpriseProfile(client, requestContext.USCC, resolvedAreaCode, requestContext.PersonalAccount)
	if err != nil {
		return err
	}
	return printJSON(result)
}

func runDownloadFiles(args []string) error {
	fs := flag.NewFlagSet("download-files", flag.ContinueOnError)
	input := fs.String("input", "", "input JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *input == "" {
		return fmt.Errorf("missing required --input")
	}
	result, err := common.ExecuteDownloadFiles(*input)
	if err != nil {
		return err
	}
	printJSON(result)
	if result["status"] == "failed" {
		return fmt.Errorf("download failed")
	}
	return nil
}

func runTaskPreflightCheck(args []string) error {
	fs := flag.NewFlagSet("task-preflight-check", flag.ContinueOnError)
	enterpriseName := fs.String("enterprise-name", "", "enterprise name")
	useConfigAPIKey := fs.Bool("use-config-apikey", false, "confirm using configured API key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	result := common.TaskPreflightCheckWithOptions(*enterpriseName, common.PreflightOptions{
		RequireAPIKeyConfirmation: true,
		UseConfiguredAPIKey:       *useConfigAPIKey,
		DeferLoginChecks:          common.CapabilityCode == common.InvoiceIssueCapabilityCode,
	})
	if err := preflightVersionError(result); err != nil {
		return err
	}
	return printJSON(result)
}

// preflightVersionError promotes a structured preflight version failure to the CLI error boundary.
func preflightVersionError(result map[string]any) error {
	issues, _ := result["issues"].([]map[string]any)
	for _, issue := range issues {
		if common.StringValue(issue["code"]) == "skill_version_outdated" {
			return errors.New(common.StringValue(issue["detail"]))
		}
	}
	return nil
}

func runIssueSearchEnterprise(args []string) error {
	fs := flag.NewFlagSet("issue-search-enterprise", flag.ContinueOnError)
	query := fs.String("query", "", "enterprise name or uscc")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *query == "" && len(fs.Args()) > 0 {
		*query = fs.Args()[0]
	}
	result, err := issue.SearchEnterprise(*query)
	if err != nil {
		return err
	}
	return printJSON(result)
}

func runIssueSingleInvoiceInfoCheck(args []string) error {
	fs := flag.NewFlagSet("issue-single-invoice-info-check", flag.ContinueOnError)
	input := fs.String("input", "", "input JSON file")
	taskFile := fs.String("task-file", "", "task JSON file")
	output := fs.String("output", "", "legacy alias of --task-file")
	noGenerateFplsh := fs.Bool("no-generate-fplsh", false, "do not generate fplsh")
	pretty := fs.Bool("pretty", false, "pretty JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *taskFile == "" {
		*taskFile = *output
	}
	taskPath, err := issue.CheckSingle(*input, *taskFile, !*noGenerateFplsh, *pretty)
	if err != nil {
		return err
	}
	appendIssueCheckSummary(taskPath)
	return nil
}

func runIssueBatchInvoiceInfoCheck(args []string) error {
	fs := flag.NewFlagSet("issue-batch-invoice-info-check", flag.ContinueOnError)
	input := fs.String("input", "", "input JSON file")
	taskFile := fs.String("task-file", "", "task JSON file")
	output := fs.String("output", "", "legacy alias of --task-file")
	noGenerateFplsh := fs.Bool("no-generate-fplsh", false, "do not generate fplsh")
	pretty := fs.Bool("pretty", false, "pretty JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *taskFile == "" {
		*taskFile = *output
	}
	taskPath, err := issue.CheckBatch(*input, *taskFile, !*noGenerateFplsh, *pretty)
	if err != nil {
		return err
	}
	appendIssueCheckSummary(taskPath)
	return nil
}

func runIssueAuthWorkflow(args []string) error {
	fs := flag.NewFlagSet("issue-auth-workflow", flag.ContinueOnError)
	action := fs.String("action", "", "start or verify")
	taskFile := fs.String("task-file", "", "task JSON file")
	input := fs.String("input", "", "input override JSON file")
	pretty := fs.Bool("pretty", false, "pretty JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *action == "" {
		return fmt.Errorf("missing required --action")
	}
	result, err := issue.IssueAuthWorkflowOutput(*action, *taskFile, *input, *pretty)
	if err != nil {
		return err
	}
	return printJSON(result)
}

func runIssueInvoicePreviewGenerator(args []string) error {
	fs := flag.NewFlagSet("issue-invoice-preview-generator", flag.ContinueOnError)
	input := fs.String("input", "", "input JSON file")
	output := fs.String("output", "", "preview HTML output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	previewPath, err := issue.GenerateInvoicePreview(*input, *output)
	if err != nil {
		return err
	}
	common.AppendMessageLog("发票预览html已生成 ", previewPath, "\n")
	return nil
}

func runIssueFromOutput(args []string) error {
	fs := flag.NewFlagSet("issue-from-output", flag.ContinueOnError)
	taskFile := fs.String("task-file", "", "task JSON file")
	output := fs.String("output", "", "issue result markdown output")
	confirmPreview := fs.Bool("confirm-preview", false, "mark preview as confirmed before issuing")
	confirmDuplicate := fs.Bool("confirm-duplicate", false, "confirm a duplicate invoice submission inside the five-minute guard window")
	pretty := fs.Bool("pretty", false, "pretty JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return issue.IssueFromOutputWithOptions(issue.IssueFromOutputOptions{
		TaskFile:           *taskFile,
		OutputMarkdownPath: *output,
		ConfirmPreview:     *confirmPreview,
		ConfirmDuplicate:   *confirmDuplicate,
		Pretty:             *pretty,
	})
}

func printJSON(value any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func runWithFailureLog(err error, prefix, separator, suffix string) error {
	if err != nil {
		message := prefix + separator + err.Error() + suffix
		common.AppendMessageLog(message)
		return err
	}
	return nil
}

type multiValue []string

func (m *multiValue) String() string { return fmt.Sprint([]string(*m)) }
func (m *multiValue) Set(value string) error {
	*m = append(*m, value)
	return nil
}
