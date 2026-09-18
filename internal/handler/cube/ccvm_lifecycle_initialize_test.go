package cube

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeCCVMLifecycleRequestAcceptsInitialize(t *testing.T) {
	req := CCVMLifecycleRequest{Action: " INITIALIZE "}
	if err := normalizeCCVMLifecycleRequest(&req); err != nil {
		t.Fatal(err)
	}
	if req.Action != "initialize" {
		t.Fatalf("action = %q", req.Action)
	}
}

func TestRemoveCCVMInitializeFilesRemovesOnlyCCVMPrefix(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{"ccvm.xml", "ccvm.qcow2", "ccvm-data", "keep.qcow2"} {
		path := filepath.Join(directory, name)
		if name == "ccvm-data" {
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(path, []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	removed, err := removeCCVMInitializeFiles(directory)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 3 {
		t.Fatalf("removed = %d, want 3", removed)
	}
	if _, err := os.Stat(filepath.Join(directory, "keep.qcow2")); err != nil {
		t.Fatalf("unrelated file removed: %v", err)
	}
	for _, name := range []string{"ccvm.xml", "ccvm.qcow2", "ccvm-data"} {
		if _, err := os.Stat(filepath.Join(directory, name)); !os.IsNotExist(err) {
			t.Fatalf("%s was not removed", name)
		}
	}
}
