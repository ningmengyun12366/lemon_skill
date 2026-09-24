package cli

import (
	"flag"
	"fmt"
	"strings"

	"invoice-assistant-skill/go-script/lib/apikey"
)

func RequireNoArgs(flagSet *flag.FlagSet) error {
	if flagSet.NArg() == 0 {
		return nil
	}
	return fmt.Errorf("unexpected argument: %s", flagSet.Arg(0))
}

func RequireNonEmpty(value, displayName string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("missing required flag %s", displayName)
	}
	return nil
}

func RequireAPIKey(value string) error {
	if value == "" {
		return fmt.Errorf("missing required flag --api-key")
	}
	return apikey.ValidateFormat(value)
}

func validateAPIKeyFlagArgs(args []string) error {
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--api-key" || arg == "-api-key" {
			if index+1 >= len(args) {
				return nil
			}
			if err := apikey.ValidateFormat(args[index+1]); err != nil {
				return err
			}
			index++
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--api-key="); ok {
			if err := apikey.ValidateFormat(value); err != nil {
				return err
			}
			continue
		}
		if value, ok := strings.CutPrefix(arg, "-api-key="); ok {
			if err := apikey.ValidateFormat(value); err != nil {
				return err
			}
		}
	}
	return nil
}

func FlagProvided(flagSet *flag.FlagSet, names ...string) bool {
	found := false
	lookup := make(map[string]bool, len(names))
	for _, name := range names {
		lookup[name] = true
	}
	flagSet.Visit(func(flagValue *flag.Flag) {
		if lookup[flagValue.Name] {
			found = true
		}
	})
	return found
}

func RequireFlagProvided(flagSet *flag.FlagSet, displayName string, names ...string) error {
	if FlagProvided(flagSet, names...) {
		return nil
	}
	return fmt.Errorf("missing required flag %s", displayName)
}

func ValidateAllowed(value, displayName string, allowed ...string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return fmt.Errorf("invalid %s: %s", displayName, value)
}
