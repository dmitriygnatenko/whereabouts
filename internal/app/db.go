package app

import (
	"context"
	"fmt"
	"log"

	"wherewhat/internal/adapter/mysql"
	"wherewhat/internal/adapter/postgres"
	"wherewhat/internal/adapter/sqlite"
	"wherewhat/internal/config"
	itemrepo "wherewhat/internal/repository/item"
	locationrepo "wherewhat/internal/repository/location"
	sessionrepo "wherewhat/internal/repository/session"
	userrepo "wherewhat/internal/repository/user"
)

// storage is where the four repositories' separate expectations of a driver adapter meet: each names
// only the table group it touches, and this is the one place that asks a single adapter to satisfy
// all of them at once — so a driver missing an operation fails to compile here, at the composition
// root, rather than anywhere downstream.
type storage interface {
	itemrepo.Storage
	locationrepo.Storage
	sessionrepo.Storage
	userrepo.Storage

	Close() error
}

// openStorage picks the driver adapter named by cfg.Driver, makes sure the target database exists,
// opens the connection and runs migrations against it. Only this composition-root function knows
// about the concrete adapters — everything downstream depends on the interface it needs.
func openStorage(ctx context.Context, cfg config.DBConfig) (storage, error) {
	switch cfg.Driver {
	case config.DriverSQLite:
		sqliteCfg := sqlite.Config{Path: cfg.SQLitePath, ConnMaxLifetime: cfg.ConnMaxLifetime}
		log.Printf("using SQLite (%s)...", cfg.SQLitePath)

		if err := sqlite.EnsureDatabase(sqliteCfg); err != nil {
			log.Printf("warning: failed to auto-create the database: %v", err)
		}

		store, err := sqlite.Open(sqliteCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to the database: %w", err)
		}

		if err := sqlite.Migrate(ctx, store); err != nil {
			return nil, fmt.Errorf("failed to run migrations: %w", err)
		}

		return store, nil

	case config.DriverMySQL:
		mysqlCfg := mysql.Config{
			Host: cfg.Host, Port: cfg.Port, User: cfg.User, Password: cfg.Password, Name: cfg.Name,
			MaxOpenConns: cfg.MaxOpenConns, MaxIdleConns: cfg.MaxIdleConns, ConnMaxLifetime: cfg.ConnMaxLifetime,
			ConnTimeout: cfg.ConnTimeout,
		}

		log.Printf("checking database %q at %s:%s (mysql)...", cfg.Name, cfg.Host, cfg.Port)

		if err := mysql.EnsureDatabase(mysqlCfg); err != nil {
			log.Printf("warning: failed to auto-create the database: %v", err)
			log.Printf("if database %q already exists, this warning can be ignored", cfg.Name)
		}

		store, err := mysql.Open(mysqlCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to the database: %w", err)
		}

		if err := mysql.Migrate(ctx, store); err != nil {
			return nil, fmt.Errorf("failed to run migrations: %w", err)
		}

		return store, nil

	case config.DriverPostgres:
		postgresCfg := postgres.Config{
			Host: cfg.Host, Port: cfg.Port, User: cfg.User, Password: cfg.Password, Name: cfg.Name,
			MaxOpenConns: cfg.MaxOpenConns, MaxIdleConns: cfg.MaxIdleConns, ConnMaxLifetime: cfg.ConnMaxLifetime,
		}

		log.Printf("checking database %q at %s:%s (postgres)...", cfg.Name, cfg.Host, cfg.Port)

		if err := postgres.EnsureDatabase(postgresCfg); err != nil {
			log.Printf("warning: failed to auto-create the database: %v", err)
			log.Printf("if database %q already exists, this warning can be ignored", cfg.Name)
		}

		store, err := postgres.Open(postgresCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to the database: %w", err)
		}

		if err := postgres.Migrate(ctx, store); err != nil {
			return nil, fmt.Errorf("failed to run migrations: %w", err)
		}

		return store, nil

	default:
		return nil, fmt.Errorf("unsupported DB_DRIVER %q", cfg.Driver)
	}
}
