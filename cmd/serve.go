package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/fx"

	"github.com/fil-forge/delegator/internal/config"
	"github.com/fil-forge/delegator/internal/handlers"
	"github.com/fil-forge/delegator/internal/providers"
	"github.com/fil-forge/delegator/internal/server"
	"github.com/fil-forge/delegator/internal/services/registrar"
	"github.com/fil-forge/delegator/internal/store"
)

var ServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the HTTP server",
	Long:  `Start the registrar HTTP server with configured endpoints.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := bindStoreFlags(cmd.Flags()); err != nil {
			return err
		}
		cfg, err := config.NewConfig()
		if err != nil {
			return err
		}
		app := fx.New(
			// Configuration
			config.SupplyConfigs(cfg),
			fx.Provide(
				// Providers for complex types
				providers.ProvideSigner,
				providers.ProvideIndexingServiceWebDID,
				providers.ProvideIndexingServiceProof,
				providers.ProvideEgressTrackingServiceDID,
				providers.ProvideEgressTrackingServiceProof,
				providers.ProvideUploadServiceDID,
				providers.ProvideDIDResolver,
				providers.ProvideContractOperator,

				// Store
				store.NewFromConfig,

				// Service
				registrar.New,

				// Handlers and Server
				handlers.NewHandlers,
				server.NewServer,
			),
			fx.Invoke(server.Start),
		)

		app.Run()
		return nil
	},
}

const (
	FilecoinCalibrationNetworkChainID       = 314159
	FilecoinCalibrationNetworkChainEndpoint = "https://api.calibration.node.glif.io/rpc/v1"
)

func init() {
	// Server flags
	ServeCmd.Flags().String("host", "0.0.0.0", "Server host")
	ServeCmd.Flags().Int("port", 8080, "Server port")

	// Store flags
	addStoreFlags(ServeCmd.Flags())

	// Service flags
	ServeCmd.Flags().String("delegator-key", "", "Multibase-encoded delegator private key")
	ServeCmd.Flags().String("delegator-key-file", "", "Path to delegator private key file")
	ServeCmd.Flags().String("delegator-did", "", "DID web of the delegator")

	ServeCmd.Flags().String("delegator-indexing-service-did", "", "DID of the indexing service")
	ServeCmd.Flags().String("delegator-indexing-service-proof", "", "Base64-encoded proof from indexing service")
	ServeCmd.Flags().String("delegator-indexing-service-proof-file", "", "Path to file containing proof from indexing service")
	ServeCmd.Flags().String("delegator-egress-tracking-service-did", "", "DID of the egress tracking service")
	ServeCmd.Flags().String("delegator-egress-tracking-service-proof", "", "Base64-encoded proof from egress tracking service")
	ServeCmd.Flags().String("delegator-egress-tracking-service-proof-file", "", "Path to file containing proof from egress tracking service")
	ServeCmd.Flags().String("delegator-upload-service-did", "", "DID of the upload service")
	ServeCmd.Flags().Bool("delegator-insecure-did-resolution", false, "Resolve did:web over HTTP instead of HTTPS (development only)")

	// Contract operator flags
	ServeCmd.Flags().String("contract-chain-client-endpoint", FilecoinCalibrationNetworkChainEndpoint, "Blockchain client RPC endpoint URL")
	ServeCmd.Flags().String("contract-payments-contract-address", "", "Ethereum address of the payments contract")
	ServeCmd.Flags().String("contract-service-contract-address", "", "Ethereum address of the service contract")
	ServeCmd.Flags().String("contract-registry-contract-address", "", "Ethereum address of the registry contract")
	ServeCmd.Flags().Int64("contract-transactor-chain-id", FilecoinCalibrationNetworkChainID, "Chain ID for blockchain transactions")
	ServeCmd.Flags().String("contract-transactor-key", "", "Ethereum private key for transaction signing")
	ServeCmd.Flags().String("contract-transactor-keystore-path", "", "Path to Ethereum keystore file for transaction signing")
	ServeCmd.Flags().String("contract-transactor-keystore-password", "", "Password for the Ethereum keystore file")

	// Bind flags to viper
	cobra.CheckErr(viper.BindPFlag("server.host", ServeCmd.Flags().Lookup("host")))
	cobra.CheckErr(viper.BindPFlag("server.port", ServeCmd.Flags().Lookup("port")))

	// Store flags are bound in RunE, see bindStoreFlags.

	cobra.CheckErr(viper.BindPFlag("delegator.key", ServeCmd.Flags().Lookup("delegator-key")))
	cobra.CheckErr(viper.BindPFlag("delegator.key_file", ServeCmd.Flags().Lookup("delegator-key-file")))
	cobra.CheckErr(viper.BindPFlag("delegator.did", ServeCmd.Flags().Lookup("delegator-did")))
	cobra.CheckErr(viper.BindPFlag("delegator.indexing_service_web_did", ServeCmd.Flags().Lookup("delegator-indexing-service-did")))
	cobra.CheckErr(viper.BindPFlag("delegator.indexing_service_proof", ServeCmd.Flags().Lookup("delegator-indexing-service-proof")))
	cobra.CheckErr(viper.BindPFlag("delegator.indexing_service_proof_file", ServeCmd.Flags().Lookup("delegator-indexing-service-proof-file")))
	cobra.CheckErr(viper.BindPFlag("delegator.egress_tracking_service_did", ServeCmd.Flags().Lookup("delegator-egress-tracking-service-did")))
	cobra.CheckErr(viper.BindPFlag("delegator.egress_tracking_service_proof", ServeCmd.Flags().Lookup("delegator-egress-tracking-service-proof")))
	cobra.CheckErr(viper.BindPFlag("delegator.egress_tracking_service_proof_file", ServeCmd.Flags().Lookup("delegator-egress-tracking-service-proof-file")))
	cobra.CheckErr(viper.BindPFlag("delegator.upload_service_did", ServeCmd.Flags().Lookup("delegator-upload-service-did")))
	cobra.CheckErr(viper.BindPFlag("delegator.insecure_did_resolution", ServeCmd.Flags().Lookup("delegator-insecure-did-resolution")))

	cobra.CheckErr(viper.BindPFlag("contract.chain_client_endpoint", ServeCmd.Flags().Lookup("contract-chain-client-endpoint")))
	cobra.CheckErr(viper.BindPFlag("contract.payments_contract_address", ServeCmd.Flags().Lookup("contract-payments-contract-address")))
	cobra.CheckErr(viper.BindPFlag("contract.service_contract_address", ServeCmd.Flags().Lookup("contract-service-contract-address")))
	cobra.CheckErr(viper.BindPFlag("contract.registry_contract_address", ServeCmd.Flags().Lookup("contract-registry-contract-address")))
	cobra.CheckErr(viper.BindPFlag("contract.transactor.chain_id", ServeCmd.Flags().Lookup("contract-transactor-chain-id")))
	cobra.CheckErr(viper.BindPFlag("contract.transactor.key", ServeCmd.Flags().Lookup("contract-transactor-key")))
	cobra.CheckErr(viper.BindPFlag("contract.transactor.keystore_path", ServeCmd.Flags().Lookup("contract-transactor-keystore-path")))
	cobra.CheckErr(viper.BindPFlag("contract.transactor.keystore_password", ServeCmd.Flags().Lookup("contract-transactor-keystore-password")))
}
