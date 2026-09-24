package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"invoice-assistant-skill/go-script/lib/invoice-verify-common/runtimeio"
	"invoice-assistant-skill/go-script/lib/skillupdate"
)

type Handler func(context.Context, *runtimeio.Runtime) error

type ExitError struct {
	Code int
}

func (e ExitError) Error() string {
	return fmt.Sprintf("exit %d", e.Code)
}

type ConfigureFunc func(*flag.FlagSet) Handler

type Command struct {
	Name      string
	Summary   string
	Usage     string
	Configure ConfigureFunc
}

type App struct {
	Name     string
	Summary  string
	Commands []Command
}

func ExecutableName(defaultName string) string {
	name := filepath.Base(os.Args[0])
	if strings.TrimSpace(name) == "" {
		if runtime.GOOS == "windows" {
			return defaultName + ".exe"
		}
		return defaultName
	}
	return name
}

func (a App) Run(ctx context.Context, args []string, rt *runtimeio.Runtime) (exitCode int) {
	if rt == nil {
		rt = runtimeio.New(io.Discard, io.Discard)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err := fmt.Errorf("unexpected failure: %v", recovered)
			rt.Logf("diagnostic: %s", err)
			_ = rt.WriteFailure(err)
			exitCode = 1
		}
	}()

	if len(args) == 0 || isHelpFlag(args[0]) {
		a.PrintHelp(rt.Stdout())
		return 0
	}

	if args[0] == "help" {
		if len(args) == 1 {
			a.PrintHelp(rt.Stdout())
			return 0
		}
		command, ok := a.commandByName(args[1])
		if !ok {
			return a.fail(rt, fmt.Errorf("unknown subcommand %q", args[1]))
		}
		a.PrintCommandHelp(rt.Stdout(), command)
		return 0
	}

	command, ok := a.commandByName(args[0])
	if !ok {
		return a.fail(rt, fmt.Errorf("unknown subcommand %q", args[0]))
	}

	if containsHelp(args[1:]) {
		a.PrintCommandHelp(rt.Stdout(), command)
		return 0
	}

	flagSet := flag.NewFlagSet(command.Name, flag.ContinueOnError)
	flagSet.SetOutput(rt.Stderr())
	flagSet.Usage = func() { a.PrintCommandHelp(rt.Stderr(), command) }

	handler := command.Configure(flagSet)
	if flagSet.Lookup("api-key") != nil {
		if err := validateAPIKeyFlagArgs(args[1:]); err != nil {
			return a.fail(rt, err)
		}
	}
	if err := flagSet.Parse(args[1:]); err != nil {
		return a.fail(rt, err)
	}
	if handler == nil {
		return a.fail(rt, fmt.Errorf("subcommand %q is not configured", command.Name))
	}
	if err := handler(ctx, rt); err != nil {
		var exitErr ExitError
		if errors.As(err, &exitErr) {
			if exitErr.Code == 0 {
				return 0
			}
			return exitErr.Code
		}
		return a.fail(rt, err)
	}
	return 0
}

func (a App) PrintHelp(writer io.Writer) {
	_, _ = fmt.Fprintf(writer, "%s\n\n", a.Name)
	if strings.TrimSpace(a.Summary) != "" {
		_, _ = fmt.Fprintf(writer, "%s\n\n", a.Summary)
	}
	_, _ = fmt.Fprintf(writer, "Usage:\n  %s <subcommand> [flags]\n\n", a.Name)
	_, _ = fmt.Fprintln(writer, "Subcommands:")
	for _, command := range a.Commands {
		_, _ = fmt.Fprintf(writer, "  %-12s %s\n", command.Name, command.Summary)
	}
	_, _ = fmt.Fprintf(writer, "\nRun `%s help <subcommand>` for subcommand flags.\n", a.Name)
}

func (a App) PrintCommandHelp(writer io.Writer, command Command) {
	_, _ = fmt.Fprintf(writer, "%s %s\n\n", a.Name, command.Name)
	if strings.TrimSpace(command.Summary) != "" {
		_, _ = fmt.Fprintf(writer, "%s\n\n", command.Summary)
	}
	usage := command.Usage
	if strings.TrimSpace(usage) == "" {
		usage = fmt.Sprintf("%s %s [flags]", a.Name, command.Name)
	}
	_, _ = fmt.Fprintf(writer, "Usage:\n  %s\n\n", usage)

	flagSet := flag.NewFlagSet(command.Name, flag.ContinueOnError)
	flagSet.SetOutput(writer)
	if command.Configure != nil {
		_ = command.Configure(flagSet)
	}
	hasFlags := false
	flagSet.VisitAll(func(*flag.Flag) { hasFlags = true })
	if hasFlags {
		_, _ = fmt.Fprintln(writer, "Flags:")
		flagSet.PrintDefaults()
	}
}

func (a App) commandByName(name string) (Command, bool) {
	for _, command := range a.Commands {
		if command.Name == name {
			return command, true
		}
	}
	return Command{}, false
}

func (a App) fail(rt *runtimeio.Runtime, err error) int {
	err = skillupdate.UserFacingError(err)
	rt.Logf("diagnostic: %s", err)
	if writeErr := rt.WriteFailure(err); writeErr != nil {
		rt.Logf("diagnostic: failed to write JSON failure: %s", writeErr)
	}
	return 1
}

func isHelpFlag(arg string) bool {
	return arg == "-h" || arg == "--help"
}

func containsHelp(args []string) bool {
	for _, arg := range args {
		if isHelpFlag(arg) {
			return true
		}
	}
	return false
}
