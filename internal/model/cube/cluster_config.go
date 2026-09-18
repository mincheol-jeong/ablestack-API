package cube

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

const PCSClusterMaxHosts = 16

const (
	GFSDefaultJournalSizeMB       = 512
	GFSMinJournalSizeMB           = 8
	GFSMaxJournalSizeMB           = 1024
	GFSDefaultResourceGroupSizeMB = 1024
	GFSMinResourceGroupSizeMB     = 32
	GFSMaxResourceGroupSizeMB     = 2048
	GFSJournalSizeDescription     = "MB 단위. 2의 거듭제곱 값으로 설정하며 허용 범위는 8MB~1024MB입니다. 예: 128MB, 256MB, 512MB"
	GFSResourceGroupDescription   = "MB 단위. 2의 거듭제곱 값으로 설정하며 허용 범위는 32MB~2048MB입니다."
)

type TypeClusterConfig struct {
	Data map[string]any `json:"-"`
	mu   *sync.RWMutex
}

var lockClusterConfig sync.Once
var _ClusterConfig *TypeClusterConfig

func ClusterConfig() *TypeClusterConfig {
	lockClusterConfig.Do(func() {
		_ClusterConfig = &TypeClusterConfig{mu: &sync.RWMutex{}}
	})
	return _ClusterConfig
}

func (c *TypeClusterConfig) Lock() {
	if c == nil || c.mu == nil {
		return
	}
	c.mu.Lock()
}

func (c *TypeClusterConfig) Unlock() {
	if c == nil || c.mu == nil {
		return
	}
	c.mu.Unlock()
}

func (c *TypeClusterConfig) RLock() {
	if c == nil || c.mu == nil {
		return
	}
	c.mu.RLock()
}

func (c *TypeClusterConfig) RUnlock() {
	if c == nil || c.mu == nil {
		return
	}
	c.mu.RUnlock()
}

func (c *TypeClusterConfig) ApplyFrom(src map[string]any) {
	if c == nil {
		return
	}
	if c.mu != nil {
		c.mu.Lock()
		defer c.mu.Unlock()
	}
	c.Data = src
}

func (c *TypeClusterConfig) MarshalJSON() ([]byte, error) {
	if c == nil || c.Data == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(c.Data)
}

// Normalize fills legacy json_string from hosts when needed.
func (r *ClusterApplyRequest) Normalize() error {
	if r == nil {
		return nil
	}
	if r.RemoveHostname == "" && r.TargetHostname != "" {
		r.RemoveHostname = r.TargetHostname
	}
	if r.ExternalTimeserver == "" && r.DeprecatedExtenalTimeserver != "" {
		r.ExternalTimeserver = r.DeprecatedExtenalTimeserver
		r.DeprecatedExtenalTimeserver = ""
	}
	if r.StorageNetwork == "" && r.DeprecatedIscsiStorage != "" {
		r.StorageNetwork = r.DeprecatedIscsiStorage
		r.DeprecatedIscsiStorage = ""
	}
	if r.CCVM != nil {
		if r.CCVMMngtIP == "" {
			r.CCVMMngtIP = r.CCVM.IP
		}
		// Backward compatibility: older clients sent management NIC values under ccvm.
		if r.MngtNicCIDR == "" {
			r.MngtNicCIDR = r.CCVM.CIDR
		}
		if r.MngtNicDNS == "" {
			r.MngtNicDNS = r.CCVM.DNS
		}
		if r.MngtNicGW == "" {
			r.MngtNicGW = r.CCVM.GW
		}
	}
	if r.MngtNic != nil {
		if r.MngtNic.CIDR != "" {
			r.MngtNicCIDR = r.MngtNic.CIDR
		}
		if r.MngtNic.GW != "" {
			r.MngtNicGW = r.MngtNic.GW
		}
		if r.MngtNic.DNS != "" {
			r.MngtNicDNS = r.MngtNic.DNS
		}
	}
	if r.JSONString == "" && len(r.Hosts) > 0 {
		raw, err := json.Marshal(r.Hosts)
		if err != nil {
			return err
		}
		r.JSONString = string(raw)
	}
	return nil
}

// ClusterApplyRequest describes the request body for cluster apply APIs.
// @name ClusterApplyRequest
type ClusterApplyRequest struct {
	// action: insert/remove/reset/check
	Action string `json:"action" example:"insert"`
	// option (e.g. add/hostOnly/withScvm/all/local)
	Option string `json:"option,omitempty" example:"add"`
	// cluster type (e.g. ablestack-vm, ablestack-hci)
	Type string `json:"type" example:"ablestack-vm"`
	// ccvm config (ip only)
	CCVM *ClusterCCVMConfig `json:"ccvm,omitempty"`
	// management NIC config
	MngtNic *ClusterMngtNicConfig `json:"mngtNic,omitempty"`
	// security config for internal fan-out calls
	Security *ClusterSecurityConfig `json:"security,omitempty" swaggerignore:"true"`
	// ccvm management IP (internal)
	CCVMMngtIP string `json:"-"`
	// management NIC CIDR (internal)
	MngtNicCIDR string `json:"-"`
	// management NIC gateway (internal)
	MngtNicGW string `json:"-"`
	// management NIC DNS (internal)
	MngtNicDNS string `json:"-"`
	// pcs cluster node IP list
	PCSClusterList []string `json:"pcs_cluster_list" example:"10.10.31.1,10.10.31.2,10.10.31.3"`
	// legacy hosts JSON string (internal only)
	JSONString string `json:"-"`
	// hosts file copy option (internal)
	CopyOption string `json:"-"`
	// exclude hostname (hci only)
	ExcludeHostname string `json:"exclude_hostname" example:"ablecube31-2"`
	// remove hostname
	RemoveHostname string `json:"remove_hostname" example:"ablecube31-3"`
	// target hostname (deprecated alias of remove_hostname)
	TargetHostname string `json:"target_hostname,omitempty" swaggerignore:"true"`
	// new hostname (option=add: exclude scvm fanout)
	NewHostname string `json:"new_hostname,omitempty" example:"ablecube12-3"`
	// external timeserver
	ExternalTimeserver string `json:"external_timeserver" example:"time.google.com"`
	// external timeserver (deprecated typo)
	DeprecatedExtenalTimeserver string `json:"extenal_timeserver,omitempty" swaggerignore:"true"`
	// storage network usage (true/false)
	StorageNetwork string `json:"storage_network" example:"false"`
	// storage network usage (deprecated: use storage_network)
	DeprecatedIscsiStorage string `json:"iscsi_storage,omitempty" swaggerignore:"true"`
	// hosts list (preferred)
	Hosts []ClusterHost `json:"hosts"`
	// existing hostnames from cluster.json (internal)
	ExistingHostnames []string `json:"-"`
}

// ClusterSecurityConfig describes security settings stored at cluster.json root.
// @name ClusterSecurityConfig
type ClusterSecurityConfig struct {
	// internal token for API server fan-out calls
	InternalToken string `json:"internal_token,omitempty"`
}

// ClusterHost describes a single host item.
// @name ClusterHost
type ClusterHost struct {
	Index      string `json:"index" example:"1"`
	Hostname   string `json:"hostname" example:"ablecube31-1"`
	Ablecube   string `json:"ablecube" example:"10.10.31.1"`
	ScvmMngt   string `json:"scvmMngt,omitempty" example:"10.10.31.11"`
	AblecubePn string `json:"ablecubePn,omitempty" example:"100.100.31.1"`
	Scvm       string `json:"scvm,omitempty" example:"100.100.31.11"`
	ScvmCn     string `json:"scvmCn,omitempty" example:"100.200.31.11"`
}

// ClusterApplyResult is a per-target result.
// @name ClusterApplyResult
type ClusterApplyResult struct {
	Target  string `json:"target" example:"10.10.31.1"`
	Code    int    `json:"code" example:"200"`
	Message string `json:"message" example:"ok"`
}

// ClusterApplyResponse is the orchestrator response.
// @name ClusterApplyResponse
type ClusterApplyResponse struct {
	Code    int                  `json:"code" example:"200"`
	Message string               `json:"message" example:"apply success"`
	Results []ClusterApplyResult `json:"results"`
}

// ClusterApplyLocalResponse is the local apply response.
// @name ClusterApplyLocalResponse
type ClusterApplyLocalResponse struct {
	Code int    `json:"code" example:"200"`
	Val  string `json:"val" example:"Cluster Config insert Success"`
}

// ClusterHealthTargetResult is a per-target health result.
// @name ClusterHealthTargetResult
type ClusterHealthTargetResult struct {
	// target role (host/scvm/ccvm)
	Role string `json:"role" example:"host"`
	// role display name. host uses hosts[].hostname, scvm uses scvm+index, ccvm uses ccvm.
	Hostname string `json:"hostname,omitempty" example:"ablecube12-1"`
	// target IP
	Target string `json:"target" example:"10.10.31.1"`
	// response code
	Code int `json:"code" example:"200"`
	// response message
	Message string `json:"message" example:"ok"`
}

// ClusterHealthResponse is the health response.
// @name ClusterHealthResponse
type ClusterHealthResponse struct {
	// ok status for default health
	Status string `json:"status,omitempty" example:"ok"`
	// result code for option-based checks
	Code int `json:"code,omitempty" example:"200"`
	// result message for option-based checks
	Message string `json:"message,omitempty" example:"health check success"`
	// per-target results
	Results []ClusterHealthTargetResult `json:"results,omitempty"`
}

// ClusterConfigResponse is the cluster.json response body.
// @name ClusterConfigResponse
type ClusterConfigResponse struct {
	// cluster configuration data
	ClusterConfig ClusterConfigSection `json:"clusterConfig"`
	// internal API security config
	Security ClusterSecurityConfig `json:"security"`
}

// ClusterConfigSection describes the clusterConfig block.
// @name ClusterConfigSection
type ClusterConfigSection struct {
	// cluster type
	Type string `json:"type" example:"ablestack-vm"`
	// latest cluster host operation: new/add
	HostType string `json:"hostType,omitempty" example:"add"`
	// backup path for ccvm
	BackupPath string `json:"backup_path" example:"/mnt/glue-gfs/backup/ccvm"`
	// ccvm config
	CCVM ClusterCCVMConfig `json:"ccvm"`
	// management NIC config
	MngtNic ClusterMngtNicConfig `json:"mngtNic"`
	// pcs cluster IPs
	PCSCluster ClusterPCSClusterConfig `json:"pcsCluster"`
	// GFS2 filesystem format settings
	GFS ClusterGFSConfig `json:"gfs"`
	// host list
	Hosts []ClusterHost `json:"hosts"`
	// external timeserver
	ExternalTimeserver string `json:"external_timeserver" example:"time.google.com"`
	// storage network usage
	StorageNetwork string `json:"storage_network" example:"false"`
	// storage network usage (deprecated: use storage_network)
	DeprecatedIscsiStorage string `json:"iscsi_storage,omitempty" swaggerignore:"true"`
}

// ClusterGFSConfig describes mkfs.gfs2 sizing values stored in cluster.json.
// @name ClusterGFSConfig
type ClusterGFSConfig struct {
	// GFS2 journal size in MB. Must be a power of two between 8 and 1024.
	JournalSizeMB int `json:"journal_size_mb" example:"512"`
	// Human-readable journal size constraint stored with cluster.json.
	JournalSizeDescription string `json:"journal_size_description" example:"MB 단위. 2의 거듭제곱 값으로 설정하며 허용 범위는 8MB~1024MB입니다."`
	// GFS2 resource group size in MB. Must be a power of two between 32 and 2048.
	ResourceGroupSizeMB int `json:"resource_group_size_mb" example:"1024"`
	// Human-readable resource group size constraint stored with cluster.json.
	ResourceGroupDescription string `json:"resource_group_size_description" example:"MB 단위. 2의 거듭제곱 값으로 설정하며 허용 범위는 32MB~2048MB입니다."`
}

func (c *ClusterConfigSection) UnmarshalJSON(data []byte) error {
	type alias ClusterConfigSection
	var raw alias
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*c = ClusterConfigSection(raw)
	if c.StorageNetwork == "" && c.DeprecatedIscsiStorage != "" {
		c.StorageNetwork = c.DeprecatedIscsiStorage
	}
	c.DeprecatedIscsiStorage = ""
	return nil
}

// ClusterCCVMConfig describes ccvm network info.
// @name ClusterCCVMConfig
type ClusterCCVMConfig struct {
	// ccvm IP
	IP string `json:"ip" example:"10.10.31.10"`
	// management NIC CIDR (deprecated: use mngtNic.cidr)
	CIDR string `json:"cidr,omitempty" swaggerignore:"true"`
	// management NIC gateway (deprecated: use mngtNic.gw)
	GW string `json:"gw,omitempty" swaggerignore:"true"`
	// management NIC DNS (deprecated: use mngtNic.dns)
	DNS string `json:"dns,omitempty" swaggerignore:"true"`
}

// ClusterMngtNicConfig describes management NIC network info.
// @name ClusterMngtNicConfig
type ClusterMngtNicConfig struct {
	// management NIC CIDR
	CIDR string `json:"cidr" example:"16"`
	// management NIC gateway
	GW string `json:"gw" example:"10.10.0.1"`
	// management NIC DNS
	DNS string `json:"dns" example:"8.8.8.8"`
}

// ClusterPCSClusterConfig describes pcs cluster IPs. hostname4...hostname16 are also preserved at runtime.
// @name ClusterPCSClusterConfig
type ClusterPCSClusterConfig struct {
	// pcs cluster node #1
	Hostname1 string `json:"hostname1" example:"10.10.31.1"`
	// pcs cluster node #2
	Hostname2 string `json:"hostname2" example:"10.10.31.2"`
	// pcs cluster node #3
	Hostname3 string   `json:"hostname3" example:"10.10.31.3"`
	Hostnames []string `json:"-" swaggerignore:"true"`
}

func (c *ClusterPCSClusterConfig) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	values := make([]string, 0, PCSClusterMaxHosts)
	if rawHostnames, ok := raw["hostnames"]; ok {
		values = append(values, stringSliceFromAny(rawHostnames)...)
	}
	for i := 1; i <= PCSClusterMaxHosts; i++ {
		if value, ok := raw[fmt.Sprintf("hostname%d", i)]; ok {
			values = append(values, pcsStringFromAny(value))
		}
	}

	c.Hostnames = NormalizePCSClusterList(values)
	c.Hostname1 = pcsHostnameAt(c.Hostnames, 0)
	c.Hostname2 = pcsHostnameAt(c.Hostnames, 1)
	c.Hostname3 = pcsHostnameAt(c.Hostnames, 2)
	return nil
}

func (c ClusterPCSClusterConfig) MarshalJSON() ([]byte, error) {
	hostnames := c.HostnameList()
	out := make(map[string]string, maxInt(len(hostnames), 3))
	for i, hostname := range hostnames {
		out[fmt.Sprintf("hostname%d", i+1)] = hostname
	}
	for i := len(hostnames) + 1; i <= 3; i++ {
		out[fmt.Sprintf("hostname%d", i)] = ""
	}
	return json.Marshal(out)
}

func (c ClusterPCSClusterConfig) HostnameList() []string {
	values := c.Hostnames
	if len(values) == 0 {
		values = []string{c.Hostname1, c.Hostname2, c.Hostname3}
	}
	return NormalizePCSClusterList(values)
}

func NormalizePCSClusterList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		if len(out) >= PCSClusterMaxHosts {
			break
		}
	}
	return out
}

func CountPCSClusterList(values []string) int {
	return countPCSClusterList(values, 0)
}

func ValidatePCSClusterList(values []string, min int) error {
	count := countPCSClusterList(values, PCSClusterMaxHosts+1)
	if count < min {
		return fmt.Errorf("pcs_cluster_list requires at least %d host(s)", min)
	}
	if count > PCSClusterMaxHosts {
		return fmt.Errorf("pcs_cluster_list supports up to %d hosts", PCSClusterMaxHosts)
	}
	return nil
}

func countPCSClusterList(values []string, limit int) int {
	count := 0
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		count++
		if limit > 0 && count >= limit {
			return count
		}
	}
	return count
}

func pcsHostnameAt(values []string, index int) string {
	if index < 0 || index >= len(values) {
		return ""
	}
	return values[index]
}

func stringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, pcsStringFromAny(item))
		}
		return out
	default:
		return nil
	}
}

func pcsStringFromAny(value any) string {
	if value == nil {
		return ""
	}
	if typed, ok := value.(string); ok {
		return typed
	}
	return fmt.Sprint(value)
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

// ClusterBootstrapConfig describes bootstrap flags.
// @name ClusterBootstrapConfig
type ClusterBootstrapConfig struct {
	// scvm bootstrap flag
	Scvm string `json:"scvm" example:"false"`
	// ccvm bootstrap flag
	Ccvm string `json:"ccvm" example:"true"`
	// wall configuration flag
	Wall string `json:"wall" example:"true"`
	// gfs configure flag
	GFSConfigure string `json:"gfs_configure" example:"true"`
	// local configure flag
	LocalConfigure string `json:"local_configure" example:"false"`
}

// ClusterSystemProfile describes system profile blocks.
// @name ClusterSystemProfile
type ClusterSystemProfile struct {
	// bootstrap flags
	Bootstrap ClusterBootstrapConfig `json:"bootstrap"`
	// license info
	License ClusterLicenseConfig `json:"license"`
	// security patch status
	SecurityPatch ClusterSecurityPatchConfig `json:"security_patch"`
}

// ClusterLicenseConfig describes license info.
// @name ClusterLicenseConfig
type ClusterLicenseConfig struct {
	// license status
	Status string `json:"status" example:"true"`
	// license type
	Type string `json:"type" example:"ablestack"`
}

// ClusterSecurityPatchConfig describes security patch status.
// @name ClusterSecurityPatchConfig
type ClusterSecurityPatchConfig struct {
	// security patch status
	Status string `json:"status" example:"false"`
}

// SystemConfigRequest describes the request body for system config APIs.
// @name SystemConfigRequest
type SystemConfigRequest struct {
	// action: status/update/allUpdate/reset/gfs-configure
	Action string `json:"action" example:"status"`
	// option: all (fan-out to all ablecube hosts)
	Option string `json:"option,omitempty" example:"all"`
	// depth1 key (bootstrap/license/security_patch)
	Depth1 string `json:"depth1,omitempty" example:"bootstrap"`
	// depth2 key (e.g. scvm/ccvm/wall/gfs_configure/local_configure/status/type)
	Depth2 string `json:"depth2,omitempty" example:"scvm"`
	// value for update
	Value string `json:"value,omitempty" example:"true"`
}

// SystemConfigResponse describes the response body for system config APIs.
// @name SystemConfigResponse
type SystemConfigResponse struct {
	Code int `json:"code" example:"200"`
	Val  any `json:"val"`
}
