package moldservice

import "testing"

func TestFindCloudStackHostByNameOrIP(t *testing.T) {
	response := map[string]any{
		"listhostsresponse": map[string]any{
			"host": []any{
				map[string]any{"id": "host-1", "name": "ablecube1", "ipaddress": "10.10.13.1"},
				map[string]any{"id": "host-2", "name": "ablecube2", "ipaddress": "10.10.13.2"},
			},
		},
	}

	if host := findCloudStackHost(response, "ABLECUBE2", ""); stringValue(host["id"]) != "host-2" {
		t.Fatalf("name lookup returned %#v", host)
	}
	if host := findCloudStackHost(response, "", "10.10.13.1"); stringValue(host["id"]) != "host-1" {
		t.Fatalf("IP lookup returned %#v", host)
	}
	if host := findCloudStackHost(response, "missing", "10.10.13.99"); host != nil {
		t.Fatalf("missing host lookup returned %#v", host)
	}
}

func TestNestedString(t *testing.T) {
	response := map[string]any{
		"queryasyncjobresultresponse": map[string]any{
			"jobresult": map[string]any{"errortext": "host still has running VMs"},
		},
	}
	if got := nestedString(response, "queryasyncjobresultresponse", "jobresult", "errortext"); got != "host still has running VMs" {
		t.Fatalf("nestedString() = %q", got)
	}
}

func TestNestedValueSupportsBooleanDeleteResponse(t *testing.T) {
	response := map[string]any{
		"deletehostresponse": map[string]any{"success": true},
	}
	if !boolValue(nestedValue(response, "deletehostresponse", "success")) {
		t.Fatal("deleteHost success response was not recognized")
	}
}
