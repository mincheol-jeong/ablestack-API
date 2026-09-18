package moldservice

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	CubeModel "ablecloud.io/ablestack-api/internal/model/cube"
	"ablecloud.io/ablestack-api/internal/service/clusterconfig"
)

const (
	defaultAbleStackConfigPath = "/etc/ablestack"
	defaultMoldScheme          = "http"
	defaultMoldPort            = "8080"
	defaultMoldAPIPath         = "/client/api"
)

func ResolveClusterJSONPath() string {
	if path := strings.TrimSpace(os.Getenv("ABLESTACK_CLUSTER_JSON")); path != "" {
		return path
	}
	if base := strings.TrimSpace(os.Getenv("ABLESTACK_CONFIG_PATH")); base != "" {
		return filepath.Join(base, "properties", "cluster.json")
	}

	defaultPath := filepath.Join(defaultAbleStackConfigPath, "properties", "cluster.json")
	if _, err := os.Stat(defaultPath); err == nil {
		return defaultPath
	}
	if _, err := os.Stat(filepath.Join("properties", "cluster.json")); err == nil {
		return filepath.Join("properties", "cluster.json")
	}
	return defaultPath
}

func LoadClusterConfigSection() (*CubeModel.ClusterConfigSection, string, error) {
	path := ResolveClusterJSONPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, path, err
	}
	root := map[string]any{}
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, path, err
	}
	normalized := clusterconfig.NormalizeClusterJSON(root)
	rawCfg, ok := normalized["clusterConfig"]
	if !ok {
		return nil, path, fmt.Errorf("clusterConfig not found")
	}
	cfgRaw, err := json.Marshal(rawCfg)
	if err != nil {
		return nil, path, err
	}
	var cfg CubeModel.ClusterConfigSection
	if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
		return nil, path, err
	}
	return &cfg, path, nil
}

func ResolveMoldEndpointFromCluster() (string, string, string, error) {
	cfg, path, err := LoadClusterConfigSection()
	if err != nil {
		return "", "", path, err
	}
	ccvmIP := strings.TrimSpace(cfg.CCVM.IP)
	if override := moldEndpointOverride(); override != "" {
		return override, ccvmIP, path, nil
	}
	if ccvmIP == "" {
		return "", "", path, ErrMissingCCVMIP
	}

	scheme := firstNonEmpty(os.Getenv("ABLESTACK_MOLD_API_SCHEME"), defaultMoldScheme)
	port := firstNonEmpty(os.Getenv("ABLESTACK_MOLD_API_PORT"), defaultMoldPort)
	apiPath := firstNonEmpty(os.Getenv("ABLESTACK_MOLD_API_PATH"), defaultMoldAPIPath)
	if !strings.HasPrefix(apiPath, "/") {
		apiPath = "/" + apiPath
	}
	return fmt.Sprintf("%s://%s:%s%s", scheme, ccvmIP, port, apiPath), ccvmIP, path, nil
}
