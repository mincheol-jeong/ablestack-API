package wallservice

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

const DefaultPythonDir = DefaultRoot + "/python"

type PythonResult struct {
	Code    int    `json:"code"`
	Val     any    `json:"val"`
	Message string `json:"message"`
}

func (m *Manager) RunPython(ctx context.Context, script string, args ...string) (PythonResult, string, error) {
	if filepath.Base(script) != script || !strings.HasSuffix(script, ".py") {
		return PythonResult{}, "", fmt.Errorf("invalid wall python script %q", script)
	}

	commandArgs := append([]string{filepath.Join(DefaultPythonDir, script)}, args...)
	env := []string{"ABLESTACK_CLUSTER_JSON=" + DefaultClusterJSONPath}
	output, commandErr := m.runWithEnv(ctx, env, "python3", commandArgs...)
	result, parseErr := parsePythonResult(output)
	if commandErr != nil {
		return result, output, commandErr
	}
	if parseErr != nil {
		return result, output, fmt.Errorf("parse %s result: %w", script, parseErr)
	}
	if result.Code != 200 {
		return result, output, fmt.Errorf("%s: %s", script, pythonResultMessage(result))
	}
	return result, output, nil
}

func parsePythonResult(output string) (PythonResult, error) {
	output = strings.TrimSpace(output)
	for index := strings.LastIndex(output, "{"); index >= 0; index = strings.LastIndex(output[:index], "{") {
		var result PythonResult
		decoder := json.NewDecoder(strings.NewReader(output[index:]))
		if err := decoder.Decode(&result); err == nil && result.Code != 0 {
			return result, nil
		}
	}
	return PythonResult{}, fmt.Errorf("valid JSON result not found")
}

func pythonResultMessage(result PythonResult) string {
	if value := strings.TrimSpace(result.Message); value != "" {
		return value
	}
	if result.Val != nil {
		if value := strings.TrimSpace(fmt.Sprint(result.Val)); value != "" {
			return value
		}
	}
	return fmt.Sprintf("python result code %d", result.Code)
}
