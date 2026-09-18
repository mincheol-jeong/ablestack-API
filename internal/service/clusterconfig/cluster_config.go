/*
Copyright (c) 2021 ABLECLOUD Co. Ltd
설명 : 클러스터 설정 파일 cluster.json의 hosts정보를 편집하는 프로그램
최초 작성일 : 2022. 08. 25

Copyright (c) 2021 ABLECLOUD Co. Ltd
설명 : 클러스터 설정 파일 cluster.json을 기준으로 /etc/hosts 파일을 세팅하는 기능
최초 작성일 : 2022. 08. 26
*/

package clusterconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
)

const (
	defaultVersion = "1.0"
	hostsFilePath  = "/etc/hosts"
)

type Args struct {
	Action             string
	HostType           string
	Type               string
	CCVMMngtIP         string
	MngtNicCIDR        string
	MngtNicGW          string
	MngtNicDNS         string
	PCSClusterList     []string
	JSONString         string
	CopyOption         string
	ExcludeHostname    string
	RemoveHostname     string
	ExternalTimeserver string
	StorageNetwork     string
	InternalToken      string
	Verbose            int
	Human              bool
}

type Result struct {
	Code int    `json:"code"`
	Val  string `json:"val"`
}

type HostParam struct {
	Index      string `json:"index"`
	Hostname   string `json:"hostname"`
	Ablecube   string `json:"ablecube"`
	ScvmMngt   string `json:"scvmMngt"`
	AblecubePn string `json:"ablecubePn"`
	Scvm       string `json:"scvm"`
	ScvmCn     string `json:"scvmCn"`
}

type ApplyRequest struct {
	Action             string            `json:"action"`
	Type               string            `json:"type"`
	CCVM               map[string]string `json:"ccvm,omitempty"`
	MngtNic            map[string]string `json:"mngtNic,omitempty"`
	CCVMMngtIP         string            `json:"ccvm_mngt_ip"`
	MngtNicCIDR        string            `json:"mngt_nic_cidr"`
	MngtNicGW          string            `json:"mngt_nic_gw"`
	MngtNicDNS         string            `json:"mngt_nic_dns"`
	PCSClusterList     []string          `json:"pcs_cluster_list"`
	Hosts              []HostParam       `json:"hosts,omitempty"`
	CopyOption         string            `json:"copy_option"`
	ExcludeHostname    string            `json:"exclude_hostname"`
	RemoveHostname     string            `json:"remove_hostname"`
	ExternalTimeserver string            `json:"external_timeserver"`
	StorageNetwork     string            `json:"storage_network"`
	LegacyIscsiStorage string            `json:"iscsi_storage,omitempty"`
	Security           map[string]string `json:"security,omitempty"`
}

// RunCLI executes the legacy CLI behavior.
func RunCLI(argv []string) {
	args, err := parseArgs(argv)
	if err != nil {
		fmt.Println(resultError(err.Error()).JSON())
		return
	}
	result := applyArgs(args)
	fmt.Println(result.JSON())
}

// ApplyLocal runs cluster_config logic inside the API process.
func ApplyLocal(action string, req CubeModel.ClusterApplyRequest) (CubeModel.ClusterApplyLocalResponse, error) {
	if err := req.Normalize(); err != nil {
		return CubeModel.ClusterApplyLocalResponse{}, err
	}
	args := Args{
		Action:             action,
		HostType:           req.Option,
		Type:               req.Type,
		CCVMMngtIP:         req.CCVMMngtIP,
		MngtNicCIDR:        req.MngtNicCIDR,
		MngtNicGW:          req.MngtNicGW,
		MngtNicDNS:         req.MngtNicDNS,
		PCSClusterList:     req.PCSClusterList,
		JSONString:         req.JSONString,
		CopyOption:         req.CopyOption,
		ExcludeHostname:    req.ExcludeHostname,
		RemoveHostname:     req.RemoveHostname,
		ExternalTimeserver: req.ExternalTimeserver,
		StorageNetwork:     req.StorageNetwork,
	}
	if req.Security != nil {
		args.InternalToken = strings.TrimSpace(req.Security.InternalToken)
	}

	result := applyArgs(args)
	return CubeModel.ClusterApplyLocalResponse{Code: result.Code, Val: result.Val}, nil
}

func parseArgs(argv []string) (Args, error) {
	if len(argv) < 2 {
		return Args{}, errors.New("action is required")
	}

	args := Args{Action: argv[1], CopyOption: "hostOnly"}
	for i := 2; i < len(argv); {
		token := argv[i]
		switch {
		case token == "-t" || token == "--type":
			val, next, err := requireValue(argv, i)
			if err != nil {
				return args, err
			}
			args.Type = val
			i = next
		case token == "-cmi" || token == "--ccvm-mngt-ip":
			val, next, err := requireValue(argv, i)
			if err != nil {
				return args, err
			}
			args.CCVMMngtIP = val
			i = next
		case token == "-mnc" || token == "--mngt-nic-cidr":
			val, next, err := requireValue(argv, i)
			if err != nil {
				return args, err
			}
			args.MngtNicCIDR = val
			i = next
		case token == "-mng" || token == "--mngt-nic-gw":
			val, next, err := requireValue(argv, i)
			if err != nil {
				return args, err
			}
			args.MngtNicGW = val
			i = next
		case token == "-mnd" || token == "--mngt-nic-dns":
			val, next, err := requireValue(argv, i)
			if err != nil {
				return args, err
			}
			args.MngtNicDNS = val
			i = next
		case token == "-pcl" || token == "--pcs-cluster-list":
			values, next, err := consumeValues(argv, i+1)
			if err != nil {
				return args, err
			}
			args.PCSClusterList = append(args.PCSClusterList, values...)
			i = next
		case token == "-js" || token == "--json-string":
			val, next, err := requireValue(argv, i)
			if err != nil {
				return args, err
			}
			args.JSONString = val
			i = next
		case token == "-co" || token == "--copy-option":
			val, next, err := requireValue(argv, i)
			if err != nil {
				return args, err
			}
			args.CopyOption = val
			i = next
		case token == "-eh" || token == "--exclude-hostname":
			val, next, err := requireValue(argv, i)
			if err != nil {
				return args, err
			}
			args.ExcludeHostname = val
			i = next
		case token == "-rh" || token == "--remove-hostname":
			val, next, err := requireValue(argv, i)
			if err != nil {
				return args, err
			}
			args.RemoveHostname = val
			i = next
		case token == "-ets" || token == "--extenal-timeserver" || token == "--external-timeserver":
			val, next, err := requireValue(argv, i)
			if err != nil {
				return args, err
			}
			args.ExternalTimeserver = val
			i = next
		case token == "-is" || token == "--storage-network" || token == "--iscsi-storage":
			val, next, err := requireValue(argv, i)
			if err != nil {
				return args, err
			}
			args.StorageNetwork = val
			i = next
		case token == "-H" || token == "--Human":
			args.Human = true
			i++
		case token == "-V" || token == "--Version":
			fmt.Printf("%s %s\n", filepath.Base(argv[0]), defaultVersion)
			os.Exit(0)
		case token == "-v" || token == "--verbose":
			args.Verbose++
			i++
		case isVerboseBundle(token):
			args.Verbose += countVerbose(token)
			i++
		default:
			return args, fmt.Errorf("unknown flag: %s", token)
		}
	}
	return args, nil
}

func applyArgs(args Args) Result {
	ensureHugePageFS()

	clusterPath := resolveClusterJSONPath()
	switch args.Action {
	case "insert":
		if reset := resetClusterConfig(clusterPath); reset.Code != 200 {
			return reset
		}
		return insert(clusterPath, args)
	case "insertScvmHost":
		return insertScvmHost(clusterPath, args)
	case "insertAllHost":
		return insertAllHost(clusterPath, args)
	case "remove":
		return removeHost(clusterPath, args)
	case "check":
		return pingCheck(clusterPath, args)
	case "reset":
		return resetClusterConfig(clusterPath)
	default:
		return resultError("invalid action")
	}
}

func requireValue(argv []string, index int) (string, int, error) {
	if index+1 >= len(argv) {
		return "", index + 1, fmt.Errorf("missing value for %s", argv[index])
	}
	return argv[index+1], index + 2, nil
}

func consumeValues(argv []string, index int) ([]string, int, error) {
	if index >= len(argv) {
		return nil, index, fmt.Errorf("missing values for %s", argv[index-1])
	}
	values := []string{}
	i := index
	for i < len(argv) && !isFlag(argv[i]) {
		values = append(values, argv[i])
		i++
	}
	if len(values) == 0 {
		return nil, i, fmt.Errorf("missing values for %s", argv[index-1])
	}
	return values, i, nil
}

func isFlag(token string) bool {
	return strings.HasPrefix(token, "-")
}

func isVerboseBundle(token string) bool {
	if !strings.HasPrefix(token, "-") || len(token) < 3 {
		return false
	}
	for _, r := range token[1:] {
		if r != 'v' {
			return false
		}
	}
	return true
}

func countVerbose(token string) int {
	count := 0
	for _, r := range token {
		if r == 'v' {
			count++
		}
	}
	return count
}

func resultOK(message string) Result {
	return Result{Code: 200, Val: message}
}

func resultError(message string) Result {
	return Result{Code: 500, Val: message}
}

func (r Result) JSON() string {
	raw, err := json.Marshal(r)
	if err != nil {
		return fmt.Sprintf(`{"code":500,"val":"%s"}`, err.Error())
	}
	return string(raw)
}

func resolveClusterJSONPath() string {
	if env := strings.TrimSpace(os.Getenv("ABLESTACK_CLUSTER_JSON")); env != "" {
		return env
	}
	return filepath.Join(resolveConfigPath(), "properties", "cluster.json")
}

func resolveConfigPath() string {
	if env := strings.TrimSpace(os.Getenv("ABLESTACK_CONFIG_PATH")); env != "" {
		return env
	}
	return "/etc/ablestack"
}

func loadClusterJSON(path string) (map[string]any, map[string]any, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	root := map[string]any{}
	if err := json.Unmarshal(content, &root); err != nil {
		return nil, nil, err
	}
	clusterCfg := ensureMap(root, "clusterConfig")
	return root, clusterCfg, nil
}

func saveClusterJSON(path string, root map[string]any) error {
	root = NormalizeClusterJSON(root)
	raw, err := json.MarshalIndent(root, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0644)
}

func ensureMap(parent map[string]any, key string) map[string]any {
	if parent == nil {
		return map[string]any{}
	}
	if val, ok := parent[key].(map[string]any); ok {
		return val
	}
	newMap := map[string]any{}
	parent[key] = newMap
	return newMap
}

type orderedClusterConfig struct {
	Type               string         `json:"type"`
	HostType           string         `json:"hostType,omitempty"`
	BackupPath         string         `json:"backup_path"`
	CCVM               orderedCCVM    `json:"ccvm"`
	MngtNic            orderedMngtNic `json:"mngtNic"`
	PCSCluster         map[string]any `json:"pcsCluster"`
	GFS                orderedGFS     `json:"gfs"`
	Hosts              []any          `json:"hosts"`
	ExternalTimeserver string         `json:"external_timeserver"`
	StorageNetwork     string         `json:"storage_network"`
}

type orderedGFS struct {
	JournalSizeMB            int    `json:"journal_size_mb"`
	JournalSizeDescription   string `json:"journal_size_description"`
	ResourceGroupSizeMB      int    `json:"resource_group_size_mb"`
	ResourceGroupDescription string `json:"resource_group_size_description"`
}

type orderedSystemProfile struct {
	Bootstrap     orderedBootstrap     `json:"bootstrap"`
	License       orderedLicense       `json:"license"`
	SecurityPatch orderedSecurityPatch `json:"security_patch"`
}

type orderedBootstrap struct {
	Scvm           string `json:"scvm"`
	Ccvm           string `json:"ccvm"`
	Wall           string `json:"wall"`
	GFSConfigure   string `json:"gfs_configure"`
	LocalConfigure string `json:"local_configure"`
}

type orderedLicense struct {
	Status string `json:"status"`
	Type   string `json:"type"`
}

type orderedSecurityPatch struct {
	Status string `json:"status"`
}

type orderedCCVM struct {
	IP string `json:"ip"`
}

type orderedMngtNic struct {
	CIDR string `json:"cidr"`
	GW   string `json:"gw"`
	DNS  string `json:"dns"`
}

type orderedHostVM struct {
	Index    string `json:"index"`
	Hostname string `json:"hostname"`
	Ablecube string `json:"ablecube"`
}

type orderedHostHCI struct {
	Index      string `json:"index"`
	Hostname   string `json:"hostname"`
	Ablecube   string `json:"ablecube"`
	AblecubePn string `json:"ablecubePn"`
	ScvmMngt   string `json:"scvmMngt"`
	Scvm       string `json:"scvm"`
	ScvmCn     string `json:"scvmCn"`
}

// NormalizeClusterJSON enforces the output shape and key order for clusterConfig.
func NormalizeClusterJSON(root map[string]any) map[string]any {
	if root == nil {
		return root
	}
	cfg, ok := root["clusterConfig"].(map[string]any)
	if !ok {
		return root
	}

	ccvm := map[string]any{}
	if raw, ok := cfg["ccvm"].(map[string]any); ok {
		ccvm = raw
	}
	mngtNic := map[string]any{}
	if raw, ok := cfg["mngtNic"].(map[string]any); ok {
		mngtNic = raw
	}
	if val := getString(ccvm["cidr"]); getString(mngtNic["cidr"]) == "" && val != "" {
		mngtNic["cidr"] = val
	}
	if val := getString(ccvm["gw"]); getString(mngtNic["gw"]) == "" && val != "" {
		mngtNic["gw"] = val
	}
	if val := getString(ccvm["dns"]); getString(mngtNic["dns"]) == "" && val != "" {
		mngtNic["dns"] = val
	}
	pcsCluster := ensureMap(cfg, "pcsCluster")
	normalizePCSClusterMap(pcsCluster)
	gfs := ensureMap(cfg, "gfs")
	journalSizeMB := normalizeGFSSize(gfs["journal_size_mb"], CubeModel.GFSDefaultJournalSizeMB)
	resourceGroupSizeMB := normalizeGFSSize(gfs["resource_group_size_mb"], CubeModel.GFSDefaultResourceGroupSizeMB)

	externalTimeserver := getString(cfg["external_timeserver"])
	if externalTimeserver == "" {
		externalTimeserver = getString(cfg["extenal_timeserver"])
		if externalTimeserver != "" {
			cfg["external_timeserver"] = externalTimeserver
			delete(cfg, "extenal_timeserver")
		}
	}
	storageNetwork := getString(cfg["storage_network"])
	if storageNetwork == "" {
		storageNetwork = getString(cfg["iscsi_storage"])
	}
	ordered := orderedClusterConfig{
		Type:       getString(cfg["type"]),
		HostType:   getString(cfg["hostType"]),
		BackupPath: getString(cfg["backup_path"]),
		CCVM:       orderedCCVM{IP: getString(ccvm["ip"])},
		MngtNic:    orderedMngtNic{CIDR: getString(mngtNic["cidr"]), GW: getString(mngtNic["gw"]), DNS: getString(mngtNic["dns"])},
		PCSCluster: pcsCluster,
		GFS: orderedGFS{
			JournalSizeMB:            journalSizeMB,
			JournalSizeDescription:   CubeModel.GFSJournalSizeDescription,
			ResourceGroupSizeMB:      resourceGroupSizeMB,
			ResourceGroupDescription: CubeModel.GFSResourceGroupDescription,
		},
		Hosts:              buildOrderedHosts(cfg),
		ExternalTimeserver: externalTimeserver,
		StorageNetwork:     normalizeStorageNetwork(storageNetwork),
	}

	root["clusterConfig"] = ordered
	normalizeSystemProfile(root)
	normalizeSecurity(root)
	return root
}

func normalizeGFSSize(value any, fallback int) int {
	normalized, err := strconv.Atoi(strings.TrimSpace(getString(value)))
	if err != nil || normalized <= 0 {
		return fallback
	}
	return normalized
}

func normalizeStorageNetwork(val any) string {
	if strings.EqualFold(strings.TrimSpace(getString(val)), "true") {
		return "true"
	}
	return "false"
}

func normalizeSystemProfile(root map[string]any) {
	if root == nil {
		return
	}
	systemProfile := ensureMap(root, "systemProfile")
	if legacySecurity, ok := systemProfile["security"].(map[string]any); ok {
		security := ensureMap(root, "security")
		if getString(security["internal_token"]) == "" {
			security["internal_token"] = getString(legacySecurity["internal_token"])
		}
		delete(systemProfile, "security")
	}

	if bootstrap, ok := root["bootstrap"].(map[string]any); ok {
		if _, exists := systemProfile["bootstrap"]; !exists {
			systemProfile["bootstrap"] = bootstrap
		}
		delete(root, "bootstrap")
	}
	if license, ok := root["license"].(map[string]any); ok {
		if _, exists := systemProfile["license"]; !exists {
			systemProfile["license"] = license
		}
		delete(root, "license")
	}
	if securityPatch, ok := root["security_patch"].(map[string]any); ok {
		if _, exists := systemProfile["security_patch"]; !exists {
			systemProfile["security_patch"] = securityPatch
		}
		delete(root, "security_patch")
	}
	if monitoring, ok := root["monitoring"].(map[string]any); ok {
		if wall := getString(monitoring["wall"]); wall != "" {
			bootstrap := ensureMap(systemProfile, "bootstrap")
			if getString(bootstrap["wall"]) == "" {
				bootstrap["wall"] = wall
			}
		}
		delete(root, "monitoring")
	}

	bootstrap := ensureMap(systemProfile, "bootstrap")
	delete(bootstrap, "pfmp")
	if monitoring, ok := systemProfile["monitoring"].(map[string]any); ok {
		if wall := getString(monitoring["wall"]); wall != "" {
			if getString(bootstrap["wall"]) == "" {
				bootstrap["wall"] = wall
			}
		}
		delete(systemProfile, "monitoring")
	}
	bootstrap["scvm"] = normalizeBoolString(bootstrap["scvm"], "false")
	bootstrap["ccvm"] = normalizeBoolString(bootstrap["ccvm"], "false")
	bootstrap["wall"] = normalizeBoolString(bootstrap["wall"], "false")
	bootstrap["gfs_configure"] = normalizeBoolString(bootstrap["gfs_configure"], "false")
	bootstrap["local_configure"] = normalizeBoolString(bootstrap["local_configure"], "false")

	license := ensureMap(systemProfile, "license")
	license["status"] = normalizeBoolString(license["status"], "false")
	if _, ok := license["type"]; !ok {
		license["type"] = ""
	}

	securityPatch := ensureMap(systemProfile, "security_patch")
	securityPatch["status"] = normalizeBoolString(securityPatch["status"], "false")

	root["systemProfile"] = orderedSystemProfile{
		Bootstrap: orderedBootstrap{
			Scvm:           getString(bootstrap["scvm"]),
			Ccvm:           getString(bootstrap["ccvm"]),
			Wall:           getString(bootstrap["wall"]),
			GFSConfigure:   getString(bootstrap["gfs_configure"]),
			LocalConfigure: getString(bootstrap["local_configure"]),
		},
		License: orderedLicense{
			Status: getString(license["status"]),
			Type:   getString(license["type"]),
		},
		SecurityPatch: orderedSecurityPatch{
			Status: getString(securityPatch["status"]),
		},
	}
}

func normalizeSecurity(root map[string]any) {
	if root == nil {
		return
	}
	security := ensureMap(root, "security")
	if systemProfile, ok := root["systemProfile"].(map[string]any); ok {
		if legacySecurity, ok := systemProfile["security"].(map[string]any); ok {
			if getString(security["internal_token"]) == "" {
				security["internal_token"] = getString(legacySecurity["internal_token"])
			}
			delete(systemProfile, "security")
		}
	}
	if _, ok := security["internal_token"]; !ok {
		security["internal_token"] = ""
	}
}

func normalizeBoolString(value any, fallback string) string {
	normalized := strings.ToLower(strings.TrimSpace(getString(value)))
	switch normalized {
	case "true", "false":
		return normalized
	case "":
		return fallback
	default:
		return getString(value)
	}
}

func buildOrderedHosts(cfg map[string]any) []any {
	hosts := getHosts(cfg)
	if len(hosts) == 0 {
		return []any{}
	}

	clusterType := strings.ToLower(strings.TrimSpace(getString(cfg["type"])))
	isHCI := clusterType == "ablestack-hci" || clusterType == "ablestack-hci-filesystem"

	ordered := make([]any, 0, len(hosts))
	for _, host := range hosts {
		if isHCI {
			ordered = append(ordered, orderedHostHCI{
				Index:      getString(host["index"]),
				Hostname:   getString(host["hostname"]),
				Ablecube:   getString(host["ablecube"]),
				AblecubePn: getString(host["ablecubePn"]),
				ScvmMngt:   getString(host["scvmMngt"]),
				Scvm:       getString(host["scvm"]),
				ScvmCn:     getString(host["scvmCn"]),
			})
			continue
		}
		ordered = append(ordered, orderedHostVM{
			Index:    getString(host["index"]),
			Hostname: getString(host["hostname"]),
			Ablecube: getString(host["ablecube"]),
		})
	}
	return ordered
}

func getString(val any) string {
	switch v := val.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return fmt.Sprintf("%v", v)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case fmt.Stringer:
		return v.String()
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprintf("%v", v)
	}
}

func parseHostParams(jsonStr string) ([]HostParam, error) {
	if strings.TrimSpace(jsonStr) == "" {
		return nil, nil
	}
	raw := []map[string]any{}
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, err
	}
	params := make([]HostParam, 0, len(raw))
	for _, item := range raw {
		params = append(params, HostParam{
			Index:      getString(item["index"]),
			Hostname:   getString(item["hostname"]),
			Ablecube:   getString(item["ablecube"]),
			ScvmMngt:   getString(item["scvmMngt"]),
			AblecubePn: getString(item["ablecubePn"]),
			Scvm:       getString(item["scvm"]),
			ScvmCn:     getString(item["scvmCn"]),
		})
	}
	return params, nil
}

func getHosts(cfg map[string]any) []map[string]any {
	raw, ok := cfg["hosts"].([]any)
	if !ok {
		return []map[string]any{}
	}
	hosts := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if host, ok := item.(map[string]any); ok {
			hosts = append(hosts, host)
		}
	}
	return hosts
}

func setHosts(cfg map[string]any, hosts []map[string]any) {
	raw := make([]any, 0, len(hosts))
	for _, host := range hosts {
		raw = append(raw, host)
	}
	cfg["hosts"] = raw
}

func renumberHostIndexes(hosts []map[string]any) {
	for i, host := range hosts {
		if host == nil {
			continue
		}
		host["index"] = strconv.Itoa(i + 1)
	}
}

func updatePCSCluster(cfg map[string]any, pcsList []string) {
	if len(pcsList) == 0 {
		return
	}
	pcsCluster := ensureMap(cfg, "pcsCluster")
	writePCSClusterValues(pcsCluster, pcsList)
}

func normalizePCSClusterMap(pcsCluster map[string]any) {
	writePCSClusterValues(pcsCluster, collectPCSClusterValues(pcsCluster))
}

func collectPCSClusterValues(pcsCluster map[string]any) []string {
	if pcsCluster == nil {
		return nil
	}
	values := make([]string, 0, CubeModel.PCSClusterMaxHosts)
	if raw, ok := pcsCluster["hostnames"]; ok {
		values = append(values, pcsStringSliceFromAny(raw)...)
	}
	for i := 1; i <= CubeModel.PCSClusterMaxHosts; i++ {
		if value, ok := pcsCluster[fmt.Sprintf("hostname%d", i)]; ok {
			values = append(values, getString(value))
		}
	}
	return CubeModel.NormalizePCSClusterList(values)
}

func writePCSClusterValues(pcsCluster map[string]any, values []string) {
	if pcsCluster == nil {
		return
	}
	clearPCSClusterValues(pcsCluster)
	hostnames := CubeModel.NormalizePCSClusterList(values)
	for i, value := range hostnames {
		pcsCluster[fmt.Sprintf("hostname%d", i+1)] = value
	}
	for i := len(hostnames) + 1; i <= 3; i++ {
		pcsCluster[fmt.Sprintf("hostname%d", i)] = ""
	}
	delete(pcsCluster, "hostnames")
}

func clearPCSClusterValues(pcsCluster map[string]any) {
	for key := range pcsCluster {
		if isPCSClusterHostnameKey(key) {
			delete(pcsCluster, key)
		}
	}
}

func isPCSClusterHostnameKey(key string) bool {
	if !strings.HasPrefix(key, "hostname") {
		return false
	}
	index, err := strconv.Atoi(strings.TrimPrefix(key, "hostname"))
	return err == nil && index > 0
}

func pcsStringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, getString(item))
		}
		return out
	default:
		return nil
	}
}

func filterRemovedPCSClusterValues(values []string, host map[string]any) []string {
	if len(values) == 0 || host == nil {
		return values
	}
	removeValues := map[string]struct{}{}
	for _, key := range []string{"hostname", "ablecube", "ablecubePn", "scvmMngt", "scvm", "scvmCn"} {
		value := strings.ToLower(strings.TrimSpace(getString(host[key])))
		if value != "" {
			removeValues[value] = struct{}{}
		}
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, remove := removeValues[strings.ToLower(strings.TrimSpace(value))]; remove {
			continue
		}
		out = append(out, value)
	}
	return out
}

func validatePCSClusterInput(args Args) error {
	return CubeModel.ValidatePCSClusterList(args.PCSClusterList, minPCSClusterHostsForType(args.Type))
}

func minPCSClusterHostsForType(clusterType string) int {
	if strings.EqualFold(strings.TrimSpace(clusterType), "ablestack-standalone") {
		return 0
	}
	if isHCIClusterType(clusterType) {
		return 3
	}
	return 1
}

func updateMngtNic(cfg map[string]any, args Args) {
	ccvm := ensureMap(cfg, "ccvm")
	mngtNic := ensureMap(cfg, "mngtNic")
	if args.MngtNicCIDR != "" {
		mngtNic["cidr"] = args.MngtNicCIDR
	}
	if args.MngtNicGW != "" {
		mngtNic["gw"] = args.MngtNicGW
	}
	if args.MngtNicDNS != "" {
		mngtNic["dns"] = args.MngtNicDNS
	}
	delete(ccvm, "cidr")
	delete(ccvm, "gw")
	delete(ccvm, "dns")
}

func insert(clusterPath string, args Args) Result {
	if err := applyNetworkFilter(); err != nil {
		return resultError(err.Error())
	}
	if len(args.PCSClusterList) > 0 {
		if err := validatePCSClusterInput(args); err != nil {
			return resultError(err.Error())
		}
	}

	root, cfg, err := loadClusterJSON(clusterPath)
	if err != nil {
		return resultError("cluster.json read error")
	}
	if strings.TrimSpace(args.InternalToken) != "" {
		security := ensureMap(root, "security")
		security["internal_token"] = strings.TrimSpace(args.InternalToken)
	}

	if args.Type != "" {
		cfg["type"] = args.Type
	}
	if strings.EqualFold(args.HostType, "add") {
		cfg["hostType"] = "add"
	} else if strings.TrimSpace(args.HostType) != "" {
		cfg["hostType"] = "new"
	}

	if args.CCVMMngtIP != "" {
		ccvm := ensureMap(cfg, "ccvm")
		ccvm["ip"] = args.CCVMMngtIP
	}

	if args.ExternalTimeserver != "" {
		cfg["external_timeserver"] = args.ExternalTimeserver
	}

	updatePCSCluster(cfg, args.PCSClusterList)
	updateMngtNic(cfg, args)

	if strings.EqualFold(args.Type, "ablestack-vm") || strings.EqualFold(args.Type, "ablestack-standalone") {
		if args.JSONString != "" {
			params, err := parseHostParams(args.JSONString)
			if err != nil {
				return resultError("invalid json string")
			}
			hosts := getHosts(cfg)
			for _, param := range params {
				matched := false
				for _, host := range hosts {
					if getString(host["hostname"]) == param.Hostname {
						host["index"] = param.Index
						host["hostname"] = param.Hostname
						host["ablecube"] = param.Ablecube
						if args.StorageNetwork != "" && strings.EqualFold(args.StorageNetwork, "true") {
							host["ablecubePn"] = param.AblecubePn
						}
						matched = true
						break
					}
				}
				if args.StorageNetwork != "" {
					if !matched {
						newHost := map[string]any{
							"index":    param.Index,
							"hostname": param.Hostname,
							"ablecube": param.Ablecube,
						}
						if strings.EqualFold(args.StorageNetwork, "true") {
							newHost["ablecubePn"] = param.AblecubePn
						}
						hosts = append(hosts, newHost)
					}
				}
			}
			setHosts(cfg, hosts)
		}
		if args.StorageNetwork != "" {
			cfg["storage_network"] = args.StorageNetwork
		}
	} else {
		if args.JSONString != "" {
			params, err := parseHostParams(args.JSONString)
			if err != nil {
				return resultError("invalid json string")
			}
			hosts := getHosts(cfg)
			for _, param := range params {
				matched := false
				for _, host := range hosts {
					if getString(host["hostname"]) == param.Hostname {
						host["index"] = param.Index
						host["hostname"] = param.Hostname
						host["ablecube"] = param.Ablecube
						host["scvmMngt"] = param.ScvmMngt
						host["ablecubePn"] = param.AblecubePn
						host["scvm"] = param.Scvm
						host["scvmCn"] = param.ScvmCn
						matched = true
						break
					}
				}
				if !matched {
					hosts = append(hosts, map[string]any{
						"index":      param.Index,
						"hostname":   param.Hostname,
						"ablecube":   param.Ablecube,
						"scvmMngt":   param.ScvmMngt,
						"ablecubePn": param.AblecubePn,
						"scvm":       param.Scvm,
						"scvmCn":     param.ScvmCn,
					})
				}
			}
			setHosts(cfg, hosts)
		}
	}

	if err := saveClusterJSON(clusterPath, root); err != nil {
		return resultError(err.Error())
	}

	if strings.EqualFold(args.Type, "ablestack-vm") {
		_ = runCommandQuiet("systemctl", "enable", "--now", "auto-umount.timer")
	} else if strings.EqualFold(args.Type, "ablestack-hci") {
		_ = runCommandQuiet("systemctl", "enable", "--now", "cleanup-rbd.timer")
	}

	result := retryHostsApply(clusterPath, args, args.CopyOption)
	if result.Code != 200 {
		return result
	}
	return resultOK("Cluster Config insert Success")
}

func insertScvmHost(clusterPath string, args Args) Result {
	if args.JSONString == "" {
		return resultError("json string required")
	}
	if args.CCVMMngtIP == "" {
		return resultError("ccvm mngt ip required")
	}
	if err := validatePCSClusterInput(args); err != nil {
		return resultError(err.Error())
	}

	params, err := parseHostParams(args.JSONString)
	if err != nil {
		return resultError("invalid json string")
	}

	targets := make(map[string]struct{})
	for _, param := range params {
		if param.Ablecube != "" {
			targets[param.Ablecube] = struct{}{}
		}
		if param.ScvmMngt != "" {
			targets[param.ScvmMngt] = struct{}{}
		}
	}
	for target := range targets {
		ret, err := callApplyLocalAPI(target, "insert", args)
		if err != nil || ret.Code != 200 {
			return resultError("insertScvmHost Failed to modify cluster_config and hosts file. : " + target)
		}
	}

	return resultOK("Cluster Config insertScvmHost Success")
}

func insertAllHost(clusterPath string, args Args) Result {
	if args.JSONString == "" {
		return resultError("json string required")
	}
	if args.CCVMMngtIP == "" {
		return resultError("ccvm mngt ip required")
	}
	if err := validatePCSClusterInput(args); err != nil {
		return resultError(err.Error())
	}
	if !strings.EqualFold(args.Type, "ablestack-hci") && args.StorageNetwork == "" {
		return resultError("storage network required")
	}

	params, err := parseHostParams(args.JSONString)
	if err != nil {
		return resultError("invalid json string")
	}

	_ = runCommandQuiet("touch", "/var/lib/libvirt/images/ccvm-cloudinit.iso")
	_ = runCommandQuiet("chmod", "777", "/var/lib/libvirt/images/ccvm-cloudinit.iso")

	targets := make(map[string]struct{})
	for _, param := range params {
		if param.Ablecube != "" {
			targets[param.Ablecube] = struct{}{}
		}
		if strings.EqualFold(args.Type, "ablestack-hci") && args.ExcludeHostname != param.Hostname {
			if param.ScvmMngt != "" {
				targets[param.ScvmMngt] = struct{}{}
			}
		}
	}
	if args.CCVMMngtIP != "" {
		targets[args.CCVMMngtIP] = struct{}{}
	}

	for target := range targets {
		ret, err := callApplyLocalAPI(target, "insert", args)
		if err != nil || ret.Code != 200 {
			return resultError("insertAllHost Failed to modify cluster_config and hosts file. : " + target)
		}
	}

	return resultOK("Cluster Config insertAllHost Success")
}

func removeHost(clusterPath string, args Args) Result {
	if args.RemoveHostname == "" {
		return resultError("target hostname required")
	}

	root, cfg, err := loadClusterJSON(clusterPath)
	if err != nil {
		return resultError("cluster.json read error")
	}

	hosts := getHosts(cfg)
	found := -1
	var target map[string]any
	for i, host := range hosts {
		if getString(host["hostname"]) == args.RemoveHostname {
			found = i
			target = host
			break
		}
	}

	if found >= 0 {
		pcs := ensureMap(cfg, "pcsCluster")
		targetIP := getString(target["ablecube"])
		filteredPCS := filterRemovedPCSClusterValues(collectPCSClusterValues(pcs), target)
		writePCSClusterValues(pcs, filteredPCS)

		if isHCIClusterType(args.Type) {
			targetIndex := getString(target["index"])
			if targetIndex == "" {
				targetIndex = strconv.Itoa(found + 1)
			}
			removeIPs := map[string]bool{
				getString(target["ablecube"]):   true,
				getString(target["scvmMngt"]):   true,
				getString(target["ablecubePn"]): true,
				getString(target["scvm"]):       true,
				getString(target["scvmCn"]):     true,
			}
			removeNames := map[string]bool{
				getString(target["hostname"]):    true,
				"scvm" + targetIndex + "-mngt":   true,
				"pn-ablecube" + targetIndex:      true,
				"ablecube" + targetIndex + "-pn": true,
				"scvm" + targetIndex:             true,
				"cn-scvm" + targetIndex:          true,
				"scvm" + targetIndex + "-cn":     true,
			}

			if err := updateHostsFile(removeIPs, removeNames, nil); err != nil {
				return resultError(err.Error())
			}

			hosts = append(hosts[:found], hosts[found+1:]...)
			renumberHostIndexes(hosts)
			setHosts(cfg, hosts)

			if err := saveClusterJSON(clusterPath, root); err != nil {
				return resultError(err.Error())
			}

			result := retryHostsApply(clusterPath, args, args.CopyOption)
			if result.Code != 200 {
				return result
			}
			return resultOK("Cluster Config remove Success")
		}

		filtered := make([]map[string]any, 0, len(hosts))
		for _, host := range hosts {
			if getString(host["hostname"]) != args.RemoveHostname {
				filtered = append(filtered, host)
			}
		}
		renumberHostIndexes(filtered)
		setHosts(cfg, filtered)

		if err := saveClusterJSON(clusterPath, root); err != nil {
			return resultError(err.Error())
		}

		if targetIP != "" {
			if err := updateHostsFile(map[string]bool{targetIP: true}, nil, nil); err != nil {
				return resultError(err.Error())
			}
		}
		return resultOK("Cluster Config remove Success")
	}

	if err := saveClusterJSON(clusterPath, root); err != nil {
		return resultError(err.Error())
	}
	if isHCIClusterType(args.Type) {
		result := retryHostsApply(clusterPath, args, args.CopyOption)
		if result.Code != 200 {
			return result
		}
	}
	return resultOK("Cluster Config remove Success")
}

func isHCIClusterType(clusterType string) bool {
	switch strings.ToLower(strings.TrimSpace(clusterType)) {
	case "ablestack-hci", "ablestack-hci-filesystem":
		return true
	default:
		return false
	}
}

func pingCheck(clusterPath string, args Args) Result {
	if args.JSONString == "" {
		return resultError("json string required")
	}
	params, err := parseHostParams(args.JSONString)
	if err != nil {
		return resultError("invalid json string")
	}

	for _, param := range params {
		if err := checkAPITarget(param.Ablecube); err != nil {
			return resultError("The ping test failed. Check ablecube hosts network IPs. : " + param.Ablecube)
		}
	}

	return resultOK("Cluster Config PingCheck Success")
}

func resetClusterConfig(clusterPath string) Result {
	root, cfg, err := loadClusterJSON(clusterPath)
	if err != nil {
		return resultError("cluster.json read error")
	}

	cfg["type"] = ""
	cfg["backup_path"] = ""
	ccvm := ensureMap(cfg, "ccvm")
	ccvm["ip"] = ""
	delete(ccvm, "cidr")
	delete(ccvm, "gw")
	delete(ccvm, "dns")
	mngtNic := ensureMap(cfg, "mngtNic")
	mngtNic["cidr"] = ""
	mngtNic["gw"] = ""
	mngtNic["dns"] = ""

	cfg["external_timeserver"] = ""
	delete(cfg, "extenal_timeserver")

	pcs := ensureMap(cfg, "pcsCluster")
	writePCSClusterValues(pcs, nil)

	cfg["hosts"] = []any{}
	cfg["storage_network"] = "false"

	if err := saveClusterJSON(clusterPath, root); err != nil {
		return resultError(err.Error())
	}
	return resultOK("Cluster Config reset Success")
}

func runHostAction(clusterPath string, args Args, action string) Result {
	_, cfg, err := loadClusterJSON(clusterPath)
	if err != nil {
		return resultError("cluster.json read error")
	}
	return applyHosts(action, args, cfg)
}

func applyHosts(action string, args Args, cfg map[string]any) Result {
	switch action {
	case "hostOnly":
		return changeHosts(args, cfg)
	default:
		return resultError("invalid hosts action")
	}
}

func retryHostsApply(clusterPath string, args Args, copyOption string) Result {
	var result Result
	for i := 0; i < 3; i++ {
		result = runHostAction(clusterPath, args, copyOption)
		if result.Code == 200 {
			return result
		}
	}
	return result
}

func changeHosts(args Args, cfg map[string]any) Result {
	lines, err := readHostsLines()
	if err != nil {
		return resultError(err.Error())
	}

	if strings.EqualFold(args.Type, "ablestack-vm") || strings.EqualFold(args.Type, "ablestack-standalone") {
		if len(lines) > 2 {
			lines = lines[:2]
		}
	}

	hostname, _ := os.Hostname()
	ccvm := ensureMap(cfg, "ccvm")
	ccvmIP := getString(ccvm["ip"])
	if ccvmIP != "" {
		lines = filterHostsLines(lines, map[string]bool{ccvmIP: true}, map[string]bool{
			"ccvm-mngt": true,
			"ccvm":      true,
		})
		lines = append(lines, formatHostsEntry(ccvmIP, []string{"ccvm-mngt", "ccvm"}))
	}

	hosts := getHosts(cfg)
	for _, host := range hosts {
		index := getString(host["index"])
		ablecubeIP := getString(host["ablecube"])
		scvmMngtIP := getString(host["scvmMngt"])
		ablecubePnIP := getString(host["ablecubePn"])
		scvmIP := getString(host["scvm"])
		scvmCnIP := getString(host["scvmCn"])
		hostName := getString(host["hostname"])
		scvmName := "scvm" + index

		if strings.EqualFold(args.Type, "ablestack-vm") || strings.EqualFold(args.Type, "ablestack-standalone") {
			removeNames := map[string]bool{
				hostName:   true,
				ablecubeIP: true,
			}
			if strings.EqualFold(args.StorageNetwork, "true") {
				removeNames[ablecubePnIP] = true
			}
			lines = filterHostsLines(lines, nil, removeNames)

			if hostname == hostName {
				lines = append(lines, formatHostsEntry(ablecubeIP, []string{hostName, "ablecube"}))
				if strings.EqualFold(args.StorageNetwork, "true") {
					lines = append(lines, formatHostsEntry(ablecubePnIP, []string{"pn-ablecube" + index, "pn-ablecube"}))
				}
			} else {
				lines = append(lines, formatHostsEntry(ablecubeIP, []string{hostName}))
				if strings.EqualFold(args.StorageNetwork, "true") {
					lines = append(lines, formatHostsEntry(ablecubePnIP, []string{"pn-ablecube" + index}))
				}
			}
			continue
		}

		removeIPs := map[string]bool{
			ablecubeIP:   true,
			scvmMngtIP:   true,
			ablecubePnIP: true,
			scvmIP:       true,
			scvmCnIP:     true,
		}
		removeNames := map[string]bool{
			hostName:                 true,
			"scvm" + index + "-mngt": true,
			"pn-ablecube" + index:    true,
			"scvm" + index:           true,
			"cn-scvm" + index:        true,
		}
		lines = filterHostsLines(lines, removeIPs, removeNames)

		localAblecube := hostname == hostName
		localSCVM := hostname == scvmName
		if localAblecube {
			lines = append(lines, formatHostsEntry(ablecubeIP, []string{hostName, "ablecube"}))
		} else {
			lines = append(lines, formatHostsEntry(ablecubeIP, []string{hostName}))
		}
		if localAblecube || localSCVM {
			lines = append(lines, formatHostsEntry(scvmMngtIP, []string{scvmName + "-mngt", "scvm-mngt"}))
			lines = append(lines, formatHostsEntry(ablecubePnIP, []string{"pn-ablecube" + index, "pn-ablecube"}))
			lines = append(lines, formatHostsEntry(scvmIP, []string{scvmName, "scvm"}))
			lines = append(lines, formatHostsEntry(scvmCnIP, []string{"cn-scvm" + index, "cn-scvm"}))
		} else {
			lines = append(lines, formatHostsEntry(scvmMngtIP, []string{scvmName + "-mngt"}))
			lines = append(lines, formatHostsEntry(ablecubePnIP, []string{"pn-ablecube" + index}))
			lines = append(lines, formatHostsEntry(scvmIP, []string{scvmName}))
			lines = append(lines, formatHostsEntry(scvmCnIP, []string{"cn-scvm" + index}))
		}
	}

	if err := writeHostsLines(lines); err != nil {
		return resultError(err.Error())
	}

	return resultOK("hosts file config success.")
}

func readHostsLines() ([]string, error) {
	content, err := os.ReadFile(hostsFilePath)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(string(content), "\n"), "\n")
	return lines, nil
}

func writeHostsLines(lines []string) error {
	content := strings.Join(lines, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return os.WriteFile(hostsFilePath, []byte(content), 0644)
}

func updateHostsFile(removeIPs map[string]bool, removeNames map[string]bool, addEntries []string) error {
	lines, err := readHostsLines()
	if err != nil {
		return err
	}
	lines = filterHostsLines(lines, removeIPs, removeNames)
	if len(addEntries) > 0 {
		lines = append(lines, addEntries...)
	}
	return writeHostsLines(lines)
}

func filterHostsLines(lines []string, removeIPs map[string]bool, removeNames map[string]bool) []string {
	if len(removeIPs) == 0 && len(removeNames) == 0 {
		return lines
	}
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			result = append(result, line)
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			result = append(result, line)
			continue
		}
		ip := fields[0]
		if removeIPs[ip] {
			continue
		}
		match := false
		for _, name := range fields[1:] {
			if removeNames[name] {
				match = true
				break
			}
		}
		if match {
			continue
		}
		result = append(result, line)
	}
	return result
}

func formatHostsEntry(ip string, names []string) string {
	if ip == "" || len(names) == 0 {
		return ""
	}
	return fmt.Sprintf("%s\t%s", ip, strings.Join(names, " "))
}

func ensureHugePageFS() {
	if !fileExists("/hugepages") {
		_ = os.MkdirAll("/hugepages", 0755)
	}
	fstab, err := os.ReadFile("/etc/fstab")
	if err == nil {
		if !bytes.Contains(fstab, []byte(" /hugepages ")) {
			entry := "\nhugetlbfs /hugepages hugetlbfs defaults 0 0\n"
			_ = appendToFile("/etc/fstab", entry)
		}
	}
	_ = runCommandQuiet("mount", "-a")
}

func appendToFile(path string, content string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

func applyNetworkFilter() error {
	if err := exec.Command("systemctl", "is-active", "openvswitch").Run(); err == nil {
		return nil
	}

	out, _ := runCommand("virsh", "nwfilter-list")
	if !strings.Contains(out, "allow-all") {
		_ = runCommandQuiet("virsh", "nwfilter-define", "--file", "/usr/local/sbin/nwfilter-allow-all.xml")
	}

	out, _ = runCommand("lsmod")
	if !strings.Contains(out, "br_netfilter") {
		_ = runCommandQuiet("modprobe", "br_netfilter")
	}

	settings := []string{
		"net.bridge.bridge-nf-call-arptables=1",
		"net.bridge.bridge-nf-call-iptables=1",
		"net.bridge.bridge-nf-call-ip6tables=1",
	}
	existing, _ := os.ReadFile("/etc/sysctl.conf")
	existingStr := string(existing)
	var additions []string
	for _, setting := range settings {
		key := strings.SplitN(setting, "=", 2)[0]
		if !strings.Contains(existingStr, key) {
			additions = append(additions, setting)
		}
	}
	if len(additions) > 0 {
		var buf strings.Builder
		for _, setting := range additions {
			buf.WriteString("\n")
			buf.WriteString(setting)
		}
		_ = appendToFile("/etc/sysctl.conf", buf.String())
	}
	_ = runCommandQuiet("sysctl", "-p")
	return nil
}

func checkAPITarget(host string) error {
	if host == "" {
		return errors.New("empty host")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("%s/api/v1/health", buildTargetURL(host))
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	attachInternalToken(req)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("status: %s", resp.Status)
	}
	return nil
}

func callApplyLocalAPI(target string, action string, args Args) (Result, error) {
	hosts, err := parseHostParams(args.JSONString)
	if err != nil {
		return Result{}, err
	}
	req := ApplyRequest{
		Action:             action,
		Type:               args.Type,
		CCVM:               map[string]string{"ip": args.CCVMMngtIP},
		MngtNic:            map[string]string{"cidr": args.MngtNicCIDR, "gw": args.MngtNicGW, "dns": args.MngtNicDNS},
		CCVMMngtIP:         args.CCVMMngtIP,
		MngtNicCIDR:        args.MngtNicCIDR,
		MngtNicGW:          args.MngtNicGW,
		MngtNicDNS:         args.MngtNicDNS,
		PCSClusterList:     args.PCSClusterList,
		Hosts:              hosts,
		CopyOption:         "hostOnly",
		ExcludeHostname:    args.ExcludeHostname,
		RemoveHostname:     args.RemoveHostname,
		ExternalTimeserver: args.ExternalTimeserver,
		StorageNetwork:     args.StorageNetwork,
	}
	if strings.TrimSpace(args.InternalToken) != "" {
		req.Security = map[string]string{"internal_token": strings.TrimSpace(args.InternalToken)}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return Result{}, err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("%s/api/v1/cube/cluster/apply-local", buildTargetURL(target))
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	attachInternalToken(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("apply-local failed: %s", resp.Status)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, err
	}
	var res Result
	if err := json.Unmarshal(raw, &res); err != nil {
		return Result{}, fmt.Errorf("invalid apply-local response")
	}
	return res, nil
}

func buildTargetURL(target string) string {
	scheme := os.Getenv("ABLESTACK_API_SCHEME")
	if scheme == "" {
		scheme = "http"
	}
	port := os.Getenv("ABLESTACK_API_PORT")
	if port == "" {
		port = "18090"
	}
	return fmt.Sprintf("%s://%s:%s", scheme, target, port)
}

func attachInternalToken(req *http.Request) {
	if req == nil {
		return
	}
	token := readInternalToken()
	if token == "" {
		return
	}
	req.Header.Set("X-Cube-Internal-Token", token)
}

func readInternalToken() string {
	if token := strings.TrimSpace(os.Getenv("CUBE_INTERNAL_TOKEN")); token != "" {
		return token
	}
	if token := strings.TrimSpace(os.Getenv("ABLESTACK_INTERNAL_TOKEN")); token != "" {
		return token
	}
	raw, err := os.ReadFile(resolveClusterJSONPath())
	if err != nil {
		return ""
	}
	root := map[string]any{}
	if err := json.Unmarshal(raw, &root); err != nil {
		return ""
	}
	securityMap, ok := root["security"].(map[string]any)
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(securityMap["internal_token"]))
}

func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

func runCommandQuiet(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func parseResult(raw string) (Result, error) {
	if raw == "" {
		return Result{}, errors.New("empty response")
	}
	var res Result
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		return Result{}, err
	}
	if res.Code == 0 && res.Val == "" {
		return Result{}, errors.New("invalid response")
	}
	return res, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
