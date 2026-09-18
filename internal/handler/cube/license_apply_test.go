package cube

import (
	"reflect"
	"testing"
	"time"

	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
)

func TestCCVMLicenseReadyPolicy(t *testing.T) {
	if ccvmLicenseReadyInterval != 30*time.Second {
		t.Fatalf("ready interval = %s", ccvmLicenseReadyInterval)
	}
	if ccvmLicenseReadyAttempts != 6 {
		t.Fatalf("ready attempts = %d", ccvmLicenseReadyAttempts)
	}
}

func TestCCVMLicensePCSReadyRequiresStartedNode(t *testing.T) {
	if !ccvmLicensePCSReady(ccvmSecondaryResizePCSStatusValue{Role: "Started", Started: "ablecube13-2"}) {
		t.Fatal("started cloudcenter_res should be ready")
	}
	if ccvmLicensePCSReady(ccvmSecondaryResizePCSStatusValue{Role: "Started"}) {
		t.Fatal("cloudcenter_res without a started node must not be ready")
	}
	if ccvmLicensePCSReady(ccvmSecondaryResizePCSStatusValue{Role: "Stopped", Started: "ablecube13-2"}) {
		t.Fatal("stopped cloudcenter_res must not be ready")
	}
}

func TestBuildLicenseApplyTargetsDefaultsToAblecube(t *testing.T) {
	cfg := licenseApplyTestClusterConfig()

	targets, err := buildLicenseApplyTargets(LicenseApplyRequest{}, &cfg)
	if err != nil {
		t.Fatalf("buildLicenseApplyTargets error = %v", err)
	}

	want := []string{
		"ablecube|ablecube31-1|10.10.31.1",
		"ablecube|ablecube31-2|10.10.31.2",
	}
	if got := licenseApplyTargetIDs(targets); !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %#v, want %#v", got, want)
	}
}

func TestBuildLicenseApplyTargetsRoleSCVM(t *testing.T) {
	cfg := licenseApplyTestClusterConfig()

	targets, err := buildLicenseApplyTargets(LicenseApplyRequest{Roles: []string{"scvm"}}, &cfg)
	if err != nil {
		t.Fatalf("buildLicenseApplyTargets error = %v", err)
	}

	want := []string{
		"scvm|scvm1|10.10.31.11",
		"scvm|scvm2|10.10.31.12",
	}
	if got := licenseApplyTargetIDs(targets); !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %#v, want %#v", got, want)
	}
}

func TestBuildLicenseApplyTargetsRoleCCVM(t *testing.T) {
	cfg := licenseApplyTestClusterConfig()

	targets, err := buildLicenseApplyTargets(LicenseApplyRequest{Roles: []string{"ccvm"}}, &cfg)
	if err != nil {
		t.Fatalf("buildLicenseApplyTargets error = %v", err)
	}

	want := []string{"ccvm|ccvm|10.10.31.10"}
	if got := licenseApplyTargetIDs(targets); !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %#v, want %#v", got, want)
	}
}

func TestBuildLicenseApplyTargetsRoleAll(t *testing.T) {
	cfg := licenseApplyTestClusterConfig()

	targets, err := buildLicenseApplyTargets(LicenseApplyRequest{Roles: []string{"all"}}, &cfg)
	if err != nil {
		t.Fatalf("buildLicenseApplyTargets error = %v", err)
	}

	want := []string{
		"ablecube|ablecube31-1|10.10.31.1",
		"ablecube|ablecube31-2|10.10.31.2",
		"scvm|scvm1|10.10.31.11",
		"scvm|scvm2|10.10.31.12",
		"ccvm|ccvm|10.10.31.10",
	}
	if got := licenseApplyTargetIDs(targets); !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %#v, want %#v", got, want)
	}
}

func TestBuildLicenseApplyTargetsExplicitTargetsIgnoreRoles(t *testing.T) {
	cfg := licenseApplyTestClusterConfig()

	targets, err := buildLicenseApplyTargets(LicenseApplyRequest{
		Roles:   []string{"all"},
		Targets: []string{"10.10.99.1", "10.10.99.1"},
	}, &cfg)
	if err != nil {
		t.Fatalf("buildLicenseApplyTargets error = %v", err)
	}

	want := []string{"custom||10.10.99.1"}
	if got := licenseApplyTargetIDs(targets); !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %#v, want %#v", got, want)
	}
}

func TestBuildLicenseApplyTargetsFilterSCVMByParentHostname(t *testing.T) {
	cfg := licenseApplyTestClusterConfig()

	targets, err := buildLicenseApplyTargets(LicenseApplyRequest{
		Roles:           []string{"scvm"},
		TargetHostnames: []string{"ablecube31-2"},
	}, &cfg)
	if err != nil {
		t.Fatalf("buildLicenseApplyTargets error = %v", err)
	}

	want := []string{"scvm|scvm2|10.10.31.12"}
	if got := licenseApplyTargetIDs(targets); !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %#v, want %#v", got, want)
	}
}

func TestLicenseContentForSCVMTargetUsesSCVMAlias(t *testing.T) {
	cfg := licenseApplyTestClusterConfig()
	targets, err := buildLicenseApplyTargets(LicenseApplyRequest{Roles: []string{"scvm"}}, &cfg)
	if err != nil {
		t.Fatalf("buildLicenseApplyTargets error = %v", err)
	}

	content := licenseContentForTarget(LicenseApplyRequest{
		Licenses: map[string]string{"scvm2": "SCVM2_LICENSE"},
	}, targets[1], "DEFAULT_LICENSE")
	if content != "SCVM2_LICENSE" {
		t.Fatalf("content = %q, want SCVM2_LICENSE", content)
	}
}

func licenseApplyTestClusterConfig() CubeModel.ClusterConfigSection {
	return CubeModel.ClusterConfigSection{
		CCVM: CubeModel.ClusterCCVMConfig{IP: "10.10.31.10"},
		Hosts: []CubeModel.ClusterHost{
			{
				Index:    "1",
				Hostname: "ablecube31-1",
				Ablecube: "10.10.31.1",
				ScvmMngt: "10.10.31.11",
				Scvm:     "100.100.31.11",
			},
			{
				Index:    "2",
				Hostname: "ablecube31-2",
				Ablecube: "10.10.31.2",
				ScvmMngt: "10.10.31.12",
				Scvm:     "100.100.31.12",
			},
		},
	}
}

func licenseApplyTargetIDs(targets []licenseApplyTarget) []string {
	out := make([]string, 0, len(targets))
	for _, target := range targets {
		out = append(out, target.Role+"|"+target.Hostname+"|"+target.Target)
	}
	return out
}
