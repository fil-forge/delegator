package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var validDynamo = DynamoConfig{
	Region:                "us-east-1",
	AllowListTableName:    "allowlist",
	ProviderInfoTableName: "providerinfo",
}

func TestStoreConfigValidateAccepts(t *testing.T) {
	validConfigs := map[string]StoreConfig{
		"type is empty and DynamoDB is configured":    {DynamoConfig: validDynamo},
		"type is dynamodb and DynamoDB is configured": {Type: StoreTypeDynamoDB, DynamoConfig: validDynamo},
		"type is postgres and the DSN is set":         {Type: StoreTypePostgres, Postgres: PostgresConfig{DSN: "postgres://localhost/delegator"}},
		"type is postgres and max conns is set":       {Type: StoreTypePostgres, Postgres: PostgresConfig{DSN: "postgres://localhost/delegator", MaxConns: 5}},
	}
	for desc, cfg := range validConfigs {
		t.Run(desc, func(t *testing.T) {
			require.NoError(t, cfg.Validate())
		})
	}
}

func TestStoreConfigValidateRejects(t *testing.T) {
	testCases := map[string]struct {
		cfg             StoreConfig
		expectedMessage string
	}{
		"DynamoDB region is missing": {
			cfg:             StoreConfig{DynamoConfig: DynamoConfig{AllowListTableName: "a", ProviderInfoTableName: "p"}},
			expectedMessage: "store region not set",
		},
		"DynamoDB allow list table is missing": {
			cfg:             StoreConfig{Type: StoreTypeDynamoDB, DynamoConfig: DynamoConfig{Region: "r", ProviderInfoTableName: "p"}},
			expectedMessage: "store allow list table not set",
		},
		"DynamoDB provider info table is missing": {
			cfg:             StoreConfig{Type: StoreTypeDynamoDB, DynamoConfig: DynamoConfig{Region: "r", AllowListTableName: "a"}},
			expectedMessage: "store provider info table not set",
		},
		"Postgres DSN is missing": {
			cfg:             StoreConfig{Type: StoreTypePostgres, DynamoConfig: validDynamo},
			expectedMessage: `store postgres dsn not set (required when store type is "postgres")`,
		},
		"Postgres max conns is negative": {
			cfg:             StoreConfig{Type: StoreTypePostgres, Postgres: PostgresConfig{DSN: "postgres://localhost/delegator", MaxConns: -1}},
			expectedMessage: "store postgres max conns must not be negative, got -1",
		},
		"type is unknown": {
			cfg:             StoreConfig{Type: "mysql", DynamoConfig: validDynamo},
			expectedMessage: `unknown store type "mysql" (valid: dynamodb, postgres)`,
		},
	}
	for desc, tc := range testCases {
		t.Run(desc, func(t *testing.T) {
			require.EqualError(t, tc.cfg.Validate(), tc.expectedMessage)
		})
	}
}
