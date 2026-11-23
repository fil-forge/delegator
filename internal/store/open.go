package store

import (
	"context"
	"fmt"

	"go.uber.org/fx"

	"github.com/fil-forge/delegator/internal/config"
)

// Open returns the store cfg.Type selects, with a function that releases its
// resources. An empty type selects DynamoDB.
func Open(ctx context.Context, cfg config.StoreConfig) (Store, func(), error) {
	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}
	switch cfg.Type {
	case config.StoreTypePostgres:
		pg, err := NewPostgresStore(ctx, cfg)
		if err != nil {
			return nil, nil, err
		}
		return pg, pg.Close, nil
	default:
		dynamo, err := NewDynamoDBStore(cfg.DynamoConfig)
		if err != nil {
			return nil, nil, err
		}
		return dynamo, func() {}, nil
	}
}

// NewFromConfig opens the configured store for the fx application and closes
// it when the application stops.
func NewFromConfig(lc fx.Lifecycle, cfg config.StoreConfig) (Store, error) {
	s, closeStore, err := Open(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize store: %w", err)
	}
	lc.Append(fx.StopHook(closeStore))
	return s, nil
}
