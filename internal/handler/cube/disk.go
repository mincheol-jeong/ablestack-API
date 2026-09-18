package cube

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ablecloud.io/ablestack-api/internal/infra/logging"
	"ablecloud.io/ablestack-api/internal/infra/utils"
	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	"github.com/gin-gonic/gin"
	"github.com/goccy/go-json"
)

type DiskDevice = CubeModel.DiskDevice
type TypeBlockDevice = CubeModel.TypeBlockDevice
type DiskResponse = CubeModel.DiskResponse
type FlatViewResponse = CubeModel.FlatViewResponse
type FlatDiskItem = CubeModel.FlatDiskItem

type multipathLLMap struct {
	Name string
	UUID string
	Size string
}

/*
	핵심 목표입니다.

	1) 응답에서 code/val/name/type 같은 래핑 없이 아래 형태로만 내려가게 합니다.
	   {
	     "blockdevices": [...],
	     "raidcontrollers": [...],
	     "refresh_time": "..."
	   }

	2) children은 "자식이 있을 때만" 나오게 합니다.
	   - children slice가 빈 경우 nil로 통일 -> `omitempty`로 키 자체가 사라집니다.

	3) action(list,gfs,rbd,detail)에 따라
	   - lsblk 컬럼을 다르게 가져오고
	   - 필터링 정책 및 id/path/rbd_path 보강을 적용합니다.

	4) view=flat 일 때는 트리 구조를 평탄화하여 devices 배열로 내려줍니다.
*/

// GetDisk godoc
//
//	@Summary		Show List of Disk
//	@Description	Cube-Disk의 Disk목록을 보여줍니다. action=detail은 multipath -ll 기준 map 또는 OS 영역을 제외한 lsblk 물리 디스크 목록을 반환합니다.
//	@Tags			Cube-Disk
//	@Accept			x-www-form-urlencoded
//	@Produce		json
//	@Param			action	query	string	false	"disk action"	Enums(list,gfs,rbd,detail)
//	@Param			view	query	string	false	"response view"	Enums(tree,flat,list)
//	@Success		200	{object}	CubeModel.DiskResponse
//	@Failure		400	{object}	HTTP400BadRequest
//	@Failure		404	{object}	HTTP404NotFound
//	@Failure		500	{object}	HTTP500InternalServerError
//	@Router			/cube/disk [get]
func GetDisk(context *gin.Context) {
	action := normalizeDiskAction(context.DefaultQuery("action", "list"))

	view := strings.ToLower(context.DefaultQuery("view", "tree"))

	current := &TypeBlockDevice{}
	if err := updateWithAction(current, action); err != nil {
		context.JSON(http.StatusInternalServerError, utils.HTTP500InternalServerError{
			ErrCode: http.StatusInternalServerError,
			Message: "failed to read disk list",
		})
		return
	}

	// children slice가 비어있으면 nil로 통일하여 omitempty로 키 자체가 사라지게 합니다.
	block := ensureNilChildren(current.Blockdevices)

	if view == "flat" || view == "list" {
		context.IndentedJSON(http.StatusOK, buildFlatResponse(block, current.RaidControllers, current.RefreshTime))
		return
	}

	context.IndentedJSON(http.StatusOK, DiskResponse{
		Blockdevices:    block,
		RaidControllers: current.RaidControllers,
		RefreshTime:     current.RefreshTime.Format(time.RFC3339),
	})
}

// UpdateDisk는 전역 디스크 캐시를 list 기준으로 한 번 갱신한다.
func UpdateDisk() {
	d := CubeModel.Disk()
	d.Lock()
	defer d.Unlock()
	err := updateWithAction(d, "list")
	logging.RecordJobResult("cube.UpdateDisk", err, map[string]any{"action": "list"})
}

// updateWithAction은 action 정책에 맞춰 디스크 정보를 수집하고 후처리까지 수행한다.
func updateWithAction(d *TypeBlockDevice, action string) error {
	if d == nil {
		return nil
	}

	action = normalizeDiskAction(action)

	// 1) lsblk에서 JSON 수집
	if err := refreshFromLsblk(d, action); err != nil {
		return err
	}

	// 2) pkname 기반으로 트리 재구성
	if action != "detail" && action != "gfs" {
		rebuildTreeByPkname(d)
	}

	// 3) 상세 조회의 멀티패스 목록은 multipath -ll 결과를 기준으로 확정합니다.
	var detailMaps []multipathLLMap
	detailModeKnown := false
	detailMultipathMode := false
	if action == "detail" {
		detailMaps, detailModeKnown = readMultipathLLMaps()
		if detailModeKnown {
			detailMultipathMode = len(detailMaps) > 0
			detailMaps = filterOSMultipathLLMaps(detailMaps, d.Blockdevices)
		}
	}

	// 4) action 정책 적용(필터 + id/path/rbd_path 보강 + raidcontroller 수집)
	applyDiskAction(d, action, detailMaps, detailModeKnown, detailMultipathMode)
	return nil
}

func readMultipathLLMaps() ([]multipathLLMap, bool) {
	out, timedOut, err := runCommandOutputWithEnv(
		"multipath",
		5*time.Second,
		append([]string{"LANG=C", "LANGUAGE=C"}, os.Environ()...),
		"-ll",
	)
	if timedOut || err != nil {
		return nil, false
	}
	return parseMultipathLL(out), true
}

func parseMultipathLL(output string) []multipathLLMap {
	maps := make([]multipathLLMap, 0)
	currentIndex := -1

	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		dmIndex := -1
		for index, field := range fields {
			if strings.HasPrefix(field, "dm-") {
				dmIndex = index
				break
			}
		}
		if dmIndex > 0 {
			name := fields[0]
			uuid := name
			if dmIndex > 1 && strings.HasPrefix(fields[1], "(") && strings.HasSuffix(fields[1], ")") {
				uuid = strings.Trim(fields[1], "()")
			}
			maps = append(maps, multipathLLMap{
				Name: strings.TrimSpace(name),
				UUID: strings.TrimSpace(uuid),
			})
			currentIndex = len(maps) - 1
		}

		if currentIndex >= 0 {
			for _, field := range fields {
				if strings.HasPrefix(field, "size=") {
					maps[currentIndex].Size = strings.TrimPrefix(field, "size=")
					break
				}
			}
		}
	}

	return maps
}

func filterOSMultipathLLMaps(maps []multipathLLMap, devices []DiskDevice) []multipathLLMap {
	osIdentities := map[string]struct{}{}
	var walk func(DiskDevice)
	walk = func(device DiskDevice) {
		if isMultipathDevice(device) && hasOSMountpoint(device) {
			for _, identity := range multipathDeviceIdentities(device) {
				osIdentities[identity] = struct{}{}
			}
		}
		for _, child := range device.Children {
			walk(child)
		}
	}
	for _, device := range devices {
		walk(device)
	}

	filtered := make([]multipathLLMap, 0, len(maps))
	for _, item := range maps {
		if _, found := osIdentities[normalizeDiskIdentity(item.Name)]; found {
			continue
		}
		if _, found := osIdentities[normalizeDiskIdentity(item.UUID)]; found {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

// refreshFromLsblk는 lsblk JSON 출력을 읽어 현재 디스크 모델에 반영한다.
func refreshFromLsblk(d *TypeBlockDevice, action string) error {
	args := lsblkArgsForAction(action)
	cmd := exec.Command("lsblk", args...)
	stdout, err := cmd.CombinedOutput()
	if err != nil && strings.Contains(string(stdout), "unknown column") && strings.Contains(string(stdout), "dm_uuid") {
		// dm_uuid 미지원 환경에서는 재시도합니다.
		args = lsblkArgsForActionNoDmUUID(action)
		cmd = exec.Command("lsblk", args...)
		stdout, err = cmd.CombinedOutput()
	}
	if err != nil {
		if gin.IsDebugging() {
			msg := strings.TrimSpace(string(stdout))
			if msg != "" {
				utils.FancyHandleError(fmt.Errorf("lsblk failed: %w: %s", err, msg))
			} else {
				utils.FancyHandleError(fmt.Errorf("lsblk failed: %w", err))
			}
		}
		return err
	}

	// lsblk JSON -> TypeBlockDevice로 파싱됩니다(필드명이 맞아야 합니다).
	if err := json.Unmarshal(stdout, d); err != nil {
		if gin.IsDebugging() {
			utils.FancyHandleError(err)
		}
		return err
	}
	d.RefreshTime = time.Now()
	return nil
}

// lsblkArgsForAction은 action별로 필요한 lsblk 컬럼 목록을 반환한다.
func lsblkArgsForAction(action string) []string {
	switch action {
	case "detail":
		return []string{"-J", "-o", "name,kname,pkname,path,mountpoint,dm_uuid,state,size,type"}
	case "multipath":
		return []string{"-J", "-o", "name,kname,pkname,path,mountpoint,dm_uuid,state,size,type"}
	case "rbd":
		return []string{"-J", "-o", "name,kname,pkname,path,mountpoint,dm_uuid,state,size,type"}
	default:
		// list, gfs and fallback
		return []string{"-J", "-o", "name,kname,pkname,path,mountpoint,dm_uuid,rota,model,size,state,group,type,tran,subsystems,vendor,wwn"}
	}
}

// lsblkArgsForActionNoDmUUID는 dm_uuid 컬럼이 없는 환경에서 사용할 대체 인자를 반환한다.
func lsblkArgsForActionNoDmUUID(action string) []string {
	switch action {
	case "detail":
		return []string{"-J", "-o", "name,kname,pkname,path,mountpoint,state,size,type"}
	case "multipath":
		return []string{"-J", "-o", "name,kname,pkname,path,mountpoint,state,size,type"}
	case "rbd":
		return []string{"-J", "-o", "name,kname,pkname,path,mountpoint,state,size,type"}
	default:
		// list, gfs and fallback
		return []string{"-J", "-o", "name,kname,pkname,path,mountpoint,rota,model,size,state,group,type,tran,subsystems,vendor,wwn"}
	}
}

/*
	-------------------------------------------
	flat(view=flat) 응답 생성
	-------------------------------------------
*/

// buildFlatResponse는 트리 구조 디스크 정보를 평탄화해 flat 응답 모델로 변환한다.
func buildFlatResponse(block []DiskDevice, raid []map[string]string, refresh time.Time) FlatViewResponse {
	items := make([]FlatDiskItem, 0, 64)

	var walk func(node DiskDevice, parent string, depth int)
	walk = func(node DiskDevice, parent string, depth int) {
		key := diskNodeKeyTyped(node)
		item := FlatDiskItem{
			Name:    node.Name,
			Kname:   node.Kname,
			Path:    node.Path,
			ID:      node.ID,
			RbdPath: node.RbdPath,
			Type:    node.Type,
			Parent:  parent,
			Depth:   depth,
		}
		items = append(items, item)

		for _, child := range node.Children {
			walk(child, key, depth+1)
		}
	}

	for _, dev := range block {
		walk(dev, "", 0)
	}

	return FlatViewResponse{
		Devices:         items,
		RaidControllers: raid,
		RefreshTime:     refresh.Format(time.RFC3339),
	}
}

// diskNodeKeyTyped는 디스크 노드를 식별하기 위한 우선순위 키를 계산한다.
func diskNodeKeyTyped(node DiskDevice) string {
	if node.Kname != "" {
		return node.Kname
	}
	if node.Name != "" {
		return node.Name
	}
	if node.Path != nil && *node.Path != "" {
		return *node.Path
	}
	return ""
}

/*
	-------------------------------------------
	children 처리: 비어있으면 nil로 바꿔 omitempty 적용
	-------------------------------------------
*/

// ensureNilChildren은 비어 있는 children 슬라이스를 nil로 바꿔 JSON 응답을 정리한다.
func ensureNilChildren(list []DiskDevice) []DiskDevice {
	if len(list) == 0 {
		return nil
	}
	out := make([]DiskDevice, 0, len(list))
	for _, dev := range list {
		if len(dev.Children) == 0 {
			dev.Children = nil
		} else {
			dev.Children = ensureNilChildren(dev.Children)
		}
		out = append(out, dev)
	}
	return out
}

/*
	-------------------------------------------
	pkname 기반 트리 재조립
	-------------------------------------------
*/

// strOrEmpty는 포인터 문자열을 nil-safe하게 일반 문자열로 바꾼다.
func strOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// flattenDevices는 중첩된 children 구조를 단일 슬라이스로 펼친다.
func flattenDevices(devs []DiskDevice) []DiskDevice {
	out := make([]DiskDevice, 0)
	var walk func(list []DiskDevice)
	walk = func(list []DiskDevice) {
		for _, dev := range list {
			out = append(out, dev)
			if len(dev.Children) > 0 {
				walk(dev.Children)
			}
		}
	}
	walk(devs)
	return out
}

// mergeDiskDevice는 비어 있는 필드만 채우는 방식으로 두 디스크 노드를 합친다.
func mergeDiskDevice(dst DiskDevice, src DiskDevice) DiskDevice {
	// 값이 비어있는 쪽만 채우는 간단 merge입니다.
	if dst.Name == "" {
		dst.Name = src.Name
	}
	if dst.Kname == "" {
		dst.Kname = src.Kname
	}
	if dst.Pkname == nil {
		dst.Pkname = src.Pkname
	}
	if dst.Path == nil {
		dst.Path = src.Path
	}
	if dst.Mountpoint == nil {
		dst.Mountpoint = src.Mountpoint
	}
	if dst.DmUUID == nil {
		dst.DmUUID = src.DmUUID
	}
	if dst.ID == nil {
		dst.ID = src.ID
	}
	if dst.RbdPath == nil {
		dst.RbdPath = src.RbdPath
	}
	if dst.Rota == nil {
		dst.Rota = src.Rota
	}
	if dst.Model == nil {
		dst.Model = src.Model
	}
	if dst.Size == nil {
		dst.Size = src.Size
	}
	if dst.State == nil {
		dst.State = src.State
	}
	if dst.Group == nil {
		dst.Group = src.Group
	}
	if dst.Type == nil {
		dst.Type = src.Type
	}
	if dst.Tran == nil {
		dst.Tran = src.Tran
	}
	if dst.Subsystems == nil {
		dst.Subsystems = src.Subsystems
	}
	if dst.Vendor == nil {
		dst.Vendor = src.Vendor
	}
	if dst.Wwn == nil {
		dst.Wwn = src.Wwn
	}
	return dst
}

// mergeDiskDeviceWithChildren은 기본 필드 병합 후 children까지 보존해 합친다.
func mergeDiskDeviceWithChildren(dst DiskDevice, src DiskDevice) DiskDevice {
	merged := mergeDiskDevice(dst, src)
	if len(merged.Children) == 0 && len(src.Children) > 0 {
		merged.Children = src.Children
	}
	return merged
}

// singlePathEntry는 multipath 응답에 넣을 최소 단일 경로 정보를 구성한다.
func singlePathEntry(dev DiskDevice) DiskDevice {
	return DiskDevice{
		Name:  dev.Name,
		Kname: dev.Kname,
		Path:  dev.Path,
	}
}

// mpathAccumulator는 multipath 디바이스와 연결된 single path 목록을 누적하는 내부 구조체다.
type mpathAccumulator struct {
	device      DiskDevice
	singleSeen  map[string]bool
	singlePaths []DiskDevice
}

// addSinglePath는 중복 없이 single path 정보를 multipath 누적기에 추가한다.
func addSinglePath(acc *mpathAccumulator, single DiskDevice) {
	key := diskNodeKeyTyped(single)
	if key == "" {
		return
	}
	if acc.singleSeen[key] {
		return
	}
	acc.singleSeen[key] = true
	single.Children = nil
	single.SinglePath = nil
	single.Pkname = nil
	acc.singlePaths = append(acc.singlePaths, single)
}

// isMultipathDevice는 현재 노드가 multipath 디바이스인지 판별한다.
func isMultipathDevice(dev DiskDevice) bool {
	if strings.EqualFold(strOrEmpty(dev.Type), "mpath") {
		return true
	}
	if dev.DmUUID == nil {
		return false
	}
	if strings.EqualFold(strOrEmpty(dev.Type), "part") {
		return false
	}
	return strings.Contains(strings.ToLower(*dev.DmUUID), "mpath-")
}

// collectMultipathNodes는 특정 디바이스 하위에서 발견되는 multipath 노드만 수집한다.
func collectMultipathNodes(dev DiskDevice) []DiskDevice {
	out := make([]DiskDevice, 0)
	var walk func(node DiskDevice)
	walk = func(node DiskDevice) {
		if isMultipathDevice(node) {
			out = append(out, node)
			return
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	for _, child := range dev.Children {
		walk(child)
	}
	return out
}

// buildMultipathDevices는 평탄한 디스크 목록을 multipath 중심 응답 구조로 재조합한다.
func buildMultipathDevices(devs []DiskDevice) []DiskDevice {
	mpathBy := map[string]*mpathAccumulator{}
	order := make([]string, 0)
	singles := make([]DiskDevice, 0)

	addMpath := func(key string, node DiskDevice) *mpathAccumulator {
		if acc, ok := mpathBy[key]; ok {
			acc.device = mergeDiskDeviceWithChildren(acc.device, node)
			return acc
		}
		acc := &mpathAccumulator{
			device:     node,
			singleSeen: map[string]bool{},
		}
		mpathBy[key] = acc
		order = append(order, key)
		return acc
	}

	for _, dev := range devs {
		if isMultipathDevice(dev) {
			key := diskNodeKeyTyped(dev)
			if key == "" {
				continue
			}
			addMpath(key, dev)
			continue
		}

		mpathChildren := collectMultipathNodes(dev)
		if len(mpathChildren) == 0 {
			singleRoot := dev
			singleRoot.SinglePath = []DiskDevice{singlePathEntry(dev)}
			singles = append(singles, singleRoot)
			continue
		}

		single := singlePathEntry(dev)
		for _, child := range mpathChildren {
			key := diskNodeKeyTyped(child)
			if key == "" {
				continue
			}
			acc := addMpath(key, child)
			addSinglePath(acc, single)
		}
	}

	result := make([]DiskDevice, 0, len(order))
	for _, key := range order {
		acc := mpathBy[key]
		if acc == nil {
			continue
		}
		if len(acc.singlePaths) == 0 {
			acc.device.SinglePath = nil
		} else {
			acc.device.SinglePath = acc.singlePaths
		}
		result = append(result, acc.device)
	}
	if len(singles) > 0 {
		result = append(result, singles...)
	}
	return result
}

// filterDetailSingles는 multipath가 존재하는 detail 응답에서 단일 디스크를 제거한다.
func filterDetailSingles(devs []DiskDevice) []DiskDevice {
	if len(devs) == 0 {
		return devs
	}
	hasMultipath := false
	for _, dev := range devs {
		if isMultipathDevice(dev) {
			hasMultipath = true
			break
		}
	}
	if !hasMultipath {
		return devs
	}

	out := make([]DiskDevice, 0, len(devs))
	for _, dev := range devs {
		if isMultipathDevice(dev) {
			out = append(out, dev)
		}
	}
	return out
}

// isOSMountpoint는 운영체제 영역으로 간주할 마운트포인트인지 확인한다.
func isOSMountpoint(mp string) bool {
	switch mp {
	case "/", "/boot", "/boot/efi":
		return true
	default:
		return strings.Contains(mp, "[SWAP]")
	}
}

// hasOSMountpoint는 디스크 또는 하위 경로에 OS 마운트포인트가 포함됐는지 확인한다.
func hasOSMountpoint(dev DiskDevice) bool {
	if dev.Mountpoint != nil && isOSMountpoint(*dev.Mountpoint) {
		return true
	}
	for _, child := range dev.Children {
		if hasOSMountpoint(child) {
			return true
		}
	}
	for _, single := range dev.SinglePath {
		if hasOSMountpoint(single) {
			return true
		}
	}
	return false
}

// filterOutOSDisks는 OS가 사용하는 디스크를 API 응답에서 제외한다.
func filterOutOSDisks(devs []DiskDevice) []DiskDevice {
	if len(devs) == 0 {
		return devs
	}
	out := make([]DiskDevice, 0, len(devs))
	for _, dev := range devs {
		if hasOSMountpoint(dev) {
			continue
		}
		out = append(out, dev)
	}
	return out
}

func markPartitionedMultipathDevicesInUse(devs []DiskDevice) {
	for i := range devs {
		if isMultipathDevice(devs[i]) && hasPartitionDescendant(devs[i]) {
			devs[i].InUse = true
			devs[i].InUseReason = "partition exists"
		}
	}
}

func annotateDiskIdentities(devs []DiskDevice) {
	for i := range devs {
		if isMultipathDevice(devs[i]) {
			devs[i].PathMode = "multipath"
		} else {
			devs[i].PathMode = "single"
		}
		devs[i].UUID = stableDiskUUID(devs[i])
	}
}

func stableDiskUUID(dev DiskDevice) string {
	if isMultipathDevice(dev) {
		if uuid := multipathUUID(strOrEmpty(dev.DmUUID)); uuid != "" {
			return uuid
		}
	}
	if dev.ID != nil {
		if uuid := diskUUIDFromID(*dev.ID); uuid != "" {
			return uuid
		}
	}
	return strings.TrimSpace(strOrEmpty(dev.Wwn))
}

func normalizeDiskIdentity(value string) string {
	return strings.ToLower(strings.TrimSpace(filepath.Base(value)))
}

func multipathDeviceIdentities(dev DiskDevice) []string {
	values := []string{
		dev.Name,
		dev.Kname,
		strOrEmpty(dev.Path),
		stableDiskUUID(dev),
		multipathUUID(strOrEmpty(dev.DmUUID)),
	}
	identities := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		identity := normalizeDiskIdentity(value)
		if identity == "" {
			continue
		}
		if _, found := seen[identity]; found {
			continue
		}
		seen[identity] = struct{}{}
		identities = append(identities, identity)
	}
	return identities
}

func reconcileMultipathLLDevices(devices []DiskDevice, maps []multipathLLMap) []DiskDevice {
	byIdentity := map[string]DiskDevice{}
	for _, device := range devices {
		if !isMultipathDevice(device) {
			continue
		}
		for _, identity := range multipathDeviceIdentities(device) {
			byIdentity[identity] = device
		}
	}

	result := make([]DiskDevice, 0, len(maps))
	for _, item := range maps {
		device, found := byIdentity[normalizeDiskIdentity(item.Name)]
		if !found {
			device, found = byIdentity[normalizeDiskIdentity(item.UUID)]
		}
		if !found {
			device = DiskDevice{Name: item.Name, Kname: item.Name}
		}

		device.Name = item.Name
		device.UUID = item.UUID
		device.PathMode = "multipath"
		devicePath := "/dev/mapper/" + item.Name
		device.Path = &devicePath
		stablePath := "/dev/disk/by-id/dm-uuid-mpath-" + item.UUID
		device.ID = &stablePath
		deviceType := "mpath"
		device.Type = &deviceType
		if strings.TrimSpace(item.Size) != "" {
			size := item.Size
			device.Size = &size
		}
		device.Children = nil
		device.SinglePath = nil
		device.Pkname = nil
		result = append(result, device)
	}
	return result
}

func physicalDiskRoots(devices []DiskDevice) []DiskDevice {
	result := make([]DiskDevice, 0, len(devices))
	for _, device := range devices {
		if !strings.EqualFold(strOrEmpty(device.Type), "disk") {
			continue
		}
		result = append(result, device)
	}
	return result
}

func multipathUUID(value string) string {
	value = strings.TrimSpace(filepath.Base(value))
	for _, prefix := range []string{"dm-uuid-part1-mpath-", "dm-uuid-mpath-", "part1-mpath-", "mpath-"} {
		if strings.HasPrefix(strings.ToLower(value), prefix) {
			return value[len(prefix):]
		}
	}
	return ""
}

func diskUUIDFromID(value string) string {
	base := strings.TrimSpace(filepath.Base(value))
	if uuid := multipathUUID(base); uuid != "" {
		return uuid
	}
	base = strings.TrimSuffix(base, "-part1")
	for _, prefix := range []string{"wwn-", "scsi-"} {
		if strings.HasPrefix(strings.ToLower(base), prefix) {
			return base[len(prefix):]
		}
	}
	return ""
}

func hasPartitionDescendant(dev DiskDevice) bool {
	for _, child := range dev.Children {
		if strings.EqualFold(strOrEmpty(child.Type), "part") || hasPartitionDescendant(child) {
			return true
		}
	}
	return false
}

// clearPknameRecursive는 재구성 후 pkname 필드를 재귀적으로 제거한다.
func clearPknameRecursive(dev *DiskDevice) {
	if dev == nil {
		return
	}
	dev.Pkname = nil
	for i := range dev.Children {
		clearPknameRecursive(&dev.Children[i])
	}
}

// clearMountpointRecursive는 응답에 불필요한 mountpoint와 dm_uuid를 재귀적으로 제거한다.
func clearMountpointRecursive(dev *DiskDevice) {
	if dev == nil {
		return
	}
	dev.Mountpoint = nil
	dev.DmUUID = nil
	for i := range dev.Children {
		clearMountpointRecursive(&dev.Children[i])
	}
	for i := range dev.SinglePath {
		clearMountpointRecursive(&dev.SinglePath[i])
	}
}

// rebuildTreeByPkname는 lsblk 평면 결과를 pkname 기준 부모-자식 트리로 다시 조립한다.
func rebuildTreeByPkname(d *TypeBlockDevice) {
	if d == nil {
		return
	}

	flat := flattenDevices(d.Blockdevices)
	if len(flat) == 0 {
		return
	}

	byKname := map[string]DiskDevice{}
	parentBy := map[string]string{}
	order := make([]string, 0)
	seen := map[string]bool{}
	noKeyRoots := make([]DiskDevice, 0)

	for _, dev := range flat {
		// 트리를 깨고 pkname으로 재구성하기 위해 children 제거
		dev.Children = nil

		// kname이 없는 경우는 root로 그대로 둡니다.
		if dev.Kname == "" {
			dev.Pkname = nil
			noKeyRoots = append(noKeyRoots, dev)
			continue
		}

		if !seen[dev.Kname] {
			order = append(order, dev.Kname)
			seen[dev.Kname] = true
		}

		if existing, ok := byKname[dev.Kname]; ok {
			byKname[dev.Kname] = mergeDiskDevice(existing, dev)
		} else {
			byKname[dev.Kname] = dev
		}

		// 부모 후보(pkname) 기록
		if dev.Pkname != nil && *dev.Pkname != "" {
			if prev, ok := parentBy[dev.Kname]; !ok || prev == "" {
				parentBy[dev.Kname] = *dev.Pkname
			}
		}
	}

	// parent -> children 목록 만들기
	childrenOf := map[string][]string{}
	roots := make([]string, 0)
	for _, kname := range order {
		parent := parentBy[kname]
		if parent != "" {
			if _, ok := byKname[parent]; ok {
				childrenOf[parent] = append(childrenOf[parent], kname)
				continue
			}
		}
		roots = append(roots, kname)
	}

	// 순환 참조 방지용
	var build func(kname string, stack map[string]bool) DiskDevice
	build = func(kname string, stack map[string]bool) DiskDevice {
		node, ok := byKname[kname]
		if !ok {
			return DiskDevice{Name: kname}
		}
		if stack[kname] {
			node.Pkname = nil
			return node
		}
		stack[kname] = true

		node.Children = nil
		for _, childKey := range childrenOf[kname] {
			child := build(childKey, stack)
			node.Children = append(node.Children, child)
		}

		delete(stack, kname)
		node.Pkname = nil // 응답에서 pkname을 숨기려면 nil로 처리합니다.
		return node
	}

	rebuilt := make([]DiskDevice, 0, len(roots)+len(noKeyRoots))
	seen = map[string]bool{}
	for _, rootKey := range roots {
		if seen[rootKey] {
			continue
		}
		rebuilt = append(rebuilt, build(rootKey, map[string]bool{}))
		seen[rootKey] = true
	}

	rebuilt = append(rebuilt, noKeyRoots...)
	d.Blockdevices = rebuilt
}

/*
	-------------------------------------------
	action 정책 적용(필터링 + id/path/rbd_path 보강)
	-------------------------------------------
*/

// normalizeDiskAction은 외부 입력 action을 내부 표준 동작 값으로 정규화한다.
func normalizeDiskAction(action string) string {
	switch action {
	case "list":
		return "list"
	case "gfs", "gfs-list":
		return "gfs"
	case "multipath", "mpath", "mpath-list", "detail":
		return "detail"
	case "rbd", "hci-file-system-list", "hci-shared-file-list":
		return "rbd"
	default:
		return "list"
	}
}

// list: /dev/disk/by-path
// gfs: /dev/disk/by-id (multipath DM UUID 또는 single-path stable ID)
// detail: /dev/disk/by-id (multipath DM UUID 또는 single-path stable ID)
// rbd: /dev/rbd/rbd
// diskPathMap은 action에 맞는 디스크 경로 심볼릭 링크 맵을 읽어 반환한다.
func diskPathMap(action string) map[string]string {
	switch action {
	case "list":
		return readSymlinkMap("/dev/disk/by-path", nil)
	case "rbd":
		return readSymlinkMap("/dev/rbd/rbd", func(name string) bool {
			return strings.Contains(name, "rbd")
		})
	case "gfs", "detail":
		return readStableDiskIDMap()
	default:
		return readSymlinkMap("/dev/disk/by-id", func(name string) bool {
			return strings.Contains(name, "dm-uuid") && !strings.Contains(name, "LVM")
		})
	}
}

func readStableDiskIDMap() map[string]string {
	result := map[string]string{}
	priorities := map[string]int{}
	entries, err := os.ReadDir("/dev/disk/by-id")
	if err != nil {
		return result
	}
	for _, entry := range entries {
		name := entry.Name()
		priority := stableDiskIDPriority(name)
		if priority == 0 || entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		full := filepath.Join("/dev/disk/by-id", name)
		resolved, err := filepath.EvalSymlinks(full)
		if err != nil {
			continue
		}
		kname := filepath.Base(resolved)
		if kname == "" || priority <= priorities[kname] {
			continue
		}
		result[kname] = full
		priorities[kname] = priority
	}
	return result
}

func stableDiskIDPriority(name string) int {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || strings.Contains(name, "-part") || strings.Contains(name, "lvm") {
		return 0
	}
	switch {
	case strings.HasPrefix(name, "dm-uuid-mpath-"):
		return 100
	case strings.HasPrefix(name, "wwn-"):
		return 80
	case strings.HasPrefix(name, "scsi-"):
		return 70
	case strings.HasPrefix(name, "nvme-"):
		return 60
	case strings.HasPrefix(name, "ata-"):
		return 50
	default:
		return 0
	}
}

// readSymlinkMap은 지정 디렉터리의 심볼릭 링크를 읽어 kname 기준 맵으로 변환한다.
func readSymlinkMap(dir string, allow func(name string) bool) map[string]string {
	result := map[string]string{}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if gin.IsDebugging() {
			fmt.Println(err)
		}
		return result
	}

	for _, entry := range entries {
		name := entry.Name()
		if allow != nil && !allow(name) {
			continue
		}
		if entry.Type()&os.ModeSymlink == 0 {
			continue
		}

		full := filepath.Join(dir, name)
		target, err := os.Readlink(full)
		if err != nil {
			continue
		}

		// 상대 경로면 dir 기준으로 보정
		resolved := target
		if !filepath.IsAbs(target) {
			resolved = filepath.Join(dir, target)
		}

		kname := filepath.Base(filepath.Clean(resolved))
		if kname == "" {
			continue
		}
		result[kname] = full
	}

	return result
}

// loop/usb/cdrom 제외 정책을 공통화합니다.
func shouldSkipDevice(dev DiskDevice) bool {
	devType := strings.ToLower(strOrEmpty(dev.Type))
	devGroup := strings.ToLower(strOrEmpty(dev.Group))
	devTran := strings.ToLower(strOrEmpty(dev.Tran))

	if strings.Contains(devType, "loop") {
		return true
	}
	if strings.Contains(devTran, "usb") {
		return true
	}
	if strings.Contains(devGroup, "cdrom") {
		return true
	}
	return false
}

// 트리 전체를 순회하면서 kname 기준으로 ID를 보강합니다.
func attachIDRecursive(dev *DiskDevice, pathMap map[string]string) {
	if dev == nil {
		return
	}
	if dev.Kname != "" {
		if id, ok := pathMap[dev.Kname]; ok {
			val := id
			dev.ID = &val
		}
	}
	for i := range dev.Children {
		attachIDRecursive(&dev.Children[i], pathMap)
	}
}

// applyDiskAction은 action별 필터링, 경로 보강, RAID 컨트롤러 수집을 적용한다.
func applyDiskAction(
	d *TypeBlockDevice,
	action string,
	detailMaps []multipathLLMap,
	detailModeKnown bool,
	detailMultipathMode bool,
) {
	if d == nil {
		return
	}

	pathMap := diskPathMap(action)
	filtered := make([]DiskDevice, 0, len(d.Blockdevices))

	for _, dev := range d.Blockdevices {
		switch action {
		case "rbd":
			// rbd 디바이스만 노출합니다.
			if strings.Contains(strings.ToLower(strOrEmpty(dev.Type)), "loop") || !strings.Contains(dev.Name, "rbd") {
				continue
			}

			// rbd symlink 경로 보강
			if path, ok := pathMap[dev.Name]; ok {
				devPath := "/dev/" + dev.Name
				dev.Path = &devPath

				val := path
				dev.RbdPath = &val

				// 파티션이 존재하면 -part1 형태로 보강합니다(정책에 맞게 조정 가능).
				if len(dev.Children) > 0 {
					rbdPartPath := path + "-part1"
					for i := range dev.Children {
						childVal := rbdPartPath
						dev.Children[i].RbdPath = &childVal
					}
				}
			}
			filtered = append(filtered, dev)

		case "gfs", "detail":
			// 공통 skip 정책 적용
			if shouldSkipDevice(dev) {
				continue
			}
			// dm-uuid(by-id) 보강
			attachIDRecursive(&dev, pathMap)
			filtered = append(filtered, dev)

		default:
			// list 기본
			if shouldSkipDevice(dev) {
				continue
			}
			// list는 by-path를 name 기준으로 넣어줍니다.
			if path, ok := pathMap[dev.Name]; ok {
				val := path
				dev.Path = &val
			}
			filtered = append(filtered, dev)
		}
	}

	if action == "detail" || action == "gfs" {
		if action == "detail" && detailModeKnown && !detailMultipathMode {
			d.Blockdevices = physicalDiskRoots(filtered)
		} else {
			d.Blockdevices = buildMultipathDevices(filtered)
		}
		annotateDiskIdentities(d.Blockdevices)
		if action == "gfs" {
			markPartitionedMultipathDevicesInUse(d.Blockdevices)
		}
		for i := range d.Blockdevices {
			clearPknameRecursive(&d.Blockdevices[i])
		}
		if action == "detail" {
			if detailModeKnown && detailMultipathMode {
				d.Blockdevices = reconcileMultipathLLDevices(d.Blockdevices, detailMaps)
			} else if !detailModeKnown {
				d.Blockdevices = filterDetailSingles(d.Blockdevices)
			}
		}
	} else {
		d.Blockdevices = filtered
	}
	d.Blockdevices = filterOutOSDisks(d.Blockdevices)
	for i := range d.Blockdevices {
		clearMountpointRecursive(&d.Blockdevices[i])
	}
	switch action {
	case "gfs", "detail":
		d.RaidControllers = nil
	default:
		d.RaidControllers = filterRaidControllers(listPCIDevices())
	}
	d.RefreshTime = time.Now()
}

/*
	-------------------------------------------
	RAID Controller 탐지(lspci)
	-------------------------------------------
*/

// listPCIDevices는 lspci 결과를 블록 단위로 파싱해 PCI 장치 목록으로 반환한다.
func listPCIDevices() []map[string]string {
	cmd := exec.Command("/usr/sbin/lspci", "-vmm", "-k")
	stdout, err := cmd.CombinedOutput()
	if err != nil {
		if gin.IsDebugging() {
			utils.FancyHandleError(err)
		}
		return nil
	}

	lines := strings.Split(string(stdout), "\n")
	devices := make([]map[string]string, 0)

	current := map[string]string{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if len(current) > 0 {
				devices = append(devices, current)
				current = map[string]string{}
			}
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if key != "" {
			current[key] = val
		}
	}
	if len(current) > 0 {
		devices = append(devices, current)
	}

	return devices
}

// filterRaidControllers는 PCI 장치 목록에서 RAID/NVMe 컨트롤러만 추려낸다.
func filterRaidControllers(devices []map[string]string) []map[string]string {
	raid := make([]map[string]string, 0)
	for _, dev := range devices {
		class := strings.ToLower(dev["Class"])
		if strings.Contains(class, "raid") || strings.Contains(class, "non-volatile memory controller") {
			raid = append(raid, dev)
		}
	}
	return raid
}
