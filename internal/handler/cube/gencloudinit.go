package cube

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	"gopkg.in/yaml.v3"
)

type GenCloudInitRequest = CubeModel.GenCloudInitRequest
type GenCloudInitResponse = CubeModel.GenCloudInitResponse
type GenCloudInitISOInfo = CubeModel.GenCloudInitISOInfo

const (
	genCloudInitCommandTimeout = 2 * time.Minute
	genCloudInitVolumeID       = "cidata"
)

type cloudInitSubnet map[string]any
type cloudInitNetworkDevice map[string]any

func normalizeGenCloudInitRequest(req *GenCloudInitRequest) error {
	if req == nil {
		return fmt.Errorf("request required")
	}
	req.Type = strings.ToLower(strings.TrimSpace(req.Type))
	switch req.Type {
	case "ccvm", "scvm":
	default:
		return fmt.Errorf("type must be ccvm or scvm")
	}

	req.ISOPath = strings.TrimSpace(req.ISOPath)
	req.Hostname = strings.TrimSpace(req.Hostname)
	req.PubKey = strings.TrimSpace(req.PubKey)
	req.PrivKey = strings.TrimSpace(req.PrivKey)
	req.Hosts = strings.TrimSpace(req.Hosts)
	req.MgmtNIC = strings.TrimSpace(req.MgmtNIC)
	req.MgmtIP = strings.TrimSpace(req.MgmtIP)
	req.MgmtGW = strings.TrimSpace(req.MgmtGW)
	req.DNS = strings.TrimSpace(req.DNS)
	req.SNNIC = strings.TrimSpace(req.SNNIC)
	req.SNIP = strings.TrimSpace(req.SNIP)
	req.SNGW = strings.TrimSpace(req.SNGW)
	req.SNDNS = strings.TrimSpace(req.SNDNS)
	req.PNNIC = strings.TrimSpace(req.PNNIC)
	req.PNIP = strings.TrimSpace(req.PNIP)
	req.CNNIC = strings.TrimSpace(req.CNNIC)
	req.CNIP = strings.TrimSpace(req.CNIP)

	if req.ISOPath == "" || req.Hostname == "" || req.PubKey == "" || req.PrivKey == "" || req.Hosts == "" {
		return fmt.Errorf("iso_path, hostname, pubkey, privkey and hosts are required")
	}
	if req.MgmtNIC == "" || req.MgmtIP == "" || req.MgmtPrefix <= 0 {
		return fmt.Errorf("mgmt_nic, mgmt_ip and mgmt_prefix are required")
	}
	if req.PNPrefix == 0 {
		req.PNPrefix = 24
	}
	if req.CNPrefix == 0 {
		req.CNPrefix = 24
	}
	if req.Type == "scvm" && (req.PNNIC == "" || req.PNIP == "" || req.CNNIC == "" || req.CNIP == "") {
		return fmt.Errorf("pn_nic, pn_ip, cn_nic and cn_ip are required for scvm")
	}
	if req.Type == "ccvm" && (req.SNNIC != "" || req.SNIP != "" || req.SNPrefix != 0 || req.SNGW != "" || req.SNDNS != "") {
		if req.SNNIC == "" || req.SNIP == "" || req.SNPrefix <= 0 {
			return fmt.Errorf("sn_nic, sn_ip and sn_prefix are required when ccvm service network is provided")
		}
	}
	return nil
}

func runGenCloudInit(req GenCloudInitRequest, cfg *CubeModel.ClusterConfigSection) GenCloudInitResponse {
	tmpDir, err := os.MkdirTemp("", "gencloudinit-*")
	if err != nil {
		return genCloudInitError(err.Error())
	}

	if err := writeGenCloudInitFiles(req, cfg, tmpDir); err != nil {
		return genCloudInitError(err.Error())
	}
	if err := makeGenCloudInitISO(req.ISOPath, tmpDir); err != nil {
		return genCloudInitError(err.Error())
	}
	info, err := genCloudInitISOInfo(req.ISOPath, tmpDir)
	if err != nil {
		return genCloudInitError(err.Error())
	}
	return GenCloudInitResponse{
		Code:    http.StatusOK,
		Val:     info,
		Message: "ok",
		Action:  "generate",
	}
}

func writeGenCloudInitFiles(req GenCloudInitRequest, cfg *CubeModel.ClusterConfigSection, tmpDir string) error {
	if err := writeGenCloudInitMeta(req.Hostname, tmpDir); err != nil {
		return err
	}
	userData, err := buildGenCloudInitUserData(req, cfg)
	if err != nil {
		return err
	}
	if err := writeCloudConfigYAML(filepath.Join(tmpDir, "user-data"), userData); err != nil {
		return err
	}
	networkConfig, err := buildGenCloudInitNetworkConfig(req)
	if err != nil {
		return err
	}
	return writeYAMLFile(filepath.Join(tmpDir, "network-config"), networkConfig)
}

func writeGenCloudInitMeta(hostname string, tmpDir string) error {
	meta := map[string]any{
		"instance-id":    hostname,
		"local-hostname": hostname,
	}
	return writeYAMLFile(filepath.Join(tmpDir, "meta-data"), meta)
}

func buildGenCloudInitNetworkConfig(req GenCloudInitRequest) (map[string]any, error) {
	devices := []cloudInitNetworkDevice{
		genCloudInitPhysicalDevice(req.MgmtNIC, req.MgmtIP, req.MgmtPrefix, req.MgmtGW, req.DNS, 0),
	}

	switch req.Type {
	case "ccvm":
		if req.SNNIC != "" {
			devices = append(devices, genCloudInitPhysicalDevice(req.SNNIC, req.SNIP, req.SNPrefix, req.SNGW, req.SNDNS, 0))
		}
	case "scvm":
		devices = append(devices,
			genCloudInitPhysicalDevice(req.PNNIC, req.PNIP, req.PNPrefix, "", "", 9000),
			genCloudInitPhysicalDevice(req.CNNIC, req.CNIP, req.CNPrefix, "", "", 9000),
		)
	default:
		return nil, fmt.Errorf("unsupported type")
	}

	return map[string]any{
		"network": map[string]any{
			"version": 1,
			"config":  devices,
		},
	}, nil
}

func genCloudInitPhysicalDevice(name string, ip string, prefix int, gateway string, dns string, mtu int) cloudInitNetworkDevice {
	subnet := cloudInitSubnet{
		"type":    "static",
		"address": fmt.Sprintf("%s/%d", ip, prefix),
	}
	if gateway != "" {
		subnet["gateway"] = gateway
	}
	if dns != "" {
		subnet["dns_nameservers"] = []string{dns}
	}
	device := cloudInitNetworkDevice{
		"type":    "physical",
		"name":    name,
		"subnets": []cloudInitSubnet{subnet},
	}
	if mtu > 0 {
		device["mtu"] = mtu
	}
	return device
}

func buildGenCloudInitUserData(req GenCloudInitRequest, cfg *CubeModel.ClusterConfigSection) (map[string]any, error) {
	pubKeyRaw, err := os.ReadFile(req.PubKey)
	if err != nil {
		return nil, err
	}
	privKeyRaw, err := os.ReadFile(req.PrivKey)
	if err != nil {
		return nil, err
	}
	hostsRaw, err := os.ReadFile(req.Hosts)
	if err != nil {
		return nil, err
	}
	pubKey := normalizeGenCloudInitPubKey(string(pubKeyRaw))
	privKey := string(privKeyRaw)
	hosts := string(hostsRaw)

	userData := genCloudInitBaseUserData(pubKey, privKey, hosts, isGenCloudInitHCI(cfg))
	switch req.Type {
	case "ccvm":
		if err := appendGenCloudInitCCVMFiles(userData, pubKey, privKey); err != nil {
			return nil, err
		}
	case "scvm":
		if err := appendGenCloudInitSCVMFiles(userData, isGenCloudInitHCI(cfg)); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported type")
	}
	return userData, nil
}

func genCloudInitBaseUserData(pubKey string, privKey string, hosts string, hci bool) map[string]any {
	users := []map[string]any{}
	writeFiles := []map[string]any{}
	if hci {
		users = append(users, map[string]any{
			"name":                "ceph",
			"homedir":             "/var/lib/ceph",
			"groups":              []string{"wheel"},
			"lock_passwd":         false,
			"plain_text_passwd":   "Ablecloud1!",
			"ssh_authorized_keys": []string{pubKey},
			"sudo":                "ALL=(ALL) NOPASSWD:ALL",
		})
	}
	users = append(users,
		map[string]any{
			"name":                "ablecloud",
			"groups":              []string{"wheel"},
			"lock_passwd":         false,
			"plain_text_passwd":   "Ablecloud1!",
			"ssh_authorized_keys": []string{pubKey},
			"sudo":                "ALL=(ALL) NOPASSWD:ALL",
		},
		map[string]any{
			"name":                "root",
			"lock_passwd":         false,
			"plain_text_passwd":   "Ablecloud1!",
			"ssh_authorized_keys": []string{pubKey},
		},
	)

	writeFiles = append(writeFiles,
		genCloudInitWriteFile("/root/.ssh/id_rsa.pub", "root:root", "0644", pubKey),
		genCloudInitWriteFile("/root/.ssh/id_rsa", "root:root", "0600", privKey),
	)
	if hci {
		writeFiles = append(writeFiles,
			genCloudInitWriteFile("/var/lib/ceph/.ssh/id_rsa.pub", "ceph:ceph", "0644", pubKey),
			genCloudInitWriteFile("/var/lib/ceph/.ssh/id_rsa", "ceph:ceph", "0600", privKey),
		)
	}
	writeFiles = append(writeFiles,
		genCloudInitWriteFile("/home/ablecloud/.ssh/id_rsa.pub", "ablecloud:ablecloud", "0644", pubKey),
		genCloudInitWriteFile("/home/ablecloud/.ssh/id_rsa", "ablecloud:ablecloud", "0600", privKey),
		genCloudInitWriteFile("/etc/hosts", "root:root", "0644", hosts),
	)

	return map[string]any{
		"disable_root": false,
		"ssh_pwauth":   true,
		"users":        users,
		"write_files":  writeFiles,
	}
}

func appendGenCloudInitCCVMFiles(userData map[string]any, pubKey string, privKey string) error {
	appendGenCloudInitManagementSSHFiles(userData, pubKey, privKey)
	appendGenCloudInitCCVMRunCommands(userData)
	if err := appendGenCloudInitPluginFile(userData, "/usr/local/sbin/security_patch.sh", "root:root", "0755", "shell/host/security_patch.sh", "shell/security_patch.sh"); err != nil {
		return err
	}
	if err := appendGenCloudInitPluginFile(userData, "/root/bootstrap.sh", "root:root", "0777", "shell/host/ccvm_bootstrap.sh", "shell/ccvm_bootstrap.sh"); err != nil {
		return err
	}
	return appendGenCloudInitPluginFile(userData, resolveAbleStackPropertyFile("cluster.json"), "root:root", "0600", "properties/cluster.json")
}

func appendGenCloudInitCCVMRunCommands(userData map[string]any) {
	apiPort := configuredAPIPort()
	userData["runcmd"] = [][]string{
		{"/bin/bash", "-lc", fmt.Sprintf("if command -v firewall-cmd >/dev/null 2>&1; then systemctl enable --now firewalld.service >/dev/null 2>&1 || true; firewall-cmd --permanent --add-port=%s/tcp >/dev/null 2>&1 || true; firewall-cmd --add-port=%s/tcp >/dev/null 2>&1 || true; fi", apiPort, apiPort)},
		{"/usr/bin/systemctl", "enable", "--now", "ablestack-api.service"},
	}
}

func appendGenCloudInitManagementSSHFiles(userData map[string]any, pubKey string, privKey string) {
	writeFiles, _ := userData["write_files"].([]map[string]any)
	writeFiles = append(writeFiles,
		genCloudInitWriteFile("/var/cloudstack/management/.ssh/id_rsa.pub", "cloud:cloud", "0644", pubKey),
		genCloudInitWriteFile("/var/cloudstack/management/.ssh/id_rsa", "cloud:cloud", "0600", privKey),
	)
	userData["write_files"] = writeFiles
}

func appendGenCloudInitSCVMFiles(userData map[string]any, hci bool) error {
	if hci {
		userData["bootcmd"] = [][]string{
			{"/usr/bin/systemctl", "enable", "--now", "cockpit.socket"},
		}
		if err := appendGenCloudInitPluginFile(userData, "/root/bootstrap.sh", "root:root", "0777", "shell/host/scvm_bootstrap.sh", "shell/scvm_bootstrap.sh"); err != nil {
			return err
		}
		if err := appendGenCloudInitPluginFile(userData, "/usr/local/bin/ipcorrector", "root:root", "0777", "shell/host/ipcorrector"); err != nil {
			return err
		}
		if err := appendGenCloudInitPluginFile(userData, resolveAbleStackPropertyFile("cluster.json"), "root:root", "0777", "properties/cluster.json"); err != nil {
			return err
		}
		if err := appendGenCloudInitPluginFile(userData, "/usr/local/sbin/security_patch.sh", "root:root", "0755", "shell/host/security_patch.sh", "shell/security_patch.sh"); err != nil {
			return err
		}
	}
	userData["runcmd"] = [][]string{
		{"/usr/bin/sh", "/usr/local/sbin/sortEth.sh"},
		{"/usr/bin/systemctl", "enable", "--now", "cockpit.service"},
	}
	return nil
}

func appendGenCloudInitPluginFile(userData map[string]any, targetPath string, owner string, permissions string, candidates ...string) error {
	content, err := readGenCloudInitPluginFile(candidates...)
	if err != nil {
		return err
	}
	writeFiles, _ := userData["write_files"].([]map[string]any)
	writeFiles = append(writeFiles, genCloudInitWriteFile(targetPath, owner, permissions, string(content)))
	userData["write_files"] = writeFiles
	return nil
}

func readGenCloudInitPluginFile(candidates ...string) ([]byte, error) {
	base := resolveAbleStackConfigPath()
	for _, candidate := range candidates {
		path := filepath.Join(base, candidate)
		content, err := os.ReadFile(path)
		if err == nil {
			return content, nil
		}
	}
	return nil, fmt.Errorf("config file not found: %s", strings.Join(candidates, ", "))
}

func genCloudInitWriteFile(path string, owner string, permissions string, content string) map[string]any {
	return map[string]any{
		"encoding":    "base64",
		"content":     base64.StdEncoding.EncodeToString([]byte(content)),
		"owner":       owner,
		"path":        path,
		"permissions": permissions,
	}
}

func makeGenCloudInitISO(filename string, tmpDir string) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		return err
	}
	command := "/usr/bin/genisoimage"
	if _, err := os.Stat(command); err != nil {
		command = "genisoimage"
	}
	out, timedOut, err := runCommandOutputWithEnv(
		command,
		genCloudInitCommandTimeout,
		genCloudInitCommandEnv(),
		"-output",
		filename,
		"-volid",
		genCloudInitVolumeID,
		"-joliet",
		"-input-charset",
		"utf-8",
		"-rock",
		filepath.Join(tmpDir, "user-data"),
		filepath.Join(tmpDir, "meta-data"),
		filepath.Join(tmpDir, "network-config"),
	)
	if timedOut {
		return fmt.Errorf("genisoimage timed out after %s", genCloudInitCommandTimeout)
	}
	if err != nil {
		return fmt.Errorf("genisoimage failed: %s", firstNonEmpty(strings.TrimSpace(out), err.Error()))
	}
	return nil
}

func genCloudInitISOInfo(filename string, tmpDir string) (GenCloudInitISOInfo, error) {
	info, err := os.Stat(filename)
	if err != nil {
		return GenCloudInitISOInfo{}, err
	}
	absPath, err := filepath.Abs(filename)
	if err != nil {
		return GenCloudInitISOInfo{}, err
	}
	ts := info.ModTime().Format(time.RFC3339)
	return GenCloudInitISOInfo{
		CTime:    ts,
		MTime:    ts,
		ATime:    ts,
		Size:     info.Size(),
		Filepath: absPath,
		TmpDir:   tmpDir,
	}, nil
}

func normalizeGenCloudInitPubKey(key string) string {
	key = strings.ReplaceAll(key, "\r", "")
	key = strings.ReplaceAll(key, "\n", " ")
	return strings.Join(strings.Fields(key), " ")
}

func writeYAMLFile(path string, value any) error {
	raw, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0644)
}

func writeCloudConfigYAML(path string, value any) error {
	raw, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	raw = append([]byte("#cloud-config\n"), raw...)
	return os.WriteFile(path, raw, 0644)
}

func isGenCloudInitHCI(cfg *CubeModel.ClusterConfigSection) bool {
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

func genCloudInitCommandEnv() []string {
	return append([]string{"LANG=en_US.utf-8", "LANGUAGE=en"}, os.Environ()...)
}

func genCloudInitError(message string) GenCloudInitResponse {
	return GenCloudInitResponse{
		Code:    http.StatusInternalServerError,
		Message: message,
		Action:  "generate",
	}
}

func statusCodeFromGenCloudInitResponse(resp GenCloudInitResponse) int {
	if resp.Code == http.StatusOK {
		return http.StatusOK
	}
	if resp.Code == http.StatusBadRequest {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}
