package providers

import (
	"context"
	"fmt"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/fil-forge/delegator/internal/config"
	"github.com/fil-forge/delegator/internal/services/registrar"
	"github.com/fil-forge/forgectl/pkg/services/chain"
	"github.com/fil-forge/forgectl/pkg/services/inspector"
	"github.com/fil-forge/forgectl/pkg/services/operator"
	"github.com/fil-forge/forgectl/pkg/services/types"
	"github.com/fil-forge/libforge/identity"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/ucan"
	"github.com/fil-forge/ucantone/ucan/delegation"
	"go.uber.org/fx"
)

type SignerParams struct {
	fx.In
	Config *config.Config
}

type SignerResult struct {
	fx.Out
	ID identity.Identity
}

func ProvideSigner(params SignerParams) (SignerResult, error) {
	var id identity.Identity
	var err error
	switch {
	case params.Config.Delegator.Key != "":
		id, err = identity.New(params.Config.Delegator.Key, params.Config.Delegator.DID)
		if err != nil {
			return SignerResult{}, fmt.Errorf("failed to parse multibase key: %w", err)
		}
	case params.Config.Delegator.KeyFile != "":
		id, err = identity.NewFromPEMFileWithDID(params.Config.Delegator.KeyFile, params.Config.Delegator.DID)
		if err != nil {
			return SignerResult{}, fmt.Errorf("failed to parse key file: %w", err)
		}
	default:
		return SignerResult{}, fmt.Errorf("no key or key file provided")
	}

	return SignerResult{ID: id}, nil
}

type IndexingServiceWebDIDParams struct {
	fx.In
	Config *config.Config
}

type IndexingServiceWebDIDResult struct {
	fx.Out
	IndexingServiceWebDID did.DID `name:"indexing_service_web_did"`
}

func ProvideIndexingServiceWebDID(params IndexingServiceWebDIDParams) (IndexingServiceWebDIDResult, error) {
	parsedDID, err := did.Parse(params.Config.Delegator.IndexingServiceWebDID)
	if err != nil {
		return IndexingServiceWebDIDResult{}, fmt.Errorf("failed to parse indexing service DID: %w", err)
	}

	return IndexingServiceWebDIDResult{IndexingServiceWebDID: parsedDID}, nil
}

type UploadServiceDIDParams struct {
	fx.In
	Config *config.Config
}

type UploadServiceDIDResult struct {
	fx.Out
	UploadServiceDID did.DID `name:"upload_service_did"`
}

func ProvideUploadServiceDID(params UploadServiceDIDParams) (UploadServiceDIDResult, error) {
	parsedDID, err := did.Parse(params.Config.Delegator.UploadServiceDID)
	if err != nil {
		return UploadServiceDIDResult{}, fmt.Errorf("failed to parse upload service DID: %w", err)
	}

	return UploadServiceDIDResult{UploadServiceDID: parsedDID}, nil
}

type IndexingServiceProofParams struct {
	fx.In
	Config *config.Config
}

type IndexingServiceProofResult struct {
	fx.Out
	IndexingServiceProof ucan.Delegation `name:"indexing_service_proof"`
}

func ProvideIndexingServiceProof(params IndexingServiceProofParams) (IndexingServiceProofResult, error) {
	var proofStr []byte

	// Prefer proof file over inline proof string
	if params.Config.Delegator.IndexingServiceProofFile != "" {
		data, err := os.ReadFile(params.Config.Delegator.IndexingServiceProofFile)
		if err != nil {
			return IndexingServiceProofResult{}, fmt.Errorf("failed to read indexing service proof file: %w", err)
		}
		proofStr = data
	} else {
		panic("indexing service proof file must be provided")
		//proofStr = params.Config.Delegator.IndexingServiceProof
	}

	proof, err := delegation.Decode(proofStr)
	if err != nil {
		return IndexingServiceProofResult{}, fmt.Errorf("failed to parse indexing service proof: %w", err)
	}

	return IndexingServiceProofResult{IndexingServiceProof: proof}, nil
}

type EgressTrackingServiceDIDParams struct {
	fx.In
	Config *config.Config
}

type EgressTrackingServiceDIDResult struct {
	fx.Out
	EgressTrackingServiceDID did.DID `name:"egress_tracking_service_did"`
}

func ProvideEgressTrackingServiceDID(params EgressTrackingServiceDIDParams) (EgressTrackingServiceDIDResult, error) {
	parsedDID, err := did.Parse(params.Config.Delegator.EgressTrackingServiceDID)
	if err != nil {
		return EgressTrackingServiceDIDResult{}, fmt.Errorf("failed to parse egress tracking service DID: %w", err)
	}

	return EgressTrackingServiceDIDResult{EgressTrackingServiceDID: parsedDID}, nil
}

type EgressTrackingServiceProofParams struct {
	fx.In
	Config *config.Config
}

type EgressTrackingServiceProofResult struct {
	fx.Out
	EgressTrackingServiceProof ucan.Delegation `name:"egress_tracking_service_proof"`
}

func ProvideEgressTrackingServiceProof(params EgressTrackingServiceProofParams) (EgressTrackingServiceProofResult, error) {
	var proofStr []byte

	// Prefer proof file over inline proof string
	if params.Config.Delegator.EgressTrackingServiceProofFile != "" {
		data, err := os.ReadFile(params.Config.Delegator.EgressTrackingServiceProofFile)
		if err != nil {
			return EgressTrackingServiceProofResult{}, fmt.Errorf("failed to read egress tracking service proof file: %w", err)
		}
		proofStr = data
	} else {
		panic("egress tracking service proof file must be provided")
		//proofStr = params.Config.Delegator.EgressTrackingServiceProof
	}

	proof, err := delegation.Decode(proofStr)
	if err != nil {
		return EgressTrackingServiceProofResult{}, fmt.Errorf("failed to parse egress tracking service proof: %w", err)
	}
	return EgressTrackingServiceProofResult{EgressTrackingServiceProof: proof}, nil
}

type SmartContractOperator struct {
	o *operator.Service
}

func (s *SmartContractOperator) IsRegisteredProvider(ctx context.Context, provider common.Address) (bool, error) {
	return s.o.RegistryContract.IsRegisteredProvider(&bind.CallOpts{Context: ctx}, provider)
}

func (s *SmartContractOperator) GetProviderByAddress(ctx context.Context, provider common.Address) (*types.ProviderInfo, error) {
	return s.o.GetProviderByAddress(ctx, provider)
}

func (s *SmartContractOperator) ApproveProvider(ctx context.Context, id uint64) (*types.ApprovalResult, error) {
	return s.o.ApproveProvider(ctx, id)
}

func ProvideContractOperator(cfg config.ContractOperatorConfig) (registrar.ContractOperator, error) {
	in, err := inspector.New(inspector.Config{
		ClientEndpoint:          cfg.ChainClientEndpoint,
		PaymentsContractAddress: common.HexToAddress(cfg.PaymentsContractAddress),
		ServiceContractAddress:  common.HexToAddress(cfg.ServiceContractAddress),
		ProviderRegistryAddress: common.HexToAddress(cfg.RegistryContractAddress),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize contract inspector: %w", err)
	}
	txtr, err := chain.NewTransactor(big.NewInt(cfg.Transactor.ChainID), chain.TransactorConfig{
		Key:              cfg.Transactor.Key,
		KeystorePath:     cfg.Transactor.KeystorePath,
		KeystorePassword: cfg.Transactor.KeystorePassword,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize contract transactor: %w", err)
	}

	op, err := operator.New(in, txtr)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize contract operator: %w", err)
	}

	return &SmartContractOperator{o: op}, nil
}
