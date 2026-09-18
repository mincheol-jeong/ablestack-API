package moldservice

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	MoldModel "ablecloud.io/ablestack-api/internal/model/mold"
)

func TestBootstrapRuntimeRunsAllStepsWithAsyncCreateAndVerification(t *testing.T) {
	state := &fakeMoldBootstrapState{
		trafficTypes:     map[string]bool{},
		providers:        map[string]bool{},
		providerElements: map[string]bool{},
	}
	commands := make([]string, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm error = %v", err)
		}
		command := r.Form.Get("command")
		commands = append(commands, command)
		switch command {
		case "login":
			writeJSON(t, w, map[string]any{
				"loginresponse": map[string]any{"sessionkey": "session-key"},
			})
		case "listCapabilities":
			if r.Form.Get("sessionkey") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				writeJSON(t, w, map[string]any{
					"listcapabilitiesresponse": map[string]any{
						"errorcode": 401,
						"errortext": "unable to verify user credentials and/or request signature",
					},
				})
				return
			}
			assertFormValue(t, r.Form, "sessionkey", "session-key")
			writeJSON(t, w, map[string]any{
				"listcapabilitiesresponse": map[string]any{"capability": map[string]any{"securitygroupsenabled": false}},
			})
		case "queryAsyncJobResult":
			assertFormValue(t, r.Form, "jobid", "job-zone")
			writeJSON(t, w, map[string]any{
				"queryasyncjobresultresponse": map[string]any{"jobstatus": 1},
			})
		case "listZones":
			writeListResponse(t, w, "listzonesresponse", "zone", state.zone, map[string]any{
				"id": "zone-id", "name": "Zone", "allocationstate": state.zoneState(),
			})
		case "createZone":
			assertFormValue(t, r.Form, "name", "Zone")
			state.zone = true
			writeJSON(t, w, map[string]any{
				"createzoneresponse": map[string]any{"jobid": "job-zone"},
			})
		case "listPhysicalNetworks":
			writeListResponse(t, w, "listphysicalnetworksresponse", "physicalnetwork", state.physicalNetwork, map[string]any{
				"id": "physical-network-id", "name": "Physicalnetwork", "state": state.physicalNetworkState(),
			})
		case "createPhysicalNetwork":
			assertFormValue(t, r.Form, "zoneid", "zone-id")
			assertFormValue(t, r.Form, "networkspeed", "10G")
			assertFormMissing(t, r.Form, "speed")
			state.physicalNetwork = true
			writeJSON(t, w, map[string]any{"createphysicalnetworkresponse": map[string]any{"physicalnetwork": map[string]any{"id": "physical-network-id"}}})
		case "listTrafficTypes":
			trafficType := r.Form.Get("traffictype")
			writeListResponse(t, w, "listtraffictypesresponse", "traffictype", state.trafficTypes[trafficType], map[string]any{
				"id": "traffic-" + trafficType, "traffictype": trafficType, "kvmnetworklabel": "bridge0",
			})
		case "addTrafficType":
			assertFormValue(t, r.Form, "kvmnetworklabel", "bridge0")
			state.trafficTypes[r.Form.Get("traffictype")] = true
			writeJSON(t, w, map[string]any{"addtraffictyperesponse": map[string]any{}})
		case "updatePhysicalNetwork":
			assertFormValue(t, r.Form, "state", "Enabled")
			state.physicalNetworkEnabled = true
			writeJSON(t, w, map[string]any{"updatephysicalnetworkresponse": map[string]any{}})
		case "listNetworkServiceProviders":
			name := r.Form.Get("name")
			writeListResponse(t, w, "listnetworkserviceprovidersresponse", "networkserviceprovider", true, map[string]any{
				"id": "provider-" + name, "name": name, "state": state.providerState(name),
			})
		case "listVirtualRouterElements":
			providerID := r.Form.Get("nspid")
			writeListResponse(t, w, "listvirtualrouterelementsresponse", "virtualrouterelement", true, map[string]any{
				"id": "element-" + providerID, "nspid": providerID, "enabled": state.providerElements[providerID],
			})
		case "configureVirtualRouterElement":
			providerID := strings.TrimPrefix(r.Form.Get("id"), "element-")
			state.providerElements[providerID] = true
			writeJSON(t, w, map[string]any{"configurevirtualrouterelementresponse": map[string]any{}})
		case "updateNetworkServiceProvider":
			name := strings.TrimPrefix(r.Form.Get("id"), "provider-")
			state.providers[name] = true
			writeJSON(t, w, map[string]any{"updatenetworkserviceproviderresponse": map[string]any{}})
		case "listPods":
			writeListResponse(t, w, "listpodsresponse", "pod", state.pod, map[string]any{
				"id": "pod-id", "name": "Pod",
			})
		case "createPod":
			assertFormValue(t, r.Form, "zoneid", "zone-id")
			state.pod = true
			writeJSON(t, w, map[string]any{"createpodresponse": map[string]any{"pod": map[string]any{"id": "pod-id"}}})
		case "listVlanIpRanges":
			writeListResponse(t, w, "listvlaniprangesresponse", "vlaniprange", state.publicIPRange, map[string]any{
				"id": "public-range-id", "startip": "10.10.20.100", "endip": "10.10.20.150",
			})
		case "createVlanIpRange":
			assertFormValue(t, r.Form, "forvirtualnetwork", "true")
			assertFormMissing(t, r.Form, "vlan")
			state.publicIPRange = true
			writeJSON(t, w, map[string]any{"createvlaniprangeresponse": map[string]any{}})
		case "listClusters":
			writeListResponse(t, w, "listclustersresponse", "cluster", state.cluster, map[string]any{
				"id": "cluster-id", "name": "Cluster",
			})
		case "addCluster":
			assertFormValue(t, r.Form, "clustername", "Cluster")
			state.cluster = true
			writeJSON(t, w, map[string]any{"addclusterresponse": map[string]any{"cluster": []any{map[string]any{"id": "cluster-id"}}}})
		case "listHosts":
			writeListResponse(t, w, "listhostsresponse", "host", state.host, map[string]any{
				"id": "host-id", "name": "10.10.101.1", "ipaddress": "10.10.101.1", "state": "Up",
			})
		case "addHost":
			assertFormValue(t, r.Form, "clusterid", "cluster-id")
			state.host = true
			writeJSON(t, w, map[string]any{"addhostresponse": map[string]any{"host": []any{map[string]any{"id": "host-id"}}}})
		case "listStoragePools":
			writeListResponse(t, w, "liststoragepoolsresponse", "storagepool", state.primaryStorage, map[string]any{
				"id": "primary-storage-id", "name": "Primarystorage", "url": "nfs://10.10.101.20/export/primary", "state": "Up",
			})
		case "createStoragePool":
			assertFormValue(t, r.Form, "clusterid", "cluster-id")
			state.primaryStorage = true
			writeJSON(t, w, map[string]any{"createstoragepoolresponse": map[string]any{"storagepool": map[string]any{"id": "primary-storage-id"}}})
		case "listImageStores":
			writeListResponse(t, w, "listimagestoresresponse", "imagestore", state.secondaryStorage, map[string]any{
				"id": "secondary-storage-id", "url": "nfs://10.10.101.20/export/secondary",
			})
		case "addImageStore":
			assertFormValue(t, r.Form, "zoneid", "zone-id")
			assertFormValue(t, r.Form, "provider", "NFS")
			state.secondaryStorage = true
			writeJSON(t, w, map[string]any{"addimagestoreresponse": map[string]any{"imagestore": map[string]any{"id": "secondary-storage-id"}}})
		case "listSecondaryStagingStores":
			writeListResponse(t, w, "listsecondarystagingstoresresponse", "secondarystagingstore", state.secondaryStagingStore, map[string]any{
				"id": "secondary-staging-store-id", "url": "nfs://10.10.101.20/export/staging",
			})
		case "createSecondaryStagingStore":
			state.secondaryStagingStore = true
			writeJSON(t, w, map[string]any{"createsecondarystagingstoreresponse": map[string]any{"secondarystagingstore": map[string]any{"id": "secondary-staging-store-id"}}})
		case "updateZone":
			assertFormValue(t, r.Form, "id", "zone-id")
			assertFormValue(t, r.Form, "allocationstate", "Enabled")
			state.zoneEnabled = true
			writeJSON(t, w, map[string]any{"updatezoneresponse": map[string]any{"zone": map[string]any{"id": "zone-id"}}})
		case "listSystemVms":
			writeJSON(t, w, map[string]any{
				"listsystemvmsresponse": map[string]any{
					"count": 1,
					"systemvm": []any{
						map[string]any{"id": "system-vm-id", "name": "s-1-VM", "state": "Running"},
					},
				},
			})
		default:
			t.Fatalf("unexpected command %q", command)
		}
	}))
	defer server.Close()

	clusterPath := writeClusterJSON(t, `{"clusterConfig":{"ccvm":{"ip":"10.10.254.236"},"hosts":[]}}`)
	t.Setenv("ABLESTACK_CLUSTER_JSON", clusterPath)
	t.Setenv("ABLESTACK_MOLD_ENDPOINT", server.URL)
	t.Setenv("ABLESTACK_MOLD_ASYNC_POLL_INTERVAL", "1ms")
	t.Setenv("ABLESTACK_MOLD_SYSTEM_VM_POLL_INTERVAL", "1ms")

	req := fullBootstrapRequest()
	plan := BuildBootstrapPlan(req)
	if !plan.Ready {
		t.Fatalf("plan not ready: %#v", plan.MissingInputs)
	}
	trustStep := -1
	addHostStep := -1
	for index, step := range plan.Steps {
		switch step.Name {
		case "sync_host_ssh_trust":
			trustStep = index
		case "add_host":
			addHostStep = index
		}
	}
	if trustStep < 0 || addHostStep < 0 || trustStep >= addHostStep {
		t.Fatalf("sync_host_ssh_trust must run before add_host: trust=%d add_host=%d", trustStep, addHostStep)
	}
	runtime := NewBootstrapRuntime(req)
	runtime.hostTrust = func(context.Context) (any, any, error) {
		return []map[string]any{{"target": "10.10.31.1", "code": 200}}, []map[string]any{{"target": "10.10.31.1", "code": 200}}, nil
	}
	for _, planStep := range plan.Steps {
		step := MoldModel.BootstrapJobStep{
			Name:          planStep.Name,
			Command:       planStep.Command,
			VerifyCommand: planStep.VerifyCommand,
			ResultKey:     planStep.ResultKey,
		}
		result := runtime.RunStep(context.Background(), step)
		if result.Error != nil {
			t.Fatalf("step %s failed: %#v", step.Name, result.Error)
		}
		if result.Status != MoldModel.BootstrapStepStatusSucceeded {
			t.Fatalf("step %s status=%s", step.Name, result.Status)
		}
	}
	if !contains(commands, "queryAsyncJobResult") {
		t.Fatalf("commands missing queryAsyncJobResult: %#v", commands)
	}
	for _, want := range []string{"createZone", "createPhysicalNetwork", "addTrafficType", "updatePhysicalNetwork", "configureVirtualRouterElement", "updateNetworkServiceProvider", "createPod", "createVlanIpRange", "addCluster", "addHost", "createStoragePool", "addImageStore", "createSecondaryStagingStore", "updateZone"} {
		if !contains(commands, want) {
			t.Fatalf("commands missing %s: %#v", want, commands)
		}
	}
}

func TestEnsurePrimaryStorageSendsGlueBlockParameters(t *testing.T) {
	created := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm error = %v", err)
		}
		switch r.Form.Get("command") {
		case "listStoragePools":
			writeListResponse(t, w, "liststoragepoolsresponse", "storagepool", created, map[string]any{
				"id": "primary-storage-id", "name": "Primary Storage(RBD)",
			})
		case "createStoragePool":
			assertFormValue(t, r.Form, "name", "Primary Storage(RBD)")
			assertFormValue(t, r.Form, "provider", "ABLESTACK")
			assertFormValue(t, r.Form, "url", "rbd://admin:secret-value@scvm1,scvm2/rbd")
			assertFormValue(t, r.Form, "krbdPath", "/dev/rbd")
			assertFormValue(t, r.Form, "details[0].provider", "ABLESTACK")
			assertFormValue(t, r.Form, "tags", "Primary Storage(RBD)")
			created = true
			writeJSON(t, w, map[string]any{"createstoragepoolresponse": map[string]any{"storagepool": map[string]any{"id": "primary-storage-id"}}})
		default:
			t.Fatalf("unexpected command %q", r.Form.Get("command"))
		}
	}))
	defer server.Close()

	runtime := NewBootstrapRuntime(MoldModel.BootstrapRequest{
		Cluster: MoldModel.ClusterRequest{Hypervisor: "KVM"},
		PrimaryStorage: MoldModel.PrimaryStorageRequest{
			Name:            "Primary Storage(RBD)",
			Scope:           "cluster",
			Provider:        "ABLESTACK",
			Tags:            "Primary Storage(RBD)",
			RADOSMonitors:   "scvm1,scvm2",
			RADOSPool:       "rbd",
			RADOSUser:       "admin",
			KRBDPath:        "/dev/rbd",
			DetailsProvider: "ABLESTACK",
			AutoRBDSecret:   true,
			CephKeyHosts:    []string{"10.10.101.1"},
		},
	})
	runtime.client = NewClient(server.URL)
	runtime.session = Session{SessionKey: "session-key"}
	runtime.zoneID = "zone-id"
	runtime.podID = "pod-id"
	runtime.clusterID = "cluster-id"
	runtime.cephKey = func(context.Context, []string) (string, error) { return "secret-value", nil }

	result := runtime.ensurePrimaryStorage(context.Background(), MoldModel.BootstrapJobStep{
		Name: "add_primary_storage", Command: "createStoragePool", VerifyCommand: "listStoragePools", ResultKey: "primary_storage",
	})
	if result.Error != nil || !result.Created || !result.Verified {
		t.Fatalf("result = %#v", result)
	}
}

func fullBootstrapRequest() MoldModel.BootstrapRequest {
	return MoldModel.BootstrapRequest{
		Auth: MoldModel.MoldAuthRequest{Username: "admin", Password: "password", Domain: "/", Response: "json"},
		Zone: MoldModel.ZoneRequest{
			Name: "Zone", DNS1: "8.8.8.8", InternalDNS1: "10.10.0.1", NetworkType: "Advanced",
		},
		PhysicalNetwork: MoldModel.PhysicalNetworkRequest{Name: "Physicalnetwork", BroadcastDomainRange: "Zone", IsolationMethods: "VLAN", NetworkSpeed: "10G"},
		Pod:             MoldModel.PodRequest{Name: "Pod", Gateway: "10.10.101.1", Netmask: "255.255.255.0", StartIP: "10.10.101.100", EndIP: "10.10.101.150"},
		PublicIP:        MoldModel.PublicIPRangeRequest{Gateway: "10.10.20.1", Netmask: "255.255.255.0", StartIP: "10.10.20.100", EndIP: "10.10.20.150"},
		Cluster:         MoldModel.ClusterRequest{Name: "Cluster", ClusterType: "CloudManaged", Hypervisor: "KVM", Arch: "x86_64"},
		Hosts: []MoldModel.HostRequest{
			{URL: "http://10.10.101.1", Username: "root", Password: "password", Hypervisor: "KVM"},
		},
		PrimaryStorage:        MoldModel.PrimaryStorageRequest{Name: "Primarystorage", URL: "nfs://10.10.101.20/export/primary"},
		SecondaryStorage:      MoldModel.SecondaryStorageRequest{URL: "nfs://10.10.101.20/export/secondary"},
		SecondaryStagingStore: &MoldModel.SecondaryStagingStoreRequest{URL: "nfs://10.10.101.20/export/staging", Provider: "NFS", Scope: "zone"},
	}
}

type fakeMoldBootstrapState struct {
	zone                   bool
	zoneEnabled            bool
	physicalNetwork        bool
	physicalNetworkEnabled bool
	trafficTypes           map[string]bool
	providers              map[string]bool
	providerElements       map[string]bool
	pod                    bool
	publicIPRange          bool
	cluster                bool
	host                   bool
	primaryStorage         bool
	secondaryStorage       bool
	secondaryStagingStore  bool
}

func (s *fakeMoldBootstrapState) physicalNetworkState() string {
	if s.physicalNetworkEnabled {
		return "Enabled"
	}
	return "Disabled"
}

func (s *fakeMoldBootstrapState) providerState(name string) string {
	if s.providers[name] {
		return "Enabled"
	}
	return "Disabled"
}

func (s *fakeMoldBootstrapState) zoneState() string {
	if s.zoneEnabled {
		return "Enabled"
	}
	return "Disabled"
}

func writeListResponse(t *testing.T, w http.ResponseWriter, responseKey string, itemKey string, exists bool, item map[string]any) {
	t.Helper()
	resp := map[string]any{"count": 0}
	if exists {
		resp["count"] = 1
		resp[itemKey] = []any{item}
	}
	writeJSON(t, w, map[string]any{responseKey: resp})
}

func assertFormMissing(t *testing.T, form url.Values, key string) {
	t.Helper()
	if got := form.Get(key); got != "" {
		t.Fatalf("%s = %q, want empty", key, got)
	}
}
