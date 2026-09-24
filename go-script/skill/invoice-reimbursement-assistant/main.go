package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/cli"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/pathx"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/reimbursement"
	"invoice-assistant-skill/go-script/lib/invoice-verify-common/runtimeio"
)

func main() {
	runtime := runtimeio.New(os.Stdout, os.Stderr)
	os.Exit(newApp().Run(context.Background(), os.Args[1:], runtime))
}

func newApp() cli.App {
	name := cli.ExecutableName("invoice-reimbursement-assistant")
	return cli.App{
		Name:    name,
		Summary: "发票报销辅助命令行工具。",
		Commands: []cli.Command{
			phase1Command(name),
			reverifyCommand(name),
			phase2Command(name),
		},
	}
}

func phase1Command(executable string) cli.Command {
	return cli.Command{
		Name:    "phase1",
		Summary: "扫描邮箱附件并生成查验中间文件。",
		Usage:   fmt.Sprintf("%s phase1 --email <email> --password <auth_code> --start-date <yyyyMMdd> --end-date <yyyyMMdd> --invoice-dir <dir> --api-key <api_key> [--keyword <keyword>] [--mail-provider qq|163|gmail] [--imap-host <host>] [--port <port>] [--limit <n>]", executable),
		Configure: func(flagSet *flag.FlagSet) cli.Handler {
			var email string
			var password string
			var startDate string
			var endDate string
			var invoiceDir string
			var apiKey string
			var keyword string
			var mailProvider string
			var imapHost string
			var port int
			var limit int
			flagSet.StringVar(&email, "email", "", "mailbox email address")
			flagSet.StringVar(&password, "password", "", "mailbox auth code")
			flagSet.StringVar(&startDate, "start-date", "", "start date yyyyMMdd")
			flagSet.StringVar(&endDate, "end-date", "", "end date yyyyMMdd")
			flagSet.StringVar(&invoiceDir, "invoice-dir", "", "invoice attachment directory")
			flagSet.StringVar(&apiKey, "api-key", "", "invoice verify API key")
			flagSet.StringVar(&keyword, "keyword", "", "mail keyword")
			flagSet.StringVar(&mailProvider, "mail-provider", "", "mail provider qq, 163, or gmail")
			flagSet.StringVar(&imapHost, "imap-host", "", "custom IMAP host")
			flagSet.IntVar(&port, "port", 993, "IMAP port")
			flagSet.IntVar(&limit, "limit", 0, "maximum mail count")

			return func(ctx context.Context, rt *runtimeio.Runtime) error {
				if err := cli.RequireNoArgs(flagSet); err != nil {
					return err
				}
				for _, required := range []struct {
					value string
					name  string
				}{
					{email, "--email"},
					{password, "--password"},
					{startDate, "--start-date"},
					{endDate, "--end-date"},
					{invoiceDir, "--invoice-dir"},
					{apiKey, "--api-key"},
				} {
					if err := cli.RequireNonEmpty(required.value, required.name); err != nil {
						return err
					}
				}
				if err := cli.RequireAPIKey(apiKey); err != nil {
					return err
				}
				start, err := parseContractDate(startDate, "--start-date")
				if err != nil {
					return err
				}
				end, err := parseContractDate(endDate, "--end-date")
				if err != nil {
					return err
				}
				if end.Before(start) {
					return errors.New("--end-date must not be earlier than --start-date")
				}
				if err := cli.ValidateAllowed(mailProvider, "--mail-provider", "qq", "163", "gmail"); err != nil {
					return err
				}
				if port <= 0 {
					return errors.New("--port must be greater than 0")
				}
				if limit < 0 {
					return errors.New("--limit must be greater than or equal to 0")
				}
				result, err := reimbursement.Phase1(ctx, reimbursement.Phase1Options{
					Email:        email,
					Password:     password,
					StartDate:    start,
					EndDate:      end,
					InvoiceDir:   invoiceDir,
					APIKey:       apiKey,
					Keyword:      keyword,
					MailProvider: mailProvider,
					IMAPHost:     imapHost,
					Port:         port,
					Limit:        limit,
				})
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
		Usage:   fmt.Sprintf("%s reverify --verify-excel <intermediate.xlsx> [--output-excel <xlsx-or-dir>] --api-key <api_key>", executable),
		Configure: func(flagSet *flag.FlagSet) cli.Handler {
			var verifyExcel string
			var outputExcel string
			var apiKey string
			flagSet.StringVar(&verifyExcel, "verify-excel", "", "intermediate verify xlsx file")
			flagSet.StringVar(&outputExcel, "output-excel", "", "output xlsx file or directory")
			flagSet.StringVar(&apiKey, "api-key", "", "invoice verify API key")

			return func(ctx context.Context, rt *runtimeio.Runtime) error {
				if err := cli.RequireNoArgs(flagSet); err != nil {
					return err
				}
				if err := cli.RequireNonEmpty(verifyExcel, "--verify-excel"); err != nil {
					return err
				}
				if err := cli.RequireAPIKey(apiKey); err != nil {
					return err
				}
				verifyPath, err := validateExistingFile(verifyExcel, "verify-excel", ".xlsx")
				if err != nil {
					return err
				}
				result, err := cli.RunReverify(ctx, cli.ReverifyOptions{
					InputPath: verifyPath,
					Output:    outputExcel,
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

func phase2Command(executable string) cli.Command {
	return cli.Command{
		Name:    "phase2",
		Summary: "根据查验中间文件生成报销清单。",
		Usage:   fmt.Sprintf("%s phase2 --verify-excel <xlsx> [--keywords-file <json>] [--output-excel <xlsx-or-dir>] [--company <company>]", executable),
		Configure: func(flagSet *flag.FlagSet) cli.Handler {
			var verifyExcel string
			var keywordsFile string
			var outputExcel string
			var company string
			flagSet.StringVar(&verifyExcel, "verify-excel", "", "verify xlsx file")
			flagSet.StringVar(&keywordsFile, "keywords-file", "", "classification keywords json file")
			flagSet.StringVar(&outputExcel, "output-excel", "", "output xlsx file or directory")
			flagSet.StringVar(&company, "company", "", "company name")

			return func(ctx context.Context, rt *runtimeio.Runtime) error {
				_ = ctx
				if err := cli.RequireNoArgs(flagSet); err != nil {
					return err
				}
				if err := cli.RequireNonEmpty(verifyExcel, "--verify-excel"); err != nil {
					return err
				}
				verifyPath, err := validateExistingFile(verifyExcel, "verify-excel", ".xlsx")
				if err != nil {
					return err
				}
				if strings.TrimSpace(keywordsFile) != "" {
					if _, err := validateExistingFile(keywordsFile, "keywords-file", ".json"); err != nil {
						return err
					}
				}
				result, err := reimbursement.Phase2(reimbursement.Phase2Options{
					VerifyExcel:  verifyPath,
					KeywordsFile: keywordsFile,
					OutputExcel:  outputExcel,
					Company:      company,
				})
				if err != nil {
					return err
				}
				if result.MultiCompany != nil {
					if writeErr := rt.WriteJSON(result.MultiCompany); writeErr != nil {
						return writeErr
					}
					return cli.ExitError{Code: 1}
				}
				return rt.WriteJSON(result.Success)
			}
		},
	}
}

func parseContractDate(value, displayName string) (time.Time, error) {
	parsed, err := time.Parse("20060102", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid %s, expected yyyyMMdd", displayName)
	}
	return parsed, nil
}

func validateExistingDir(raw, label string) (string, error) {
	path, err := pathx.Normalize(raw)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("%s directory does not exist: %s", label, path)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s must be a directory: %s", label, path)
	}
	return path, nil
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
