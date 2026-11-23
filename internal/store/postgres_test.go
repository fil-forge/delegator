package store_test

import (
	"context"
	"fmt"
	"math"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/multikey/ed25519"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/fil-forge/delegator/internal/config"
	"github.com/fil-forge/delegator/internal/store"
)

const testProviderWeight = 7

func TestPostgresIsAllowedDIDReturnsFalseForUnknownDID(t *testing.T) {
	s := newTestStore(t)

	allowed, err := s.IsAllowedDID(t.Context(), newDID(t))
	require.NoError(t, err)
	require.False(t, allowed)
}

func TestPostgresIsAllowedDIDReturnsTrueAfterAddAllowedDID(t *testing.T) {
	s := newTestStore(t)
	id := newDID(t)
	require.NoError(t, s.AddAllowedDID(t.Context(), id))

	allowed, err := s.IsAllowedDID(t.Context(), id)
	require.NoError(t, err)
	require.True(t, allowed)
}

func TestPostgresAddAllowedDIDIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	id := newDID(t)
	require.NoError(t, s.AddAllowedDID(t.Context(), id))

	require.NoError(t, s.AddAllowedDID(t.Context(), id))
}

func TestPostgresAddAllowedDIDRecordsWhoAddedIt(t *testing.T) {
	s, pool := newTestStoreWithPool(t)
	id := newDID(t)
	require.NoError(t, s.AddAllowedDID(t.Context(), id))

	var row struct{ AddedBy, Notes string }
	err := pool.QueryRow(t.Context(), `SELECT added_by, notes FROM allow_list WHERE did = $1`, id.String()).Scan(&row.AddedBy, &row.Notes)
	require.NoError(t, err)
	require.Equal(t, struct{ AddedBy, Notes string }{"system", "added by delegator store cli command"}, row)
}

func TestPostgresIsAllowedDIDReturnsFalseAfterRemoveAllowedDID(t *testing.T) {
	s := newTestStore(t)
	id := newDID(t)
	require.NoError(t, s.AddAllowedDID(t.Context(), id))
	require.NoError(t, s.RemoveAllowedDID(t.Context(), id))

	allowed, err := s.IsAllowedDID(t.Context(), id)
	require.NoError(t, err)
	require.False(t, allowed)
}

func TestPostgresRemoveAllowedDIDSucceedsForUnknownDID(t *testing.T) {
	s := newTestStore(t)

	require.NoError(t, s.RemoveAllowedDID(t.Context(), newDID(t)))
}

func TestPostgresIsRegisteredDIDReturnsFalseForUnknownDID(t *testing.T) {
	s := newTestStore(t)

	registered, err := s.IsRegisteredDID(t.Context(), newDID(t))
	require.NoError(t, err)
	require.False(t, registered)
}

func TestPostgresIsRegisteredDIDReturnsTrueAfterRegisterProvider(t *testing.T) {
	s := newTestStore(t)
	info := newProviderInfo(t)
	require.NoError(t, s.RegisterProvider(t.Context(), info))

	registered, err := s.IsRegisteredDID(t.Context(), mustParseDID(t, info.Provider))
	require.NoError(t, err)
	require.True(t, registered)
}

func TestPostgresRegisterProviderStoresAllFields(t *testing.T) {
	s, pool := newTestStoreWithPool(t)
	info := newProviderInfo(t)
	before := time.Now().Add(-time.Second)
	require.NoError(t, s.RegisterProvider(t.Context(), info))

	type storedRow struct {
		Provider, Endpoint, Address, OperatorEmail, Proofs string
		ProofSet, Weight                                   int64
		InsertedAtIsRecent, UpdatedAtEqualsInsertedAt      bool
	}
	var row storedRow
	var insertedAt, updatedAt time.Time
	err := pool.QueryRow(t.Context(),
		`SELECT provider, endpoint, address, operator_email, proofs, proof_set, weight, inserted_at, updated_at
		 FROM provider_info WHERE provider = $1`, info.Provider).
		Scan(&row.Provider, &row.Endpoint, &row.Address, &row.OperatorEmail, &row.Proofs, &row.ProofSet, &row.Weight, &insertedAt, &updatedAt)
	require.NoError(t, err)
	row.InsertedAtIsRecent = insertedAt.After(before)
	row.UpdatedAtEqualsInsertedAt = updatedAt.Equal(insertedAt)

	require.Equal(t, storedRow{
		Provider:                  info.Provider,
		Endpoint:                  info.Endpoint,
		Address:                   info.Address,
		OperatorEmail:             info.OperatorEmail,
		Proofs:                    info.Proofs,
		ProofSet:                  int64(info.ProofSet),
		Weight:                    testProviderWeight,
		InsertedAtIsRecent:        true,
		UpdatedAtEqualsInsertedAt: true,
	}, row)
}

func TestPostgresRegisterProviderRejects(t *testing.T) {
	testCases := map[string]struct {
		modify          func(*store.StorageProviderInfo)
		expectedMessage string
	}{
		"empty proofs": {
			modify:          func(info *store.StorageProviderInfo) { info.Proofs = "" },
			expectedMessage: "proofs must be provided",
		},
		"a proof set above the BIGINT range": {
			modify:          func(info *store.StorageProviderInfo) { info.ProofSet = math.MaxInt64 + 1 },
			expectedMessage: "proof set 9223372036854775808 does not fit the proof_set column (max 9223372036854775807)",
		},
	}
	s := newTestStore(t)
	for desc, tc := range testCases {
		t.Run(desc, func(t *testing.T) {
			info := newProviderInfo(t)
			tc.modify(&info)
			require.EqualError(t, s.RegisterProvider(t.Context(), info), tc.expectedMessage)
		})
	}
}

func TestPostgresRegisterProviderRejectsAlreadyRegisteredProvider(t *testing.T) {
	s := newTestStore(t)
	info := newProviderInfo(t)
	require.NoError(t, s.RegisterProvider(t.Context(), info))

	err := s.RegisterProvider(t.Context(), info)
	require.EqualError(t, err, fmt.Sprintf("failed to register provider: provider %s is already registered", info.Provider))
}

func TestPostgresStoreKeepsDataAcrossRestarts(t *testing.T) {
	cfg := newTestStoreConfig(t)
	id := newDID(t)
	first := openStore(t, cfg)
	require.NoError(t, first.AddAllowedDID(t.Context(), id))

	second := openStore(t, cfg)
	allowed, err := second.IsAllowedDID(t.Context(), id)
	require.NoError(t, err)
	require.True(t, allowed)
}

// Service instances that start together migrate the same database at once.
// The migration lock makes the late ones wait and then find nothing to do.
func TestPostgresConcurrentStartupsAllSucceed(t *testing.T) {
	cfg := newTestStoreConfig(t)
	const instances = 4
	errs := make(chan error, instances)
	var wg sync.WaitGroup
	for range instances {
		wg.Go(func() {
			_, closeStore, err := store.Open(t.Context(), cfg)
			if err == nil {
				closeStore()
			}
			errs <- err
		})
	}
	wg.Wait()
	close(errs)

	var failures []error
	for err := range errs {
		if err != nil {
			failures = append(failures, err)
		}
	}
	require.Empty(t, failures)
}

func newTestStore(t *testing.T) store.Store {
	t.Helper()
	s, _ := newTestStoreWithPool(t)
	return s
}

// newTestStoreWithPool returns a store on an empty database of its own, and a
// pool on the same database for inspecting what the store wrote.
func newTestStoreWithPool(t *testing.T) (store.Store, *pgxpool.Pool) {
	t.Helper()
	cfg := newTestStoreConfig(t)
	s := openStore(t, cfg)
	pool, err := pgxpool.New(t.Context(), cfg.Postgres.DSN)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return s, pool
}

func openStore(t *testing.T, cfg config.StoreConfig) store.Store {
	t.Helper()
	s, closeStore, err := store.Open(t.Context(), cfg)
	require.NoError(t, err)
	t.Cleanup(closeStore)
	return s
}

// newTestStoreConfig creates an empty database in the shared Postgres
// container and returns a Postgres store config pointing at it.
func newTestStoreConfig(t *testing.T) config.StoreConfig {
	t.Helper()
	adminDSN := startPostgres(t)
	name := fmt.Sprintf("test_%d", databaseCounter.Add(1))

	admin, err := pgx.Connect(t.Context(), adminDSN)
	require.NoError(t, err)
	defer admin.Close(context.Background())
	_, err = admin.Exec(t.Context(), "CREATE DATABASE "+name)
	require.NoError(t, err)

	cfg, err := pgx.ParseConfig(adminDSN)
	require.NoError(t, err)
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable", cfg.User, cfg.Password, cfg.Host, cfg.Port, name)
	return config.StoreConfig{
		Type: config.StoreTypePostgres,
		DynamoConfig: config.DynamoConfig{
			ProviderWeight: testProviderWeight,
		},
		Postgres: config.PostgresConfig{DSN: dsn},
	}
}

var (
	databaseCounter atomic.Int64

	postgresOnce      sync.Once
	postgresDSN       string
	postgresErr       error
	postgresContainer *tcpostgres.PostgresContainer
)

// startPostgres starts one Postgres container for the whole package and
// returns the DSN of its default database. It skips the test when Docker is
// not available, except on Linux CI, where Docker is expected.
func startPostgres(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("postgres store tests require Docker")
	}
	if os.Getenv("CI") == "" || runtime.GOOS != "linux" {
		testcontainers.SkipIfProviderIsNotHealthy(t)
	}
	postgresOnce.Do(func() {
		ctx := context.Background()
		postgresContainer, postgresErr = tcpostgres.Run(ctx,
			"postgres:16-alpine",
			tcpostgres.WithDatabase("delegator"),
			tcpostgres.WithUsername("delegator"),
			tcpostgres.WithPassword("delegator"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(60*time.Second),
			),
		)
		if postgresErr != nil {
			return
		}
		postgresDSN, postgresErr = postgresContainer.ConnectionString(ctx, "sslmode=disable")
	})
	require.NoError(t, postgresErr, "starting the Postgres container")
	return postgresDSN
}

func TestMain(m *testing.M) {
	code := m.Run()
	if postgresContainer != nil {
		if err := testcontainers.TerminateContainer(postgresContainer); err != nil {
			fmt.Fprintf(os.Stderr, "terminating the Postgres container: %v\n", err)
		}
	}
	os.Exit(code)
}

func newDID(t *testing.T) did.DID {
	t.Helper()
	issuer, err := ed25519.GenerateIssuer()
	require.NoError(t, err)
	return issuer.DID()
}

func mustParseDID(t *testing.T, s string) did.DID {
	t.Helper()
	id, err := did.Parse(s)
	require.NoError(t, err)
	return id
}

func newProviderInfo(t *testing.T) store.StorageProviderInfo {
	t.Helper()
	return store.StorageProviderInfo{
		Provider:      newDID(t).String(),
		Endpoint:      "https://storage.example.com",
		Address:       "0x1234567890abcdef1234567890abcdef12345678",
		ProofSet:      42,
		OperatorEmail: "operator@example.com",
		Proofs:        "proofs-container",
	}
}
