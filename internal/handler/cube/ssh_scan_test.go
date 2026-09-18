package cube

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
)

const testKnownHostsKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAICn4RuFiIYqs/iDKK8jQzMPh9E7lBzf3Pj2QAZsH1fXW"

func TestSanitizeKnownHostsRejectsCommandErrors(t *testing.T) {
	input := strings.Join([]string{
		"# ccvm:22 SSH-2.0-OpenSSH_8.7",
		"ccvm " + testKnownHostsKey,
		"getaddrinfo scvm1: Name or service not known",
		"ccvm ssh-ed25519 invalid-base64",
	}, "\n")

	got, removed := sanitizeKnownHosts([]byte(input), false)
	if removed != 2 {
		t.Fatalf("removed = %d, want 2", removed)
	}
	if string(got) != "ccvm "+testKnownHostsKey+"\n" {
		t.Fatalf("unexpected sanitized output: %q", got)
	}
}

func TestSanitizeKnownHostsPreservesValidExistingLines(t *testing.T) {
	input := strings.Join([]string{
		"# retained comment",
		"ccvm,10.10.13.10 " + testKnownHostsKey + " ccvm-key",
		"invalid line",
	}, "\n")

	got, removed := sanitizeKnownHosts([]byte(input), true)
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	expect := "# retained comment\nccvm,10.10.13.10 " + testKnownHostsKey + " ccvm-key\n"
	if string(got) != expect {
		t.Fatalf("unexpected sanitized output: %q", got)
	}
}

func TestRepairKnownHostsFileRemovesInvalidLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	content := "broken data\nccvm " + testKnownHostsKey + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	removed, err := repairKnownHostsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "ccvm "+testKnownHostsKey+"\n" {
		t.Fatalf("unexpected repaired file: %q", raw)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("known_hosts mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestCCVMKnownHostsTargetsIncludesAliasesAndIP(t *testing.T) {
	cfg := &CubeModel.ClusterConfigSection{}
	cfg.CCVM.IP = "10.10.13.10"

	got := ccvmKnownHostsTargets(cfg)
	want := []string{"ccvm", "ccvm-mngt", "10.10.13.10"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %#v, want %#v", got, want)
	}
}

func TestShouldRefreshCCVMKnownHosts(t *testing.T) {
	for _, action := range []string{"setup", "reset", "start", "restart"} {
		if !shouldRefreshCCVMKnownHosts(action) {
			t.Fatalf("action %q should refresh known_hosts", action)
		}
	}
	for _, action := range []string{"stop", "delete", "copy"} {
		if shouldRefreshCCVMKnownHosts(action) {
			t.Fatalf("action %q should not refresh known_hosts", action)
		}
	}
}
