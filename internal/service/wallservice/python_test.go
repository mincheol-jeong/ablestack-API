package wallservice

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type pythonRunner struct {
	output  string
	err     error
	command string
	env     []string
}

func (r *pythonRunner) Run(_ context.Context, env []string, command string, args ...string) (string, error) {
	r.command = strings.Join(append([]string{command}, args...), " ")
	r.env = append([]string(nil), env...)
	return r.output, r.err
}

func TestRunPythonAcceptsPrettyJSONResult(t *testing.T) {
	runner := &pythonRunner{output: "warning before result\n{\n  \"code\": 200,\n  \"val\": \"success wall configuration\"\n}\ntrailing log"}
	manager := New()
	manager.Runner = runner

	result, _, err := manager.RunPython(context.Background(), "config_wall.py", "config", "--ccvm", "10.10.31.10")
	if err != nil {
		t.Fatal(err)
	}
	if result.Code != 200 || result.Val != "success wall configuration" {
		t.Fatalf("result = %+v", result)
	}
	if !strings.Contains(runner.command, DefaultPythonDir+"/config_wall.py config --ccvm 10.10.31.10") {
		t.Fatalf("command = %q", runner.command)
	}
	if !containsString(runner.env, "ABLESTACK_CLUSTER_JSON="+DefaultClusterJSONPath) {
		t.Fatalf("env = %#v", runner.env)
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func TestRunPythonRejectsFailedJSONResult(t *testing.T) {
	runner := &pythonRunner{output: `{"code": 500, "val": "prometheus failed"}`}
	manager := New()
	manager.Runner = runner

	_, _, err := manager.RunPython(context.Background(), "config_wall.py", "config")
	if err == nil || !strings.Contains(err.Error(), "prometheus failed") {
		t.Fatalf("RunPython() error = %v", err)
	}
}

func TestRunPythonPreservesCommandFailure(t *testing.T) {
	runner := &pythonRunner{output: "python3: script not found", err: errors.New("exit status 2")}
	manager := New()
	manager.Runner = runner

	_, output, err := manager.RunPython(context.Background(), "config_wall.py", "config")
	if err == nil || output != runner.output {
		t.Fatalf("RunPython() output=%q error=%v", output, err)
	}
}
