package infra

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"encore.dev/appruntime/exported/config"
	"encr.dev/cli/daemon/apps"
	"encr.dev/cli/daemon/namespace"
	"encr.dev/cli/daemon/objects"
	"encr.dev/cli/daemon/pubsub"
	"encr.dev/cli/daemon/redis"
	"encr.dev/cli/daemon/sqldb"
	"encr.dev/internal/optracker"
	"encr.dev/pkg/appfile"
	"encr.dev/pkg/environ"
	meta "encr.dev/proto/afterpiece/parser/meta/v1"
)

type Type string

const (
	PubSub  Type = "pubsub"
	Cache   Type = "cache"
	SQLDB   Type = "sqldb"
	Objects Type = "objects"
)

const (
	// these IDs are used in the Encore Cloud README file as an example
	// on how to create a topic resource
	encoreCloudExampleTopicID        = "res_0o9ioqnrirflhhm3t720"
	encoreCloudExampleSubscriptionID = "res_0o9ioqnrirflhhm3t730"
)

// ResourceManager manages a set of infrastructure resources
// to support the running Encore application.
type ResourceManager struct {
	app           *apps.Instance
	dbProxyPort   int
	sqlMgr        *sqldb.ClusterManager
	objectsMgr    *objects.ClusterManager
	publicBuckets *objects.PublicBucketServer
	ns            *namespace.Namespace
	environ       environ.Environ
	log           zerolog.Logger
	forTests      bool

	mutex   sync.Mutex
	servers map[Type]Resource

	infraConfigs *appfile.Infra

	// Cached resolvers
	databaseResolver DatabaseResolver
	cacheResolver    CacheResolver
	pubsubResolver   PubSubResolver
	objectResolver   ObjectResolver
}

func NewResourceManager(app *apps.Instance, sqlMgr *sqldb.ClusterManager, objectsMgr *objects.ClusterManager, publicBuckets *objects.PublicBucketServer, ns *namespace.Namespace, environ environ.Environ, infraConfigs *appfile.Infra, dbProxyPort int, forTests bool) *ResourceManager {
	rm := &ResourceManager{
		app:           app,
		dbProxyPort:   dbProxyPort,
		sqlMgr:        sqlMgr,
		objectsMgr:    objectsMgr,
		publicBuckets: publicBuckets,
		ns:            ns,
		environ:       environ,
		forTests:      forTests,
		infraConfigs:  infraConfigs,

		servers: make(map[Type]Resource),
		log:     log.With().Str("app_id", app.PlatformOrLocalID()).Logger(),

		// Initialize cached resolvers
		databaseResolver: DatabaseResolver{
			LocalProxyPort: dbProxyPort,
		},
		cacheResolver:  CacheResolver{},
		pubsubResolver: PubSubResolver{},
		objectResolver: ObjectResolver{},
	}
	return rm
}

func (rm *ResourceManager) StopAll() {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	rm.log.Info().Int("num", len(rm.servers)).Msg("Stopping all resource services")

	for _, daemon := range rm.servers {
		daemon.Stop()
	}
}

type Resource interface {
	// Stop shuts down the resource.
	Stop()
}

func (rm *ResourceManager) resolveValue(v appfile.Value) (string, bool) {
	return v.Resolve(rm.environ.Get)
}

func (rm *ResourceManager) needsLocalCheck() NeedsLocalCheck {
	return NeedsLocalCheck{
		Infra:        rm.infraConfigs,
		ResolveValue: rm.resolveValue,
	}
}

func (rm *ResourceManager) StartRequiredServices(a *optracker.AsyncBuildJobs, md *meta.Data) {
	if sqldb.IsUsed(md) && rm.GetSQLCluster() == nil {
		dbNames := make([]string, len(md.SqlDatabases))
		for i, db := range md.SqlDatabases {
			dbNames[i] = db.Name
		}
		if rm.needsLocalCheck().NeedsLocalDatabase(dbNames) {
			a.Go("Creating PostgreSQL database cluster", true, 300*time.Millisecond, rm.StartSQLCluster(a, md))
		}
	}

	if pubsub.IsUsed(md) && rm.GetPubSub() == nil {
		if rm.needsLocalCheck().NeedsLocalPubSub() {
			a.Go("Starting PubSub daemon", true, 250*time.Millisecond, rm.StartPubSub)
		}
	}

	if redis.IsUsed(md) && rm.GetRedis() == nil {
		cacheNames := make([]string, len(md.CacheClusters))
		for i, cache := range md.CacheClusters {
			cacheNames[i] = cache.Name
		}
		if rm.needsLocalCheck().NeedsLocalCache(cacheNames) {
			a.Go("Starting Redis server", true, 250*time.Millisecond, rm.StartRedis)
		}
	}

	if objects.IsUsed(md) && rm.GetObjects() == nil {
		if rm.needsLocalCheck().NeedsLocalObjects() {
			a.Go("Starting Object Storage (Local Emulator)", true, 250*time.Millisecond, rm.StartObjects(md))
		} else {
			a.Go("Configuring Object Storage (External)", true, 10*time.Millisecond, func(ctx context.Context) error {
				rm.log.Info().Msg("Using externally configured object storage")
				return nil
			})
		}
	}
}

// StartPubSub starts a PubSub daemon.
func (rm *ResourceManager) StartPubSub(ctx context.Context) error {
	nsqd := &pubsub.NSQDaemon{}
	err := nsqd.Start()
	if err != nil {
		return err
	}

	rm.mutex.Lock()
	rm.servers[PubSub] = nsqd
	rm.pubsubResolver.LocalNSQ = &NSQProvider{Host: nsqd.Addr()}
	rm.mutex.Unlock()
	return nil
}

// GetPubSub returns the PubSub daemon if it is running otherwise it returns nil
func (rm *ResourceManager) GetPubSub() *pubsub.NSQDaemon {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	if daemon, found := rm.servers[PubSub]; found {
		return daemon.(*pubsub.NSQDaemon)
	}
	return nil
}

// StartRedis starts a Redis server.
func (rm *ResourceManager) StartRedis(ctx context.Context) error {
	srv := redis.New()
	err := srv.Start()
	if err != nil {
		return err
	}

	rm.mutex.Lock()
	rm.servers[Cache] = srv
	rm.cacheResolver.LocalServer = srv
	rm.mutex.Unlock()
	return nil
}

// GetRedis returns the Redis server if it is running otherwise it returns nil
func (rm *ResourceManager) GetRedis() *redis.Server {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	if srv, found := rm.servers[Cache]; found {
		return srv.(*redis.Server)
	}
	return nil
}

// StartObjects starts an Object Storage server.
func (rm *ResourceManager) StartObjects(md *meta.Data) func(context.Context) error {
	return func(ctx context.Context) error {
		var srv *objects.Server
		if rm.forTests {
			srv = objects.NewInMemoryServer(rm.publicBuckets)
		} else {
			if rm.objectsMgr == nil {
				return fmt.Errorf("StartObjects: no Object Storage cluster manager provided")
			} else if rm.publicBuckets == nil {
				return fmt.Errorf("StartObjects: no Object Storage public bucket server provided")
			}
			baseDir, err := rm.objectsMgr.BaseDir(rm.ns.ID)
			if err != nil {
				return err
			}
			srv = objects.NewDirServer(rm.publicBuckets, rm.ns.ID, baseDir)
		}

		if err := srv.Initialize(md); err != nil {
			return err
		} else if err := srv.Start(); err != nil {
			return err
		}

		rm.mutex.Lock()
		rm.servers[Objects] = srv
		rm.objectResolver.LocalObjects = srv
		rm.mutex.Unlock()
		return nil
	}
}

// GetObjects returns the Object Storage server if it is running otherwise it returns nil
func (rm *ResourceManager) GetObjects() *objects.Server {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	if srv, found := rm.servers[Objects]; found {
		return srv.(*objects.Server)
	}
	return nil
}

func (rm *ResourceManager) StartSQLCluster(a *optracker.AsyncBuildJobs, md *meta.Data) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		// This can be the case in tests.
		if rm.sqlMgr == nil {
			return fmt.Errorf("StartSQLCluster: no SQL Cluster manager provided")
		}

		typ := sqldb.Run
		if rm.forTests {
			typ = sqldb.Test
		}

		if err := rm.sqlMgr.Ready(); err != nil {
			return err
		}

		cluster := rm.sqlMgr.Create(ctx, &sqldb.CreateParams{
			ClusterID: sqldb.GetClusterID(rm.app, typ, rm.ns),
			Memfs:     typ.Memfs(),
		})

		if _, err := cluster.Start(ctx, a.Tracker()); err != nil {
			return errors.Wrap(err, "failed to start cluster")
		}

		rm.mutex.Lock()
		rm.servers[SQLDB] = cluster
		rm.databaseResolver.LocalCluster = cluster
		rm.mutex.Unlock()

		// Set up the database asynchronously since it can take a while.
		if rm.forTests {
			a.Go("Recreating databases", true, 250*time.Millisecond, func(ctx context.Context) error {
				err := cluster.Recreate(ctx, rm.app.Root(), nil, md)
				if err != nil {
					rm.log.Error().Err(err).Msg("failed to recreate db")
					return err
				}
				return nil
			})
		} else {
			a.Go("Running database migrations", true, 250*time.Millisecond, func(ctx context.Context) error {
				err := cluster.SetupAndMigrate(ctx, rm.app.Root(), md.SqlDatabases)
				if err != nil {
					rm.log.Error().Err(err).Msg("failed to setup db")
					return err
				}
				return nil
			})
		}

		return nil
	}
}

// GetSQLCluster returns the SQL cluster
func (rm *ResourceManager) GetSQLCluster() *sqldb.Cluster {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	if cluster, found := rm.servers[SQLDB]; found {
		return cluster.(*sqldb.Cluster)
	}
	return nil
}

// UpdateConfig updates the given config with infrastructure information.
// Note that all the requisite services must have started up already,
// which in practice means that (*optracker.AsyncBuildJobs).Wait must have returned first.
func (rm *ResourceManager) UpdateConfig(cfg *config.Runtime, md *meta.Data, dbProxyPort int) error {
	useLocalEncoreCloudAPIForTesting, err := rm.setTestEncoreCloud(cfg)
	if err != nil {
		return err
	}

	if cluster := rm.GetSQLCluster(); cluster != nil {
		srv := &config.SQLServer{
			Host: "localhost:" + fmt.Sprintf("%d", dbProxyPort),
		}
		serverID := len(cfg.SQLServers)
		cfg.SQLServers = append(cfg.SQLServers, srv)

		for _, db := range md.SqlDatabases {
			cfg.SQLDatabases = append(cfg.SQLDatabases, &config.SQLDatabase{
				ServerID:     serverID,
				EncoreName:   db.Name,
				DatabaseName: db.Name,
				User:         "encore",
				Password:     cluster.Password,
			})
		}

		// Configure max connections based on 96 connections
		// divided evenly among the databases
		maxConns := 96 / len(cfg.SQLDatabases)
		for _, db := range cfg.SQLDatabases {
			db.MaxConnections = maxConns
		}
	}

	if nsq := rm.GetPubSub(); nsq != nil {
		provider := &config.PubsubProvider{
			NSQ: &config.NSQProvider{
				Host: nsq.Addr(),
			},
		}
		providerID := len(cfg.PubsubProviders)
		cfg.PubsubProviders = append(cfg.PubsubProviders, provider)

		// If we're testing the Encore Cloud API locally, override from NSQ
		if useLocalEncoreCloudAPIForTesting {
			providerID = len(cfg.PubsubProviders)
			cfg.PubsubProviders = append(cfg.PubsubProviders, &config.PubsubProvider{
				EncoreCloud: &config.EncoreCloudPubsubProvider{},
			})
		}

		cfg.PubsubTopics = make(map[string]*config.PubsubTopic)
		for _, t := range md.PubsubTopics {
			providerName := t.Name
			if useLocalEncoreCloudAPIForTesting {
				providerName = encoreCloudExampleTopicID
			}

			topicCfg := &config.PubsubTopic{
				ProviderID:    providerID,
				EncoreName:    t.Name,
				ProviderName:  providerName,
				Subscriptions: make(map[string]*config.PubsubSubscription),
			}

			for _, s := range t.Subscriptions {
				subscriptionID := t.Name
				if useLocalEncoreCloudAPIForTesting {
					subscriptionID = encoreCloudExampleSubscriptionID
				}

				topicCfg.Subscriptions[s.Name] = &config.PubsubSubscription{
					ID:           subscriptionID,
					EncoreName:   s.Name,
					ProviderName: s.Name,
				}
			}

			cfg.PubsubTopics[t.Name] = topicCfg
		}
	}

	if redis := rm.GetRedis(); redis != nil {
		srv := &config.RedisServer{
			Host: redis.Addr(),
		}
		serverID := len(cfg.RedisServers)
		cfg.RedisServers = append(cfg.RedisServers, srv)

		for _, cluster := range md.CacheClusters {
			cfg.RedisDatabases = append(cfg.RedisDatabases, &config.RedisDatabase{
				ServerID:   serverID,
				Database:   0,
				EncoreName: cluster.Name,
				KeyPrefix:  cluster.Name + "/",
			})
		}
	}

	return nil
}

func (rm *ResourceManager) SQLConfig(db *meta.SQLDatabase) (config.SQLServer, config.SQLDatabase, error) {
	cfg, err := rm.databaseResolver.ResolveWithFallback(db.Name, rm.infraConfigs, rm.resolveValue)
	if err != nil {
		return config.SQLServer{}, config.SQLDatabase{}, err
	}
	return ToConfigSQLServer(cfg), ToConfigSQLDatabase(cfg), nil
}

func (rm *ResourceManager) PubSubProviderConfig() (config.PubsubProvider, error) {
	nsq := rm.GetPubSub()
	rm.mutex.Lock()
	if nsq != nil && rm.pubsubResolver.LocalNSQ == nil {
		rm.pubsubResolver.LocalNSQ = &NSQProvider{Host: nsq.Addr()}
	}
	rm.mutex.Unlock()

	providers, err := rm.pubsubResolver.ResolveWithFallback(rm.infraConfigs, rm.resolveValue)
	if err != nil {
		return config.PubsubProvider{}, err
	}

	if len(providers) == 0 {
		return config.PubsubProvider{}, errors.New("no PubSub provider available")
	}

	return ToConfigPubsubProvider(providers[0]), nil
}

// PubSubTopicConfig returns the PubSub provider and topic configuration for the given topic.
func (rm *ResourceManager) PubSubTopicConfig(topic *meta.PubSubTopic) (config.PubsubProvider, config.PubsubTopic, error) {
	providerCfg, err := rm.PubSubProviderConfig()
	if err != nil {
		return config.PubsubProvider{}, config.PubsubTopic{}, err
	}

	topicCfg, ok := rm.pubsubResolver.ResolveTopic(topic.Name, rm.infraConfigs, rm.resolveValue)
	if !ok {
		providerIdx := 0
		if rm.infraConfigs != nil && len(rm.infraConfigs.PubSub) > 0 {
			providerIdx = GetDefaultProviderIndex(rm.infraConfigs.PubSub)
		}
		topicCfg = PubSubTopicConfig{
			ProviderID:   providerIdx,
			EncoreName:   topic.Name,
			ProviderName: EnsureValidNSQName(topic.Name),
		}
	}

	return providerCfg, ToConfigPubsubTopic(topicCfg), nil
}

func (rm *ResourceManager) PubSubSubscriptionConfig(topic *meta.PubSubTopic, sub *meta.PubSubTopic_Subscription) (config.PubsubSubscription, error) {
	subCfg, ok := rm.pubsubResolver.ResolveSubscription(topic.Name, sub.Name, rm.infraConfigs, rm.resolveValue)
	if !ok {
		subCfg = PubSubSubscriptionConfig{
			ID:           sub.Name,
			EncoreName:   sub.Name,
			ProviderName: EnsureValidNSQName(sub.Name),
		}
	}
	return ToConfigPubsubSubscription(subCfg), nil
}

func (rm *ResourceManager) RedisConfig(redisCluster *meta.CacheCluster) (config.RedisServer, config.RedisDatabase, error) {
	cfg, err := rm.cacheResolver.ResolveWithFallback(redisCluster.Name, rm.infraConfigs, rm.resolveValue)
	if err != nil {
		return config.RedisServer{}, config.RedisDatabase{}, err
	}
	return ToConfigRedisServer(cfg), ToConfigRedisDatabase(cfg), nil
}

func (rm *ResourceManager) BucketProviderConfig() (config.BucketProvider, string, error) {
	providers, err := rm.objectResolver.ResolveWithFallback(rm.infraConfigs, rm.resolveValue)
	if err != nil {
		return config.BucketProvider{}, "", err
	}

	if len(providers) == 0 {
		return config.BucketProvider{}, "", errors.New("no object storage provider available")
	}

	cfg := providers[0]
	provider := ToConfigBucketProvider(cfg)

	publicURL := ""
	if cfg.Provider == "gcs" && cfg.GCSEndpoint != "" {
		publicURL = cfg.GCSEndpoint
	} else if cfg.Provider == "s3" && cfg.S3Endpoint != nil {
		publicURL = *cfg.S3Endpoint
	}

	return provider, publicURL, nil
}
