package main

import (
	"flag"
	"fmt"
	"io"

	taxreport "invoice-assistant-skill/go-script/lib/tax-analysis/report"
)

// runReportAnalysis dispatches local-only reading, reference lookup and JSON
// submission. None of these commands constructs an API client or logs in.
func runReportAnalysis(command string, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	workspace := flags.String(reportWorkspaceFlag, "", "report-prepare生成的工作区")
	pretty := flags.Bool("pretty", false, "缩进JSON")
	var execute func() (any, error)
	switch command {
	case commandReportRead:
		next := flags.Bool("next", false, "读取下一未完成批次，或章节完成后的综合结论材料（默认）")
		batch := flags.String("batch", "", "读取指定批次")
		section := flags.String("section", "", "读取一个正文章节")
		execute = func() (any, error) {
			if *next && (*batch != "" || *section != "") {
				return nil, fmt.Errorf("next不能与batch或section同时使用")
			}
			return taxreport.ReadAnalysis(*workspace, *batch, *section)
		}
	case commandReportRefs:
		section := flags.String("section", "", "章节语义ID；不指定则查整个工作区")
		name := flags.String("name", "", "中文指标名包含的文字")
		subject := flags.String("subject", "", "主体或品名包含的文字")
		period := flags.String("period", "", "完整所属期，精确匹配")
		base := flags.String("base-period", "", "完整比较基期，精确匹配")
		id := flags.String("id", "", "精确引用ID")
		offset := flags.Int("offset", 0, "结果起始位置")
		limit := flags.Int("limit", 50, "每页1—200条；未返回部分保留nextOffset")
		execute = func() (any, error) {
			return taxreport.QueryAnalysisReferences(*workspace, taxreport.ReferenceQuery{SectionID: *section, Name: *name, Subject: *subject, Period: *period, BasePeriod: *base, ID: *id, Offset: *offset, Limit: *limit})
		}
	case commandReportWrite:
		input := flags.String("input", "", "UTF-8 JSON，包含analysisDataDigest及sections/conclusion")
		execute = func() (any, error) { return taxreport.WriteAnalysis(*workspace, *input) }
	default:
		return writeError(stderr, fmt.Errorf("未知报告分析命令 %s", command))
	}
	if err := flags.Parse(args); err != nil {
		return 1
	}
	if flags.NArg() != 0 {
		return writeError(stderr, fmt.Errorf("不接受额外位置参数"))
	}
	result, err := execute()
	if err != nil {
		return writeError(stderr, err)
	}
	if err = writeJSON(stdout, result, *pretty); err != nil {
		return writeError(stderr, err)
	}
	if written, ok := result.(taxreport.AnalysisWriteResult); ok && !written.Valid {
		return 1
	}
	return 0
}
