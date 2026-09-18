package cube

import (
	"reflect"
	"strings"
	"testing"

	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
)

func TestCCVMCloudInitCopyTargetsForVMUsesAblecubeWhenIscsiDisabled(t *testing.T) {
	cfg := ccvmCloudInitTargetTestConfig("ablestack-vm", "false")

	targets, field := ccvmCloudInitCopyTargets(&cfg)

	if field != "hosts[].ablecube" {
		t.Fatalf("field = %q, want hosts[].ablecube", field)
	}
	want := []string{"10.10.31.1", "10.10.31.2"}
	if !reflect.DeepEqual(targets, want) {
		t.Fatalf("targets = %#v, want %#v", targets, want)
	}
}

func TestCCVMCloudInitCopyTargetsForVMUsesAblecubePnOnlyWhenIscsiEnabled(t *testing.T) {
	cfg := ccvmCloudInitTargetTestConfig("ablestack-vm", "true")

	targets, field := ccvmCloudInitCopyTargets(&cfg)

	if field != "hosts[].ablecubePn" {
		t.Fatalf("field = %q, want hosts[].ablecubePn", field)
	}
	want := []string{"100.100.31.1", "100.100.31.2"}
	if !reflect.DeepEqual(targets, want) {
		t.Fatalf("targets = %#v, want %#v", targets, want)
	}
}

func TestCCVMCloudInitCopyTargetsKeepsAblecubePnForHCI(t *testing.T) {
	cfg := ccvmCloudInitTargetTestConfig("ablestack-hci", "false")

	targets, field := ccvmCloudInitCopyTargets(&cfg)

	if field != "hosts[].ablecubePn" {
		t.Fatalf("field = %q, want hosts[].ablecubePn", field)
	}
	want := []string{"100.100.31.1", "100.100.31.2"}
	if !reflect.DeepEqual(targets, want) {
		t.Fatalf("targets = %#v, want %#v", targets, want)
	}
}

func TestAppendGenCloudInitManagementSSHFiles(t *testing.T) {
	userData := map[string]any{"write_files": []map[string]any{}}
	appendGenCloudInitManagementSSHFiles(userData, "ssh-rsa public", "private")

	files, ok := userData["write_files"].([]map[string]any)
	if !ok || len(files) != 2 {
		t.Fatalf("write_files = %#v", userData["write_files"])
	}
	want := []struct {
		path        string
		permissions string
	}{
		{"/var/cloudstack/management/.ssh/id_rsa.pub", "0644"},
		{"/var/cloudstack/management/.ssh/id_rsa", "0600"},
	}
	for i := range want {
		if files[i]["path"] != want[i].path || files[i]["owner"] != "cloud:cloud" || files[i]["permissions"] != want[i].permissions {
			t.Fatalf("write_files[%d] = %#v", i, files[i])
		}
	}
}

func TestAppendGenCloudInitCCVMFilesStartsAPIService(t *testing.T) {
	t.Setenv("ABLESTACK_API_PORT", "18090")
	userData := map[string]any{}
	appendGenCloudInitCCVMRunCommands(userData)

	runCommands, ok := userData["runcmd"].([][]string)
	if !ok || len(runCommands) != 2 {
		t.Fatalf("runcmd = %#v", userData["runcmd"])
	}
	want := []string{"/usr/bin/systemctl", "enable", "--now", "ablestack-api.service"}
	if !reflect.DeepEqual(runCommands[1], want) {
		t.Fatalf("api service command = %#v, want %#v", runCommands[1], want)
	}
	if !strings.Contains(runCommands[0][2], "--add-port=18090/tcp") {
		t.Fatalf("firewall command = %q", runCommands[0][2])
	}
}

func ccvmCloudInitTargetTestConfig(clusterType string, iscsiStorage string) CubeModel.ClusterConfigSection {
	return CubeModel.ClusterConfigSection{
		Type:           clusterType,
		StorageNetwork: iscsiStorage,
		Hosts: []CubeModel.ClusterHost{
			{
				Index:      "1",
				Hostname:   "ablecube31-1",
				Ablecube:   "10.10.31.1",
				AblecubePn: "100.100.31.1",
			},
			{
				Index:      "2",
				Hostname:   "ablecube31-2",
				Ablecube:   "10.10.31.2",
				AblecubePn: "100.100.31.2",
			},
		},
	}
}
