package cube

import (
	"fmt"
	"net/http"
	"testing"

	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
)

func TestIsDeployClusterConfigReadyRequiresPCSForCloudClusterTypes(t *testing.T) {
	tests := []struct {
		name   string
		cfg    CubeModel.ClusterConfigSection
		expect bool
	}{
		{
			name: "vm requires pcs cluster",
			cfg:  deployStatusTestConfig("ablestack-vm", false),
		},
		{
			name:   "vm with pcs cluster is ready",
			cfg:    deployStatusTestConfig("ablestack-vm", true),
			expect: true,
		},
		{
			name: "hci requires three pcs cluster hosts",
			cfg:  deployStatusHCITestConfig(3, 1),
		},
		{
			name:   "hci with three pcs cluster hosts is ready",
			cfg:    deployStatusHCITestConfig(3, 3),
			expect: true,
		},
		{
			name:   "standalone does not require pcs cluster",
			cfg:    deployStatusTestConfig("ablestack-standalone", false),
			expect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDeployClusterConfigReady(&tt.cfg); got != tt.expect {
				t.Fatalf("isDeployClusterConfigReady() = %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestDeployCloudClusterStatusFromPCSResponse(t *testing.T) {
	tests := []struct {
		name   string
		resp   CCVMPCSControlResponse
		expect string
	}{
		{
			name:   "cluster not configured",
			resp:   CCVMPCSControlResponse{Code: http.StatusBadRequest, Message: "cluster is not configured."},
			expect: CubeModel.DeployRuntimeHealthErrCluster,
		},
		{
			name:   "resource not found",
			resp:   CCVMPCSControlResponse{Code: http.StatusBadRequest, Message: "resource not found."},
			expect: CubeModel.DeployRuntimeHealthErrResource,
		},
		{
			name: "started resource is healthy",
			resp: CCVMPCSControlResponse{
				Code: http.StatusOK,
				Val:  CCVMPCSStatusValue{Role: "Started", Started: "ablecube31-1", Active: "true", Failed: "false"},
			},
			expect: CubeModel.DeployRuntimeHealthOK,
		},
		{
			name: "remote map value is decoded",
			resp: CCVMPCSControlResponse{
				Code: http.StatusOK,
				Val: map[string]any{
					"role":    "Started",
					"started": "ablecube31-1",
					"active":  "true",
					"failed":  "false",
				},
			},
			expect: CubeModel.DeployRuntimeHealthOK,
		},
		{
			name: "stopped resource needs resource configure",
			resp: CCVMPCSControlResponse{
				Code: http.StatusOK,
				Val:  CCVMPCSStatusValue{Role: "Stopped"},
			},
			expect: CubeModel.DeployRuntimeHealthErrResource,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deployCloudClusterStatusFromPCSResponse(tt.resp); got != tt.expect {
				t.Fatalf("deployCloudClusterStatusFromPCSResponse() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestBuildDeployPollingPolicy(t *testing.T) {
	tests := []struct {
		name           string
		data           DeployStatusData
		storageVM      bool
		storageCluster bool
		gfs            bool
		cloudVM        bool
		cloudCluster   bool
		mold           bool
	}{
		{
			name: "cluster configuration blocks all polling",
			data: DeployStatusData{
				OSType: "ablestack-hci",
				Raw:    DeployStatusRaw{CCFGStatus: CubeModel.DeployStatusFalse},
			},
		},
		{
			name: "hci enables storage VM polling after storage VM is running",
			data: DeployStatusData{
				OSType: "ablestack-hci",
				Raw: DeployStatusRaw{
					CCFGStatus: CubeModel.DeployStatusTrue,
					SCVMStatus: CubeModel.DeployRuntimeRunning,
				},
			},
			storageVM: true,
		},
		{
			name: "hci enables storage cluster polling after storage VM bootstrap",
			data: DeployStatusData{
				OSType: "ablestack-hci",
				Raw: DeployStatusRaw{
					CCFGStatus:          CubeModel.DeployStatusTrue,
					SCVMBootstrapStatus: CubeModel.DeployStatusTrue,
				},
			},
			storageVM:      true,
			storageCluster: true,
		},
		{
			name: "vm enables cloud cluster polling after gfs configuration",
			data: DeployStatusData{
				OSType: "ablestack-vm",
				Raw: DeployStatusRaw{
					CCFGStatus:   CubeModel.DeployStatusTrue,
					GFSConfigure: CubeModel.DeployStatusTrue,
				},
			},
			gfs:          true,
			cloudCluster: true,
		},
		{
			name: "vm enables cloud VM polling after cloud VM is running",
			data: DeployStatusData{
				OSType: "ablestack-vm",
				Raw: DeployStatusRaw{
					CCFGStatus: CubeModel.DeployStatusTrue,
					CCVMStatus: CubeModel.DeployRuntimeRunning,
				},
			},
			cloudVM: true,
		},
		{
			name: "hci enables cloud polling after cloud VM is running",
			data: DeployStatusData{
				OSType: "ablestack-hci",
				Raw: DeployStatusRaw{
					CCFGStatus: CubeModel.DeployStatusTrue,
					CCVMStatus: CubeModel.DeployRuntimeRunning,
				},
			},
			cloudVM:      true,
			cloudCluster: true,
		},
		{
			name: "cloud bootstrap enables cloud VM and mold",
			data: DeployStatusData{
				OSType: "ablestack-vm",
				Raw: DeployStatusRaw{
					CCFGStatus:          CubeModel.DeployStatusTrue,
					CCVMBootstrapStatus: CubeModel.DeployStatusTrue,
				},
			},
			cloudVM: true,
			mold:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildDeployPollingPolicy(tt.data)
			if got.StorageVM.Enabled != tt.storageVM ||
				got.StorageCluster.Enabled != tt.storageCluster ||
				got.GFSResource.Enabled != tt.gfs ||
				got.GFSDisk.Enabled != tt.gfs ||
				got.CloudVM.Enabled != tt.cloudVM ||
				got.CloudCluster.Enabled != tt.cloudCluster ||
				got.Mold.Enabled != tt.mold {
				t.Fatalf("unexpected polling policy: %+v", got)
			}
		})
	}
}

func deployStatusTestConfig(osType string, withPCS bool) CubeModel.ClusterConfigSection {
	cfg := CubeModel.ClusterConfigSection{
		Type: osType,
		CCVM: CubeModel.ClusterCCVMConfig{
			IP: "10.10.31.10",
		},
		MngtNic: CubeModel.ClusterMngtNicConfig{
			CIDR: "16",
			GW:   "10.10.0.1",
			DNS:  "8.8.8.8",
		},
		Hosts: []CubeModel.ClusterHost{
			{
				Index:    "1",
				Hostname: "ablecube31-1",
				Ablecube: "10.10.31.1",
			},
		},
	}
	if withPCS {
		cfg.PCSCluster = CubeModel.ClusterPCSClusterConfig{
			Hostname1: "10.10.31.1",
		}
	}
	return cfg
}

func deployStatusHCITestConfig(hostCount int, pcsCount int) CubeModel.ClusterConfigSection {
	cfg := deployStatusTestConfig("ablestack-hci", false)
	cfg.Hosts = make([]CubeModel.ClusterHost, 0, hostCount)
	for i := 1; i <= hostCount; i++ {
		cfg.Hosts = append(cfg.Hosts, CubeModel.ClusterHost{
			Index:      fmt.Sprint(i),
			Hostname:   fmt.Sprintf("ablecube31-%d", i),
			Ablecube:   fmt.Sprintf("10.10.31.%d", i),
			AblecubePn: fmt.Sprintf("10.10.32.%d", i),
			ScvmMngt:   fmt.Sprintf("10.10.33.%d", i),
			Scvm:       fmt.Sprintf("10.10.34.%d", i),
			ScvmCn:     fmt.Sprintf("10.10.35.%d", i),
		})
	}
	hostnames := make([]string, 0, pcsCount)
	for i := 1; i <= pcsCount; i++ {
		hostnames = append(hostnames, fmt.Sprintf("10.10.32.%d", i))
	}
	cfg.PCSCluster = CubeModel.ClusterPCSClusterConfig{Hostnames: hostnames}
	return cfg
}
