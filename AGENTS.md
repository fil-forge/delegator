# AGENTS.md — delegator

Guidance for engineers and AI agents working in this repo. The code is ground
truth; if this file disagrees with the code, trust the code and update this file.

## Purpose

Go HTTP service that manages storage provider onboarding for the network:

- Validates a registering provider's identity (DID allowlist, endpoint check,
  UCAN proof validation) and records the registration in DynamoDB.
- Issues UCAN delegation chains that let registered storage nodes invoke
  `/claim/cache` on the indexing service and `/space/egress/track` on the
  egress tracking service.
- Handles on-chain contract approval of providers via the provider registry
  smart contract (through `forgectl`).

Module: `github.com/fil-forge/delegator` (Go 1.27).

## Build / Test / Run

```bash
make build          # go build -o bin/delegator ./main.go
make test           # go test -v ./...
make run            # build + run
./bin/delegator serve --host 0.0.0.0 --port 8080
```

System tests live in `test/system_test.go` (~1000 lines): full server
lifecycle, all endpoints, end-to-end registration, mock storage node
interactions, concurrency, validation failures.

```bash
go test -v ./test/...                              # system tests only
go test -v ./test/... -run TestSystemRegistrationFlow
```

## Layout

```
main.go                  # entry point
cmd/                     # Cobra CLI: root ("registrar"), serve, store, gen
  serve.go               #   start the HTTP server
  store.go               #   store allow-did / disallow-did (allowlist management)
  gen.go                 #   generate a UCAN delegation
internal/
  config/config.go       # Viper config structs (server, store, delegator, contract operator)
  server/server.go       # Echo server + route table
  handlers/handlers.go   # HTTP handlers (thin; delegate to registrar service)
  services/registrar/    # Core logic: registration, proof validation, delegation minting,
    delegator.go         #   contract approval
  providers/providers.go # uber/fx dependency providers (signer, DIDs, proofs, contract operator)
  store/                 # DynamoDB persistence (Store interface, DynamoDB impl)
client/client.go         # Go client library for external consumers
test/system_test.go      # system test suite
deploy/                  # Terraform (app/ + shared/) using storoku modules
```

## HTTP API

Routes are wired in `internal/server/server.go`:

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/` | Connectivity check (plain "hello") |
| GET | `/healthcheck` | Health check |
| GET | `/.well-known/did.json` | Service DID document |
| PUT | `/registrar/register-node` | Register a storage provider |
| POST | `/registrar/request-approval` | On-chain contract approval of a provider |
| GET | `/registrar/request-proof` | DEPRECATED single-proof variant — remove when unused |
| GET | `/registrar/request-proofs` | Delegation proofs (indexer + egress tracker) |
| GET | `/registrar/is-registered` | Check registration status |

`request-proofs` returns two gzip-encoded UCAN containers
(`container.RawGzip`): one for the indexing service, one for the egress
tracker (`egress_tracker` JSON field).

## UCAN model (ucantone, not go-ucanto)

This repo uses the `github.com/fil-forge/ucantone` UCAN implementation and
capability commands from `github.com/fil-forge/libforge/commands/...`.
Capabilities are identified by *commands* with leading-slash paths
(e.g. `/blob/allocate`), not the older `domain/verb` capability strings.

**Validated on registration** — the provider must delegate these commands to
the upload service (see `requiredProofs` in
`internal/services/registrar/delegator.go`):

- `/blob/allocate`, `/blob/accept` (blob commands)
- `/blob/replica/allocate` (replica commands)
- `/pdp/info` (pdp commands)

**Issued to registered nodes** — the delegator extends proofs it holds by one
hop, keeping the original subject so the chain resolves at invocation time:

- `/claim/cache`: indexing service → delegator → storage node
- `/space/egress/track`: egress tracking service → delegator → storage node

Each returned container carries both the root proof and the new hop.
Delegations are minted with `WithNoExpiration()`.

## Registration validation (`Service.Register`)

1. DID must be in the DynamoDB allowlist (`ErrDIDNotAllowed` otherwise).
2. DID must not already be registered.
3. The provider's public URL must serve its DID at the root endpoint —
   accepted as either JSON `{"did": "..."}` or the storage node's plain-text
   banner containing a `did:` line (see `assertEndpointServesDID`; a dedicated
   DID endpoint on storage nodes is a known TODO).
4. The submitted proof container must delegate all `requiredProofs` commands
   to the configured upload service DID.

## Contract approval (`Service.RequestContractApproval`)

Marked in code as a temporary solution pending a billing/operations design
decision. Flow: allowlist check → signature verification (provider signs its
own DID with its private key) → confirm registration with the registry smart
contract → check existing approval → submit approval transaction if needed.
The contract-level approval call is NOT idempotent; the pre-check exists to
avoid duplicate-approval failures. On-chain access goes through
`forgectl/pkg/services/{chain,inspector,operator}` wrapped by
`SmartContractOperator` in `internal/providers/providers.go`.

## Storage

DynamoDB via `internal/store/dynamo.go`, two tables:

- allowlist table (`store.allowlist_table_name`)
- provider info table (`store.providerinfo_table_name`)

The store creates missing tables on initialization — convenient for local
DynamoDB, relevant to IAM permissions in real deployments. Local dev:

```bash
docker run -p 8000:8000 amazon/dynamodb-local -jar DynamoDBLocal.jar -sharedDb
export REGISTRAR_STORE_ENDPOINT=localhost:8000
```

## Configuration

Precedence: flags > env vars > config file (`.delegator.yaml`). Env vars use
the `REGISTRAR_` prefix (set in `cmd/root.go`), e.g. `REGISTRAR_SERVER_PORT`.
Key sections (see `internal/config/config.go`):

- `server`: host, port
- `store`: region, table names, provider weight, optional endpoint
- `delegator`: signing key (inline or file), DID, indexing/egress-tracking
  service DIDs and proofs (inline or file), upload service DID
- `contract`: chain RPC endpoint, payments/service/registry contract
  addresses, transactor (chain ID, key or keystore)

The service identity is an Ed25519 key; the indexing and egress-tracking
proofs are delegations *to* this delegator that it re-delegates to nodes.

## Key dependencies

- `github.com/fil-forge/ucantone` — UCAN (delegations, containers, DIDs)
- `github.com/fil-forge/libforge` — capability command definitions
  (ucantone/libforge replaced the older go-ucanto/go-libstoracha stack —
  older docs referencing those are stale)
- `github.com/fil-forge/forgectl` — smart contract operations
- `github.com/ethereum/go-ethereum` — addresses, contract bindings, signatures
- `github.com/labstack/echo/v4` — HTTP server
- `go.uber.org/fx` — dependency injection (all wiring in `internal/providers`)
- `github.com/spf13/cobra` + `viper` — CLI and config
- `github.com/aws/aws-sdk-go-v2` — DynamoDB

## Deployment

Terraform in `deploy/app` and `deploy/shared`, built on the storoku modules
(`github.com/storacha/storoku//{app,shared}`). `deploy/app/external.tf` grants
the ECS task DynamoDB access. A `Dockerfile` builds the service image.

## Gotchas / blast radius

- **Delegation format changes affect all downstream consumers** — storage
  nodes (piri), the indexing service, and the egress tracker all consume the
  proofs minted here. Changing container encoding, chain shape, or commands
  can break nodes in the field.
- **Proof validation changes can lock out providers** — tightening
  `requiredProofs` or validation logic affects storage node onboarding and
  re-registration.
- The legacy `GET /registrar/request-proof` endpoint is deprecated but still
  routed; do not remove it without confirming no node still calls it.
- Endpoint DID verification parses the storage node's plain-text banner as a
  fallback; changes to the node's root-endpoint output format can break
  registration.
- Naming quirk: the Cobra root command and env prefix are `registrar`/
  `REGISTRAR_`, while the binary/module is `delegator`.
