package config

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/viper"
	"go.uber.org/fx"
)

func SupplyConfigs(cfg *Config) fx.Option {
	return fx.Supply(
		cfg,
		cfg.Server,
		cfg.Store,
		cfg.Delegator,
		cfg.Contract,
		cfg.Contract.Transactor,
	)
}

func NewConfig() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Host: viper.GetString("server.host"),
			Port: viper.GetInt("server.port"),
		},
		Store: StoreConfig{
			Type: viper.GetString("store.type"),
			DynamoConfig: DynamoConfig{
				Region:                viper.GetString("store.region"),
				AllowListTableName:    viper.GetString("store.allowlist_table_name"),
				ProviderInfoTableName: viper.GetString("store.providerinfo_table_name"),
				ProviderWeight:        viper.GetUint("store.providerweight"),
				Endpoint:              viper.GetString("store.endpoint"),
			},
			Postgres: PostgresConfig{
				DSN:      viper.GetString("store.postgres.dsn"),
				MaxConns: viper.GetInt32("store.postgres.max_conns"),
			},
		},
		Delegator: DelegatorServiceConfig{
			Key:                            viper.GetString("delegator.key"),
			KeyFile:                        viper.GetString("delegator.key_file"),
			DID:                            viper.GetString("delegator.did"),
			IndexingServiceWebDID:          viper.GetString("delegator.indexing_service_web_did"),
			IndexingServiceProof:           viper.GetString("delegator.indexing_service_proof"),
			IndexingServiceProofFile:       viper.GetString("delegator.indexing_service_proof_file"),
			EgressTrackingServiceDID:       viper.GetString("delegator.egress_tracking_service_did"),
			EgressTrackingServiceProof:     viper.GetString("delegator.egress_tracking_service_proof"),
			EgressTrackingServiceProofFile: viper.GetString("delegator.egress_tracking_service_proof_file"),
			UploadServiceDID:               viper.GetString("delegator.upload_service_did"),
			InsecureDIDResolution:          viper.GetBool("delegator.insecure_did_resolution"),
		},
		Contract: ContractOperatorConfig{
			ChainClientEndpoint:     viper.GetString("contract.chain_client_endpoint"),
			PaymentsContractAddress: viper.GetString("contract.payments_contract_address"),
			ServiceContractAddress:  viper.GetString("contract.service_contract_address"),
			RegistryContractAddress: viper.GetString("contract.registry_contract_address"),
			Transactor: ContractTransactorConfig{
				ChainID:          viper.GetInt64("contract.transactor.chain_id"),
				Key:              viper.GetString("contract.transactor.key"),
				KeystorePath:     viper.GetString("contract.transactor.keystore_path"),
				KeystorePassword: viper.GetString("contract.transactor.keystore_password"),
			},
		},
	}

	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}

	if err := cfg.Store.Validate(); err != nil {
		return nil, err
	}

	if cfg.Delegator.Key == "" && cfg.Delegator.KeyFile == "" {
		return nil, fmt.Errorf("either delegator key or key file must be set")
	}
	if cfg.Delegator.Key != "" && cfg.Delegator.KeyFile != "" {
		return nil, fmt.Errorf("both delegator key and key file are set, please provide only one")
	}
	if cfg.Delegator.DID == "" {
		return nil, fmt.Errorf("delegator DID not set")
	}
	if cfg.Delegator.IndexingServiceWebDID == "" {
		return nil, fmt.Errorf("delegator indexing service DID not set")
	}
	if cfg.Delegator.IndexingServiceProof == "" && cfg.Delegator.IndexingServiceProofFile == "" {
		return nil, fmt.Errorf("delegator indexing service proof not set (set either indexing_service_proof or indexing_service_proof_file)")
	}
	if cfg.Delegator.IndexingServiceProof != "" && cfg.Delegator.IndexingServiceProofFile != "" {
		return nil, fmt.Errorf("both delegator indexing service proof and proof file are set, please provide only one")
	}
	if cfg.Delegator.EgressTrackingServiceDID == "" {
		return nil, fmt.Errorf("delegator egress tracking service DID not set")
	}
	if cfg.Delegator.EgressTrackingServiceProof == "" && cfg.Delegator.EgressTrackingServiceProofFile == "" {
		return nil, fmt.Errorf("delegator egress tracking service proof not set (set either egress_tracking_service_proof or egress_tracking_service_proof_file)")
	}
	if cfg.Delegator.EgressTrackingServiceProof != "" && cfg.Delegator.EgressTrackingServiceProofFile != "" {
		return nil, fmt.Errorf("both delegator egress tracking service proof and proof file are set, please provide only one")
	}
	if cfg.Delegator.UploadServiceDID == "" {
		return nil, fmt.Errorf("delegator upload service DID not set")
	}

	if !common.IsHexAddress(cfg.Contract.RegistryContractAddress) {
		return nil, fmt.Errorf("registry contract address invalid")
	}
	if !common.IsHexAddress(cfg.Contract.PaymentsContractAddress) {
		return nil, fmt.Errorf("payments contract address invalid")
	}
	if !common.IsHexAddress(cfg.Contract.ServiceContractAddress) {
		return nil, fmt.Errorf("service contract address invalid")
	}
	if cfg.Contract.ChainClientEndpoint == "" {
		return nil, fmt.Errorf("chain client endpoint not set")
	}
	// TODO we can have stronger validation here, there are only two allowed values
	// the filecoin mainnet and calibnet ID
	if cfg.Contract.Transactor.ChainID == 0 {
		return nil, fmt.Errorf("chain client id not set")
	}
	// decision on doing this will be made here: https://www.notion.so/storacha/Storacha-Forge-Contract-Billing-Operations-Design-Questions-28b5305b552480d08ea7c8a1ff077a2d?source=copy_link#28f5305b5524801e95e9f556ca7d8a9e
	// TODO this is really insecure, we may want to import the aws secret manager sdk and use directly
	if cfg.Contract.Transactor.Key == "" {
		if cfg.Contract.Transactor.KeystorePath == "" {
			return nil, fmt.Errorf("either a transactor key or a keystore must be provided")
		}
		// TODO this is really insecure, we may want to import the aws secret manager sdk and use directly
		if cfg.Contract.Transactor.KeystorePassword == "" {
			return nil, fmt.Errorf("transactor keystore password not set")
		}
	}
	if cfg.Contract.Transactor.Key != "" && cfg.Contract.Transactor.KeystorePath != "" {
		return nil, fmt.Errorf("both transactor key and keystore are set, only one can be provided")
	}
	return cfg, nil
}

type Config struct {
	Server    ServerConfig           `mapstructure:"server"`
	Store     StoreConfig            `mapstructure:"store"`
	Delegator DelegatorServiceConfig `mapstructure:"delegator"`
	Contract  ContractOperatorConfig `mapstructure:"contract"`
}
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

func (c *ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

const (
	StoreTypeDynamoDB = "dynamodb"
	StoreTypePostgres = "postgres"
)

// StoreConfig selects the storage backend and configures it. DynamoDB is the
// default, so a deployment that sets no type keeps its DynamoDB tables.
type StoreConfig struct {
	// Type is the storage backend: "dynamodb" (default) or "postgres".
	Type string `mapstructure:"type"`
	// DynamoConfig sits directly under `store`, so the DynamoDB keys and
	// REGISTRAR_STORE_* variables existing deployments set keep working.
	DynamoConfig `mapstructure:",squash"`
	Postgres     PostgresConfig `mapstructure:"postgres"`
}

// Validate checks the settings of the selected backend. An empty Type means
// DynamoDB.
func (c StoreConfig) Validate() error {
	switch c.Type {
	case StoreTypeDynamoDB, "":
		if c.Region == "" {
			return fmt.Errorf("store region not set")
		}
		if c.AllowListTableName == "" {
			return fmt.Errorf("store allow list table not set")
		}
		if c.ProviderInfoTableName == "" {
			return fmt.Errorf("store provider info table not set")
		}
	case StoreTypePostgres:
		if c.Postgres.DSN == "" {
			return fmt.Errorf("store postgres dsn not set (required when store type is %q)", StoreTypePostgres)
		}
		if c.Postgres.MaxConns < 0 {
			return fmt.Errorf("store postgres max conns must not be negative, got %d", c.Postgres.MaxConns)
		}
	default:
		return fmt.Errorf("unknown store type %q (valid: %s, %s)", c.Type, StoreTypeDynamoDB, StoreTypePostgres)
	}
	return nil
}

type PostgresConfig struct {
	// DSN is the Postgres connection string, e.g.
	// postgres://user:password@host:5432/delegator. It holds the database
	// password, so prefer setting it through REGISTRAR_STORE_POSTGRES_DSN.
	DSN string `mapstructure:"dsn"`
	// MaxConns caps the connection pool. Zero keeps pgx's default.
	MaxConns int32 `mapstructure:"max_conns"`
}

type DynamoConfig struct {
	// Region of the dynamoDB instance
	Region string `mapstructure:"region"`
	// Name of table we use for allowing users to register
	AllowListTableName string `mapstructure:"allowlist_table_name"`
	// Name of table we persist registered user data to
	ProviderInfoTableName string `mapstructure:"providerinfo_table_name"`

	// ProviderWeight is the weight that will be assigned to a provider
	// when they are registered. This value will affect their odds of being
	// selected for an upload. `0` means they will not be selected.
	ProviderWeight uint `mapstructure:"providerweight"`

	// Endpoint may be set for local testing, usually with docker, e.g.
	// docker run -p 8000:8000 amazon/dynamodb-local -jar DynamoDBLocal.jar -sharedDb
	// then set endpoint to localhost:8080
	// Do not set for production.
	Endpoint string `mapstructure:"endpoint"` // for development
}

type DelegatorServiceConfig struct {
	Key                            string `mapstructure:"key"`
	KeyFile                        string `mapstructure:"key_file"`
	DID                            string `mapstructure:"did"`
	IndexingServiceWebDID          string `mapstructure:"indexing_service_web_did"`
	IndexingServiceProof           string `mapstructure:"indexing_service_proof"`
	IndexingServiceProofFile       string `mapstructure:"indexing_service_proof_file"`
	EgressTrackingServiceDID       string `mapstructure:"egress_tracking_service_did"`
	EgressTrackingServiceProof     string `mapstructure:"egress_tracking_service_proof"`
	EgressTrackingServiceProofFile string `mapstructure:"egress_tracking_service_proof_file"`
	UploadServiceDID               string `mapstructure:"upload_service_did"`
	// InsecureDIDResolution resolves did:web over HTTP instead of HTTPS. For
	// development stacks only.
	InsecureDIDResolution bool `mapstructure:"insecure_did_resolution"`
}

type ContractOperatorConfig struct {
	ChainClientEndpoint     string                   `mapstructure:"chain_client_endpoint"`
	PaymentsContractAddress string                   `mapstructure:"payments_contract_address"`
	ServiceContractAddress  string                   `mapstructure:"service_contract_address"`
	RegistryContractAddress string                   `mapstructure:"registry_contract_address"`
	Transactor              ContractTransactorConfig `mapstructure:"transactor"`
}

type ContractTransactorConfig struct {
	ChainID          int64  `mapstructure:"chain_id"`
	Key              string `mapstructure:"key"`
	KeystorePath     string `mapstructure:"keystore_path"`
	KeystorePassword string `mapstructure:"keystore_password"`
}
