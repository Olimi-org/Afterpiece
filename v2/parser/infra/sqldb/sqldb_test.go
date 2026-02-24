package sqldb

import (
	"testing"

	"encr.dev/pkg/appfile"
	"encr.dev/v2/internals/parsectx"
	"encr.dev/v2/parser/resource/resourcetest"
)

func TestParseDatabase(t *testing.T) {
	tests := []resourcetest.Case[*Database]{
		{
			Name: "constructor",
			Code: `
var x = sqldb.NewDatabase("name", sqldb.DatabaseConfig{
	Migrations: "some/migration/path",
})
-- some/migration/path/foo.txt --
`,
			Want: &Database{
				Name:         "name",
				MigrationDir: "some/migration/path",
			},
		},
		{
			Name: "migration_file",
			Code: `
var x = sqldb.NewDatabase("name", sqldb.DatabaseConfig{
	Migrations: "some/migration/path",
})
-- some/migration/path/1_foo.up.sql --
CREATE TABLE foo (id int);
`,
			Want: &Database{
				Name:         "name",
				MigrationDir: "some/migration/path",
				Migrations: []MigrationFile{{
					Filename:    "1_foo.up.sql",
					Number:      1,
					Description: "foo",
				}},
			},
		},
		{
			Name: "abs_path",
			Code: `
var x = sqldb.NewDatabase("name", sqldb.DatabaseConfig{
	Migrations: "/path",
})
`,
			WantErrs: []string{`.*The migration path must be a relative path.*`},
		},
		{
			Name: "non_local_path",
			Code: `
var x = sqldb.NewDatabase("name", sqldb.DatabaseConfig{
	Migrations: "../path",
})
`,
			WantErrs: []string{`.*The migration path must be a relative path.*`},
		},
		{
			Name: "atlas_strategy_optional_migrations",
			Code: `
var x = sqldb.NewDatabase("name", sqldb.DatabaseConfig{})
`,
			SetupContext: func(c *parsectx.Context) {
				c.Build.MigrationStrategy = appfile.MigrationStrategyAtlas
			},
			Want: &Database{
				Name: "name",
			},
		},
		{
			Name: "atlas_strategy_with_migrations_path",
			Code: `
var x = sqldb.NewDatabase("name", sqldb.DatabaseConfig{
	Migrations: "some/migration/path",
})
`,
			SetupContext: func(c *parsectx.Context) {
				c.Build.MigrationStrategy = appfile.MigrationStrategyAtlas
			},
			WantErrs: []string{`.*A call to sqldb.NewDatabase must not specify the Migrations directory.*`},
		},
	}

	resourcetest.Run(t, DatabaseParser, tests)
}
