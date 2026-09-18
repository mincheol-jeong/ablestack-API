package cube

import (
	"reflect"
	"strings"
	"testing"

	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
)

func TestSelectedDeployRunStepsInsertsRBDPrepareBeforeStorage(t *testing.T) {
	req := DeployRunRequest{
		Only: []string{
			CubeModel.DeployRunStepSCVMBootstrap,
			CubeModel.DeployRunStepStoragePrepare,
			CubeModel.DeployRunStepCCVMPrepare,
		},
		RBD: &RBDManageRequest{Action: "create", Size: 5000},
	}

	got := selectedDeployRunSteps(req)
	want := []string{
		CubeModel.DeployRunStepSCVMBootstrap,
		CubeModel.DeployRunStepRBDPrepare,
		CubeModel.DeployRunStepStoragePrepare,
		CubeModel.DeployRunStepCCVMPrepare,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("steps = %#v, want %#v", got, want)
	}
}

func TestBuildDeployRunGFSRequestsForVM(t *testing.T) {
	base := GFSManageRequest{
		Disks: []string{"/dev/disk/by-id/dm-uuid-mpath-test"},
		Stonith: []GFSManageStonithDevice{{
			IPAddr: "192.168.0.10", Login: "admin", Passwd: "password",
		}},
	}

	requests, err := buildDeployRunGFSRequests(base, "ablestack-vm", nil)
	if err != nil {
		t.Fatal(err)
	}
	wantActions := []string{"init-pcs-cluster", "configure-stonith", "create-gfs", "set-alert"}
	if len(requests) != len(wantActions) {
		t.Fatalf("request count = %d, want %d", len(requests), len(wantActions))
	}
	for index, action := range wantActions {
		if requests[index].Action != action {
			t.Errorf("request[%d].Action = %q, want %q", index, requests[index].Action, action)
		}
	}
	if len(requests[0].VolumeGroups) != 1 || requests[0].VolumeGroups[0].VGName != "vg_glue" {
		t.Fatalf("default volume group not applied: %#v", requests[0].VolumeGroups)
	}
}

func TestBuildDeployRunGFSRequestsForHCIFilesystemUsesRBDPaths(t *testing.T) {
	base := GFSManageRequest{
		Stonith: []GFSManageStonithDevice{{
			IPAddr: "192.168.0.10", Login: "admin", Passwd: "password",
		}},
	}

	requests, err := buildDeployRunGFSRequests(base, "ablestack-hci-filesystem", []string{"rbd/gfs01", "rbd/gfs02"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/dev/rbd/rbd/gfs01", "/dev/rbd/rbd/gfs02"}
	if !reflect.DeepEqual(requests[0].Disks, want) || !reflect.DeepEqual(requests[2].Disks, want) {
		t.Fatalf("RBD disks = %#v / %#v, want %#v", requests[0].Disks, requests[2].Disks, want)
	}
}

func TestBuildDeployRunGFSRequestsRequiresStonith(t *testing.T) {
	_, err := buildDeployRunGFSRequests(
		GFSManageRequest{Disks: []string{"/dev/sdb"}},
		"ablestack-vm",
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "stonith") {
		t.Fatalf("error = %v, want stonith validation error", err)
	}
}

func TestValidateDeployRunSelectionRejectsUnknownValues(t *testing.T) {
	for _, req := range []DeployRunRequest{
		{Mode: "everything"},
		{Only: []string{"cluster_apply", "typo"}},
		{Skip: []string{"unknown"}},
	} {
		if err := validateDeployRunSelection(req); err == nil {
			t.Fatalf("expected validation error for %#v", req)
		}
	}
}

func TestDeployRunJobStoreRejectsConcurrentActiveJob(t *testing.T) {
	store := newDeployRunJobStore()
	steps := []string{CubeModel.DeployRunStepClusterApply}
	first, ok := store.createIfIdle(DeployRunRequest{}, steps)
	if !ok || first.JobID == "" {
		t.Fatalf("first job was not created: %#v, ok=%v", first, ok)
	}
	if _, ok := store.createIfIdle(DeployRunRequest{}, steps); ok {
		t.Fatal("second active job must be rejected")
	}
	store.update(first.JobID, func(job *CubeModel.DeployRunJob) {
		job.Status = CubeModel.DeployRunStatusSucceeded
	})
	if _, ok := store.createIfIdle(DeployRunRequest{}, steps); !ok {
		t.Fatal("new job must be accepted after prior job finishes")
	}
}

func TestDeployRunLicenseConfigUsesRequestedHostsBeforeClusterApply(t *testing.T) {
	current := &CubeModel.ClusterConfigSection{
		Type:  "ablestack-vm",
		Hosts: []CubeModel.ClusterHost{{Hostname: "old", Ablecube: "10.0.0.1"}},
	}
	req := DeployRunRequest{Cluster: &ClusterConfigApplyRequest{
		Type:  "ablestack-hci",
		Hosts: []CubeModel.ClusterHost{{Hostname: "new", Ablecube: "10.0.0.2"}},
	}}

	got := deployRunLicenseConfig(req, current)
	if got.Type != "ablestack-hci" || len(got.Hosts) != 1 || got.Hosts[0].Hostname != "new" {
		t.Fatalf("license config = %#v", got)
	}
	if current.Hosts[0].Hostname != "old" {
		t.Fatal("current cluster config was mutated")
	}
}

func TestSelectedDeployRunStepsDoesNotInsertRBDWithoutPayload(t *testing.T) {
	req := DeployRunRequest{
		Only: []string{CubeModel.DeployRunStepStoragePrepare},
	}

	got := selectedDeployRunSteps(req)
	want := []string{CubeModel.DeployRunStepStoragePrepare}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("steps = %#v, want %#v", got, want)
	}
}

func TestSelectedDeployRunStepsHonorsRBDPrepareSkip(t *testing.T) {
	req := DeployRunRequest{
		Only: []string{CubeModel.DeployRunStepStoragePrepare},
		Skip: []string{CubeModel.DeployRunStepRBDPrepare},
		RBD:  &RBDManageRequest{Action: "create", Size: 5000},
	}

	got := selectedDeployRunSteps(req)
	want := []string{CubeModel.DeployRunStepStoragePrepare}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("steps = %#v, want %#v", got, want)
	}
}

func TestNormalizeDeployRunRBDStepAliases(t *testing.T) {
	for _, alias := range []string{"rbd", "rbd-create", "rbd_map", "rbd prepare"} {
		if got := normalizeDeployRunStepName(alias); got != CubeModel.DeployRunStepRBDPrepare {
			t.Errorf("normalize %q = %q", alias, got)
		}
	}
}
