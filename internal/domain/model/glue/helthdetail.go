package glue

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/goccy/go-json"
	"github.com/ycyun/Cube-API/internal/infra/utils"
	"os/exec"
	"reflect"
	"sync"
	"time"
)

type TypeGlueHealthDetailChecks struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Summary  struct {
		Message string `json:"message"`
		Count   int    `json:"count"`
	} `json:"summary"`
	Detail []string `json:"detail"`
	Muted  bool     `json:"muted"`
}
type TypeGlueHealthDetailChecksRaw struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Summary  struct {
		Message string `json:"message"`
		Count   int    `json:"count"`
	} `json:"summary"`
	Detail []struct {
		Message string `json:"message"`
	} `json:"detail"`
	Muted bool `json:"muted"`
}

type TypeGlueHealthDetailMutes struct {
	Code    string `json:"code"`
	Sticky  bool   `json:"sticky"`
	Summary string `json:"summary"`
	Count   int    `json:"count"`
}

// TypeGlueHealthDetail model info
// @Description Glue의 health를 나타내는 구조체
type TypeGlueHealthDetail struct {
	Status      string                       `json:"status"`
	Checks      []TypeGlueHealthDetailChecks `json:"checks"`
	Mutes       []TypeGlueHealthDetailMutes  `json:"mutes"`
	RefreshTime time.Time                    `json:"refresh_time"`
}

func (d TypeGlueHealthDetail) GetMessages() []string {
	var messages []string
	for _, check := range d.Checks {
		messages = append(messages, check.Detail...)
	}
	return messages //strings.Join(messages, "\r\n")
} // @name TypeGlueHealthDetail

var lockGlueHealthDetail sync.Once

var _glueHealthDetail *TypeGlueHealthDetail

func Health() *TypeGlueHealthDetail {
	if _glueHealthDetail == nil {
		lockGlueHealthDetail.Do(
			func() {
				if gin.IsDebugging() {
					fmt.Println("Creating ", reflect.TypeOf(_glueStorageSize), " now.")
				}
				_glueHealthDetail = &TypeGlueHealthDetail{}
			})
	} else {
		if gin.IsDebugging() {
			fmt.Println("get old ", reflect.TypeOf(_glueStorageSize), " instance.")
		}
	}
	return _glueHealthDetail
}

func UpdateHealth() *TypeGlueHealthDetail {
	var _tmpGlueHealthDetail map[string]interface{}
	if gin.Mode() == gin.ReleaseMode {
		var stdout []byte

		cmd := exec.Command("ceph", "health", "detail", "-f", "json")
		stdout, _ = cmd.CombinedOutput()

		if err := json.Unmarshal(stdout, &_tmpGlueHealthDetail); err != nil && gin.IsDebugging() {
			utils.FancyHandleError(err)

		}
	} else {

		stdout := []byte("{\n  \"status\": \"HEALTH_WARN\",\n  \"checks\": {\n    \"OSDMAP_FLAGS\": {\n      \"severity\": \"HEALTH_WARN\",\n      \"summary\": {\"message\": \"noup flag(s) set\", \"count\": 4},\n      \"detail\": [],\n      \"muted\": false\n    },\n    \"OSD_DOWN\": {\n      \"severity\": \"HEALTH_WARN\",\n      \"summary\": {\"message\": \"1 osds down\", \"count\": 1},\n      \"detail\": [ {\"message\": \"osd.0 (root=default,host=scvm1) is down\"} ],\n      \"muted\": false\n    },\n    \"PG_DEGRADED\": {\n      \"severity\": \"HEALTH_WARN\",\n      \"summary\": {\n        \"message\": \"Degraded data redundancy: 81238/1570122 objects degraded (5.174%), 138 pgs degraded\",\n        \"count\": 138\n      },\n      \"detail\": [\n        {\"message\": \"pg 2.74 is active+undersized+degraded+wait, acting [5]\"},\n        {\"message\": \"pg 2.103 is active+undersized+degraded+wait, acting [1]\"},\n        {\"message\": \"pg 2.10f is active+undersized+degraded+wait, acting [5]\"},\n        {\n          \"message\": \"pg 2.119 is active+undersized+degraded+wait, acting [16]\"\n        },\n        {\n          \"message\": \"pg 2.127 is active+undersized+degraded+wait, acting [11]\"\n        },\n        {\"message\": \"pg 2.12e is active+undersized+degraded+wait, acting [1]\"},\n        {\"message\": \"pg 2.132 is active+undersized+degraded+wait, acting [2]\"},\n        {\"message\": \"pg 2.139 is active+undersized+degraded+wait, acting [2]\"},\n        {\n          \"message\": \"pg 2.13c is active+undersized+degraded+wait, acting [18]\"\n        },\n        {\n          \"message\": \"pg 2.14a is active+undersized+degraded+wait, acting [14]\"\n        },\n        {\"message\": \"pg 2.151 is active+undersized+degraded+wait, acting [5]\"},\n        {\n          \"message\": \"pg 2.167 is active+undersized+degraded+wait, acting [16]\"\n        },\n        {\"message\": \"pg 2.170 is active+undersized+degraded+wait, acting [8]\"},\n        {\"message\": \"pg 2.1af is active+undersized+degraded+wait, acting [5]\"},\n        {\n          \"message\": \"pg 2.1b7 is active+undersized+degraded+wait, acting [13]\"\n        },\n        {\"message\": \"pg 2.1b9 is active+undersized+degraded+wait, acting [2]\"},\n        {\n          \"message\": \"pg 2.1c7 is active+undersized+degraded+wait, acting [17]\"\n        },\n        {\n          \"message\": \"pg 2.1cd is active+undersized+degraded+wait, acting [17]\"\n        },\n        {\"message\": \"pg 2.1d9 is active+undersized+degraded+wait, acting [4]\"},\n        {\"message\": \"pg 2.1ef is active+undersized+degraded+wait, acting [7]\"},\n        {\n          \"message\": \"pg 2.1f1 is active+undersized+degraded+wait, acting [10]\"\n        },\n        {\n          \"message\": \"pg 2.1f7 is active+undersized+degraded+wait, acting [11]\"\n        },\n        {\"message\": \"pg 4.72 is active+undersized+degraded+wait, acting [16]\"},\n        {\"message\": \"pg 4.7a is active+undersized+degraded+wait, acting [11]\"},\n        {\"message\": \"pg 4.7f is active+undersized+degraded+wait, acting [7]\"},\n        {\"message\": \"pg 4.10a is active+undersized+degraded+wait, acting [2]\"},\n        {\"message\": \"pg 4.115 is active+undersized+degraded+wait, acting [7]\"},\n        {\n          \"message\": \"pg 4.120 is active+undersized+degraded+wait, acting [13]\"\n        },\n        {\n          \"message\": \"pg 4.125 is active+undersized+degraded+wait, acting [17]\"\n        },\n        {\"message\": \"pg 4.139 is active+undersized+degraded+wait, acting [8]\"},\n        {\n          \"message\": \"pg 4.13f is active+undersized+degraded+wait, acting [18]\"\n        },\n        {\n          \"message\": \"pg 4.145 is active+undersized+degraded+wait, acting [16]\"\n        },\n        {\"message\": \"pg 4.146 is active+undersized+degraded+wait, acting [8]\"},\n        {\n          \"message\": \"pg 4.148 is active+undersized+degraded+wait, acting [16]\"\n        },\n        {\n          \"message\": \"pg 4.155 is active+undersized+degraded+wait, acting [18]\"\n        },\n        {\"message\": \"pg 4.15c is active+undersized+degraded+wait, acting [4]\"},\n        {\"message\": \"pg 4.16c is active+undersized+degraded+wait, acting [5]\"},\n        {\n          \"message\": \"pg 4.190 is active+undersized+degraded+wait, acting [16]\"\n        },\n        {\"message\": \"pg 4.197 is active+undersized+degraded+wait, acting [1]\"},\n        {\n          \"message\": \"pg 4.19b is active+undersized+degraded+wait, acting [17]\"\n        },\n        {\"message\": \"pg 4.1a2 is active+undersized+degraded+wait, acting [8]\"},\n        {\n          \"message\": \"pg 4.1af is active+undersized+degraded+wait, acting [18]\"\n        },\n        {\"message\": \"pg 4.1b6 is active+undersized+degraded+wait, acting [8]\"},\n        {\n          \"message\": \"pg 4.1ba is active+undersized+degraded+wait, acting [16]\"\n        },\n        {\"message\": \"pg 4.1ce is active+undersized+degraded+wait, acting [7]\"},\n        {\"message\": \"pg 4.1d8 is active+undersized+degraded+wait, acting [2]\"},\n        {\n          \"message\": \"pg 4.1dc is active+undersized+degraded+wait, acting [17]\"\n        },\n        {\"message\": \"pg 4.1ed is active+undersized+degraded+wait, acting [1]\"},\n        {\"message\": \"pg 4.1f5 is active+undersized+degraded+wait, acting [1]\"},\n        {\n          \"message\": \"pg 4.1fc is active+undersized+degraded+wait, acting [11]\"\n        },\n        {\"message\": \"pg 11.7d is active+undersized+degraded+wait, acting [11]\"}\n      ],\n      \"muted\": false\n    }\n  },\n  \"mutes\": [\n    {\n      \"code\": \"OSDMAP_FLAGS\",\n      \"sticky\": false,\n      \"summary\": \"nodown flag(s) set\",\n      \"count\": 6\n    }\n  ]\n}\n")

		if err := json.Unmarshal(stdout, &_tmpGlueHealthDetail); err != nil {
			utils.FancyHandleError(err)

		}
	}
	Health()
	for key, value := range _tmpGlueHealthDetail {
		if key == "mutes" {
			var localMutes []TypeGlueHealthDetailMutes
			for _, item := range value.([]interface{}) {
				s := TypeGlueHealthDetailMutes{}
				marshal, err := json.MarshalIndent(item, "", "\t")
				if err != nil {
					return nil
				}
				err = json.Unmarshal(marshal, &s)
				if err != nil {
					return nil
				}
				//fmt.Println(name, ":\t", item, ":\t", s)
				localMutes = append(localMutes, s)
			}
			_glueHealthDetail.Mutes = localMutes
		} else if key == "status" {
			_glueHealthDetail.Status = value.(string)
		} else if key == "checks" {

			var localCheck []TypeGlueHealthDetailChecks
			for name, item := range value.(map[string]interface{}) {

				s := TypeGlueHealthDetailChecksRaw{}
				s2 := TypeGlueHealthDetailChecks{}
				marshal, err := json.MarshalIndent(item, "", "\t")
				if err != nil {
					return nil
				}
				err = json.Unmarshal(marshal, &s)
				if err != nil {
					return nil
				}
				s.Type = name

				s2.Type = s.Type
				s2.Severity = s.Severity
				s2.Summary = s.Summary
				s2.Muted = s.Muted
				for _, j := range s.Detail {

					s2.Detail = append(s2.Detail, j.Message)
				}
				localCheck = append(localCheck, s2)
			}
			_glueHealthDetail.Checks = localCheck
		}

	}
	return _glueHealthDetail
}
