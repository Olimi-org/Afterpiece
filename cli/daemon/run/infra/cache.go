package infra

import (
	"fmt"
	"net/url"

	"encore.dev/appruntime/exported/config"
	"encr.dev/cli/daemon/redis"
	"encr.dev/pkg/appfile"
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

func (r CacheResolver) Resolve(name string, infra *appfile.Infra, resolveValue ValueResolver) (CacheConfig, bool) {
	if infra == nil || infra.Caches == nil {
		return CacheConfig{}, false
	}

	cacheInfra, ok := infra.Caches[name]
	if !ok {
		return CacheConfig{}, false
	}

	rawURL, ok := resolveValue(cacheInfra.URL)
	if !ok {
		return CacheConfig{}, false
	}

	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return CacheConfig{}, false
	}

	cfg := CacheConfig{
		Host:       u.Host,
		EncoreName: name,
		KeyPrefix:  name + "/",
	}

	if u.User != nil {
		cfg.User = u.User.Username()
		cfg.Password, _ = u.User.Password()
	}

	return cfg, true
}

func (r CacheResolver) ResolveWithFallback(name string, infra *appfile.Infra, resolveValue ValueResolver) (CacheConfig, error) {
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
