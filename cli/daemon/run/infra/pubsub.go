package infra

import (
	"fmt"
	"strings"

	"encore.dev/appruntime/exported/config"
	"encr.dev/pkg/appfile"
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
				Provider:     provider.Provider,
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
					Provider:   provider.Provider,
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
