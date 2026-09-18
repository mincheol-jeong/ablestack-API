package cube

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
)

func TestGatherSecurityPatchTargetsPreservesTargetKind(t *testing.T) {
	cfg := &CubeModel.ClusterConfigSection{
		CCVM: CubeModel.ClusterCCVMConfig{IP: "10.10.31.10"},
		Hosts: []CubeModel.ClusterHost{
			{Ablecube: "10.10.31.1", Scvm: "100.100.31.11"},
			{Ablecube: "10.10.31.2", Scvm: "100.100.31.12"},
		},
	}

	targets := gatherSecurityPatchTargets(cfg, []string{"all"})
	got := make(map[string]string, len(targets))
	for _, target := range targets {
		got[target.IP] = target.Kind
	}

	want := map[string]string{
		"10.10.31.1":    "ablecube",
		"10.10.31.2":    "ablecube",
		"10.10.31.10":   "ccvm",
		"100.100.31.11": "scvm",
		"100.100.31.12": "scvm",
	}
	if len(got) != len(want) {
		t.Fatalf("target count = %d, want %d: %#v", len(got), len(want), got)
	}
	for ip, kind := range want {
		if got[ip] != kind {
			t.Errorf("target %s kind = %q, want %q", ip, got[ip], kind)
		}
	}
}

func TestRunSecurityPatchRemoteAPIRequestsLocalExecution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != securityPatchAPIPath {
			t.Fatalf("path = %q", r.URL.Path)
		}
		var req SecurityPatchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if !req.Local || len(req.Targets) != 1 || req.Targets[0] != "ccvm" {
			t.Fatalf("remote request must be local ccvm execution: %#v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(SecurityPatchResponse{
			Code: http.StatusOK,
			Val: SecurityPatchValue{Targets: []SecurityPatchTargetResult{{
				IP: "127.0.0.1", TargetKind: "ccvm", ScriptPath: securityPatchVMPath,
				OK: true, RC: 0, IsLocal: true,
			}}},
			Message: "ok",
		})
	}))
	defer server.Close()

	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, portText, _ := strings.Cut(parsed.Host, ":")
	if _, err := strconv.Atoi(portText); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ABLESTACK_API_SCHEME", parsed.Scheme)
	t.Setenv("ABLESTACK_API_PORT", portText)

	result := runSecurityPatchRemoteAPI("127.0.0.1", "ccvm", SecurityPatchRequest{}, "ablestack-vm")
	if !result.OK || result.Transport != "api" || result.IsLocal || result.HTTPStatus != http.StatusOK {
		t.Fatalf("unexpected API result: %#v", result)
	}
	if result.IP != "127.0.0.1" || result.APIURL != server.URL+securityPatchAPIPath {
		t.Fatalf("unexpected target metadata: %#v", result)
	}
}

func TestRunSecurityPatchStatusUpdateLocalUsesClusterJSONWithoutPython(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cluster.json")
	initial := map[string]any{
		"clusterConfig": map[string]any{"type": "ablestack-vm", "hosts": []any{}},
		"systemProfile": map[string]any{
			"security_patch": map[string]any{"status": "false"},
		},
	}
	raw, err := json.Marshal(initial)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ABLESTACK_CLUSTER_JSON", path)

	if err := runSecurityPatchStatusUpdateLocal(); err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(updated, &root); err != nil {
		t.Fatal(err)
	}
	profile, ok := root["systemProfile"].(map[string]any)
	if !ok {
		t.Fatalf("systemProfile missing: %#v", root)
	}
	securityPatch, ok := profile["security_patch"].(map[string]any)
	if !ok || securityPatch["status"] != "true" {
		t.Fatalf("security patch status not updated: %#v", profile)
	}
}

func TestSecurityPatchScriptPathDependsOnTargetKind(t *testing.T) {
	t.Setenv("ABLESTACK_SECURITY_PATCH_SCRIPT", "/test/ablecube/security_patch.sh")

	if got := securityPatchScriptPathForKind("ablecube"); got != "/test/ablecube/security_patch.sh" {
		t.Fatalf("ablecube script = %q", got)
	}
	if got := securityPatchScriptPathForKind("scvm"); got != securityPatchVMPath {
		t.Fatalf("scvm script = %q, want %q", got, securityPatchVMPath)
	}
	if got := securityPatchScriptPathForKind("ccvm"); got != securityPatchVMPath {
		t.Fatalf("ccvm script = %q, want %q", got, securityPatchVMPath)
	}
}

func TestBuildSecurityPatchCommandUsesSelectedScript(t *testing.T) {
	port := 10022
	command := buildSecurityPatchCommandString(
		"/test/security_patch.sh",
		&port,
		true,
		false,
	)

	for _, expected := range []string{"/test/security_patch.sh", "-P", "10022", "--port-change"} {
		if !strings.Contains(command, expected) {
			t.Errorf("command %q does not contain %q", command, expected)
		}
	}
}

func TestRunSecurityPatchReportsSelectedTargetAndScript(t *testing.T) {
	t.Setenv("ABLESTACK_SECURITY_PATCH_SCRIPT", "/test/ablecube/security_patch.sh")
	cfg := &CubeModel.ClusterConfigSection{
		Type: "ablestack-vm",
		Hosts: []CubeModel.ClusterHost{
			{Ablecube: "192.0.2.10"},
		},
	}
	response := runSecurityPatch(SecurityPatchRequest{
		Targets: []string{"ablecube"},
		SSHUser: "root",
		SSHPort: 22,
		DryRun:  true,
	}, cfg)

	value, ok := response.Val.(SecurityPatchValue)
	if !ok {
		t.Fatalf("response val type = %T", response.Val)
	}
	if len(value.Targets) != 1 {
		t.Fatalf("target count = %d, want 1", len(value.Targets))
	}
	result := value.Targets[0]
	if result.TargetKind != "ablecube" {
		t.Errorf("target kind = %q", result.TargetKind)
	}
	if result.ScriptPath != "/test/ablecube/security_patch.sh" {
		t.Errorf("script path = %q", result.ScriptPath)
	}
	if !strings.Contains(result.DryRunCmd, "/test/ablecube/security_patch.sh") {
		t.Errorf("dry-run command = %q", result.DryRunCmd)
	}
}
