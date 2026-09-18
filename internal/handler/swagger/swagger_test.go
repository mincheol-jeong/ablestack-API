package swagger

import (
	"encoding/json"
	"strings"
	"testing"

	"ablecloud.io/ablestack-api/docs"
)

func TestFilterDocForSCVMHidesGluePathsTagsAndDefinitions(t *testing.T) {
	raw := `{
		"paths": {
			"/glue": {"get": {}},
			"/glue/rgw": {"get": {}},
			"/mold": {
				"get": {
					"responses": {
						"200": {
							"schema": {
								"$ref": "#/definitions/ablecloud_io_ablestack-api_internal_model_mold.RootStatus"
							}
						}
					}
				}
			},
			"/cube/license": {
				"post": {
					"responses": {
						"200": {
							"schema": {
								"$ref": "#/definitions/ablecloud_io_ablestack-api_internal_model_cube.LicenseResponse"
							}
						}
					}
				}
			}
		},
		"tags": [
			{"name": "Glue-RGW"},
			{"name": "Mold"},
			{"name": "Cube-License"}
		],
		"definitions": {
			"ablecloud_io_ablestack-api_internal_model_glue.Response": {},
			"ablecloud_io_ablestack-api_internal_model_mold.RootStatus": {},
			"ablecloud_io_ablestack-api_internal_model_cube.LicenseResponse": {}
		}
	}`

	filtered, err := FilterDocForSCVM(raw, false)
	if err != nil {
		t.Fatalf("FilterDocForSCVM returned error: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(filtered, &doc); err != nil {
		t.Fatalf("filtered doc is not valid JSON: %v", err)
	}
	paths := doc["paths"].(map[string]any)
	if _, ok := paths["/glue"]; ok {
		t.Fatalf("filtered doc still contains /glue")
	}
	if _, ok := paths["/glue/rgw"]; ok {
		t.Fatalf("filtered doc still contains /glue/rgw")
	}
	if _, ok := paths["/mold"]; ok {
		t.Fatalf("filtered doc still contains /mold")
	}
	if _, ok := paths["/cube/license"]; !ok {
		t.Fatalf("filtered doc removed cube path")
	}

	tags := doc["tags"].([]any)
	if len(tags) != 1 || tags[0].(map[string]any)["name"] != "Cube-License" {
		t.Fatalf("tags = %#v, want only Cube-License", tags)
	}

	defs := doc["definitions"].(map[string]any)
	if _, ok := defs["ablecloud_io_ablestack-api_internal_model_glue.Response"]; ok {
		t.Fatalf("filtered doc still contains Glue definition")
	}
	if _, ok := defs["ablecloud_io_ablestack-api_internal_model_mold.RootStatus"]; ok {
		t.Fatalf("filtered doc still contains Mold definition")
	}
	if _, ok := defs["ablecloud_io_ablestack-api_internal_model_cube.LicenseResponse"]; !ok {
		t.Fatalf("filtered doc removed Cube definition")
	}
}

func TestFilterDocForNodeKeepsMoldForCCVM(t *testing.T) {
	raw := `{
		"paths": {
			"/glue/status": {"get": {}},
			"/mold/session": {
				"post": {
					"responses": {
						"200": {
							"schema": {
								"$ref": "#/definitions/ablecloud_io_ablestack-api_internal_model_mold.MoldSessionValue"
							}
						}
					}
				}
			},
			"/cube/license": {
				"post": {
					"responses": {
						"200": {
							"schema": {
								"$ref": "#/definitions/ablecloud_io_ablestack-api_internal_model_cube.LicenseResponse"
							}
						}
					}
				}
			}
		},
		"tags": [
			{"name": "Glue-Core"},
			{"name": "Mold"},
			{"name": "Cube-License"}
		],
		"definitions": {
			"ablecloud_io_ablestack-api_internal_model_glue.Response": {},
			"ablecloud_io_ablestack-api_internal_model_mold.MoldSessionValue": {},
			"ablecloud_io_ablestack-api_internal_model_cube.LicenseResponse": {}
		}
	}`

	filtered, err := FilterDocForNode(raw, false, true)
	if err != nil {
		t.Fatalf("FilterDocForNode returned error: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(filtered, &doc); err != nil {
		t.Fatalf("filtered doc is not valid JSON: %v", err)
	}
	paths := doc["paths"].(map[string]any)
	if _, ok := paths["/glue/status"]; ok {
		t.Fatalf("filtered CCVM doc still contains Glue path")
	}
	if _, ok := paths["/mold/session"]; !ok {
		t.Fatalf("filtered CCVM doc removed Mold path")
	}

	tags := doc["tags"].([]any)
	for _, rawTag := range tags {
		if rawTag.(map[string]any)["name"] == "Glue-Core" {
			t.Fatalf("filtered CCVM doc still contains Glue tag")
		}
	}

	defs := doc["definitions"].(map[string]any)
	if _, ok := defs["ablecloud_io_ablestack-api_internal_model_glue.Response"]; ok {
		t.Fatalf("filtered CCVM doc still contains Glue definition")
	}
	if _, ok := defs["ablecloud_io_ablestack-api_internal_model_mold.MoldSessionValue"]; !ok {
		t.Fatalf("filtered CCVM doc removed Mold definition")
	}
}

func TestFilterDocForSCVMShowsGlueAndMinimumAPIsOnly(t *testing.T) {
	raw := `{
		"paths": {
			"/auth/login": {"post": {}},
			"/health": {"get": {}},
			"/version": {"get": {}},
			"/cube/cluster/apply-local": {"post": {}},
			"/cube/license": {
				"post": {
					"responses": {
						"200": {
							"schema": {
								"$ref": "#/definitions/ablecloud_io_ablestack-api_internal_model_cube.LicenseResponse"
							}
						}
					}
				}
			},
			"/cube/license/apply": {"post": {}},
			"/cube/system/config": {"post": {}},
			"/mold/session": {
				"post": {
					"responses": {
						"200": {
							"schema": {
								"$ref": "#/definitions/ablecloud_io_ablestack-api_internal_model_mold.MoldSessionValue"
							}
						}
					}
				}
			},
			"/cube/nics": {
				"get": {
					"responses": {
						"200": {
							"schema": {
								"$ref": "#/definitions/ablecloud_io_ablestack-api_internal_model_cube.NICResponse"
							}
						}
					}
				}
			},
			"/glue/status": {
				"get": {
					"responses": {
						"200": {
							"schema": {
								"$ref": "#/definitions/ablecloud_io_ablestack-api_internal_model_glue.Response"
							}
						}
					}
				}
			}
		},
		"tags": [
			{"name": "Auth"},
			{"name": "Health"},
			{"name": "Cube-Cluster"},
			{"name": "Cube-Version"},
			{"name": "Cube-License"},
			{"name": "Cube-System"},
			{"name": "Cube-Nic"},
			{"name": "Mold"},
			{"name": "Glue-Core"}
		],
		"definitions": {
			"ablecloud_io_ablestack-api_internal_model_cube.LicenseResponse": {},
			"ablecloud_io_ablestack-api_internal_model_cube.NICResponse": {},
			"ablecloud_io_ablestack-api_internal_model_mold.MoldSessionValue": {},
			"ablecloud_io_ablestack-api_internal_model_glue.Response": {}
		}
	}`
	filtered, err := FilterDocForSCVM(raw, true)
	if err != nil {
		t.Fatalf("FilterDocForSCVM returned error: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(filtered, &doc); err != nil {
		t.Fatalf("filtered doc is not valid JSON: %v", err)
	}

	paths := doc["paths"].(map[string]any)
	for _, path := range []string{"/auth/login", "/health", "/version", "/cube/license", "/cube/license/apply", "/glue/status"} {
		if _, ok := paths[path]; !ok {
			t.Fatalf("filtered SCVM doc removed required path: %s", path)
		}
	}
	for _, path := range []string{"/cube/cluster/apply-local", "/cube/system/config"} {
		if _, ok := paths[path]; ok {
			t.Fatalf("filtered SCVM doc still contains hidden communication path: %s", path)
		}
	}
	if _, ok := paths["/cube/nics"]; ok {
		t.Fatalf("filtered SCVM doc still contains Cube operation path")
	}
	if _, ok := paths["/mold/session"]; ok {
		t.Fatalf("filtered SCVM doc still contains Mold path")
	}

	tags := doc["tags"].([]any)
	if len(tags) == 0 || !strings.HasPrefix(tags[0].(map[string]any)["name"].(string), "Glue") {
		t.Fatalf("first SCVM tag = %#v, want Glue first", tags)
	}
	for _, rawTag := range tags {
		tag := rawTag.(map[string]any)
		switch tag["name"] {
		case "Cube-Cluster", "Cube-System", "Cube-Nic":
			t.Fatalf("filtered SCVM doc still contains hidden Cube tag: %s", tag["name"])
		case "Mold":
			t.Fatalf("filtered SCVM doc still contains Mold tag")
		}
	}

	defs := doc["definitions"].(map[string]any)
	if _, ok := defs["ablecloud_io_ablestack-api_internal_model_cube.NICResponse"]; ok {
		t.Fatalf("filtered SCVM doc still contains hidden Cube definition")
	}
	if _, ok := defs["ablecloud_io_ablestack-api_internal_model_mold.MoldSessionValue"]; ok {
		t.Fatalf("filtered SCVM doc still contains Mold definition")
	}
	if _, ok := defs["ablecloud_io_ablestack-api_internal_model_cube.LicenseResponse"]; !ok {
		t.Fatalf("filtered SCVM doc removed Cube license definition")
	}
	if _, ok := defs["ablecloud_io_ablestack-api_internal_model_glue.Response"]; !ok {
		t.Fatalf("filtered SCVM doc removed Glue definition")
	}
}

func isTestSCVMVisibleCubePath(path string) bool {
	switch path {
	case "/cube/license", "/cube/license/apply":
		return true
	default:
		return false
	}
}

func TestGeneratedDocFilterRemovesGlueForNonSCVM(t *testing.T) {
	filtered, err := FilterDocForSCVM(docs.SwaggerInfo.ReadDoc(), false)
	if err != nil {
		t.Fatalf("FilterDocForSCVM returned error: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(filtered, &doc); err != nil {
		t.Fatalf("filtered doc is not valid JSON: %v", err)
	}

	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("filtered doc has no paths")
	}
	for path := range paths {
		if path == "/glue" || strings.HasPrefix(path, "/glue/") {
			t.Fatalf("filtered generated doc still contains Glue path: %s", path)
		}
		if path == "/mold" || strings.HasPrefix(path, "/mold/") {
			t.Fatalf("filtered generated doc still contains Mold path: %s", path)
		}
	}
	if _, ok := paths["/cube/license"]; !ok {
		t.Fatalf("filtered generated doc removed /cube/license")
	}
}

func TestGeneratedDocFilterKeepsMoldForCCVM(t *testing.T) {
	filtered, err := FilterDocForNode(docs.SwaggerInfo.ReadDoc(), false, true)
	if err != nil {
		t.Fatalf("FilterDocForNode returned error: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(filtered, &doc); err != nil {
		t.Fatalf("filtered doc is not valid JSON: %v", err)
	}

	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("filtered doc has no paths")
	}
	for _, path := range []string{"/mold", "/mold/session", "/mold/bootstrap", "/mold/jobs/{job_id}"} {
		if _, ok := paths[path]; !ok {
			t.Fatalf("filtered generated CCVM doc removed Mold path: %s", path)
		}
	}
	for path := range paths {
		if path == "/glue" || strings.HasPrefix(path, "/glue/") {
			t.Fatalf("filtered generated CCVM doc still contains Glue path: %s", path)
		}
	}
}

func TestGeneratedDocFilterRemovesCubeOperationsForSCVM(t *testing.T) {
	filtered, err := FilterDocForSCVM(docs.SwaggerInfo.ReadDoc(), true)
	if err != nil {
		t.Fatalf("FilterDocForSCVM returned error: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(filtered, &doc); err != nil {
		t.Fatalf("filtered doc is not valid JSON: %v", err)
	}

	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("filtered doc has no paths")
	}
	for _, path := range []string{"/auth/login", "/health", "/version", "/cube/license", "/cube/license/apply", "/glue/status"} {
		if _, ok := paths[path]; !ok {
			t.Fatalf("filtered generated SCVM doc removed required path: %s", path)
		}
	}
	for path := range paths {
		if strings.HasPrefix(path, "/cube/") && !isTestSCVMVisibleCubePath(path) {
			t.Fatalf("filtered generated SCVM doc still contains Cube operation path: %s", path)
		}
		if path == "/mold" || strings.HasPrefix(path, "/mold/") {
			t.Fatalf("filtered generated SCVM doc still contains Mold path: %s", path)
		}
	}

	tags, ok := doc["tags"].([]any)
	if !ok || len(tags) == 0 {
		t.Fatalf("filtered generated SCVM doc has no tags")
	}
	if first := tags[0].(map[string]any)["name"].(string); !strings.HasPrefix(first, "Glue") {
		t.Fatalf("first generated SCVM tag = %s, want Glue first", first)
	}
}
