package infra

import (
	configinfra "encore.dev/appruntime/exported/config/infra"
)

// ValueResolver resolves configinfra.EnvString instances.
type ValueResolver func(configinfra.EnvString) (string, bool)

// ResourceResolver resolves infrastructure resources from config.
type ResourceResolver[T any] interface {
	Resolve(name string, infra *configinfra.InfraConfig, resolveValue ValueResolver) (T, bool)
}

// NeedsLocalCheck provides methods to determine if local services are needed.
type NeedsLocalCheck struct {
	Infra        *configinfra.InfraConfig
	ResolveValue ValueResolver
}

func (c NeedsLocalCheck) NeedsLocalDatabase(dbNames []string) bool {
	if c.Infra == nil || len(c.Infra.SQLServers) == 0 {
		return true
	}

	for _, name := range dbNames {
		found := false
		for _, srv := range c.Infra.SQLServers {
			if srv.Databases != nil {
				if db, ok := srv.Databases[name]; ok {
					if _, usrOk := c.ResolveValue(db.Username); usrOk {
						found = true
						break
					}
				}
			}
		}
		if !found {
			return true
		}
	}

	return false
}

func (c NeedsLocalCheck) NeedsLocalCache(cacheNames []string) bool {
	if c.Infra == nil || c.Infra.Redis == nil {
		return true
	}

	for _, name := range cacheNames {
		if cache, ok := c.Infra.Redis[name]; !ok {
			return true
		} else if cache.Auth != nil {
			if cache.Auth.AuthString != nil {
				if _, ok := c.ResolveValue(*cache.Auth.AuthString); !ok {
					return true
				}
			} else if cache.Auth.Password != nil {
				if _, ok := c.ResolveValue(*cache.Auth.Password); !ok {
					return true
				}
			}
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
			if matcher.Match(provider.Type) {
				matched = true
				if matcher.NeedsLocal(provider, c.ResolveValue) {
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
	if c.Infra == nil || len(c.Infra.ObjectStorage) == 0 {
		return true
	}

	for _, provider := range c.Infra.ObjectStorage {
		matched := false
		for _, matcher := range objectMatchers {
			if matcher.Match(provider.Type) {
				matched = true
				if matcher.NeedsLocal(provider, c.ResolveValue) {
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
func AutoAssignTopic(topicName string, infra *configinfra.InfraConfig) (providerIdx int, found bool) {
	if infra == nil || len(infra.PubSub) == 0 {
		return -1, false
	}

	for i, provider := range infra.PubSub {
		topics := provider.GetTopics()
		if _, ok := topics[topicName]; ok {
			return i, true
		}
	}

	if len(infra.PubSub) == 1 {
		return 0, true
	}

	return -1, false
}

// AutoAssignBucket assigns a bucket to a provider if only one provider exists.
func AutoAssignBucket(bucketName string, infra *configinfra.InfraConfig) (providerIdx int, found bool) {
	if infra == nil || len(infra.ObjectStorage) == 0 {
		return -1, false
	}

	for i, provider := range infra.ObjectStorage {
		buckets := provider.GetBuckets()
		if _, ok := buckets[bucketName]; ok {
			return i, true
		}
	}

	if len(infra.ObjectStorage) == 1 {
		return 0, true
	}

	return -1, false
}
