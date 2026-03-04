package infra

import (
	"encr.dev/pkg/appfile"
)

// ValueResolver resolves appfile.Value instances.
type ValueResolver func(appfile.Value) (string, bool)

// ResourceResolver resolves infrastructure resources from config.
type ResourceResolver[T any] interface {
	Resolve(name string, infra *appfile.Infra, resolveValue ValueResolver) (T, bool)
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
