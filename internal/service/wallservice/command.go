package wallservice

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type CommandRunner interface {
	Run(ctx context.Context, env []string, command string, args ...string) (string, error)
}

type OSCommandRunner struct{}

func (OSCommandRunner) Run(ctx context.Context, env []string, command string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	out, err := cmd.CombinedOutput()
	value := strings.TrimSpace(string(out))
	if ctx.Err() != nil {
		return value, ctx.Err()
	}
	if err != nil {
		return value, fmt.Errorf("%s: %w", firstNonEmpty(value, command+" failed"), err)
	}
	return value, nil
}

func (m *Manager) run(ctx context.Context, command string, args ...string) (string, error) {
	return m.runWithEnv(ctx, nil, command, args...)
}

func (m *Manager) runWithEnv(ctx context.Context, env []string, command string, args ...string) (string, error) {
	if m == nil || m.Runner == nil {
		return "", fmt.Errorf("wall command runner is not configured")
	}
	return m.Runner.Run(ctx, env, command, args...)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
