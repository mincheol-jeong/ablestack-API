package wallservice

import "testing"

func TestEnabledServiceState(t *testing.T) {
	for _, state := range []string{"enabled", "static", "indirect", "generated"} {
		if !enabledServiceState(state) {
			t.Fatalf("state %q should be enabled", state)
		}
	}
	if enabledServiceState("disabled") {
		t.Fatal("disabled service must not be enabled")
	}
}
