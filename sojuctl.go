package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type SojuCtl struct {
	Path    string
	Config  string
	Timeout time.Duration
}

func (s *SojuCtl) Run(parent context.Context, args []string) (string, error) {
	if s.Path == "" {
		return "", errors.New("sojuctl path is empty")
	}
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	argv := make([]string, 0, len(args)+2)
	if s.Config != "" {
		argv = append(argv, "-config", s.Config)
	}
	argv = append(argv, args...)
	command := exec.CommandContext(ctx, s.Path, argv...)
	output, err := command.CombinedOutput()
	text := string(output)
	if ctx.Err() != nil {
		return text, fmt.Errorf("sojuctl timed out after %s", timeout)
	}
	if err != nil {
		return text, fmt.Errorf("sojuctl failed: %w", err)
	}
	return text, nil
}

func formatSojuCtlCommand(config string, args []string) string {
	parts := []string{"sojuctl"}
	if config != "" {
		parts = append(parts, "-config", config)
	}
	parts = append(parts, args...)
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		quoted = append(quoted, quoteDisplayArg(part))
	}
	return strings.Join(quoted, " ")
}

func quoteDisplayArg(value string) string {
	if value != "" && safeDisplayArg.MatchString(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
