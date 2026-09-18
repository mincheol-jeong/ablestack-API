package cube

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ablecloud.io/ablestack-api/internal/infra/utils"
	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	"github.com/gin-gonic/gin"
)

type CCVMXMLCreateRequest = CubeModel.CCVMXMLCreateRequest
type CCVMXMLCreateResponse = CubeModel.CCVMXMLCreateResponse

const (
	ccvmXMLLocalHeader    = "X-Cube-CCVM-XML-Local"
	ccvmXMLModeHeader     = "X-Cube-CCVM-XML-Mode"
	ccvmXMLModeInstall    = "install"
	ccvmXMLModeSecret     = "secret"
	ccvmXMLRequestTimeout = 5 * time.Minute
	ccvmXMLCommandTimeout = 2 * time.Minute

	ccvmXMLTemplateName     = "ccvm-xml-template.xml"
	ccvmXMLFileName         = "ccvm.xml"
	ccvmXMLTempFileName     = "ccvm-temp.xml"
	ccvmXMLCloudInitISOPath = "/var/lib/libvirt/images/ccvm-cloudinit.iso"
	ccvmXMLSecretScript     = "virsh_secret_key.sh"
	ccvmXMLSuccessMessage   = "클라우드센터 가상머신 xml 생성 성공"
)

// CreateCCVMXML godoc
//
//	@Summary		Create CCVM XML
//	@Description	Cloud Center VM XML을 생성하고 cluster.json 대상에 배포합니다.
//	@Tags			Cube-CCVM
//	@Accept			json
//	@Produce		json
//	@Param			body	body		CubeModel.CCVMXMLCreateRequest	true	"ccvm xml create request"
//	@Success		200	{object}	CubeModel.CCVMXMLCreateResponse
//	@Failure		400	{object}	HTTP400BadRequest
//	@Failure		500	{object}	HTTP500InternalServerError
//	@Router			/cube/ccvm/xml [post]
func CreateCCVMXML(context *gin.Context) {
	var req CCVMXMLCreateRequest
	if err := context.ShouldBindJSON(&req); err != nil {
		context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
			ErrCode: http.StatusBadRequest,
			Message: "invalid request",
		})
		return
	}

	cfg, err := loadClusterConfigSection()
	if err != nil {
		context.JSON(http.StatusInternalServerError, utils.HTTP500InternalServerError{
			ErrCode: http.StatusInternalServerError,
			Message: "failed to read cluster.json",
		})
		return
	}

	if isCCVMXMLLocalRequest(context) {
		resp := runCCVMXMLLocal(context.GetHeader(ccvmXMLModeHeader), req)
		context.JSON(statusCodeFromCCVMXMLCreateResponse(resp), resp)
		return
	}

	if err := normalizeCCVMXMLCreateRequest(&req, cfg); err != nil {
		context.JSON(http.StatusBadRequest, utils.HTTP400BadRequest{
			ErrCode: http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}

	resp := runCreateCCVMXML(cfg, req)
	context.JSON(statusCodeFromCCVMXMLCreateResponse(resp), resp)
}

func normalizeCCVMXMLCreateRequest(req *CCVMXMLCreateRequest, cfg *CubeModel.ClusterConfigSection) error {
	if req == nil {
		return fmt.Errorf("request required")
	}
	if req.CPU <= 0 {
		return fmt.Errorf("cpu required")
	}
	if req.Memory <= 0 {
		return fmt.Errorf("memory required")
	}
	req.GFSMountPoint = strings.TrimSpace(req.GFSMountPoint)
	req.ManagementNetworkBridge = strings.TrimSpace(req.ManagementNetworkBridge)
	req.ServiceNetworkBridge = strings.TrimSpace(req.ServiceNetworkBridge)
	if req.ManagementNetworkBridge == "" {
		return fmt.Errorf("management_network_bridge required")
	}
	if cfg != nil && strings.EqualFold(strings.TrimSpace(cfg.Type), "ablestack-vm") && req.GFSMountPoint == "" {
		return fmt.Errorf("gfs_mount_point required")
	}
	return nil
}

func isCCVMXMLLocalRequest(context *gin.Context) bool {
	return strings.EqualFold(strings.TrimSpace(context.GetHeader(ccvmXMLLocalHeader)), "1")
}

func runCCVMXMLLocal(mode string, req CCVMXMLCreateRequest) CCVMXMLCreateResponse {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ccvmXMLModeSecret:
		if err := createCCVMSecretKeyLocal(); err != nil {
			return ccvmXMLCreateError(err.Error(), nil)
		}
		return ccvmXMLCreateOK("pcs 클러스터 secret.xml 설정 성공", "", nil)
	case ccvmXMLModeInstall:
		path, err := installCCVMXMLLocal(req.XMLContent)
		if err != nil {
			return ccvmXMLCreateError(err.Error(), nil)
		}
		return ccvmXMLCreateOK("ccvm xml installed", path, nil)
	default:
		return ccvmXMLCreateError("invalid local mode", nil)
	}
}

func runCreateCCVMXML(cfg *CubeModel.ClusterConfigSection, req CCVMXMLCreateRequest) CCVMXMLCreateResponse {
	results := make([]CubeModel.ClusterApplyResult, 0)

	if isCCVMXMLHCI(cfg) {
		secretTargets := ccvmXMLAblecubeTargets(cfg)
		if len(secretTargets) == 0 {
			return ccvmXMLCreateError("hosts[].ablecube required", nil)
		}
		secretResults := fanoutCCVMXMLLocal(secretTargets, ccvmXMLModeSecret, req)
		results = append(results, secretResults...)
		if failed := firstFailedClusterApplyResult(secretResults); failed != nil {
			return ccvmXMLCreateError(failed.Message, results)
		}
	}

	content, err := renderCCVMXML(cfg, req)
	if err != nil {
		return ccvmXMLCreateError(err.Error(), results)
	}
	req.XMLContent = content

	installTargets, installTargetField := ccvmXMLInstallTargets(cfg)
	if len(installTargets) == 0 {
		return ccvmXMLCreateError(installTargetField+" required", results)
	}
	installResults := fanoutCCVMXMLLocal(installTargets, ccvmXMLModeInstall, req)
	results = append(results, installResults...)
	if failed := firstFailedClusterApplyResult(installResults); failed != nil {
		return ccvmXMLCreateError(failed.Message, results)
	}

	xmlPath := filepath.Join(resolveAbleStackVMConfigDir("ccvm"), ccvmXMLFileName)
	return ccvmXMLCreateOK(ccvmXMLSuccessMessage, xmlPath, results)
}

func renderCCVMXML(cfg *CubeModel.ClusterConfigSection, req CCVMXMLCreateRequest) (string, error) {
	templatePath := filepath.Join(resolveAbleStackXMLTemplatePath(), ccvmXMLTemplateName)
	raw, err := os.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("read ccvm xml template: %w", err)
	}

	slots := &scvmXMLSlotAllocator{next: 20}
	bridgeNum := 0
	activeOpenVSwitch := isSCVMXMLOpenVSwitchActive()
	var out strings.Builder
	for _, line := range strings.SplitAfter(string(raw), "\n") {
		switch {
		case strings.Contains(line, "<!--memory-->"):
			line = strings.ReplaceAll(line, "<!--memory-->", strconv.Itoa(req.Memory))
		case strings.Contains(line, "<!--cpu-->"):
			line = strings.ReplaceAll(line, "<!--cpu-->", strconv.Itoa(req.CPU))
		case strings.Contains(line, "<!--ccvm_cloudinit-->"):
			line = strings.ReplaceAll(line, "<!--ccvm_cloudinit-->", ccvmXMLCloudInitDisk())
		case strings.Contains(line, "<!--ccvm_disk-->"):
			disk, err := ccvmXMLDisk(cfg, req)
			if err != nil {
				return "", err
			}
			line = strings.ReplaceAll(line, "<!--ccvm_disk-->", disk)
		case strings.Contains(line, "<!--management_network_bridge-->"):
			block := scvmXMLBridgeInterface(req.ManagementNetworkBridge, bridgeNum, slots, activeOpenVSwitch, true)
			bridgeNum++
			line = strings.ReplaceAll(line, "<!--management_network_bridge-->", block)
		case strings.Contains(line, "<!--service_network_bridge-->"):
			if strings.TrimSpace(req.ServiceNetworkBridge) == "" {
				line = ""
				break
			}
			block := scvmXMLBridgeInterface(req.ServiceNetworkBridge, bridgeNum, slots, activeOpenVSwitch, false)
			bridgeNum++
			line = strings.ReplaceAll(line, "<!--service_network_bridge-->", block)
		}
		out.WriteString(line)
	}
	return out.String(), nil
}

func ccvmXMLCloudInitDisk() string {
	return strings.Join([]string{
		"    <disk type='file' device='cdrom'>",
		"      <driver name='qemu' type='raw'/>",
		"      <source file='" + xmlAttr(ccvmXMLCloudInitISOPath) + "'/>",
		"      <target dev='sdz' bus='sata'/>",
		"      <readonly/>",
		"      <shareable/>",
		"      <address type='drive' controller='0' bus='0' target='0' unit='0'/>",
		"    </disk>",
	}, "\n")
}

func ccvmXMLDisk(cfg *CubeModel.ClusterConfigSection, req CCVMXMLCreateRequest) (string, error) {
	clusterType := ""
	if cfg != nil {
		clusterType = strings.ToLower(strings.TrimSpace(cfg.Type))
	}
	switch clusterType {
	case "ablestack-hci", "ablestack-hci-filesystem":
		return strings.Join([]string{
			"    <disk type='network' device='disk'>",
			"      <source protocol='rbd' name='rbd/ccvm'>",
			"        <host name='scvm' port='6789'/>",
			"      </source>",
			"      <driver name='qemu' type='raw' cache='writeback' io='io_uring'/>",
			"      <auth username='admin'>",
			"        <secret type='ceph' uuid='11111111-1111-1111-1111-111111111111'/>",
			"      </auth>",
			"      <target dev='vda' bus='virtio'/>",
			"    </disk>",
		}, "\n"), nil
	case "ablestack-vm":
		if strings.TrimSpace(req.GFSMountPoint) == "" {
			return "", fmt.Errorf("gfs_mount_point required")
		}
		return strings.Join([]string{
			"    <disk type='file' device='disk'>",
			"      <driver name='qemu' type='qcow2'/>",
			"      <source file='" + xmlAttr(filepath.Join(req.GFSMountPoint, "ccvm.qcow2")) + "' index='1'/>",
			"      <target dev='vda' bus='virtio'/>",
			"      <address type='pci' domain='0x0000' bus='0x04' slot='0x00' function='0x0'/>",
			"    </disk>",
		}, "\n"), nil
	case "ablestack-standalone":
		return strings.Join([]string{
			"    <disk type='file' device='disk'>",
			"      <driver name='qemu' type='qcow2'/>",
			"      <source file='/mnt/glue/ccvm.qcow2'/>",
			"      <target dev='vda' bus='virtio'/>",
			"    </disk>",
		}, "\n"), nil
	default:
		return strings.Join([]string{
			"    <disk type='file' device='disk'>",
			"      <driver name='qemu' type='qcow2'/>",
			"      <source file='/var/lib/libvirt/images/ablestack-template.qcow2'/>",
			"      <target dev='vda' bus='virtio'/>",
			"    </disk>",
		}, "\n"), nil
	}
}

func createCCVMSecretKeyLocal() error {
	script := resolveAbleStackShellFile(ccvmXMLSecretScript, filepath.Join("host", ccvmXMLSecretScript))
	if err := requireRegularFile(script, "virsh secret key script not found"); err != nil {
		return err
	}
	_, timedOut, err := runCommandOutputWithEnv("sh", ccvmXMLCommandTimeout, scvmUpdateCommandEnv(), script)
	if timedOut {
		return fmt.Errorf("virsh secret key script timed out after %s", ccvmXMLCommandTimeout)
	}
	return err
}

func installCCVMXMLLocal(content string) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", fmt.Errorf("xml_content required")
	}

	vmConfigDir := resolveAbleStackVMConfigDir("ccvm")
	xmlPath := filepath.Join(vmConfigDir, ccvmXMLFileName)
	tempPath := filepath.Join(vmConfigDir, ccvmXMLTempFileName)
	if err := os.MkdirAll(vmConfigDir, 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(tempPath, []byte(content+"\n"), 0644); err != nil {
		return "", err
	}
	if err := os.Rename(tempPath, xmlPath); err != nil {
		_ = os.Remove(tempPath)
		return "", err
	}
	_ = os.Remove(filepath.Join(vmConfigDir, "ccvm.xml.bak"))
	_ = os.Remove(tempPath + ".bak")
	if err := chmodRecursive(vmConfigDir, 0755); err != nil {
		return "", err
	}
	return xmlPath, nil
}

func fanoutCCVMXMLLocal(targets []string, mode string, req CCVMXMLCreateRequest) []CubeModel.ClusterApplyResult {
	results := make([]CubeModel.ClusterApplyResult, 0, len(targets))
	for _, target := range targets {
		result := CubeModel.ClusterApplyResult{Target: target}
		var resp CCVMXMLCreateResponse
		var err error
		if isLocalTarget(target) {
			resp = runCCVMXMLLocal(mode, req)
		} else {
			resp, err = callCCVMXMLRemote(target, mode, req)
		}
		if err != nil {
			result.Code = http.StatusInternalServerError
			result.Message = err.Error()
		} else {
			result.Code = resp.Code
			result.Message = firstNonEmpty(resp.Message, fmt.Sprint(resp.Val))
		}
		results = append(results, result)
	}
	return results
}

func callCCVMXMLRemote(target string, mode string, req CCVMXMLCreateRequest) (CCVMXMLCreateResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return CCVMXMLCreateResponse{}, err
	}
	url := fmt.Sprintf("%s/api/v1/cube/ccvm/xml", buildTargetURL(target))
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return CCVMXMLCreateResponse{}, err
	}
	attachInternalToken(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(ccvmXMLLocalHeader, "1")
	httpReq.Header.Set(ccvmXMLModeHeader, mode)

	client := &http.Client{Timeout: ccvmXMLRequestTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return CCVMXMLCreateResponse{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out CCVMXMLCreateResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return CCVMXMLCreateResponse{}, fmt.Errorf("ccvm xml remote decode failed: %s", firstNonEmpty(string(raw), err.Error()))
	}
	if out.Code == 0 {
		out.Code = resp.StatusCode
	}
	if resp.StatusCode >= 300 {
		return out, fmt.Errorf("%s", firstNonEmpty(out.Message, fmt.Sprint(out.Val), resp.Status))
	}
	return out, nil
}

func ccvmXMLAblecubeTargets(cfg *CubeModel.ClusterConfigSection) []string {
	if cfg == nil {
		return nil
	}
	targets := make([]string, 0, len(cfg.Hosts))
	for _, host := range cfg.Hosts {
		if target := strings.TrimSpace(host.Ablecube); target != "" {
			targets = append(targets, target)
		}
	}
	return dedupeHosts(targets)
}

func ccvmXMLInstallTargets(cfg *CubeModel.ClusterConfigSection) ([]string, string) {
	if cfg == nil {
		return nil, "hosts[].ablecubePn"
	}
	useAblecube := strings.EqualFold(strings.TrimSpace(cfg.Type), "ablestack-vm") &&
		!strings.EqualFold(strings.TrimSpace(cfg.StorageNetwork), "true")
	targetField := "hosts[].ablecubePn"
	if useAblecube {
		targetField = "hosts[].ablecube"
	}
	targets := make([]string, 0, len(cfg.Hosts))
	for _, host := range cfg.Hosts {
		target := host.AblecubePn
		if useAblecube {
			target = host.Ablecube
		}
		if target = strings.TrimSpace(target); target != "" {
			targets = append(targets, target)
		}
	}
	return dedupeHosts(targets), targetField
}

func isCCVMXMLHCI(cfg *CubeModel.ClusterConfigSection) bool {
	if cfg == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Type)) {
	case "ablestack-hci", "ablestack-hci-filesystem":
		return true
	default:
		return false
	}
}

func firstFailedClusterApplyResult(results []CubeModel.ClusterApplyResult) *CubeModel.ClusterApplyResult {
	for i := range results {
		if results[i].Code != http.StatusOK {
			return &results[i]
		}
	}
	return nil
}

func ccvmXMLCreateOK(message string, xmlPath string, results []CubeModel.ClusterApplyResult) CCVMXMLCreateResponse {
	return CCVMXMLCreateResponse{
		Code:    http.StatusOK,
		Val:     message,
		Message: message,
		XMLPath: xmlPath,
		Results: results,
	}
}

func ccvmXMLCreateError(message string, results []CubeModel.ClusterApplyResult) CCVMXMLCreateResponse {
	return CCVMXMLCreateResponse{
		Code:    http.StatusInternalServerError,
		Val:     message,
		Message: message,
		Results: results,
	}
}

func statusCodeFromCCVMXMLCreateResponse(resp CCVMXMLCreateResponse) int {
	if resp.Code == http.StatusOK {
		return http.StatusOK
	}
	if resp.Code == http.StatusBadRequest {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}
