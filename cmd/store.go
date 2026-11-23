package cmd

import (
	"context"
	"fmt"

	"github.com/fil-forge/ucantone/did"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/fil-forge/delegator/internal/config"
	"github.com/fil-forge/delegator/internal/store"
)

var StoreCmd = &cobra.Command{
	Use:   "store",
	Short: "Manage store operations",
	Long:  `Commands for managing the store (DynamoDB or Postgres), including allowlist management.`,
}

var allowDIDCmd = &cobra.Command{
	Use:   "allow-did [did]",
	Short: "Add a DID to the allowlist",
	Long:  `Add a DID to the allowlist. If the DID is already in the list, the command returns success (idempotent).`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		didStr := args[0]

		// Validate DID
		parsedDID, err := did.Parse(didStr)
		if err != nil {
			return fmt.Errorf("invalid DID format: %w", err)
		}

		// Initialize store
		cfg := &config.Config{}
		if err := viper.Unmarshal(cfg); err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		ctx := context.Background()
		db, closeStore, err := store.Open(ctx, cfg.Store)
		if err != nil {
			return fmt.Errorf("failed to initialize store: %w", err)
		}
		defer closeStore()

		// Check if DID is already allowed
		allowed, err := db.IsAllowedDID(ctx, parsedDID)
		if err != nil {
			return fmt.Errorf("failed to check DID status: %w", err)
		}

		if allowed {
			fmt.Printf("DID %s is already allowed\n", didStr)
			return nil
		}

		// Add DID to allowlist
		err = db.AddAllowedDID(ctx, parsedDID)
		if err != nil {
			return fmt.Errorf("failed to add DID to allowlist: %w", err)
		}

		fmt.Printf("Successfully added DID %s to allowlist\n", didStr)
		return nil
	},
}

var disallowDIDCmd = &cobra.Command{
	Use:   "disallow-did [did]",
	Short: "Remove a DID from the allowlist",
	Long:  `Remove a DID from the allowlist. If the DID is not in the list, the command returns success (idempotent).`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		didStr := args[0]

		// Validate DID
		parsedDID, err := did.Parse(didStr)
		if err != nil {
			return fmt.Errorf("invalid DID format: %w", err)
		}

		// Initialize store
		cfg := &config.Config{}
		if err := viper.Unmarshal(cfg); err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		ctx := context.Background()
		db, closeStore, err := store.Open(ctx, cfg.Store)
		if err != nil {
			return fmt.Errorf("failed to initialize store: %w", err)
		}
		defer closeStore()

		// Check if DID is allowed
		allowed, err := db.IsAllowedDID(ctx, parsedDID)
		if err != nil {
			return fmt.Errorf("failed to check DID status: %w", err)
		}

		if !allowed {
			fmt.Printf("DID %s is not in the allowlist\n", didStr)
			return nil
		}

		// Remove DID from allowlist
		err = db.RemoveAllowedDID(ctx, parsedDID)
		if err != nil {
			return fmt.Errorf("failed to remove DID from allowlist: %w", err)
		}

		fmt.Printf("Successfully removed DID %s from allowlist\n", didStr)
		return nil
	},
}

func init() {
	addStoreFlags(StoreCmd.PersistentFlags())
	StoreCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		return bindStoreFlags(cmd.Flags())
	}

	// Add subcommands
	StoreCmd.AddCommand(allowDIDCmd)
	StoreCmd.AddCommand(disallowDIDCmd)
}
