package infra

import (
	"testing"

	"encr.dev/pkg/appfile"
)

func mockResolver(val string, ok bool) ValueResolver {
	return func(v appfile.Value) (string, bool) {
		return val, ok
	}
}

func mockRealResolver(envs map[string]string) ValueResolver {
	return func(v appfile.Value) (string, bool) {
		// Use the value's native Resolve function but mock the env getter
		return v.Resolve(func(s string) string {
			return envs[s]
		})
	}
}
func TestDatabaseResolver(t *testing.T) {
	dummyInfra := &appfile.Infra{
		Databases: map[string]appfile.DatabaseInfra{
			"auth":  {ConnectionString: appfile.Value("postgres://user:pass@localhost:5432/auth")},
			"envdb": {ConnectionString: appfile.Value("env:DB_URL")},
		},
	}

	resolver := DatabaseResolver{LocalProxyPort: 4001}

	// Test 1: Literal Override
	cfg, ok := resolver.Resolve("auth", dummyInfra, mockRealResolver(nil))
	if !ok || cfg.Host != "localhost:5432" || cfg.User != "user" || cfg.Password != "pass" {
		t.Errorf("expected parsed auth db, got %+v", cfg)
	}

	// Test 2: ENV override
	envs := map[string]string{"DB_URL": "postgres://produser:prodpass@remote:5432/proddb"}
	cfg, ok = resolver.Resolve("envdb", dummyInfra, mockRealResolver(envs))
	if !ok || cfg.Host != "remote:5432" || cfg.User != "produser" {
		t.Errorf("expected ENV override, got %+v", cfg)
	}

	// Test 3: Fallback explicitly throws error when local cluster is nil
	cfg, err := resolver.ResolveWithFallback("missingdb", dummyInfra, mockRealResolver(nil))
	if err == nil {
		t.Errorf("expected error fallback on missing LocalCluster, parsed: %+v", cfg)
	}
}

func TestCacheResolver(t *testing.T) {
	dummyInfra := &appfile.Infra{
		Caches: map[string]appfile.CacheInfra{
			"redis":      {URL: appfile.Value("redis://localhost:6379")},
			"prod_redis": {URL: appfile.Value("env:REDIS_URL")},
		},
	}

	resolver := CacheResolver{}

	// Test 1: Literal Override
	cfg, ok := resolver.Resolve("redis", dummyInfra, mockRealResolver(nil))
	if !ok || cfg.Host != "localhost:6379" || cfg.KeyPrefix != "redis/" {
		t.Errorf("expected parsed redis cache, got %+v", cfg)
	}

	// Test 2: ENV override with Auth
	envs := map[string]string{"REDIS_URL": "redis://user:pass@remote.redis:6380"}
	cfg, ok = resolver.Resolve("prod_redis", dummyInfra, mockRealResolver(envs))
	if !ok || cfg.Host != "remote.redis:6380" || cfg.User != "user" || cfg.Password != "pass" {
		t.Errorf("expected REDIS env override with auth, got %+v", cfg)
	}
}

func TestObjectResolver(t *testing.T) {
	dummyInfra := &appfile.Infra{
		Objects: []appfile.ObjectInfra{
			{
				Provider: "s3",
				S3: &appfile.S3Infra{
					Endpoint:        appfile.Value("http://localhost:4566"),
					Region:          appfile.Value("us-east-1"),
					AccessKeyID:     appfile.Value("env:AWS_KEY"),
					SecretAccessKey: appfile.Value("secret"),
				},
				Buckets: map[string]appfile.BucketInfra{
					"uploads": {Name: appfile.Value("my-uploads")},
				},
			},
		},
	}

	resolver := ObjectResolver{}
	envs := map[string]string{"AWS_KEY": "AKIAIOS"}

	// Test Provider Resolution
	cfg, ok := resolver.ResolveProvider(0, dummyInfra, mockRealResolver(envs))
	if !ok || cfg.Provider != "s3" || *cfg.S3Endpoint != "http://localhost:4566" || cfg.S3Region != "us-east-1" || *cfg.S3AccessKeyID != "AKIAIOS" {
		t.Errorf("expected parsed S3 provider, got %+v", cfg)
	}

	// Test Bucket Resolution
	bktCfg, ok := resolver.ResolveBucket("uploads", dummyInfra, mockRealResolver(envs))
	if !ok || bktCfg.CloudName != "my-uploads" {
		t.Errorf("expected parsed bucket my-uploads, got %+v", bktCfg)
	}
}

func TestNeedsLocalCheck(t *testing.T) {
	dummyInfra := &appfile.Infra{
		Caches: map[string]appfile.CacheInfra{
			"redis": {URL: appfile.Value("redis://localhost")},
		},
		Databases: map[string]appfile.DatabaseInfra{},
	}

	check := NeedsLocalCheck{
		Infra:        dummyInfra,
		ResolveValue: mockRealResolver(nil),
	}

	// Cache 'redis' has an override, so it doesn't need local
	if check.NeedsLocalCache([]string{"redis"}) {
		t.Errorf("expected NeedsLocalCache to false for 'redis' since it is overridden")
	}

	// Cache 'missing' doesn't have an override, so it does need local
	if !check.NeedsLocalCache([]string{"missing"}) {
		t.Errorf("expected NeedsLocalCache to be true for 'missing'")
	}

	// DB 'auth' doesn't have an override, so it needs local
	if !check.NeedsLocalDatabase([]string{"auth"}) {
		t.Errorf("expected NeedsLocalDatabase to be true for 'auth'")
	}
}

func TestPubSubResolver(t *testing.T) {
	dummyInfra := &appfile.Infra{
		PubSub: []appfile.PubSubInfra{
			{
				Provider: "gcp",
				GCP: &appfile.GCPPubSubInfra{
					ProjectID: appfile.Value("env:GCP_PROJECT"),
				},
				Topics: map[string]appfile.TopicInfra{
					"events": {
						Name: "events-topic",
						Subscriptions: map[string]appfile.SubscriptionInfra{
							"emailer": {
								Name: appfile.Value("emailer-sub"),
								PushConfig: &appfile.PushInfra{
									ServiceAccount: appfile.Value("sa@gcp.com"),
								},
							},
						},
					},
				},
			},
		},
	}

	resolver := PubSubResolver{}
	envs := map[string]string{"GCP_PROJECT": "my-project-123"}

	// Test Provider
	cfg, ok := resolver.ResolveProvider(0, dummyInfra, mockRealResolver(envs))
	if !ok || cfg.Provider != "gcp" || cfg.GCPProject != "my-project-123" {
		t.Errorf("expected GCP provider my-project-123, got %+v", cfg)
	}

	// Test Topic
	topicCfg, ok := resolver.ResolveTopic("events", dummyInfra, mockRealResolver(envs))
	if !ok || topicCfg.ProviderName != "events-topic" {
		t.Errorf("expected topic events-topic, got %+v", topicCfg)
	}

	// Test Subscription
	subCfg, ok := resolver.ResolveSubscription("events", "emailer", dummyInfra, mockRealResolver(envs))
	if !ok || subCfg.ProviderName != "emailer-sub" || subCfg.GCPPushSA != "sa@gcp.com" || !subCfg.PushOnly {
		t.Errorf("expected subscription emailer-sub with push config, got %+v", subCfg)
	}
}

func TestResolverMissingConfigs(t *testing.T) {
	dummyInfra := &appfile.Infra{
		PubSub: []appfile.PubSubInfra{
			{Provider: "nsq"}, // Missing NSQ Config
			{
				Provider: "gcp",
				GCP:      &appfile.GCPPubSubInfra{ProjectID: appfile.Value("env:GCP_MISSING")},
			},
		},
		Objects: []appfile.ObjectInfra{
			{Provider: "s3"}, // Missing S3 Block
			{
				Provider: "s3",
				S3:       &appfile.S3Infra{Region: appfile.Value("env:S3_MISSING")},
			},
		},
	}

	pubsubResolver := PubSubResolver{}
	objResolver := ObjectResolver{}
	emptyEnvs := map[string]string{}

	// Expect PubSub: NSQ nil fails
	if cfg, ok := pubsubResolver.ResolveProvider(0, dummyInfra, mockRealResolver(emptyEnvs)); ok {
		t.Errorf("Expected NSQ missing config to fail, got %v", cfg)
	}

	// Expect PubSub: GCP unresolvable project fails
	if cfg, ok := pubsubResolver.ResolveProvider(1, dummyInfra, mockRealResolver(emptyEnvs)); ok {
		t.Errorf("Expected GCP empty string to fail, got %v", cfg)
	}

	// Expect S3: S3 config nil fails
	if cfg, ok := objResolver.ResolveProvider(0, dummyInfra, mockRealResolver(emptyEnvs)); ok {
		t.Errorf("Expected S3 missing config to fail, got %v", cfg)
	}

	// Expect S3: S3 region unresolvable fails
	if cfg, ok := objResolver.ResolveProvider(1, dummyInfra, mockRealResolver(emptyEnvs)); ok {
		t.Errorf("Expected S3 empty string region to fail, got %v", cfg)
	}
}
