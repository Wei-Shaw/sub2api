-- Create tls_fingerprint_profiles table for managing TLS fingerprint templates.
-- Each profile contains ClientHello parameters to simulate specific client TLS handshake characteristics.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

CREATE TABLE IF NOT EXISTS tls_fingerprint_profiles (
    id           BIGSERIAL    PRIMARY KEY,
    name         VARCHAR(100) NOT NULL UNIQUE,
    description  TEXT,
    enable_grease BOOLEAN     NOT NULL DEFAULT false,
    cipher_suites        JSONB,
    curves               JSONB,
    point_formats        JSONB,
    signature_algorithms JSONB,
    alpn_protocols       JSONB,
    supported_versions   JSONB,
    key_share_groups     JSONB,
    psk_modes            JSONB,
    extensions           JSONB,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE tls_fingerprint_profiles IS 'TLS fingerprint templates for simulating specific client TLS handshake characteristics';
COMMENT ON COLUMN tls_fingerprint_profiles.name IS 'Unique profile name, e.g. "macOS Node.js v24"';
COMMENT ON COLUMN tls_fingerprint_profiles.enable_grease IS 'Whether to insert GREASE values in ClientHello extensions';
COMMENT ON COLUMN tls_fingerprint_profiles.cipher_suites IS 'TLS cipher suite list as JSON array of uint16 (order-sensitive, affects JA3)';
COMMENT ON COLUMN tls_fingerprint_profiles.extensions IS 'TLS extension type IDs in send order as JSON array of uint16';
