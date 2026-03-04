package infra

import (
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"encore.dev/appruntime/exported/config"
	"encr.dev/cli/daemon/sqldb"
	"encr.dev/pkg/appfile"
)

// DatabaseConfig represents resolved database configuration.
type DatabaseConfig struct {
	Host       string
	Database   string
	User       string
	Password   string
	EncoreName string
}

// DatabaseResolver resolves SQL database configurations.
type DatabaseResolver struct {
	LocalCluster   *sqldb.Cluster
	LocalProxyPort int
}

func (r DatabaseResolver) Resolve(name string, infra *appfile.Infra, resolveValue ValueResolver) (DatabaseConfig, bool) {
	if infra == nil || infra.Databases == nil {
		return DatabaseConfig{}, false
	}

	dbInfra, ok := infra.Databases[name]
	if !ok {
		return DatabaseConfig{}, false
	}

	connStr, ok := resolveValue(dbInfra.ConnectionString)
	if !ok {
		return DatabaseConfig{}, false
	}

	pCfg, err := pgx.ParseConfig(connStr)
	if err != nil {
		return DatabaseConfig{}, false
	}

	host := pCfg.Host
	if pCfg.Port != 0 {
		host = fmt.Sprintf("%s:%d", host, pCfg.Port)
	}

	return DatabaseConfig{
		Host:       host,
		Database:   pCfg.Database,
		User:       pCfg.User,
		Password:   pCfg.Password,
		EncoreName: name,
	}, true
}

func (r DatabaseResolver) ResolveWithFallback(name string, infra *appfile.Infra, resolveValue ValueResolver) (DatabaseConfig, error) {
	if cfg, ok := r.Resolve(name, infra, resolveValue); ok {
		return cfg, nil
	}

	if r.LocalCluster == nil {
		return DatabaseConfig{}, fmt.Errorf("no SQL cluster available for database %q", name)
	}

	return DatabaseConfig{
		Host:       "localhost:" + strconv.Itoa(r.LocalProxyPort),
		Database:   name,
		User:       "encore",
		Password:   r.LocalCluster.Password,
		EncoreName: name,
	}, nil
}

// ToConfigSQLServer converts DatabaseConfig to config.SQLServer.
func ToConfigSQLServer(cfg DatabaseConfig) config.SQLServer {
	return config.SQLServer{Host: cfg.Host}
}

// ToConfigSQLDatabase converts DatabaseConfig to config.SQLDatabase.
func ToConfigSQLDatabase(cfg DatabaseConfig) config.SQLDatabase {
	return config.SQLDatabase{
		EncoreName:   cfg.EncoreName,
		DatabaseName: cfg.Database,
		User:         cfg.User,
		Password:     cfg.Password,
	}
}
