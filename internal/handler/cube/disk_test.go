package cube

import "testing"

func TestMarkPartitionedMultipathDevicesInUse(t *testing.T) {
	mpathType := "mpath"
	partType := "part"
	diskType := "disk"
	devices := []DiskDevice{
		{Name: "mpatha", Type: &mpathType, Children: []DiskDevice{{Name: "mpatha1", Type: &partType}}},
		{Name: "mpathb", Type: &mpathType},
		{Name: "sda", Type: &diskType, Children: []DiskDevice{{Name: "sda1", Type: &partType}}},
	}

	markPartitionedMultipathDevicesInUse(devices)

	if !devices[0].InUse || devices[0].InUseReason != "partition exists" {
		t.Fatalf("partitioned multipath disk was not marked in use: %#v", devices[0])
	}
	if devices[1].InUse {
		t.Fatal("empty multipath disk must remain selectable")
	}
	if devices[2].InUse {
		t.Fatal("non-multipath disk must not be marked by the multipath policy")
	}
}

func TestHasNestedPartitionDescendant(t *testing.T) {
	mpathType := "mpath"
	lvmType := "lvm"
	partType := "part"
	device := DiskDevice{
		Name: "mpatha",
		Type: &mpathType,
		Children: []DiskDevice{{
			Name:     "vg-lv",
			Type:     &lvmType,
			Children: []DiskDevice{{Name: "mpatha1", Type: &partType}},
		}},
	}
	if !hasPartitionDescendant(device) {
		t.Fatal("nested partition must be detected")
	}
}

func TestAnnotateDiskIdentities(t *testing.T) {
	mpathType := "mpath"
	diskType := "disk"
	dmUUID := "mpath-3600140510763a8bd9247d4f6ad8460de"
	wwn := "0x5000c50012345678"
	devices := []DiskDevice{
		{Name: "mpathh", Type: &mpathType, DmUUID: &dmUUID},
		{Name: "sdr", Type: &diskType, Wwn: &wwn},
	}

	annotateDiskIdentities(devices)

	if devices[0].UUID != "3600140510763a8bd9247d4f6ad8460de" || devices[0].PathMode != "multipath" {
		t.Fatalf("unexpected multipath identity: %#v", devices[0])
	}
	if devices[1].UUID != wwn || devices[1].PathMode != "single" {
		t.Fatalf("unexpected single-path identity: %#v", devices[1])
	}
}

func TestStableDiskIDPriority(t *testing.T) {
	if stableDiskIDPriority("dm-uuid-mpath-36001405") <= stableDiskIDPriority("wwn-0x5000") {
		t.Fatal("multipath DM UUID must be preferred")
	}
	if stableDiskIDPriority("wwn-0x5000-part1") != 0 {
		t.Fatal("partition by-id must not be selected as a base disk")
	}
	if stableDiskIDPriority("scsi-36001405") == 0 {
		t.Fatal("single-path SCSI ID must be accepted")
	}
}

func TestParseMultipathLL(t *testing.T) {
	output := `mpatha (3600140510763a8bd9247d4f6ad8460de) dm-1 LIO-ORG,TCMU device
size=400G features='1 queue_if_no_path' hwhandler='0' wp=rw
|- 2:0:0:1 sdb 8:16 active ready running
mpathb (3600140510763a8bd9247d4f6ad8460df) dm-2 LIO-ORG,TCMU device
size=600G features='1 queue_if_no_path' hwhandler='0' wp=rw
|- 3:0:0:1 sdc 8:32 active ready running
`

	maps := parseMultipathLL(output)
	if len(maps) != 2 {
		t.Fatalf("multipath map length = %d, maps = %#v", len(maps), maps)
	}
	if maps[0].Name != "mpatha" || maps[0].UUID != "3600140510763a8bd9247d4f6ad8460de" || maps[0].Size != "400G" {
		t.Fatalf("unexpected first multipath map: %#v", maps[0])
	}
	if maps[1].Name != "mpathb" || maps[1].UUID != "3600140510763a8bd9247d4f6ad8460df" || maps[1].Size != "600G" {
		t.Fatalf("unexpected second multipath map: %#v", maps[1])
	}
}

func TestApplyDiskActionDetailUsesAllMultipathLLMaps(t *testing.T) {
	mpathType := "mpath"
	diskType := "disk"
	firstUUID := "mpath-3600a"
	secondUUID := "mpath-3600b"
	d := &TypeBlockDevice{Blockdevices: []DiskDevice{
		{Name: "mpatha", Kname: "dm-1", Path: stringPointer("/dev/mapper/mpatha"), Type: &mpathType, DmUUID: &firstUUID, Size: stringPointer("400G")},
		{Name: "mpathb", Kname: "dm-2", Path: stringPointer("/dev/mapper/mpathb"), Type: &mpathType, DmUUID: &secondUUID, Size: stringPointer("600G")},
		{Name: "sdf", Kname: "sdf", Path: stringPointer("/dev/sdf"), Type: &diskType, Size: stringPointer("1T")},
	}}
	maps := []multipathLLMap{
		{Name: "mpatha", UUID: "3600a", Size: "400G"},
		{Name: "mpathb", UUID: "3600b", Size: "600G"},
	}

	applyDiskAction(d, "detail", maps, true, true)
	if len(d.Blockdevices) != 2 {
		t.Fatalf("detail devices length = %d, devices = %#v", len(d.Blockdevices), d.Blockdevices)
	}
	for index, name := range []string{"mpatha", "mpathb"} {
		device := d.Blockdevices[index]
		if device.Name != name || device.UUID != maps[index].UUID || device.PathMode != "multipath" {
			t.Fatalf("unexpected multipath detail device: %#v", device)
		}
	}
}

func TestApplyDiskActionDetailSingleModeExcludesOSDisk(t *testing.T) {
	diskType := "disk"
	partType := "part"
	rootMount := "/"
	d := &TypeBlockDevice{Blockdevices: []DiskDevice{
		{
			Name: "sda", Kname: "sda", Path: stringPointer("/dev/sda"), Type: &diskType,
			Children: []DiskDevice{{Name: "sda1", Kname: "sda1", Type: &partType, Mountpoint: &rootMount}},
		},
		{Name: "sdb", Kname: "sdb", Path: stringPointer("/dev/sdb"), Type: &diskType, Size: stringPointer("1T")},
		{Name: "sdc", Kname: "sdc", Path: stringPointer("/dev/sdc"), Type: &diskType, Size: stringPointer("2T")},
	}}

	applyDiskAction(d, "detail", nil, true, false)
	if len(d.Blockdevices) != 2 {
		t.Fatalf("single detail devices length = %d, devices = %#v", len(d.Blockdevices), d.Blockdevices)
	}
	if d.Blockdevices[0].Name != "sdb" || d.Blockdevices[1].Name != "sdc" {
		t.Fatalf("unexpected single detail devices: %#v", d.Blockdevices)
	}
}

func TestFilterOSMultipathLLMaps(t *testing.T) {
	diskType := "disk"
	mpathType := "mpath"
	lvmType := "lvm"
	rootMount := "/"
	osUUID := "mpath-3600os"
	devices := []DiskDevice{{
		Name: "sda", Kname: "sda", Type: &diskType,
		Children: []DiskDevice{{
			Name: "mpatha", Kname: "dm-1", Type: &mpathType, DmUUID: &osUUID,
			Children: []DiskDevice{{Name: "root-lv", Kname: "dm-2", Type: &lvmType, Mountpoint: &rootMount}},
		}},
	}}
	maps := []multipathLLMap{
		{Name: "mpatha", UUID: "3600os", Size: "100G"},
		{Name: "mpathb", UUID: "3600data", Size: "1T"},
	}

	filtered := filterOSMultipathLLMaps(maps, devices)
	if len(filtered) != 1 || filtered[0].Name != "mpathb" {
		t.Fatalf("unexpected non-OS multipath maps: %#v", filtered)
	}
}

func stringPointer(value string) *string {
	return &value
}
