package sample

import (
	"encoding/xml"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/goccy/go-json"
	"github.com/ycyun/Cube-API/internal/infra/utils"
	"os/exec"
	"reflect"
	"sync"
	"time"
)

type TypeSampleStatus struct {
	XMLName string `xml:"pacemaker-result"`
	Status  struct {
		Text    string `xml:",chardata"`
		Code    string `xml:"code,attr"`
		Message string `xml:"message,attr"`
	} `xml:"status"`
	RefreshTime time.Time
} // @name TypeSampleStatus

var lockSampleStatus sync.Once

var _SampleStatus *TypeSampleStatus

func Status() *TypeSampleStatus {
	if _SampleStatus == nil {
		lockSampleStatus.Do(
			func() {
				if gin.IsDebugging() {
					fmt.Println("Creating ", reflect.TypeOf(_SampleStatus), " now.")
				}
				_SampleStatus = &TypeSampleStatus{}
			})
	} else {
		if gin.IsDebugging() {
			fmt.Println("get old ", reflect.TypeOf(_SampleStatus), " instance.")
		}
	}
	return _SampleStatus
}

func UpdateStatus() *TypeSampleStatus {
	if gin.Mode() == gin.ReleaseMode {
		var stdout []byte

		cmd := exec.Command("ceph", "-s", "-f", "json")
		stdout, _ = cmd.CombinedOutput()
		if err := json.Unmarshal(stdout, &_SampleStatus); err != nil && gin.IsDebugging() {
			utils.FancyHandleError(err)

		}
	} else {

		stdout := []byte("<pacemaker-result api-version=\"2.25\" request=\"/usr/sbin/crm_mon --one-shot --inactive --output-as xml\">\n  <pacemakerd sys_from=\"pacemakerd\" state=\"running\" last_updated=\"2025-01-08 16:05:05 +09:00\"/>\n  <summary>\n    <stack type=\"corosync\"/>\n    <current_dc present=\"true\" version=\"2.1.5-9.el9_2-a3f44794f94\" name=\"100.100.1.2\" id=\"2\" with_quorum=\"true\" mixed_version=\"false\"/>\n    <last_update time=\"Wed Jan  8 16:05:05 2025\"/>\n    <last_change time=\"Mon Jan  6 09:49:13 2025\" user=\"root\" client=\"cibadmin\" origin=\"100.100.1.3\"/>\n    <nodes_configured number=\"3\"/>\n    <resources_configured number=\"1\" disabled=\"0\" blocked=\"0\"/>\n    <cluster_options stonith-enabled=\"false\" symmetric-cluster=\"true\" no-quorum-policy=\"stop\" maintenance-mode=\"false\" stop-all-resources=\"false\" stonith-timeout-ms=\"60000\" priority-fencing-delay-ms=\"0\"/>\n  </summary>\n  <nodes>\n    <node name=\"100.100.1.1\" id=\"1\" online=\"true\" standby=\"false\" standby_onfail=\"false\" maintenance=\"false\" pending=\"false\" unclean=\"false\" health=\"green\" feature_set=\"3.16.2\" shutdown=\"false\" expected_up=\"true\" is_dc=\"false\" resources_running=\"0\" type=\"member\"/>\n    <node name=\"100.100.1.2\" id=\"2\" online=\"true\" standby=\"false\" standby_onfail=\"false\" maintenance=\"false\" pending=\"false\" unclean=\"false\" health=\"green\" feature_set=\"3.16.2\" shutdown=\"false\" expected_up=\"true\" is_dc=\"true\" resources_running=\"1\" type=\"member\"/>\n    <node name=\"100.100.1.3\" id=\"3\" online=\"true\" standby=\"false\" standby_onfail=\"false\" maintenance=\"false\" pending=\"false\" unclean=\"false\" health=\"green\" feature_set=\"3.16.2\" shutdown=\"false\" expected_up=\"true\" is_dc=\"false\" resources_running=\"0\" type=\"member\"/>\n  </nodes>\n  <resources>\n    <resource id=\"cloudcenter_res\" resource_agent=\"ocf:heartbeat:VirtualDomain\" role=\"Started\" active=\"true\" orphaned=\"false\" blocked=\"false\" managed=\"true\" failed=\"false\" failure_ignored=\"false\" nodes_running_on=\"1\">\n      <node name=\"100.100.1.2\" id=\"2\" cached=\"true\"/>\n    </resource>\n  </resources>\n  <node_history>\n    <node name=\"100.100.1.1\">\n      <resource_history id=\"cloudcenter_res\" orphan=\"false\" migration-threshold=\"1000000\" fail-count=\"1000000\" last-failure=\"Wed Jan  8 09:01:30 2025\">\n        <operation_history call=\"9\" task=\"stop\" rc=\"1\" rc_text=\"error\" last-rc-change=\"Wed Jan  8 08:59:30 2025\" exec-time=\"120001ms\" queue-time=\"0ms\"/>\n        <operation_history call=\"13\" task=\"stop\" rc=\"0\" rc_text=\"ok\" last-rc-change=\"Wed Jan  8 09:01:36 2025\" exec-time=\"47ms\" queue-time=\"25739ms\"/>\n      </resource_history>\n    </node>\n    <node name=\"100.100.1.2\">\n      <resource_history id=\"cloudcenter_res\" orphan=\"false\" migration-threshold=\"1000000\">\n        <operation_history call=\"10\" task=\"start\" rc=\"0\" rc_text=\"ok\" last-rc-change=\"Wed Jan  8 09:02:21 2025\" exec-time=\"1649ms\" queue-time=\"0ms\"/>\n        <operation_history call=\"11\" task=\"monitor\" rc=\"0\" rc_text=\"ok\" interval=\"10000ms\" last-rc-change=\"Wed Jan  8 09:02:23 2025\" exec-time=\"129ms\" queue-time=\"0ms\"/>\n      </resource_history>\n    </node>\n  </node_history>\n  <failures>\n    <failure op_key=\"cloudcenter_res_stop_0\" node=\"100.100.1.1\" exitstatus=\"error\" exitreason=\"Resource agent did not complete within 2m\" exitcode=\"1\" call=\"9\" status=\"Timed Out\" last-rc-change=\"2025-01-08 08:59:30 +09:00\" queued=\"0\" exec=\"120001\" interval=\"0\" task=\"stop\"/>\n  </failures>\n  <status code=\"0\" message=\"OK\"/>\n</pacemaker-result>")

		if err := xml.Unmarshal(stdout, &_SampleStatus); err != nil {
			utils.FancyHandleError(err)

		}
	}
	_SampleStatus.RefreshTime = time.Now()
	return _SampleStatus
}
