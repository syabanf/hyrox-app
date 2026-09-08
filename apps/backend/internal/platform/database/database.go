// Package database owns the PostgreSQL connection pool and the transaction
// boundary every module shares.
package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/syabanf/nuhabit-backend/internal/platform/config"
)

// Executor is the subset of pgx that repositories use. Both the pool and an
// open transaction satisfy it, which is what lets a repository run inside or
// outside a transaction without knowing which.
type Executor interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// DB is the handle repositories are given.
type DB struct {
	pool *pgxpool.Pool
}

// Connect opens the pool and verifies the database answers.
func Connect(ctx context.Context, cfg config.Database) (*DB, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("database: parsing DSN: %w", err)
	}
	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.ConnConfig.ConnectTimeout = cfg.ConnectTimeout
	if cfg.StatementTimeout > 0 {
		// A server-side cap, so a runaway query cannot hold a pool slot open
		// even if the client context is somehow lost.
		poolCfg.ConnConfig.RuntimeParams["statement_timeout"] =
			fmt.Sprintf("%d", cfg.StatementTimeout.Milliseconds())
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("database: creating pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: ping failed: %w", err)
	}
	return &DB{pool: pool}, nil
}

// Close releases every pooled connection.
func (db *DB) Close() { db.pool.Close() }

// Pool exposes the raw pool for the few callers that need it (health checks,
// migrations, advisory locks).
func (db *DB) Pool() *pgxpool.Pool { return db.pool }

// Health reports whether the database is reachable.
func (db *DB) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return db.pool.Ping(ctx)
}

// Query runs a query on the ambient transaction when there is one, and on the
// pool otherwise.
func (db *DB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return db.executor(ctx).Query(ctx, sql, args...)
}

func (db *DB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return db.executor(ctx).QueryRow(ctx, sql, args...)
}

func (db *DB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return db.executor(ctx).Exec(ctx, sql, args...)
}

func (db *DB) executor(ctx context.Context) Executor {
	if tx, ok := txFrom(ctx); ok {
		return tx
	}
	return db.pool
}

// ErrNoRows is returned when a query that expected a row found none. It wraps
// pgx's sentinel so callers do not import pgx just to check it.
var ErrNoRows = pgx.ErrNoRows

// IsNoRows reports whether an error means "nothing matched".
func IsNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
