// Command api runs the nuhabit backend.
//
// By default it mounts every module, which is the modular monolith. Set
// MODULES to a subset (for example MODULES=wallet,identity) and the same
// binary becomes a single service.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/app"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/config"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/database"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/logging"
	"github.com/syabanf/hyrox-app/apps/backend/migrations"
)

// health is the container healthcheck. The runtime image is scratch — no
// shell, no curl — so the binary is what asks the question, and the answer is
// the exit code.
var health = flag.Bool("health", false, "probe the local server and exit")

func main() {
	flag.Parse()
	if *health {
		os.Exit(probe())
	}
	if err := run(); err != nil {
		slog.Error("server exited with an error", "error", err)
		os.Exit(1)
	}
}

// probe asks the server on this container's own port whether it can serve.
//
// It reads /ready rather than /health: "the process is alive" is not the
// question a dependent service is asking. Readiness means the database is
// reachable, which is what the seeder and the front door actually wait for.
//
// HTTP_ADDR rather than a hard-coded :8080, so a container started on another
// port checks the port it is really listening on.
func probe() int {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}

	client := &http.Client{Timeout: 3 * time.Second}
	res, err := client.Get("http://" + addr + "/ready")
	if err != nil {
		return 1
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return 1
	}
	return 0
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

	// Applying migrations at boot keeps a deployment to one artifact. The
	// advisory lock means several replicas starting together is safe.
	if os.Getenv("AUTO_MIGRATE") != "false" {
		if err := db.Migrate(ctx, migrations.FS); err != nil {
			return err
		}
	}

	application := app.New(cfg, db)
	application.RunBackground(ctx)

	server := &http.Server{
		Addr:         cfg.HTTP.Addr,
		Handler:      application.Handler,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("nuhabit api listening",
			"addr", cfg.HTTP.Addr,
			"env", cfg.Env,
			"modules", cfg.Modules.Enabled,
		)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		slog.Info("shutting down")
	}

	// Drain in-flight requests before exiting, so a deploy does not cut a
	// member off mid-checkout.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	// Give background loops a moment to finish their current unit of work.
	time.Sleep(200 * time.Millisecond)
	return nil
}
