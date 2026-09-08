// Command bootstrap creates the first staff account on a fresh installation.
//
//	BOOTSTRAP_PASSWORD='...' go run ./cmd/bootstrap \
//	    -name "Alya Santoso" -email alya@studio.example
//
// It exists to break a deadlock. Every route that creates a staff account
// requires a caller who already has one, the demo role-picker is refused
// outside development, and the seeder will not touch a production database.
// A correctly configured production deployment therefore has no way in at all
// until somebody runs this once.
//
// It is deliberately a one-shot: it refuses when any staff account already
// exists, so it cannot be used to quietly mint a second Super Admin on a
// running system. Use the panel for that, where it is audited.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/auth"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/config"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/database"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/id"
)

// minPasswordLength mirrors the identity module's floor. Length is the part
// that costs an attacker something; composition rules are what produce
// Passw0rd! on a sticky note.
const minPasswordLength = 10

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "bootstrap:", err)
		os.Exit(1)
	}
}

func run() error {
	name := flag.String("name", "", "the person's full name")
	email := flag.String("email", "", "the email address they will sign in with")
	flag.Parse()

	if strings.TrimSpace(*name) == "" || strings.TrimSpace(*email) == "" {
		return errors.New("both -name and -email are required")
	}

	password, err := readPassword()
	if err != nil {
		return err
	}
	if len([]rune(password)) < minPasswordLength {
		return fmt.Errorf("a password is at least %d characters", minPasswordLength)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := database.Connect(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer db.Close()

	// The deadlock this command breaks only exists while there is nobody. Once
	// somebody can sign in, accounts are created in the panel, where the audit
	// trail records who created whom.
	var existing int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM identity.admin_users`).Scan(&existing); err != nil {
		return fmt.Errorf("counting staff accounts: %w (have the migrations run?)", err)
	}
	if existing > 0 {
		return fmt.Errorf(
			"%d staff account(s) already exist; create more from the panel so the change is audited", existing)
	}

	hash, err := auth.NewPasswords(cfg.Auth.PasswordIterations).Hash(password)
	if err != nil {
		return err
	}

	adminID := id.NewGenerator().New(id.AdminUser)
	_, err = db.Exec(ctx, `
		INSERT INTO identity.admin_users
			(id, name, email, role, branch_id, password_hash, password_set_at, must_change_password)
		VALUES ($1, $2, lower($3), 'SUPER_ADMIN', NULL, $4, now(), false)`,
		adminID, strings.TrimSpace(*name), strings.TrimSpace(*email), hash)
	if err != nil {
		return fmt.Errorf("creating the account: %w", err)
	}

	fmt.Printf("Created Super Admin %s <%s> (%s).\nSign in at /admin with that email and the password you supplied.\n",
		strings.TrimSpace(*name), strings.ToLower(strings.TrimSpace(*email)), adminID)
	return nil
}

// readPassword takes the password from the environment, or from stdin when it
// is piped in.
//
// Not a flag: a flag lands in the shell history and in `ps` output, where the
// first credential on a new system is the last thing that should be.
func readPassword() (string, error) {
	if fromEnv := os.Getenv("BOOTSTRAP_PASSWORD"); fromEnv != "" {
		return fromEnv, nil
	}

	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}
	if stat.Mode()&os.ModeCharDevice != 0 {
		return "", errors.New(
			"set BOOTSTRAP_PASSWORD, or pipe the password in on stdin")
	}

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("reading the password from stdin: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}
