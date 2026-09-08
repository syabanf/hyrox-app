// Command seed loads the demo studio into the database.
//
//	go run ./cmd/seed
//
// It is idempotent: running it again changes nothing.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/syabanf/nuhabit-backend/internal/platform/clock"
	"github.com/syabanf/nuhabit-backend/internal/platform/config"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/logging"
	"github.com/syabanf/nuhabit-backend/internal/seed"
	"github.com/syabanf/nuhabit-backend/migrations"
)

func main() {
	if err := run(); err != nil {
		slog.Error("seed failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logging.Setup(cfg.Log)

	if cfg.IsProduction() && os.Getenv("SEED_ALLOW_PRODUCTION") != "true" {
		return errProduction
	}

	db, err := database.Connect(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.Migrate(ctx, migrations.FS); err != nil {
		return err
	}

	summary, err := seed.New(db, clock.Real{}).Run(ctx)
	if err != nil {
		return err
	}
	slog.Info("demo studio seeded",
		"branches", summary.Branches,
		"coaches", summary.Coaches,
		"classTypes", summary.ClassTypes,
		"packages", summary.Packages,
		"members", summary.Members,
		"adminUsers", summary.AdminUsers,
		"sessions", summary.Sessions,
		"exercises", summary.Exercises,
		"employees", summary.Employees,
		"stockItems", summary.StockItems,
		"suppliers", summary.Suppliers,
	)
	return nil
}

// errProduction refuses to write demo data into a production database unless
// the operator has very deliberately said otherwise.
var errProduction = seedError("refusing to seed a production database; set SEED_ALLOW_PRODUCTION=true to override")

type seedError string

func (e seedError) Error() string { return string(e) }
