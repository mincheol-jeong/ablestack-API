package cube

import "testing"

func TestIsCCVMPCSResourceStartedOnAnyNode(t *testing.T) {
	resource := ccvmSnapPCSResource{
		Role:           "Started",
		NodesRunningOn: "1",
	}
	resource.Node.Name = "ablecube13-2"

	if !isCCVMPCSResourceStarted(resource) {
		t.Fatalf("resource should be treated as started: %#v", resource)
	}
}

func TestIsCCVMPCSResourceStartedRejectsMissingNode(t *testing.T) {
	resource := ccvmSnapPCSResource{
		Role:           "Started",
		NodesRunningOn: "1",
	}

	if isCCVMPCSResourceStarted(resource) {
		t.Fatalf("resource without a started node must not be treated as started: %#v", resource)
	}
}
