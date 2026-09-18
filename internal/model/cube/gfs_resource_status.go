package cube

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net"
	"sort"
	"strings"
)

// GFSResourceStatusResponse는 pcs status xml 기반 GFS 리소스 상태 조회 결과이다.
// @name GFSResourceStatusResponse
type GFSResourceStatusResponse struct {
	Code int `json:"code" example:"200"`
	Val  any `json:"val,omitempty"`
}

// GFSResourceStatusValue는 기존 Python createReturn val 구조와 호환되는 값이다.
// @name GFSResourceStatusValue
type GFSResourceStatusValue struct {
	NodesInfo   []map[string]string        `json:"nodes_info"`
	Resources   GFSResourceStatusGroups    `json:"resources"`
	NodeHistory []GFSNodeResourceHistories `json:"node_history"`
}

// GFSResourceStatusGroups는 GFS 관련 PCS 리소스를 종류별로 묶은 값이다.
// @name GFSResourceStatusGroups
type GFSResourceStatusGroups struct {
	FenceResources       []map[string]string `json:"fence_resources"`
	GlueLockingResources []map[string]string `json:"glue_locking_resources"`
	GFSMountResources    []map[string]string `json:"gfs_mount_resources"`
	// GlueGFSResources는 기존 클라이언트 호환을 위한 alias이다.
	GlueGFSResources []map[string]string `json:"glue_gfs_resources"`
}

// GFSNodeResourceHistories는 특정 노드의 리소스 operation history 목록이다.
// @name GFSNodeResourceHistories
type GFSNodeResourceHistories struct {
	NodeName          string               `json:"node_name"`
	ResourceHistories []GFSResourceHistory `json:"resource_histories"`
}

// GFSResourceHistory는 특정 리소스의 operation history 목록이다.
// @name GFSResourceHistory
type GFSResourceHistory struct {
	ResourceID string              `json:"resource_id"`
	Operations []map[string]string `json:"operations"`
}

type gfsPCSStatusXML struct {
	Nodes struct {
		Node []gfsPCSNodeXML `xml:"node"`
	} `xml:"nodes"`
	Resources struct {
		Resource []gfsPCSResourceXML `xml:"resource"`
		Clone    []gfsPCSCloneXML    `xml:"clone"`
	} `xml:"resources"`
	NodeHistory struct {
		Node []gfsPCSNodeHistoryXML `xml:"node"`
	} `xml:"node_history"`
	Bans struct {
		Ban []gfsPCSBanXML `xml:"ban"`
	} `xml:"bans"`
}

type gfsPCSNodeXML struct {
	Attrs []xml.Attr `xml:",any,attr"`
}

type gfsPCSResourceXML struct {
	Attrs []xml.Attr      `xml:",any,attr"`
	Node  []gfsPCSNodeXML `xml:"node"`
}

type gfsPCSCloneXML struct {
	Resource []gfsPCSResourceXML      `xml:"resource"`
	Group    []gfsPCSResourceGroupXML `xml:"group"`
}

type gfsPCSResourceGroupXML struct {
	Resource []gfsPCSResourceXML `xml:"resource"`
}

type gfsPCSNodeHistoryXML struct {
	Attrs           []xml.Attr                 `xml:",any,attr"`
	ResourceHistory []gfsPCSResourceHistoryXML `xml:"resource_history"`
}

type gfsPCSResourceHistoryXML struct {
	Attrs            []xml.Attr                  `xml:",any,attr"`
	OperationHistory []gfsPCSOperationHistoryXML `xml:"operation_history"`
}

type gfsPCSOperationHistoryXML struct {
	Attrs []xml.Attr `xml:",any,attr"`
}

type gfsPCSBanXML struct {
	Attrs []xml.Attr `xml:",any,attr"`
}

// ParseGFSResourceStatusXML은 pcs status xml 출력에서 GFS 리소스 상태 값을 만든다.
func ParseGFSResourceStatusXML(data []byte) (GFSResourceStatusValue, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return GFSResourceStatusValue{}, fmt.Errorf("empty pcs status xml")
	}

	var status gfsPCSStatusXML
	if err := xml.Unmarshal(data, &status); err != nil {
		return GFSResourceStatusValue{}, fmt.Errorf("failed to parse pcs status xml: %w", err)
	}

	nodesInfo := make([]map[string]string, 0, len(status.Nodes.Node))
	for _, node := range status.Nodes.Node {
		nodesInfo = append(nodesInfo, xmlAttrsToMap(node.Attrs))
	}

	resources := GFSResourceStatusGroups{
		FenceResources:       []map[string]string{},
		GlueLockingResources: []map[string]string{},
		GFSMountResources:    []map[string]string{},
		GlueGFSResources:     []map[string]string{},
	}

	fenceNodeByResource := collectGFSFenceNodeByResource(status.Bans.Ban)
	nodeIndex := 0
	for _, resource := range collectGFSPCSResources(status.Resources.Resource, status.Resources.Clone) {
		attrs := xmlAttrsToMap(resource.Attrs)
		resourceID := strings.TrimSpace(attrs["id"])
		if resourceID == "" {
			continue
		}

		if strings.HasPrefix(resourceID, "fence-") {
			item := copyStringMap(attrs)
			nodeName := fenceNodeByResource[resourceID]
			if nodeName == "" && len(nodesInfo) > 0 {
				nodeName = nodesInfo[nodeIndex%len(nodesInfo)]["name"]
			}
			if len(nodesInfo) > 0 {
				nodeIndex++
			}
			if nodeName != "" {
				item["node_name"] = nodeName
			}
			resources.FenceResources = append(resources.FenceResources, item)
			continue
		}

		if isGFSGlueLockingResource(resourceID) {
			resources.GlueLockingResources = appendResourceForRunningNodes(resources.GlueLockingResources, attrs, resource.Node)
			continue
		}

		if isGFSMountResource(resourceID) {
			resources.GFSMountResources = appendResourceForRunningNodes(resources.GFSMountResources, attrs, resource.Node)
		}
	}
	resources.GlueGFSResources = copyResourceMaps(resources.GFSMountResources)

	nodeHistory := buildGFSNodeHistory(status.NodeHistory.Node)
	sort.SliceStable(nodeHistory, func(i, j int) bool {
		return compareGFSNodeName(nodeHistory[i].NodeName, nodeHistory[j].NodeName) < 0
	})

	return GFSResourceStatusValue{
		NodesInfo:   nodesInfo,
		Resources:   resources,
		NodeHistory: nodeHistory,
	}, nil
}

func collectGFSPCSResources(resources []gfsPCSResourceXML, clones []gfsPCSCloneXML) []gfsPCSResourceXML {
	out := make([]gfsPCSResourceXML, 0, len(resources))
	out = append(out, resources...)
	for _, clone := range clones {
		out = append(out, clone.Resource...)
		for _, group := range clone.Group {
			out = append(out, group.Resource...)
		}
	}
	return out
}

func collectGFSFenceNodeByResource(bans []gfsPCSBanXML) map[string]string {
	out := map[string]string{}
	for _, ban := range bans {
		attrs := xmlAttrsToMap(ban.Attrs)
		resource := strings.TrimSpace(attrs["resource"])
		node := strings.TrimSpace(attrs["node"])
		if resource == "" || node == "" || !strings.HasPrefix(resource, "fence-") {
			continue
		}
		if _, exists := out[resource]; !exists {
			out[resource] = node
		}
	}
	return out
}

func isGFSGlueLockingResource(resourceID string) bool {
	return resourceID == "glue-dlm" || resourceID == "glue-lvmlockd"
}

func isGFSMountResource(resourceID string) bool {
	return resourceID == "glue-gfs" || resourceID == "glue-gfs_res"
}

func copyResourceMaps(resources []map[string]string) []map[string]string {
	out := make([]map[string]string, 0, len(resources))
	for _, resource := range resources {
		out = append(out, copyStringMap(resource))
	}
	return out
}

func appendResourceForRunningNodes(out []map[string]string, attrs map[string]string, nodes []gfsPCSNodeXML) []map[string]string {
	if len(nodes) == 0 {
		return append(out, copyStringMap(attrs))
	}

	for _, node := range nodes {
		item := copyStringMap(attrs)
		nodeAttrs := xmlAttrsToMap(node.Attrs)
		if nodeName := strings.TrimSpace(nodeAttrs["name"]); nodeName != "" {
			item["node_name"] = nodeName
		}
		out = append(out, item)
	}
	return out
}

func buildGFSNodeHistory(nodes []gfsPCSNodeHistoryXML) []GFSNodeResourceHistories {
	out := make([]GFSNodeResourceHistories, 0, len(nodes))
	for _, node := range nodes {
		nodeAttrs := xmlAttrsToMap(node.Attrs)
		resourceHistories := make([]GFSResourceHistory, 0, len(node.ResourceHistory))

		for _, resource := range node.ResourceHistory {
			resourceAttrs := xmlAttrsToMap(resource.Attrs)
			operations := make([]map[string]string, 0, len(resource.OperationHistory))
			seenCalls := map[string]struct{}{}

			for _, operation := range resource.OperationHistory {
				operationAttrs := xmlAttrsToMap(operation.Attrs)
				if operationAttrs["task"] == "probe" {
					continue
				}
				call := operationAttrs["call"]
				if _, exists := seenCalls[call]; exists {
					continue
				}
				seenCalls[call] = struct{}{}
				operations = append(operations, operationAttrs)
			}

			resourceHistories = append(resourceHistories, GFSResourceHistory{
				ResourceID: strings.TrimSpace(resourceAttrs["id"]),
				Operations: operations,
			})
		}

		out = append(out, GFSNodeResourceHistories{
			NodeName:          strings.TrimSpace(nodeAttrs["name"]),
			ResourceHistories: resourceHistories,
		})
	}
	return out
}

func xmlAttrsToMap(attrs []xml.Attr) map[string]string {
	out := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		out[attr.Name.Local] = attr.Value
	}
	return out
}

func copyStringMap(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func compareGFSNodeName(left, right string) int {
	leftIP := net.ParseIP(strings.TrimSpace(left)).To4()
	rightIP := net.ParseIP(strings.TrimSpace(right)).To4()

	switch {
	case leftIP != nil && rightIP != nil:
		return bytes.Compare(leftIP, rightIP)
	case leftIP != nil:
		return -1
	case rightIP != nil:
		return 1
	default:
		return strings.Compare(left, right)
	}
}
