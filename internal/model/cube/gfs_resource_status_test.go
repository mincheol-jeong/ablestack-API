package cube

import "testing"

func TestParseGFSResourceStatusXMLIncludesMountResourcesPerNode(t *testing.T) {
	input := `<pacemaker-result>
  <nodes>
    <node name="10.10.13.1" online="true"/>
    <node name="10.10.13.2" online="true"/>
  </nodes>
  <resources>
    <clone id="glue-gfs_res-clone">
      <resource id="glue-gfs_res" resource_agent="ocf:heartbeat:LVM-activate" role="Started" active="true"><node name="10.10.13.1"/></resource>
      <resource id="glue-gfs_res" resource_agent="ocf:heartbeat:LVM-activate" role="Started" active="true"><node name="10.10.13.2"/></resource>
    </clone>
    <clone id="glue-gfs-clone">
      <resource id="glue-gfs" resource_agent="ocf:heartbeat:Filesystem" role="Started" active="true"><node name="10.10.13.1"/></resource>
      <resource id="glue-gfs" resource_agent="ocf:heartbeat:Filesystem" role="Started" active="true"><node name="10.10.13.2"/></resource>
    </clone>
  </resources>
</pacemaker-result>`

	status, err := ParseGFSResourceStatusXML([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	resources := status.Resources.GFSMountResources
	if len(resources) != 4 {
		t.Fatalf("gfs_mount_resources = %#v", resources)
	}
	counts := map[string]int{}
	for _, resource := range resources {
		counts[resource["id"]]++
		if resource["node_name"] == "" || resource["role"] != "Started" {
			t.Fatalf("resource = %#v", resource)
		}
	}
	if counts["glue-gfs"] != 2 || counts["glue-gfs_res"] != 2 {
		t.Fatalf("resource counts = %#v", counts)
	}
	if len(status.Resources.GlueGFSResources) != len(resources) {
		t.Fatalf("compatibility resources = %#v", status.Resources.GlueGFSResources)
	}
}

func TestParseGFSResourceStatusXMLExcludesSimilarResourceNames(t *testing.T) {
	input := `<pacemaker-result><resources><resource id="glue-gfs-backup" role="Started" active="true"/></resources></pacemaker-result>`
	status, err := ParseGFSResourceStatusXML([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Resources.GFSMountResources) != 0 {
		t.Fatalf("gfs_mount_resources = %#v", status.Resources.GFSMountResources)
	}
}
