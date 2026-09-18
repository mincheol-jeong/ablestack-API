package cube

import "testing"

func TestNormalizeSystemConfigConvenienceRequestGFSConfigure(t *testing.T) {
	req := SystemConfigRequest{Action: "gfs-configure"}
	normalizeSystemConfigConvenienceRequest(&req)

	if req.Action != "update" || req.Option != "all" || req.Depth1 != "bootstrap" || req.Depth2 != "gfs_configure" || req.Value != "true" {
		t.Fatalf("unexpected request: %#v", req)
	}
}
