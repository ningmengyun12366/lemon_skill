package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/skillupdate"
	taxanalysis "invoice-assistant-skill/go-script/lib/tax-analysis"
	taxreport "invoice-assistant-skill/go-script/lib/tax-analysis/report"
)

var versionCheckerFactory = skillupdate.Shared

var newTaxAnalysisClient = newClient

var prepareLoginReadiness = func(ctx context.Context, client *taxanalysis.Client, companyOrUSCC string) (taxanalysis.LoginReadinessResult, error) {
	return client.PrepareLoginReadiness(ctx, companyOrUSCC)
}

var resolveBoundEnterprise = func(ctx context.Context, client *taxanalysis.Client, companyOrUSCC string) (map[string]any, error) {
	return client.ResolveBoundEnterprise(ctx, companyOrUSCC)
}

var listBoundEnterprises = func(ctx context.Context, client *taxanalysis.Client) ([]taxanalysis.BoundEnterpriseChoice, error) {
	return client.ListBoundEnterprises(ctx)
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || isHelp(args[0]) {
		printHelp(stdout)
		return 0
	}
	if args[0] == "help" {
		printHelp(stdout)
		return 0
	}
	switch args[0] {
	case commandRun:
		return runCreateOrPoll(ctx, args[1:], true, stdout, stderr)
	case commandCreate:
		return runCreateOrPoll(ctx, args[1:], false, stdout, stderr)
	case commandQuery:
		return runQuery(ctx, args[1:], stdout, stderr)
	case commandReportPrepare:
		return runReportPrepare(ctx, args[1:], stdout, stderr)
	case commandReportRead, commandReportRefs, commandReportWrite:
		return runReportAnalysis(args[0], args[1:], stdout, stderr)
	case commandReportValidate:
		return runReportValidate(args[1:], stdout, stderr)
	case commandReport, commandReportRender:
		return runReportRender(ctx, args[1:], stdout, stderr)
	case commandEnterprises:
		return runEnterprises(ctx, args[1:], stdout, stderr)
	case commandUnpackRuntime:
		return runUnpackRuntime(args[1:], stdout, stderr)
	default:
		return writeError(stderr, fmt.Errorf("unknown subcommand %q", args[0]))
	}
}

type enterpriseListResult struct {
	Status      string                              `json:"status"`
	Enterprises []taxanalysis.BoundEnterpriseChoice `json:"enterprises"`
}

// runEnterprises 列出当前 API Key 的绑定企业，供用户在业务命令前明确选择。
//
// @param ctx 调用上下文。
// @param args enterprises 子命令参数。
// @param stdout 成功 JSON 输出流。
// @param stderr 参数或接口错误输出流。
// @returns 查询成功返回 0，否则返回 1。
func runEnterprises(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet(commandEnterprises, flag.ContinueOnError)
	flags.SetOutput(stderr)
	apiKey := flags.String("api-key", "", "Open Tax API Key")
	pretty := flags.Bool("pretty", false, "缩进 stdout JSON")
	if err := flags.Parse(args); err != nil {
		return 1
	}
	client, err := newTaxAnalysisClient(strings.TrimSpace(*apiKey))
	if err != nil {
		return writeError(stderr, err)
	}
	enterprises, err := listBoundEnterprises(ctx, client)
	if err != nil {
		return writeError(stderr, err)
	}
	if err := writeJSON(stdout, enterpriseListResult{Status: "success", Enterprises: enterprises}, *pretty); err != nil {
		return writeError(stderr, err)
	}
	return 0
}

type unpackRuntimeResult struct {
	Status      string `json:"status"`
	RuntimePath string `json:"runtimePath"`
}

// runUnpackRuntime 校验并解压当前平台的便携 Node.js 运行时。
//
// @param args unpack-runtime 子命令参数。
// @param stdout 成功 JSON 输出流。
// @param stderr 参数或安装错误输出流。
// @returns 安装成功返回 0，否则返回 1。
func runUnpackRuntime(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet(commandUnpackRuntime, flag.ContinueOnError)
	flags.SetOutput(stderr)
	archivePath := flags.String("archive", "", "当前平台便携 Node.js 的 .gz 下载文件")
	outputPath := flags.String("output", "", "解压后的 Node.js 可执行文件路径")
	pretty := flags.Bool("pretty", false, "缩进 stdout JSON")
	if err := flags.Parse(args); err != nil {
		return 1
	}
	runtimePath, err := taxreport.InstallChartRendererRuntimeArchive(*archivePath, *outputPath)
	if err != nil {
		return writeError(stderr, err)
	}
	if err := writeJSON(stdout, unpackRuntimeResult{Status: "success", RuntimePath: runtimePath}, *pretty); err != nil {
		return writeError(stderr, err)
	}
	return 0
}

// runReportPrepare fetches one complete tax-analysis task and freezes the
// trusted report workspace that an Agent must fill.
//
// @param ctx 调用上下文。
// @param args report-prepare 子命令参数。
// @param stdout 成功 JSON 输出流。
// @param stderr 参数、登录或取数错误输出流。
// @returns 工作区创建成功返回 0，否则返回 1。
func runReportPrepare(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet(commandReportPrepare, flag.ContinueOnError)
	flags.SetOutput(stderr)
	company := flags.String(enterpriseNameFlag, "", "当前 API Key 已绑定的完整企业名称")
	uscc := flags.String(enterpriseUSCCFlag, "", "当前 API Key 已绑定企业的统一社会信用代码")
	startMonth := flags.String("start-month", "", "开始月份 YYYY-MM")
	endMonth := flags.String("end-month", "", "结束月份 YYYY-MM")
	apiKey := flags.String("api-key", "", "Open Tax API Key")
	workspaceDir := flags.String(reportWorkspaceFlag, "", "可信分析工作区目录")
	forceRefresh := flags.Bool("force-refresh", false, "首次创建时强制刷新全部支持模块")
	pretty := flags.Bool("pretty", false, "缩进 stdout JSON")
	pollInterval := flags.Duration("poll-interval", taxanalysis.DefaultPollInterval, "轮询间隔")
	pollTimeout := flags.Duration("poll-timeout", taxanalysis.DefaultPollTimeout, "单次任务最长轮询时间")
	if err := flags.Parse(args); err != nil {
		return 1
	}
	reportTimeout, err := validatePollingOptions(*pollInterval, *pollTimeout, reportPollAttemptCount)
	if err != nil {
		return writeError(stderr, err)
	}
	selector, err := enterpriseSelector(*company, *uscc)
	if err != nil {
		return writeError(stderr, err)
	}
	request := taxanalysis.CreateTaskRequest{
		StartMonth:   strings.TrimSpace(*startMonth),
		EndMonth:     strings.TrimSpace(*endMonth),
		ForceRefresh: *forceRefresh,
	}
	client, err := newTaxAnalysisClient(strings.TrimSpace(*apiKey))
	if err != nil {
		return writeError(stderr, err)
	}
	preparationStarted := time.Now()
	readiness, err := prepareLoginReadiness(ctx, client, selector)
	if err != nil {
		return writeError(stderr, err)
	}
	request.USCC, err = resolvedEnterpriseUSCC(readiness.Enterprise)
	if err != nil {
		return writeError(stderr, err)
	}
	if err := taxanalysis.ValidateCreateTaskRequest(request); err != nil {
		return writeError(stderr, err)
	}
	reportContext, cancel := context.WithTimeout(ctx, reportTimeout)
	defer cancel()
	result, err := taxreport.PrepareWorkspace(reportContext, client, request, taxreport.PrepareOptions{
		WorkspaceDir: strings.TrimSpace(*workspaceDir), PollInterval: *pollInterval,
		EnterpriseContext: readiness.Enterprise, PreparationMilliseconds: time.Since(preparationStarted).Milliseconds(),
	})
	if err != nil {
		return writeError(stderr, err)
	}
	if err := writeJSON(stdout, result, *pretty); err != nil {
		return writeError(stderr, err)
	}
	return 0
}

// runReportValidate validates the Agent-authored analysis-output.json and
// records the latest result without limiting the Agent's correction attempts.
//
// @param args report-validate 子命令参数。
// @param stdout 结构化校验结果输出流。
// @param stderr 参数或工作区错误输出流。
// @returns 校验通过返回 0，内容不通过或执行错误返回 1。
func runReportValidate(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet(commandReportValidate, flag.ContinueOnError)
	flags.SetOutput(stderr)
	workspaceDir := flags.String(reportWorkspaceFlag, "", "已由report-prepare创建的分析工作区")
	pretty := flags.Bool("pretty", false, "缩进 stdout JSON")
	if err := flags.Parse(args); err != nil {
		return 1
	}
	result, err := taxreport.ValidateWorkspace(strings.TrimSpace(*workspaceDir), true)
	if err != nil {
		return writeError(stderr, err)
	}
	if err := writeJSON(stdout, result, *pretty); err != nil {
		return writeError(stderr, err)
	}
	if !result.Valid {
		return 1
	}
	return 0
}

// runReportRender revalidates a prepared and Agent-filled workspace and emits
// the atomic DOCX/PDF pair. The report command is an alias of this workflow.
//
// @param ctx 调用上下文。
// @param args report-render 或 report 子命令参数。
// @param stdout 成功 JSON 输出流。
// @param stderr 参数、校验或渲染错误输出流。
// @returns 两种成品均生成成功返回 0，否则返回 1。
func runReportRender(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet(commandReportRender, flag.ContinueOnError)
	flags.SetOutput(stderr)
	workspaceDir := flags.String(reportWorkspaceFlag, "", "已通过AI分析校验的工作区")
	outputDir := flags.String("output-dir", "", "DOCX 和 PDF 输出目录")
	pretty := flags.Bool("pretty", false, "缩进 stdout JSON")
	if err := flags.Parse(args); err != nil {
		return 1
	}
	result, err := taxreport.RenderWorkspace(ctx, strings.TrimSpace(*workspaceDir), strings.TrimSpace(*outputDir), taxreport.GenerateOptions{})
	if err != nil {
		return writeError(stderr, err)
	}
	if err := writeJSON(stdout, result, *pretty); err != nil {
		return writeError(stderr, err)
	}
	return 0
}

func runCreateOrPoll(
	ctx context.Context,
	args []string,
	poll bool,
	stdout io.Writer,
	stderr io.Writer,
) int {
	flags := flag.NewFlagSet("create", flag.ContinueOnError)
	flags.SetOutput(stderr)
	company := flags.String(enterpriseNameFlag, "", "当前 API Key 已绑定的完整企业名称")
	uscc := flags.String(enterpriseUSCCFlag, "", "当前 API Key 已绑定企业的统一社会信用代码")
	startMonth := flags.String("start-month", "", "开始月份 YYYY-MM")
	endMonth := flags.String("end-month", "", "结束月份 YYYY-MM")
	apiKey := flags.String("api-key", "", "Open Tax API Key")
	forceRefresh := flags.Bool("force-refresh", false, "强制刷新全部支持模块")
	pretty := flags.Bool("pretty", false, "缩进 stdout JSON")
	pollInterval := flags.Duration("poll-interval", taxanalysis.DefaultPollInterval, "轮询间隔")
	pollTimeout := flags.Duration("poll-timeout", taxanalysis.DefaultPollTimeout, "最长轮询时间")
	if err := flags.Parse(args); err != nil {
		return 1
	}
	if poll {
		if _, err := validatePollingOptions(*pollInterval, *pollTimeout, 1); err != nil {
			return writeError(stderr, err)
		}
	}
	selector, err := enterpriseSelector(*company, *uscc)
	if err != nil {
		return writeError(stderr, err)
	}
	request := taxanalysis.CreateTaskRequest{
		StartMonth: strings.TrimSpace(*startMonth),
		EndMonth:   strings.TrimSpace(*endMonth), ForceRefresh: *forceRefresh,
	}
	client, err := newTaxAnalysisClient(strings.TrimSpace(*apiKey))
	if err != nil {
		return writeError(stderr, err)
	}
	readiness, err := prepareLoginReadiness(ctx, client, selector)
	if err != nil {
		return writeError(stderr, err)
	}
	request.USCC, err = resolvedEnterpriseUSCC(readiness.Enterprise)
	if err != nil {
		return writeError(stderr, err)
	}
	if err := taxanalysis.ValidateCreateTaskRequest(request); err != nil {
		return writeError(stderr, err)
	}
	var result taxanalysis.TaskResponse
	if poll {
		pollContext, cancel := context.WithTimeout(ctx, *pollTimeout)
		defer cancel()
		result, err = client.Run(pollContext, request, *pollInterval)
	} else {
		result, err = client.CreateTask(ctx, request)
	}
	if err != nil {
		return writeError(stderr, err)
	}
	if err := writeJSON(stdout, result, *pretty); err != nil {
		return writeError(stderr, err)
	}
	if result.Status == taxanalysis.StatusFailed || (result.Status == taxanalysis.StatusInterrupted && result.Terminal()) {
		return 1
	}
	return 0
}

func runQuery(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("query", flag.ContinueOnError)
	flags.SetOutput(stderr)
	company := flags.String(enterpriseNameFlag, "", "当前 API Key 已绑定的完整企业名称")
	uscc := flags.String(enterpriseUSCCFlag, "", "当前 API Key 已绑定企业的统一社会信用代码")
	taskID := flags.String("task-id", "", "create 返回的确定性任务 ID")
	apiKey := flags.String("api-key", "", "Open Tax API Key")
	pretty := flags.Bool("pretty", false, "缩进 stdout JSON")
	if err := flags.Parse(args); err != nil {
		return 1
	}
	selector, err := enterpriseSelector(*company, *uscc)
	if err != nil {
		return writeError(stderr, err)
	}
	client, err := newTaxAnalysisClient(strings.TrimSpace(*apiKey))
	if err != nil {
		return writeError(stderr, err)
	}
	enterprise, err := resolveBoundEnterprise(ctx, client, selector)
	if err != nil {
		return writeError(stderr, err)
	}
	resolvedUSCC, err := resolvedEnterpriseUSCC(enterprise)
	if err != nil {
		return writeError(stderr, err)
	}
	result, err := client.QueryTask(ctx, taxanalysis.QueryTaskRequest{
		USCC: resolvedUSCC, TaskID: strings.TrimSpace(*taskID),
	})
	if err != nil {
		return writeError(stderr, err)
	}
	if err := writeJSON(stdout, result, *pretty); err != nil {
		return writeError(stderr, err)
	}
	if result.Status == taxanalysis.StatusFailed || (result.Status == taxanalysis.StatusInterrupted && result.Terminal()) {
		return 1
	}
	return 0
}

// enterpriseSelector 校验企业名称和 USCC 两种兼容输入，并返回唯一选择条件。
//
// @param company 用户提供的完整企业名称。
// @param uscc 用户提供的统一社会信用代码。
// @returns 唯一的企业选择条件；同时提供或均未提供时返回错误。
func enterpriseSelector(company, uscc string) (string, error) {
	company = strings.TrimSpace(company)
	uscc = strings.TrimSpace(uscc)
	if company != "" && uscc != "" {
		return "", fmt.Errorf("--%s and --%s cannot be used together", enterpriseNameFlag, enterpriseUSCCFlag)
	}
	if company != "" {
		return company, nil
	}
	if uscc != "" {
		return uscc, nil
	}
	return "", fmt.Errorf("either --%s or --%s is required", enterpriseNameFlag, enterpriseUSCCFlag)
}

// resolvedEnterpriseUSCC 只接受绑定企业接口返回的统一社会信用代码。
//
// @param enterprise 已通过绑定企业查询获得的企业快照。
// @returns 后续税务分析请求使用的 USCC；字段缺失时返回错误。
func resolvedEnterpriseUSCC(enterprise map[string]any) (string, error) {
	uscc := strings.TrimSpace(invoiceValue(enterprise, "uscc"))
	if uscc == "" {
		return "", fmt.Errorf("bound enterprise response is missing uscc")
	}
	return uscc, nil
}

// invoiceValue 将企业快照字段转换为字符串。
//
// @param source 企业快照。
// @param key 待读取的字段名。
// @returns 字段不存在时返回空字符串，否则返回其字符串形式。
func invoiceValue(source map[string]any, key string) string {
	if source == nil {
		return ""
	}
	value, exists := source[key]
	if !exists || value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func newClient(apiKey string) (*taxanalysis.Client, error) {
	config, err := loadRuntimeConfig()
	if err != nil {
		return nil, err
	}
	return taxanalysis.NewClient(taxanalysis.ClientConfig{
		BaseURL:           config.BaseURL,
		APIKey:            apiKey,
		VersionChecker:    versionCheckerFactory(config.SkillName, config.InstallDocURL),
		AllowLoopbackHTTP: config.Environment == taxAnalysisTestEnvironment,
	})
}

func writeJSON(writer io.Writer, value any, pretty bool) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(value)
}

func writeError(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintf(stderr, "diagnostic: %s\n", skillupdate.UserFacingError(err))
	return 1
}

func isHelp(value string) bool {
	return value == "-h" || value == "--help"
}

func printHelp(writer io.Writer) {
	_, _ = fmt.Fprintf(writer, "%s\n\n", commandName)
	_, _ = fmt.Fprintln(writer, "Fetch the 17 tax-analysis modules, prepare trusted AI analysis, and render matching DOCX/PDF reports.")
	_, _ = fmt.Fprintln(writer, "\nUsage:")
	_, _ = fmt.Fprintln(writer, "  tax-analysis enterprises --api-key <key> [--pretty]")
	_, _ = fmt.Fprintln(writer, "  tax-analysis run (--company <name> | --uscc <uscc>) --start-month YYYY-MM --end-month YYYY-MM --api-key <key> [--pretty]")
	_, _ = fmt.Fprintln(writer, "  tax-analysis create (--company <name> | --uscc <uscc>) --start-month YYYY-MM --end-month YYYY-MM --api-key <key>")
	_, _ = fmt.Fprintln(writer, "  tax-analysis query (--company <name> | --uscc <uscc>) --task-id <taskId> --api-key <key>")
	_, _ = fmt.Fprintln(writer, "  tax-analysis report-prepare (--company <name> | --uscc <uscc>) --start-month YYYY-MM --end-month YYYY-MM --api-key <key> --workspace <dir> [--pretty]")
	_, _ = fmt.Fprintln(writer, "  tax-analysis report-read --workspace <dir> [--next | --batch <id> | --section <id>] [--pretty]")
	_, _ = fmt.Fprintln(writer, "  tax-analysis report-refs --workspace <dir> [--section <id>] [--name <中文指标>] [--subject <主体>] [--period <属期>] [--base-period <基期>] [--id <引用>] [--offset 0] [--limit 50]")
	_, _ = fmt.Fprintln(writer, "  tax-analysis report-write --workspace <dir> --input <正文片段.json> [--pretty]")
	_, _ = fmt.Fprintln(writer, "  tax-analysis report-validate --workspace <dir> [--pretty]")
	_, _ = fmt.Fprintln(writer, "  tax-analysis report-render --workspace <dir> --output-dir <dir> [--pretty]")
	_, _ = fmt.Fprintln(writer, "  tax-analysis report --workspace <dir> --output-dir <dir> [--pretty]  # report-render alias")
	_, _ = fmt.Fprintln(writer, "  tax-analysis unpack-runtime --archive <node.gz> --output <node-path> [--pretty]")
	_, _ = fmt.Fprintln(writer, "\nUse --force-refresh only when the user explicitly requests a full refresh.")
}
