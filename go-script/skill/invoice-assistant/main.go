package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"invoice-assistant-skill/go-script/lib/apikey"
	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/app"
)

func main() {
	args := os.Args[1:]
	if err := validateAPIKeyArgs(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := app.Run(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func validateAPIKeyArgs(args []string) error {
	if len(args) == 0 || args[0] != "workspace-config-writer" {
		return nil
	}
	for index := 1; index < len(args); index++ {
		arg := args[index]
		if arg == "--apikey" || arg == "-apikey" {
			if index+1 >= len(args) {
				return nil
			}
			if err := apikey.ValidateFormat(args[index+1]); err != nil {
				return err
			}
			index++
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--apikey="); ok {
			if err := apikey.ValidateFormat(value); err != nil {
				return err
			}
			continue
		}
		if value, ok := strings.CutPrefix(arg, "-apikey="); ok {
			if err := apikey.ValidateFormat(value); err != nil {
				return err
			}
			continue
		}
		if arg == "--set" || arg == "-set" {
			if index+1 >= len(args) {
				return nil
			}
			if err := validateSetAPIKey(args[index+1]); err != nil {
				return err
			}
			index++
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--set="); ok {
			if err := validateSetAPIKey(value); err != nil {
				return err
			}
			continue
		}
		if value, ok := strings.CutPrefix(arg, "-set="); ok {
			if err := validateSetAPIKey(value); err != nil {
				return err
			}
			continue
		}
		if arg == "--extra-json" || arg == "-extra-json" {
			if index+1 >= len(args) {
				return nil
			}
			if err := validateExtraJSONAPIKey(args[index+1]); err != nil {
				return err
			}
			index++
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--extra-json="); ok {
			if err := validateExtraJSONAPIKey(value); err != nil {
				return err
			}
			continue
		}
		if value, ok := strings.CutPrefix(arg, "-extra-json="); ok {
			if err := validateExtraJSONAPIKey(value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSetAPIKey(pair string) error {
	key, value, ok := strings.Cut(pair, "=")
	if !ok || strings.TrimSpace(key) != "apikey" {
		return nil
	}
	return apikey.ValidateFormat(value)
}

func validateExtraJSONAPIKey(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil
	}
	value, ok := payload["apikey"]
	if !ok {
		return nil
	}
	text, ok := value.(string)
	if !ok {
		return apikey.ValidateFormat("")
	}
	return apikey.ValidateFormat(text)
}
