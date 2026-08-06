package main

import (
	"context"
	"errors"

	"github.com/lmilojevicc/seshagy/internal/cli"
	"github.com/lmilojevicc/seshagy/internal/completion"
)

func runEarlyCompletion(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	if args[0] == "__complete" {
		if len(args) < 4 || args[1] != "--" {
			return true, nil
		}
		result := completion.Resolve(
			context.Background(),
			args[2:],
			completion.NewRuntimeProvider(),
		)
		if output := completion.Encode(result); output != "" {
			cli.Print(output)
		}
		return true, nil
	}
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg != "--ephemeral" {
			filtered = append(filtered, arg)
		}
	}
	if len(filtered) == 0 || filtered[0] != "completion" {
		return false, nil
	}
	if len(filtered) != 2 {
		return true, errors.New(joinUsage("completion", "bash|zsh|fish"))
	}
	script, err := completion.Script(completion.Shell(filtered[1]))
	if err != nil {
		return true, errors.New(joinUsage("completion", "bash|zsh|fish"))
	}
	cli.Print(script)
	return true, nil
}
