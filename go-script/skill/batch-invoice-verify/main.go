package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/batch"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/cli"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/invoiceinput"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/pathx"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/runtimeio"
)

func main() {
	runtime := runtimeio.New(os.Stdout, os.Stderr)
	os.Exit(newApp().Run(context.Background(), os.Args[1:], runtime))
}

func newApp() cli.App {
	name := cli.ExecutableName("batch-invoice-verify")
	return cli.App{
		Name:    name,
		Summary: "批量发票查验命令行工具。",
		Commands: []cli.Command{
			verifyCommand(name),
			reverifyCommand(name),
			manageCommand(name),
		},
	}
}

func verifyCommand(executable string) cli.Command {
	return cli.Command{
		Name:    "verify",
		Summary: "扫描发票输入并生成中间查验结果。",
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
				inputPath, inputIsDir, err := validateInvoiceInput(input)
				if err != nil {
					return err
				}
				defaultDir := filepath.Dir(inputPath)
				if inputIsDir {
					defaultDir = inputPath
				}
				resolvedOutput, err := pathx.ResolveOutput(output, defaultDir, invoiceinput.IntermediateResultFilename, ".xlsx")
				if err != nil {
					return err
				}
				result, err := batch.Verify(ctx, batch.VerifyOptions{InputPath: inputPath, Output: resolvedOutput, APIKey: apiKey})
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

func manageCommand(executable string) cli.Command {
	return cli.Command{
		Name:    "manage",
		Summary: "处理中间查验结果并生成最终清单。",
		Usage:   fmt.Sprintf("%s manage -i <xlsx> [-o <xlsx-or-dir>] [--check-duplicate] [--history <csv>] [--check-reimburse] [--check-account] [-c <company>]", executable),
		Configure: func(flagSet *flag.FlagSet) cli.Handler {
			var input string
			var output string
			var history string
			var checkDuplicate bool
			var checkReimburse bool
			var checkAccount bool
			var company string
			flagSet.StringVar(&input, "i", "", "intermediate xlsx file")
			flagSet.StringVar(&input, "input", "", "intermediate xlsx file")
			flagSet.StringVar(&output, "o", "", "output xlsx file or directory")
			flagSet.StringVar(&output, "output", "", "output xlsx file or directory")
			flagSet.BoolVar(&checkDuplicate, "check-duplicate", false, "duplicate checking is enabled by default; flag kept for compatibility")
			flagSet.StringVar(&history, "history", "", "history csv file")
			flagSet.BoolVar(&checkReimburse, "check-reimburse", false, "enable reimbursement status checking")
			flagSet.BoolVar(&checkAccount, "check-account", false, "enable account status checking")
			flagSet.StringVar(&company, "c", "", "company name")
			flagSet.StringVar(&company, "company", "", "company name")

			return func(ctx context.Context, rt *runtimeio.Runtime) error {
				_ = ctx
				if err := cli.RequireNoArgs(flagSet); err != nil {
					return err
				}
				if err := cli.RequireNonEmpty(input, "-i/--input"); err != nil {
					return err
				}
				inputPath, err := validateExistingFile(input, "input", ".xlsx")
				if err != nil {
					return err
				}
				if strings.TrimSpace(history) != "" {
					if _, err := validateExistingFile(history, "history", ".csv"); err != nil {
						return err
					}
				}
				defaultName := pathx.BuildDatedFilename("发票查验结果清单", ".xlsx", time.Now())
				resolvedOutput, err := pathx.ResolveOutput(output, filepath.Dir(inputPath), defaultName, ".xlsx")
				if err != nil {
					return err
				}
				result, err := batch.Manage(batch.ManageOptions{
					InputPath:      inputPath,
					Output:         resolvedOutput,
					Company:        company,
					HistoryPath:    history,
					CheckDuplicate: checkDuplicate,
					CheckReimburse: checkReimburse,
					CheckAccount:   checkAccount,
				})
				if err != nil {
					return err
				}
				if result.MultiCompany != nil {
					if err := rt.WriteJSON(result.MultiCompany); err != nil {
						return err
					}
					return cli.ExitError{Code: 1}
				}
				return rt.WriteJSON(result.Success)
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
