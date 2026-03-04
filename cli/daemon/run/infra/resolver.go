package infra

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"encore.dev/appruntime/exported/config"
	"encr.dev/cli/daemon/objects"
	"encr.dev/cli/daemon/redis"
	"encr.dev/cli/daemon/sqldb"
	"encr.dev/pkg/appfile"
)

// ValueResolver resolves appfile.Value instances.
type ValueResolver func(appfile.Value) (string, bool)

// DatabaseConfig represents resolved database configuration.
type DatabaseConfig struct {
	Host       string
	Database   string
	User       string
	Password   string
	EncoreName string
}

// CacheConfig represents resolved cache configuration.
type CacheConfig struct {
	Host       string
	User       string
	Password   string
	EncoreName string
	KeyPrefix  string
}

// PubSubProviderConfig represents resolved PubSub provider configuration.
type PubSubProviderConfig struct {
	Provider   string
	NSQHost    string
	GCPProject string
	AWSRegion  string
	AzureNS    string
}

// PubSubTopicConfig represents resolved PubSub topic configuration.
type PubSubTopicConfig struct {
	ProviderID   int
	EncoreName   string
	ProviderName string
	GCPProjectID string
}

// PubSubSubscriptionConfig represents resolved PubSub subscription configuration.
type PubSubSubscriptionConfig struct {
	ID              string
	EncoreName      string
	ProviderName    string
	PushOnly        bool
	GCPProjectID    string
	GCPPushSA       string
	GCPPushAudience string
}

// ObjectProviderConfig represents resolved object storage provider configuration.
type ObjectProviderConfig struct {
	Provider      string
	S3Endpoint    *string
	S3Region      string
	S3AccessKeyID *string
	S3SecretKey   *string
	GCSEndpoint   string
}

// BucketConfig represents resolved bucket configuration.
type BucketConfig struct {
	ProviderID    int
	EncoreName    string
	CloudName     string
	KeyPrefix     string
	PublicBaseURL string
}

// ResourceResolver resolves infrastructure resources from config.
type ResourceResolver[T any] interface {
	Resolve(name string, infra *appfile.Infra, resolveValue ValueResolver) (T, bool)
}

// DatabaseResolver resolves SQL database configurations.
type DatabaseResolver struct {
	LocalCluster   *sqldb.Cluster
	LocalProxyPort int
}

func (r DatabaseResolver) Resolve(name string, infra *appfile.Infra, resolveValue ValueResolver) (DatabaseConfig, bool) {
	if infra == nil || infra.Databases == nil {
		return DatabaseConfig{}, false
	}

	dbInfra, ok := infra.Databases[name]
	if !ok {
		return DatabaseConfig{}, false
	}

	connStr, ok := resolveValue(dbInfra.ConnectionString)
	if !ok {
		return DatabaseConfig{}, false
	}

	pCfg, err := pgx.ParseConfig(connStr)
	if err != nil {
		return DatabaseConfig{}, false
	}

	host := pCfg.Host
	if pCfg.Port != 0 {
		host = fmt.Sprintf("%s:%d", host, pCfg.Port)
	}

	return DatabaseConfig{
		Host:       host,
		Database:   pCfg.Database,
		User:       pCfg.User,
		Password:   pCfg.Password,
		EncoreName: name,
	}, true
}

func (r DatabaseResolver) ResolveWithFallback(name string, infra *appfile.Infra, resolveValue ValueResolver) (DatabaseConfig, error) {
	if cfg, ok := r.Resolve(name, infra, resolveValue); ok {
		return cfg, nil
	}

	if r.LocalCluster == nil {
		return DatabaseConfig{}, fmt.Errorf("no SQL cluster available for database %q", name)
	}

	return DatabaseConfig{
		Host:       "localhost:" + strconv.Itoa(r.LocalProxyPort),
		Database:   name,
		User:       "encore",
		Password:   r.LocalCluster.Password,
		EncoreName: name,
	}, nil
}

// CacheResolver resolves Redis cache configurations.
type CacheResolver struct {
	LocalServer *redis.Server
}

func (r CacheResolver) Resolve(name string, infra *appfile.Infra, resolveValue ValueResolver) (CacheConfig, bool) {
	if infra == nil || infra.Caches == nil {
		return CacheConfig{}, false
	}

	cacheInfra, ok := infra.Caches[name]
	if !ok {
		return CacheConfig{}, false
	}

	rawURL, ok := resolveValue(cacheInfra.URL)
	if !ok {
		return CacheConfig{}, false
	}

	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return CacheConfig{}, false
	}

	cfg := CacheConfig{
		Host:       u.Host,
		EncoreName: name,
		KeyPrefix:  name + "/",
	}

	if u.User != nil {
		cfg.User = u.User.Username()
		cfg.Password, _ = u.User.Password()
	}

	return cfg, true
}

func (r CacheResolver) ResolveWithFallback(name string, infra *appfile.Infra, resolveValue ValueResolver) (CacheConfig, error) {
	if cfg, ok := r.Resolve(name, infra, resolveValue); ok {
		return cfg, nil
	}

	if r.LocalServer == nil {
		return CacheConfig{}, fmt.Errorf("no Redis server available for cache %q", name)
	}

	return CacheConfig{
		Host:       r.LocalServer.Addr(),
		EncoreName: name,
		KeyPrefix:  name + "/",
	}, nil
}

// PubSubResolver resolves PubSub provider and topic configurations.
type PubSubResolver struct {
	LocalNSQ *NSQProvider
}

type NSQProvider struct {
	Host string
}

func (r PubSubResolver) ResolveProvider(providerIdx int, infra *appfile.Infra, resolveValue ValueResolver) (PubSubProviderConfig, bool) {
	if infra == nil || providerIdx < 0 || providerIdx >= len(infra.PubSub) {
		return PubSubProviderConfig{}, false
	}

	provider := infra.PubSub[providerIdx]
	cfg := PubSubProviderConfig{Provider: provider.Provider}

	for _, matcher := range pubSubMatchers {
		if matcher.Match(provider.Provider) {
			if ok := matcher.ResolveProvider(&provider, resolveValue, &cfg); !ok {
				return PubSubProviderConfig{}, false
			}
			return cfg, true
		}
	}

	return PubSubProviderConfig{}, false
}

func (r PubSubResolver) ResolveTopic(topicName string, infra *appfile.Infra, resolveValue ValueResolver) (PubSubTopicConfig, bool) {
	if infra == nil {
		return PubSubTopicConfig{}, false
	}

	for providerIdx, provider := range infra.PubSub {
		if topic, ok := provider.Topics[topicName]; ok {
			cfg := PubSubTopicConfig{
				ProviderID:   providerIdx,
				EncoreName:   topicName,
				ProviderName: topic.Name,
			}

			for _, matcher := range pubSubMatchers {
				if matcher.Match(provider.Provider) {
					matcher.ResolveTopic(&topic, &provider, resolveValue, &cfg)
					break
				}
			}

			return cfg, true
		}
	}

	return PubSubTopicConfig{}, false
}

func (r PubSubResolver) ResolveSubscription(topicName, subName string, infra *appfile.Infra, resolveValue ValueResolver) (PubSubSubscriptionConfig, bool) {
	if infra == nil {
		return PubSubSubscriptionConfig{}, false
	}

	for _, provider := range infra.PubSub {
		if topic, ok := provider.Topics[topicName]; ok {
			if sub, ok := topic.Subscriptions[subName]; ok {
				cfg := PubSubSubscriptionConfig{
					EncoreName: subName,
					PushOnly:   sub.PushConfig != nil,
				}

				name, ok := resolveValue(sub.Name)
				if !ok {
					return PubSubSubscriptionConfig{}, false
				}
				cfg.ProviderName = name
				cfg.ID = name

				for _, matcher := range pubSubMatchers {
					if matcher.Match(provider.Provider) {
						if ok := matcher.ResolveSubscription(&sub, &provider, resolveValue, &cfg); !ok {
							return PubSubSubscriptionConfig{}, false
						}
						break
					}
				}

				return cfg, true
			}
		}
	}

	return PubSubSubscriptionConfig{}, false
}

func (r PubSubResolver) ResolveWithFallback(infra *appfile.Infra, resolveValue ValueResolver) ([]PubSubProviderConfig, error) {
	if infra != nil && len(infra.PubSub) > 0 {
		var providers []PubSubProviderConfig
		for i := range infra.PubSub {
			cfg, ok := r.ResolveProvider(i, infra, resolveValue)
			if !ok {
				return nil, fmt.Errorf("failed to resolve PubSub provider %d", i)
			}
			providers = append(providers, cfg)
		}
		return providers, nil
	}

	if r.LocalNSQ == nil {
		return nil, fmt.Errorf("no PubSub provider available")
	}

	return []PubSubProviderConfig{{
		Provider: "nsq",
		NSQHost:  r.LocalNSQ.Host,
	}}, nil
}

// ObjectResolver resolves object storage provider and bucket configurations.
type ObjectResolver struct {
	LocalObjects *objects.Server
}

func (r ObjectResolver) ResolveProvider(providerIdx int, infra *appfile.Infra, resolveValue ValueResolver) (ObjectProviderConfig, bool) {
	if infra == nil || providerIdx < 0 || providerIdx >= len(infra.Objects) {
		return ObjectProviderConfig{}, false
	}

	provider := infra.Objects[providerIdx]
	cfg := ObjectProviderConfig{Provider: provider.Provider}

	for _, matcher := range objectMatchers {
		if matcher.Match(provider.Provider) {
			if ok := matcher.ResolveProvider(&provider, resolveValue, &cfg); !ok {
				return ObjectProviderConfig{}, false
			}
			return cfg, true
		}
	}

	return ObjectProviderConfig{}, false
}

func (r ObjectResolver) ResolveBucket(bucketName string, infra *appfile.Infra, resolveValue ValueResolver) (BucketConfig, bool) {
	if infra == nil {
		return BucketConfig{}, false
	}

	for providerIdx, provider := range infra.Objects {
		if bucket, ok := provider.Buckets[bucketName]; ok {
			cfg := BucketConfig{
				ProviderID: providerIdx,
				EncoreName: bucketName,
			}

			name, ok := resolveValue(bucket.Name)
			if !ok {
				return BucketConfig{}, false
			}
			cfg.CloudName = name

			if bucket.KeyPrefix != "" {
				if prefix, ok := resolveValue(bucket.KeyPrefix); ok {
					cfg.KeyPrefix = prefix
				}
			}

			if bucket.PublicBaseURL != "" {
				if pubURL, ok := resolveValue(bucket.PublicBaseURL); ok {
					cfg.PublicBaseURL = pubURL
				}
			}

			return cfg, true
		}
	}

	return BucketConfig{}, false
}

func (r ObjectResolver) ResolveWithFallback(infra *appfile.Infra, resolveValue ValueResolver) ([]ObjectProviderConfig, error) {
	if infra != nil && len(infra.Objects) > 0 {
		var providers []ObjectProviderConfig
		for i := range infra.Objects {
			cfg, ok := r.ResolveProvider(i, infra, resolveValue)
			if !ok {
				return nil, fmt.Errorf("failed to resolve object storage provider %d", i)
			}
			providers = append(providers, cfg)
		}
		return providers, nil
	}

	if r.LocalObjects == nil {
		return nil, fmt.Errorf("no object storage provider available")
	}

	return []ObjectProviderConfig{{
		Provider:    "gcs",
		GCSEndpoint: r.LocalObjects.Endpoint(),
	}}, nil
}

// NeedsLocalCheck provides methods to determine if local services are needed.
type NeedsLocalCheck struct {
	Infra        *appfile.Infra
	ResolveValue ValueResolver
}

func (c NeedsLocalCheck) NeedsLocalDatabase(dbNames []string) bool {
	if c.Infra == nil || c.Infra.Databases == nil {
		return true
	}

	for _, name := range dbNames {
		if _, ok := c.Infra.Databases[name]; !ok {
			return true
		}
		if _, ok := c.ResolveValue(c.Infra.Databases[name].ConnectionString); !ok {
			return true
		}
	}

	return false
}

func (c NeedsLocalCheck) NeedsLocalCache(cacheNames []string) bool {
	if c.Infra == nil || c.Infra.Caches == nil {
		return true
	}

	for _, name := range cacheNames {
		if _, ok := c.Infra.Caches[name]; !ok {
			return true
		}
		if _, ok := c.ResolveValue(c.Infra.Caches[name].URL); !ok {
			return true
		}
	}

	return false
}

func (c NeedsLocalCheck) NeedsLocalPubSub() bool {
	if c.Infra == nil || len(c.Infra.PubSub) == 0 {
		return true
	}

	for _, provider := range c.Infra.PubSub {
		matched := false
		for _, matcher := range pubSubMatchers {
			if matcher.Match(provider.Provider) {
				matched = true
				if matcher.NeedsLocal(&provider, c.ResolveValue) {
					return true
				}
				break
			}
		}
		if !matched {
			return true
		}
	}

	return false
}

func (c NeedsLocalCheck) NeedsLocalObjects() bool {
	if c.Infra == nil || len(c.Infra.Objects) == 0 {
		return true
	}

	for _, provider := range c.Infra.Objects {
		matched := false
		for _, matcher := range objectMatchers {
			if matcher.Match(provider.Provider) {
				matched = true
				if matcher.NeedsLocal(&provider, c.ResolveValue) {
					return true
				}
				break
			}
		}
		if !matched {
			return true
		}
	}

	return false
}

// GetDefaultProviderIndex returns the index of the provider to use for auto-assignment.
// If only one provider exists, it returns 0. Otherwise, it returns -1.
func GetDefaultProviderIndex[T any](providers []T) int {
	if len(providers) == 1 {
		return 0
	}
	return -1
}

// AutoAssignTopic assigns a topic to a provider if only one provider exists.
func AutoAssignTopic(topicName string, infra *appfile.Infra) (providerIdx int, found bool) {
	if infra == nil || len(infra.PubSub) == 0 {
		return -1, false
	}

	for i, provider := range infra.PubSub {
		if _, ok := provider.Topics[topicName]; ok {
			return i, true
		}
	}

	if len(infra.PubSub) == 1 {
		return 0, true
	}

	return -1, false
}

// AutoAssignBucket assigns a bucket to a provider if only one provider exists.
func AutoAssignBucket(bucketName string, infra *appfile.Infra) (providerIdx int, found bool) {
	if infra == nil || len(infra.Objects) == 0 {
		return -1, false
	}

	for i, provider := range infra.Objects {
		if _, ok := provider.Buckets[bucketName]; ok {
			return i, true
		}
	}

	if len(infra.Objects) == 1 {
		return 0, true
	}

	return -1, false
}

// ToConfigSQLServer converts DatabaseConfig to config.SQLServer.
func ToConfigSQLServer(cfg DatabaseConfig) config.SQLServer {
	return config.SQLServer{Host: cfg.Host}
}

// ToConfigSQLDatabase converts DatabaseConfig to config.SQLDatabase.
func ToConfigSQLDatabase(cfg DatabaseConfig) config.SQLDatabase {
	return config.SQLDatabase{
		EncoreName:   cfg.EncoreName,
		DatabaseName: cfg.Database,
		User:         cfg.User,
		Password:     cfg.Password,
	}
}

// ToConfigRedisServer converts CacheConfig to config.RedisServer.
func ToConfigRedisServer(cfg CacheConfig) config.RedisServer {
	return config.RedisServer{
		Host:     cfg.Host,
		User:     cfg.User,
		Password: cfg.Password,
	}
}

// ToConfigRedisDatabase converts CacheConfig to config.RedisDatabase.
func ToConfigRedisDatabase(cfg CacheConfig) config.RedisDatabase {
	return config.RedisDatabase{
		EncoreName: cfg.EncoreName,
		KeyPrefix:  cfg.KeyPrefix,
	}
}

// ToConfigPubsubProvider converts PubSubProviderConfig to config.PubsubProvider.
func ToConfigPubsubProvider(cfg PubSubProviderConfig) config.PubsubProvider {
	switch cfg.Provider {
	case "nsq":
		return config.PubsubProvider{
			NSQ: &config.NSQProvider{Host: cfg.NSQHost},
		}
	case "gcp":
		return config.PubsubProvider{
			GCP: &config.GCPPubsubProvider{},
		}
	case "aws":
		return config.PubsubProvider{
			AWS: &config.AWSPubsubProvider{},
		}
	case "azure":
		return config.PubsubProvider{
			Azure: &config.AzureServiceBusProvider{Namespace: cfg.AzureNS},
		}
	default:
		return config.PubsubProvider{}
	}
}

// ToConfigPubsubTopic converts PubSubTopicConfig to config.PubsubTopic.
func ToConfigPubsubTopic(cfg PubSubTopicConfig) config.PubsubTopic {
	topic := config.PubsubTopic{
		ProviderID:    cfg.ProviderID,
		EncoreName:    cfg.EncoreName,
		ProviderName:  cfg.ProviderName,
		Subscriptions: make(map[string]*config.PubsubSubscription),
	}

	if cfg.GCPProjectID != "" {
		topic.GCP = &config.PubsubTopicGCPData{ProjectID: cfg.GCPProjectID}
	}

	return topic
}

// ToConfigPubsubSubscription converts PubSubSubscriptionConfig to config.PubsubSubscription.
func ToConfigPubsubSubscription(cfg PubSubSubscriptionConfig) config.PubsubSubscription {
	sub := config.PubsubSubscription{
		ID:           cfg.ID,
		EncoreName:   cfg.EncoreName,
		ProviderName: cfg.ProviderName,
		PushOnly:     cfg.PushOnly,
	}

	if cfg.GCPProjectID != "" || cfg.GCPPushSA != "" {
		sub.GCP = &config.PubsubSubscriptionGCPData{
			ProjectID:          cfg.GCPProjectID,
			PushServiceAccount: cfg.GCPPushSA,
		}
	}

	return sub
}

// ToConfigBucketProvider converts ObjectProviderConfig to config.BucketProvider.
func ToConfigBucketProvider(cfg ObjectProviderConfig) config.BucketProvider {
	switch cfg.Provider {
	case "s3":
		return config.BucketProvider{
			S3: &config.S3BucketProvider{
				Region:          cfg.S3Region,
				Endpoint:        cfg.S3Endpoint,
				AccessKeyID:     cfg.S3AccessKeyID,
				SecretAccessKey: cfg.S3SecretKey,
			},
		}
	case "gcs":
		return config.BucketProvider{
			GCS: &config.GCSBucketProvider{
				Endpoint: cfg.GCSEndpoint,
			},
		}
	default:
		return config.BucketProvider{}
	}
}

// ToConfigBucket converts BucketConfig to config.Bucket.
func ToConfigBucket(cfg BucketConfig) config.Bucket {
	return config.Bucket{
		ProviderID:    cfg.ProviderID,
		EncoreName:    cfg.EncoreName,
		CloudName:     cfg.CloudName,
		KeyPrefix:     cfg.KeyPrefix,
		PublicBaseURL: cfg.PublicBaseURL,
	}
}

// EnsureValidNSQName ensures a name is valid for NSQ by replacing invalid characters.
func EnsureValidNSQName(name string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			return r
		}
		return '_'
	}, name)
}
