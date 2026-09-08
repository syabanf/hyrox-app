// Command migrate applies the database schema.
//
//	go run ./cmd/migrate         # apply everything pending
//	go run ./cmd/migrate status  # list migrations and their state
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/syabanf/nuhabit-backend/internal/platform/config"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/logging"
	"github.com/syabanf/nuhabit-backend/migrations"
)

func main() {
	if err := run(); err != nil {
		slog.Error("migrate failed", "error", err)
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

	db, err := database.Connect(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer db.Close()

	if len(os.Args) > 1 && os.Args[1] == "status" {
		lines, err := db.MigrationStatus(ctx, migrations.FS)
		if err != nil {
			return err
		}
		for _, line := range lines {
			fmt.Println(line)
		}
		return nil
	}

	if err := db.Migrate(ctx, migrations.FS); err != nil {
		return err
	}
	slog.Info("schema is up to date")
	return nil
}
