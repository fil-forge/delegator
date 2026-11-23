-- +goose Up
-- +goose StatementBegin
CREATE TABLE allow_list (
    did      TEXT        PRIMARY KEY,
    added_by TEXT        NOT NULL,
    notes    TEXT        NOT NULL,
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE allow_list IS 'DIDs of storage providers allowed to register';

CREATE TABLE provider_info (
    provider       TEXT        PRIMARY KEY,
    endpoint       TEXT        NOT NULL,
    address        TEXT        NOT NULL,
    proof_set      BIGINT      NOT NULL,
    operator_email TEXT        NOT NULL,
    proofs         TEXT        NOT NULL,
    weight         BIGINT      NOT NULL,
    inserted_at    TIMESTAMPTZ NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL
);

COMMENT ON TABLE provider_info IS 'Registered storage providers';
COMMENT ON COLUMN provider_info.provider IS 'DID of the storage node';
COMMENT ON COLUMN provider_info.endpoint IS 'Public URL the storage node is reachable at';
COMMENT ON COLUMN provider_info.address IS 'Ethereum address the storage node submits proofs from';
COMMENT ON COLUMN provider_info.proof_set IS 'PDP proof set ID of the storage node';
COMMENT ON COLUMN provider_info.proofs IS 'UCAN container delegating the required capabilities to the upload service';
COMMENT ON COLUMN provider_info.weight IS 'Odds of the provider being selected for an upload; 0 excludes it';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS provider_info;
DROP TABLE IF EXISTS allow_list;
-- +goose StatementEnd
