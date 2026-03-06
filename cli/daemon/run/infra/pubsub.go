package infra

import (
	"fmt"
	"strings"

	"encore.dev/appruntime/exported/config"
	configinfra "encore.dev/appruntime/exported/config/infra"
)

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
	Provider     string
	ProviderID   int
	EncoreName   string
	ProviderName string
	GCPProjectID string
}

// PubSubSubscriptionConfig represents resolved PubSub subscription configuration.
type PubSubSubscriptionConfig struct {
	Provider        string
	ID              string
	EncoreName      string
	ProviderName    string
	PushOnly        bool
	GCPProjectID    string
	GCPPushSA       string
	GCPPushAudience string
}

// PubSubResolver resolves PubSub provider and topic configurations.
type PubSubResolver struct {
	LocalNSQ *NSQProvider
}

type NSQProvider struct {
	Host string
}

func (r PubSubResolver) ResolveProvider(providerIdx int, infra *configinfra.InfraConfig, resolveValue ValueResolver) (PubSubProviderConfig, bool) {
	if infra == nil || providerIdx < 0 || providerIdx >= len(infra.PubSub) {
		return PubSubProviderConfig{}, false
	}

	provider := infra.PubSub[providerIdx]
	cfg := PubSubProviderConfig{Provider: provider.Type}

	for _, matcher := range pubSubMatchers {
		if matcher.Match(provider.Type) {
			if ok := matcher.ResolveProvider(provider, resolveValue, &cfg); !ok {
				return PubSubProviderConfig{}, false
			}
			return cfg, true
		}
	}

	return PubSubProviderConfig{}, false
}

func (r PubSubResolver) ResolveTopic(topicName string, infra *configinfra.InfraConfig, resolveValue ValueResolver) (PubSubTopicConfig, bool) {
	if infra == nil {
		return PubSubTopicConfig{}, false
	}

	for providerIdx, provider := range infra.PubSub {
		topics := provider.GetTopics()
		if topic, ok := topics[topicName]; ok {
			providerName := topicName
			if gcpTopic, isGcp := topic.(*configinfra.GCPTopic); isGcp && gcpTopic.Name != "" {
				providerName = gcpTopic.Name
			} else if nsqTopic, isNsq := topic.(*configinfra.NSQTopic); isNsq && nsqTopic.Name != "" {
				providerName = nsqTopic.Name
			} else if awsTopic, isAws := topic.(*configinfra.AWSTopic); isAws && awsTopic.ARN != "" {
				providerName = awsTopic.ARN
			}

			cfg := PubSubTopicConfig{
				Provider:     provider.Type,
				ProviderID:   providerIdx,
				EncoreName:   topicName,
				ProviderName: providerName,
			}

			for _, matcher := range pubSubMatchers {
				if matcher.Match(provider.Type) {
					matcher.ResolveTopic(topic, provider, resolveValue, &cfg)
					break
				}
			}

			return cfg, true
		}
	}

	return PubSubTopicConfig{}, false
}

func (r PubSubResolver) ResolveSubscription(topicName, subName string, infra *configinfra.InfraConfig, resolveValue ValueResolver) (PubSubSubscriptionConfig, bool) {
	if infra == nil {
		return PubSubSubscriptionConfig{}, false
	}

	for _, provider := range infra.PubSub {
		topics := provider.GetTopics()
		if topic, ok := topics[topicName]; ok {
			subs := topic.GetSubscriptions()
			if sub, ok := subs[subName]; ok {
				cfg := PubSubSubscriptionConfig{
					Provider:   provider.Type,
					EncoreName: subName,
				}

				envName := configinfra.EnvString{Str: subName}
				pushOnly := false
				if gcpSub, isGcp := sub.(*configinfra.GCPSub); isGcp {
					if gcpSub.Name != "" {
						envName = configinfra.EnvString{Str: gcpSub.Name}
					}
					pushOnly = gcpSub.PushConfig != nil
				} else if nsqSub, isNsq := sub.(*configinfra.NSQSub); isNsq && nsqSub.Name != "" {
					envName = configinfra.EnvString{Str: nsqSub.Name}
				} else if awsSub, isAws := sub.(*configinfra.AWSSub); isAws && awsSub.URL.Value() != "" {
					envName = awsSub.URL
				}

				cfg.PushOnly = pushOnly
				resolvedName, ok := resolveValue(envName)
				if !ok {
					return PubSubSubscriptionConfig{}, false
				}
				cfg.ProviderName = resolvedName
				cfg.ID = resolvedName

				for _, matcher := range pubSubMatchers {
					if matcher.Match(provider.Type) {
						if ok := matcher.ResolveSubscription(sub, provider, resolveValue, &cfg); !ok {
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

func (r PubSubResolver) ResolveWithFallback(infra *configinfra.InfraConfig, resolveValue ValueResolver) ([]PubSubProviderConfig, error) {
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

// ToConfigPubsubProvider converts PubSubProviderConfig to config.PubsubProvider.
func ToConfigPubsubProvider(cfg PubSubProviderConfig) config.PubsubProvider {
	for _, matcher := range pubSubMatchers {
		if matcher.Match(cfg.Provider) {
			return matcher.ToConfigProvider(cfg)
		}
	}
	return config.PubsubProvider{}
}

// ToConfigPubsubTopic converts PubSubTopicConfig to config.PubsubTopic.
func ToConfigPubsubTopic(cfg PubSubTopicConfig) config.PubsubTopic {
	for _, matcher := range pubSubMatchers {
		if matcher.Match(cfg.Provider) {
			return matcher.ToConfigTopic(cfg)
		}
	}
	return config.PubsubTopic{}
}

// ToConfigPubsubSubscription converts PubSubSubscriptionConfig to config.PubsubSubscription.
func ToConfigPubsubSubscription(cfg PubSubSubscriptionConfig) config.PubsubSubscription {
	for _, matcher := range pubSubMatchers {
		if matcher.Match(cfg.Provider) {
			return matcher.ToConfigSubscription(cfg)
		}
	}
	return config.PubsubSubscription{}
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
