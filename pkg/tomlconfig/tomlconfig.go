// Package tomlconfig reads and writes encore.toml files.
package tomlconfig

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/pelletier/go-toml/v2"

	"encore.dev/appruntime/exported/experiments"
)

// Names for the TOML config file.
const (
	NameOld = "encore.toml"
	Name    = "afterpiece.toml"
)

// File represents the TOML configuration file structure.
type File struct {
	ID          string             `toml:"id"`
	Experiments []experiments.Name `toml:"experiments,omitempty"`
	Language    string             `toml:"language"`
	Log         Log                `toml:"log,omitempty"`
	CORS        *CORS              `toml:"cors,omitempty"`
	Build       Build              `toml:"build,omitempty"`
	Migrations  Migrations         `toml:"migrations,omitempty"`
}

type Log struct {
	Level string `toml:"level,omitempty"`
}

type CORS struct {
	Debug                          bool     `toml:"debug,omitempty"`
	AllowHeaders                   []string `toml:"allow_headers"`
	ExposeHeaders                  []string `toml:"expose_headers"`
	AllowOriginsWithoutCredentials []string `toml:"allow_origins_without_credentials,omitempty"`
	AllowOriginsWithCredentials    []string `toml:"allow_origins_with_credentials,omitempty"`
}

type Build struct {
	CgoEnabled    bool   `toml:"cgo_enabled,omitempty"`
	WorkerPooling bool   `toml:"worker_pooling,omitempty"`
	Docker        Docker `toml:"docker,omitempty"`
}

type Docker struct {
	BaseImage         string `toml:"base_image,omitempty"`
	BundleSource      bool   `toml:"bundle_source,omitempty"`
	WorkingDir        string `toml:"working_dir,omitempty"`
	ProcessPerService bool   `toml:"process_per_service,omitempty"`
}

type Migrations struct {
	Strategy string `toml:"strategy,omitempty"`
}

// Parse parses the TOML config file data into a File.
func Parse(data []byte) (*File, error) {
	var f File
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("tomlconfig.Parse: %w", err)
	}

	switch f.Language {
	case "go", "typescript":
	// Do nothing
	case "":
		f.Language = "go"
	default:
		return nil, fmt.Errorf("tomlconfig.Parse: invalid lang %q", f.Language)
	}

	return &f, nil
}

// ParseFile parses the toml file located at path.
func ParseFile(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &File{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("tomlconfig.ParseFile: %w", err)
	}
	return Parse(data)
}
