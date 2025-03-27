package utils

import (
	"encoding/json"
	"os"
)

type RootConfig struct {
	DB          string `json:"db"`
	StatisticDB string `json:"statistic_db"`
	WebAddress  string `json:"web_address"`
	WebSecret   string `json:"web_secret"`
	StaticPath  string `json:"static_path"`
}

func LoadRootConfig(file string) (*RootConfig, error) {
	config := &RootConfig{}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}
	return config, nil
}
func UnmarshalConfig(c string) (map[string]interface{}, error) {
	var config map[string]interface{}
	err := json.Unmarshal([]byte(c), &config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func MarshalConfig(config map[string]interface{}) string {
	configData, err := json.MarshalIndent(config, "", "    ")
	if err != nil {
		return "Error marshalling config"
	} else {
		return string(configData)
	}
}
