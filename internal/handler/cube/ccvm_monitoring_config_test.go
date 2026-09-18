package cube

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
)

func TestCheckCCVMMonitoringAPIHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/health" {
			http.NotFound(response, request)
			return
		}
		response.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	host, port, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ABLESTACK_API_SCHEME", "http")
	t.Setenv("ABLESTACK_API_PORT", port)
	if _, err := checkCCVMMonitoringAPIHealth(context.Background(), host); err != nil {
		t.Fatal(err)
	}
}

func TestRunCCVMMonitoringStepReportsConfiguredTimeout(t *testing.T) {
	step := runCCVMMonitoringStep("slow_step", time.Millisecond, func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	if step.Status != "failed" || step.Message != "slow_step timed out after 1ms" {
		t.Fatalf("step = %+v", step)
	}
}

func TestBuildCCVMMonitoringConfigRequest(t *testing.T) {
	cfg := &CubeModel.ClusterConfigSection{
		Type: "ablestack-hci",
		CCVM: CubeModel.ClusterCCVMConfig{IP: "10.10.31.10"},
		Hosts: []CubeModel.ClusterHost{
			{Ablecube: "10.10.31.1", ScvmMngt: "10.10.31.11"},
			{Ablecube: "10.10.31.2", ScvmMngt: "10.10.31.12"},
		},
	}

	req := buildCCVMMonitoringConfigRequest(cfg)
	if err := normalizeCCVMMonitoringConfigRequest(&req); err != nil {
		t.Fatalf("normalizeCCVMMonitoringConfigRequest() error = %v", err)
	}

	if !reflect.DeepEqual(req.CCVM, []string{"10.10.31.10"}) {
		t.Fatalf("CCVM = %#v", req.CCVM)
	}
	if !reflect.DeepEqual(req.Cube, []string{"10.10.31.1", "10.10.31.2"}) {
		t.Fatalf("Cube = %#v", req.Cube)
	}
	if !reflect.DeepEqual(req.SCVM, []string{"10.10.31.11", "10.10.31.12"}) {
		t.Fatalf("SCVM = %#v", req.SCVM)
	}
}

func TestMergeCCVMMonitoringConfigRequestPreservesClusterTargets(t *testing.T) {
	target := CCVMMonitoringConfigRequest{
		Action:      "update",
		CCVM:        []string{"10.10.31.10"},
		Cube:        []string{"10.10.31.1"},
		SCVM:        []string{"10.10.32.1"},
		ClusterType: "ablestack-hci",
	}
	input := CCVMMonitoringConfigRequest{
		Action: "configure",
		CCVM:   []string{"10.10.41.10"},
		Cube:   []string{"10.10.41.1", "10.10.41.2"},
		SCVM:   []string{"10.10.42.1", "10.10.42.2"},
		SMTP: &CubeModel.CCVMMonitoringSMTPConfig{
			Enabled:  true,
			Host:     "mail.example.com",
			Port:     25,
			User:     "admin@example.com",
			Password: "secret",
		},
	}

	mergeCCVMMonitoringConfigRequest(&target, input)
	if err := normalizeCCVMMonitoringConfigRequest(&target); err != nil {
		t.Fatal(err)
	}
	if target.Action != "configure" {
		t.Fatalf("unexpected merged request: %+v", target)
	}
	if !reflect.DeepEqual(target.CCVM, []string{"10.10.31.10"}) || !reflect.DeepEqual(target.Cube, []string{"10.10.31.1"}) || !reflect.DeepEqual(target.SCVM, []string{"10.10.32.1"}) {
		t.Fatalf("cluster targets were overwritten: %+v", target)
	}
	if target.ClusterType != "ablestack-hci" {
		t.Fatalf("ClusterType = %q", target.ClusterType)
	}
	if target.SMTP == nil || !target.SMTP.Enabled || target.SMTP.Host != "mail.example.com" {
		t.Fatalf("SMTP was not merged: %+v", target.SMTP)
	}
}

func TestBuildCCVMMonitoringConfigRequestUsesProductTargets(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
		wantCube []string
		wantSCVM []string
	}{
		{
			name:     "vm excludes scvm",
			typeName: "ablestack-vm",
			wantCube: []string{"10.10.31.1", "10.10.31.2"},
			wantSCVM: []string{},
		},
		{
			name:     "standalone uses first host",
			typeName: "ablestack-standalone",
			wantCube: []string{"10.10.31.1"},
			wantSCVM: []string{},
		},
		{
			name:     "hci includes scvm",
			typeName: "ablestack-hci",
			wantCube: []string{"10.10.31.1", "10.10.31.2"},
			wantSCVM: []string{"10.10.31.11", "10.10.31.12"},
		},
		{
			name:     "hci filesystem includes scvm",
			typeName: "ablestack-hci-filesystem",
			wantCube: []string{"10.10.31.1", "10.10.31.2"},
			wantSCVM: []string{"10.10.31.11", "10.10.31.12"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &CubeModel.ClusterConfigSection{
				Type: tt.typeName,
				CCVM: CubeModel.ClusterCCVMConfig{IP: "10.10.31.10"},
				Hosts: []CubeModel.ClusterHost{
					{Ablecube: "10.10.31.1", ScvmMngt: "10.10.31.11"},
					{Ablecube: "10.10.31.2", ScvmMngt: "10.10.31.12"},
				},
			}

			req := buildCCVMMonitoringConfigRequest(cfg)
			if err := normalizeCCVMMonitoringConfigRequest(&req); err != nil {
				t.Fatalf("normalizeCCVMMonitoringConfigRequest() error = %v", err)
			}
			if !reflect.DeepEqual(req.Cube, tt.wantCube) {
				t.Fatalf("Cube = %#v, want %#v", req.Cube, tt.wantCube)
			}
			if !reflect.DeepEqual(req.SCVM, tt.wantSCVM) {
				t.Fatalf("SCVM = %#v, want %#v", req.SCVM, tt.wantSCVM)
			}
		})
	}
}

func TestNormalizeCCVMMonitoringConfigRequestAllowsVMWithoutSCVM(t *testing.T) {
	req := CCVMMonitoringConfigRequest{
		CCVM: []string{"10.10.31.10"},
		Cube: []string{"10.10.31.1", "10.10.31.1", ""},
	}

	if err := normalizeCCVMMonitoringConfigRequest(&req); err != nil {
		t.Fatalf("normalizeCCVMMonitoringConfigRequest() error = %v", err)
	}
	if !reflect.DeepEqual(req.Cube, []string{"10.10.31.1"}) {
		t.Fatalf("Cube = %#v", req.Cube)
	}
}

func TestNormalizeCCVMMonitoringConfigRequestRequiresAddresses(t *testing.T) {
	tests := []struct {
		name string
		req  CCVMMonitoringConfigRequest
	}{
		{name: "ccvm", req: CCVMMonitoringConfigRequest{Cube: []string{"10.10.31.1"}}},
		{name: "cube", req: CCVMMonitoringConfigRequest{CCVM: []string{"10.10.31.10"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := normalizeCCVMMonitoringConfigRequest(&tt.req); err == nil {
				t.Fatal("normalizeCCVMMonitoringConfigRequest() error = nil")
			}
		})
	}
}

func TestNormalizeCCVMMonitoringConfigRequestRequiresSCVMForHCI(t *testing.T) {
	req := CCVMMonitoringConfigRequest{
		ClusterType: "ablestack-hci-filesystem",
		CCVM:        []string{"10.10.31.10"},
		Cube:        []string{"10.10.31.1"},
	}

	if err := normalizeCCVMMonitoringConfigRequest(&req); err == nil {
		t.Fatal("normalizeCCVMMonitoringConfigRequest() error = nil")
	}
}

func TestMonitoringPythonArgumentsMatchLegacyWallScripts(t *testing.T) {
	req := CCVMMonitoringConfigRequest{
		CCVM: []string{"10.10.31.10"},
		Cube: []string{"10.10.31.1", "10.10.31.2"},
		SCVM: []string{"10.10.32.1", "10.10.32.2"},
	}

	if got, want := monitoringPingArgs(req), []string{"-hns", "10.10.31.10", "10.10.31.1", "10.10.31.2", "10.10.32.1", "10.10.32.2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("monitoringPingArgs() = %#v, want %#v", got, want)
	}
	if got, want := monitoringConfigArgs("configure", req, true), []string{"config", "--ccvm", "10.10.31.10", "--cube", "10.10.31.1", "10.10.31.2", "--scvm", "10.10.32.1", "10.10.32.2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("monitoringConfigArgs() = %#v, want %#v", got, want)
	}
	if got, want := monitoringConfigArgs("update", req, false)[0], "update"; got != want {
		t.Fatalf("monitoringConfigArgs(update) action = %q, want %q", got, want)
	}
	if got := monitoringServiceArgs("stop"); reflect.DeepEqual(got, monitoringServiceArgs("start")) || got[len(got)-1] != "prometheus" {
		t.Fatalf("monitoringServiceArgs(stop) = %#v", got)
	}
}
