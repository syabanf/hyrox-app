// Package config loads all runtime configuration from the environment.
//
// Every module reads its settings from here, so a module extracted into its
// own service keeps the same knobs it had inside the monolith.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/auth"
)

// Config is the whole configuration surface of any nuhabit binary.
type Config struct {
	Env string
	// StudioTimezone is the location the studio's day is measured in. Reports,
	// payroll periods and "today" all resolve against it, never against UTC.
	StudioTimezone string
	HTTP           HTTP
	Database       Database
	Auth           Auth
	Payments       Payments
	Social         Social
	Modules        Modules
	Log            Log
}

type HTTP struct {
	Addr            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
	// AllowedOrigins are echoed back for CORS; "*" allows any origin.
	AllowedOrigins []string
}

type Database struct {
	DSN             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	ConnectTimeout  time.Duration
	// StatementTimeout guards against a runaway query holding a pool slot.
	StatementTimeout time.Duration
}

type Auth struct {
	// Secret signs access tokens. Every service that verifies tokens needs the
	// same value, which is what keeps verification stateless across a split.
	Secret    []byte
	TokenTTL  time.Duration
	Issuer    string
	OTPTTL    time.Duration
	OTPLength int
	// DemoOTP accepts any well-formed code, for demo and test environments.
	DemoOTP bool
	// PasswordIterations is the PBKDF2 cost for staff passwords. It is
	// configurable so it can be raised as hardware gets faster, and lowered in
	// test runs that hash a whole demo roster on every setup.
	PasswordIterations int
	// LoginAttempts is how many wrong passwords an account tolerates before it
	// locks, and LockoutFor is how long it stays shut.
	LoginAttempts int
	LockoutFor    time.Duration
}

type Payments struct {
	// Provider selects the gateway adapter: "mock" or "xendit".
	Provider       string
	XenditAPIKey   string
	XenditCallback string
	// InvoiceTTL is how long a pending payment stays payable.
	InvoiceTTL time.Duration
}

// Social is what an inbound social webhook needs to prove itself.
//
// Both are empty by default, and an unconfigured webhook refuses everything.
// A publicly reachable endpoint that writes to the support inbox without
// checking a signature is an open door, so the safe state is off.
type Social struct {
	// InstagramVerifyToken is echoed back during the subscription handshake.
	InstagramVerifyToken string
	// InstagramAppSecret signs every delivery.
	InstagramAppSecret string
}

// Modules decides which bounded contexts this process mounts. Running the
// whole set is the modular monolith; running a subset is a microservice.
type Modules struct {
	Enabled []string
}

type Log struct {
	Level  string
	Format string
}

// ModuleAll is the sentinel that mounts every module.
const ModuleAll = "all"

// Enabled reports whether the named module should be mounted by this process.
func (m Modules) IsEnabled(name string) bool {
	for _, n := range m.Enabled {
		if n == ModuleAll || n == name {
			return true
		}
	}
	return false
}

// Load reads configuration from the environment, applying defaults that make
// `go run ./cmd/api` work against the docker-compose database.
func Load() (Config, error) {
	cfg := Config{
		Env:            env("APP_ENV", "development"),
		StudioTimezone: env("STUDIO_TIMEZONE", "Asia/Jakarta"),
		HTTP: HTTP{
			Addr:            env("HTTP_ADDR", ":8080"),
			ReadTimeout:     duration("HTTP_READ_TIMEOUT", 15*time.Second),
			WriteTimeout:    duration("HTTP_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:     duration("HTTP_IDLE_TIMEOUT", 60*time.Second),
			ShutdownTimeout: duration("HTTP_SHUTDOWN_TIMEOUT", 15*time.Second),
			AllowedOrigins:  list("HTTP_ALLOWED_ORIGINS", []string{"*"}),
		},
		Database: Database{
			DSN:              env("DATABASE_URL", "postgres://nuhabit:nuhabit@localhost:5432/nuhabit?sslmode=disable"),
			MaxConns:         int32(number("DATABASE_MAX_CONNS", 20)),
			MinConns:         int32(number("DATABASE_MIN_CONNS", 2)),
			MaxConnLifetime:  duration("DATABASE_MAX_CONN_LIFETIME", time.Hour),
			ConnectTimeout:   duration("DATABASE_CONNECT_TIMEOUT", 10*time.Second),
			StatementTimeout: duration("DATABASE_STATEMENT_TIMEOUT", 15*time.Second),
		},
		Auth: Auth{
			Secret:    []byte(env("AUTH_SECRET", "dev-secret-change-me")),
			TokenTTL:  duration("AUTH_TOKEN_TTL", 720*time.Hour),
			Issuer:    env("AUTH_ISSUER", "nuhabit"),
			OTPTTL:    duration("AUTH_OTP_TTL", 5*time.Minute),
			OTPLength: number("AUTH_OTP_LENGTH", 6),
			DemoOTP:   boolean("AUTH_DEMO_OTP", true),

			PasswordIterations: number("AUTH_PASSWORD_ITERATIONS", auth.DefaultPasswordIterations),
			LoginAttempts:      number("AUTH_LOGIN_ATTEMPTS", 10),
			LockoutFor:         duration("AUTH_LOCKOUT_FOR", 15*time.Minute),
		},
		Payments: Payments{
			Provider:       env("PAYMENTS_PROVIDER", "mock"),
			XenditAPIKey:   env("XENDIT_API_KEY", ""),
			XenditCallback: env("XENDIT_CALLBACK_TOKEN", ""),
			InvoiceTTL:     duration("PAYMENTS_INVOICE_TTL", 24*time.Hour),
		},
		Social: Social{
			InstagramVerifyToken: env("INSTAGRAM_VERIFY_TOKEN", ""),
			InstagramAppSecret:   env("INSTAGRAM_APP_SECRET", ""),
		},
		Modules: Modules{Enabled: list("MODULES", []string{ModuleAll})},
		Log: Log{
			Level:  env("LOG_LEVEL", "info"),
			Format: env("LOG_FORMAT", "json"),
		},
	}
	return cfg, cfg.validate()
}

func (c Config) validate() error {
	if c.Database.DSN == "" {
		return fmt.Errorf("config: DATABASE_URL is required")
	}
	if len(c.Auth.Secret) == 0 {
		return fmt.Errorf("config: AUTH_SECRET is required")
	}
	if c.IsProduction() {
		if string(c.Auth.Secret) == "dev-secret-change-me" {
			return fmt.Errorf("config: AUTH_SECRET must be set outside development")
		}
		if c.Auth.DemoOTP {
			return fmt.Errorf("config: AUTH_DEMO_OTP must be false in production")
		}
		// The knob exists so a test run does not spend half a second hashing a
		// demo roster. A production deployment that inherited that value would
		// be storing passwords behind a KDF that costs an attacker nothing.
		if c.Auth.PasswordIterations < auth.DefaultPasswordIterations {
			return fmt.Errorf("config: AUTH_PASSWORD_ITERATIONS must be at least %d in production",
				auth.DefaultPasswordIterations)
		}
	}
	if c.Payments.Provider == "xendit" && c.Payments.XenditAPIKey == "" {
		return fmt.Errorf("config: XENDIT_API_KEY is required when PAYMENTS_PROVIDER=xendit")
	}
	// Only the mock gateway is implemented. Accepting another name and then
	// wiring the mock anyway would mean a studio believes it is taking card
	// payments while the server settles every one of them itself.
	if c.Payments.Provider != "mock" {
		return fmt.Errorf(
			"config: PAYMENTS_PROVIDER=%q is not implemented; only \"mock\" exists",
			c.Payments.Provider)
	}
	// The mock settles every invoice without any money moving. That is fine
	// for a studio whose members top up at the desk and whose staff record it,
	// and ruinous for one that puts a payment link in front of a member. In
	// production the operator has to say which they are.
	if c.IsProduction() && !boolean("PAYMENTS_ALLOW_MOCK", false) {
		return fmt.Errorf(
			"config: no payment gateway is implemented — the mock settles every top-up " +
				"without taking money. Set PAYMENTS_ALLOW_MOCK=true only if top-ups are " +
				"taken at the desk and recorded by staff. See docs/DEPLOYMENT.md")
	}
	return nil
}

func (c Config) IsProduction() bool { return c.Env == "production" }

// StudioLocation resolves the configured timezone, falling back to UTC when
// the host has no timezone database.
func (c Config) StudioLocation() *time.Location {
	loc, err := time.LoadLocation(c.StudioTimezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func number(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func boolean(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func duration(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func list(key string, fallback []string) []string {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return fallback
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}
