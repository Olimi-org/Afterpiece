package infra

import (
	"fmt"

	"encore.dev/appruntime/exported/config"
	configinfra "encore.dev/appruntime/exported/config/infra"
	"encr.dev/cli/daemon/objects"
)

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

// ObjectResolver resolves object storage provider and bucket configurations.
type ObjectResolver struct {
	LocalObjects *objects.Server
}

func (r ObjectResolver) ResolveProvider(providerIdx int, infra *configinfra.InfraConfig, resolveValue ValueResolver) (ObjectProviderConfig, bool) {
	if infra == nil || providerIdx < 0 || providerIdx >= len(infra.ObjectStorage) {
		return ObjectProviderConfig{}, false
	}

	provider := infra.ObjectStorage[providerIdx]
	cfg := ObjectProviderConfig{Provider: provider.Type}

	for _, matcher := range objectMatchers {
		if matcher.Match(provider.Type) {
			if ok := matcher.ResolveProvider(provider, resolveValue, &cfg); !ok {
				return ObjectProviderConfig{}, false
			}
			return cfg, true
		}
	}

	return ObjectProviderConfig{}, false
}

func (r ObjectResolver) ResolveBucket(bucketName string, infra *configinfra.InfraConfig, resolveValue ValueResolver) (BucketConfig, bool) {
	if infra == nil || len(infra.ObjectStorage) == 0 {
		return BucketConfig{}, false
	}

	for providerIdx, provider := range infra.ObjectStorage {
		buckets := provider.GetBuckets()
		if bucket, ok := buckets[bucketName]; ok {
			cfg := BucketConfig{
				ProviderID: providerIdx,
				EncoreName: bucketName,
			}

			name, ok := resolveValue(bucket.Name)
			if !ok {
				return BucketConfig{}, false
			}
			cfg.CloudName = name

			if bucket.KeyPrefix.Value() != "" {
				if prefix, ok := resolveValue(bucket.KeyPrefix); ok {
					cfg.KeyPrefix = prefix
				}
			}

			if bucket.PublicBaseURL.Value() != "" {
				if pubURL, ok := resolveValue(bucket.PublicBaseURL); ok {
					cfg.PublicBaseURL = pubURL
				}
			}

			return cfg, true
		}
	}

	return BucketConfig{}, false
}

func (r ObjectResolver) ResolveWithFallback(infra *configinfra.InfraConfig, resolveValue ValueResolver) ([]ObjectProviderConfig, error) {
	if infra != nil && len(infra.ObjectStorage) > 0 {
		var providers []ObjectProviderConfig
		for i := range infra.ObjectStorage {
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

// ToConfigBucketProvider converts ObjectProviderConfig to config.BucketProvider.
func ToConfigBucketProvider(cfg ObjectProviderConfig) config.BucketProvider {
	for _, matcher := range objectMatchers {
		if matcher.Match(cfg.Provider) {
			return matcher.ToConfigProvider(cfg)
		}
	}
	return config.BucketProvider{}
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
