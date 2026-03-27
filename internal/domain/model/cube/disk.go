package cube

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/goccy/go-json"
	"github.com/ycyun/Cube-API/internal/infra/utils"
)

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

	3) action(list/gfs/multipath/rbd/detail)에 따라
	   - lsblk 컬럼을 다르게 가져오고
	   - 필터링 정책 및 id/path/rbd_path 보강을 적용합니다.

	4) view=flat 일 때는 트리 구조를 평탄화하여 devices 배열로 내려줍니다.
*/

// DiskDevice는 lsblk JSON 구조와 맞추기 위한 모델입니다.
type DiskDevice struct {
	Name       string       `json:"name"`
	Kname      string       `json:"kname,omitempty"`
	Pkname     *string      `json:"pkname,omitempty"` // 내부 트리 재구성에만 쓰고, 최종 응답에서는 제거합니다.
	Path       *string      `json:"path,omitempty"`
	ID         *string      `json:"id,omitempty"`
	RbdPath    *string      `json:"rbd_path,omitempty"`
	Rota       *bool        `json:"rota,omitempty"`
	Model      *string      `json:"model,omitempty"`
	Size       *string      `json:"size,omitempty"`
	State      *string      `json:"state,omitempty"`
	Group      *string      `json:"group,omitempty"`
	Type       *string      `json:"type,omitempty"`
	Tran       *string      `json:"tran,omitempty"`
	Subsystems *string      `json:"subsystems,omitempty"`
	Vendor     *string      `json:"vendor,omitempty"`
	Wwn        *string      `json:"wwn,omitempty"`
	Children   []DiskDevice `json:"children,omitempty"`
}

// TypeBlockDevice는 내부 캐시/처리용 구조입니다.
type TypeBlockDevice struct {
	Blockdevices    []DiskDevice        `json:"blockdevices"`
	RaidControllers []map[string]string `json:"raidcontrollers,omitempty"`
	RefreshTime     time.Time           `json:"-"`
} // @name TypeBlockDevice

// DiskResponse는 “API 응답 전용 DTO”입니다.
// (TypeBlockDevice를 그대로 반환하면 내부 필드가 섞이거나 스키마가 꼬일 수 있어서 분리합니다.)
type DiskResponse struct {
	Blockdevices    []DiskDevice        `json:"blockdevices"`
	RaidControllers []map[string]string `json:"raidcontrollers,omitempty"`
	RefreshTime     string              `json:"refresh_time,omitempty"`
} // @name DiskResponse

// DiskDetailResponse는 action=detail 응답 DTO입니다.
type DiskDetailResponse struct {
	Type        string                      `json:"type"`
	Devices     map[string]DiskDetailDevice `json:"devices"`
	RefreshTime string                      `json:"refresh_time,omitempty"`
} // @name DiskDetailResponse

// DiskDetailDevice는 multipath/single 상세 정보를 담습니다.
type DiskDetailDevice struct {
	MultipathID   []string `json:"multipath_id,omitempty"`
	MultipathName []string `json:"multipath_name,omitempty"`
	SingleID      []string `json:"single_id,omitempty"`
	SingleName    []string `json:"single_name,omitempty"`
	Scsi          []string `json:"scsi,omitempty"`
	Wwn           []string `json:"wwn,omitempty"`
}

// FlatViewResponse는 view=flat 일 때 응답입니다.
type FlatViewResponse struct {
	Devices         []FlatDiskItem      `json:"devices"`
	RaidControllers []map[string]string `json:"raidcontrollers,omitempty"`
	RefreshTime     string              `json:"refresh_time,omitempty"`
} // @name FlatViewResponse

// FlatDiskItem은 트리 구조를 평탄화한 단일 행입니다.
type FlatDiskItem struct {
	Name    string  `json:"name"`
	Kname   string  `json:"kname,omitempty"`
	Path    *string `json:"path,omitempty"`
	ID      *string `json:"id,omitempty"`
	RbdPath *string `json:"rbd_path,omitempty"`
	Type    *string `json:"type,omitempty"`
	Parent  string  `json:"parent,omitempty"`
	Depth   int     `json:"depth"`
}

var lockBlockDevice sync.Once
var _BlockDevice *TypeBlockDevice

func Disk() *TypeBlockDevice {
	if _BlockDevice == nil {
		lockBlockDevice.Do(func() {
			if gin.IsDebugging() {
				fmt.Println("Creating ", reflect.TypeOf(_BlockDevice), " now.")
			}
			_BlockDevice = &TypeBlockDevice{}
		})
	} else {
		if gin.IsDebugging() {
			fmt.Println("get old ", reflect.TypeOf(_BlockDevice), " instance.")
		}
	}
	return _BlockDevice
}

// Get godoc
//
//	@Summary		Show List of Disk
//	@Description	Cube의 Disk목록을 보여줍니다. action=detail은 multipath/single 상세 정보를 반환합니다.
//	@Tags			API, CUBE
//	@Accept			x-www-form-urlencoded
//	@Produce		json
//	@Param			action	query	string	false	"disk action"	Enums(list,gfs,multipath,rbd,detail)
//	@Param			view	query	string	false	"response view"	Enums(tree,flat,list)
//	@Success		200	{object}	DiskResponse
//	@Failure		400	{object}	HTTP400BadRequest
//	@Failure		404	{object}	HTTP404NotFound
//	@Failure		500	{object}	HTTP500InternalServerError
//	@Router			/cube/disk [get]
func (d *TypeBlockDevice) Get(context *gin.Context) {
	action := normalizeDiskAction(context.DefaultQuery("action", "list"))

	if action == "detail" {
		resp, err := buildDetailResponse()
		if err != nil {
			context.JSON(http.StatusInternalServerError, utils.HTTP500InternalServerError{
				ErrCode: http.StatusInternalServerError,
				Message: "failed to build disk detail",
			})
			return
		}
		context.IndentedJSON(http.StatusOK, resp)
		return
	}

	view := strings.ToLower(context.DefaultQuery("view", "tree"))

	current := &TypeBlockDevice{}
	if err := current.UpdateWithAction(action); err != nil {
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

func (d *TypeBlockDevice) Update() {
	_ = d.UpdateWithAction("list")
}

func (d *TypeBlockDevice) UpdateWithAction(action string) error {
	if d == nil {
		return nil
	}

	action = normalizeDiskAction(action)
	if action == "detail" {
		return nil
	}

	// 1) lsblk에서 JSON 수집
	if err := d.refreshFromLsblk(action); err != nil {
		return err
	}

	// 2) pkname 기반으로 트리 재구성
	d.rebuildTreeByPkname()

	// 3) action 정책 적용(필터 + id/path/rbd_path 보강 + raidcontroller 수집)
	d.applyDiskAction(action)
	return nil
}

func (d *TypeBlockDevice) refreshFromLsblk(action string) error {
	cmd := exec.Command("lsblk", lsblkArgsForAction(action)...)
	stdout, err := cmd.CombinedOutput()
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

func lsblkArgsForAction(action string) []string {
	switch action {
	case "multipath":
		return []string{"-J", "-o", "name,kname,pkname,path,state,size,type"}
	case "rbd":
		return []string{"-J", "-o", "name,kname,pkname,path,state,size,type"}
	default:
		// list, gfs and fallback
		return []string{"-J", "-o", "name,kname,pkname,path,rota,model,size,state,group,type,tran,subsystems,vendor,wwn"}
	}
}

/*
	-------------------------------------------
	flat(view=flat) 응답 생성
	-------------------------------------------
*/

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

func strOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

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

func (d *TypeBlockDevice) rebuildTreeByPkname() {
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

func normalizeDiskAction(action string) string {
	switch action {
	case "list":
		return "list"
	case "gfs", "gfs-list":
		return "gfs"
	case "multipath", "mpath", "mpath-list":
		return "multipath"
	case "rbd", "hci-file-system-list", "hci-shared-file-list":
		return "rbd"
	case "detail":
		return "detail"
	default:
		return "list"
	}
}

// list: /dev/disk/by-path
// gfs/multipath: /dev/disk/by-id (dm-uuid 기반)
// rbd: /dev/rbd/rbd
func diskPathMap(action string) map[string]string {
	switch action {
	case "list":
		return readSymlinkMap("/dev/disk/by-path", nil)
	case "rbd":
		return readSymlinkMap("/dev/rbd/rbd", func(name string) bool {
			return strings.Contains(name, "rbd")
		})
	default:
		return readSymlinkMap("/dev/disk/by-id", func(name string) bool {
			return strings.Contains(name, "dm-uuid") && !strings.Contains(name, "LVM")
		})
	}
}

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

// children 중 type=want 인 것만 남기고 나머지는 제거합니다.
// 유지된 children이 없으면 false를 반환합니다.
func keepChildrenByType(dev *DiskDevice, want string) bool {
	if dev == nil {
		return false
	}
	if len(dev.Children) == 0 {
		return false
	}
	kept := make([]DiskDevice, 0, len(dev.Children))
	for _, child := range dev.Children {
		if strings.EqualFold(strOrEmpty(child.Type), want) {
			kept = append(kept, child)
		}
	}
	dev.Children = kept
	return len(kept) > 0
}

func (d *TypeBlockDevice) applyDiskAction(action string) {
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

		case "gfs":
			// 공통 skip 정책 적용
			if shouldSkipDevice(dev) {
				continue
			}
			// dm-uuid(by-id) 보강
			attachIDRecursive(&dev, pathMap)
			filtered = append(filtered, dev)

		case "multipath":
			// 공통 skip 정책 적용
			if shouldSkipDevice(dev) {
				continue
			}
			// children 중 mpath만 남김. 없으면 제외
			if !keepChildrenByType(&dev, "mpath") {
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

	d.Blockdevices = filtered
	d.RaidControllers = filterRaidControllers(listPCIDevices())
	d.RefreshTime = time.Now()
}

/*
	-------------------------------------------
	action=detail 처리
	-------------------------------------------
*/

func buildDetailResponse() (DiskDetailResponse, error) {
	if isMultipathActive() {
		dmDevices := getDMDevices()
		devices, err := mapLinksByDM(dmDevices)
		if err != nil {
			return DiskDetailResponse{}, err
		}
		return DiskDetailResponse{
			Type:        "multipath",
			Devices:     devices,
			RefreshTime: time.Now().Format(time.RFC3339),
		}, nil
	}

	singleDevices, err := getSingleDevices()
	if err != nil {
		return DiskDetailResponse{}, err
	}
	devices, err := mapLinksBySingle(singleDevices)
	if err != nil {
		return DiskDetailResponse{}, err
	}
	return DiskDetailResponse{
		Type:        "single",
		Devices:     devices,
		RefreshTime: time.Now().Format(time.RFC3339),
	}, nil
}

func isMultipathActive() bool {
	cmd := exec.Command("systemctl", "is-active", "multipathd")
	stdout, err := cmd.CombinedOutput()
	if err != nil && gin.IsDebugging() {
		utils.FancyHandleError(err)
	}
	return strings.TrimSpace(string(stdout)) == "active"
}

func getDMDevices() map[string]struct{} {
	cmd := exec.Command("multipath", "-l")
	stdout, err := cmd.CombinedOutput()
	if err != nil && gin.IsDebugging() {
		utils.FancyHandleError(err)
	}

	devices := map[string]struct{}{}
	for _, line := range strings.Split(string(stdout), "\n") {
		if !strings.Contains(line, "mpath") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if strings.HasPrefix(fields[2], "dm-") {
			devices[fields[2]] = struct{}{}
		}
	}
	return devices
}

func getSingleDevices() (map[string]struct{}, error) {
	cmd := exec.Command("lsblk", "-o", "NAME,WWN", "--json")
	stdout, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	var data struct {
		Blockdevices []struct {
			Name string `json:"name"`
			Wwn  string `json:"wwn"`
		} `json:"blockdevices"`
	}
	if err := json.Unmarshal(stdout, &data); err != nil {
		return nil, err
	}

	devices := map[string]struct{}{}
	for _, dev := range data.Blockdevices {
		if dev.Name == "" {
			continue
		}
		if strings.HasPrefix(dev.Name, "dm-") {
			continue
		}
		if dev.Wwn == "" {
			continue
		}
		devices[dev.Name] = struct{}{}
	}
	return devices, nil
}

func mapLinksByDM(dmDevices map[string]struct{}) (map[string]DiskDetailDevice, error) {
	byIDPath := "/dev/disk/by-id"
	entries, err := os.ReadDir(byIDPath)
	if err != nil {
		return nil, err
	}

	dmMap := map[string]DiskDetailDevice{}
	for _, entry := range entries {
		fullPath := filepath.Join(byIDPath, entry.Name())
		if entry.Type()&os.ModeSymlink == 0 {
			continue
		}

		target, err := os.Readlink(fullPath)
		if err != nil {
			continue
		}
		targetBasename := filepath.Base(target)
		if _, ok := dmDevices[targetBasename]; !ok {
			continue
		}

		dev := dmMap[targetBasename]
		switch {
		case strings.HasPrefix(entry.Name(), "dm-uuid-mpath"):
			dev.MultipathID = append(dev.MultipathID, filepath.Join(byIDPath, entry.Name()))
		case strings.HasPrefix(entry.Name(), "dm-name-mpath"):
			mpathName := strings.TrimPrefix(entry.Name(), "dm-name-")
			dev.MultipathName = append(dev.MultipathName, filepath.Join("/dev/mapper", mpathName))
		case strings.HasPrefix(entry.Name(), "scsi-"):
			dev.Scsi = append(dev.Scsi, entry.Name())
		case strings.HasPrefix(entry.Name(), "wwn-"):
			dev.Wwn = append(dev.Wwn, entry.Name())
		}
		dmMap[targetBasename] = dev
	}
	return dmMap, nil
}

func mapLinksBySingle(singleDevices map[string]struct{}) (map[string]DiskDetailDevice, error) {
	byIDPath := "/dev/disk/by-id"
	entries, err := os.ReadDir(byIDPath)
	if err != nil {
		return nil, err
	}

	singleMap := map[string]DiskDetailDevice{}
	for _, entry := range entries {
		fullPath := filepath.Join(byIDPath, entry.Name())
		if entry.Type()&os.ModeSymlink == 0 {
			continue
		}

		target, err := os.Readlink(fullPath)
		if err != nil {
			continue
		}
		targetBasename := filepath.Base(target)
		if _, ok := singleDevices[targetBasename]; !ok {
			continue
		}

		dev := singleMap[targetBasename]
		if len(dev.SingleName) == 0 {
			dev.SingleName = append(dev.SingleName, filepath.Join("/dev", targetBasename))
		}

		byIDFullPath := filepath.Join(byIDPath, entry.Name())
		switch {
		case strings.HasPrefix(entry.Name(), "scsi-"):
			dev.Scsi = append(dev.Scsi, byIDFullPath)
		case strings.HasPrefix(entry.Name(), "wwn-"):
			dev.Wwn = append(dev.Wwn, byIDFullPath)
		default:
			dev.SingleID = append(dev.SingleID, byIDFullPath)
		}
		singleMap[targetBasename] = dev
	}
	return singleMap, nil
}

/*
	-------------------------------------------
	RAID Controller 탐지(lspci)
	-------------------------------------------
*/

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

func Update() *TypeBlockDevice {
	Disk().Update()
	return _BlockDevice
}
