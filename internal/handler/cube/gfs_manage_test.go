package cube

import (
	"reflect"
	"slices"
	"testing"

	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
)

func TestNormalizeGFSManageInitPCSClusterDefaults(t *testing.T) {
	req := GFSManageRequest{Action: "init-pcs-cluster"}
	if err := normalizeGFSManageRequest(&req); err != nil {
		t.Fatal(err)
	}
	if req.ClusterName != gfsManageDefaultCluster {
		t.Errorf("cluster name = %q", req.ClusterName)
	}
	if req.ClusterUser != gfsManageDefaultPCSUser {
		t.Errorf("cluster user = %q", req.ClusterUser)
	}
	if req.ClusterPassword != gfsManageDefaultPCSPass {
		t.Errorf("cluster password default was not applied")
	}
}

func TestBuildGFSManageHostAuthArgs(t *testing.T) {
	got := buildGFSManageHostAuthArgs("10.10.31.1", "hacluster", "secret")
	want := []string{"host", "auth", "10.10.31.1", "-u", "hacluster", "-p", "secret"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestBuildGFSManageClusterSetupArgs(t *testing.T) {
	got := buildGFSManageClusterSetupArgs("cloudcenter_cluster", []string{"10.10.31.1", "10.10.31.2", "10.10.31.3"})
	want := []string{
		"cluster", "setup", "cloudcenter_cluster", "--start",
		"10.10.31.1", "10.10.31.2", "10.10.31.3",
		"quorum", "wait_for_all=1", "last_man_standing=1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestNormalizeGFSManageRejectsPasswordNewline(t *testing.T) {
	req := GFSManageRequest{
		Action:          "init-pcs-cluster",
		ClusterPassword: "secret\nroot:changed",
	}
	if err := normalizeGFSManageRequest(&req); err == nil {
		t.Fatal("expected invalid cluster_password error")
	}
}

func TestNormalizeGFSManageCreateGFSDefaults(t *testing.T) {
	req := GFSManageRequest{Action: "create-gfs", Disks: []string{"/dev/sdb"}}
	if err := normalizeGFSManageRequest(&req); err != nil {
		t.Fatal(err)
	}
	if req.ClusterName != "cloudcenter_cluster" || req.VGName != "vg_glue" || req.LVName != "lv_glue" {
		t.Fatalf("unexpected defaults: %#v", req)
	}
	if req.GFSName != "glue-gfs" || req.MountPoint != "/mnt/glue-gfs" {
		t.Fatalf("unexpected GFS defaults: %#v", req)
	}
}

func TestGFSManageCloneStatusReady(t *testing.T) {
	status := `Cluster name: cloudcenter_cluster
Cluster Summary:
- 2 nodes configured
- 11 resource instances configured

Node List:
- Online: [ 10.10.13.1 10.10.13.2 ]

Full List of Resources:
- Clone Set: glue-locking-clone [glue-locking]:
  Started: [ ablecube1 ablecube2 ]
- Clone Set: glue-gfs_res-clone [glue-gfs_res]:
  Started: [ ablecube1 ablecube2 ]
- Clone Set: glue-gfs-clone [glue-gfs]:
  Started: [ ablecube1 ablecube2 ]`
	if !gfsManageCloneStatusReady(status) {
		t.Fatal("expected GFS clone resources to be ready")
	}
	statusWithUnrelatedFailure := status + `
Failed Resource Actions:
- cloudcenter_res start on ablecube1 returned 'error'
- fence-ablecube1 monitor on ablecube2 returned 'error'`
	if !gfsManageCloneStatusReady(statusWithUnrelatedFailure) {
		t.Fatal("fence and unrelated failed actions must not affect GFS clone readiness")
	}
	lockingOnly := `Clone Set: glue-locking-clone [glue-locking]:
  Started: [ ablecube1 ]`
	if !gfsManageCloneStatusReady(lockingOnly) {
		t.Fatal("started node count must not affect readiness")
	}
	for name, stopped := range map[string]string{
		"locking": `Clone Set: glue-locking-clone [glue-locking]:
  Started: [ ablecube1 ablecube2 ]
	Stopped: [ ablecube3 ]`,
		"lvm": `Clone Set: glue-locking-clone [glue-locking]:
	Started: [ ablecube1 ablecube2 ]
Clone Set: glue-gfs_res-clone [glue-gfs_res]:
	Stopped: [ ablecube2 ]`,
		"filesystem": `Clone Set: glue-locking-clone [glue-locking]:
	Started: [ ablecube1 ablecube2 ]
Clone Set: glue-gfs-clone [glue-gfs]:
	Stopped: [ ablecube2 ]`,
	} {
		t.Run(name, func(t *testing.T) {
			if gfsManageCloneStatusReady(stopped) {
				t.Fatal("a stopped GFS clone resource must not be considered ready")
			}
		})
	}
	if gfsManageCloneStatusReady("Clone Set: glue-gfs-clone [glue-gfs]:\n  Started: [ ablecube1 ]") {
		t.Fatal("glue-locking-clone must exist")
	}
}

func TestBuildGFSManageResourceCommands(t *testing.T) {
	req := GFSManageRequest{GFSName: "glue-gfs", VGName: "vg_glue", LVName: "lv_glue", MountPoint: "/mnt/glue-gfs"}
	commands := buildGFSManageResourceCommands(req, "/dev/vg_glue/lv_glue")
	if len(commands) != 8 {
		t.Fatalf("command count = %d", len(commands))
	}
	if !slices.Contains(commands[4], "fstype=gfs2") || !slices.Contains(commands[4], "directory=/mnt/glue-gfs") {
		t.Fatalf("unexpected filesystem command: %#v", commands[4])
	}
}

func TestAppendGFSManageAncestorPathsUsesActualMultipathPartition(t *testing.T) {
	ancestors := []gfsManageBlockDevice{
		{Name: "sda", Path: "/dev/sda", Type: "disk"},
		{Name: "mpatha", Path: "/dev/mapper/mpatha", Type: "mpath"},
		{Name: "mpatha1", Path: "/dev/mapper/mpatha-part1", Type: "part"},
	}

	paths := appendGFSManageAncestorPaths(gfsManageDevicePaths{}, ancestors, "ablestack-vm")
	if !slices.Equal(paths.Disks, []string{"/dev/mapper/mpatha"}) {
		t.Fatalf("unexpected delete disks: %#v", paths.Disks)
	}
	if !slices.Equal(paths.Partitions, []string{"/dev/mapper/mpatha-part1"}) {
		t.Fatalf("unexpected PV partitions: %#v", paths.Partitions)
	}
}

func TestAppendGFSManageAncestorPathsDoesNotAppendPartitionNumber(t *testing.T) {
	ancestors := []gfsManageBlockDevice{
		{Name: "sdb", Path: "/dev/sdb", Type: "disk"},
		{Name: "mpathb1", Path: "/dev/mapper/mpathb1", Type: "mpath"},
	}

	paths := appendGFSManageAncestorPaths(gfsManageDevicePaths{}, ancestors, "ablestack-vm")
	if !slices.Equal(paths.Partitions, []string{"/dev/mapper/mpathb1"}) {
		t.Fatalf("multipath PV path must remain unchanged: %#v", paths.Partitions)
	}
}

func TestGFSManageJournalCountAddsSpareJournal(t *testing.T) {
	cfg := &CubeModel.ClusterConfigSection{Hosts: []CubeModel.ClusterHost{
		{Ablecube: "10.10.31.1"},
		{Ablecube: "10.10.31.2"},
		{Ablecube: "10.10.31.3"},
	}}
	if got := gfsManageJournalCount(cfg); got != 4 {
		t.Fatalf("journal count = %d, want 4", got)
	}
}

func TestBuildGFSManageMkfsArgsUsesClusterConfigSizes(t *testing.T) {
	cfg := &CubeModel.ClusterConfigSection{
		Hosts: []CubeModel.ClusterHost{{Ablecube: "10.10.31.1"}, {Ablecube: "10.10.31.2"}},
		GFS: CubeModel.ClusterGFSConfig{
			JournalSizeMB:       256,
			ResourceGroupSizeMB: 512,
		},
	}
	req := GFSManageRequest{ClusterName: "cloudcenter_cluster", GFSName: "glue-gfs"}

	args, settings, err := buildGFSManageMkfsArgs(req, cfg, "/dev/vg_glue/lv_glue")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"-j3", "-J", "256", "-r", "512", "-p", "lock_dlm"} {
		if !slices.Contains(args, want) {
			t.Fatalf("mkfs args %#v do not contain %q", args, want)
		}
	}
	if settings.JournalSizeMB != 256 || settings.ResourceGroupSizeMB != 512 {
		t.Fatalf("settings = %#v", settings)
	}
}

func TestResolveGFSManageFormatSettingsUsesDefaults(t *testing.T) {
	settings, err := resolveGFSManageFormatSettings(&CubeModel.ClusterConfigSection{})
	if err != nil {
		t.Fatal(err)
	}
	if settings.JournalSizeMB != 512 || settings.ResourceGroupSizeMB != 1024 {
		t.Fatalf("settings = %#v", settings)
	}
}

func TestResolveGFSManageFormatSettingsRejectsInvalidValues(t *testing.T) {
	tests := []CubeModel.ClusterGFSConfig{
		{JournalSizeMB: 12, ResourceGroupSizeMB: 1024},
		{JournalSizeMB: 512, ResourceGroupSizeMB: 4096},
	}
	for _, gfs := range tests {
		if _, err := resolveGFSManageFormatSettings(&CubeModel.ClusterConfigSection{GFS: gfs}); err == nil {
			t.Fatalf("expected validation error for %#v", gfs)
		}
	}
}

func TestBuildGFSManageStonithCommandUsesReboot(t *testing.T) {
	device := GFSManageStonithDevice{
		IPAddr:   "192.168.0.10",
		IPPort:   "623",
		Login:    "admin",
		Passwd:   "password",
		Host:     "10.10.31.1",
		Hostname: "ablecube1",
	}
	command := buildGFSManageStonithCommand(device, false)
	for _, want := range []string{"pcmk_reboot_action=reboot", "pcmk_off_action=off", "pcmk_host_list=10.10.31.1"} {
		if !slices.Contains(command, want) {
			t.Fatalf("command %#v does not contain %q", command, want)
		}
	}
}

func TestResolveGFSManageStonithDevicesUsesDynamicClusterHosts(t *testing.T) {
	cfg := &CubeModel.ClusterConfigSection{Hosts: []CubeModel.ClusterHost{
		{Hostname: "ablecube1", Ablecube: "10.10.31.1"},
		{Hostname: "ablecube2", Ablecube: "10.10.31.2"},
		{Hostname: "ablecube3", Ablecube: "10.10.31.3"},
		{Hostname: "ablecube4", Ablecube: "10.10.31.4"},
	}}
	devices := make([]GFSManageStonithDevice, len(cfg.Hosts))
	for index := range devices {
		devices[index] = GFSManageStonithDevice{
			IPAddr: "192.168.0." + string(rune('1'+index)),
			IPPort: "623",
			Login:  "admin",
			Passwd: "password",
		}
	}
	resolved, err := resolveGFSManageStonithDevices(devices, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 4 || resolved[3].Host != "10.10.31.4" || resolved[3].Hostname != "ablecube4" {
		t.Fatalf("unexpected devices: %#v", resolved)
	}
}
