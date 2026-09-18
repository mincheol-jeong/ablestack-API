package cube

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// GFSDiskStatusResponse는 GFS2 마운트 디스크 상태 조회 결과이다.
// @name GFSDiskStatusResponse
type GFSDiskStatusResponse struct {
	Code int `json:"code" example:"200"`
	Val  any `json:"val,omitempty"`
}

// GFSDiskStatusValue는 기존 Python createReturn val 구조와 호환되는 값이다.
// @name GFSDiskStatusValue
type GFSDiskStatusValue struct {
	Mode         string          `json:"mode" example:"multi"`
	Blockdevices []GFSDiskDevice `json:"blockdevices"`
}

// GFSDiskDevice는 mountpoint 기준으로 묶은 GFS2 디스크 정보이다.
// @name GFSDiskDevice
type GFSDiskDevice struct {
	LVM        string   `json:"lvm"`
	Mountpoint string   `json:"mountpoint"`
	Size       string   `json:"size"`
	Used       string   `json:"used"`
	Avail      string   `json:"avail"`
	UsePercent string   `json:"use_percent"`
	Multipaths []string `json:"multipaths"`
	Devices    []string `json:"devices"`
	DiskID     []string `json:"disk_id,omitempty"`
}

const gfsDiskUsageNA = "N/A"

// GFSDiskUsage는 df 출력에서 읽은 GFS2 마운트 사용량이다.
// @name GFSDiskUsage
type GFSDiskUsage struct {
	Filesystem string `json:"filesystem"`
	Mountpoint string `json:"mountpoint"`
	Size       string `json:"size"`
	Used       string `json:"used"`
	Avail      string `json:"avail"`
	UsePercent string `json:"use_percent"`
}

// GFSMount는 /proc/self/mounts에서 읽은 GFS2 마운트 정보이다.
// @name GFSMount
type GFSMount struct {
	Device     string `json:"device"`
	Mountpoint string `json:"mountpoint"`
}

type GFSLsblkPayload struct {
	Blockdevices []GFSBlockDevice `json:"blockdevices"`
}

// GFSBlockDevice는 lsblk JSON 중 GFS2 디스크 추적에 필요한 필드만 담는다.
// @name GFSBlockDevice
type GFSBlockDevice struct {
	Name       string           `json:"name"`
	Kname      string           `json:"kname,omitempty"`
	Path       *string          `json:"path,omitempty"`
	Size       *string          `json:"size,omitempty"`
	Type       *string          `json:"type,omitempty"`
	Mountpoint *string          `json:"mountpoint,omitempty"`
	DmUUID     *string          `json:"dm_uuid,omitempty"`
	Children   []GFSBlockDevice `json:"children,omitempty"`
}

type gfsDiskCandidate struct {
	mount      GFSMount
	node       GFSBlockDevice
	ancestors  []GFSBlockDevice
	entry      GFSDiskDevice
	idKeyNames []string
}

// ParseGFSLSBLK는 lsblk -J 출력을 GFS 디스크용 구조로 파싱한다.
func ParseGFSLSBLK(data []byte) ([]GFSBlockDevice, error) {
	var payload GFSLsblkPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse lsblk json: %w", err)
	}
	return payload.Blockdevices, nil
}

// ParseGFS2Mounts는 /proc/self/mounts 내용에서 gfs2 마운트만 추출한다.
func ParseGFS2Mounts(data []byte) []GFSMount {
	lines := strings.Split(string(data), "\n")
	mounts := make([]GFSMount, 0)
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[2] != "gfs2" {
			continue
		}
		device := unescapeProcMountField(fields[0])
		mountpoint := unescapeProcMountField(fields[1])
		if device == "" || mountpoint == "" {
			continue
		}
		mounts = append(mounts, GFSMount{
			Device:     device,
			Mountpoint: mountpoint,
		})
	}
	return mounts
}

// HasGFSMultipathDevice는 lsblk 결과에 multipath 장치가 있는지 확인한다.
func HasGFSMultipathDevice(devices []GFSBlockDevice) bool {
	for _, dev := range devices {
		if isGFSBlockMultipath(dev) {
			return true
		}
		if HasGFSMultipathDevice(dev.Children) {
			return true
		}
	}
	return false
}

// BuildGFSDiskStatus는 GFS2 마운트, lsblk 트리, by-id 맵으로 API 응답 값을 만든다.
func BuildGFSDiskStatus(devices []GFSBlockDevice, mounts []GFSMount, osType string, multipathMode bool, diskIDByKname map[string][]string, usageByMountpoint map[string]GFSDiskUsage) GFSDiskStatusValue {
	candidates := collectGFSDiskCandidates(devices, mounts, osType, multipathMode, diskIDByKname)
	blockdevices := groupGFSDiskCandidates(candidates, multipathMode, usageByMountpoint)
	mode := "single"
	for _, device := range blockdevices {
		if len(device.Multipaths) > 0 {
			mode = "multi"
			break
		}
	}
	return GFSDiskStatusValue{
		Mode:         mode,
		Blockdevices: blockdevices,
	}
}

func collectGFSDiskCandidates(devices []GFSBlockDevice, mounts []GFSMount, osType string, multipathMode bool, diskIDByKname map[string][]string) []gfsDiskCandidate {
	if len(devices) == 0 || len(mounts) == 0 {
		return nil
	}

	out := make([]gfsDiskCandidate, 0)
	var walk func(node GFSBlockDevice, ancestors []GFSBlockDevice)
	walk = func(node GFSBlockDevice, ancestors []GFSBlockDevice) {
		for _, mount := range mounts {
			if !matchesGFSMount(node, mount) {
				continue
			}
			candidate := buildGFSDiskCandidate(node, ancestors, mount, osType, multipathMode, diskIDByKname)
			if candidate.entry.LVM != "" && candidate.entry.Mountpoint != "" {
				out = append(out, candidate)
			}
		}

		nextAncestors := append(append([]GFSBlockDevice{}, ancestors...), node)
		for _, child := range node.Children {
			walk(child, nextAncestors)
		}
	}

	for _, dev := range devices {
		walk(dev, nil)
	}
	return out
}

func buildGFSDiskCandidate(node GFSBlockDevice, ancestors []GFSBlockDevice, mount GFSMount, osType string, multipathMode bool, diskIDByKname map[string][]string) gfsDiskCandidate {
	root := node
	if len(ancestors) > 0 {
		root = ancestors[0]
	}

	multipathNode, hasMultipath := nearestGFSMultipathNode(node, ancestors)
	multipathPath := ""
	idKeyNames := make([]string, 0, 2)
	if hasMultipath {
		multipathPath = gfsNodePath(multipathNode)
		idKeyNames = append(idKeyNames, multipathNode.Kname)
	}

	if multipathPath == "" {
		if isHciFilesystemCluster(osType) && len(ancestors) > 0 {
			parent := ancestors[len(ancestors)-1]
			multipathPath = gfsNodePath(parent)
			idKeyNames = append(idKeyNames, parent.Kname)
		} else {
			multipathPath = gfsNodePath(root)
			idKeyNames = append(idKeyNames, root.Kname)
		}
	}

	lvmPath := gfsNodePath(node)
	if lvmPath == "" {
		lvmPath = mount.Device
	}
	devicePath := gfsNodePath(root)
	if devicePath == "" {
		devicePath = multipathPath
	}
	size := strings.TrimSpace(gfsString(node.Size))

	entry := GFSDiskDevice{
		LVM:        lvmPath,
		Mountpoint: mount.Mountpoint,
		Size:       size,
	}

	if multipathMode && hasMultipath {
		entry.Multipaths = []string{multipathPath}
		idKeyNames = append(idKeyNames, node.Kname)
		entry.DiskID = collectGFSDiskIDs(idKeyNames, diskIDByKname)
	} else {
		entry.Devices = []string{devicePath}
	}

	return gfsDiskCandidate{
		mount:      mount,
		node:       node,
		ancestors:  ancestors,
		entry:      entry,
		idKeyNames: idKeyNames,
	}
}

func groupGFSDiskCandidates(candidates []gfsDiskCandidate, multipathMode bool, usageByMountpoint map[string]GFSDiskUsage) []GFSDiskDevice {
	type acc struct {
		item GFSDiskDevice
	}

	grouped := map[string]*acc{}
	order := make([]string, 0)
	for _, candidate := range candidates {
		item := candidate.entry
		key := item.Mountpoint
		if key == "" {
			continue
		}

		current, exists := grouped[key]
		if !exists {
			current = &acc{item: GFSDiskDevice{
				LVM:        item.LVM,
				Mountpoint: item.Mountpoint,
				Size:       item.Size,
			}}
			grouped[key] = current
			order = append(order, key)
		}

		if current.item.LVM == "" {
			current.item.LVM = item.LVM
		}
		if current.item.Size == "" {
			current.item.Size = item.Size
		}
		current.item.Multipaths = appendUniqueStrings(current.item.Multipaths, item.Multipaths...)
		current.item.Devices = appendUniqueStrings(current.item.Devices, item.Devices...)
		if multipathMode {
			current.item.DiskID = appendUniqueStrings(current.item.DiskID, item.DiskID...)
		}
	}

	out := make([]GFSDiskDevice, 0, len(order))
	for _, key := range order {
		item := grouped[key].item
		item = applyGFSDiskUsage(item, usageByMountpoint)
		sort.Strings(item.Multipaths)
		sort.Strings(item.Devices)
		sort.Strings(item.DiskID)
		if len(item.DiskID) > 1 {
			item.DiskID = item.DiskID[:1]
		}
		out = append(out, item)
	}
	return out
}

// ParseGFSDiskUsageDF는 `df -hP` 출력을 mountpoint 기준 사용량 맵으로 파싱한다.
func ParseGFSDiskUsageDF(data []byte) map[string]GFSDiskUsage {
	out := map[string]GFSDiskUsage{}
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 || strings.EqualFold(fields[0], "Filesystem") {
			continue
		}
		mountpoint := strings.Join(fields[5:], " ")
		usage := GFSDiskUsage{
			Filesystem: fields[0],
			Size:       fields[1],
			Used:       fields[2],
			Avail:      fields[3],
			UsePercent: fields[4],
			Mountpoint: mountpoint,
		}
		out[mountpoint] = usage
		cleanMountpoint := filepath.Clean(mountpoint)
		if cleanMountpoint != mountpoint {
			out[cleanMountpoint] = usage
		}
	}
	return out
}

func applyGFSDiskUsage(item GFSDiskDevice, usageByMountpoint map[string]GFSDiskUsage) GFSDiskDevice {
	usage, ok := lookupGFSDiskUsage(item.Mountpoint, usageByMountpoint)
	if !ok {
		return fillGFSDiskUsageDefaults(item)
	}
	if strings.TrimSpace(usage.Size) != "" {
		item.Size = strings.TrimSpace(usage.Size)
	}
	item.Used = defaultGFSDiskUsageValue(usage.Used)
	item.Avail = defaultGFSDiskUsageValue(usage.Avail)
	item.UsePercent = defaultGFSDiskUsageValue(usage.UsePercent)
	return fillGFSDiskUsageDefaults(item)
}

func lookupGFSDiskUsage(mountpoint string, usageByMountpoint map[string]GFSDiskUsage) (GFSDiskUsage, bool) {
	if len(usageByMountpoint) == 0 {
		return GFSDiskUsage{}, false
	}
	if usage, ok := usageByMountpoint[mountpoint]; ok {
		return usage, true
	}
	cleanMountpoint := filepath.Clean(mountpoint)
	usage, ok := usageByMountpoint[cleanMountpoint]
	return usage, ok
}

func fillGFSDiskUsageDefaults(item GFSDiskDevice) GFSDiskDevice {
	item.Used = defaultGFSDiskUsageValue(item.Used)
	item.Avail = defaultGFSDiskUsageValue(item.Avail)
	item.UsePercent = defaultGFSDiskUsageValue(item.UsePercent)
	return item
}

func defaultGFSDiskUsageValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return gfsDiskUsageNA
	}
	return value
}

func nearestGFSMultipathNode(node GFSBlockDevice, ancestors []GFSBlockDevice) (GFSBlockDevice, bool) {
	if isGFSBlockMultipath(node) {
		return node, true
	}
	for i := len(ancestors) - 1; i >= 0; i-- {
		if isGFSBlockMultipath(ancestors[i]) {
			return ancestors[i], true
		}
	}
	return GFSBlockDevice{}, false
}

func isGFSBlockMultipath(node GFSBlockDevice) bool {
	if strings.EqualFold(gfsString(node.Type), "mpath") {
		return true
	}
	return strings.Contains(strings.ToLower(gfsString(node.DmUUID)), "mpath-")
}

func matchesGFSMount(node GFSBlockDevice, mount GFSMount) bool {
	nodePath := gfsNodePath(node)
	mountDevice := strings.TrimSpace(mount.Device)
	mountpoint := strings.TrimSpace(mount.Mountpoint)

	if nodePath != "" && mountDevice != "" && filepath.Clean(nodePath) == filepath.Clean(mountDevice) {
		return true
	}
	if gfsString(node.Mountpoint) != "" && mountpoint != "" && filepath.Clean(gfsString(node.Mountpoint)) == filepath.Clean(mountpoint) {
		return true
	}
	base := filepath.Base(filepath.Clean(mountDevice))
	return base != "." && base != "/" && (node.Kname == base || node.Name == base)
}

func gfsNodePath(node GFSBlockDevice) string {
	if value := strings.TrimSpace(gfsString(node.Path)); value != "" {
		return value
	}
	if strings.TrimSpace(node.Kname) != "" {
		return "/dev/" + strings.TrimSpace(node.Kname)
	}
	if strings.TrimSpace(node.Name) != "" {
		return "/dev/" + strings.TrimSpace(node.Name)
	}
	return ""
}

func collectGFSDiskIDs(knameCandidates []string, diskIDByKname map[string][]string) []string {
	out := make([]string, 0)
	for _, kname := range knameCandidates {
		kname = strings.TrimSpace(kname)
		if kname == "" {
			continue
		}
		out = appendUniqueStrings(out, diskIDByKname[kname]...)
	}
	return out
}

func appendUniqueStrings(values []string, additions ...string) []string {
	seen := make(map[string]struct{}, len(values)+len(additions))
	out := make([]string, 0, len(values)+len(additions))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	for _, value := range additions {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func isHciFilesystemCluster(osType string) bool {
	return strings.EqualFold(strings.TrimSpace(osType), "ablestack-hci-filesystem")
}

func gfsString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func unescapeProcMountField(value string) string {
	replacer := strings.NewReplacer(
		`\\`, `\`,
		`\040`, " ",
		`\011`, "\t",
		`\012`, "\n",
		`\134`, `\`,
	)
	return replacer.Replace(value)
}
