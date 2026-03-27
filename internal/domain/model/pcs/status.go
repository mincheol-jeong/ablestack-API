package pcs

import (
	"encoding/xml"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/ycyun/Cube-API/internal/infra/utils"
	"os/exec"
	"reflect"
	"sync"
	"time"
)

type TypePCSResources struct {
	PCSResources []TypePCSResource `json:"PCSResource"`
}
type TypePCSResource struct {
	//Text           string `xml:",chardata"`
	ID             string `xml:"id,attr"`
	ResourceAgent  string `xml:"resource_agent,attr"`
	Role           string `xml:"role,attr"`
	Active         string `xml:"active,attr"`
	Orphaned       string `xml:"orphaned,attr"`
	Blocked        string `xml:"blocked,attr"`
	Maintenance    string `xml:"maintenance,attr"`
	Managed        string `xml:"managed,attr"`
	Failed         string `xml:"failed,attr"`
	FailureIgnored string `xml:"failure_ignored,attr"`
	NodesRunningOn string `xml:"nodes_running_on,attr"`
	Node           struct {
		//Text   string `xml:",chardata"`
		Name   string `xml:"name,attr"`
		ID     string `xml:"id,attr"`
		Cached string `xml:"cached,attr"`
	} `xml:"node"`
} // @name TypePCSesource
type TypePCSResourceGroup struct {
	//Text            string            `xml:",chardata"`
	ID              string            `xml:"id,attr"`
	NumberResources string            `xml:"number_resources,attr"`
	Maintenance     string            `xml:"maintenance,attr"`
	Managed         string            `xml:"managed,attr"`
	Disabled        string            `xml:"disabled,attr"`
	Resource        []TypePCSResource `xml:"resource"`
}

type TypePCSClone struct {
	//Text           string                 `xml:",chardata"`
	ID             string                 `xml:"id,attr"`
	MultiState     string                 `xml:"multi_state,attr"`
	Unique         string                 `xml:"unique,attr"`
	Maintenance    string                 `xml:"maintenance,attr"`
	Managed        string                 `xml:"managed,attr"`
	Disabled       string                 `xml:"disabled,attr"`
	Failed         string                 `xml:"failed,attr"`
	FailureIgnored string                 `xml:"failure_ignored,attr"`
	Group          []TypePCSResourceGroup `xml:"group"`
	Resource       []TypePCSResource      `xml:"resource"`
}

type TypePCSStatus struct {
	//XMLName    xml.Name `xml:"PCS-result"`
	//Text       string `xml:",chardata"`
	ApiVersion string `xml:"api-version,attr"`
	Request    string `xml:"request,attr"`
	Summary    struct {
		//Text  string `xml:",chardata"`
		Stack struct {
			//Text      string `xml:",chardata"`
			Type      string `xml:"type,attr"`
			PCSdState string `xml:"PCSd-state,attr"`
		} `xml:"stack"`
		CurrentDc struct {
			//Text         string `xml:",chardata"`
			Present      string `xml:"present,attr"`
			Version      string `xml:"version,attr"`
			Name         string `xml:"name,attr"`
			ID           string `xml:"id,attr"`
			WithQuorum   string `xml:"with_quorum,attr"`
			MixedVersion string `xml:"mixed_version,attr"`
		} `xml:"current_dc"`
		LastUpdate struct {
			//Text   string `xml:",chardata"`
			Time   string `xml:"time,attr"`
			Origin string `xml:"origin,attr"`
		} `xml:"last_update"`
		LastChange struct {
			//Text   string `xml:",chardata"`
			Time   string `xml:"time,attr"`
			User   string `xml:"user,attr"`
			Client string `xml:"client,attr"`
			Origin string `xml:"origin,attr"`
		} `xml:"last_change"`
		NodesConfigured struct {
			//Text   string `xml:",chardata"`
			Number string `xml:"number,attr"`
		} `xml:"nodes_configured"`
		ResourcesConfigured struct {
			//Text     string `xml:",chardata"`
			Number   string `xml:"number,attr"`
			Disabled string `xml:"disabled,attr"`
			Blocked  string `xml:"blocked,attr"`
		} `xml:"resources_configured"`
		ClusterOptions struct {
			//Text                   string `xml:",chardata"`
			StonithEnabled         string `xml:"stonith-enabled,attr"`
			SymmetricCluster       string `xml:"symmetric-cluster,attr"`
			NoQuorumPolicy         string `xml:"no-quorum-policy,attr"`
			MaintenanceMode        string `xml:"maintenance-mode,attr"`
			StopAllResources       string `xml:"stop-all-resources,attr"`
			StonithTimeoutMs       string `xml:"stonith-timeout-ms,attr"`
			PriorityFencingDelayMs string `xml:"priority-fencing-delay-ms,attr"`
		} `xml:"cluster_options"`
	} `xml:"summary"`
	Nodes struct {
		//Text string `xml:",chardata"`
		Node []struct {
			//Text             string `xml:",chardata"`
			Name             string `xml:"name,attr"`
			ID               string `xml:"id,attr"`
			Online           string `xml:"online,attr"`
			Standby          string `xml:"standby,attr"`
			StandbyOnfail    string `xml:"standby_onfail,attr"`
			Maintenance      string `xml:"maintenance,attr"`
			Pending          string `xml:"pending,attr"`
			Unclean          string `xml:"unclean,attr"`
			Health           string `xml:"health,attr"`
			FeatureSet       string `xml:"feature_set,attr"`
			Shutdown         string `xml:"shutdown,attr"`
			ExpectedUp       string `xml:"expected_up,attr"`
			IsDc             string `xml:"is_dc,attr"`
			ResourcesRunning string `xml:"resources_running,attr"`
			Type             string `xml:"type,attr"`
		} `xml:"node"`
	} `xml:"nodes"`
	Resources struct {
		//Text     string            `xml:",chardata"`
		Resource []TypePCSResource `xml:"resource"`
		Clone    []TypePCSClone    `xml:"clone"`
	} `xml:"resources"`
	NodeHistory struct {
		//Text string `xml:",chardata"`
		Node []struct {
			//Text            string `xml:",chardata"`
			Name            string `xml:"name,attr"`
			ResourceHistory []struct {
				//Text               string `xml:",chardata"`
				ID                 string `xml:"id,attr"`
				Orphan             string `xml:"orphan,attr"`
				MigrationThreshold string `xml:"migration-threshold,attr"`
				OperationHistory   []struct {
					//Text         string `xml:",chardata"`
					Call         string `xml:"call,attr"`
					Task         string `xml:"task,attr"`
					Rc           string `xml:"rc,attr"`
					RcText       string `xml:"rc_text,attr"`
					LastRcChange string `xml:"last-rc-change,attr"`
					ExecTime     string `xml:"exec-time,attr"`
					QueueTime    string `xml:"queue-time,attr"`
					Interval     string `xml:"interval,attr"`
				} `xml:"operation_history"`
			} `xml:"resource_history"`
		} `xml:"node"`
	} `xml:"node_history"`
	Bans struct {
		//Text string `xml:",chardata"`
		Ban []struct {
			//Text         string `xml:",chardata"`
			ID           string `xml:"id,attr"`
			Resource     string `xml:"resource,attr"`
			Node         string `xml:"node,attr"`
			Weight       string `xml:"weight,attr"`
			PromotedOnly string `xml:"promoted-only,attr"`
			MasterOnly   string `xml:"master_only,attr"`
		} `xml:"ban"`
	} `xml:"bans"`
	Status struct {
		//Text    string `xml:",chardata"`
		Code    string `xml:"code,attr"`
		Message string `xml:"message,attr"`
	} `xml:"status"`
	RefreshTime time.Time
} // @name TypePCSStatus

var lockPCSResult sync.Once

var _PCSResult *TypePCSStatus

func Status() *TypePCSStatus {
	if _PCSResult == nil {
		lockPCSResult.Do(
			func() {
				if gin.IsDebugging() {
					fmt.Println("Creating ", reflect.TypeOf(_PCSResult), " now.")
				}
				_PCSResult = &TypePCSStatus{}
			})
	} else {
		if gin.IsDebugging() {
			fmt.Println("get old ", reflect.TypeOf(_PCSResult), " instance.")
		}
	}
	return _PCSResult
}

func UpdateStatus() *TypePCSStatus {
	if gin.Mode() == gin.ReleaseMode {
		var stdout []byte

		cmd := exec.Command("pcs", "status", "xml")
		stdout, _ = cmd.CombinedOutput()
		if err := xml.Unmarshal(stdout, &_PCSResult); err != nil && gin.IsDebugging() {
			utils.FancyHandleError(err)

		}
	} else {

		stdout := []byte("<PCS-result api-version=\"2.32\" request=\"/usr/sbin/crm_mon --one-shot --inactive --output-as xml\">\n  <summary>\n    <stack type=\"corosync\" PCSd-state=\"running\"/>\n    <current_dc present=\"true\" version=\"2.1.7-5.2.el9_4-0f7f88312\" name=\"10.10.32.2\" id=\"2\" with_quorum=\"true\" mixed_version=\"false\"/>\n    <last_update time=\"Wed Jan  8 16:52:45 2025\" origin=\"10.10.32.1\"/>\n    <last_change time=\"Wed Jan  8 16:20:27 2025\" user=\"root\" client=\"root\" origin=\"10.10.32.1\"/>\n    <nodes_configured number=\"3\"/>\n    <resources_configured number=\"16\" disabled=\"0\" blocked=\"0\"/>\n    <cluster_options stonith-enabled=\"true\" symmetric-cluster=\"true\" no-quorum-policy=\"stop\" maintenance-mode=\"false\" stop-all-resources=\"false\" stonith-timeout-ms=\"60000\" priority-fencing-delay-ms=\"0\"/>\n  </summary>\n  <nodes>\n    <node name=\"10.10.32.1\" id=\"1\" online=\"true\" standby=\"false\" standby_onfail=\"false\" maintenance=\"false\" pending=\"false\" unclean=\"false\" health=\"green\" feature_set=\"3.19.0\" shutdown=\"false\" expected_up=\"true\" is_dc=\"false\" resources_running=\"7\" type=\"member\"/>\n    <node name=\"10.10.32.2\" id=\"2\" online=\"true\" standby=\"false\" standby_onfail=\"false\" maintenance=\"false\" pending=\"false\" unclean=\"false\" health=\"green\" feature_set=\"3.19.0\" shutdown=\"false\" expected_up=\"true\" is_dc=\"true\" resources_running=\"5\" type=\"member\"/>\n    <node name=\"10.10.32.3\" id=\"3\" online=\"true\" standby=\"false\" standby_onfail=\"false\" maintenance=\"false\" pending=\"false\" unclean=\"false\" health=\"green\" feature_set=\"3.19.0\" shutdown=\"false\" expected_up=\"true\" is_dc=\"false\" resources_running=\"4\" type=\"member\"/>\n  </nodes>\n  <resources>\n    <resource id=\"cloudcenter_res\" resource_agent=\"ocf:heartbeat:VirtualDomain\" role=\"Started\" active=\"true\" orphaned=\"false\" blocked=\"false\" maintenance=\"false\" managed=\"true\" failed=\"false\" failure_ignored=\"false\" nodes_running_on=\"1\">\n      <node name=\"10.10.32.1\" id=\"1\" cached=\"true\"/>\n    </resource>\n    <resource id=\"fence-ablecube1\" resource_agent=\"stonith:fence_ipmilan\" role=\"Started\" active=\"true\" orphaned=\"false\" blocked=\"false\" maintenance=\"false\" managed=\"true\" failed=\"false\" failure_ignored=\"false\" nodes_running_on=\"1\">\n      <node name=\"10.10.32.2\" id=\"2\" cached=\"true\"/>\n    </resource>\n    <resource id=\"fence-ablecube2\" resource_agent=\"stonith:fence_ipmilan\" role=\"Started\" active=\"true\" orphaned=\"false\" blocked=\"false\" maintenance=\"false\" managed=\"true\" failed=\"false\" failure_ignored=\"false\" nodes_running_on=\"1\">\n      <node name=\"10.10.32.1\" id=\"1\" cached=\"true\"/>\n    </resource>\n    <resource id=\"fence-ablecube3\" resource_agent=\"stonith:fence_ipmilan\" role=\"Started\" active=\"true\" orphaned=\"false\" blocked=\"false\" maintenance=\"false\" managed=\"true\" failed=\"false\" failure_ignored=\"false\" nodes_running_on=\"1\">\n      <node name=\"10.10.32.1\" id=\"1\" cached=\"true\"/>\n    </resource>\n    <clone id=\"glue-locking-clone\" multi_state=\"false\" unique=\"false\" maintenance=\"false\" managed=\"true\" disabled=\"false\" failed=\"false\" failure_ignored=\"false\">\n      <group id=\"glue-locking:0\" number_resources=\"2\" maintenance=\"false\" managed=\"true\" disabled=\"false\">\n        <resource id=\"glue-dlm\" resource_agent=\"ocf:PCS:controld\" role=\"Started\" active=\"true\" orphaned=\"false\" blocked=\"false\" maintenance=\"false\" managed=\"true\" failed=\"false\" failure_ignored=\"false\" nodes_running_on=\"1\">\n          <node name=\"10.10.32.2\" id=\"2\" cached=\"true\"/>\n        </resource>\n        <resource id=\"glue-lvmlockd\" resource_agent=\"ocf:heartbeat:lvmlockd\" role=\"Started\" active=\"true\" orphaned=\"false\" blocked=\"false\" maintenance=\"false\" managed=\"true\" failed=\"false\" failure_ignored=\"false\" nodes_running_on=\"1\">\n          <node name=\"10.10.32.2\" id=\"2\" cached=\"true\"/>\n        </resource>\n      </group>\n      <group id=\"glue-locking:1\" number_resources=\"2\" maintenance=\"false\" managed=\"true\" disabled=\"false\">\n        <resource id=\"glue-dlm\" resource_agent=\"ocf:PCS:controld\" role=\"Started\" active=\"true\" orphaned=\"false\" blocked=\"false\" maintenance=\"false\" managed=\"true\" failed=\"false\" failure_ignored=\"false\" nodes_running_on=\"1\">\n          <node name=\"10.10.32.3\" id=\"3\" cached=\"true\"/>\n        </resource>\n        <resource id=\"glue-lvmlockd\" resource_agent=\"ocf:heartbeat:lvmlockd\" role=\"Started\" active=\"true\" orphaned=\"false\" blocked=\"false\" maintenance=\"false\" managed=\"true\" failed=\"false\" failure_ignored=\"false\" nodes_running_on=\"1\">\n          <node name=\"10.10.32.3\" id=\"3\" cached=\"true\"/>\n        </resource>\n      </group>\n      <group id=\"glue-locking:2\" number_resources=\"2\" maintenance=\"false\" managed=\"true\" disabled=\"false\">\n        <resource id=\"glue-dlm\" resource_agent=\"ocf:PCS:controld\" role=\"Started\" active=\"true\" orphaned=\"false\" blocked=\"false\" maintenance=\"false\" managed=\"true\" failed=\"false\" failure_ignored=\"false\" nodes_running_on=\"1\">\n          <node name=\"10.10.32.1\" id=\"1\" cached=\"true\"/>\n        </resource>\n        <resource id=\"glue-lvmlockd\" resource_agent=\"ocf:heartbeat:lvmlockd\" role=\"Started\" active=\"true\" orphaned=\"false\" blocked=\"false\" maintenance=\"false\" managed=\"true\" failed=\"false\" failure_ignored=\"false\" nodes_running_on=\"1\">\n          <node name=\"10.10.32.1\" id=\"1\" cached=\"true\"/>\n        </resource>\n      </group>\n    </clone>\n    <clone id=\"glue-gfs_res-clone\" multi_state=\"false\" unique=\"false\" maintenance=\"false\" managed=\"true\" disabled=\"false\" failed=\"false\" failure_ignored=\"false\">\n      <resource id=\"glue-gfs_res\" resource_agent=\"ocf:heartbeat:LVM-activate\" role=\"Started\" active=\"true\" orphaned=\"false\" blocked=\"false\" maintenance=\"false\" managed=\"true\" failed=\"false\" failure_ignored=\"false\" nodes_running_on=\"1\">\n        <node name=\"10.10.32.2\" id=\"2\" cached=\"true\"/>\n      </resource>\n      <resource id=\"glue-gfs_res\" resource_agent=\"ocf:heartbeat:LVM-activate\" role=\"Started\" active=\"true\" orphaned=\"false\" blocked=\"false\" maintenance=\"false\" managed=\"true\" failed=\"false\" failure_ignored=\"false\" nodes_running_on=\"1\">\n        <node name=\"10.10.32.3\" id=\"3\" cached=\"true\"/>\n      </resource>\n      <resource id=\"glue-gfs_res\" resource_agent=\"ocf:heartbeat:LVM-activate\" role=\"Started\" active=\"true\" orphaned=\"false\" blocked=\"false\" maintenance=\"false\" managed=\"true\" failed=\"false\" failure_ignored=\"false\" nodes_running_on=\"1\">\n        <node name=\"10.10.32.1\" id=\"1\" cached=\"true\"/>\n      </resource>\n    </clone>\n    <clone id=\"glue-gfs-clone\" multi_state=\"false\" unique=\"false\" maintenance=\"false\" managed=\"true\" disabled=\"false\" failed=\"false\" failure_ignored=\"false\">\n      <resource id=\"glue-gfs\" resource_agent=\"ocf:heartbeat:Filesystem\" role=\"Started\" active=\"true\" orphaned=\"false\" blocked=\"false\" maintenance=\"false\" managed=\"true\" failed=\"false\" failure_ignored=\"false\" nodes_running_on=\"1\">\n        <node name=\"10.10.32.2\" id=\"2\" cached=\"true\"/>\n      </resource>\n      <resource id=\"glue-gfs\" resource_agent=\"ocf:heartbeat:Filesystem\" role=\"Started\" active=\"true\" orphaned=\"false\" blocked=\"false\" maintenance=\"false\" managed=\"true\" failed=\"false\" failure_ignored=\"false\" nodes_running_on=\"1\">\n        <node name=\"10.10.32.3\" id=\"3\" cached=\"true\"/>\n      </resource>\n      <resource id=\"glue-gfs\" resource_agent=\"ocf:heartbeat:Filesystem\" role=\"Started\" active=\"true\" orphaned=\"false\" blocked=\"false\" maintenance=\"false\" managed=\"true\" failed=\"false\" failure_ignored=\"false\" nodes_running_on=\"1\">\n        <node name=\"10.10.32.1\" id=\"1\" cached=\"true\"/>\n      </resource>\n    </clone>\n  </resources>\n  <node_history>\n    <node name=\"10.10.32.2\">\n      <resource_history id=\"glue-gfs_res\" orphan=\"false\" migration-threshold=\"1000000\">\n        <operation_history call=\"34\" task=\"start\" rc=\"0\" rc_text=\"ok\" last-rc-change=\"Wed Jan  8 16:19:42 2025\" exec-time=\"3685ms\" queue-time=\"0ms\"/>\n        <operation_history call=\"35\" task=\"monitor\" rc=\"0\" rc_text=\"ok\" interval=\"30000ms\" last-rc-change=\"Wed Jan  8 16:19:47 2025\" exec-time=\"10ms\" queue-time=\"0ms\"/>\n      </resource_history>\n      <resource_history id=\"glue-lvmlockd\" orphan=\"false\" migration-threshold=\"1000000\">\n        <operation_history call=\"28\" task=\"start\" rc=\"0\" rc_text=\"ok\" last-rc-change=\"Wed Jan  8 16:19:23 2025\" exec-time=\"76ms\" queue-time=\"0ms\"/>\n        <operation_history call=\"29\" task=\"monitor\" rc=\"0\" rc_text=\"ok\" interval=\"45000ms\" last-rc-change=\"Wed Jan  8 16:19:23 2025\" exec-time=\"20ms\" queue-time=\"0ms\"/>\n      </resource_history>\n      <resource_history id=\"glue-dlm\" orphan=\"false\" migration-threshold=\"1000000\">\n        <operation_history call=\"21\" task=\"start\" rc=\"0\" rc_text=\"ok\" last-rc-change=\"Wed Jan  8 16:19:22 2025\" exec-time=\"1083ms\" queue-time=\"0ms\"/>\n        <operation_history call=\"27\" task=\"monitor\" rc=\"0\" rc_text=\"ok\" interval=\"45000ms\" last-rc-change=\"Wed Jan  8 16:19:23 2025\" exec-time=\"15ms\" queue-time=\"0ms\"/>\n      </resource_history>\n      <resource_history id=\"fence-ablecube1\" orphan=\"false\" migration-threshold=\"1000000\">\n        <operation_history call=\"25\" task=\"start\" rc=\"0\" rc_text=\"ok\" last-rc-change=\"Wed Jan  8 16:19:22 2025\" exec-time=\"289ms\" queue-time=\"0ms\"/>\n        <operation_history call=\"26\" task=\"monitor\" rc=\"0\" rc_text=\"ok\" interval=\"60000ms\" last-rc-change=\"Wed Jan  8 16:19:22 2025\" exec-time=\"639ms\" queue-time=\"0ms\"/>\n      </resource_history>\n      <resource_history id=\"glue-gfs\" orphan=\"false\" migration-threshold=\"1000000\">\n        <operation_history call=\"41\" task=\"start\" rc=\"0\" rc_text=\"ok\" last-rc-change=\"Wed Jan  8 16:19:47 2025\" exec-time=\"1706ms\" queue-time=\"0ms\"/>\n        <operation_history call=\"42\" task=\"monitor\" rc=\"0\" rc_text=\"ok\" interval=\"10000ms\" last-rc-change=\"Wed Jan  8 16:19:49 2025\" exec-time=\"37ms\" queue-time=\"0ms\"/>\n      </resource_history>\n    </node>\n    <node name=\"10.10.32.3\">\n      <resource_history id=\"glue-gfs_res\" orphan=\"false\" migration-threshold=\"1000000\">\n        <operation_history call=\"32\" task=\"start\" rc=\"0\" rc_text=\"ok\" last-rc-change=\"Wed Jan  8 16:19:42 2025\" exec-time=\"3759ms\" queue-time=\"0ms\"/>\n        <operation_history call=\"33\" task=\"monitor\" rc=\"0\" rc_text=\"ok\" interval=\"30000ms\" last-rc-change=\"Wed Jan  8 16:19:47 2025\" exec-time=\"9ms\" queue-time=\"0ms\"/>\n      </resource_history>\n      <resource_history id=\"glue-lvmlockd\" orphan=\"false\" migration-threshold=\"1000000\">\n        <operation_history call=\"26\" task=\"start\" rc=\"0\" rc_text=\"ok\" last-rc-change=\"Wed Jan  8 16:19:23 2025\" exec-time=\"75ms\" queue-time=\"0ms\"/>\n        <operation_history call=\"27\" task=\"monitor\" rc=\"0\" rc_text=\"ok\" interval=\"45000ms\" last-rc-change=\"Wed Jan  8 16:19:23 2025\" exec-time=\"20ms\" queue-time=\"0ms\"/>\n      </resource_history>\n      <resource_history id=\"glue-dlm\" orphan=\"false\" migration-threshold=\"1000000\">\n        <operation_history call=\"21\" task=\"start\" rc=\"0\" rc_text=\"ok\" last-rc-change=\"Wed Jan  8 16:19:22 2025\" exec-time=\"1084ms\" queue-time=\"0ms\"/>\n        <operation_history call=\"25\" task=\"monitor\" rc=\"0\" rc_text=\"ok\" interval=\"45000ms\" last-rc-change=\"Wed Jan  8 16:19:23 2025\" exec-time=\"16ms\" queue-time=\"0ms\"/>\n      </resource_history>\n      <resource_history id=\"glue-gfs\" orphan=\"false\" migration-threshold=\"1000000\">\n        <operation_history call=\"39\" task=\"start\" rc=\"0\" rc_text=\"ok\" last-rc-change=\"Wed Jan  8 16:19:47 2025\" exec-time=\"1866ms\" queue-time=\"0ms\"/>\n        <operation_history call=\"40\" task=\"monitor\" rc=\"0\" rc_text=\"ok\" interval=\"10000ms\" last-rc-change=\"Wed Jan  8 16:19:49 2025\" exec-time=\"36ms\" queue-time=\"0ms\"/>\n      </resource_history>\n    </node>\n    <node name=\"10.10.32.1\">\n      <resource_history id=\"fence-ablecube3\" orphan=\"false\" migration-threshold=\"1000000\">\n        <operation_history call=\"26\" task=\"start\" rc=\"0\" rc_text=\"ok\" last-rc-change=\"Wed Jan  8 16:19:22 2025\" exec-time=\"293ms\" queue-time=\"0ms\"/>\n        <operation_history call=\"28\" task=\"monitor\" rc=\"0\" rc_text=\"ok\" interval=\"60000ms\" last-rc-change=\"Wed Jan  8 16:19:22 2025\" exec-time=\"650ms\" queue-time=\"0ms\"/>\n      </resource_history>\n      <resource_history id=\"glue-gfs_res\" orphan=\"false\" migration-threshold=\"1000000\">\n        <operation_history call=\"35\" task=\"probe\" rc=\"0\" rc_text=\"ok\" last-rc-change=\"Wed Jan  8 16:19:41 2025\" exec-time=\"9ms\" queue-time=\"0ms\"/>\n        <operation_history call=\"35\" task=\"probe\" rc=\"0\" rc_text=\"ok\" last-rc-change=\"Wed Jan  8 16:19:41 2025\" exec-time=\"9ms\" queue-time=\"0ms\"/>\n        <operation_history call=\"36\" task=\"monitor\" rc=\"0\" rc_text=\"ok\" interval=\"30000ms\" last-rc-change=\"Wed Jan  8 16:19:41 2025\" exec-time=\"8ms\" queue-time=\"0ms\"/>\n      </resource_history>\n      <resource_history id=\"glue-lvmlockd\" orphan=\"false\" migration-threshold=\"1000000\">\n        <operation_history call=\"30\" task=\"start\" rc=\"0\" rc_text=\"ok\" last-rc-change=\"Wed Jan  8 16:19:23 2025\" exec-time=\"72ms\" queue-time=\"0ms\"/>\n        <operation_history call=\"31\" task=\"monitor\" rc=\"0\" rc_text=\"ok\" interval=\"45000ms\" last-rc-change=\"Wed Jan  8 16:19:23 2025\" exec-time=\"20ms\" queue-time=\"0ms\"/>\n      </resource_history>\n      <resource_history id=\"fence-ablecube2\" orphan=\"false\" migration-threshold=\"1000000\">\n        <operation_history call=\"25\" task=\"start\" rc=\"0\" rc_text=\"ok\" last-rc-change=\"Wed Jan  8 16:19:22 2025\" exec-time=\"281ms\" queue-time=\"0ms\"/>\n        <operation_history call=\"27\" task=\"monitor\" rc=\"0\" rc_text=\"ok\" interval=\"60000ms\" last-rc-change=\"Wed Jan  8 16:19:22 2025\" exec-time=\"628ms\" queue-time=\"0ms\"/>\n      </resource_history>\n      <resource_history id=\"glue-dlm\" orphan=\"false\" migration-threshold=\"1000000\">\n        <operation_history call=\"21\" task=\"start\" rc=\"0\" rc_text=\"ok\" last-rc-change=\"Wed Jan  8 16:19:22 2025\" exec-time=\"1086ms\" queue-time=\"0ms\"/>\n        <operation_history call=\"29\" task=\"monitor\" rc=\"0\" rc_text=\"ok\" interval=\"45000ms\" last-rc-change=\"Wed Jan  8 16:19:23 2025\" exec-time=\"21ms\" queue-time=\"0ms\"/>\n      </resource_history>\n      <resource_history id=\"glue-gfs\" orphan=\"false\" migration-threshold=\"1000000\">\n        <operation_history call=\"42\" task=\"start\" rc=\"0\" rc_text=\"ok\" last-rc-change=\"Wed Jan  8 16:19:47 2025\" exec-time=\"1366ms\" queue-time=\"0ms\"/>\n        <operation_history call=\"43\" task=\"monitor\" rc=\"0\" rc_text=\"ok\" interval=\"10000ms\" last-rc-change=\"Wed Jan  8 16:19:48 2025\" exec-time=\"35ms\" queue-time=\"0ms\"/>\n      </resource_history>\n      <resource_history id=\"cloudcenter_res\" orphan=\"false\" migration-threshold=\"1000000\">\n        <operation_history call=\"48\" task=\"start\" rc=\"0\" rc_text=\"ok\" last-rc-change=\"Wed Jan  8 16:20:24 2025\" exec-time=\"2335ms\" queue-time=\"0ms\"/>\n        <operation_history call=\"49\" task=\"monitor\" rc=\"0\" rc_text=\"ok\" interval=\"10000ms\" last-rc-change=\"Wed Jan  8 16:20:27 2025\" exec-time=\"119ms\" queue-time=\"0ms\"/>\n      </resource_history>\n    </node>\n  </node_history>\n  <bans>\n    <ban id=\"location-fence-ablecube3-10.10.32.3--INFINITY\" resource=\"fence-ablecube3\" node=\"10.10.32.3\" weight=\"-1000000\" promoted-only=\"false\" master_only=\"false\"/>\n    <ban id=\"location-fence-ablecube2-10.10.32.2--INFINITY\" resource=\"fence-ablecube2\" node=\"10.10.32.2\" weight=\"-1000000\" promoted-only=\"false\" master_only=\"false\"/>\n    <ban id=\"location-fence-ablecube1-10.10.32.1--INFINITY\" resource=\"fence-ablecube1\" node=\"10.10.32.1\" weight=\"-1000000\" promoted-only=\"false\" master_only=\"false\"/>\n  </bans>\n  <status code=\"0\" message=\"OK\"/>\n</PCS-result>")

		if err := xml.Unmarshal(stdout, &_PCSResult); err != nil {
			utils.FancyHandleError(err)

		}
	}
	_PCSResult.RefreshTime = time.Now()
	return _PCSResult
}

func GetClone() []TypePCSClone {
	return _PCSResult.Resources.Clone
}

func GetResource() TypePCSResources {
	ret := []TypePCSResource{}

	ret = append(ret, _PCSResult.Resources.Resource...)

	return TypePCSResources{PCSResources: ret}
}
