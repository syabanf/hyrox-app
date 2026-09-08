package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type txKey struct{}

func txFrom(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	return tx, ok
}

// InTx runs fn inside a transaction, committing when it returns nil and
// rolling back on any error or panic.
//
// The transaction rides on the context, so repositories called inside fn join
// it automatically. This is what makes a gate scan atomic: consuming the QR
// token, deducting the credit and checking the booking in either all happen or
// none do.
//
// Nested calls join the outer transaction rather than opening a second one, so
// a use case can call another use case without either knowing.
func (db *DB) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := txFrom(ctx); ok {
		return fn(ctx)
	}

	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("database: begin: %w", err)
	}

	committed := false
	defer func() {
		if committed {
			return
		}
		// Rollback on the background context: the request context may already
		// be cancelled, and the rollback still has to reach the server.
		_ = tx.Rollback(context.WithoutCancel(ctx))
	}()

	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("database: commit: %w", err)
	}
	committed = true
	return nil
}

// InTxSerializable runs fn at SERIALIZABLE isolation and retries when the
// database reports a serialization failure.
//
// Booking the last slot in a class is the case that needs it: two members can
// otherwise each read "one place left" and both take it.
func (db *DB) InTxSerializable(ctx context.Context, attempts int, fn func(ctx context.Context) error) error {
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		err := db.inTxWithOptions(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable}, fn)
		if err == nil {
			return nil
		}
		if !isRetryable(err) {
			return err
		}
		lastErr = err
	}
	return fmt.Errorf("database: gave up after %d serialization retries: %w", attempts, lastErr)
}

func (db *DB) inTxWithOptions(ctx context.Context, opts pgx.TxOptions, fn func(ctx context.Context) error) error {
	if _, ok := txFrom(ctx); ok {
		return fn(ctx)
	}
	tx, err := db.pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("database: begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()
	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("database: commit: %w", err)
	}
	committed = true
	return nil
}

// isRetryable reports whether a failed transaction would plausibly succeed on
// a second attempt: serialization failure or deadlock.
func isRetryable(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	switch pgErr.Code {
	case "40001", "40P01": // serialization_failure, deadlock_detected
		return true
	}
	return false
}

// PostgreSQL error codes the modules translate into domain outcomes.
const (
	codeUniqueViolation     = "23505"
	codeForeignKeyViolation = "23503"
	codeCheckViolation      = "23514"
	codeRestrictViolation   = "23001"
)

// IsUniqueViolation reports a duplicate key, optionally narrowed to one
// constraint so a handler can tell which uniqueness rule was hit.
func IsUniqueViolation(err error, constraints ...string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != codeUniqueViolation {
		return false
	}
	if len(constraints) == 0 {
		return true
	}
	for _, name := range constraints {
		if pgErr.ConstraintName == name {
			return true
		}
	}
	return false
}

// IsForeignKeyViolation reports a reference to a row that does not exist, or a
// delete blocked by children.
func IsForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codeForeignKeyViolation
}

// IsCheckViolation reports a CHECK constraint failure, which means a value got
// past application validation and the database caught it.
func IsCheckViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codeCheckViolation
}

// IsAppendOnlyViolation reports an attempt to modify an append-only table.
// The trigger raises restrict_violation, which is what the ledger and the
// audit log use to refuse an UPDATE or DELETE outright.
func IsAppendOnlyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codeRestrictViolation
}
