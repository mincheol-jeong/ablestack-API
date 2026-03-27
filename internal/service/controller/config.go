package controller

import (
	"os"
	"path/filepath"

	"github.com/goccy/go-json"
)

type Config struct {
	Neighbor []TypeNeighbor `json:"neighbor"`
}

const defaultConfigFile = "configs/config.json"

func configPath() string {
	if v := os.Getenv("CUBE_CONFIG_PATH"); v != "" {
		return v
	}
	if _, err := os.Stat(defaultConfigFile); err == nil {
		return defaultConfigFile
	}
	exe, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exe)
		candidate := filepath.Join(exeDir, "..", "configs", "config.json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		candidate = filepath.Join(exeDir, "configs", "config.json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return defaultConfigFile
}

func SaveConfig() {
	config := Config{
		Neighbor: controller.Neighbor.Neighbors,
	}

	fc, err := os.OpenFile(configPath(), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return
	}
	defer fc.Close()

	strconfig, err := json.Marshal(config)

	_, err = fc.Write(strconfig)
	if err != nil {
		return
	}

}
