package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/analysis"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/batch"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/cli"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/pathx"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/reportpdf"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/runtimeio"
)

func main() {
	runtime := runtimeio.New(os.Stdout, os.Stderr)
	os.Exit(newApp().Run(context.Background(), os.Args[1:], runtime))
}

func newApp() cli.App {
	name := cli.ExecutableName("invoice-verify-analysis")
	return cli.App{
		Name:    name,
		Summary: "发票查验分析命令行工具。",
		Commands: []cli.Command{
			verifyCommand(name),
			reverifyCommand(name),
			analysisCommand(name),
			reportPDFCommand(name),
		},
	}
}

func verifyCommand(executable string) cli.Command {
	return cli.Command{
		Name:    "verify",
		Summary: "扫描发票输入并生成分析用查验结果。",
		Usage:   fmt.Sprintf("%s verify -i <folder-or-xlsx-or-csv> [-o <xlsx-or-dir>] --api-key <api_key>", executable),
		Configure: func(flagSet *flag.FlagSet) cli.Handler {
			var input string
			var output string
			var apiKey string
			flagSet.StringVar(&input, "i", "", "invoice folder or list file")
			flagSet.StringVar(&input, "input", "", "invoice folder or list file")
			flagSet.StringVar(&output, "o", "", "output xlsx file or directory")
			flagSet.StringVar(&output, "output", "", "output xlsx file or directory")
			flagSet.StringVar(&apiKey, "api-key", "", "invoice verify API key")

			return func(ctx context.Context, rt *runtimeio.Runtime) error {
				if err := cli.RequireNoArgs(flagSet); err != nil {
					return err
				}
				if err := cli.RequireNonEmpty(input, "-i/--input"); err != nil {
					return err
				}
				if err := cli.RequireAPIKey(apiKey); err != nil {
					return err
				}
				inputPath, _, err := validateInvoiceInput(input)
				if err != nil {
					return err
				}
				result, err := batch.Verify(ctx, batch.VerifyOptions{InputPath: inputPath, Output: output, APIKey: apiKey})
				if err != nil {
					return err
				}
				return rt.WriteJSON(result)
			}
		},
	}
}

func reverifyCommand(executable string) cli.Command {
	return cli.Command{
		Name:    "reverify",
		Summary: "Reverify failed or empty rows in an intermediate workbook.",
		Usage:   fmt.Sprintf("%s reverify -i <intermediate.xlsx> [-o <xlsx-or-dir>] --api-key <api_key>", executable),
		Configure: func(flagSet *flag.FlagSet) cli.Handler {
			var input string
			var output string
			var apiKey string
			flagSet.StringVar(&input, "i", "", "intermediate xlsx file")
			flagSet.StringVar(&input, "input", "", "intermediate xlsx file")
			flagSet.StringVar(&output, "o", "", "output xlsx file or directory")
			flagSet.StringVar(&output, "output", "", "output xlsx file or directory")
			flagSet.StringVar(&apiKey, "api-key", "", "invoice verify API key")

			return func(ctx context.Context, rt *runtimeio.Runtime) error {
				if err := cli.RequireNoArgs(flagSet); err != nil {
					return err
				}
				if err := cli.RequireNonEmpty(input, "-i/--input"); err != nil {
					return err
				}
				if err := cli.RequireAPIKey(apiKey); err != nil {
					return err
				}
				inputPath, err := validateExistingFile(input, "input", ".xlsx")
				if err != nil {
					return err
				}
				result, err := cli.RunReverify(ctx, cli.ReverifyOptions{
					InputPath: inputPath,
					Output:    output,
					APIKey:    apiKey,
				})
				if err != nil {
					return err
				}
				return rt.WriteJSON(result)
			}
		},
	}
}

func analysisCommand(executable string) cli.Command {
	return cli.Command{
		Name:    "analysis",
		Summary: "分析查验结果并生成企业清单。",
		Usage:   fmt.Sprintf("%s analysis -i <xlsx> -c <company> [-o <xlsx-or-dir>]", executable),
		Configure: func(flagSet *flag.FlagSet) cli.Handler {
			var input string
			var company string
			var output string
			flagSet.StringVar(&input, "i", "", "verify xlsx file")
			flagSet.StringVar(&input, "input", "", "verify xlsx file")
			flagSet.StringVar(&company, "c", "", "company name")
			flagSet.StringVar(&company, "company", "", "company name")
			flagSet.StringVar(&output, "o", "", "output xlsx file or directory")
			flagSet.StringVar(&output, "output", "", "output xlsx file or directory")

			return func(ctx context.Context, rt *runtimeio.Runtime) error {
				_ = ctx
				if err := cli.RequireNoArgs(flagSet); err != nil {
					return err
				}
				if err := cli.RequireNonEmpty(input, "-i/--input"); err != nil {
					return err
				}
				if err := cli.RequireNonEmpty(company, "-c/--company"); err != nil {
					return err
				}
				inputPath, err := validateExistingFile(input, "input", ".xlsx")
				if err != nil {
					return err
				}
				defaultName := pathx.BuildDatedFilename("企业发票查验结果清单", ".xlsx", time.Now())
				resolvedOutput, err := pathx.ResolveOutput(output, filepath.Dir(inputPath), defaultName, ".xlsx")
				if err != nil {
					return err
				}
				result, err := analysis.Run(analysis.Options{InputPath: inputPath, Output: resolvedOutput, Company: company})
				if err != nil {
					return err
				}
				return rt.WriteJSON(result)
			}
		},
	}
}

func reportPDFCommand(executable string) cli.Command {
	return cli.Command{
		Name:    "report-pdf",
		Summary: "将 UTF-8 文本报告转换为 PDF。",
		Usage:   fmt.Sprintf("%s report-pdf --input-text-file <utf8-txt> --output <pdf>", executable),
		Configure: func(flagSet *flag.FlagSet) cli.Handler {
			var inputTextFile string
			var output string
			flagSet.StringVar(&inputTextFile, "input-text-file", "", "UTF-8 report text file")
			flagSet.StringVar(&output, "output", "", "output pdf file")

			return func(ctx context.Context, rt *runtimeio.Runtime) error {
				_ = ctx
				if err := cli.RequireNoArgs(flagSet); err != nil {
					return err
				}
				if err := cli.RequireNonEmpty(inputTextFile, "--input-text-file"); err != nil {
					return err
				}
				if err := cli.RequireNonEmpty(output, "--output"); err != nil {
					return err
				}
				inputPath, err := validateExistingFile(inputTextFile, "input-text-file", ".txt")
				if err != nil {
					return err
				}
				resolvedOutput, err := pathx.ResolveRequiredFile(output, ".pdf")
				if err != nil {
					return err
				}
				result, err := reportpdf.Run(reportpdf.Options{InputTextFile: inputPath, Output: resolvedOutput})
				if err != nil {
					return err
				}
				return rt.WriteJSON(result)
			}
		},
	}
}

func validateInvoiceInput(raw string) (string, bool, error) {
	path, err := pathx.Normalize(raw)
	if err != nil {
		return "", false, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", false, fmt.Errorf("输入路径不存在或该文件不支持查验: %s", path)
	}
	if info.IsDir() {
		return path, true, nil
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".xlsx" && ext != ".csv" {
		return "", false, fmt.Errorf("输入路径不存在或该文件不支持查验: %s", path)
	}
	return path, false, nil
}

func validateExistingFile(raw, label, requiredExt string) (string, error) {
	path, err := pathx.Normalize(raw)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("%s file does not exist: %s", label, path)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s must be a file: %s", label, path)
	}
	if requiredExt != "" && !strings.EqualFold(filepath.Ext(path), requiredExt) {
		return "", fmt.Errorf("%s must use %s suffix: %s", label, requiredExt, path)
	}
	return path, nil
}
