package builtin

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"
)

type commandResult struct {
	Stdout   string
	Stderr   string
	Combined string
	ExitCode int
	Duration time.Duration
	TimedOut bool
}

func runCommandWithInput(ctx context.Context, dir string, name string, args []string, stdin string, timeoutMs int) (commandResult, error) {
	timeout := 120 * time.Second
	if timeoutMs > 0 {
		timeout = time.Duration(timeoutMs) * time.Millisecond
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	result := commandResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
		Duration: time.Since(start),
	}

	if result.Stderr != "" {
		result.Combined = result.Stdout
		if result.Combined != "" {
			result.Combined += "\n"
		}
		result.Combined += result.Stderr
	} else {
		result.Combined = result.Stdout
	}

	if cmdCtx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		result.ExitCode = -1
		return result, err
	}

	if err == nil {
		return result, nil
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
		return result, err
	}

	result.ExitCode = -1
	return result, err
}
