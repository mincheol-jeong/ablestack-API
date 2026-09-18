package cube

import (
	"reflect"
	"testing"
)

func TestCCVMXMLInstallTargetsForVMUsesAblecubeWithoutStorageNetwork(t *testing.T) {
	cfg := ccvmCloudInitTargetTestConfig("ablestack-vm", "false")
	for index := range cfg.Hosts {
		cfg.Hosts[index].AblecubePn = ""
	}

	targets, field := ccvmXMLInstallTargets(&cfg)

	if field != "hosts[].ablecube" {
		t.Fatalf("field = %q, want hosts[].ablecube", field)
	}
	want := []string{"10.10.31.1", "10.10.31.2"}
	if !reflect.DeepEqual(targets, want) {
		t.Fatalf("targets = %#v, want %#v", targets, want)
	}
}

func TestCCVMXMLInstallTargetsForHCIUsesAblecubePn(t *testing.T) {
	cfg := ccvmCloudInitTargetTestConfig("ablestack-hci", "false")

	targets, field := ccvmXMLInstallTargets(&cfg)

	if field != "hosts[].ablecubePn" {
		t.Fatalf("field = %q, want hosts[].ablecubePn", field)
	}
	want := []string{"100.100.31.1", "100.100.31.2"}
	if !reflect.DeepEqual(targets, want) {
		t.Fatalf("targets = %#v, want %#v", targets, want)
	}
}
