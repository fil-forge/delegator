package cmd

import (
	"fmt"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/fil-forge/delegator/internal/config"
)

// storeFlagKeys maps each store flag to the viper key it sets.
var storeFlagKeys = map[string]string{
	"store-type":               "store.type",
	"store-region":             "store.region",
	"store-allowlist-table":    "store.allowlist_table_name",
	"store-providerinfo-table": "store.providerinfo_table_name",
	"store-provider-weight":    "store.providerweight",
	"store-endpoint":           "store.endpoint",
	"store-postgres-dsn":       "store.postgres.dsn",
	"store-postgres-max-conns": "store.postgres.max_conns",
}

// addStoreFlags defines the store flags shared by the serve and store
// commands.
func addStoreFlags(flags *pflag.FlagSet) {
	flags.String("store-type", config.StoreTypeDynamoDB, "Storage backend: dynamodb or postgres")
	flags.String("store-region", "", "AWS region for DynamoDB")
	flags.String("store-allowlist-table", "", "DynamoDB table name for allowlist")
	flags.String("store-providerinfo-table", "", "DynamoDB table name for provider info")
	flags.Uint("store-provider-weight", 1, "Default weight for registered providers")
	flags.String("store-endpoint", "", "DynamoDB endpoint (for local testing)")
	flags.String("store-postgres-dsn", "", "Postgres connection string (prefer REGISTRAR_STORE_POSTGRES_DSN, it holds the password)")
	flags.Int32("store-postgres-max-conns", 10, "Maximum number of Postgres connections")
}

// bindStoreFlags binds the store flags of the command being run to viper.
// Both serve and store define these flags, and viper keeps one flag per key,
// so binding them in init() would let the last command registered win and
// ignore the flags of the other.
func bindStoreFlags(flags *pflag.FlagSet) error {
	for flag, key := range storeFlagKeys {
		if err := viper.BindPFlag(key, flags.Lookup(flag)); err != nil {
			return fmt.Errorf("binding flag --%s: %w", flag, err)
		}
	}
	// storoku passes the allowlist table name in this variable.
	if err := viper.BindEnv("store.allowlist_table_name", "STORAGE_PROVIDER_ALLOW_LIST_TABLE_ID"); err != nil {
		return fmt.Errorf("binding allowlist table environment variable: %w", err)
	}
	return nil
}
