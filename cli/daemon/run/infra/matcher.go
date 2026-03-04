package infra

import (
	"encore.dev/appruntime/exported/config"
	"encr.dev/pkg/appfile"
)

// PubSubMatcher defines the resolution matching interfaces for different Pub/Sub technologies.
type PubSubMatcher interface {
	Match(provider string) bool
	NeedsLocal(provider *appfile.PubSubInfra, resolveValue ValueResolver) bool
	ResolveProvider(provider *appfile.PubSubInfra, resolveValue ValueResolver, cfg *PubSubProviderConfig) bool
	ResolveTopic(topic *appfile.TopicInfra, provider *appfile.PubSubInfra, resolveValue ValueResolver, cfg *PubSubTopicConfig)
	ResolveSubscription(sub *appfile.SubscriptionInfra, provider *appfile.PubSubInfra, resolveValue ValueResolver, cfg *PubSubSubscriptionConfig) bool

	ToConfigProvider(cfg PubSubProviderConfig) config.PubsubProvider
	ToConfigTopic(cfg PubSubTopicConfig) config.PubsubTopic
	ToConfigSubscription(cfg PubSubSubscriptionConfig) config.PubsubSubscription
}

// ObjectMatcher defines the resolution matching interfaces for object storage technologies.
type ObjectMatcher interface {
	Match(provider string) bool
	NeedsLocal(provider *appfile.ObjectInfra, resolveValue ValueResolver) bool
	ResolveProvider(provider *appfile.ObjectInfra, resolveValue ValueResolver, cfg *ObjectProviderConfig) bool

	ToConfigProvider(cfg ObjectProviderConfig) config.BucketProvider
}

// pubSubMatchers contains the available provider matcher logic for PubSub architectures.
var pubSubMatchers = []PubSubMatcher{
	&nsqPubSubMatcher{},
	&gcpPubSubMatcher{},
	&awsPubSubMatcher{},
	&azurePubSubMatcher{},
}

// objectMatchers contains the available provider matcher logic for Object Storage.
var objectMatchers = []ObjectMatcher{
	&s3ObjectMatcher{},
	&gcsObjectMatcher{},
}

// nsqPubSubMatcher
type nsqPubSubMatcher struct{}

func (m *nsqPubSubMatcher) Match(provider string) bool { return provider == "nsq" }
func (m *nsqPubSubMatcher) NeedsLocal(p *appfile.PubSubInfra, res ValueResolver) bool {
	if p.NSQ == nil {
		return true
	}
	_, ok := res(p.NSQ.Hosts)
	return !ok
}
func (m *nsqPubSubMatcher) ResolveProvider(p *appfile.PubSubInfra, res ValueResolver, cfg *PubSubProviderConfig) bool {
	if p.NSQ == nil {
		return false
	}
	hosts, ok := res(p.NSQ.Hosts)
	if !ok || hosts == "" {
		return false
	}
	cfg.NSQHost = hosts
	return true
}
func (m *nsqPubSubMatcher) ResolveTopic(topic *appfile.TopicInfra, provider *appfile.PubSubInfra, resolveValue ValueResolver, cfg *PubSubTopicConfig) {
}
func (m *nsqPubSubMatcher) ResolveSubscription(sub *appfile.SubscriptionInfra, provider *appfile.PubSubInfra, resolveValue ValueResolver, cfg *PubSubSubscriptionConfig) bool {
	return true
}
func (m *nsqPubSubMatcher) ToConfigProvider(cfg PubSubProviderConfig) config.PubsubProvider {
	return config.PubsubProvider{NSQ: &config.NSQProvider{Host: cfg.NSQHost}}
}
func (m *nsqPubSubMatcher) ToConfigTopic(cfg PubSubTopicConfig) config.PubsubTopic {
	return config.PubsubTopic{ProviderID: cfg.ProviderID, EncoreName: cfg.EncoreName, ProviderName: cfg.ProviderName, Subscriptions: make(map[string]*config.PubsubSubscription)}
}
func (m *nsqPubSubMatcher) ToConfigSubscription(cfg PubSubSubscriptionConfig) config.PubsubSubscription {
	return config.PubsubSubscription{ID: cfg.ID, EncoreName: cfg.EncoreName, ProviderName: cfg.ProviderName, PushOnly: cfg.PushOnly}
}

// gcpPubSubMatcher
type gcpPubSubMatcher struct{}

func (m *gcpPubSubMatcher) Match(provider string) bool { return provider == "gcp" }
func (m *gcpPubSubMatcher) NeedsLocal(p *appfile.PubSubInfra, res ValueResolver) bool {
	if p.GCP == nil {
		return true
	}
	_, ok := res(p.GCP.ProjectID)
	return !ok
}
func (m *gcpPubSubMatcher) ResolveProvider(p *appfile.PubSubInfra, res ValueResolver, cfg *PubSubProviderConfig) bool {
	if p.GCP == nil {
		return false
	}
	projectID, ok := res(p.GCP.ProjectID)
	if !ok || projectID == "" {
		return false
	}
	cfg.GCPProject = projectID
	return true
}
func (m *gcpPubSubMatcher) ResolveTopic(topic *appfile.TopicInfra, provider *appfile.PubSubInfra, resolveValue ValueResolver, cfg *PubSubTopicConfig) {
	if topic.ProjectID != "" {
		if pid, ok := resolveValue(topic.ProjectID); ok {
			cfg.GCPProjectID = pid
		}
	} else if provider.GCP != nil {
		if pid, ok := resolveValue(provider.GCP.ProjectID); ok {
			cfg.GCPProjectID = pid
		}
	}
}
func (m *gcpPubSubMatcher) ResolveSubscription(sub *appfile.SubscriptionInfra, provider *appfile.PubSubInfra, resolveValue ValueResolver, cfg *PubSubSubscriptionConfig) bool {
	if sub.ProjectID != "" {
		if pid, ok := resolveValue(sub.ProjectID); ok {
			cfg.GCPProjectID = pid
		}
	} else if provider.GCP != nil {
		if pid, ok := resolveValue(provider.GCP.ProjectID); ok {
			cfg.GCPProjectID = pid
		}
	}
	if sub.PushConfig != nil {
		cfg.PushOnly = true
		if sa, ok := resolveValue(sub.PushConfig.ServiceAccount); ok {
			cfg.GCPPushSA = sa
		}
		if aud, ok := resolveValue(sub.PushConfig.JWTAudience); ok {
			cfg.GCPPushAudience = aud
		}
		if id, ok := resolveValue(sub.PushConfig.ID); ok {
			cfg.ID = id
		}
	}
	return true
}
func (m *gcpPubSubMatcher) ToConfigProvider(cfg PubSubProviderConfig) config.PubsubProvider {
	return config.PubsubProvider{GCP: &config.GCPPubsubProvider{}}
}
func (m *gcpPubSubMatcher) ToConfigTopic(cfg PubSubTopicConfig) config.PubsubTopic {
	topic := config.PubsubTopic{ProviderID: cfg.ProviderID, EncoreName: cfg.EncoreName, ProviderName: cfg.ProviderName, Subscriptions: make(map[string]*config.PubsubSubscription)}
	if cfg.GCPProjectID != "" {
		topic.GCP = &config.PubsubTopicGCPData{ProjectID: cfg.GCPProjectID}
	}
	return topic
}
func (m *gcpPubSubMatcher) ToConfigSubscription(cfg PubSubSubscriptionConfig) config.PubsubSubscription {
	sub := config.PubsubSubscription{ID: cfg.ID, EncoreName: cfg.EncoreName, ProviderName: cfg.ProviderName, PushOnly: cfg.PushOnly}
	if cfg.GCPProjectID != "" || cfg.GCPPushSA != "" {
		sub.GCP = &config.PubsubSubscriptionGCPData{ProjectID: cfg.GCPProjectID, PushServiceAccount: cfg.GCPPushSA}
	}
	return sub
}

// awsPubSubMatcher
type awsPubSubMatcher struct{}

func (m *awsPubSubMatcher) Match(provider string) bool { return provider == "aws" }
func (m *awsPubSubMatcher) NeedsLocal(p *appfile.PubSubInfra, res ValueResolver) bool {
	if p.AWS == nil {
		return true
	}
	_, ok := res(p.AWS.Region)
	return !ok
}
func (m *awsPubSubMatcher) ResolveProvider(p *appfile.PubSubInfra, res ValueResolver, cfg *PubSubProviderConfig) bool {
	if p.AWS == nil {
		return false
	}
	region, ok := res(p.AWS.Region)
	if !ok || region == "" {
		return false
	}
	cfg.AWSRegion = region
	return true
}
func (m *awsPubSubMatcher) ResolveTopic(topic *appfile.TopicInfra, provider *appfile.PubSubInfra, resolveValue ValueResolver, cfg *PubSubTopicConfig) {
}
func (m *awsPubSubMatcher) ResolveSubscription(sub *appfile.SubscriptionInfra, provider *appfile.PubSubInfra, resolveValue ValueResolver, cfg *PubSubSubscriptionConfig) bool {
	if subURL, ok := resolveValue(sub.URL); ok {
		cfg.ProviderName = subURL
	}
	return true
}
func (m *awsPubSubMatcher) ToConfigProvider(cfg PubSubProviderConfig) config.PubsubProvider {
	return config.PubsubProvider{AWS: &config.AWSPubsubProvider{}}
}
func (m *awsPubSubMatcher) ToConfigTopic(cfg PubSubTopicConfig) config.PubsubTopic {
	return config.PubsubTopic{ProviderID: cfg.ProviderID, EncoreName: cfg.EncoreName, ProviderName: cfg.ProviderName, Subscriptions: make(map[string]*config.PubsubSubscription)}
}
func (m *awsPubSubMatcher) ToConfigSubscription(cfg PubSubSubscriptionConfig) config.PubsubSubscription {
	return config.PubsubSubscription{ID: cfg.ID, EncoreName: cfg.EncoreName, ProviderName: cfg.ProviderName, PushOnly: cfg.PushOnly}
}

// azurePubSubMatcher
type azurePubSubMatcher struct{}

func (m *azurePubSubMatcher) Match(provider string) bool { return provider == "azure" }
func (m *azurePubSubMatcher) NeedsLocal(p *appfile.PubSubInfra, res ValueResolver) bool {
	if p.Azure == nil {
		return true
	}
	_, ok := res(p.Azure.Namespace)
	return !ok
}
func (m *azurePubSubMatcher) ResolveProvider(p *appfile.PubSubInfra, res ValueResolver, cfg *PubSubProviderConfig) bool {
	if p.Azure == nil {
		return false
	}
	ns, ok := res(p.Azure.Namespace)
	if !ok || ns == "" {
		return false
	}
	cfg.AzureNS = ns
	return true
}
func (m *azurePubSubMatcher) ResolveTopic(topic *appfile.TopicInfra, provider *appfile.PubSubInfra, resolveValue ValueResolver, cfg *PubSubTopicConfig) {
}
func (m *azurePubSubMatcher) ResolveSubscription(sub *appfile.SubscriptionInfra, provider *appfile.PubSubInfra, resolveValue ValueResolver, cfg *PubSubSubscriptionConfig) bool {
	return true
}
func (m *azurePubSubMatcher) ToConfigProvider(cfg PubSubProviderConfig) config.PubsubProvider {
	return config.PubsubProvider{Azure: &config.AzureServiceBusProvider{Namespace: cfg.AzureNS}}
}
func (m *azurePubSubMatcher) ToConfigTopic(cfg PubSubTopicConfig) config.PubsubTopic {
	return config.PubsubTopic{ProviderID: cfg.ProviderID, EncoreName: cfg.EncoreName, ProviderName: cfg.ProviderName, Subscriptions: make(map[string]*config.PubsubSubscription)}
}
func (m *azurePubSubMatcher) ToConfigSubscription(cfg PubSubSubscriptionConfig) config.PubsubSubscription {
	return config.PubsubSubscription{ID: cfg.ID, EncoreName: cfg.EncoreName, ProviderName: cfg.ProviderName, PushOnly: cfg.PushOnly}
}

// s3ObjectMatcher
type s3ObjectMatcher struct{}

func (m *s3ObjectMatcher) Match(provider string) bool { return provider == "s3" }
func (m *s3ObjectMatcher) NeedsLocal(p *appfile.ObjectInfra, res ValueResolver) bool {
	if p.S3 == nil {
		return true
	}
	_, ok := res(p.S3.Region)
	return !ok
}
func (m *s3ObjectMatcher) ResolveProvider(p *appfile.ObjectInfra, res ValueResolver, cfg *ObjectProviderConfig) bool {
	if p.S3 == nil {
		return false
	}
	if endpoint, ok := res(p.S3.Endpoint); ok {
		cfg.S3Endpoint = &endpoint
	}
	if region, ok := res(p.S3.Region); ok {
		cfg.S3Region = region
	} else {
		return false
	}
	if accessKey, ok := res(p.S3.AccessKeyID); ok {
		cfg.S3AccessKeyID = &accessKey
	}
	if secretKey, ok := res(p.S3.SecretAccessKey); ok {
		cfg.S3SecretKey = &secretKey
	}
	return true
}
func (m *s3ObjectMatcher) ToConfigProvider(cfg ObjectProviderConfig) config.BucketProvider {
	return config.BucketProvider{S3: &config.S3BucketProvider{Region: cfg.S3Region, Endpoint: cfg.S3Endpoint, AccessKeyID: cfg.S3AccessKeyID, SecretAccessKey: cfg.S3SecretKey}}
}

// gcsObjectMatcher
type gcsObjectMatcher struct{}

func (m *gcsObjectMatcher) Match(provider string) bool { return provider == "gcs" }
func (m *gcsObjectMatcher) NeedsLocal(p *appfile.ObjectInfra, res ValueResolver) bool {
	return false // GCS can work without explicit config
}
func (m *gcsObjectMatcher) ResolveProvider(p *appfile.ObjectInfra, res ValueResolver, cfg *ObjectProviderConfig) bool {
	if p.GCS != nil {
		if endpoint, ok := res(p.GCS.Endpoint); ok {
			cfg.GCSEndpoint = endpoint
		}
	}
	return true
}
func (m *gcsObjectMatcher) ToConfigProvider(cfg ObjectProviderConfig) config.BucketProvider {
	return config.BucketProvider{GCS: &config.GCSBucketProvider{Endpoint: cfg.GCSEndpoint}}
}
