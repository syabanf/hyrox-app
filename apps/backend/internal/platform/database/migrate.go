package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
)

// advisoryLockKey serializes migrations across replicas: when several
// instances start at once, exactly one applies and the rest wait, then find
// nothing to do.
const advisoryLockKey int64 = 8_675_309

// Migrate applies every unapplied migration from the filesystem, in filename
// order, each in its own transaction.
//
// Applied migrations are checksummed. Editing a migration that already ran is
// a hard error rather than a silent divergence between environments.
func (db *DB) Migrate(ctx context.Context, files fs.FS) error {
	conn, err := db.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("migrate: acquiring connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockKey); err != nil {
		return fmt.Errorf("migrate: acquiring advisory lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, advisoryLockKey)
	}()

	if _, err := conn.Exec(ctx, `
		CREATE SCHEMA IF NOT EXISTS platform;
		CREATE TABLE IF NOT EXISTS platform.schema_migrations (
			version    TEXT PRIMARY KEY,
			checksum   TEXT        NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
	`); err != nil {
		return fmt.Errorf("migrate: creating migrations table: %w", err)
	}

	applied := map[string]string{}
	rows, err := conn.Query(ctx, `SELECT version, checksum FROM platform.schema_migrations`)
	if err != nil {
		return fmt.Errorf("migrate: reading applied migrations: %w", err)
	}
	for rows.Next() {
		var version, checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			rows.Close()
			return fmt.Errorf("migrate: scanning applied migrations: %w", err)
		}
		applied[version] = checksum
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("migrate: reading applied migrations: %w", err)
	}

	names, err := migrationNames(files)
	if err != nil {
		return err
	}

	for _, name := range names {
		body, err := fs.ReadFile(files, name)
		if err != nil {
			return fmt.Errorf("migrate: reading %s: %w", name, err)
		}
		sum := sha256.Sum256(body)
		checksum := hex.EncodeToString(sum[:])

		if previous, ok := applied[name]; ok {
			if previous != checksum {
				return fmt.Errorf(
					"migrate: %s changed after it was applied (add a new migration instead of editing this one)",
					name,
				)
			}
			continue
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("migrate: begin %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, string(body)); err != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
			return fmt.Errorf("migrate: applying %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO platform.schema_migrations (version, checksum) VALUES ($1, $2)`,
			name, checksum,
		); err != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
			return fmt.Errorf("migrate: recording %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("migrate: commit %s: %w", name, err)
		}
		slog.InfoContext(ctx, "migration applied", "version", name)
	}
	return nil
}

// MigrationStatus lists each migration and whether it has been applied.
func (db *DB) MigrationStatus(ctx context.Context, files fs.FS) ([]string, error) {
	names, err := migrationNames(files)
	if err != nil {
		return nil, err
	}
	applied := map[string]bool{}
	rows, err := db.pool.Query(ctx, `SELECT version FROM platform.schema_migrations`)
	if err == nil {
		for rows.Next() {
			var version string
			if err := rows.Scan(&version); err == nil {
				applied[version] = true
			}
		}
		rows.Close()
	}

	out := make([]string, 0, len(names))
	for _, name := range names {
		state := "pending"
		if applied[name] {
			state = "applied"
		}
		out = append(out, fmt.Sprintf("%-24s %s", name, state))
	}
	return out, nil
}

func migrationNames(files fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, fmt.Errorf("migrate: listing migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	// Filenames are zero-padded, so lexical order is numeric order.
	sort.Strings(names)
	return names, nil
}
