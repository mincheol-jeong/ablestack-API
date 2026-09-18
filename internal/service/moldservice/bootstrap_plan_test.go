package moldservice

import (
	"context"
	"fmt"
	"testing"

	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	MoldModel "ablecloud.io/ablestack-api/internal/model/mold"
)

func TestBuildBootstrapPlanOmitsPasswordChangeAndReportsMissingInputs(t *testing.T) {
	plan := BuildBootstrapPlan(MoldModel.BootstrapRequest{})

	if plan.Ready {
		t.Fatalf("Ready = true, want false")
	}
	if contains(plan.MissingInputs, "zone.name") {
		t.Fatalf("zone.name should be defaulted, missing=%#v", plan.MissingInputs)
	}
	if contains(plan.MissingInputs, "zone.dns1") == false {
		t.Fatalf("missing inputs should include zone.dns1: %#v", plan.MissingInputs)
	}
	if contains(plan.MissingInputs, "hosts[].url") == false {
		t.Fatalf("missing inputs should include hosts[].url: %#v", plan.MissingInputs)
	}
	for _, step := range plan.Steps {
		if step.Name == "change_initial_password_if_required" || step.Name == "login_with_new_password" {
			t.Fatalf("unexpected password-change step: %s", step.Name)
		}
	}
}

func TestApplyClusterBootstrapDefaultsByClusterType(t *testing.T) {
	tests := []struct {
		clusterType string
		name        string
		provider    string
		protocol    string
		url         string
		autoRBD     bool
	}{
		{"ablestack-hci", "Primary Storage(RBD)", "ABLESTACK", "Glue Block", "", true},
		{"ablestack-vm", "Primary Storage(Glue)", "DefaultPrimary", "SharedMountPoint", "SharedMountPoint://localhost/mnt/glue-gfs", false},
		{"ablestack-standalone", "Primary Storage(Glue)", "DefaultPrimary", "SharedMountPoint", "SharedMountPoint://localhost/mnt/glue", false},
		{"ablestack-hci-filesystem", "Primary Storage(Glue)", "DefaultPrimary", "SharedMountPoint", "SharedMountPoint://localhost/mnt/glue-gfs", false},
	}
	for _, tt := range tests {
		t.Run(tt.clusterType, func(t *testing.T) {
			cfg := &CubeModel.ClusterConfigSection{
				Type: tt.clusterType,
				CCVM: CubeModel.ClusterCCVMConfig{IP: "10.10.254.236"},
				Hosts: []CubeModel.ClusterHost{
					{Index: "1", Ablecube: "10.10.101.1"},
					{Index: "2", Ablecube: "10.10.101.2"},
				},
			}
			req, err := ApplyClusterBootstrapDefaults(MoldModel.BootstrapRequest{}, cfg)
			if err != nil {
				t.Fatalf("ApplyClusterBootstrapDefaults error = %v", err)
			}
			storage := req.PrimaryStorage
			if storage.Name != tt.name || storage.Provider != tt.provider || storage.Protocol != tt.protocol || storage.URL != tt.url || storage.AutoRBDSecret != tt.autoRBD {
				t.Fatalf("primary storage = %#v", storage)
			}
			if storage.Tags != tt.name {
				t.Fatalf("Tags = %q, want %q", storage.Tags, tt.name)
			}
			if req.SecondaryStorage.URL != "nfs://10.10.254.236" {
				t.Fatalf("SecondaryStorage.URL = %q", req.SecondaryStorage.URL)
			}
			if req.PhysicalNetwork.VLAN != "1-1" {
				t.Fatalf("PhysicalNetwork.VLAN = %q", req.PhysicalNetwork.VLAN)
			}
			if len(req.Hosts) != 2 || req.Hosts[0].URL != "http://10.10.101.1" || req.Hosts[0].Password != "" {
				t.Fatalf("Hosts = %#v", req.Hosts)
			}
			if tt.autoRBD {
				if storage.RADOSMonitors != "scvm1,scvm2" || storage.RADOSPool != "rbd" || storage.RADOSUser != "admin" || storage.KRBDPath != "/dev/rbd" {
					t.Fatalf("RBD storage = %#v", storage)
				}
			}
		})
	}
}

func TestResolvePrimaryStorageURLUsesRuntimeCephKey(t *testing.T) {
	runtime := NewBootstrapRuntime(MoldModel.BootstrapRequest{
		PrimaryStorage: MoldModel.PrimaryStorageRequest{
			AutoRBDSecret: true,
			RADOSMonitors: "scvm1,scvm2",
			RADOSPool:     "rbd",
			RADOSUser:     "admin",
			CephKeyHosts:  []string{"10.10.101.1"},
		},
	})
	runtime.cephKey = func(_ context.Context, hosts []string) (string, error) {
		if len(hosts) != 1 || hosts[0] != "10.10.101.1" {
			return "", fmt.Errorf("unexpected hosts: %#v", hosts)
		}
		return "AQAHOghpeDVpLBAAI5o8rwd95Y83/oUo1+kOMw==", nil
	}
	got, err := runtime.resolvePrimaryStorageURL(context.Background())
	if err != nil {
		t.Fatalf("resolvePrimaryStorageURL error = %v", err)
	}
	want := "rbd://admin:AQAHOghpeDVpLBAAI5o8rwd95Y83/oUo1+kOMw==@scvm1,scvm2/rbd"
	if got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
}

func TestNormalizeBootstrapRequestDefaultNames(t *testing.T) {
	req := NormalizeBootstrapRequest(MoldModel.BootstrapRequest{
		Zone: MoldModel.ZoneRequest{Name: "zone"},
		Pod:  MoldModel.PodRequest{Name: "POD"},
	})

	if req.Zone.Name != "Zone" {
		t.Fatalf("Zone.Name = %q, want Zone", req.Zone.Name)
	}
	if req.Pod.Name != "Pod" {
		t.Fatalf("Pod.Name = %q, want Pod", req.Pod.Name)
	}
	if req.Cluster.Name != "Cluster" {
		t.Fatalf("Cluster.Name = %q, want Cluster", req.Cluster.Name)
	}
	if req.PhysicalNetwork.Name != "Physicalnetwork" {
		t.Fatalf("PhysicalNetwork.Name = %q, want Physicalnetwork", req.PhysicalNetwork.Name)
	}
	if req.PrimaryStorage.Name != "Primarystorage" {
		t.Fatalf("PrimaryStorage.Name = %q, want Primarystorage", req.PrimaryStorage.Name)
	}
}

func TestBuildBootstrapPlanReadyWithRequiredInputs(t *testing.T) {
	plan := BuildBootstrapPlan(MoldModel.BootstrapRequest{
		Zone: MoldModel.ZoneRequest{
			Name:         "zone-1",
			DNS1:         "8.8.8.8",
			InternalDNS1: "10.10.0.1",
			NetworkType:  "Advanced",
		},
		PhysicalNetwork: MoldModel.PhysicalNetworkRequest{
			Name: "physical-network-1",
		},
		Pod: MoldModel.PodRequest{
			Name:    "pod-1",
			Gateway: "10.10.31.1",
			Netmask: "255.255.255.0",
			StartIP: "10.10.31.100",
			EndIP:   "10.10.31.150",
		},
		PublicIP: MoldModel.PublicIPRangeRequest{
			Gateway: "10.10.20.1",
			Netmask: "255.255.255.0",
			StartIP: "10.10.20.100",
			EndIP:   "10.10.20.150",
		},
		Cluster: MoldModel.ClusterRequest{
			Name:        "cluster-1",
			ClusterType: "CloudManaged",
			Hypervisor:  "KVM",
		},
		Hosts: []MoldModel.HostRequest{
			{
				URL:      "http://10.10.31.1",
				Username: "root",
				Password: "password",
			},
		},
		PrimaryStorage: MoldModel.PrimaryStorageRequest{
			Name: "primary-storage-1",
			URL:  "nfs://10.10.31.20/export/primary",
		},
		SecondaryStorage: MoldModel.SecondaryStorageRequest{
			URL: "nfs://10.10.31.20/export/secondary",
		},
	})

	if !plan.Ready {
		t.Fatalf("Ready = false, missing=%#v", plan.MissingInputs)
	}
	if len(plan.MissingInputs) != 0 {
		t.Fatalf("MissingInputs = %#v, want empty", plan.MissingInputs)
	}
	if !hasStep(plan.Steps, "enable_zone") {
		t.Fatalf("plan missing enable_zone step: %#v", plan.Steps)
	}
	zoneStep := findStep(plan.Steps, "create_zone")
	if zoneStep == nil {
		t.Fatalf("plan missing create_zone step")
	}
	if zoneStep.ResultKey != "zone" {
		t.Fatalf("create_zone result_key = %q, want zone", zoneStep.ResultKey)
	}
	if len(zoneStep.SuccessCriteria) == 0 {
		t.Fatalf("create_zone success criteria should not be empty")
	}
	if !contains(zoneStep.ErrorFields, "error_text") {
		t.Fatalf("create_zone error fields should include error_text: %#v", zoneStep.ErrorFields)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func hasStep(steps []MoldModel.BootstrapPlanStep, want string) bool {
	return findStep(steps, want) != nil
}

func findStep(steps []MoldModel.BootstrapPlanStep, want string) *MoldModel.BootstrapPlanStep {
	for _, step := range steps {
		if step.Name == want {
			return &step
		}
	}
	return nil
}
