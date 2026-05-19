package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/pflag"
	"github.com/go-postnest/postnest/internal/auth"
	"github.com/go-postnest/postnest/internal/config"
	"github.com/go-postnest/postnest/internal/logger"
	"github.com/joho/godotenv"
	"github.com/go-postnest/postnest/internal/models"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: admin <command> [args]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Commands:")
		fmt.Fprintln(os.Stderr, "  create-user    -e/--email -p/--password -n/--name [-s/--super-admin]")
		fmt.Fprintln(os.Stderr, "  create-domain  -n/--name [-t/--postmark-token]")
		fmt.Fprintln(os.Stderr, "  add-member     -d/--domain-id -u/--user-id -r/--role [admin|user|readonly]")
		fmt.Fprintln(os.Stderr, "  reset-password -e/--email -p/--password")
		fmt.Fprintln(os.Stderr, "  setup          -e/--email -p/--password -d/--domain -n/--name  (creates user + domain + membership)")
		os.Exit(1)
	}
	// Load .env file if present so CLI users don't need to manually export vars.
	_ = godotenv.Load()

	log := logger.New()
	if os.Getenv("POSTNEST_DATABASE_DSN") == "" {
		fmt.Fprintln(os.Stderr, "Error: POSTNEST_DATABASE_DSN is not set.")
		fmt.Fprintln(os.Stderr, "Example: POSTNEST_DATABASE_DSN=postgres://postnest:changeme@localhost:5432/postnest?sslmode=disable")
		fmt.Fprintln(os.Stderr, "If using Docker Compose, get the Postgres IP first:")
		fmt.Fprintln(os.Stderr, "  export POSTNEST_DATABASE_DSN=\"postgres://postnest:changeme@$(docker inspect go-postnest-postgres-1 --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}'):5432/postnest?sslmode=disable\"")
		os.Exit(1)
	}

	if os.Getenv("POSTNEST_SECURITY_SESSION_KEY") == "" {
		fmt.Fprintln(os.Stderr, "Error: POSTNEST_SECURITY_SESSION_KEY is not set.")
		fmt.Fprintln(os.Stderr, "Example: POSTNEST_SECURITY_SESSION_KEY=your-32-byte-secret-key")
		os.Exit(1)
	}
	cfg, err := config.Load()
	if err != nil {
		log.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	authSvc := auth.NewService(pool, cfg.Argon2idTime, cfg.Argon2idMemory, cfg.Argon2idThreads, cfg.SessionKey)

	switch os.Args[1] {
	case "create-user":
		os.Exit(cmdCreateUser(ctx, pool, authSvc))
	case "create-domain":
		os.Exit(cmdCreateDomain(ctx, pool))
	case "add-member":
		os.Exit(cmdAddMember(ctx, pool))
	case "setup":
		os.Exit(cmdSetup(ctx, pool, authSvc))
	case "reset-password":
		os.Exit(cmdResetPassword(ctx, pool, authSvc))
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func cmdCreateUser(ctx context.Context, pool *pgxpool.Pool, authSvc *auth.Service) int {
	fs := pflag.NewFlagSet("create-user", pflag.ExitOnError)
	email := fs.StringP("email", "e", "", "User email address")
	password := fs.StringP("password", "p", "", "User password")
	displayName := fs.StringP("name", "n", "", "Display name")
	superAdmin := fs.BoolP("super-admin", "s", false, "Grant super-admin privileges")
	fs.Parse(os.Args[2:])

	if *email == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "Error: --email and --password are required")
		fs.Usage()
		return 1
	}

	user := &models.User{
		ID:           uuid.Must(uuid.NewV7()),
		Email:        *email,
		DisplayName:  *displayName,
		Timezone:     "UTC",
		Locale:       "en",
		IsSuperAdmin: *superAdmin,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	if err := authSvc.CreateUser(ctx, user, *password); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating user: %v\n", err)
		return 1
	}

	fmt.Printf("User created: %s (ID: %s)\n", user.Email, user.ID)
	return 0
}

func cmdResetPassword(ctx context.Context, pool *pgxpool.Pool, authSvc *auth.Service) int {
	fs := pflag.NewFlagSet("reset-password", pflag.ExitOnError)
	email := fs.StringP("email", "e", "", "User email address")
	password := fs.StringP("password", "p", "", "New password")
	fs.Parse(os.Args[2:])

	if *email == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "Error: --email and --password are required")
		fs.Usage()
		return 1
	}

	user, err := authSvc.GetUserByEmail(ctx, *email)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching user: %v\n", err)
		return 1
	}

	if err := authSvc.AdminResetPassword(ctx, user.ID, *password); err != nil {
		fmt.Fprintf(os.Stderr, "Error resetting password: %v\n", err)
		return 1
	}

	fmt.Printf("Password reset for: %s\n", user.Email)
	return 0
}

func cmdCreateDomain(ctx context.Context, pool *pgxpool.Pool) int {
	fs := pflag.NewFlagSet("create-domain", pflag.ExitOnError)
	name := fs.StringP("name", "n", "", "Domain name (e.g. example.com)")
	postmarkToken := fs.StringP("postmark-token", "t", "", "Postmark server API token (optional)")
	fs.Parse(os.Args[2:])

	if *name == "" {
		fmt.Fprintln(os.Stderr, "Error: --name is required")
		fs.Usage()
		return 1
	}

	domain := &models.Domain{
		ID:            uuid.Must(uuid.NewV7()),
		Name:          *name,
		PostmarkToken: *postmarkToken,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO domains (id, name, postmark_token, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (name) DO NOTHING
	`, domain.ID, domain.Name, domain.PostmarkToken, domain.CreatedAt, domain.UpdatedAt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating domain: %v\n", err)
		return 1
	}

	fmt.Printf("Domain created: %s (ID: %s)\n", domain.Name, domain.ID)
	return 0
}

func cmdAddMember(ctx context.Context, pool *pgxpool.Pool) int {
	fs := pflag.NewFlagSet("add-member", pflag.ExitOnError)
	domainIDStr := fs.StringP("domain-id", "d", "", "Domain UUID")
	userIDStr := fs.StringP("user-id", "u", "", "User UUID")
	role := fs.StringP("role", "r", "user", "Role: admin, user, or readonly")
	fs.Parse(os.Args[2:])

	if *domainIDStr == "" || *userIDStr == "" {
		fmt.Fprintln(os.Stderr, "Error: --domain-id and --user-id are required")
		fs.Usage()
		return 1
	}

	domainID, err := uuid.Parse(*domainIDStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid domain-id: %v\n", err)
		return 1
	}
	userID, err := uuid.Parse(*userIDStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid user-id: %v\n", err)
		return 1
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO domain_members (domain_id, user_id, role, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (domain_id, user_id) DO UPDATE SET role = EXCLUDED.role
	`, domainID, userID, *role, time.Now().UTC())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error adding member: %v\n", err)
		return 1
	}

	// Seed system labels for the user on this domain
	_, _ = pool.Exec(ctx, `
		INSERT INTO labels (id, domain_id, user_id, name, color, is_system, created_at)
		SELECT gen_random_uuid(), $1, $2, unnest.label, '#4285f4', true, now()
		FROM LATERAL unnest(ARRAY['INBOX','SENT','DRAFTS','TRASH','JUNK','IMPORTANT','STARRED','ALL_MAIL']) AS unnest(label)
		ON CONFLICT (domain_id, user_id, name) DO NOTHING
	`, domainID, userID)

	fmt.Printf("Member added: user %s -> domain %s (role: %s)\n", userID, domainID, *role)
	return 0
}

func cmdSetup(ctx context.Context, pool *pgxpool.Pool, authSvc *auth.Service) int {
	fs := pflag.NewFlagSet("setup", pflag.ExitOnError)
	email := fs.StringP("email", "e", "", "Admin email address")
	password := fs.StringP("password", "p", "", "Admin password")
	domainName := fs.StringP("domain", "d", "", "Domain name (e.g. example.com)")
	displayName := fs.StringP("name", "n", "Admin", "Display name")
	fs.Parse(os.Args[2:])

	if *email == "" || *password == "" || *domainName == "" {
		fmt.Fprintln(os.Stderr, "Error: --email, --password, and --domain are required")
		fs.Usage()
		return 1
	}

	// 1. Create domain
	domainID := uuid.Must(uuid.NewV7())
	_, err := pool.Exec(ctx, `
		INSERT INTO domains (id, name, created_at, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (name) DO UPDATE SET updated_at = EXCLUDED.updated_at
		RETURNING id
	`, domainID, *domainName, time.Now().UTC(), time.Now().UTC())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating domain: %v\n", err)
		return 1
	}
	// Re-fetch domain ID in case of conflict
	row := pool.QueryRow(ctx, `SELECT id FROM domains WHERE name=$1`, *domainName)
	if err := row.Scan(&domainID); err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching domain: %v\n", err)
		return 1
	}

	// 2. Create super-admin user
	user := &models.User{
		ID:           uuid.Must(uuid.NewV7()),
		Email:        *email,
		DisplayName:  *displayName,
		Timezone:     "UTC",
		Locale:       "en",
		IsSuperAdmin: true,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	if err := authSvc.CreateUser(ctx, user, *password); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating user: %v\n", err)
		return 1
	}

	// 3. Add user as domain admin
	_, err = pool.Exec(ctx, `
		INSERT INTO domain_members (domain_id, user_id, role, created_at)
		VALUES ($1, $2, 'admin', $3)
		ON CONFLICT (domain_id, user_id) DO UPDATE SET role = EXCLUDED.role
	`, domainID, user.ID, time.Now().UTC())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error adding domain membership: %v\n", err)
		return 1
	}

	// 4. Seed labels (CreateUser won't have seeded them because domain_members was added after)
	_, _ = pool.Exec(ctx, `
		INSERT INTO labels (id, domain_id, user_id, name, color, is_system, created_at)
		SELECT gen_random_uuid(), $1, $2, unnest.label, '#4285f4', true, now()
		FROM LATERAL unnest(ARRAY['INBOX','SENT','DRAFTS','TRASH','JUNK','IMPORTANT','STARRED','ALL_MAIL']) AS unnest(label)
		ON CONFLICT (domain_id, user_id, name) DO NOTHING
	`, domainID, user.ID)

	fmt.Printf("Setup complete.\n")
	fmt.Printf("  Domain:  %s (ID: %s)\n", *domainName, domainID)
	fmt.Printf("  User:    %s (ID: %s)\n", user.Email, user.ID)
	fmt.Printf("  Role:    super-admin + domain admin\n")
	return 0
}
