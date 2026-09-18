package cube

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"ablecloud.io/ablestack-api/internal/infra/utils"
	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	"github.com/gin-gonic/gin"
)

type CLVMManageRequest = CubeModel.CLVMManageRequest
type CLVMManageResponse = CubeModel.CLVMManageResponse
type CLVMManageDisk = CubeModel.CLVMManageDisk

const (
	clvmManageCommandTO = 5 * time.Minute
	clvmManageShortTO   = 30 * time.Second
	clvmManagePrefix    = "vg_clvm"
)

var clvmManageNumberSuffix = regexp.MustCompile(`^vg_clvm(\d+)$`)

type clvmManageVGReport struct {
	Report []struct {
		VG []struct {
			VGName string `json:"vg_name"`
		} `json:"vg"`
	} `json:"report"`
}

type clvmManagePVReport struct {
	Report []struct {
		PV []struct {
			VGName string `json:"vg_name"`
			PVName string `json:"pv_name"`
			PVSize string `json:"pv_size"`
		} `json:"pv"`
	} `json:"report"`
}

type clvmManageLSBLKPayload struct {
	Blockdevices []clvmManageBlockDevice `json:"blockdevices"`
}

type clvmManageBlockDevice struct {
	Name     string                  `json:"name"`
	Path     string                  `json:"path"`
	WWN      string                  `json:"wwn"`
	Children []clvmManageBlockDevice `json:"children,omitempty"`
}

// CLVMManage godoc
//
//	@Summary		CLVM Manage
//	@Description	CLVM 디스크를 생성, 조회, 삭제합니다. 원격 노드 반영은 SSH 대신 ablecube API fan-out을 사용합니다.
//	@Tags			Cube-CLVM
//	@Accept			json
//	@Produce		json
//	@Param			body	body		CubeModel.CLVMManageRequest	true	"clvm manage request"
//	@Success		200	{object}	CubeModel.CLVMManageResponse
//	@Failure		400	{object}	HTTP400BadRequest
//	@Failure		500	{object}	HTTP500InternalServerError
//	@Router			/cube/clvm/manage [post]
func CLVMManage(context *gin.Context) {
	var req CLVMManageRequest
	if err := context.ShouldBindJSON(&req); err != nil {
		context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
			ErrCode: http.StatusBadRequest,
			Message: "invalid request",
		})
		return
	}

	if err := normalizeCLVMManageRequest(&req); err != nil {
		context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
			ErrCode: http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}

	cfg, _ := loadClusterConfigSection()
	resp := runCLVMManage(req, cfg)
	context.JSON(statusCodeFromCLVMManageResponse(resp), resp)
}

func normalizeCLVMManageRequest(req *CLVMManageRequest) error {
	if req == nil {
		return fmt.Errorf("request required")
	}
	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "create-clvm", "create":
		req.Action = "create-clvm"
	case "list-clvm", "list":
		req.Action = "list-clvm"
	case "delete-clvm", "delete":
		req.Action = "delete-clvm"
	default:
		return fmt.Errorf("unsupported action")
	}
	req.Disk = strings.TrimSpace(req.Disk)
	req.Disks = normalizeStringSlice(append(req.Disks, splitCommaValues(req.Disk)...))
	req.VGName = strings.TrimSpace(req.VGName)
	req.VGNames = normalizeStringSlice(append(req.VGNames, splitCommaValues(req.VGName)...))
	req.PVName = strings.TrimSpace(req.PVName)
	req.PVNames = normalizeStringSlice(append(req.PVNames, splitCommaValues(req.PVName)...))

	if req.Action == "create-clvm" && len(req.Disks) == 0 {
		return fmt.Errorf("disks required")
	}
	if req.Action == "delete-clvm" && (len(req.VGNames) == 0 || len(req.PVNames) == 0) {
		return fmt.Errorf("vg_names and pv_names required")
	}
	return nil
}

func runCLVMManage(req CLVMManageRequest, cfg *CubeModel.ClusterConfigSection) CLVMManageResponse {
	switch req.Action {
	case "create-clvm":
		if err := createCLVMManageDisks(req.Disks, cfg); err != nil {
			return clvmManageError(req, "Create CLVM Disk Failure: "+err.Error())
		}
		return clvmManageOK(req, "Create CLVM Disk Success")
	case "list-clvm":
		disks, err := listCLVMManageDisks()
		if err != nil {
			return clvmManageError(req, err.Error())
		}
		return clvmManageOK(req, disks)
	case "delete-clvm":
		if err := deleteCLVMManageDisks(req.VGNames, req.PVNames, req.Disks, cfg); err != nil {
			return clvmManageError(req, err.Error())
		}
		return clvmManageOK(req, "Success to clvm delete")
	default:
		return clvmManageError(req, "unsupported action")
	}
}

func createCLVMManageDisks(disks []string, cfg *CubeModel.ClusterConfigSection) error {
	nextNum, err := nextCLVMManageNumber()
	if err != nil {
		return err
	}
	for _, disk := range disks {
		name := filepath.Base(disk)
		if _, err := execCLVMManageCommand(clvmManageCommandTO, "parted", "-s", disk, "mklabel", "gpt", "mkpart", name, "0%", "100%", "set", "1", "lvm", "on"); err != nil {
			return err
		}
		_, _ = execCLVMManageCommand(clvmManageShortTO, "partprobe", disk)

		partition, err := waitForCLVMManagePartition(disk, clvmManageShortTO, "")
		if err != nil {
			return err
		}
		vgName := fmt.Sprintf("%s%02d", clvmManagePrefix, nextNum)
		if _, err := execCLVMManageCommand(clvmManageCommandTO, "pvcreate", "-y", partition); err != nil {
			return err
		}
		if _, err := execCLVMManageCommand(clvmManageCommandTO, "vgcreate", vgName, partition); err != nil {
			return err
		}

		if err := refreshCLVMManageDevicesOnHosts([]string{disk}, cfg); err != nil {
			return err
		}
		nextNum++
	}
	return nil
}

func nextCLVMManageNumber() (int, error) {
	out, err := execCLVMManageCommand(clvmManageShortTO, "vgs", "-o", "vg_name", "--reportformat", "json")
	if err != nil {
		return 1, nil
	}
	var report clvmManageVGReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		return 0, err
	}
	maxNum := 0
	for _, section := range report.Report {
		for _, vg := range section.VG {
			matches := clvmManageNumberSuffix.FindStringSubmatch(strings.TrimSpace(vg.VGName))
			if len(matches) != 2 {
				continue
			}
			num, err := strconv.Atoi(matches[1])
			if err == nil && num > maxNum {
				maxNum = num
			}
		}
	}
	return maxNum + 1, nil
}

func waitForCLVMManagePartition(disk string, timeout time.Duration, osType string) (string, error) {
	deadline := time.Now().Add(timeout)
	candidates := gfsManagePartitionCandidates(disk, osType)
	for {
		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("partition not found for %s", disk)
		}
		time.Sleep(time.Second)
	}
}

func refreshCLVMManageDevicesOnHosts(disks []string, cfg *CubeModel.ClusterConfigSection) error {
	if cfg == nil {
		loaded, err := loadClusterConfigSection()
		if err == nil {
			cfg = loaded
		}
	}
	if cfg == nil {
		return nil
	}
	for _, action := range []string{"partprobe", "lvmdevices-add"} {
		req := GFSManageRequest{Action: action, Disks: disks}
		if err := normalizeGFSManageRequest(&req); err != nil {
			return err
		}
		resp := runGFSManageFanout(req, cfg)
		if resp.Code != http.StatusOK {
			return fmt.Errorf("%s failed: %s", action, firstNonEmpty(resp.Message, fmt.Sprint(resp.Val)))
		}
	}
	return nil
}

func listCLVMManageDisks() ([]CLVMManageDisk, error) {
	pvsOut, err := execCLVMManageCommand(clvmManageShortTO, "pvs", "-o", "vg_name,pv_name,pv_size", "--reportformat", "json")
	if err != nil {
		return nil, err
	}
	var pvs clvmManagePVReport
	if err := json.Unmarshal([]byte(pvsOut), &pvs); err != nil {
		return nil, err
	}

	wwnByName := mapCLVMManageWWNByName()
	diskIDByDM := mapCLVMManageDiskIDByDMName()
	out := make([]CLVMManageDisk, 0)
	for _, section := range pvs.Report {
		for _, pv := range section.PV {
			if !strings.Contains(pv.VGName, clvmManagePrefix) {
				continue
			}
			realPath, err := filepath.EvalSymlinks(pv.PVName)
			if err != nil {
				realPath = pv.PVName
			}
			dmName := filepath.Base(realPath)
			diskID := diskIDByDM[dmName]
			if diskID == "" {
				diskID = "N/A"
			}
			diskName := resetCloudCenterBaseDisk(filepath.Base(pv.PVName))
			wwn := firstNonEmpty(wwnByName[diskName], wwnByName[dmName], "N/A")
			uuid := firstNonEmpty(diskUUIDFromID(diskID), wwn, "N/A")
			pathMode := "single"
			if multipathUUID(diskID) != "" {
				pathMode = "multipath"
			}
			out = append(out, CLVMManageDisk{
				VGName:   pv.VGName,
				PVName:   pv.PVName,
				PVSize:   parseCLVMManageSize(pv.PVSize),
				UUID:     uuid,
				PathMode: pathMode,
				DiskID:   diskID,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return clvmManageVGNumber(out[i].VGName) < clvmManageVGNumber(out[j].VGName)
	})
	return out, nil
}

func mapCLVMManageWWNByName() map[string]string {
	out, err := execCLVMManageCommand(clvmManageShortTO, "lsblk", "-o", "name,path,wwn", "--json")
	if err != nil {
		return nil
	}
	var payload clvmManageLSBLKPayload
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		return nil
	}
	wwnByName := map[string]string{}
	var walk func(dev clvmManageBlockDevice, parentWWN string)
	walk = func(dev clvmManageBlockDevice, parentWWN string) {
		wwn := firstNonEmpty(dev.WWN, parentWWN)
		if dev.Name != "" && wwn != "" {
			wwnByName[dev.Name] = wwn
		}
		for _, child := range dev.Children {
			walk(child, wwn)
		}
	}
	for _, dev := range payload.Blockdevices {
		walk(dev, "")
	}
	return wwnByName
}

func mapCLVMManageDiskIDByDMName() map[string]string {
	entries, err := os.ReadDir("/dev/disk/by-id")
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, entry := range entries {
		fullPath := filepath.Join("/dev/disk/by-id", entry.Name())
		resolved, err := filepath.EvalSymlinks(fullPath)
		if err != nil {
			continue
		}
		base := filepath.Base(resolved)
		current := out[base]
		if current == "" || strings.HasPrefix(entry.Name(), "dm-uuid-part1-mpath") {
			out[base] = fullPath
		}
	}
	return out
}

func clvmManageVGNumber(vgName string) int {
	matches := clvmManageNumberSuffix.FindStringSubmatch(strings.TrimSpace(vgName))
	if len(matches) != 2 {
		return 0
	}
	num, _ := strconv.Atoi(matches[1])
	return num
}

func parseCLVMManageSize(size string) string {
	size = strings.TrimSpace(size)
	matches := regexp.MustCompile(`^[<>]?([\d.]+)([a-zA-Z]*)`).FindStringSubmatch(size)
	if len(matches) != 3 {
		return size
	}
	unit := map[string]string{"t": "TB", "g": "GB", "m": "MB"}[strings.ToLower(matches[2])]
	if unit == "" {
		unit = matches[2]
	}
	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return size
	}
	return fmt.Sprintf("%.2f%s", value, unit)
}

func deleteCLVMManageDisks(vgNames []string, pvNames []string, disks []string, cfg *CubeModel.ClusterConfigSection) error {
	for i, vgName := range vgNames {
		if _, err := execCLVMManageCommand(clvmManageCommandTO, "vgremove", "-y", vgName); err != nil {
			return err
		}
		if i < len(pvNames) {
			if _, err := execCLVMManageCommand(clvmManageCommandTO, "pvremove", "-ff", "--yes", pvNames[i]); err != nil {
				return err
			}
		}
	}

	probeDisks := clvmManageDeleteProbeDisks(disks, pvNames)
	for _, disk := range probeDisks {
		_, _ = execCLVMManageCommand(clvmManageShortTO, "parted", "-s", disk, "rm", "1")
		_, _ = execCLVMManageCommand(clvmManageShortTO, "partprobe", disk)
	}
	return refreshCLVMManagePartprobeOnHosts(probeDisks, cfg)
}

func clvmManageDeleteProbeDisks(disks []string, pvNames []string) []string {
	out := make([]string, 0, len(disks)+len(pvNames))
	for _, disk := range disks {
		disk = strings.TrimSpace(disk)
		if disk == "" {
			continue
		}
		disk = strings.Replace(disk, "dm-uuid-part1-mpath-", "dm-uuid-mpath-", 1)
		out = append(out, clvmManageBaseDisk(disk))
	}
	for _, pv := range pvNames {
		out = append(out, clvmManageBaseDisk(pv))
	}
	return normalizeStringSlice(out)
}

func clvmManageBaseDisk(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.Contains(path, "/dev/disk/by-id/") {
		path = strings.Replace(path, "dm-uuid-part1-mpath-", "dm-uuid-mpath-", 1)
		return regexp.MustCompile(`-part[0-9]+$`).ReplaceAllString(path, "")
	}
	return resetCloudCenterBaseDisk(path)
}

func refreshCLVMManagePartprobeOnHosts(disks []string, cfg *CubeModel.ClusterConfigSection) error {
	if len(disks) == 0 || cfg == nil {
		return nil
	}
	req := GFSManageRequest{Action: "partprobe", Disks: disks}
	if err := normalizeGFSManageRequest(&req); err != nil {
		return err
	}
	resp := runGFSManageFanout(req, cfg)
	if resp.Code != http.StatusOK {
		return fmt.Errorf("partprobe failed: %s", firstNonEmpty(resp.Message, fmt.Sprint(resp.Val)))
	}
	return nil
}

func execCLVMManageCommand(timeout time.Duration, command string, args ...string) (string, error) {
	out, timedOut, err := runCommandOutputWithEnv(command, timeout, gfsManageCommandEnv(), args...)
	if timedOut {
		return out, fmt.Errorf("%s %s timed out after %s", command, strings.Join(args, " "), timeout)
	}
	if err != nil {
		return out, fmt.Errorf("%s %s failed: %s", command, strings.Join(args, " "), firstNonEmpty(strings.TrimSpace(out), err.Error()))
	}
	return out, nil
}

func clvmManageOK(req CLVMManageRequest, val any) CLVMManageResponse {
	return CLVMManageResponse{
		Code:    http.StatusOK,
		Val:     val,
		Message: "ok",
		Action:  req.Action,
	}
}

func clvmManageError(req CLVMManageRequest, message string) CLVMManageResponse {
	return CLVMManageResponse{
		Code:    http.StatusInternalServerError,
		Val:     message,
		Message: message,
		Action:  req.Action,
	}
}

func statusCodeFromCLVMManageResponse(resp CLVMManageResponse) int {
	if resp.Code == http.StatusOK {
		return http.StatusOK
	}
	if resp.Code == http.StatusBadRequest {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}
