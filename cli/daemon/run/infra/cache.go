package infra

import (
	"fmt"

	"encore.dev/appruntime/exported/config"
	configinfra "encore.dev/appruntime/exported/config/infra"
	"encr.dev/cli/daemon/redis"
)

// CacheConfig represents resolved cache configuration.
type CacheConfig struct {
	Host       string
	User       string
	Password   string
	EncoreName string
	KeyPrefix  string
}

// CacheResolver resolves Redis cache configurations.
type CacheResolver struct {
	LocalServer *redis.Server
}

func (r CacheResolver) Resolve(name string, infra *configinfra.InfraConfig, resolveValue ValueResolver) (CacheConfig, bool) {
	if infra == nil || infra.Redis == nil {
		return CacheConfig{}, false
	}

	cache, ok := infra.Redis[name]
	if !ok {
		return CacheConfig{}, false
	}

	cfg := CacheConfig{
		Host:       cache.Host,
		EncoreName: name,
		KeyPrefix:  name + "/",
	}

	if cache.Auth != nil {
		if cache.Auth.AuthString != nil {
			if pass, ok := resolveValue(*cache.Auth.AuthString); ok {
				cfg.Password = pass
			} else {
				return CacheConfig{}, false
			}
		} else if cache.Auth.Password != nil {
			if pass, ok := resolveValue(*cache.Auth.Password); ok {
				cfg.Password = pass
			} else {
				return CacheConfig{}, false
			}
			if cache.Auth.Username != nil {
				if user, ok := resolveValue(*cache.Auth.Username); ok {
					cfg.User = user
				}
			}
		}
	}

	return cfg, true
}

func (r CacheResolver) ResolveWithFallback(name string, infra *configinfra.InfraConfig, resolveValue ValueResolver) (CacheConfig, error) {
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
