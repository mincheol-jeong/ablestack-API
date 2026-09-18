package clusterconfig

import (
	"encoding/json"
	"strconv"
	"testing"
)

func TestNormalizeClusterJSONMigratesIscsiStorageToStorageNetwork(t *testing.T) {
	root := map[string]any{
		"clusterConfig": map[string]any{
			"type":                "ablestack-vm",
			"ccvm":                map[string]any{"ip": "10.10.31.10"},
			"mngtNic":             map[string]any{"cidr": "16", "gw": "10.10.0.1", "dns": "8.8.8.8"},
			"pcsCluster":          map[string]any{},
			"hosts":               []any{},
			"external_timeserver": "time.google.com",
			"iscsi_storage":       "true",
		},
	}

	normalized := NormalizeClusterJSON(root)
	rawCfg, err := json.Marshal(normalized["clusterConfig"])
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(rawCfg, &cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg["storage_network"]; got != "true" {
		t.Fatalf("storage_network = %#v, want true", got)
	}
	if _, ok := cfg["iscsi_storage"]; ok {
		t.Fatalf("iscsi_storage should not be present")
	}
}

func TestNormalizeClusterJSONPreservesHostType(t *testing.T) {
	root := map[string]any{
		"clusterConfig": map[string]any{
			"type":       "ablestack-hci",
			"hostType":   "add",
			"ccvm":       map[string]any{},
			"mngtNic":    map[string]any{},
			"pcsCluster": map[string]any{},
			"hosts":      []any{},
		},
	}

	normalized := NormalizeClusterJSON(root)
	raw, err := json.Marshal(normalized["clusterConfig"])
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg["hostType"]; got != "add" {
		t.Fatalf("hostType = %#v, want add", got)
	}
}

func TestNormalizeClusterJSONAddsGFSFormatDefaultsAndDescriptions(t *testing.T) {
	root := map[string]any{
		"clusterConfig": map[string]any{
			"type":       "ablestack-vm",
			"ccvm":       map[string]any{},
			"mngtNic":    map[string]any{},
			"pcsCluster": map[string]any{},
			"hosts":      []any{},
		},
	}

	normalized := NormalizeClusterJSON(root)
	raw, err := json.Marshal(normalized["clusterConfig"])
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	gfs, ok := cfg["gfs"].(map[string]any)
	if !ok {
		t.Fatalf("gfs config missing: %#v", cfg)
	}
	if gfs["journal_size_mb"] != float64(512) || gfs["resource_group_size_mb"] != float64(1024) {
		t.Fatalf("unexpected GFS defaults: %#v", gfs)
	}
	if gfs["journal_size_description"] == "" || gfs["resource_group_size_description"] == "" {
		t.Fatalf("GFS descriptions missing: %#v", gfs)
	}
}

func TestNormalizeClusterJSONPreservesGFSFormatValues(t *testing.T) {
	root := map[string]any{
		"clusterConfig": map[string]any{
			"ccvm":       map[string]any{},
			"mngtNic":    map[string]any{},
			"pcsCluster": map[string]any{},
			"hosts":      []any{},
			"gfs": map[string]any{
				"journal_size_mb":        256,
				"resource_group_size_mb": 2048,
			},
		},
	}

	normalized := NormalizeClusterJSON(root)
	raw, err := json.Marshal(normalized["clusterConfig"])
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		GFS struct {
			JournalSizeMB       int `json:"journal_size_mb"`
			ResourceGroupSizeMB int `json:"resource_group_size_mb"`
		} `json:"gfs"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.GFS.JournalSizeMB != 256 || cfg.GFS.ResourceGroupSizeMB != 2048 {
		t.Fatalf("unexpected GFS values: %#v", cfg.GFS)
	}
}

func TestRenumberHostIndexes(t *testing.T) {
	hosts := []map[string]any{
		{"index": "1", "hostname": "ablecube12-1"},
		{"index": "3", "hostname": "ablecube12-3"},
		{"index": "4", "hostname": "ablecube12-4"},
	}

	renumberHostIndexes(hosts)

	for i, host := range hosts {
		want := strconv.Itoa(i + 1)
		if got := host["index"]; got != want {
			t.Fatalf("hosts[%d].index = %#v, want %q", i, got, want)
		}
	}
}
