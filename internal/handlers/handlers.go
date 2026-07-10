package handlers

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/fil-forge/delegator/internal/services/registrar"
	"github.com/fil-forge/libforge/identity"
	"github.com/fil-forge/ucantone/did"
	"github.com/fil-forge/ucantone/ucan/container"
	"github.com/labstack/echo/v4"
)

type Handlers struct {
	id      identity.Identity
	service *registrar.Service
}

func NewHandlers(svcID identity.Identity, svc *registrar.Service) *Handlers {
	return &Handlers{
		id:      svcID,
		service: svc,
	}
}

func (h *Handlers) HealthCheck(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"status": "healthy",
	})
}

func (h *Handlers) Root(c echo.Context) error {
	return c.String(http.StatusOK, "hello")
}

func (h *Handlers) DIDDocument(c echo.Context) error {
	doc, err := h.id.DIDDocument()
	if err != nil {
		log.Error("failed to get DID document", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to get DID document",
		})
	}
	return c.JSON(http.StatusOK, doc)
}

type RegisterRequest struct {
	Operator      string `json:"operator"`
	OwnerAddress  string `json:"owner_address"`
	ProofSetID    uint64 `json:"proof_set_id"`
	OperatorEmail string `json:"operator_email"`
	PublicURL     string `json:"public_url"`
	// Proofs is a UCAN container delegating the upload service the capabilities
	// required of a storage provider.
	Proofs string `json:"proofs"`
}

func (h *Handlers) Register(c echo.Context) error {
	var req RegisterRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	// parse and validate request
	operator, err := did.Parse(req.Operator)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid DID"})
	}
	if !common.IsHexAddress(req.OwnerAddress) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid owner address"})
	}
	endpoint, err := url.Parse(req.PublicURL)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid public URL"})
	}
	proofs, err := container.Decode([]byte(req.Proofs))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid proofs"})
	}

	if err := h.service.Register(c.Request().Context(), registrar.RegisterParams{
		DID:           operator,
		OwnerAddress:  common.HexToAddress(req.OwnerAddress),
		ProofSetID:    req.ProofSetID,
		OperatorEmail: req.OperatorEmail,
		PublicURL:     *endpoint,
		Proofs:        proofs,
	}); err != nil {
		var status int
		var message string
		switch {
		case errors.Is(err, registrar.ErrContractProviderNotRegistered):
			status = http.StatusUnprocessableEntity
			message = "provider not registered with smart-contract, must register first"
		case errors.Is(err, registrar.ErrDIDNotAllowed):
			status = http.StatusForbidden
			message = "DID not allowed to register, contact Storacha team for help registering"
		case errors.Is(err, registrar.ErrDIDAlreadyRegistered):
			status = http.StatusConflict
			message = "DID already registered"
		case errors.Is(err, registrar.ErrBadEndpoint):
			status = http.StatusBadRequest
			message = "invalid public URL"
		case errors.Is(err, registrar.ErrInvalidProof):
			status = http.StatusBadRequest
			message = "invalid proof"
		default:
			status = http.StatusInternalServerError
			message = err.Error()
		}
		log.Error("failed to register", "operator", operator, "error", err)
		return c.JSON(status, map[string]string{"error": message})
	}

	return c.NoContent(http.StatusCreated)
}

func (h *Handlers) RequestProof(c echo.Context) error {
	return c.String(http.StatusGone, "this endpoint is deprecated, use /registrar/request-proofs instead")
}

type RequestProofsRequest struct {
	DID string `json:"did"`
}

type RequestProofsResponse struct {
	Proofs Proofs `json:"proofs"`
}

type Proofs struct {
	Indexer       []byte `json:"indexer"`
	EgressTracker []byte `json:"egress_tracker"`
}

func (h *Handlers) RequestProofs(c echo.Context) error {
	var req RequestProofsRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	operator, err := did.Parse(req.DID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid DID"})
	}

	indexerProof, egressTrackerProof, err := h.service.RequestProofs(c.Request().Context(), operator)
	if err != nil {
		// Map service errors to appropriate HTTP status codes
		status := http.StatusInternalServerError
		if errors.Is(err, registrar.ErrDIDNotAllowed) || errors.Is(err, registrar.ErrDIDNotRegistered) {
			status = http.StatusForbidden
		}

		return c.JSON(status, map[string]string{
			"error": err.Error(),
		})
	}

	indexerProofStr, err := container.Encode(container.RawGzip, indexerProof)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to read generated indexer proof",
		})
	}

	egressTrackerProofStr, err := container.Encode(container.RawGzip, egressTrackerProof)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to read generated egress tracker proof",
		})
	}

	return c.JSON(http.StatusOK, RequestProofsResponse{Proofs: Proofs{
		Indexer:       indexerProofStr,
		EgressTracker: egressTrackerProofStr,
	}})
}

type IsRegisteredRequest struct {
	DID string `json:"did"`
}

func (h *Handlers) IsRegistered(c echo.Context) error {
	var req IsRegisteredRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	operator, err := did.Parse(req.DID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid DID"})
	}

	registered, err := h.service.IsRegisteredDID(c.Request().Context(), operator)
	if err != nil {
		// TODO map the errors the service returns to http codes
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
	}

	if registered {
		return c.NoContent(http.StatusOK)
	}

	return c.NoContent(http.StatusNotFound)
}

// ContractApprovalRequest represents a request to approve a provider for registration
// before full registration with the Storacha network.
type ContractApprovalRequest struct {
	// DID is the decentralized identifier of the operator requesting approval
	Operator string `json:"operator"`
	// OwnerAddress is the Ethereum address of the provider owner (hex format)
	OwnerAddress string `json:"owner_address"`
	// Signature is the cryptographic signature proving ownership/authorization
	Signature []byte `json:"signature"`
}

// RequestContractApproval handles HTTP requests for contract approval. It validates
// the operator's DID, owner address, and signature, then processes the approval request
// through the registrar service. The provider must already be registered with the
// smart contract and be on the allow list before approval can be granted.
//
// Returns:
//   - 204 No Content on success
//   - 400 Bad Request for invalid input (DID, owner address, signature, or public URL)
//   - 403 Forbidden if the DID is not on the allow list
//   - 422 Unprocessable Entity if the provider is not registered with the smart contract
//   - 500 Internal Server Error for unexpected errors
func (h *Handlers) RequestContractApproval(c echo.Context) error {
	var req ContractApprovalRequest
	if err := c.Bind(&req); err != nil {
		return c.String(http.StatusBadRequest, "invalid request body")
	}

	// parse and validate request
	operator, err := did.Parse(req.Operator)
	if err != nil {
		return c.String(http.StatusBadRequest, "invalid DID")
	}
	if !common.IsHexAddress(req.OwnerAddress) {
		return c.String(http.StatusBadRequest, "invalid OwnerAddress")
	}
	if len(req.Signature) == 0 {
		return c.String(http.StatusBadRequest, "invalid signature")
	}
	if err := h.service.RequestContractApproval(c.Request().Context(), registrar.RequestApprovalParams{
		Operator:     operator,
		OwnerAddress: common.HexToAddress(req.OwnerAddress),
		Signature:    req.Signature,
	}); err != nil {
		if errors.Is(err, registrar.ErrContractProviderNotRegistered) {
			return c.String(http.StatusUnprocessableEntity, "Provider not registered with smart-contract, must register first")
		}
		if errors.Is(err, registrar.ErrDIDNotAllowed) {
			return c.String(http.StatusForbidden, "DID not allowed to register, contact Storacha team for help registering")
		}
		if errors.Is(err, registrar.ErrInvalidDID) {
			return c.String(http.StatusBadRequest, "invalid DID")
		}
		if errors.Is(err, registrar.ErrInvalidSignature) {
			return c.String(http.StatusBadRequest, "signature is invalid")
		}
		log.Error("failed to request contract approval", "error", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	return c.NoContent(http.StatusNoContent)
}
