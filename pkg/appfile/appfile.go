// Package appfile reads and writes encore.app files.
package appfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/tailscale/hujson"

	"encore.dev/appruntime/exported/experiments"
)

const (
	MigrationStrategyGoMigrate = "go-migrate"
	MigrationStrategyAtlas     = "atlas"
)

const (
	LegacyAppConfig     = "encore.app"
	LegacyJsonAppConfig = "encore.json"
	LegacyTomlAppConfig = "encore.toml"
	JsonAppConfig       = "afterpiece.json"
	TomlAppConfig       = "afterpiece.toml"
)

// FindProjectConfig searches for a valid project configuration file
func FindProjectConfig(dir string) (string, error) {
	paths := []string{
		filepath.Join(dir, LegacyAppConfig),
		filepath.Join(dir, LegacyJsonAppConfig),
		filepath.Join(dir, LegacyTomlAppConfig),
		filepath.Join(dir, JsonAppConfig),
		filepath.Join(dir, TomlAppConfig),
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
	}
	return "", fs.ErrNotExist
}

func IsAnyOf(c string) bool {
	return c == LegacyAppConfig || c == LegacyJsonAppConfig || c == LegacyTomlAppConfig || c == JsonAppConfig || c == TomlAppConfig
}

type File struct {
	// ID is the encore.dev app id for the app.
	// It is empty if the app is not linked to encore.dev.
	ID string `json:"id" toml:"id"` // can be empty

	// Experiments is a list of values to enable experimental features in Encore.
	// These are not guaranteed to be stable in either runtime behaviour
	// or in API design.
	//
	// Do not use these features in production without consulting the Encore team.
	Experiments []experiments.Name `json:"experiments,omitempty" toml:"experiments,omitempty"`

	// Configure global CORS settings for the application which
	// will be applied to all API gateways into the application.
	GlobalCORS *CORS `json:"global_cors,omitempty" toml:"cors,omitempty"`

	// Build contains build settings for the application.
	Build Build `json:"build,omitzero" toml:"build,omitzero"`

	// LogLevel is the minimum log level for the app.
	// If empty it defaults to "trace".
	LogLevel string `json:"log_level,omitempty" toml:"log_level,omitempty"`

	// Migrations configures the database migration strategy.
	Migrations Migrations `json:"migrations,omitzero" toml:"migrations,omitzero"`

	// Infra defines external infrastructure configurations per environment
	Infra *Infra `json:"infra,omitempty" toml:"infra,omitempty"`
}

type Infra struct {
	Databases map[string]DatabaseInfra `json:"databases,omitempty" toml:"databases,omitempty"`
	Caches    map[string]CacheInfra    `json:"caches,omitempty" toml:"caches,omitempty"`
	PubSub    []PubSubInfra            `json:"pubsub,omitempty" toml:"pubsub,omitempty"`
	Objects   []ObjectInfra            `json:"objects,omitempty" toml:"objects,omitempty"`
}

// Value is a configuration value that can be either a literal string
// or an environment variable reference in the form "env:VAR_NAME".
type Value string

// Resolve returns the resolved value. If the value starts with "env:",
// it looks up the environment variable using the provided getter.
// Returns the resolved value and true if found, or empty string and false
// if the environment variable is not set.
func (v Value) Resolve(getEnv func(string) string) (string, bool) {
	s := string(v)
	if envVar, ok := strings.CutPrefix(s, "env:"); ok {
		val := getEnv(envVar)
		return val, val != ""
	}
	return s, s != ""
}

// DatabaseInfra configures an external SQL database.
type DatabaseInfra struct {
	ConnectionString Value `json:"connection_string" toml:"connection_string"`
}

// CacheInfra configures an external Redis cache.
type CacheInfra struct {
	URL Value `json:"url" toml:"url"`
}

// PubSubInfra configures a PubSub provider with its topics.
type PubSubInfra struct {
	Provider string `json:"provider" toml:"provider"`

	NSQ   *NSQPubSubInfra   `json:"nsq,omitempty" toml:"nsq,omitempty"`
	GCP   *GCPPubSubInfra   `json:"gcp,omitempty" toml:"gcp,omitempty"`
	AWS   *AWSPubSubInfra   `json:"aws,omitempty" toml:"aws,omitempty"`
	Azure *AzurePubSubInfra `json:"azure,omitempty" toml:"azure,omitempty"`

	Topics map[string]TopicInfra `json:"topics,omitempty" toml:"topics,omitempty"`
}

type NSQPubSubInfra struct {
	Hosts Value `json:"hosts" toml:"hosts"`
}

type GCPPubSubInfra struct {
	ProjectID Value `json:"project_id" toml:"project_id"`
}

type AWSPubSubInfra struct {
	Region Value `json:"region" toml:"region"`
}

type AzurePubSubInfra struct {
	Namespace Value `json:"namespace" toml:"namespace"`
}

type TopicInfra struct {
	Name          string                       `json:"name" toml:"name"`
	ProjectID     Value                        `json:"project_id,omitempty" toml:"project_id,omitempty"`
	Subscriptions map[string]SubscriptionInfra `json:"subscriptions,omitempty" toml:"subscriptions,omitempty"`
}

type SubscriptionInfra struct {
	Name       Value      `json:"name" toml:"name"`
	ProjectID  Value      `json:"project_id,omitempty" toml:"project_id,omitempty"`
	URL        Value      `json:"url,omitempty" toml:"url,omitempty"`
	PushConfig *PushInfra `json:"push_config,omitempty" toml:"push_config,omitempty"`
}

type PushInfra struct {
	ServiceAccount Value `json:"service_account" toml:"service_account"`
	JWTAudience    Value `json:"jwt_audience" toml:"jwt_audience"`
	ID             Value `json:"id" toml:"id"`
}

// ObjectInfra configures an object storage provider with its buckets.
type ObjectInfra struct {
	Provider string `json:"provider" toml:"provider"`

	S3  *S3Infra  `json:"s3,omitempty" toml:"s3,omitempty"`
	GCS *GCSInfra `json:"gcs,omitempty" toml:"gcs,omitempty"`

	Buckets map[string]BucketInfra `json:"buckets,omitempty" toml:"buckets,omitempty"`
}

type S3Infra struct {
	Endpoint        Value `json:"endpoint,omitempty" toml:"endpoint,omitempty"`
	Region          Value `json:"region" toml:"region"`
	AccessKeyID     Value `json:"access_key_id,omitempty" toml:"access_key_id,omitempty"`
	SecretAccessKey Value `json:"secret_access_key,omitempty" toml:"secret_access_key,omitempty"`
}

type GCSInfra struct {
	Endpoint Value `json:"endpoint,omitempty" toml:"endpoint,omitempty"`
}

type BucketInfra struct {
	Name          Value `json:"name" toml:"name"`
	KeyPrefix     Value `json:"key_prefix,omitempty" toml:"key_prefix,omitempty"`
	PublicBaseURL Value `json:"public_base_url,omitempty" toml:"public_base_url,omitempty"`
}

type Migrations struct {
	Strategy string `json:"strategy,omitempty" toml:"strategy,omitempty"`
	Auto     *bool  `json:"auto,omitempty" toml:"auto,omitempty"`
}

type Build struct {
	// CgoEnabled enables building with cgo.
	CgoEnabled bool `json:"cgo_enabled,omitempty" toml:"cgo_enabled,omitempty"`

	// Docker configures the docker images built
	// by Encore's CI/CD system.
	Docker Docker `json:"docker,omitzero" toml:"docker,omitzero"`

	// WorkerPooling enables worker pooling for Encore.ts.
	WorkerPooling bool `json:"worker_pooling,omitempty" toml:"worker_pooling,omitempty"`
}

type Docker struct {
	// BaseImage changes the docker base image used for building the application
	// in Encore's CI/CD system. If unspecified it defaults to "scratch".
	BaseImage string `json:"base_image,omitempty" toml:"base_image,omitempty"`

	// BundleSource determines whether the source code of the application
	// should be bundled into the binary, at "/workspace".
	BundleSource bool `json:"bundle_source,omitempty" toml:"bundle_source,omitempty"`

	// WorkingDir specifies the working directory to start the docker image in.
	// If empty it defaults to "/workspace" if the source code is bundled, and to "/" otherwise.
	WorkingDir string `json:"working_dir,omitempty" toml:"working_dir,omitempty"`

	// ProcessPerService specifies whether each service should run in its own process. If false,
	// all services are run in the same process.
	ProcessPerService bool `json:"process_per_service,omitempty" toml:"process_per_service,omitempty"`
}

type CORS struct {
	// Debug enables CORS debug logging.
	Debug bool `json:"debug,omitempty" toml:"debug,omitempty"`

	// AllowHeaders allows an app to specify additional headers that should be
	// accepted by the app.
	//
	// If the list contains "*", then all headers are allowed.
	AllowHeaders []string `json:"allow_headers" toml:"allow_headers"`

	// ExposeHeaders allows an app to specify additional headers that should be
	// exposed from the app, beyond the default set always recognized by Encore.
	//
	// If the list contains "*", then all headers are exposed.
	ExposeHeaders []string `json:"expose_headers" toml:"expose_headers"`

	// AllowOriginsWithoutCredentials specifies the allowed origins for requests
	// that don't include credentials. If nil it defaults to allowing all domains
	// (equivalent to []string{"*"}).
	AllowOriginsWithoutCredentials []string `json:"allow_origins_without_credentials,omitempty" toml:"allow_origins_without_credentials,omitempty"`

	// AllowOriginsWithCredentials specifies the allowed origins for requests
	// that include credentials. If a request is made from an Origin in this list
	// Encore responds with Access-Control-Allow-Origin: <Origin>.
	//
	// The URLs in this list may include wildcards (e.g. "https://*.example.com"
	// or "https://*-myapp.example.com").
	AllowOriginsWithCredentials []string `json:"allow_origins_with_credentials,omitempty" toml:"allow_origins_with_credentials,omitempty"`
}

// Parse parses the app file data into a File.
func Parse(data []byte, format string) (*File, error) {
	var f File
	var err error
	switch format {
	case "json":
		data, err := hujson.Standardize(data)
		if err == nil {
			err = json.Unmarshal(data, &f)
		}
	case "toml":
		err = toml.Unmarshal(data, &f)
	}
	if err != nil {
		return nil, fmt.Errorf("appfile.Parse: %v", err)
	}

	return &f, nil
}

// ParseFile parses the app file located at path.
func ParseFile(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &File{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("appfile.ParseFile: %w", err)
	}

	base := filepath.Base(path)
	if base == TomlAppConfig || base == LegacyTomlAppConfig {
		tomlConf, err := Parse(data, "toml")
		if err != nil {
			return nil, fmt.Errorf("appfile.ParseFile (%s): %w", base, err)
		}
		return tomlConf, nil
	}

	return Parse(data, "json")
}

// ID returns the app ID for the project located at appRoot.
// The ID can be empty if the app is not linked to encore.dev.
func ID(appRoot string) (string, error) {
	f, err := getParsedFile(appRoot)
	if err != nil {
		return "", err
	}
	return f.ID, nil
}

// Experiments returns the experimental feature the app located
// at appRoot has opted into.
func Experiments(appRoot string) ([]experiments.Name, error) {
	f, err := getParsedFile(appRoot)
	if err != nil {
		return nil, err
	}
	return f.Experiments, nil
}

// GlobalCORS returns the global CORS settings for the app located
func GlobalCORS(appRoot string) (*CORS, error) {
	f, err := getParsedFile(appRoot)
	if err != nil {
		return nil, err
	}
	return f.GlobalCORS, nil
}

func getParsedFile(appRoot string) (*File, error) {
	configPath, err := FindProjectConfig(appRoot)
	if errors.Is(err, fs.ErrNotExist) {
		return &File{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("appfile.ParseFile: %w", err)
	}
	return ParseFile(configPath)
}
