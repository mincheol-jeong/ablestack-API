package moldservice

import (
	"fmt"
	"strings"

	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	MoldModel "ablecloud.io/ablestack-api/internal/model/mold"
)

func NormalizeBootstrapRequest(req MoldModel.BootstrapRequest) MoldModel.BootstrapRequest {
	req.Zone.Name = normalizeBootstrapName(req.Zone.Name, "Zone")
	req.PhysicalNetwork.Name = normalizeBootstrapName(req.PhysicalNetwork.Name, "Physicalnetwork")
	req.Pod.Name = normalizeBootstrapName(req.Pod.Name, "Pod")
	req.Cluster.Name = normalizeBootstrapName(req.Cluster.Name, "Cluster")
	if strings.TrimSpace(req.PrimaryStorage.Name) == "" {
		req.PrimaryStorage.Name = "Primarystorage"
	}
	return req
}

// PrepareBootstrapRequest applies cluster.json-owned values so callers only
// provide the Zone and Pod network inputs that cannot be derived locally.
func PrepareBootstrapRequest(req MoldModel.BootstrapRequest) (MoldModel.BootstrapRequest, error) {
	cfg, path, err := LoadClusterConfigSection()
	if err != nil {
		return req, fmt.Errorf("failed to read cluster config %s: %w", path, err)
	}
	return ApplyClusterBootstrapDefaults(req, cfg)
}

func ApplyClusterBootstrapDefaults(req MoldModel.BootstrapRequest, cfg *CubeModel.ClusterConfigSection) (MoldModel.BootstrapRequest, error) {
	if cfg == nil {
		return req, fmt.Errorf("cluster config required")
	}
	req = NormalizeBootstrapRequest(req)
	req.PhysicalNetwork.VLAN = "1-1"

	ccvmIP := strings.TrimSpace(cfg.CCVM.IP)
	if ccvmIP == "" {
		return req, ErrMissingCCVMIP
	}
	req.SecondaryStorage = MoldModel.SecondaryStorageRequest{
		Name:     "Secondary Storage",
		Provider: "NFS",
		URL:      "nfs://" + ccvmIP,
	}

	hosts := make([]MoldModel.HostRequest, 0, len(cfg.Hosts))
	cephKeyHosts := make([]string, 0, len(cfg.Hosts))
	monitors := make([]string, 0, len(cfg.Hosts))
	for i, host := range cfg.Hosts {
		ip := strings.TrimSpace(host.Ablecube)
		if ip == "" {
			continue
		}
		hosts = append(hosts, MoldModel.HostRequest{
			URL:        "http://" + ip,
			Username:   "root",
			Hypervisor: "KVM",
			HostTags:   "compute",
		})
		cephKeyHosts = append(cephKeyHosts, ip)
		index := strings.TrimSpace(host.Index)
		if index == "" {
			index = fmt.Sprintf("%d", i+1)
		}
		monitors = append(monitors, "scvm"+index)
	}
	req.Hosts = hosts

	switch strings.ToLower(strings.TrimSpace(cfg.Type)) {
	case "ablestack-hci":
		req.PrimaryStorage = MoldModel.PrimaryStorageRequest{
			Name:            "Primary Storage(RBD)",
			Scope:           "cluster",
			Provider:        "ABLESTACK",
			Protocol:        "Glue Block",
			Hypervisor:      "KVM",
			Tags:            "Primary Storage(RBD)",
			RADOSMonitors:   strings.Join(monitors, ","),
			RADOSPool:       "rbd",
			RADOSUser:       "admin",
			KRBDPath:        "/dev/rbd",
			DetailsProvider: "ABLESTACK",
			AutoRBDSecret:   true,
			CephKeyHosts:    cephKeyHosts,
		}
	case "ablestack-vm", "ablestack-hci-filesystem", "ablestack-standalone":
		mountPath := "/mnt/glue-gfs"
		if strings.EqualFold(strings.TrimSpace(cfg.Type), "ablestack-standalone") {
			mountPath = "/mnt/glue"
		}
		req.PrimaryStorage = MoldModel.PrimaryStorageRequest{
			Name:       "Primary Storage(Glue)",
			URL:        "SharedMountPoint://localhost" + mountPath,
			Scope:      "cluster",
			Provider:   "DefaultPrimary",
			Protocol:   "SharedMountPoint",
			Hypervisor: "KVM",
			Tags:       "Primary Storage(Glue)",
		}
	default:
		return req, fmt.Errorf("unsupported clusterConfig.type %q", cfg.Type)
	}
	return req, nil
}

func normalizeBootstrapName(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	runes := []rune(strings.ToLower(value))
	if len(runes) == 0 {
		return ""
	}
	runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
	return string(runes)
}
