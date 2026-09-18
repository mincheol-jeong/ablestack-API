package moldservice

import (
	"fmt"
	"strings"

	MoldModel "ablecloud.io/ablestack-api/internal/model/mold"
)

func BuildBootstrapPlan(req MoldModel.BootstrapRequest) MoldModel.BootstrapPlanValue {
	req = NormalizeBootstrapRequest(req)
	steps := []MoldModel.BootstrapPlanStep{
		{
			Name:          "wait_mold_api_ready",
			Command:       "listCapabilities",
			VerifyCommand: "listCapabilities",
			Notes: []string{
				"clusterConfig.ccvm.ip resolves the Mold endpoint; no infrastructure input is required for this step.",
			},
		},
		{
			Name:          "login_admin",
			Command:       "login",
			VerifyCommand: "listCapabilities",
			RequiredInputs: []string{
				"auth.username optional; default admin",
				"auth.password optional; default password",
				"auth.domain optional; default /",
			},
			Notes: []string{
				"Password change is intentionally not part of this flow.",
			},
		},
		planStep("create_zone", "createZone", "listZones", []requirement{
			required("zone.name", req.Zone.Name),
			required("zone.dns1", req.Zone.DNS1),
			required("zone.internaldns1", req.Zone.InternalDNS1),
			required("zone.networktype", req.Zone.NetworkType),
		}),
		planStep("create_physical_network", "createPhysicalNetwork", "listPhysicalNetworks", []requirement{
			required("physical_network.name", req.PhysicalNetwork.Name),
			computed("physical_network.zoneid", "zoneid is resolved from create_zone/listZones"),
		}),
		planStep("add_traffic_types", "addTrafficType", "listTrafficTypes", []requirement{
			computed("traffic_types.physicalnetworkid", "physicalnetworkid is resolved from create_physical_network/listPhysicalNetworks"),
			computed("traffic_types.traffictype", "Management, Guest and Public are fixed traffic types"),
			computed("traffic_types.kvmnetworklabel", "bridge0 is used for every KVM traffic type"),
		}),
		planStep("enable_physical_network", "updatePhysicalNetwork", "listPhysicalNetworks", []requirement{
			computed("physical_network.id", "physicalnetworkid is resolved from create_physical_network/listPhysicalNetworks"),
			computed("physical_network.state", "state=Enabled"),
		}),
		planStep("enable_network_providers", "configureVirtualRouterElement,updateNetworkServiceProvider", "listVirtualRouterElements,listNetworkServiceProviders", []requirement{
			computed("network_providers", "VirtualRouter and VpcVirtualRouter are enabled automatically"),
		}),
		planStep("create_pod", "createPod", "listPods", []requirement{
			required("pod.name", req.Pod.Name),
			required("pod.gateway", req.Pod.Gateway),
			required("pod.netmask", req.Pod.Netmask),
			required("pod.startip", req.Pod.StartIP),
			required("pod.endip", req.Pod.EndIP),
			computed("pod.zoneid", "zoneid is resolved from create_zone/listZones"),
		}),
		planStep("create_public_ip_range", "createVlanIpRange", "listVlanIpRanges", []requirement{
			required("public_ip.gateway", req.PublicIP.Gateway),
			required("public_ip.netmask", req.PublicIP.Netmask),
			required("public_ip.startip", req.PublicIP.StartIP),
			required("public_ip.endip", req.PublicIP.EndIP),
			computed("public_ip.zoneid", "zoneid is resolved from create_zone/listZones"),
			computed("public_ip.physicalnetworkid", "physicalnetworkid is resolved from create_physical_network/listPhysicalNetworks"),
			computed("public_ip.forvirtualnetwork", "forvirtualnetwork=true"),
		}),
		planStep("create_cluster", "addCluster", "listClusters", []requirement{
			required("cluster.name", req.Cluster.Name),
			required("cluster.clustertype", req.Cluster.ClusterType),
			required("cluster.hypervisor", req.Cluster.Hypervisor),
			computed("cluster.zoneid", "zoneid is resolved from create_zone/listZones"),
			computed("cluster.podid", "podid is resolved from create_pod/listPods"),
		}),
		planStep("sync_host_ssh_trust", "ensure cloud@ccvm authorized key", "ssh BatchMode connection", []requirement{
			computed("clusterConfig.hosts[].ablecube", "all ablecube targets are read from cluster.json"),
			computed("cloudstack management public key", "/var/cloudstack/management/.ssh/id_rsa.pub is read on CCVM"),
		}),
		hostPlanStep(req),
		planStep("add_primary_storage", "createStoragePool", "listStoragePools", []requirement{
			required("primary_storage.name", req.PrimaryStorage.Name),
			primaryStorageURLRequirement(req.PrimaryStorage),
			computed("primary_storage.zoneid", "zoneid is resolved from create_zone/listZones"),
			computed("primary_storage.podid", "podid is resolved from create_pod/listPods"),
			computed("primary_storage.clusterid", "clusterid is resolved from create_cluster/listClusters"),
		}),
		planStep("add_secondary_storage", "addImageStore", "listImageStores", []requirement{
			required("secondary_storage.url", req.SecondaryStorage.URL),
			computed("secondary_storage.zoneid", "zoneid is resolved from create_zone/listZones"),
		}),
		{
			Name:          "enable_zone",
			Command:       "updateZone",
			VerifyCommand: "listZones",
			RequiredInputs: []string{
				"zone.id computed from create_zone/listZones",
				"allocationstate=Enabled",
			},
		},
		{
			Name:          "wait_system_vm_running",
			Command:       "listSystemVms",
			VerifyCommand: "listSystemVms",
			RequiredInputs: []string{
				"zone.id computed from create_zone/listZones",
			},
			Notes: []string{
				"Poll until Console Proxy VM and Secondary Storage VM are Running when those system VMs are expected.",
			},
		},
		{
			Name:          "final_health_check",
			VerifyCommand: "listZones,listPhysicalNetworks,listTrafficTypes,listNetworkServiceProviders,listVirtualRouterElements,listPods,listVlanIpRanges,listClusters,listHosts,listStoragePools,listImageStores,listSystemVms",
			Notes: []string{
				"Every created resource is verified again by list APIs, not by create API success alone.",
			},
		},
	}

	if req.SecondaryStagingStore != nil {
		step := planStep("add_secondary_staging_store", "createSecondaryStagingStore", "listSecondaryStagingStores", []requirement{
			required("secondary_staging_store.url", req.SecondaryStagingStore.URL),
			computed("secondary_staging_store.zoneid", "zoneid is resolved from create_zone/listZones"),
		})
		steps = insertBeforeStep(steps, "enable_zone", step)
	}
	applyBootstrapPlanContracts(steps)

	missing := dedupeMissingInputs(steps)
	return MoldModel.BootstrapPlanValue{
		Ready:         len(missing) == 0,
		Steps:         steps,
		MissingInputs: missing,
		Notes: []string{
			"Initial password change is excluded from this flow.",
			"create steps should be implemented as ensure operations: list first, create if missing, then list again.",
			"zoneid, podid, clusterid are runtime IDs captured from earlier create/list responses and are not user inputs.",
		},
	}
}

type requirement struct {
	path     string
	value    string
	missing  bool
	computed bool
	note     string
}

func required(path string, value string) requirement {
	return requirement{path: path, value: value, missing: strings.TrimSpace(value) == ""}
}

func computed(path string, note string) requirement {
	return requirement{path: path, computed: true, note: note}
}

func planStep(name string, command string, verifyCommand string, requirements []requirement) MoldModel.BootstrapPlanStep {
	requiredInputs := make([]string, 0, len(requirements))
	missingInputs := make([]string, 0)
	notes := make([]string, 0)
	for _, req := range requirements {
		requiredInputs = append(requiredInputs, req.path)
		if req.missing {
			missingInputs = append(missingInputs, req.path)
		}
		if req.computed && strings.TrimSpace(req.note) != "" {
			notes = append(notes, req.note)
		}
	}
	return MoldModel.BootstrapPlanStep{
		Name:            name,
		Command:         command,
		VerifyCommand:   verifyCommand,
		ResultKey:       bootstrapResultKey(name),
		RequiredInputs:  requiredInputs,
		MissingInputs:   missingInputs,
		SuccessCriteria: bootstrapSuccessCriteria(name),
		ErrorFields:     bootstrapErrorFields(command, verifyCommand),
		Notes:           notes,
	}
}

func hostPlanStep(req MoldModel.BootstrapRequest) MoldModel.BootstrapPlanStep {
	requirements := []requirement{
		computed("hosts[].zoneid", "zoneid is resolved from create_zone/listZones"),
		computed("hosts[].podid", "podid is resolved from create_pod/listPods"),
		computed("hosts[].clusterid", "clusterid is resolved from create_cluster/listClusters"),
	}
	if len(req.Hosts) == 0 {
		requirements = append(requirements,
			requirement{path: "hosts[].url", missing: true},
			requirement{path: "hosts[].username", missing: true},
		)
	} else {
		for i, host := range req.Hosts {
			prefix := fmt.Sprintf("hosts[%d]", i)
			requirements = append(requirements,
				required(prefix+".url", host.URL),
				required(prefix+".username", host.Username),
			)
			if strings.TrimSpace(host.Hypervisor) == "" && strings.TrimSpace(req.Cluster.Hypervisor) == "" {
				requirements = append(requirements, requirement{path: prefix + ".hypervisor or cluster.hypervisor", missing: true})
			}
		}
	}
	return planStep("add_host", "addHost", "listHosts", requirements)
}

func primaryStorageURLRequirement(storage MoldModel.PrimaryStorageRequest) requirement {
	if storage.AutoRBDSecret {
		return computed("primary_storage.url", "RBD URL and client.admin secret are resolved from cluster.json and a configured host at runtime")
	}
	return required("primary_storage.url", storage.URL)
}

func insertBeforeStep(steps []MoldModel.BootstrapPlanStep, before string, step MoldModel.BootstrapPlanStep) []MoldModel.BootstrapPlanStep {
	out := make([]MoldModel.BootstrapPlanStep, 0, len(steps)+1)
	inserted := false
	for _, current := range steps {
		if !inserted && current.Name == before {
			out = append(out, step)
			inserted = true
		}
		out = append(out, current)
	}
	if !inserted {
		out = append(out, step)
	}
	return out
}

func dedupeMissingInputs(steps []MoldModel.BootstrapPlanStep) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, step := range steps {
		for _, input := range step.MissingInputs {
			input = strings.TrimSpace(input)
			if input == "" {
				continue
			}
			if _, ok := seen[input]; ok {
				continue
			}
			seen[input] = struct{}{}
			out = append(out, input)
		}
	}
	return out
}

func applyBootstrapPlanContracts(steps []MoldModel.BootstrapPlanStep) {
	for i := range steps {
		if steps[i].ResultKey == "" {
			steps[i].ResultKey = bootstrapResultKey(steps[i].Name)
		}
		if len(steps[i].SuccessCriteria) == 0 {
			steps[i].SuccessCriteria = bootstrapSuccessCriteria(steps[i].Name)
		}
		if len(steps[i].ErrorFields) == 0 {
			steps[i].ErrorFields = bootstrapErrorFields(steps[i].Command, steps[i].VerifyCommand)
		}
	}
}

func bootstrapResultKey(stepName string) string {
	switch stepName {
	case "wait_mold_api_ready":
		return "api_ready"
	case "login_admin":
		return "session"
	case "create_zone":
		return "zone"
	case "create_physical_network":
		return "physical_network"
	case "add_traffic_types":
		return "traffic_types"
	case "enable_physical_network":
		return "physical_network"
	case "enable_network_providers":
		return "network_providers"
	case "create_pod":
		return "pod"
	case "create_public_ip_range":
		return "public_ip_range"
	case "create_cluster":
		return "cluster"
	case "sync_host_ssh_trust":
		return "host_ssh_trust"
	case "add_host":
		return "hosts"
	case "add_primary_storage":
		return "primary_storage"
	case "add_secondary_storage":
		return "secondary_storage"
	case "add_secondary_staging_store":
		return "secondary_staging_store"
	case "enable_zone":
		return "zone"
	case "wait_system_vm_running":
		return "system_vms"
	case "final_health_check":
		return "resources"
	default:
		return stepName
	}
}

func bootstrapSuccessCriteria(stepName string) []string {
	switch stepName {
	case "wait_mold_api_ready":
		return []string{"listCapabilities returns a successful response"}
	case "login_admin":
		return []string{"loginresponse.sessionkey is present", "listCapabilities succeeds with the sessionkey"}
	case "create_zone":
		return []string{"createZone returns a zone id or an existing zone is found", "listZones returns the zone by name/id"}
	case "create_physical_network":
		return []string{"createPhysicalNetwork returns a physical network id or an existing physical network is found", "listPhysicalNetworks returns the physical network by name/id"}
	case "add_traffic_types":
		return []string{"Management, Guest and Public traffic types exist", "listTrafficTypes returns kvmnetworklabel=bridge0 for every traffic type"}
	case "enable_physical_network":
		return []string{"updatePhysicalNetwork state=Enabled succeeds", "listPhysicalNetworks returns state=Enabled"}
	case "enable_network_providers":
		return []string{"VirtualRouter and VpcVirtualRouter elements are enabled", "listNetworkServiceProviders returns state=Enabled for both providers"}
	case "create_pod":
		return []string{"createPod returns a pod id or an existing pod is found", "listPods returns the pod by name/id"}
	case "create_public_ip_range":
		return []string{"createVlanIpRange creates an untagged public range", "listVlanIpRanges returns the requested start/end IP range"}
	case "create_cluster":
		return []string{"addCluster returns a cluster id or an existing cluster is found", "listClusters returns the cluster by name/id"}
	case "sync_host_ssh_trust":
		return []string{"cloud@ccvm public key is present on every clusterConfig.hosts[].ablecube host", "CCVM can connect to every host with the CloudStack management private key"}
	case "add_host":
		return []string{"addHost returns a host id or an existing host is found", "listHosts returns each host", "each host reaches Up/Enabled state when expected"}
	case "add_primary_storage":
		return []string{"createStoragePool returns a storage pool id or an existing pool is found", "listStoragePools returns the pool", "pool state is Up when expected"}
	case "add_secondary_storage":
		return []string{"addImageStore returns an image store id or an existing image store is found", "listImageStores returns the NFS secondary storage"}
	case "add_secondary_staging_store":
		return []string{"createSecondaryStagingStore returns a staging store id or an existing staging store is found", "listSecondaryStagingStores returns the staging store"}
	case "enable_zone":
		return []string{"updateZone allocationstate=Enabled succeeds", "listZones returns allocationstate=Enabled for the zone"}
	case "wait_system_vm_running":
		return []string{"listSystemVms returns expected system VMs", "expected system VMs are Running"}
	case "final_health_check":
		return []string{"Zone, physical network, traffic types, providers, Pod, public IP range, cluster, hosts and storage are returned by their list APIs", "failed resources include step-level error details"}
	default:
		return nil
	}
}

func bootstrapErrorFields(command string, verifyCommand string) []string {
	fields := []string{"step", "command", "code", "error_code", "error_text", "message", "raw"}
	if strings.TrimSpace(verifyCommand) != "" && verifyCommand != command {
		fields = append(fields, "verify_command")
	}
	return fields
}
