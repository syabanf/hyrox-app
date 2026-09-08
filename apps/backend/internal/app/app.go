// Package app assembles the modules into a running process.
//
// This is the only place that knows every module exists. Each module declares
// the interfaces it needs, and the wiring here satisfies them — in-process
// today, over the network the day a module moves out. Which modules a given
// binary mounts is configuration (MODULES=...), so the monolith and a single
// extracted service are the same code with different settings.
package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/syabanf/nuhabit-backend/internal/modules/access"
	"github.com/syabanf/nuhabit-backend/internal/modules/catalog"
	"github.com/syabanf/nuhabit-backend/internal/modules/identity"
	"github.com/syabanf/nuhabit-backend/internal/modules/incentives"
	"github.com/syabanf/nuhabit-backend/internal/modules/reporting"
	"github.com/syabanf/nuhabit-backend/internal/modules/scheduling"
	"github.com/syabanf/nuhabit-backend/internal/modules/wallet"
	"github.com/syabanf/nuhabit-backend/internal/platform/audit"
	"github.com/syabanf/nuhabit-backend/internal/platform/auth"
	"github.com/syabanf/nuhabit-backend/internal/platform/clock"
	"github.com/syabanf/nuhabit-backend/internal/platform/config"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
	"github.com/syabanf/nuhabit-backend/internal/platform/id"
	"github.com/syabanf/nuhabit-backend/internal/platform/outbox"
)

// Module names, used by the MODULES setting to decide what this process runs.
const (
	ModuleCatalog    = "catalog"
	ModuleIdentity   = "identity"
	ModuleWallet     = "wallet"
	ModuleScheduling = "scheduling"
	ModuleAccess     = "access"
	ModuleIncentives = "incentives"
	ModuleReporting  = "reporting"
)

// App is a wired-up process: an HTTP handler plus the background work that
// keeps the data honest.
type App struct {
	Config     config.Config
	DB         *database.DB
	Handler    http.Handler
	Dispatcher *outbox.Dispatcher

	// Services stay exposed so tests and the seeder can drive the same code
	// paths the HTTP layer does.
	Catalog    *catalog.Service
	Identity   *identity.Service
	Wallet     *wallet.Service
	Scheduling *scheduling.Service
	Access     *access.Service
	Incentives *incentives.Service
	Reporting  *reporting.Service

	clock clock.Clock
}

// New builds the application from configuration and an open database.
func New(cfg config.Config, db *database.DB) *App {
	ids := id.NewGenerator()
	now := clock.Real{}
	auditor := audit.NewRecorder(db, ids, now)
	events := outbox.New(db, ids, now)

	issuer := auth.NewIssuer(cfg.Auth.Secret, cfg.Auth.TokenTTL, cfg.Auth.Issuer, func() string {
		return ids.New("tok")
	})
	guard := auth.NewGuard(issuer, now, func(role, permission string) bool {
		return domainHasPermission(role, permission)
	})

	// Repositories and services. Construction order follows the dependency
	// direction: catalog depends on nothing, and access depends on everything.
	catalogRepo := catalog.NewRepository(db)
	catalogService := catalog.NewService(catalogRepo, ids, now, auditor)

	identityRepo := identity.NewRepository(db)
	identityService := identity.NewService(identityRepo, ids, now, issuer, auditor, cfg.Auth)

	var gateway wallet.Gateway = wallet.NewMockGateway(now.Now)
	walletRepo := wallet.NewRepository(db)
	walletService := wallet.NewService(db, walletRepo, catalogService, identityService,
		gateway, ids, now, auditor, events, cfg.Payments)

	schedulingRepo := scheduling.NewRepository(db)
	schedulingService := scheduling.NewService(db, schedulingRepo, catalogService,
		identityService, walletService, ids, now, auditor, events)

	accessRepo := access.NewRepository(db)
	accessService := access.NewService(db, accessRepo, catalogService, identityService,
		schedulingService, walletService, ids, now, auditor, events)

	incentivesRepo := incentives.NewRepository(db)
	incentivesService := incentives.NewService(db, incentivesRepo, catalogService,
		schedulingService, ids, now, auditor, cfg.StudioLocation())

	reportingService := reporting.NewService(
		reportingMembers{identityService}, walletService, schedulingService,
		accessService, catalogService, incentivesService, auditor, now, cfg.StudioLocation())

	router := httpx.NewRouter(
		httpx.RequestID(ids),
		httpx.Recoverer,
		httpx.Logger,
		httpx.CORS(cfg.HTTP.AllowedOrigins),
	)

	development := !cfg.IsProduction()

	if cfg.Modules.IsEnabled(ModuleCatalog) {
		catalog.NewHandler(catalogService, catalogUsage{schedulingService, walletService}, guard).Mount(router)
	}
	if cfg.Modules.IsEnabled(ModuleIdentity) {
		identity.NewHandler(identityService, guard, cfg.Auth.DemoOTP).Mount(router)
	}
	if cfg.Modules.IsEnabled(ModuleWallet) {
		wallet.NewHandler(walletService, catalogService, identityService, guard, development).Mount(router)
	}
	if cfg.Modules.IsEnabled(ModuleScheduling) {
		scheduling.NewHandler(schedulingService, guard).Mount(router)
	}
	if cfg.Modules.IsEnabled(ModuleAccess) {
		access.NewHandler(accessService, guard, "").Mount(router)
	}
	if cfg.Modules.IsEnabled(ModuleIncentives) {
		incentives.NewHandler(incentivesService, guard).Mount(router)
	}
	if cfg.Modules.IsEnabled(ModuleReporting) {
		reporting.NewHandler(reportingService, guard).Mount(router)
	}

	app := &App{
		Config:     cfg,
		DB:         db,
		Dispatcher: outbox.NewDispatcher(db, now),
		Catalog:    catalogService,
		Identity:   identityService,
		Wallet:     walletService,
		Scheduling: schedulingService,
		Access:     accessService,
		Incentives: incentivesService,
		Reporting:  reportingService,
		clock:      now,
	}

	app.mountOperational(router)
	app.subscribeEvents()
	router.NotFoundHandler()
	app.Handler = router
	return app
}

// mountOperational adds the endpoints an operator needs rather than a user.
func (a *App) mountOperational(r *httpx.Router) {
	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		httpx.OK(w, map[string]string{"status": "ok"})
	})
	// Readiness answers whether this instance can actually serve traffic,
	// which is a different question from whether the process is alive.
	r.Get("/ready", func(w http.ResponseWriter, req *http.Request) {
		if err := a.DB.Health(req.Context()); err != nil {
			httpx.Fail(w, req, httpx.ErrInternal.WithMessage("Database is unreachable.").Wrap(err))
			return
		}
		httpx.OK(w, map[string]any{
			"status":  "ready",
			"modules": a.Config.Modules.Enabled,
		})
	})
}

// subscribeEvents connects outbox topics to their handlers. Until the
// engagement module lands, delivery is recorded rather than sent, which keeps
// the messages flowing and visible instead of silently dropped.
func (a *App) subscribeEvents() {
	log := func(topic string) outbox.Handler {
		return func(ctx context.Context, msg outbox.Message) error {
			slog.InfoContext(ctx, "domain event", "topic", topic, "payload", string(msg.Payload))
			return nil
		}
	}
	for _, topic := range []string{
		outbox.TopicBookingConfirmed,
		outbox.TopicWaitlistPromoted,
		outbox.TopicVisitLogged,
		outbox.TopicPaymentPaid,
	} {
		a.Dispatcher.Subscribe(topic, log(topic))
	}
}

// RunBackground starts the loops that keep derived state correct: outbox
// delivery, credit expiry, no-show marking and credential cleanup.
//
// Every loop is idempotent, so running several instances is safe and a missed
// tick only means the work happens on the next one.
func (a *App) RunBackground(ctx context.Context) {
	go a.Dispatcher.Run(ctx)
	go a.runMaintenance(ctx)
}

func (a *App) runMaintenance(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.maintenanceTick(ctx)
		}
	}
}

func (a *App) maintenanceTick(ctx context.Context) {
	if a.Config.Modules.IsEnabled(ModuleWallet) {
		if result, err := a.Wallet.SweepAll(ctx); err != nil {
			slog.ErrorContext(ctx, "credit expiry sweep failed", "error", err)
		} else if result.Entries > 0 {
			slog.InfoContext(ctx, "credits expired",
				"members", result.AffectedMembers, "entries", result.Entries)
		}
	}
	if a.Config.Modules.IsEnabled(ModuleScheduling) {
		if marked, err := a.Scheduling.SweepNoShows(ctx, 200); err != nil {
			slog.ErrorContext(ctx, "no-show sweep failed", "error", err)
		} else if marked > 0 {
			slog.InfoContext(ctx, "bookings marked no-show", "count", marked)
		}
	}
	if a.Config.Modules.IsEnabled(ModuleAccess) {
		if _, err := a.Access.PurgeExpiredTokens(ctx); err != nil {
			slog.ErrorContext(ctx, "qr token purge failed", "error", err)
		}
	}
	if a.Config.Modules.IsEnabled(ModuleIdentity) {
		if _, err := a.Identity.PurgeExpiredChallenges(ctx); err != nil {
			slog.ErrorContext(ctx, "otp challenge purge failed", "error", err)
		}
	}
}
