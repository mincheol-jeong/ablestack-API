package swagger

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"ablecloud.io/ablestack-api/docs"
	GlueHandler "ablecloud.io/ablestack-api/internal/handler/glue"
	MoldHandler "ablecloud.io/ablestack-api/internal/handler/mold"
)

// Handler는 Swagger UI asset은 gin-swagger에 맡기고 doc.json만 role에 맞게 필터링한다.
// host에서는 Glue/Mold를 숨기고, CCVM에서는 Mold를 보이며, SCVM에서는 Glue 중심으로 보이도록 Cube 운영 API를 숨긴다.
func Handler() gin.HandlerFunc {
	ui := ginSwagger.WrapHandler(swaggerFiles.Handler)
	return func(ctx *gin.Context) {
		if strings.HasSuffix(ctx.Request.URL.Path, "/doc.json") {
			doc, err := FilterDocForNode(docs.SwaggerInfo.ReadDoc(), GlueHandler.IsSCVMNode(), MoldHandler.IsCCVMNode())
			if err != nil {
				ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
				return
			}
			ctx.Data(http.StatusOK, "application/json; charset=utf-8", doc)
			return
		}
		ui(ctx)
	}
}

func FilterDocForSCVM(raw string, scvm bool) ([]byte, error) {
	return FilterDocForNode(raw, scvm, false)
}

func FilterDocForNode(raw string, scvm bool, ccvm bool) ([]byte, error) {
	var doc map[string]any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&doc); err != nil {
		return nil, err
	}

	if !ccvm {
		removeMoldPaths(doc)
	}
	if scvm {
		removeSCVMHiddenCubePaths(doc)
		orderSCVMTags(doc)
	} else {
		removeGluePaths(doc)
		removeGlueTags(doc)
	}
	if !ccvm {
		removeMoldTags(doc)
	}
	pruneUnreferencedDefinitions(doc)

	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(doc); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func removeGluePaths(doc map[string]any) {
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		return
	}
	for path := range paths {
		if path == "/glue" || strings.HasPrefix(path, "/glue/") {
			delete(paths, path)
		}
	}
}

func removeGlueTags(doc map[string]any) {
	rawTags, ok := doc["tags"].([]any)
	if !ok {
		return
	}
	tags := make([]any, 0, len(rawTags))
	for _, rawTag := range rawTags {
		tag, ok := rawTag.(map[string]any)
		if !ok {
			tags = append(tags, rawTag)
			continue
		}
		name, _ := tag["name"].(string)
		if strings.HasPrefix(name, "Glue") {
			continue
		}
		tags = append(tags, rawTag)
	}
	doc["tags"] = tags
}

func removeMoldPaths(doc map[string]any) {
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		return
	}
	for path := range paths {
		if path == "/mold" || strings.HasPrefix(path, "/mold/") {
			delete(paths, path)
		}
	}
}

func removeMoldTags(doc map[string]any) {
	rawTags, ok := doc["tags"].([]any)
	if !ok {
		return
	}
	tags := make([]any, 0, len(rawTags))
	for _, rawTag := range rawTags {
		tag, ok := rawTag.(map[string]any)
		if !ok {
			tags = append(tags, rawTag)
			continue
		}
		name, _ := tag["name"].(string)
		if strings.HasPrefix(name, "Mold") {
			continue
		}
		tags = append(tags, rawTag)
	}
	doc["tags"] = tags
}

func removeSCVMHiddenCubePaths(doc map[string]any) {
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		return
	}
	for path := range paths {
		if strings.HasPrefix(path, "/cube/") && !isSCVMVisibleCubePath(path) {
			delete(paths, path)
		}
	}
}

func isSCVMVisibleCubePath(path string) bool {
	switch path {
	case "/cube/license", "/cube/license/apply":
		return true
	default:
		return false
	}
}

func orderSCVMTags(doc map[string]any) {
	names := collectOperationTagNames(doc)
	if len(names) == 0 {
		names = collectTopLevelTagNames(doc)
	}
	for name := range names {
		if !isSCVMVisibleTagName(name) {
			delete(names, name)
		}
	}
	if len(names) == 0 {
		return
	}

	orderedNames := orderTagNamesForSCVM(names)
	tags := make([]any, 0, len(orderedNames))
	for _, name := range orderedNames {
		tags = append(tags, map[string]any{"name": name})
	}
	doc["tags"] = tags
}

func collectTopLevelTagNames(doc map[string]any) map[string]bool {
	names := map[string]bool{}
	rawTags, ok := doc["tags"].([]any)
	if !ok {
		return names
	}
	for _, rawTag := range rawTags {
		tag, ok := rawTag.(map[string]any)
		if !ok {
			continue
		}
		name, _ := tag["name"].(string)
		if name = strings.TrimSpace(name); name != "" {
			names[name] = true
		}
	}
	return names
}

func isSCVMVisibleTagName(name string) bool {
	if !strings.HasPrefix(name, "Cube") {
		return true
	}
	switch name {
	case "Cube-License", "Cube-Version":
		return true
	default:
		return false
	}
}

func collectOperationTagNames(doc map[string]any) map[string]bool {
	names := map[string]bool{}
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		return names
	}
	for _, rawPathItem := range paths {
		pathItem, ok := rawPathItem.(map[string]any)
		if !ok {
			continue
		}
		for _, rawOperation := range pathItem {
			operation, ok := rawOperation.(map[string]any)
			if !ok {
				continue
			}
			for _, rawTag := range operationTags(operation) {
				if name := strings.TrimSpace(rawTag); name != "" {
					names[name] = true
				}
			}
		}
	}
	return names
}

func operationTags(operation map[string]any) []string {
	rawTags, ok := operation["tags"].([]any)
	if !ok {
		return nil
	}
	tags := make([]string, 0, len(rawTags))
	for _, rawTag := range rawTags {
		if tag, ok := rawTag.(string); ok {
			tags = append(tags, tag)
		}
	}
	return tags
}

func orderTagNamesForSCVM(names map[string]bool) []string {
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		leftRank := scvmTagRank(ordered[i])
		rightRank := scvmTagRank(ordered[j])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return ordered[i] < ordered[j]
	})
	return ordered
}

func scvmTagRank(name string) int {
	switch {
	case strings.HasPrefix(name, "Glue"):
		return 0
	case name == "Auth":
		return 1
	case name == "Health":
		return 2
	case name == "Cube-License":
		return 3
	case name == "Cube-Version":
		return 4
	default:
		return 5
	}
}

func pruneUnreferencedDefinitions(doc map[string]any) {
	definitions, ok := doc["definitions"].(map[string]any)
	if !ok {
		return
	}

	refs := map[string]bool{}
	collectDefinitionRefs(doc["paths"], refs)
	collectDefinitionRefs(doc["parameters"], refs)
	collectDefinitionRefs(doc["responses"], refs)

	queue := make([]string, 0, len(refs))
	for name := range refs {
		queue = append(queue, name)
	}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]

		definition, ok := definitions[name]
		if !ok {
			continue
		}
		nextRefs := map[string]bool{}
		collectDefinitionRefs(definition, nextRefs)
		for next := range nextRefs {
			if !refs[next] {
				refs[next] = true
				queue = append(queue, next)
			}
		}
	}

	for name := range definitions {
		if !refs[name] {
			delete(definitions, name)
		}
	}
}

func collectDefinitionRefs(value any, refs map[string]bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "$ref" {
				if ref, ok := child.(string); ok {
					if name, ok := strings.CutPrefix(ref, "#/definitions/"); ok {
						refs[name] = true
					}
				}
				continue
			}
			collectDefinitionRefs(child, refs)
		}
	case []any:
		for _, child := range typed {
			collectDefinitionRefs(child, refs)
		}
	}
}
