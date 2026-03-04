package run

import (
	"testing"

	"encore.dev/appruntime/exported/config"
	"encr.dev/pkg/appfile"
	meta "encr.dev/proto/afterpiece/parser/meta/v1"
)

type mockInfraManager struct {
	bkt config.BucketProvider
}

func (m mockInfraManager) SQLConfig(db *meta.SQLDatabase) (config.SQLServer, config.SQLDatabase, error) {
	return config.SQLServer{}, config.SQLDatabase{}, nil
}

func (m mockInfraManager) PubSubProviderConfig() (config.PubsubProvider, error) {
	return config.PubsubProvider{}, nil
}

func (m mockInfraManager) PubSubTopicConfig(topic *meta.PubSubTopic) (config.PubsubProvider, config.PubsubTopic, error) {
	return config.PubsubProvider{}, config.PubsubTopic{}, nil
}

func (m mockInfraManager) PubSubSubscriptionConfig(topic *meta.PubSubTopic, sub *meta.PubSubTopic_Subscription) (config.PubsubSubscription, error) {
	return config.PubsubSubscription{}, nil
}

func (m mockInfraManager) RedisConfig(redis *meta.CacheCluster) (config.RedisServer, config.RedisDatabase, error) {
	return config.RedisServer{}, config.RedisDatabase{}, nil
}

func (m mockInfraManager) BucketProviderConfig() (config.BucketProvider, string, error) {
	return m.bkt, "", nil
}

type mockApp struct{}

func (mockApp) PlatformID() string                    { return "test" }
func (mockApp) PlatformOrLocalID() string             { return "test" }
func (mockApp) GlobalCORS() (appfile.CORS, error)     { return appfile.CORS{}, nil }
func (mockApp) AppFile() (*appfile.File, error)       { return &appfile.File{}, nil }
func (mockApp) BuildSettings() (appfile.Build, error) { return appfile.Build{}, nil }

func TestBucketValidation(t *testing.T) {
	gen := &RuntimeConfigGenerator{
		app: mockApp{},
		md: &meta.Data{
			Buckets: []*meta.Bucket{{Name: "b1"}},
		},
		infraManager: mockInfraManager{
			bkt: config.BucketProvider{}, // Neither S3 nor GCS set
		},
	}

	err := gen.initialize()
	if err == nil {
		t.Errorf("Expected initialization to fail due to missing bucket providers")
	} else if err.Error() != "invalid bucket provider config: set exactly one of S3 or GCS" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestPubSubValidation(t *testing.T) {
	gen := &RuntimeConfigGenerator{
		app: mockApp{},
		md: &meta.Data{
			PubsubTopics: []*meta.PubSubTopic{{Name: "t1"}},
		},
		infraManager: mockInfraManager{}, // Empty PubsubProvider
	}

	err := gen.initialize()
	if err == nil {
		t.Errorf("Expected initialization to fail due to missing pubsub providers")
	} else if err.Error() != "invalid pubsub provider config: no provider set" {
		t.Errorf("Unexpected error: %v", err)
	}
}
