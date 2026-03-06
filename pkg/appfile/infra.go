package appfile

import (
	"encoding/json"
	"os"

	"github.com/tailscale/hujson"

	configinfra "encore.dev/appruntime/exported/config/infra"
)

// LoadInfraConfig reads, standardizes, and parses the given config path as an infra.config.json file.
func LoadInfraConfig(infraFilePath string) (*configinfra.InfraConfig, error) {
	infraData, err := os.ReadFile(infraFilePath)
	if err != nil {
		return nil, err
	}

	stdData, err := hujson.Standardize(infraData)
	if err != nil {
		return nil, err
	}

	var parsed configinfra.InfraConfig
	if err := json.Unmarshal(stdData, &parsed); err != nil {
		return nil, err
	}

	return &parsed, nil
}
