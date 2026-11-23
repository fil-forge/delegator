# Delegator HTTP Server

A delegation service that manages storage provider registration and proof generation for the Storacha network. The service validates provider identities, manages their registration status, and issues delegations for interaction with indexing services.

## Architecture

Built with:
- **Cobra** - CLI framework for command management
- **Viper** - Configuration management (files, env vars, flags)
- **Uber.fx** - Dependency injection framework
- **Echo** - High-performance HTTP server
- **DynamoDB** or **PostgreSQL** - Persistent storage for the allow list and provider registry

## Quick Start

```bash
# Build the server
go build -o delegator

# Run the server
./delegator serve

# Or with custom host/port
./delegator serve --host 127.0.0.1 --port 9090
```

## API Endpoints

### Health & Status

#### `GET /` 
Simple root endpoint for basic connectivity check.

**Response:**
- `200 OK` - Returns plain text "hello"

#### `GET /healthcheck`
Health check endpoint for monitoring service availability.

**Response:**
- `200 OK` - Service is healthy
```json
{
  "status": "healthy"
}
```

### Provider Registration

#### `PUT /registrar/register-node`
Register a storage provider node with the delegator service. Validates the provider's identity, proof, and endpoint availability.

**Request Body:**
```json
{
  "did": "did:key:z6MksvRCPWoXvMj8sUzuHiQ4pFkSawkKRz2eh1TALNEG6s3e",
  "owner_address": "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb",
  "proof_set_id": 1,
  "operator_email": "operator@example.com",
  "public_url": "https://storage.example.com",
  "proof": "<base64-encoded-delegation-proof>"
}
```

**Validation:**
- DID must be in the allow list
- DID must not already be registered
- Public URL must serve the provider's DID at root endpoint
- Proof must be valid delegation from provider to upload service with required capabilities:
  - `blob/accept`
  - `blob/allocate`
  - `blob/replica/allocate`
  - `pdp/info`

**Response:**
- `201 Created` - Successfully registered
- `400 Bad Request` - Invalid request data or unauthorized DID
- `500 Internal Server Error` - Server-side processing error

#### `GET /registrar/is-registered`
Check if a provider DID is registered with the service.

**Request Body:**
```json
{
  "did": "did:key:z6MksvRCPWoXvMj8sUzuHiQ4pFkSawkKRz2eh1TALNEG6s3e"
}
```

**Response:**
- `200 OK` - Provider is registered
- `404 Not Found` - Provider is not registered
- `400 Bad Request` - Invalid DID format

#### `GET /registrar/request-proof`
Request a delegation proof for a registered provider to interact with the indexing service.

**Request Body:**
```json
{
  "did": "did:key:z6MksvRCPWoXvMj8sUzuHiQ4pFkSawkKRz2eh1TALNEG6s3e"
}
```

**Response:**
- `200 OK` - Returns delegation proof
```json
{
  "proof": "<base64-encoded-delegation-for-indexing>"
}
```
- `400 Bad Request` - Invalid DID or provider not registered/authorized

The returned proof grants the provider the `claim/cache` capability for the indexing service.

## Client Library

A Go client library is provided in the `client/` package for interacting with the delegator service:

```go
import "github.com/fil-forge/delegator/client"

// Create client
c, err := client.New("http://localhost:8080")

// Register a provider
err = c.Register(ctx, &client.RegisterRequest{
    DID:           "did:key:...",
    OwnerAddress:  "0x...",
    ProofSetID:    1,
    OperatorEmail: "operator@example.com",
    PublicURL:     "https://storage.example.com",
    Proof:         proofString,
})

// Check registration status
registered, err := c.IsRegistered(ctx, &client.IsRegisteredRequest{
    DID: "did:key:...",
})

// Request proof
resp, err := c.RequestProof(ctx, "did:key:...")
```

## Testing

### Run All Tests
```bash
# Run all tests including system tests
go test ./...

# Run with verbose output
go test -v ./...

# Run only system tests
go test -v ./test/...
```

### System Tests
The comprehensive system test suite in `test/system_test.go` covers:
- Full server lifecycle testing
- All API endpoints with success and failure scenarios
- End-to-end registration workflow
- Mock storage node interactions
- Concurrent request handling
- Input validation and error cases

### Test Individual Components
```bash
# Test specific functionality
go test -v ./test/... -run TestSystemRegistrationFlow
go test -v ./test/... -run TestSystemRequestProof
```

### Test Coverage
```bash
# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Configuration

The server can be configured through multiple sources (in order of precedence):
1. Command line flags
2. Environment variables
3. Configuration file (`.delegator.yaml`)

### Configuration File Example
```yaml
server:
  host: 0.0.0.0
  port: 8080

store:
  type: dynamodb # dynamodb (default) or postgres
  providerweight: 100
  # DynamoDB settings, used when type is dynamodb
  region: us-east-1
  allowlist_table_name: delegator-allowlist
  providerinfo_table_name: delegator-providers
  endpoint: "" # Optional: for local DynamoDB
  # Postgres settings, used when type is postgres
  postgres:
    dsn: "" # Prefer REGISTRAR_STORE_POSTGRES_DSN, the DSN holds the password
    max_conns: 10

delegator:
  key_file: /path/to/private/key.pem
  indexing_service_web_did: did:web:indexing.example.com
  indexing_service_proof: "<base64-proof>"
  upload_service_did: did:key:uploadservice...
  insecure_did_resolution: false # Optional: resolve did:web over HTTP (development only)
```

### Environment Variables
All configuration values can be set via environment variables with the prefix `REGISTRAR_`:
```bash
export REGISTRAR_SERVER_PORT=9090
export REGISTRAR_STORE_REGION=us-west-2
```

### Storage backends

The delegator keeps its allow list and provider registry in DynamoDB by
default. To use PostgreSQL instead, set the store type and a connection string:

```bash
export REGISTRAR_STORE_TYPE=postgres
export REGISTRAR_STORE_POSTGRES_DSN='postgres://delegator:YOUR_PASSWORD_HERE@localhost:5432/delegator'
export REGISTRAR_STORE_POSTGRES_MAX_CONNS=10 # Optional, default 10
```

The same settings are available as `--store-type`, `--store-postgres-dsn` and
`--store-postgres-max-conns` flags on `serve` and `store`. A flag value shows
up in the process list, so pass the DSN through the environment.

On startup, the Postgres store creates its tables by running the schema
migrations in `internal/store/migrations`. An advisory lock lets several
instances start against the same database at once. The database user needs
permission to create tables in its schema.

The `store allow-did` and `store disallow-did` commands use whichever backend
the configuration selects.

## Development

### Prerequisites
- Go 1.27 or later
- Access to DynamoDB (AWS or local) or PostgreSQL
- Docker, to run the Postgres store tests (they are skipped without it)
- Ed25519 private key for delegator identity

### Local Development with Docker
```bash
# Run local DynamoDB
docker run -p 8000:8000 amazon/dynamodb-local -jar DynamoDBLocal.jar -sharedDb

# Configure for local development
export REGISTRAR_STORE_ENDPOINT=localhost:8000
```

Or run a local Postgres:

```bash
docker run -p 5432:5432 -e POSTGRES_USER=delegator -e POSTGRES_PASSWORD=delegator postgres:16-alpine

export REGISTRAR_STORE_TYPE=postgres
export REGISTRAR_STORE_POSTGRES_DSN='postgres://delegator:delegator@localhost:5432/delegator?sslmode=disable'
```

## Container images

A push to `main` publishes to GHCR from the `Container` workflow, as
`ghcr.io/fil-forge/delegator:main` plus a `sha-<short-sha>` tag, covering
`linux/amd64` and `linux/arm64`. Release tags publish the same image under their
semver version.

## Deploying to dev

The `main` run also asks [infra-central][] to deploy what it just published. It
dispatches a `bump-deployed-image` event carrying the manifest digest, and
infra-central's [Bump deployed image][receiver] workflow opens a pull request
pinning that digest in `terraform/envs/dev/apps/terraform.tfvars`, with
auto-merge enabled. infra-central's [Check and deploy][deploy] workflow runs
`tofu apply` on `dev/apps` on every push to its `main`, so merging that pull
request is what deploys.

The dispatch runs as the `fil-forge-bot` GitHub App and needs the
`FORGE_BOT_APP_ID` variable and the `FORGE_BOT_PRIVATE_KEY` secret. Prod pins
are promoted by hand.

[infra-central]: https://github.com/fil-forge/infra-central
[receiver]: https://github.com/fil-forge/infra-central/blob/main/.github/workflows/bump-deployed-image.yml
[deploy]: https://github.com/fil-forge/infra-central/blob/main/.github/workflows/check-and-deploy.yml
