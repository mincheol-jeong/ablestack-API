package cube

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeSecurityEvidenceRequestDefaults(t *testing.T) {
	req := SecurityEvidenceRequest{}
	if err := normalizeSecurityEvidenceRequest(&req); err != nil {
		t.Fatal(err)
	}
	if len(req.Targets) != 1 || req.Targets[0] != "all" || req.Items != "all" || req.SSHUser != "root" || req.Timeout != 120 || req.MaxOutputLines != 400 {
		t.Fatalf("unexpected defaults: %#v", req)
	}
}

func TestNormalizeSecurityEvidenceRequestRejectsInvalidItems(t *testing.T) {
	req := SecurityEvidenceRequest{Items: "all;rm -rf /"}
	if err := normalizeSecurityEvidenceRequest(&req); err == nil {
		t.Fatal("expected invalid items error")
	}
}

func TestParseSecurityEvidenceHelperOutput(t *testing.T) {
	value := SecurityEvidenceMetadata{Filename: "evidence.zip", Size: 42, Hosts: 3}
	rawValue, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	output := `{"code":200,"val":` + string(rawValue) + `}`
	code, metadata, message, err := parseSecurityEvidenceHelperOutput(output)
	if err != nil {
		t.Fatal(err)
	}
	if code != 200 || message != "" || metadata == nil || metadata.Filename != "evidence.zip" || metadata.Hosts != 3 {
		t.Fatalf("unexpected response: code=%d metadata=%#v message=%q", code, metadata, message)
	}
}

func TestValidateSecurityEvidenceDownloadRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ABLESTACK_SECURITY_EVIDENCE_ROOT", root)
	outside := filepath.Join(t.TempDir(), "evidence.zip")
	if err := os.WriteFile(outside, []byte("zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateSecurityEvidenceDownload(&SecurityEvidenceMetadata{Path: outside}); err == nil {
		t.Fatal("expected outside-root error")
	}
}

func TestValidateSecurityEvidenceDownloadAcceptsRootFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ABLESTACK_SECURITY_EVIDENCE_ROOT", root)
	path := filepath.Join(root, "evidence.zip")
	if err := os.WriteFile(path, []byte("zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := validateSecurityEvidenceDownload(&SecurityEvidenceMetadata{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("path = %q, want %q", got, path)
	}
}
