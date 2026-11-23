package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"time"

	"github.com/fil-forge/ucantone/did"
	logging "github.com/ipfs/go-log/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/fil-forge/delegator/internal/config"
)

var pgLog = logging.Logger("store/postgres")

//go:embed migrations/*.sql
var migrationsFS embed.FS

// uniqueViolation is the Postgres error code for a duplicate primary key.
const uniqueViolation = "23505"

// Postgres provides storage via PostgreSQL.
type Postgres struct {
	pool           *pgxpool.Pool
	providerWeight uint
}

// NewPostgresStore connects to the database behind cfg.Postgres.DSN and
// applies any pending schema migrations before returning. Call Close to
// release the connection pool.
func NewPostgresStore(ctx context.Context, cfg config.StoreConfig) (*Postgres, error) {
	if uint64(cfg.ProviderWeight) > math.MaxInt64 {
		return nil, fmt.Errorf("provider weight %d does not fit the weight column (max %d)", cfg.ProviderWeight, int64(math.MaxInt64))
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.Postgres.DSN)
	if err != nil {
		// pgx error messages redact the password, so the DSN is safe here.
		return nil, fmt.Errorf("parsing postgres DSN: %w", err)
	}
	if cfg.Postgres.MaxConns > 0 {
		poolCfg.MaxConns = cfg.Postgres.MaxConns
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("creating postgres pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connecting to postgres at %s:%d/%s: %w", poolCfg.ConnConfig.Host, poolCfg.ConnConfig.Port, poolCfg.ConnConfig.Database, err)
	}
	if err := migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}

	pgLog.Infow("Postgres store initialized",
		"host", poolCfg.ConnConfig.Host,
		"port", poolCfg.ConnConfig.Port,
		"database", poolCfg.ConnConfig.Database,
		"max_conns", poolCfg.MaxConns)
	return &Postgres{pool: pool, providerWeight: cfg.ProviderWeight}, nil
}

// Close releases the connection pool.
func (p *Postgres) Close() {
	p.pool.Close()
}

// IsAllowedDID checks if a DID is allowed for onboarding.
func (p *Postgres) IsAllowedDID(ctx context.Context, id did.DID) (bool, error) {
	var allowed bool
	err := p.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM allow_list WHERE did = $1)`, id.String()).Scan(&allowed)
	if err != nil {
		return false, fmt.Errorf("failed to check DID %s allowlist: %w", id.String(), err)
	}
	return allowed, nil
}

// AddAllowedDID adds a DID to the allowlist. Adding a DID that is already on
// the list succeeds and keeps the original record.
func (p *Postgres) AddAllowedDID(ctx context.Context, id did.DID) error {
	_, err := p.pool.Exec(ctx,
		`INSERT INTO allow_list (did, added_by, notes) VALUES ($1, $2, $3)
		 ON CONFLICT (did) DO NOTHING`,
		id.String(), "system", "added by delegator store cli command")
	if err != nil {
		return fmt.Errorf("failed to add DID to allowlist: %w", err)
	}
	return nil
}

// RemoveAllowedDID removes a DID from the allowlist. Removing a DID that is
// not on the list succeeds.
func (p *Postgres) RemoveAllowedDID(ctx context.Context, id did.DID) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM allow_list WHERE did = $1`, id.String())
	if err != nil {
		return fmt.Errorf("failed to remove DID from allowlist: %w", err)
	}
	return nil
}

// IsRegisteredDID checks if a DID is registered as a provider.
func (p *Postgres) IsRegisteredDID(ctx context.Context, id did.DID) (bool, error) {
	var registered bool
	err := p.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM provider_info WHERE provider = $1)`, id.String()).Scan(&registered)
	if err != nil {
		return false, fmt.Errorf("failed to check provider registration: %w", err)
	}
	return registered, nil
}

// RegisterProvider records a new provider. It fails if the provider is
// already registered.
func (p *Postgres) RegisterProvider(ctx context.Context, info StorageProviderInfo) error {
	if info.Proofs == "" {
		return fmt.Errorf("proofs must be provided")
	}
	if info.ProofSet > math.MaxInt64 {
		return fmt.Errorf("proof set %d does not fit the proof_set column (max %d)", info.ProofSet, int64(math.MaxInt64))
	}

	pgLog.Infow("Registering provider",
		"did", info.Provider,
		"endpoint", info.Endpoint)

	now := time.Now()
	_, err := p.pool.Exec(ctx,
		`INSERT INTO provider_info
		   (provider, endpoint, address, proof_set, operator_email, proofs, weight, inserted_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)`,
		info.Provider,
		info.Endpoint,
		info.Address,
		int64(info.ProofSet),
		info.OperatorEmail,
		info.Proofs,
		int64(p.providerWeight),
		now,
	)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return fmt.Errorf("failed to register provider: provider %s is already registered", info.Provider)
	}
	if err != nil {
		return fmt.Errorf("failed to register provider: %w", err)
	}
	return nil
}

// migrate applies the pending migrations embedded in migrationsFS. A Postgres
// advisory lock serializes it, so service instances starting at the same time
// do not run the same migration twice.
func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	migrations, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("opening embedded migrations: %w", err)
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("creating migration lock: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations,
		goose.WithSessionLocker(locker),
		goose.WithDisableGlobalRegistry(true),
	)
	if err != nil {
		return fmt.Errorf("creating migration provider: %w", err)
	}

	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("running postgres migrations: %w", err)
	}
	for _, result := range results {
		pgLog.Infow("Applied migration", "source", result.Source.Path, "duration", result.Duration)
	}
	version, err := provider.GetDBVersion(ctx)
	if err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}
	pgLog.Infow("Postgres schema up to date", "version", version, "applied", len(results))
	return nil
}

var _ Store = (*Postgres)(nil)
