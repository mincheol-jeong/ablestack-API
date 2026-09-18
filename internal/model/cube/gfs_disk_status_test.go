package cube

import "testing"

func TestParseGFSDiskUsageDF(t *testing.T) {
	raw := []byte(`Filesystem               Size  Used Avail Use% Mounted on
/dev/mapper/vg_glue-lv_glue  800G   57G  744G   8% /mnt/glue-gfs
`)

	usageByMountpoint := ParseGFSDiskUsageDF(raw)
	usage, ok := usageByMountpoint["/mnt/glue-gfs"]
	if !ok {
		t.Fatalf("usage for /mnt/glue-gfs not found")
	}
	if usage.Filesystem != "/dev/mapper/vg_glue-lv_glue" {
		t.Fatalf("Filesystem = %q", usage.Filesystem)
	}
	if usage.Size != "800G" || usage.Used != "57G" || usage.Avail != "744G" || usage.UsePercent != "8%" {
		t.Fatalf("unexpected usage: %#v", usage)
	}
}

func TestBuildGFSDiskStatusAppliesDFUsage(t *testing.T) {
	size := "800G"
	mountpoint := "/mnt/glue-gfs"
	devices := []GFSBlockDevice{
		{
			Name:  "sdb",
			Kname: "sdb",
			Path:  strPtr("/dev/sdb"),
			Size:  strPtr(size),
			Type:  strPtr("disk"),
			Children: []GFSBlockDevice{
				{
					Name:       "vg_glue-lv_glue",
					Kname:      "dm-0",
					Path:       strPtr("/dev/mapper/vg_glue-lv_glue"),
					Size:       strPtr(size),
					Type:       strPtr("lvm"),
					Mountpoint: strPtr(mountpoint),
				},
			},
		},
	}
	mounts := []GFSMount{
		{Device: "/dev/mapper/vg_glue-lv_glue", Mountpoint: mountpoint},
	}
	usageByMountpoint := map[string]GFSDiskUsage{
		mountpoint: {
			Filesystem: "/dev/mapper/vg_glue-lv_glue",
			Mountpoint: mountpoint,
			Size:       "800G",
			Used:       "57G",
			Avail:      "744G",
			UsePercent: "8%",
		},
	}

	status := BuildGFSDiskStatus(devices, mounts, "ablestack-vm", false, nil, usageByMountpoint)
	if len(status.Blockdevices) != 1 {
		t.Fatalf("Blockdevices length = %d", len(status.Blockdevices))
	}
	got := status.Blockdevices[0]
	if got.Size != "800G" || got.Used != "57G" || got.Avail != "744G" || got.UsePercent != "8%" {
		t.Fatalf("unexpected disk usage: %#v", got)
	}
}

func TestBuildGFSDiskStatusDefaultsMissingDFUsage(t *testing.T) {
	mountpoint := "/mnt/glue-gfs"
	devices := []GFSBlockDevice{
		{
			Name:       "vg_glue-lv_glue",
			Kname:      "dm-0",
			Path:       strPtr("/dev/mapper/vg_glue-lv_glue"),
			Size:       strPtr("800G"),
			Type:       strPtr("lvm"),
			Mountpoint: strPtr(mountpoint),
		},
	}
	mounts := []GFSMount{
		{Device: "/dev/mapper/vg_glue-lv_glue", Mountpoint: mountpoint},
	}

	status := BuildGFSDiskStatus(devices, mounts, "ablestack-vm", false, nil, nil)
	if len(status.Blockdevices) != 1 {
		t.Fatalf("Blockdevices length = %d", len(status.Blockdevices))
	}
	got := status.Blockdevices[0]
	if got.Used != "N/A" || got.Avail != "N/A" || got.UsePercent != "N/A" {
		t.Fatalf("unexpected default usage: %#v", got)
	}
}

func TestGroupGFSDiskCandidatesReturnsSingleUUIDPerMount(t *testing.T) {
	candidates := []gfsDiskCandidate{
		{entry: GFSDiskDevice{
			LVM:        "/dev/mapper/vg_glue-lv_glue",
			Mountpoint: "/mnt/glue-gfs",
			DiskID:     []string{"/dev/disk/by-id/dm-uuid-mpath-3600b"},
		}},
		{entry: GFSDiskDevice{
			LVM:        "/dev/mapper/vg_glue-lv_glue",
			Mountpoint: "/mnt/glue-gfs",
			DiskID:     []string{"/dev/disk/by-id/dm-uuid-mpath-3600a"},
		}},
	}

	grouped := groupGFSDiskCandidates(candidates, true, nil)
	if len(grouped) != 1 {
		t.Fatalf("grouped mounts length = %d", len(grouped))
	}
	if len(grouped[0].DiskID) != 1 {
		t.Fatalf("disk_id length = %d, disk_id = %#v", len(grouped[0].DiskID), grouped[0].DiskID)
	}
	if grouped[0].DiskID[0] != "/dev/disk/by-id/dm-uuid-mpath-3600a" {
		t.Fatalf("unexpected representative UUID: %q", grouped[0].DiskID[0])
	}
}

func TestBuildGFSDiskStatusExposesOnlyMultipathDeviceInMultipathMode(t *testing.T) {
	mountpoint := "/mnt/glue-gfs"
	diskType := "disk"
	mpathType := "mpath"
	lvmType := "lvm"
	dmUUID := "mpath-3600a"
	devices := []GFSBlockDevice{{
		Name: "sdb", Kname: "sdb", Path: strPtr("/dev/sdb"), Type: &diskType,
		Children: []GFSBlockDevice{{
			Name: "mpathb", Kname: "dm-1", Path: strPtr("/dev/mapper/mpathb"), Type: &mpathType, DmUUID: &dmUUID,
			Children: []GFSBlockDevice{{
				Name: "vg_glue-lv_glue", Kname: "dm-2", Path: strPtr("/dev/mapper/vg_glue-lv_glue"), Type: &lvmType, Mountpoint: strPtr(mountpoint),
			}},
		}},
	}}

	status := BuildGFSDiskStatus(
		devices,
		[]GFSMount{{Device: "/dev/mapper/vg_glue-lv_glue", Mountpoint: mountpoint}},
		"ablestack-vm",
		true,
		map[string][]string{"dm-1": {"/dev/disk/by-id/dm-uuid-mpath-3600a"}},
		nil,
	)
	if len(status.Blockdevices) != 1 {
		t.Fatalf("Blockdevices length = %d", len(status.Blockdevices))
	}
	got := status.Blockdevices[0]
	if len(got.Multipaths) != 1 || got.Multipaths[0] != "/dev/mapper/mpathb" {
		t.Fatalf("unexpected multipath devices: %#v", got.Multipaths)
	}
	if status.Mode != "multi" {
		t.Fatalf("unexpected mode: %q", status.Mode)
	}
	if len(got.Devices) != 0 {
		t.Fatalf("physical paths must be hidden in multipath mode: %#v", got.Devices)
	}
}

func TestBuildGFSDiskStatusExposesOnlyPhysicalDeviceInSingleMode(t *testing.T) {
	mountpoint := "/mnt/glue-gfs"
	diskType := "disk"
	lvmType := "lvm"
	devices := []GFSBlockDevice{{
		Name: "sdb", Kname: "sdb", Path: strPtr("/dev/sdb"), Type: &diskType,
		Children: []GFSBlockDevice{{
			Name: "vg_glue-lv_glue", Kname: "dm-2", Path: strPtr("/dev/mapper/vg_glue-lv_glue"), Type: &lvmType, Mountpoint: strPtr(mountpoint),
		}},
	}}

	status := BuildGFSDiskStatus(
		devices,
		[]GFSMount{{Device: "/dev/mapper/vg_glue-lv_glue", Mountpoint: mountpoint}},
		"ablestack-vm",
		true,
		nil,
		nil,
	)
	if len(status.Blockdevices) != 1 {
		t.Fatalf("Blockdevices length = %d", len(status.Blockdevices))
	}
	got := status.Blockdevices[0]
	if len(got.Devices) != 1 || got.Devices[0] != "/dev/sdb" {
		t.Fatalf("unexpected single-path devices: %#v", got.Devices)
	}
	if status.Mode != "single" {
		t.Fatalf("single-path topology must not be classified as multipath: %q", status.Mode)
	}
	if len(got.Multipaths) != 0 {
		t.Fatalf("multipath devices must be hidden in single mode: %#v", got.Multipaths)
	}
}

func strPtr(value string) *string {
	return &value
}
