package infra

import (
	"fmt"
	"strconv"

	"encore.dev/appruntime/exported/config"
	configinfra "encore.dev/appruntime/exported/config/infra"
	"encr.dev/cli/daemon/sqldb"
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

func (r DatabaseResolver) Resolve(name string, infra *configinfra.InfraConfig, resolveValue ValueResolver) (DatabaseConfig, bool) {
	if infra == nil || len(infra.SQLServers) == 0 {
		return DatabaseConfig{}, false
	}

	for _, srv := range infra.SQLServers {
		if srv.Databases != nil {
			if db, ok := srv.Databases[name]; ok {
				username, usrOk := resolveValue(db.Username)
				password, passOk := resolveValue(db.Password)
				if !usrOk || !passOk {
					return DatabaseConfig{}, false
				}

				dbName := db.Name
				if dbName == "" {
					dbName = name
				}

				return DatabaseConfig{
					Host:       srv.Host,
					Database:   dbName,
					User:       username,
					Password:   password,
					EncoreName: name,
				}, true
			}
		}
	}

	return DatabaseConfig{}, false
}

func (r DatabaseResolver) ResolveWithFallback(name string, infra *configinfra.InfraConfig, resolveValue ValueResolver) (DatabaseConfig, error) {
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
