package moldservice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	MoldModel "ablecloud.io/ablestack-api/internal/model/mold"
	"ablecloud.io/ablestack-api/internal/service/security"
	"ablecloud.io/ablestack-api/internal/service/sshtrust"
)

const (
	defaultAsyncPollInterval    = 5 * time.Second
	defaultAsyncPollTimeout     = 30 * time.Minute
	defaultSystemVMPollInterval = 10 * time.Second
	defaultSystemVMPollTimeout  = 20 * time.Minute
)

type BootstrapRuntime struct {
	req       MoldModel.BootstrapRequest
	client    *Client
	session   Session
	cephKey   func(context.Context, []string) (string, error)
	hostTrust func(context.Context) (any, any, error)

	endpoint string
	ccvmIP   string

	zoneID                  string
	physicalNetworkID       string
	podID                   string
	clusterID               string
	primaryStorageID        string
	secondaryStorageID      string
	secondaryStagingStoreID string
}

func NewBootstrapRuntime(req MoldModel.BootstrapRequest) *BootstrapRuntime {
	runtime := &BootstrapRuntime{req: NormalizeBootstrapRequest(req), cephKey: fetchCephAdminKey}
	runtime.hostTrust = runtime.reconcileHostSSHTrust
	return runtime
}

func (r *BootstrapRuntime) RunStep(ctx context.Context, step MoldModel.BootstrapJobStep) MoldModel.BootstrapStepResult {
	switch step.Name {
	case "wait_mold_api_ready":
		return r.waitMoldAPIReady(ctx, step)
	case "login_admin":
		return r.loginAdmin(ctx, step)
	case "create_zone":
		return r.ensureZone(ctx, step)
	case "create_physical_network":
		return r.ensurePhysicalNetwork(ctx, step)
	case "add_traffic_types":
		return r.ensureTrafficTypes(ctx, step)
	case "enable_physical_network":
		return r.enablePhysicalNetwork(ctx, step)
	case "enable_network_providers":
		return r.enableNetworkProviders(ctx, step)
	case "create_pod":
		return r.ensurePod(ctx, step)
	case "create_public_ip_range":
		return r.ensurePublicIPRange(ctx, step)
	case "create_cluster":
		return r.ensureCluster(ctx, step)
	case "sync_host_ssh_trust":
		return r.syncHostSSHTrust(ctx, step)
	case "add_host":
		return r.ensureHosts(ctx, step)
	case "add_primary_storage":
		return r.ensurePrimaryStorage(ctx, step)
	case "add_secondary_storage":
		return r.ensureSecondaryStorage(ctx, step)
	case "add_secondary_staging_store":
		return r.ensureSecondaryStagingStore(ctx, step)
	case "enable_zone":
		return r.enableZone(ctx, step)
	case "wait_system_vm_running":
		return r.waitSystemVMRunning(ctx, step)
	case "final_health_check":
		return r.finalHealthCheck(ctx, step)
	default:
		return failedStep(step, fmt.Errorf("unknown mold bootstrap step: %s", step.Name), nil)
	}
}

type moldHostTrustTarget struct {
	Hostname string `json:"hostname,omitempty"`
	Target   string `json:"target"`
}

type moldHostTrustResult struct {
	Hostname    string `json:"hostname,omitempty"`
	Target      string `json:"target"`
	Code        int    `json:"code"`
	Changed     bool   `json:"changed,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Message     string `json:"message,omitempty"`
}

func (r *BootstrapRuntime) syncHostSSHTrust(ctx context.Context, step MoldModel.BootstrapJobStep) MoldModel.BootstrapStepResult {
	if r.hostTrust == nil {
		return failedStep(step, fmt.Errorf("host ssh trust reconciler is not configured"), nil)
	}
	registered, verified, err := r.hostTrust(ctx)
	if err != nil {
		return failedStep(step, err, map[string]any{"registration": registered, "verification": verified})
	}
	return succeededStep(step, stepResultOptions{Raw: registered, Verify: verified})
}

func (r *BootstrapRuntime) reconcileHostSSHTrust(ctx context.Context) (any, any, error) {
	publicKey, fingerprint, err := sshtrust.ReadManagementPublicKey()
	if err != nil {
		return nil, nil, err
	}
	targets, err := loadMoldHostTrustTargets()
	if err != nil {
		return nil, nil, err
	}
	if len(targets) == 0 {
		return nil, nil, fmt.Errorf("clusterConfig.hosts[].ablecube required")
	}
	token, err := security.GetInternalToken()
	if err != nil {
		return nil, nil, fmt.Errorf("read internal token: %w", err)
	}

	registered := make([]moldHostTrustResult, 0, len(targets))
	for _, target := range targets {
		result := ensureMoldHostTrust(ctx, target, publicKey, token)
		if result.Fingerprint == "" {
			result.Fingerprint = fingerprint
		}
		registered = append(registered, result)
		if result.Code < 200 || result.Code >= 300 {
			return registered, nil, fmt.Errorf("register cloud@ccvm on %s failed: %s", target.Target, result.Message)
		}
	}

	verified := make([]moldHostTrustResult, 0, len(targets))
	for _, target := range targets {
		result := verifyMoldHostSSH(ctx, target, fingerprint)
		verified = append(verified, result)
		if result.Code != http.StatusOK {
			return registered, verified, fmt.Errorf("verify CCVM SSH access to %s failed: %s", target.Target, result.Message)
		}
	}
	return registered, verified, nil
}

func loadMoldHostTrustTargets() ([]moldHostTrustTarget, error) {
	path := strings.TrimSpace(os.Getenv("ABLESTACK_CLUSTER_JSON"))
	if path == "" {
		path = "/etc/ablestack/properties/cluster.json"
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read cluster.json: %w", err)
	}
	var root struct {
		ClusterConfig CubeModel.ClusterConfigSection `json:"clusterConfig"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("decode cluster.json: %w", err)
	}
	seen := map[string]struct{}{}
	targets := make([]moldHostTrustTarget, 0, len(root.ClusterConfig.Hosts))
	for _, host := range root.ClusterConfig.Hosts {
		target := strings.TrimSpace(host.Ablecube)
		if target == "" {
			continue
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		targets = append(targets, moldHostTrustTarget{Hostname: strings.TrimSpace(host.Hostname), Target: target})
	}
	return targets, nil
}

func ensureMoldHostTrust(ctx context.Context, target moldHostTrustTarget, publicKey string, token string) moldHostTrustResult {
	result := moldHostTrustResult{Hostname: target.Hostname, Target: target.Target}
	payload, _ := json.Marshal(CubeModel.SSHTrustRequest{Action: "ensure", PublicKey: publicKey, Identifier: sshtrust.DefaultIdentifier})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, moldHostAPIURL(target.Target)+"/api/v1/cube/ssh/trust", bytes.NewReader(payload))
	if err != nil {
		result.Message = err.Error()
		return result
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(security.InternalTokenHeader, token)
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		result.Message = err.Error()
		return result
	}
	defer resp.Body.Close()
	result.Code = resp.StatusCode
	raw, _ := io.ReadAll(resp.Body)
	var decoded CubeModel.SSHTrustResponse
	if err := json.Unmarshal(raw, &decoded); err == nil {
		result.Changed = decoded.Changed
		result.Fingerprint = decoded.Fingerprint
		result.Message = decoded.Message
	}
	if result.Message == "" {
		result.Message = strings.TrimSpace(string(raw))
	}
	if result.Message == "" {
		result.Message = resp.Status
	}
	return result
}

func verifyMoldHostSSH(ctx context.Context, target moldHostTrustTarget, fingerprint string) moldHostTrustResult {
	result := moldHostTrustResult{Hostname: target.Hostname, Target: target.Target, Fingerprint: fingerprint}
	port := moldHostSSHPort()
	tempDir, err := os.MkdirTemp("", "mold-ssh-verify-*")
	if err != nil {
		result.Message = err.Error()
		return result
	}
	defer os.RemoveAll(tempDir)
	knownHosts := filepath.Join(tempDir, "known_hosts")
	scan := exec.CommandContext(ctx, "ssh-keyscan", "-T", "5", "-p", strconv.Itoa(port), target.Target)
	scanOutput, err := scan.Output()
	if err != nil || len(bytes.TrimSpace(scanOutput)) == 0 {
		result.Message = firstNonEmpty(commandErrorMessage(err), "ssh-keyscan returned no host key")
		return result
	}
	if err := os.WriteFile(knownHosts, scanOutput, 0o600); err != nil {
		result.Message = err.Error()
		return result
	}
	args := []string{
		"-p", strconv.Itoa(port), "-i", sshtrust.ManagementPrivateKeyPath(),
		"-o", "BatchMode=yes", "-o", "IdentitiesOnly=yes", "-o", "ConnectTimeout=10",
		"-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile=" + knownHosts,
		"root@" + target.Target, "true",
	}
	output, err := exec.CommandContext(ctx, "ssh", args...).CombinedOutput()
	if err != nil {
		result.Message = firstNonEmpty(strings.TrimSpace(string(output)), err.Error())
		return result
	}
	result.Code = http.StatusOK
	result.Message = "CCVM CloudStack management SSH access verified"
	return result
}

func moldHostAPIURL(target string) string {
	scheme := strings.TrimSpace(os.Getenv("ABLESTACK_API_SCHEME"))
	if scheme == "" {
		scheme = "http"
	}
	port := strings.TrimSpace(os.Getenv("ABLESTACK_API_PORT"))
	if port == "" {
		port = "18090"
	}
	return fmt.Sprintf("%s://%s:%s", scheme, target, port)
}

func moldHostSSHPort() int {
	value := firstNonEmpty(os.Getenv("ABLESTACK_SSH_PORT"), os.Getenv("SSH_PORT"))
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 22
	}
	return port
}

func commandErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (r *BootstrapRuntime) ensureTrafficTypes(ctx context.Context, step MoldModel.BootstrapJobStep) MoldModel.BootstrapStepResult {
	if err := r.requirePhysicalNetworkID(); err != nil {
		return failedStep(step, err, nil)
	}
	trafficTypes := []string{"Management", "Guest", "Public"}
	resources := make([]MoldModel.ResourceResult, 0, len(trafficTypes))
	created := false
	raw := make([]any, 0)
	verify := make([]any, 0)
	for _, trafficType := range trafficTypes {
		listResponse, item, err := r.findTrafficType(ctx, trafficType)
		verify = append(verify, listResponse)
		if err != nil {
			return failedStep(step, err, listResponse)
		}
		if item == nil {
			params := url.Values{}
			setParam(params, "physicalnetworkid", r.physicalNetworkID)
			setParam(params, "traffictype", trafficType)
			setParam(params, "kvmnetworklabel", "bridge0")
			response, err := r.applyCommand(ctx, "addTrafficType", params)
			raw = append(raw, response)
			if err != nil {
				return failedStep(step, err, response)
			}
			created = true
		} else if !strings.EqualFold(stringValue(item["kvmnetworklabel"]), "bridge0") {
			params := url.Values{}
			setParam(params, "id", stringValue(item["id"]))
			setParam(params, "kvmnetworklabel", "bridge0")
			response, err := r.applyCommand(ctx, "updateTrafficType", params)
			raw = append(raw, response)
			if err != nil {
				return failedStep(step, err, response)
			}
			created = true
		}
		listResponse, item, err = r.findTrafficType(ctx, trafficType)
		verify = append(verify, listResponse)
		if err != nil {
			return failedStep(step, withVerifyCommand(err, "listTrafficTypes"), listResponse)
		}
		if item == nil || !strings.EqualFold(stringValue(item["kvmnetworklabel"]), "bridge0") {
			return failedStep(step, fmt.Errorf("traffic type %s was not verified with kvmnetworklabel=bridge0", trafficType), listResponse)
		}
		resources = append(resources, *resourceFromItem(item, "listTrafficTypes"))
	}
	return succeededStep(step, stepResultOptions{Resources: resources, Created: created, Existing: !created, Raw: raw, Verify: verify})
}

func (r *BootstrapRuntime) enablePhysicalNetwork(ctx context.Context, step MoldModel.BootstrapJobStep) MoldModel.BootstrapStepResult {
	if err := r.requirePhysicalNetworkID(); err != nil {
		return failedStep(step, err, nil)
	}
	verify, item, err := r.findPhysicalNetwork(ctx)
	if err != nil {
		return failedStep(step, err, verify)
	}
	if item != nil && strings.EqualFold(stringValue(item["state"]), "Enabled") {
		return succeededStep(step, stepResultOptions{Resource: resourceFromItem(item, "listPhysicalNetworks"), Existing: true, Verify: verify})
	}
	params := url.Values{}
	setParam(params, "id", r.physicalNetworkID)
	setParam(params, "state", "Enabled")
	raw, err := r.applyCommand(ctx, "updatePhysicalNetwork", params)
	if err != nil {
		return failedStep(step, err, raw)
	}
	verify, item, err = r.findPhysicalNetwork(ctx)
	if err != nil {
		return failedStep(step, withVerifyCommand(err, "listPhysicalNetworks"), verify)
	}
	if item == nil || !strings.EqualFold(stringValue(item["state"]), "Enabled") {
		return failedStep(step, fmt.Errorf("physical network was not verified with state=Enabled"), verify)
	}
	return succeededStep(step, stepResultOptions{Resource: resourceFromItem(item, "listPhysicalNetworks"), Created: true, Raw: raw, Verify: verify})
}

func (r *BootstrapRuntime) enableNetworkProviders(ctx context.Context, step MoldModel.BootstrapJobStep) MoldModel.BootstrapStepResult {
	if err := r.requirePhysicalNetworkID(); err != nil {
		return failedStep(step, err, nil)
	}
	providerNames := []string{"VirtualRouter", "VpcVirtualRouter"}
	resources := make([]MoldModel.ResourceResult, 0, len(providerNames))
	raw := make([]any, 0)
	verify := make([]any, 0)
	changed := false
	for _, providerName := range providerNames {
		providerResponse, provider, err := r.findNetworkProvider(ctx, providerName)
		verify = append(verify, providerResponse)
		if err != nil {
			return failedStep(step, err, providerResponse)
		}
		if provider == nil {
			return failedStep(step, fmt.Errorf("network provider %s not found", providerName), providerResponse)
		}
		providerID := stringValue(provider["id"])
		elementResponse, element, err := r.findVirtualRouterElement(ctx, providerID)
		verify = append(verify, elementResponse)
		if err != nil {
			return failedStep(step, err, elementResponse)
		}
		if element == nil {
			return failedStep(step, fmt.Errorf("virtual router element for provider %s not found", providerName), elementResponse)
		}
		if !boolValue(element["enabled"]) {
			params := url.Values{}
			setParam(params, "id", stringValue(element["id"]))
			setParam(params, "enabled", "true")
			response, err := r.applyCommand(ctx, "configureVirtualRouterElement", params)
			raw = append(raw, response)
			if err != nil {
				return failedStep(step, err, response)
			}
			changed = true
		}
		if !strings.EqualFold(stringValue(provider["state"]), "Enabled") {
			params := url.Values{}
			setParam(params, "id", providerID)
			setParam(params, "state", "Enabled")
			response, err := r.applyCommand(ctx, "updateNetworkServiceProvider", params)
			raw = append(raw, response)
			if err != nil {
				return failedStep(step, err, response)
			}
			changed = true
		}
		providerResponse, provider, err = r.findNetworkProvider(ctx, providerName)
		if err != nil {
			return failedStep(step, withVerifyCommand(err, "listNetworkServiceProviders"), providerResponse)
		}
		elementResponse, element, err = r.findVirtualRouterElement(ctx, providerID)
		if err != nil {
			return failedStep(step, withVerifyCommand(err, "listVirtualRouterElements"), elementResponse)
		}
		verify = append(verify, providerResponse, elementResponse)
		if provider == nil || !strings.EqualFold(stringValue(provider["state"]), "Enabled") || element == nil || !boolValue(element["enabled"]) {
			return failedStep(step, fmt.Errorf("network provider %s was not fully enabled", providerName), verify)
		}
		resources = append(resources, *resourceFromItem(provider, "listNetworkServiceProviders"))
	}
	return succeededStep(step, stepResultOptions{Resources: resources, Created: changed, Existing: !changed, Raw: raw, Verify: verify})
}

func (r *BootstrapRuntime) waitMoldAPIReady(ctx context.Context, step MoldModel.BootstrapJobStep) MoldModel.BootstrapStepResult {
	endpoint, ccvmIP, _, err := ResolveMoldEndpointFromCluster()
	if err != nil {
		return failedStep(step, err, nil)
	}
	r.endpoint = endpoint
	r.ccvmIP = ccvmIP
	r.client = NewClient(endpoint, WithCCVMIP(ccvmIP))
	raw, err := r.client.CallCommand(ctx, http.MethodGet, "listCapabilities", "", nil)
	if err != nil {
		// An unauthenticated CloudStack API normally answers this probe with 401.
		// A structured Mold error still proves that the API endpoint is ready;
		// authentication is verified in the following login_admin step.
		if apiErr, ok := APIErrorFrom(err); ok && apiErr.Raw != nil {
			if rawMap, ok := apiErr.Raw.(map[string]any); ok {
				raw = rawMap
			}
		} else {
			return failedStep(step, err, raw)
		}
	}
	return succeededStep(step, stepResultOptions{
		Resource: &MoldModel.ResourceResult{
			Name:       "mold_api",
			URL:        endpoint,
			Status:     "responding",
			VerifiedBy: "listCapabilities",
			Raw:        map[string]any{"ccvm_ip": ccvmIP, "probe": raw},
		},
		Raw:    map[string]any{"endpoint": endpoint, "ccvm_ip": ccvmIP},
		Verify: raw,
	})
}

func (r *BootstrapRuntime) loginAdmin(ctx context.Context, step MoldModel.BootstrapJobStep) MoldModel.BootstrapStepResult {
	if err := r.ensureClient(); err != nil {
		return failedStep(step, err, nil)
	}
	session, err := r.client.Login(ctx, AuthConfig{
		Username: r.req.Auth.Username,
		Password: r.req.Auth.Password,
		Domain:   r.req.Auth.Domain,
		Response: r.req.Auth.Response,
	})
	if err != nil {
		return failedStep(step, err, nil)
	}
	r.session = session

	capabilities, err := r.client.ListCapabilities(ctx, "")
	if err != nil {
		return failedStep(step, withVerifyCommand(err, "listCapabilities"), nil)
	}
	return succeededStep(step, stepResultOptions{
		Resource: &MoldModel.ResourceResult{
			Name:       "session",
			Status:     "authenticated",
			VerifiedBy: "listCapabilities",
			Raw: map[string]any{
				"endpoint":     session.Endpoint,
				"ccvm_ip":      session.CCVMIP,
				"capabilities": capabilities,
			},
		},
		Raw:    map[string]any{"endpoint": session.Endpoint, "ccvm_ip": session.CCVMIP},
		Verify: capabilities,
	})
}

func (r *BootstrapRuntime) ensureZone(ctx context.Context, step MoldModel.BootstrapJobStep) MoldModel.BootstrapStepResult {
	if err := r.ensureSession(ctx); err != nil {
		return failedStep(step, err, nil)
	}
	verify, item, err := r.findZone(ctx)
	if err != nil {
		return failedStep(step, err, verify)
	}
	if item != nil {
		resource := resourceFromItem(item, "listZones")
		r.zoneID = resource.ID
		return succeededStep(step, stepResultOptions{Resource: resource, Existing: true, Verify: verify})
	}
	params := url.Values{}
	setParam(params, "name", r.req.Zone.Name)
	setParam(params, "dns1", r.req.Zone.DNS1)
	setParam(params, "dns2", r.req.Zone.DNS2)
	setParam(params, "internaldns1", r.req.Zone.InternalDNS1)
	setParam(params, "internaldns2", r.req.Zone.InternalDNS2)
	setParam(params, "networktype", r.req.Zone.NetworkType)
	setParam(params, "guestcidraddress", r.req.Zone.GuestCIDRAddress)
	setParam(params, "domain", r.req.Zone.Domain)
	setParam(params, "localstorageenabled", r.req.Zone.LocalStorageEnabled)
	setParam(params, "securitygroupenabled", r.req.Zone.SecurityGroupEnabled)
	raw, err := r.applyCommand(ctx, "createZone", params)
	if err != nil {
		return failedStep(step, err, raw)
	}

	verify, item, err = r.findZone(ctx)
	if err != nil {
		return failedStep(step, withVerifyCommand(err, "listZones"), verify)
	}
	if item == nil {
		return failedStep(step, fmt.Errorf("createZone succeeded but listZones did not return zone %q", r.req.Zone.Name), verify)
	}
	resource := resourceFromItem(item, "listZones")
	r.zoneID = resource.ID
	return succeededStep(step, stepResultOptions{Resource: resource, Created: true, Raw: raw, Verify: verify})
}

func (r *BootstrapRuntime) ensurePhysicalNetwork(ctx context.Context, step MoldModel.BootstrapJobStep) MoldModel.BootstrapStepResult {
	if err := r.requireZoneID(); err != nil {
		return failedStep(step, err, nil)
	}
	verify, item, err := r.findPhysicalNetwork(ctx)
	if err != nil {
		return failedStep(step, err, verify)
	}
	if item != nil {
		resource := resourceFromItem(item, "listPhysicalNetworks")
		r.physicalNetworkID = resource.ID
		return succeededStep(step, stepResultOptions{Resource: resource, Existing: true, Verify: verify})
	}
	params := url.Values{}
	setParam(params, "zoneid", r.zoneID)
	setParam(params, "name", r.req.PhysicalNetwork.Name)
	setParam(params, "broadcastdomainrange", r.req.PhysicalNetwork.BroadcastDomainRange)
	setParam(params, "isolationmethods", r.req.PhysicalNetwork.IsolationMethods)
	setParam(params, "networkspeed", r.req.PhysicalNetwork.NetworkSpeed)
	setParam(params, "vlan", r.req.PhysicalNetwork.VLAN)
	setParam(params, "tags", r.req.PhysicalNetwork.Tags)
	raw, err := r.applyCommand(ctx, "createPhysicalNetwork", params)
	if err != nil {
		return failedStep(step, err, raw)
	}
	verify, item, err = r.findPhysicalNetwork(ctx)
	if err != nil {
		return failedStep(step, withVerifyCommand(err, "listPhysicalNetworks"), verify)
	}
	if item == nil {
		return failedStep(step, fmt.Errorf("createPhysicalNetwork succeeded but listPhysicalNetworks did not return physical network %q", r.req.PhysicalNetwork.Name), verify)
	}
	resource := resourceFromItem(item, "listPhysicalNetworks")
	r.physicalNetworkID = resource.ID
	return succeededStep(step, stepResultOptions{Resource: resource, Created: true, Raw: raw, Verify: verify})
}

func (r *BootstrapRuntime) ensurePod(ctx context.Context, step MoldModel.BootstrapJobStep) MoldModel.BootstrapStepResult {
	if err := r.requireZoneID(); err != nil {
		return failedStep(step, err, nil)
	}
	verify, item, err := r.findPod(ctx)
	if err != nil {
		return failedStep(step, err, verify)
	}
	if item != nil {
		resource := resourceFromItem(item, "listPods")
		r.podID = resource.ID
		return succeededStep(step, stepResultOptions{Resource: resource, Existing: true, Verify: verify})
	}

	params := url.Values{}
	setParam(params, "zoneid", r.zoneID)
	setParam(params, "name", r.req.Pod.Name)
	setParam(params, "gateway", r.req.Pod.Gateway)
	setParam(params, "netmask", r.req.Pod.Netmask)
	setParam(params, "startip", r.req.Pod.StartIP)
	setParam(params, "endip", r.req.Pod.EndIP)
	raw, err := r.applyCommand(ctx, "createPod", params)
	if err != nil {
		return failedStep(step, err, raw)
	}
	verify, item, err = r.findPod(ctx)
	if err != nil {
		return failedStep(step, withVerifyCommand(err, "listPods"), verify)
	}
	if item == nil {
		return failedStep(step, fmt.Errorf("createPod succeeded but listPods did not return pod %q", r.req.Pod.Name), verify)
	}
	resource := resourceFromItem(item, "listPods")
	r.podID = resource.ID
	return succeededStep(step, stepResultOptions{Resource: resource, Created: true, Raw: raw, Verify: verify})
}

func (r *BootstrapRuntime) ensurePublicIPRange(ctx context.Context, step MoldModel.BootstrapJobStep) MoldModel.BootstrapStepResult {
	if err := r.requireZonePodPhysicalNetworkID(); err != nil {
		return failedStep(step, err, nil)
	}
	verify, item, err := r.findPublicIPRange(ctx)
	if err != nil {
		return failedStep(step, err, verify)
	}
	if item != nil {
		resource := resourceFromItem(item, "listVlanIpRanges")
		resource.Name = r.req.PublicIP.StartIP + "-" + r.req.PublicIP.EndIP
		return succeededStep(step, stepResultOptions{Resource: resource, Existing: true, Verify: verify})
	}
	params := url.Values{}
	setParam(params, "zoneid", r.zoneID)
	setParam(params, "physicalnetworkid", r.physicalNetworkID)
	setParam(params, "gateway", r.req.PublicIP.Gateway)
	setParam(params, "netmask", r.req.PublicIP.Netmask)
	setParam(params, "startip", r.req.PublicIP.StartIP)
	setParam(params, "endip", r.req.PublicIP.EndIP)
	setParam(params, "forvirtualnetwork", "true")
	raw, err := r.applyCommand(ctx, "createVlanIpRange", params)
	if err != nil {
		return failedStep(step, err, raw)
	}
	verify, item, err = r.findPublicIPRange(ctx)
	if err != nil {
		return failedStep(step, withVerifyCommand(err, "listVlanIpRanges"), verify)
	}
	if item == nil {
		return failedStep(step, fmt.Errorf("createVlanIpRange succeeded but listVlanIpRanges did not return %s-%s", r.req.PublicIP.StartIP, r.req.PublicIP.EndIP), verify)
	}
	resource := resourceFromItem(item, "listVlanIpRanges")
	resource.Name = r.req.PublicIP.StartIP + "-" + r.req.PublicIP.EndIP
	return succeededStep(step, stepResultOptions{Resource: resource, Created: true, Raw: raw, Verify: verify})
}

func (r *BootstrapRuntime) ensureCluster(ctx context.Context, step MoldModel.BootstrapJobStep) MoldModel.BootstrapStepResult {
	if err := r.requireZonePodID(); err != nil {
		return failedStep(step, err, nil)
	}
	verify, item, err := r.findCluster(ctx)
	if err != nil {
		return failedStep(step, err, verify)
	}
	if item != nil {
		resource := resourceFromItem(item, "listClusters")
		r.clusterID = resource.ID
		return succeededStep(step, stepResultOptions{Resource: resource, Existing: true, Verify: verify})
	}

	params := url.Values{}
	setParam(params, "zoneid", r.zoneID)
	setParam(params, "podid", r.podID)
	setParam(params, "clustername", r.req.Cluster.Name)
	setParam(params, "clustertype", r.req.Cluster.ClusterType)
	setParam(params, "hypervisor", r.req.Cluster.Hypervisor)
	setParam(params, "arch", r.req.Cluster.Arch)
	raw, err := r.applyCommand(ctx, "addCluster", params)
	if err != nil {
		return failedStep(step, err, raw)
	}
	verify, item, err = r.findCluster(ctx)
	if err != nil {
		return failedStep(step, withVerifyCommand(err, "listClusters"), verify)
	}
	if item == nil {
		return failedStep(step, fmt.Errorf("addCluster succeeded but listClusters did not return cluster %q", r.req.Cluster.Name), verify)
	}
	resource := resourceFromItem(item, "listClusters")
	r.clusterID = resource.ID
	return succeededStep(step, stepResultOptions{Resource: resource, Created: true, Raw: raw, Verify: verify})
}

func (r *BootstrapRuntime) ensureHosts(ctx context.Context, step MoldModel.BootstrapJobStep) MoldModel.BootstrapStepResult {
	if err := r.requireZonePodClusterID(); err != nil {
		return failedStep(step, err, nil)
	}
	resources := make([]MoldModel.ResourceResult, 0, len(r.req.Hosts))
	created := false
	var rawResponses []any
	var verifyResponses []any

	for _, host := range r.req.Hosts {
		verify, item, err := r.findHost(ctx, host)
		verifyResponses = append(verifyResponses, verify)
		if err != nil {
			return failedStep(step, err, verify)
		}
		if item == nil {
			params := url.Values{}
			setParam(params, "zoneid", r.zoneID)
			setParam(params, "podid", r.podID)
			setParam(params, "clusterid", r.clusterID)
			setParam(params, "url", host.URL)
			setParam(params, "username", host.Username)
			setParam(params, "password", host.Password)
			setParam(params, "hypervisor", firstNonEmpty(host.Hypervisor, r.req.Cluster.Hypervisor))
			setParam(params, "hosttags", host.HostTags)
			raw, err := r.applyCommand(ctx, "addHost", params)
			rawResponses = append(rawResponses, raw)
			if err != nil {
				return failedStep(step, err, raw)
			}
			created = true
			verify, item, err = r.findHost(ctx, host)
			verifyResponses = append(verifyResponses, verify)
			if err != nil {
				return failedStep(step, withVerifyCommand(err, "listHosts"), verify)
			}
		}
		if item == nil {
			return failedStep(step, fmt.Errorf("addHost succeeded but listHosts did not return host %q", host.URL), verifyResponses)
		}
		resources = append(resources, *resourceFromItem(item, "listHosts"))
	}
	return succeededStep(step, stepResultOptions{Resources: resources, Created: created, Existing: !created, Raw: rawResponses, Verify: verifyResponses})
}

func (r *BootstrapRuntime) ensurePrimaryStorage(ctx context.Context, step MoldModel.BootstrapJobStep) MoldModel.BootstrapStepResult {
	if err := r.requireZonePodClusterID(); err != nil {
		return failedStep(step, err, nil)
	}
	verify, item, err := r.findPrimaryStorage(ctx)
	if err != nil {
		return failedStep(step, err, verify)
	}
	if item != nil {
		resource := resourceFromItem(item, "listStoragePools")
		r.primaryStorageID = resource.ID
		return succeededStep(step, stepResultOptions{Resource: resource, Existing: true, Verify: verify})
	}
	storageURL, err := r.resolvePrimaryStorageURL(ctx)
	if err != nil {
		return failedStep(step, err, nil)
	}

	params := url.Values{}
	setParam(params, "zoneid", r.zoneID)
	setParam(params, "podid", r.podID)
	setParam(params, "clusterid", r.clusterID)
	setParam(params, "name", r.req.PrimaryStorage.Name)
	setParam(params, "url", storageURL)
	setParam(params, "scope", firstNonEmpty(r.req.PrimaryStorage.Scope, "cluster"))
	setParam(params, "provider", firstNonEmpty(r.req.PrimaryStorage.Provider, "DefaultPrimary"))
	setParam(params, "hypervisor", firstNonEmpty(r.req.PrimaryStorage.Hypervisor, r.req.Cluster.Hypervisor))
	setParam(params, "tags", r.req.PrimaryStorage.Tags)
	setParam(params, "krbdPath", r.req.PrimaryStorage.KRBDPath)
	setParam(params, "details[0].provider", r.req.PrimaryStorage.DetailsProvider)
	raw, err := r.applyCommand(ctx, "createStoragePool", params)
	if err != nil {
		return failedStep(step, err, raw)
	}
	verify, item, err = r.findPrimaryStorage(ctx)
	if err != nil {
		return failedStep(step, withVerifyCommand(err, "listStoragePools"), verify)
	}
	if item == nil {
		return failedStep(step, fmt.Errorf("createStoragePool succeeded but listStoragePools did not return storage pool %q", r.req.PrimaryStorage.Name), verify)
	}
	resource := resourceFromItem(item, "listStoragePools")
	r.primaryStorageID = resource.ID
	return succeededStep(step, stepResultOptions{Resource: resource, Created: true, Raw: raw, Verify: verify})
}

func (r *BootstrapRuntime) resolvePrimaryStorageURL(ctx context.Context) (string, error) {
	storage := r.req.PrimaryStorage
	if !storage.AutoRBDSecret {
		return strings.TrimSpace(storage.URL), nil
	}
	if strings.TrimSpace(storage.RADOSMonitors) == "" || strings.TrimSpace(storage.RADOSPool) == "" || strings.TrimSpace(storage.RADOSUser) == "" {
		return "", fmt.Errorf("automatic Glue Block configuration requires monitors, pool and user")
	}
	keyFetcher := r.cephKey
	if keyFetcher == nil {
		keyFetcher = fetchCephAdminKey
	}
	secret, err := keyFetcher(ctx, storage.CephKeyHosts)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("rbd://%s:%s@%s/%s",
		storage.RADOSUser,
		secret,
		storage.RADOSMonitors,
		strings.TrimPrefix(storage.RADOSPool, "/"),
	), nil
}

func fetchCephAdminKey(ctx context.Context, hosts []string) (string, error) {
	if len(hosts) == 0 {
		return "", fmt.Errorf("clusterConfig.hosts[].ablecube required to read client.admin key")
	}
	identity := resolveMoldHostSSHKey()
	var lastErr error
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		command := exec.CommandContext(ctx,
			"ssh",
			"-i", identity,
			"-o", "BatchMode=yes",
			"-o", "ConnectTimeout=10",
			"-o", "StrictHostKeyChecking=accept-new",
			"root@"+host,
			"ceph", "auth", "get-key", "client.admin",
		)
		output, err := command.Output()
		if err != nil {
			lastErr = err
			continue
		}
		secret := strings.TrimSpace(string(output))
		if secret != "" {
			return secret, nil
		}
		lastErr = fmt.Errorf("empty client.admin key")
	}
	return "", fmt.Errorf("failed to read Ceph client.admin key from configured hosts using %s: %w", identity, lastErr)
}

func resolveMoldHostSSHKey() string {
	if configured := strings.TrimSpace(os.Getenv("ABLESTACK_MOLD_HOST_SSH_KEY")); configured != "" {
		return configured
	}
	for _, candidate := range []string{
		"/var/cloudstack/management/.ssh/id_rsa",
		"/var/lib/cloudstack/management/.ssh/id_rsa",
		"/root/.ssh/id_rsa",
	} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return "/var/cloudstack/management/.ssh/id_rsa"
}

func (r *BootstrapRuntime) ensureSecondaryStorage(ctx context.Context, step MoldModel.BootstrapJobStep) MoldModel.BootstrapStepResult {
	if err := r.requireZoneID(); err != nil {
		return failedStep(step, err, nil)
	}
	verify, item, err := r.findSecondaryStorage(ctx)
	if err != nil {
		return failedStep(step, err, verify)
	}
	if item != nil {
		resource := resourceFromItem(item, "listImageStores")
		r.secondaryStorageID = resource.ID
		return succeededStep(step, stepResultOptions{Resource: resource, Existing: true, Verify: verify})
	}

	params := url.Values{}
	setParam(params, "zoneid", r.zoneID)
	setParam(params, "url", r.req.SecondaryStorage.URL)
	setParam(params, "name", firstNonEmpty(r.req.SecondaryStorage.Name, "Secondary Storage"))
	setParam(params, "provider", firstNonEmpty(r.req.SecondaryStorage.Provider, "NFS"))
	raw, err := r.applyCommand(ctx, "addImageStore", params)
	if err != nil {
		return failedStep(step, err, raw)
	}
	verify, item, err = r.findSecondaryStorage(ctx)
	if err != nil {
		return failedStep(step, withVerifyCommand(err, "listImageStores"), verify)
	}
	if item == nil {
		return failedStep(step, fmt.Errorf("addImageStore succeeded but listImageStores did not return secondary storage %q", r.req.SecondaryStorage.URL), verify)
	}
	resource := resourceFromItem(item, "listImageStores")
	r.secondaryStorageID = resource.ID
	return succeededStep(step, stepResultOptions{Resource: resource, Created: true, Raw: raw, Verify: verify})
}

func (r *BootstrapRuntime) ensureSecondaryStagingStore(ctx context.Context, step MoldModel.BootstrapJobStep) MoldModel.BootstrapStepResult {
	if r.req.SecondaryStagingStore == nil {
		return succeededStep(step, stepResultOptions{Existing: true, Verify: map[string]any{"skipped": "secondary_staging_store not requested"}})
	}
	if err := r.requireZoneID(); err != nil {
		return failedStep(step, err, nil)
	}
	verify, item, err := r.findSecondaryStagingStore(ctx)
	if err != nil {
		return failedStep(step, err, verify)
	}
	if item != nil {
		resource := resourceFromItem(item, "listSecondaryStagingStores")
		r.secondaryStagingStoreID = resource.ID
		return succeededStep(step, stepResultOptions{Resource: resource, Existing: true, Verify: verify})
	}

	params := url.Values{}
	setParam(params, "zoneid", r.zoneID)
	setParam(params, "url", r.req.SecondaryStagingStore.URL)
	setParam(params, "provider", firstNonEmpty(r.req.SecondaryStagingStore.Provider, "NFS"))
	setParam(params, "scope", firstNonEmpty(r.req.SecondaryStagingStore.Scope, "zone"))
	raw, err := r.applyCommand(ctx, "createSecondaryStagingStore", params)
	if err != nil {
		return failedStep(step, err, raw)
	}
	verify, item, err = r.findSecondaryStagingStore(ctx)
	if err != nil {
		return failedStep(step, withVerifyCommand(err, "listSecondaryStagingStores"), verify)
	}
	if item == nil {
		return failedStep(step, fmt.Errorf("createSecondaryStagingStore succeeded but listSecondaryStagingStores did not return staging store %q", r.req.SecondaryStagingStore.URL), verify)
	}
	resource := resourceFromItem(item, "listSecondaryStagingStores")
	r.secondaryStagingStoreID = resource.ID
	return succeededStep(step, stepResultOptions{Resource: resource, Created: true, Raw: raw, Verify: verify})
}

func (r *BootstrapRuntime) enableZone(ctx context.Context, step MoldModel.BootstrapJobStep) MoldModel.BootstrapStepResult {
	if err := r.requireZoneID(); err != nil {
		return failedStep(step, err, nil)
	}
	verify, item, err := r.findZone(ctx)
	if err != nil {
		return failedStep(step, err, verify)
	}
	if item != nil && strings.EqualFold(stringValue(item["allocationstate"]), "Enabled") {
		resource := resourceFromItem(item, "listZones")
		return succeededStep(step, stepResultOptions{Resource: resource, Existing: true, Verify: verify})
	}

	params := url.Values{}
	setParam(params, "id", r.zoneID)
	setParam(params, "allocationstate", "Enabled")
	raw, err := r.applyCommand(ctx, "updateZone", params)
	if err != nil {
		return failedStep(step, err, raw)
	}
	verify, item, err = r.findZone(ctx)
	if err != nil {
		return failedStep(step, withVerifyCommand(err, "listZones"), verify)
	}
	if item == nil || !strings.EqualFold(stringValue(item["allocationstate"]), "Enabled") {
		return failedStep(step, fmt.Errorf("updateZone succeeded but listZones did not return allocationstate=Enabled for zone %q", r.req.Zone.Name), verify)
	}
	resource := resourceFromItem(item, "listZones")
	return succeededStep(step, stepResultOptions{Resource: resource, Created: false, Raw: raw, Verify: verify})
}

func (r *BootstrapRuntime) waitSystemVMRunning(ctx context.Context, step MoldModel.BootstrapJobStep) MoldModel.BootstrapStepResult {
	if err := r.requireZoneID(); err != nil {
		return failedStep(step, err, nil)
	}
	timeout := durationFromEnv("ABLESTACK_MOLD_SYSTEM_VM_TIMEOUT", defaultSystemVMPollTimeout)
	interval := durationFromEnv("ABLESTACK_MOLD_SYSTEM_VM_POLL_INTERVAL", defaultSystemVMPollInterval)
	deadline := time.Now().Add(timeout)
	var last map[string]any
	var resources []MoldModel.ResourceResult
	for {
		resp, items, err := r.listSystemVMs(ctx)
		last = resp
		if err != nil {
			return failedStep(step, err, resp)
		}
		resources = resourcesFromItems(items, "listSystemVms")
		if len(items) > 0 && allSystemVMsRunning(items) {
			return succeededStep(step, stepResultOptions{Resources: resources, Verify: resp})
		}
		if time.Now().After(deadline) {
			return failedStep(step, fmt.Errorf("timed out waiting for system VMs to reach Running state"), last)
		}
		if err := sleepContext(ctx, interval); err != nil {
			return failedStep(step, err, last)
		}
	}
}

func (r *BootstrapRuntime) finalHealthCheck(ctx context.Context, step MoldModel.BootstrapJobStep) MoldModel.BootstrapStepResult {
	if err := r.requireZonePodClusterID(); err != nil {
		return failedStep(step, err, nil)
	}
	results := make([]MoldModel.ResourceResult, 0)
	raw := map[string]any{}

	if resp, item, err := r.findZone(ctx); err != nil {
		return failedStep(step, withVerifyCommand(err, "listZones"), resp)
	} else if item == nil {
		return failedStep(step, fmt.Errorf("final health check failed: zone %q not found", r.req.Zone.Name), resp)
	} else {
		raw["zones"] = resp
		results = append(results, *resourceFromItem(item, "listZones"))
	}
	if resp, item, err := r.findPhysicalNetwork(ctx); err != nil {
		return failedStep(step, withVerifyCommand(err, "listPhysicalNetworks"), resp)
	} else if item == nil || !strings.EqualFold(stringValue(item["state"]), "Enabled") {
		return failedStep(step, fmt.Errorf("final health check failed: enabled physical network %q not found", r.req.PhysicalNetwork.Name), resp)
	} else {
		raw["physical_networks"] = resp
		results = append(results, *resourceFromItem(item, "listPhysicalNetworks"))
	}
	for _, trafficType := range []string{"Management", "Guest", "Public"} {
		if resp, item, err := r.findTrafficType(ctx, trafficType); err != nil {
			return failedStep(step, withVerifyCommand(err, "listTrafficTypes"), resp)
		} else if item == nil || !strings.EqualFold(stringValue(item["kvmnetworklabel"]), "bridge0") {
			return failedStep(step, fmt.Errorf("final health check failed: traffic type %s with bridge0 not found", trafficType), resp)
		} else {
			raw["traffic_types"] = resp
			results = append(results, *resourceFromItem(item, "listTrafficTypes"))
		}
	}
	for _, providerName := range []string{"VirtualRouter", "VpcVirtualRouter"} {
		if resp, item, err := r.findNetworkProvider(ctx, providerName); err != nil {
			return failedStep(step, withVerifyCommand(err, "listNetworkServiceProviders"), resp)
		} else if item == nil || !strings.EqualFold(stringValue(item["state"]), "Enabled") {
			return failedStep(step, fmt.Errorf("final health check failed: enabled network provider %s not found", providerName), resp)
		} else {
			raw["network_providers"] = resp
			results = append(results, *resourceFromItem(item, "listNetworkServiceProviders"))
			providerID := stringValue(item["id"])
			if elementResp, element, elementErr := r.findVirtualRouterElement(ctx, providerID); elementErr != nil {
				return failedStep(step, withVerifyCommand(elementErr, "listVirtualRouterElements"), elementResp)
			} else if element == nil || !boolValue(element["enabled"]) {
				return failedStep(step, fmt.Errorf("final health check failed: virtual router element for %s is not enabled", providerName), elementResp)
			} else {
				raw["virtual_router_elements"] = elementResp
			}
		}
	}
	if resp, item, err := r.findPod(ctx); err != nil {
		return failedStep(step, withVerifyCommand(err, "listPods"), resp)
	} else if item == nil {
		return failedStep(step, fmt.Errorf("final health check failed: pod %q not found", r.req.Pod.Name), resp)
	} else {
		raw["pods"] = resp
		results = append(results, *resourceFromItem(item, "listPods"))
	}
	if resp, item, err := r.findPublicIPRange(ctx); err != nil {
		return failedStep(step, withVerifyCommand(err, "listVlanIpRanges"), resp)
	} else if item == nil {
		return failedStep(step, fmt.Errorf("final health check failed: public IP range %s-%s not found", r.req.PublicIP.StartIP, r.req.PublicIP.EndIP), resp)
	} else {
		raw["public_ip_ranges"] = resp
		results = append(results, *resourceFromItem(item, "listVlanIpRanges"))
	}
	if resp, item, err := r.findCluster(ctx); err != nil {
		return failedStep(step, withVerifyCommand(err, "listClusters"), resp)
	} else if item == nil {
		return failedStep(step, fmt.Errorf("final health check failed: cluster %q not found", r.req.Cluster.Name), resp)
	} else {
		raw["clusters"] = resp
		results = append(results, *resourceFromItem(item, "listClusters"))
	}
	for _, host := range r.req.Hosts {
		if resp, item, err := r.findHost(ctx, host); err != nil {
			return failedStep(step, withVerifyCommand(err, "listHosts"), resp)
		} else if item == nil {
			return failedStep(step, fmt.Errorf("final health check failed: host %q not found", host.URL), resp)
		} else {
			raw["hosts"] = resp
			results = append(results, *resourceFromItem(item, "listHosts"))
		}
	}
	if resp, item, err := r.findPrimaryStorage(ctx); err != nil {
		return failedStep(step, withVerifyCommand(err, "listStoragePools"), resp)
	} else if item == nil {
		return failedStep(step, fmt.Errorf("final health check failed: primary storage %q not found", r.req.PrimaryStorage.Name), resp)
	} else {
		raw["storage_pools"] = resp
		results = append(results, *resourceFromItem(item, "listStoragePools"))
	}
	if resp, item, err := r.findSecondaryStorage(ctx); err != nil {
		return failedStep(step, withVerifyCommand(err, "listImageStores"), resp)
	} else if item == nil {
		return failedStep(step, fmt.Errorf("final health check failed: secondary storage %q not found", r.req.SecondaryStorage.URL), resp)
	} else {
		raw["image_stores"] = resp
		results = append(results, *resourceFromItem(item, "listImageStores"))
	}
	if r.req.SecondaryStagingStore != nil {
		if resp, item, err := r.findSecondaryStagingStore(ctx); err != nil {
			return failedStep(step, withVerifyCommand(err, "listSecondaryStagingStores"), resp)
		} else if item == nil {
			return failedStep(step, fmt.Errorf("final health check failed: secondary staging store %q not found", r.req.SecondaryStagingStore.URL), resp)
		} else {
			raw["secondary_staging_stores"] = resp
			results = append(results, *resourceFromItem(item, "listSecondaryStagingStores"))
		}
	}
	return succeededStep(step, stepResultOptions{Resources: results, Verify: raw})
}

func (r *BootstrapRuntime) ensureClient() error {
	if r.client != nil {
		return nil
	}
	endpoint, ccvmIP, _, err := ResolveMoldEndpointFromCluster()
	if err != nil {
		return err
	}
	r.endpoint = endpoint
	r.ccvmIP = ccvmIP
	r.client = NewClient(endpoint, WithCCVMIP(ccvmIP))
	return nil
}

func (r *BootstrapRuntime) ensureSession(ctx context.Context) error {
	if err := r.ensureClient(); err != nil {
		return err
	}
	if strings.TrimSpace(r.session.SessionKey) != "" {
		return nil
	}
	session, err := r.client.Login(ctx, AuthConfig{
		Username: r.req.Auth.Username,
		Password: r.req.Auth.Password,
		Domain:   r.req.Auth.Domain,
		Response: r.req.Auth.Response,
	})
	if err != nil {
		return err
	}
	r.session = session
	return nil
}

func (r *BootstrapRuntime) callList(ctx context.Context, command string, params url.Values) (map[string]any, error) {
	if err := r.ensureSession(ctx); err != nil {
		return nil, err
	}
	return r.client.CallCommand(ctx, http.MethodGet, command, "", params)
}

func (r *BootstrapRuntime) applyCommand(ctx context.Context, command string, params url.Values) (map[string]any, error) {
	if err := r.ensureSession(ctx); err != nil {
		return nil, err
	}
	resp, err := r.client.CallCommand(ctx, http.MethodPost, command, "", params)
	if err != nil {
		return resp, err
	}
	jobID := findStringKey(resp, "jobid", "jobId", "asyncjobid")
	if jobID == "" {
		return resp, nil
	}
	asyncResp, err := r.pollAsyncJob(ctx, jobID)
	if err != nil {
		if apiErr, ok := APIErrorFrom(err); ok {
			copied := *apiErr
			copied.Command = command
			copied.VerifyCommand = "queryAsyncJobResult"
			return asyncResp, &copied
		}
		return asyncResp, err
	}
	return asyncResp, nil
}

func (r *BootstrapRuntime) pollAsyncJob(ctx context.Context, jobID string) (map[string]any, error) {
	timeout := durationFromEnv("ABLESTACK_MOLD_ASYNC_TIMEOUT", defaultAsyncPollTimeout)
	interval := durationFromEnv("ABLESTACK_MOLD_ASYNC_POLL_INTERVAL", defaultAsyncPollInterval)
	deadline := time.Now().Add(timeout)
	params := url.Values{}
	params.Set("jobid", jobID)
	for {
		resp, err := r.client.CallCommand(ctx, http.MethodGet, "queryAsyncJobResult", "", params)
		if err != nil {
			return resp, err
		}
		status := asyncJobStatus(resp)
		switch status {
		case 0:
			if time.Now().After(deadline) {
				return resp, fmt.Errorf("timed out waiting for async job %s", jobID)
			}
			if err := sleepContext(ctx, interval); err != nil {
				return resp, err
			}
		case 1:
			return resp, nil
		case 2:
			return resp, &APIError{
				Kind:      ErrMoldErrorResponse,
				Command:   "queryAsyncJobResult",
				ErrorText: firstNonEmpty(findStringKey(resp, "errortext"), "async job failed"),
				Raw:       resp,
			}
		default:
			return resp, fmt.Errorf("queryAsyncJobResult returned unknown jobstatus=%d for job %s", status, jobID)
		}
	}
}

func (r *BootstrapRuntime) findZone(ctx context.Context) (map[string]any, map[string]any, error) {
	params := url.Values{}
	setParam(params, "name", r.req.Zone.Name)
	if r.zoneID != "" {
		setParam(params, "id", r.zoneID)
	}
	resp, err := r.callList(ctx, "listZones", params)
	if err != nil {
		return resp, nil, err
	}
	item := findListItem(resp, "listzonesresponse", "zone", func(item map[string]any) bool {
		return matchIDOrName(item, r.zoneID, r.req.Zone.Name)
	})
	return resp, item, nil
}

func (r *BootstrapRuntime) findPhysicalNetwork(ctx context.Context) (map[string]any, map[string]any, error) {
	params := url.Values{}
	setParam(params, "zoneid", r.zoneID)
	resp, err := r.callList(ctx, "listPhysicalNetworks", params)
	if err != nil {
		return resp, nil, err
	}
	item := findListItem(resp, "listphysicalnetworksresponse", "physicalnetwork", func(item map[string]any) bool {
		return matchIDOrName(item, r.physicalNetworkID, r.req.PhysicalNetwork.Name)
	})
	return resp, item, nil
}

func (r *BootstrapRuntime) findTrafficType(ctx context.Context, trafficType string) (map[string]any, map[string]any, error) {
	params := url.Values{}
	setParam(params, "physicalnetworkid", r.physicalNetworkID)
	setParam(params, "traffictype", trafficType)
	resp, err := r.callList(ctx, "listTrafficTypes", params)
	if err != nil {
		return resp, nil, err
	}
	item := findListItem(resp, "listtraffictypesresponse", "traffictype", func(item map[string]any) bool {
		return strings.EqualFold(stringValue(item["traffictype"]), trafficType)
	})
	return resp, item, nil
}

func (r *BootstrapRuntime) findNetworkProvider(ctx context.Context, name string) (map[string]any, map[string]any, error) {
	params := url.Values{}
	setParam(params, "physicalnetworkid", r.physicalNetworkID)
	setParam(params, "name", name)
	resp, err := r.callList(ctx, "listNetworkServiceProviders", params)
	if err != nil {
		return resp, nil, err
	}
	item := findListItem(resp, "listnetworkserviceprovidersresponse", "networkserviceprovider", func(item map[string]any) bool {
		return strings.EqualFold(stringValue(item["name"]), name)
	})
	return resp, item, nil
}

func (r *BootstrapRuntime) findVirtualRouterElement(ctx context.Context, providerID string) (map[string]any, map[string]any, error) {
	params := url.Values{}
	setParam(params, "nspid", providerID)
	resp, err := r.callList(ctx, "listVirtualRouterElements", params)
	if err != nil {
		return resp, nil, err
	}
	item := findListItem(resp, "listvirtualrouterelementsresponse", "virtualrouterelement", nil)
	return resp, item, nil
}

func (r *BootstrapRuntime) findPublicIPRange(ctx context.Context) (map[string]any, map[string]any, error) {
	params := url.Values{}
	setParam(params, "zoneid", r.zoneID)
	setParam(params, "physicalnetworkid", r.physicalNetworkID)
	resp, err := r.callList(ctx, "listVlanIpRanges", params)
	if err != nil {
		return resp, nil, err
	}
	item := findListItem(resp, "listvlaniprangesresponse", "vlaniprange", func(item map[string]any) bool {
		return stringValue(item["startip"]) == r.req.PublicIP.StartIP && stringValue(item["endip"]) == r.req.PublicIP.EndIP
	})
	return resp, item, nil
}

func (r *BootstrapRuntime) findPod(ctx context.Context) (map[string]any, map[string]any, error) {
	params := url.Values{}
	setParam(params, "zoneid", r.zoneID)
	setParam(params, "name", r.req.Pod.Name)
	if r.podID != "" {
		setParam(params, "id", r.podID)
	}
	resp, err := r.callList(ctx, "listPods", params)
	if err != nil {
		return resp, nil, err
	}
	item := findListItem(resp, "listpodsresponse", "pod", func(item map[string]any) bool {
		return matchIDOrName(item, r.podID, r.req.Pod.Name)
	})
	return resp, item, nil
}

func (r *BootstrapRuntime) findCluster(ctx context.Context) (map[string]any, map[string]any, error) {
	params := url.Values{}
	setParam(params, "zoneid", r.zoneID)
	setParam(params, "podid", r.podID)
	setParam(params, "name", r.req.Cluster.Name)
	if r.clusterID != "" {
		setParam(params, "id", r.clusterID)
	}
	resp, err := r.callList(ctx, "listClusters", params)
	if err != nil {
		return resp, nil, err
	}
	item := findListItem(resp, "listclustersresponse", "cluster", func(item map[string]any) bool {
		return matchIDOrName(item, r.clusterID, r.req.Cluster.Name)
	})
	return resp, item, nil
}

func (r *BootstrapRuntime) findHost(ctx context.Context, host MoldModel.HostRequest) (map[string]any, map[string]any, error) {
	params := url.Values{}
	setParam(params, "zoneid", r.zoneID)
	setParam(params, "podid", r.podID)
	setParam(params, "clusterid", r.clusterID)
	resp, err := r.callList(ctx, "listHosts", params)
	if err != nil {
		return resp, nil, err
	}
	item := findListItem(resp, "listhostsresponse", "host", func(item map[string]any) bool {
		return matchHost(item, host.URL)
	})
	return resp, item, nil
}

func (r *BootstrapRuntime) findPrimaryStorage(ctx context.Context) (map[string]any, map[string]any, error) {
	params := url.Values{}
	setParam(params, "zoneid", r.zoneID)
	setParam(params, "podid", r.podID)
	setParam(params, "clusterid", r.clusterID)
	setParam(params, "name", r.req.PrimaryStorage.Name)
	resp, err := r.callList(ctx, "listStoragePools", params)
	if err != nil {
		return resp, nil, err
	}
	item := findListItem(resp, "liststoragepoolsresponse", "storagepool", func(item map[string]any) bool {
		return matchIDOrName(item, r.primaryStorageID, r.req.PrimaryStorage.Name) || matchURL(item, r.req.PrimaryStorage.URL)
	})
	return resp, item, nil
}

func (r *BootstrapRuntime) findSecondaryStorage(ctx context.Context) (map[string]any, map[string]any, error) {
	params := url.Values{}
	setParam(params, "zoneid", r.zoneID)
	resp, err := r.callList(ctx, "listImageStores", params)
	if err != nil {
		return resp, nil, err
	}
	item := findListItem(resp, "listimagestoresresponse", "imagestore", func(item map[string]any) bool {
		return matchIDOrName(item, r.secondaryStorageID, "") || matchURL(item, r.req.SecondaryStorage.URL)
	})
	return resp, item, nil
}

func (r *BootstrapRuntime) findSecondaryStagingStore(ctx context.Context) (map[string]any, map[string]any, error) {
	params := url.Values{}
	setParam(params, "zoneid", r.zoneID)
	resp, err := r.callList(ctx, "listSecondaryStagingStores", params)
	if err != nil {
		return resp, nil, err
	}
	item := findListItem(resp, "listsecondarystagingstoresresponse", "secondarystagingstore", func(item map[string]any) bool {
		return matchIDOrName(item, r.secondaryStagingStoreID, "") || matchURL(item, r.req.SecondaryStagingStore.URL)
	})
	return resp, item, nil
}

func (r *BootstrapRuntime) listSystemVMs(ctx context.Context) (map[string]any, []map[string]any, error) {
	params := url.Values{}
	setParam(params, "zoneid", r.zoneID)
	resp, err := r.callList(ctx, "listSystemVms", params)
	if err != nil {
		return resp, nil, err
	}
	items := listItems(resp, "listsystemvmsresponse", "systemvm")
	return resp, items, nil
}

func (r *BootstrapRuntime) requireZoneID() error {
	if strings.TrimSpace(r.zoneID) == "" {
		return fmt.Errorf("zone id required before this step")
	}
	return nil
}

func (r *BootstrapRuntime) requireZonePodID() error {
	if err := r.requireZoneID(); err != nil {
		return err
	}
	if strings.TrimSpace(r.podID) == "" {
		return fmt.Errorf("pod id required before this step")
	}
	return nil
}

func (r *BootstrapRuntime) requirePhysicalNetworkID() error {
	if err := r.requireZoneID(); err != nil {
		return err
	}
	if strings.TrimSpace(r.physicalNetworkID) == "" {
		return fmt.Errorf("physical network id required before this step")
	}
	return nil
}

func (r *BootstrapRuntime) requireZonePodPhysicalNetworkID() error {
	if err := r.requireZonePodID(); err != nil {
		return err
	}
	return r.requirePhysicalNetworkID()
}

func (r *BootstrapRuntime) requireZonePodClusterID() error {
	if err := r.requireZonePodID(); err != nil {
		return err
	}
	if strings.TrimSpace(r.clusterID) == "" {
		return fmt.Errorf("cluster id required before this step")
	}
	return nil
}

type stepResultOptions struct {
	Resource  *MoldModel.ResourceResult
	Resources []MoldModel.ResourceResult
	Raw       any
	Verify    any
	Created   bool
	Existing  bool
}

func succeededStep(step MoldModel.BootstrapJobStep, options stepResultOptions) MoldModel.BootstrapStepResult {
	return MoldModel.BootstrapStepResult{
		Name:           step.Name,
		Command:        step.Command,
		VerifyCommand:  step.VerifyCommand,
		ResultKey:      step.ResultKey,
		Status:         MoldModel.BootstrapStepStatusSucceeded,
		Created:        options.Created,
		Existing:       options.Existing,
		Verified:       true,
		VerifiedBy:     step.VerifyCommand,
		Resource:       options.Resource,
		Resources:      options.Resources,
		RawResponse:    options.Raw,
		VerifyResponse: options.Verify,
	}
}

func failedStep(step MoldModel.BootstrapJobStep, err error, raw any) MoldModel.BootstrapStepResult {
	message := ""
	apiErr, hasAPIError := APIErrorFrom(err)
	if err != nil {
		message = err.Error()
	}
	out := MoldModel.BootstrapStepResult{
		Name:          step.Name,
		Command:       step.Command,
		VerifyCommand: step.VerifyCommand,
		ResultKey:     step.ResultKey,
		Status:        MoldModel.BootstrapStepStatusFailed,
		Verified:      false,
		RawResponse:   raw,
		Error: &MoldModel.BootstrapError{
			Step:          step.Name,
			Command:       step.Command,
			VerifyCommand: step.VerifyCommand,
			Message:       message,
			ErrorText:     message,
			Raw:           raw,
		},
	}
	if hasAPIError {
		out.Error.Command = firstNonEmpty(apiErr.Command, out.Error.Command)
		out.Error.VerifyCommand = firstNonEmpty(apiErr.VerifyCommand, out.Error.VerifyCommand)
		out.Error.Method = apiErr.Method
		out.Error.Endpoint = apiErr.Endpoint
		out.Error.HTTPStatus = apiErr.HTTPStatus
		out.Error.HTTPBody = apiErr.HTTPBody
		out.Error.SessionKeyAttached = apiErr.SessionKeySet
		out.Error.Code = apiErr.Code
		out.Error.ErrorCode = apiErr.ErrorCode
		out.Error.ErrorText = firstNonEmpty(apiErr.ErrorText, apiErr.HTTPBody, message)
		out.Error.Raw = firstNonNil(apiErr.Raw, raw)
	}
	return out
}

func withVerifyCommand(err error, verifyCommand string) error {
	if apiErr, ok := APIErrorFrom(err); ok {
		copied := *apiErr
		copied.VerifyCommand = verifyCommand
		return &copied
	}
	return err
}

func findListItem(body map[string]any, responseKey string, itemKey string, match func(map[string]any) bool) map[string]any {
	for _, item := range listItems(body, responseKey, itemKey) {
		if match == nil || match(item) {
			return item
		}
	}
	return nil
}

func listItems(body map[string]any, responseKey string, itemKey string) []map[string]any {
	resp := responseMap(body, responseKey)
	if resp == nil {
		return nil
	}
	raw := resp[itemKey]
	switch typed := raw.(type) {
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, value := range typed {
			if item, ok := value.(map[string]any); ok {
				out = append(out, item)
			}
		}
		return out
	case []map[string]any:
		return typed
	case map[string]any:
		return []map[string]any{typed}
	default:
		return nil
	}
}

func responseMap(body map[string]any, responseKey string) map[string]any {
	if body == nil {
		return nil
	}
	if resp, ok := body[responseKey].(map[string]any); ok {
		return resp
	}
	for _, value := range body {
		if resp, ok := value.(map[string]any); ok {
			return resp
		}
	}
	return nil
}

func resourceFromItem(item map[string]any, verifiedBy string) *MoldModel.ResourceResult {
	if item == nil {
		return nil
	}
	return &MoldModel.ResourceResult{
		ID:         stringValue(item["id"]),
		Name:       firstNonEmpty(stringValue(item["name"]), stringValue(item["displayname"])),
		URL:        firstNonEmpty(stringValue(item["url"]), stringValue(item["path"])),
		State:      firstNonEmpty(stringValue(item["allocationstate"]), stringValue(item["state"])),
		Status:     firstNonEmpty(stringValue(item["status"]), stringValue(item["state"])),
		VerifiedBy: verifiedBy,
		Raw:        item,
	}
}

func resourcesFromItems(items []map[string]any, verifiedBy string) []MoldModel.ResourceResult {
	out := make([]MoldModel.ResourceResult, 0, len(items))
	for _, item := range items {
		if resource := resourceFromItem(item, verifiedBy); resource != nil {
			out = append(out, *resource)
		}
	}
	return out
}

func matchIDOrName(item map[string]any, id string, name string) bool {
	if strings.TrimSpace(id) != "" && strings.EqualFold(stringValue(item["id"]), strings.TrimSpace(id)) {
		return true
	}
	if strings.TrimSpace(name) != "" && strings.EqualFold(stringValue(item["name"]), strings.TrimSpace(name)) {
		return true
	}
	return false
}

func matchHost(item map[string]any, rawURL string) bool {
	host := hostAddress(rawURL)
	if host == "" {
		return false
	}
	for _, value := range []string{
		stringValue(item["ipaddress"]),
		stringValue(item["name"]),
		stringValue(item["url"]),
		stringValue(item["hostname"]),
	} {
		if strings.EqualFold(hostAddress(value), host) || strings.EqualFold(strings.TrimSpace(value), host) {
			return true
		}
	}
	return false
}

func matchURL(item map[string]any, want string) bool {
	want = strings.TrimRight(strings.TrimSpace(want), "/")
	if want == "" {
		return false
	}
	for _, key := range []string{"url", "path", "name"} {
		got := strings.TrimRight(strings.TrimSpace(stringValue(item[key])), "/")
		if strings.EqualFold(got, want) {
			return true
		}
	}
	return false
}

func hostAddress(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err == nil && parsed.Host != "" {
		host := parsed.Hostname()
		if host != "" {
			return host
		}
	}
	host, _, err := net.SplitHostPort(value)
	if err == nil {
		return host
	}
	return strings.Trim(value, "[]")
}

func setParam(values url.Values, key string, value string) {
	if strings.TrimSpace(value) != "" {
		values.Set(key, strings.TrimSpace(value))
	}
}

func asyncJobStatus(body map[string]any) int {
	resp := responseMap(body, "queryasyncjobresultresponse")
	if resp == nil {
		return 0
	}
	return intValue(resp["jobstatus"])
}

func findStringKey(value any, keys ...string) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range keys {
			if out := stringValue(typed[key]); out != "" {
				return out
			}
		}
		for _, child := range typed {
			if out := findStringKey(child, keys...); out != "" {
				return out
			}
		}
	case []any:
		for _, child := range typed {
			if out := findStringKey(child, keys...); out != "" {
				return out
			}
		}
	}
	return ""
}

func allSystemVMsRunning(items []map[string]any) bool {
	for _, item := range items {
		if !strings.EqualFold(stringValue(item["state"]), "Running") {
			return false
		}
	}
	return len(items) > 0
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		duration = time.Second
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func durationFromEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	duration, err := time.ParseDuration(raw)
	if err != nil || duration <= 0 {
		return fallback
	}
	return duration
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true") || strings.TrimSpace(typed) == "1"
	case float64:
		return typed != 0
	case int:
		return typed != 0
	default:
		return false
	}
}
