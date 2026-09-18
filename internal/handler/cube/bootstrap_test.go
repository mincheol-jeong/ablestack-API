package cube

import (
	"encoding/base64"
	"reflect"
	"testing"

	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
)

func TestBuildSCVMBootstrapScriptTargetsPreparesAllHostsThenBootstrapsMaster(t *testing.T) {
	cfg := &CubeModel.ClusterConfigSection{Hosts: []CubeModel.ClusterHost{
		{Index: "3", Hostname: "ablecube3", Ablecube: "10.10.31.3"},
		{Index: "1", Hostname: "ablecube1", Ablecube: "10.10.31.1"},
		{Index: "2", Hostname: "ablecube2", Ablecube: "10.10.31.2"},
	}}

	targets, err := buildSCVMBootstrapScriptTargets(BootstrapRequest{}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := []bootstrapScriptTarget{
		{Role: "scvm", Action: "prepare", Hostname: "scvm1", Target: "10.10.31.1", Domain: "scvm", Args: []string{"prepare"}},
		{Role: "scvm", Action: "prepare", Hostname: "scvm2", Target: "10.10.31.2", Domain: "scvm", Args: []string{"prepare"}},
		{Role: "scvm", Action: "prepare", Hostname: "scvm3", Target: "10.10.31.3", Domain: "scvm", Args: []string{"prepare"}},
		{Role: "scvm", Action: "bootstrap", Hostname: "scvm1", Target: "10.10.31.1", Domain: "scvm", Args: []string{"bootstrap"}},
	}
	if !reflect.DeepEqual(targets, want) {
		t.Fatalf("targets = %#v, want %#v", targets, want)
	}
}

func TestParseCephadmPublicKey(t *testing.T) {
	want := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC test"
	output := "bootstrap complete\n" + bootstrapCephPublicKeyMarker + base64.StdEncoding.EncodeToString([]byte(want))

	got, err := parseCephadmPublicKey(output)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("key = %q, want %q", got, want)
	}
}

func TestParseCephadmPublicKeyRejectsMissingMarker(t *testing.T) {
	if _, err := parseCephadmPublicKey("bootstrap complete"); err == nil {
		t.Fatal("expected missing key error")
	}
}

func TestBuildSCVMBootstrapScriptTargetsHonorsExplicitTarget(t *testing.T) {
	cfg := &CubeModel.ClusterConfigSection{Hosts: []CubeModel.ClusterHost{
		{Index: "1", Hostname: "ablecube1", Ablecube: "10.10.31.1"},
		{Index: "2", Hostname: "ablecube2", Ablecube: "10.10.31.2"},
	}}

	targets, err := buildSCVMBootstrapScriptTargets(BootstrapRequest{TargetHostnames: []string{"scvm2"}}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 || targets[0].Hostname != "scvm2" || targets[0].Action != "prepare" || targets[1].Hostname != "scvm2" || targets[1].Action != "bootstrap" {
		t.Fatalf("unexpected targets: %#v", targets)
	}
}

func TestBuildCCVMBootstrapScriptTargetUsesCCVMAPI(t *testing.T) {
	cfg := &CubeModel.ClusterConfigSection{
		CCVM: CubeModel.ClusterCCVMConfig{IP: "10.10.31.10"},
		PCSCluster: CubeModel.ClusterPCSClusterConfig{
			Hostnames: []string{"10.10.31.1", "10.10.31.2"},
		},
	}

	target, err := buildCCVMBootstrapScriptTarget(BootstrapRequest{}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if target.Role != "ccvm" || target.Hostname != "ccvm" || target.Target != "10.10.31.10" || target.Domain != "" {
		t.Fatalf("unexpected CCVM bootstrap target: %#v", target)
	}
}

func TestBuildCCVMBootstrapNodePrepareTargetsUsesClusterHosts(t *testing.T) {
	cfg := &CubeModel.ClusterConfigSection{
		Type: "ablestack-hci",
		Hosts: []CubeModel.ClusterHost{
			{Index: "2", Hostname: "ablecube2", Ablecube: "10.10.31.2", ScvmMngt: "10.10.31.12"},
			{Index: "1", Hostname: "ablecube1", Ablecube: "10.10.31.1", ScvmMngt: "10.10.31.11"},
		},
	}

	targets, err := buildCCVMBootstrapNodePrepareTargets(cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := []bootstrapScriptTarget{
		{Role: "ccvm", Action: "configure_crushmap", Hostname: "scvm1", Target: "10.10.31.11"},
		{Role: "ccvm", Action: "prepare_host", Hostname: "ablecube1", Target: "10.10.31.1", Args: []string{"enable-cluster-services"}},
		{Role: "ccvm", Action: "prepare_host", Hostname: "ablecube2", Target: "10.10.31.2", Args: []string{"enable-cluster-services"}},
	}
	if !reflect.DeepEqual(targets, want) {
		t.Fatalf("targets = %#v, want %#v", targets, want)
	}
}

func TestBuildCCVMBootstrapNodePrepareTargetsSkipsPCSForStandalone(t *testing.T) {
	cfg := &CubeModel.ClusterConfigSection{
		Type: "ablestack-standalone",
		Hosts: []CubeModel.ClusterHost{
			{Index: "1", Hostname: "ablecube1", Ablecube: "10.10.31.1"},
		},
	}

	targets, err := buildCCVMBootstrapNodePrepareTargets(cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := []bootstrapScriptTarget{
		{Role: "ccvm", Action: "prepare_host", Hostname: "ablecube1", Target: "10.10.31.1"},
	}
	if !reflect.DeepEqual(targets, want) {
		t.Fatalf("targets = %#v, want %#v", targets, want)
	}
}
