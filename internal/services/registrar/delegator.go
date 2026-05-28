package registrar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	logging "github.com/ipfs/go-log/v2"
	"go.uber.org/fx"

	"github.com/fil-forge/forgectl/pkg/services/types"
	"github.com/fil-forge/libforge/commands/claim"
	"github.com/fil-forge/libforge/commands/space/egress"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/principal"
	"github.com/fil-forge/ucantone/principal/ed25519/verifier"
	"github.com/fil-forge/ucantone/ucan"
	"github.com/fil-forge/ucantone/ucan/container"
	"github.com/fil-forge/ucantone/ucan/delegation"

	"github.com/fil-forge/delegator/internal/store"
)

var log = logging.Logger("service/delegator")

var (
	// for the unexpected
	ErrInternal = errors.New("internal error")

	// we use these to return valid http codes from the handlers using this service
	ErrDIDNotAllowed        = errors.New("did not is not allowed to register")
	ErrDIDAlreadyRegistered = errors.New("did already registered")
	ErrDIDNotRegistered     = errors.New("did not registered")
	ErrBadEndpoint          = errors.New("did not found at endpoint")
	ErrInvalidProof         = errors.New("invalid proof")
	ErrInvalidDID           = errors.New("invalid did")
	ErrInvalidSignature     = errors.New("invalid signature")

	// blockchain/smartcontract errors
	ErrContractProviderNotRegistered = errors.New("storage provider not registered with registry contract")
)

type Service struct {
	store store.Store

	signer principal.Signer

	indexingServiceWebDID did.DID
	indexingServiceProof  ucan.Delegation

	egressTrackingServiceDID   did.DID
	egressTrackingServiceProof ucan.Delegation

	uploadServiceDID did.DID

	contractOperator ContractOperator
}

type ServiceParams struct {
	fx.In

	// the store registered providers are persisted to
	Store store.Store

	// the identity of the delegator service
	Signer principal.Signer

	// the web did of the indexing service (TODO is this still required after ./well-known change?
	IndexingServiceWebDID did.DID `name:"indexing_service_web_did"`
	// proof from the indexer, delegated to this delegator, allowing it to create delegations on behalf of indexing service
	IndexingServiceProof ucan.Delegation `name:"indexing_service_proof"`

	// the did of the egress tracking service
	EgressTrackingServiceDID did.DID `name:"egress_tracking_service_did"`
	// proof from the egress tracking service, delegated to this delegator
	EgressTrackingServiceProof ucan.Delegation `name:"egress_tracking_service_proof"`

	// the did of the upload service, used for validating operator proofs are correct
	UploadServiceDID did.DID `name:"upload_service_did"`

	ContractOperator ContractOperator
}

func New(p ServiceParams) *Service {
	return &Service{
		store:                      p.Store,
		signer:                     p.Signer,
		indexingServiceWebDID:      p.IndexingServiceWebDID,
		indexingServiceProof:       p.IndexingServiceProof,
		egressTrackingServiceDID:   p.EgressTrackingServiceDID,
		egressTrackingServiceProof: p.EgressTrackingServiceProof,
		uploadServiceDID:           p.UploadServiceDID,
		contractOperator:           p.ContractOperator,
	}
}

type RegisterParams struct {
	DID           did.DID
	OwnerAddress  common.Address
	ProofSetID    uint64
	OperatorEmail string
	PublicURL     url.URL
}

func (s *Service) Register(ctx context.Context, req RegisterParams) error {
	// ensure they are allowed to register
	allowed, err := s.store.IsAllowedDID(ctx, req.DID)
	if err != nil {
		return fmt.Errorf("failed to check if DID is allowed: %w", err)
	}
	if !allowed {
		return ErrDIDNotAllowed
	}

	// ensure they haven't already registered
	registered, err := s.store.IsRegisteredDID(ctx, req.DID)
	if err != nil {
		return fmt.Errorf("failed to check if DID is registered: %w", err)
	}
	if registered {
		// TODO we may consider allowing this to succeede, if the contract is re-deployed then they need to reregister
		// alternative is we are good about clearing dynamo table across redeploys of contract
		return ErrDIDAlreadyRegistered
	}

	// ensure the did they claim to own is served from the endpoint they claim to own
	if valid, err := assertEndpointServesDID(ctx, req.PublicURL, req.DID); err != nil {
		log.Errorw("failed to assert endpoint", "DID", req.DID, "error", err)
		return err
	} else if !valid {
		return ErrBadEndpoint
	}

	// if we reach here, they have a valid unregistered did in the allow list, with a domain service the did, and valid proof
	// so we can create the provider record now.
	now := time.Now()
	if err := s.store.RegisterProvider(ctx, store.StorageProviderInfo{
		Provider:      req.DID.String(),
		Endpoint:      req.PublicURL.String(),
		Address:       req.OwnerAddress.String(),
		ProofSet:      req.ProofSetID,
		OperatorEmail: req.OperatorEmail,
		InsertedAt:    now,
		UpdatedAt:     now,
	}); err != nil {
		return fmt.Errorf("failed to register provider: %w", err)
	}
	// success!
	return nil
}

// TODO add caching for is registered
func (s *Service) IsRegisteredDID(ctx context.Context, operator did.DID) (bool, error) {
	// ensure they haven't already registered
	registered, err := s.store.IsRegisteredDID(ctx, operator)
	if err != nil {
		return false, fmt.Errorf("failed to check if DID is registered: %w", err)
	}
	return registered, nil
}

func (s *Service) RequestProofs(ctx context.Context, operator did.DID) (ucan.Container, ucan.Container, error) {
	// ensure they are allowed to register
	allowed, err := s.store.IsAllowedDID(ctx, operator)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to check if DID is allowed: %w", err)
	}
	if !allowed {
		return nil, nil, ErrDIDNotAllowed
	}

	// ensure they haven't already registered
	registered, err := s.store.IsRegisteredDID(ctx, operator)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to check if DID is registered: %w", err)
	}
	// must be registered to request a proof
	if !registered {
		return nil, nil, ErrDIDNotRegistered
	}

	// the node is in allow list, and already registered, they may haz proof
	indexerProof, err := s.generateIndexerDelegation(operator)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate indexer delegation: %w", err)
	}

	egressTrackerProof, err := s.generateEgressTrackerDelegation(operator)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate egress tracker delegation: %w", err)
	}

	return indexerProof, egressTrackerProof, nil
}

// generateIndexerDelegation mints the next hop on the chain rooted at
// s.indexingServiceProof (indexing → delegator), extending it to the storage
// node. Subject stays as the indexing service so the chain resolves at
// invocation time: indexing → delegator → storage node. The returned container
// carries both delegations so the node can pass them as proofs when it later
// invokes /claim/cache.
func (s *Service) generateIndexerDelegation(id did.DID) (ucan.Container, error) {
	isd, err := claim.Cache.Delegate(
		s.signer,
		id,
		s.indexingServiceWebDID,
		delegation.WithNoExpiration(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate delegation from indexing service to storage node: %w", err)
	}
	return container.New(
		container.WithDelegations(s.indexingServiceProof, isd),
	), nil
}

// generateEgressTrackerDelegation mints the next hop on the chain rooted at
// s.egressTrackingServiceProof (egress → delegator), extending it to the
// storage node. Subject stays as the egress tracking service so the chain
// resolves at invocation time: egress → delegator → storage node. The returned
// container carries both delegations so the node can pass them as proofs when
// it later invokes /space/egress/track.
func (s *Service) generateEgressTrackerDelegation(id did.DID) (ucan.Container, error) {
	etd, err := egress.Track.Delegate(
		s.signer,
		id,
		s.egressTrackingServiceDID,
		delegation.WithNoExpiration(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate delegation from egress tracking service to storage node: %w", err)
	}
	return container.New(
		container.WithDelegations(s.egressTrackingServiceProof, etd),
	), nil
}

func assertEndpointServesDID(ctx context.Context, endpoint url.URL, expectedDID did.DID) (bool, error) {
	// Create HTTP client with reasonable timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return false, fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("making request to %s: %w", endpoint.String(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status code %d from %s", resp.StatusCode, endpoint.String())
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("reading response body: %w", err)
	}

	// Parse the response to extract DID
	responseText := strings.TrimSpace(string(body))

	// Try to parse as JSON first (in case of structured response)
	var didResponse struct {
		DID string `json:"did"`
	}

	if err := json.Unmarshal(body, &didResponse); err == nil && didResponse.DID != "" {
		// Successfully parsed as JSON
		if didResponse.DID != expectedDID.String() {
			return false, nil
		}
		return true, nil
	}

	// TODO a dedicated DID endpoint on the storage node would be helpful here.
	// Parse plain text response to extract DID
	// Expected format: "🔥 storage v0.0.3-d6f3761-dirty\n- https://github.com/storacha/storage\n- did:key:z6MksvRCPWoXvMj8sUzuHiQ4pFkSawkKRz2eh1TALNEG6s3e"
	lines := strings.Split(responseText, "\n")
	var foundDID string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Look for lines that start with "- did:" or just "did:"
		if strings.HasPrefix(line, "- did:") {
			foundDID = strings.TrimPrefix(line, "- ")
			break
		} else if strings.HasPrefix(line, "did:") {
			foundDID = line
			break
		}
	}

	if foundDID == "" {
		return false, nil
	}

	if foundDID != expectedDID.String() {
		return false, nil
	}

	return true, nil
}

type ContractOperator interface {
	IsRegisteredProvider(ctx context.Context, provider common.Address) (bool, error)
	GetProviderByAddress(ctx context.Context, provider common.Address) (*types.ProviderInfo, error)
	ApproveProvider(ctx context.Context, id uint64) (*types.ApprovalResult, error)
}

// RequestApprovalParams contains the parameters required to request contract approval
// for a storage provider in the Storacha network.
type RequestApprovalParams struct {
	// DID is the decentralized identifier of the operator requesting approval
	Operator did.DID
	// OwnerAddress is the Ethereum address of the provider owner on the blockchain
	OwnerAddress common.Address
	// Signature is the cryptographic signature of the DID signed with the DID's private key,
	// used to prove ownership of the DID
	Signature []byte
}

// NB(forrest): this is a temporary solution until a decision here is reached:
// https://www.notion.so/storacha/Storacha-Forge-Contract-Billing-Operations-Design-Questions-28b5305b552480d08ea7c8a1ff077a2d?source=copy_link#28f5305b5524801e95e9f556ca7d8a9e

// RequestContractApproval processes a contract approval request for a storage provider.
// This method performs several validation steps before approving the provider on the blockchain:
//
//  1. Verifies the DID is in the allow list
//  2. Validates the signature to prove DID ownership (provider signs their own DID with its private key)
//  3. Confirms the provider is registered with the smart contract
//  4. Checks if the provider is already approved to avoid duplicate approval calls
//  5. If not approved, submits an approval transaction to the blockchain
//
// The approval process is NOT idempotent at the contract level - repeated calls to approve
// the same provider will fail. This method handles that by checking approval status first.
//
// Returns:
//   - nil on success (provider is approved)
//   - ErrDIDNotAllowed if the DID is not in the allow list
//   - ErrInvalidDID if the DID format is invalid
//   - ErrInvalidSignature if the signature verification fails
//   - ErrContractProviderNotRegistered if the provider is not registered with the smart contract
//   - ErrInternal for unexpected errors (database failures, contract call failures, etc.)
func (s *Service) RequestContractApproval(ctx context.Context, req RequestApprovalParams) error {
	// first check if the DID is in the allow list
	allowed, err := s.store.IsAllowedDID(ctx, req.Operator)
	if err != nil {
		log.Errorw("failed to check if DID is allowed", "DID", req.Operator, "error", err)
		return ErrInternal
	}
	if !allowed {
		return ErrDIDNotAllowed
	}

	// next validate they own the DID they claim
	v, err := verifier.Parse(req.Operator.String())
	if err != nil {
		return ErrInvalidDID
	}
	// providers sign their own DID with its private key, here we verify the signature.
	if !v.Verify([]byte(req.Operator.String()), req.Signature) {
		// logging since this may represent someone doing something nasty!
		log.Errorw("failed to verify DID", "DID", req.Operator, "signature", req.Signature)
		return ErrInvalidSignature
	}

	// check if the provider is registered with the contract, they must have register to be approved.
	registered, err := s.contractOperator.IsRegisteredProvider(ctx, req.OwnerAddress)
	if err != nil {
		log.Errorw("failed to check if provider is registered with contract", "address", req.OwnerAddress, "error", err)
		return ErrInternal
	}
	// if they are not registered bail, cannot be approved till registered
	if !registered {
		return ErrContractProviderNotRegistered
	}
	// they are registered, check if they have already been approved.
	providerInfo, err := s.contractOperator.GetProviderByAddress(ctx, req.OwnerAddress)
	if err != nil {
		// failure here is an internal error, so log it
		log.Errorw("failed to get provider info", "error", err)
		return ErrInternal
	}
	// contract approval calls are NOT idempotent, repeated calls to approve fail, so only approve if unapproved
	if !providerInfo.IsApproved {
		// approve the provider
		res, err := s.contractOperator.ApproveProvider(ctx, providerInfo.ID)
		if err != nil {
			// failure here is an internal error, so log it
			log.Errorw("failed to approve provider", "error", err)
			return ErrInternal
		}
		// TODO probably don't wanna log the whole receipt, but whatever
		log.Infow("provider approved with contract", "providerID", res.ProviderID, "transaction", res.TransactionHash, "receipt", res.Receipt)
	}
	// the provider is approved if we reach this point, success!
	return nil
}
