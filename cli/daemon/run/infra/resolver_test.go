package infra

import (
	"testing"

	configinfra "encore.dev/appruntime/exported/config/infra"
	"encr.dev/cli/daemon/sqldb"
)

func mockResolver(val string, ok bool) ValueResolver {
	return func(v configinfra.EnvString) (string, bool) {
		return val, ok
	}
}

func mockRealResolver(envs map[string]string) ValueResolver {
	return func(v configinfra.EnvString) (string, bool) {
		if v.Env != nil {
			val, ok := envs[v.Env.Env]
			return val, ok
		}
		s := v.Str
		return s, s != ""
	}
}

func TestDatabaseResolver(t *testing.T) {
	dummyInfra := &configinfra.InfraConfig{
		SQLServers: []*configinfra.SQLServer{
			{
				Host: "localhost:5432",
				Databases: map[string]*configinfra.SQLDatabase{
					"auth": {
						Name:     "auth",
						Username: configinfra.EnvString{Str: "user"},
						Password: configinfra.EnvString{Str: "pass"},
					},
					"envdb": {
						Name:     "envdb",
						Username: configinfra.EnvString{Str: "produser"},
						Password: configinfra.EnvString{Env: &configinfra.EnvRef{Env: "DB_PASS"}},
					},
				},
			},
		},
	}

	resolver := DatabaseResolver{LocalProxyPort: 4001}

	// Test 1: Literal Override
	cfg, ok := resolver.Resolve("auth", dummyInfra, mockRealResolver(nil))
	if !ok || cfg.Host != "localhost:5432" || cfg.User != "user" || cfg.Password != "pass" {
		t.Errorf("expected parsed auth db, got %+v", cfg)
	}

	// Test 2: ENV override
	envs := map[string]string{"DB_PASS": "prodpass"}
	cfg, ok = resolver.Resolve("envdb", dummyInfra, mockRealResolver(envs))
	if !ok || cfg.Host != "localhost:5432" || cfg.Password != "prodpass" || cfg.User != "produser" {
		t.Errorf("expected ENV override, got %+v", cfg)
	}

	// Test 3: Fallback explicitly throws error when local cluster is nil
	cfg, err := resolver.ResolveWithFallback("missingdb", dummyInfra, mockRealResolver(nil))
	if err == nil {
		t.Errorf("expected error fallback on missing LocalCluster, parsed: %+v", cfg)
	}

	// Test 4: Fallback works when local cluster exists
	resolver.LocalCluster = &sqldb.Cluster{Password: "localpass"}
	cfg, err = resolver.ResolveWithFallback("missingdb", dummyInfra, mockRealResolver(nil))
	if err != nil || cfg.Password != "localpass" || cfg.Database != "missingdb" {
		t.Errorf("expected localfallback to succeed, got %+v, %v", cfg, err)
	}
}

func TestCacheResolver(t *testing.T) {
	dummyInfra := &configinfra.InfraConfig{
		Redis: map[string]*configinfra.Redis{
			"redis": {
				Host: "localhost:6379",
				Auth: &configinfra.RedisAuth{
					Type:     "acl",
					Username: &configinfra.EnvString{Str: "user"},
					Password: &configinfra.EnvString{Str: "pass"},
				},
			},
			"prod_redis": {
				Host: "remote.redis:6380",
				Auth: &configinfra.RedisAuth{
					Type:       "auth_string",
					AuthString: &configinfra.EnvString{Env: &configinfra.EnvRef{Env: "REDIS_PASS"}},
				},
			},
		},
	}

	resolver := CacheResolver{}

	// Test 1: Literal Override
	cfg, ok := resolver.Resolve("redis", dummyInfra, mockRealResolver(nil))
	if !ok || cfg.Host != "localhost:6379" || cfg.KeyPrefix != "redis/" || cfg.User != "user" || cfg.Password != "pass" {
		t.Errorf("expected parsed redis cache, got %+v", cfg)
	}

	// Test 2: ENV override with Auth
	envs := map[string]string{"REDIS_PASS": "envpass"}
	cfg, ok = resolver.Resolve("prod_redis", dummyInfra, mockRealResolver(envs))
	if !ok || cfg.Host != "remote.redis:6380" || cfg.Password != "envpass" {
		t.Errorf("expected REDIS env override with auth, got %+v", cfg)
	}
}

func TestObjectResolver(t *testing.T) {
	dummyInfra := &configinfra.InfraConfig{
		ObjectStorage: []*configinfra.ObjectStorage{
			{
				Type: "s3",
				S3: &configinfra.S3{
					Endpoint:        configinfra.EnvString{Str: "http://localhost:4566"},
					Region:          configinfra.EnvString{Str: "us-east-1"},
					AccessKeyID:     configinfra.EnvString{Env: &configinfra.EnvRef{Env: "AWS_KEY"}},
					SecretAccessKey: configinfra.EnvString{Str: "secret"},
					Buckets: map[string]*configinfra.Bucket{
						"uploads": {Name: configinfra.EnvString{Str: "my-uploads"}},
					},
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
	dummyInfra := &configinfra.InfraConfig{
		Redis: map[string]*configinfra.Redis{
			"redis": {Host: "localhost"},
		},
		SQLServers: []*configinfra.SQLServer{},
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
	dummyInfra := &configinfra.InfraConfig{
		PubSub: []*configinfra.PubSub{
			{
				Type: "gcp_pubsub",
				GCP: &configinfra.GCPPubsub{
					ProjectID: configinfra.EnvString{Env: &configinfra.EnvRef{Env: "GCP_PROJECT"}},
					Topics: map[string]*configinfra.GCPTopic{
						"events": {
							Name: "events-topic",
							Subscriptions: map[string]*configinfra.GCPSub{
								"emailer": {
									Name: "emailer-sub",
									PushConfig: &configinfra.PushConfig{
										ServiceAccount: configinfra.EnvString{Str: "sa@gcp.com"},
									},
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
	if !ok || cfg.Provider != "gcp_pubsub" || cfg.GCPProject != "my-project-123" {
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
	dummyInfra := &configinfra.InfraConfig{
		PubSub: []*configinfra.PubSub{
			{Type: "nsq"}, // Missing NSQ Config
			{
				Type: "gcp_pubsub",
				GCP:  &configinfra.GCPPubsub{ProjectID: configinfra.EnvString{Env: &configinfra.EnvRef{Env: "GCP_MISSING"}}},
			},
		},
		ObjectStorage: []*configinfra.ObjectStorage{
			{Type: "s3"}, // Missing S3 Block
			{
				Type: "s3",
				S3:   &configinfra.S3{Region: configinfra.EnvString{Env: &configinfra.EnvRef{Env: "S3_MISSING"}}},
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
