package mold

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBootstrapJobStepPendingOmitsZeroTimestamps(t *testing.T) {
	raw, err := json.Marshal(BootstrapJobStep{
		Name:   "create_zone",
		Status: BootstrapStepStatusPending,
	})
	if err != nil {
		t.Fatalf("Marshal BootstrapJobStep: %v", err)
	}
	out := string(raw)
	if strings.Contains(out, "started_at") {
		t.Fatalf("pending step JSON contains started_at: %s", out)
	}
	if strings.Contains(out, "finished_at") {
		t.Fatalf("pending step JSON contains finished_at: %s", out)
	}
}
