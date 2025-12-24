## ADDED Requirements

### Requirement: Setup endpoints are protected
The system SHALL restrict setup endpoints to loopback requests unless a setup token is configured and provided.

#### Scenario: Remote request without token
- **WHEN** a non-local request calls a setup mutation endpoint without a valid token
- **THEN** the server returns 403

#### Scenario: Remote request with valid token
- **WHEN** a non-local request includes a valid setup token
- **THEN** the server allows the request

### Requirement: Setup IP trust follows configured proxies
The system SHALL only trust forwarded client IP headers when trusted proxies are explicitly configured.

#### Scenario: Untrusted forwarded headers
- **WHEN** a request includes forwarded IP headers without trusted proxy configuration
- **THEN** the server ignores forwarded headers for setup access checks

### Requirement: Auto-setup requires admin password
The system SHALL fail auto-setup when `ADMIN_PASSWORD` is missing.

#### Scenario: Auto-setup without admin password
- **WHEN** `AUTO_SETUP` is enabled and `ADMIN_PASSWORD` is empty
- **THEN** the server aborts setup with an error

### Requirement: CORS uses allowlist for credentialed requests
The system SHALL only allow credentialed CORS requests for configured origins.

#### Scenario: Allowed origin
- **WHEN** a request comes from an allowed origin with credentials
- **THEN** the server permits the request

#### Scenario: Disallowed origin
- **WHEN** a request comes from an unlisted origin
- **THEN** the server rejects the request

### Requirement: API keys are not returned after creation
The system SHALL return full API keys only at creation time and MUST mask them in list/get responses.

#### Scenario: Create API key
- **WHEN** a user creates an API key
- **THEN** the response includes the full key once

#### Scenario: List API keys
- **WHEN** a user lists API keys
- **THEN** each key is masked and does not reveal the full value

### Requirement: API keys are stored as irreversible hashes
The system SHALL store API keys as irreversible hashes and validate incoming keys by hashing.

#### Scenario: Legacy API key
- **WHEN** a legacy plaintext API key is used
- **THEN** the system accepts it and migrates it to a hashed form

### Requirement: Auth tokens use HttpOnly cookies
The system SHALL issue auth tokens via HttpOnly cookies and clients SHALL authenticate via cookies.

#### Scenario: Login sets cookie
- **WHEN** a user logs in successfully
- **THEN** the response sets an HttpOnly auth cookie

#### Scenario: Authenticated request
- **WHEN** a client sends a request with the auth cookie
- **THEN** the server authenticates the user

### Requirement: Authorization header remains supported
The system SHALL continue to accept `Authorization: Bearer` tokens for API clients.

#### Scenario: API client authentication
- **WHEN** a request includes a valid `Authorization: Bearer` token
- **THEN** the server authenticates the user

### Requirement: Cookie auth enforces Origin/Referer checks
The system SHALL reject state-changing requests authenticated via cookies when Origin/Referer is missing or mismatched.

#### Scenario: Missing Origin on POST
- **WHEN** a cookie-authenticated POST request has no Origin/Referer header
- **THEN** the server rejects the request

### Requirement: Secrets are masked in settings responses
The system SHALL not return SMTP or Turnstile secrets in settings responses.

#### Scenario: Settings response
- **WHEN** an admin requests settings
- **THEN** secret values are omitted or masked

### Requirement: Proxy probe verifies TLS by default
The system SHALL verify TLS certificates for proxy probes unless explicitly configured otherwise.

#### Scenario: Default probe
- **WHEN** a proxy probe is executed without overriding TLS settings
- **THEN** TLS verification is enabled

### Requirement: Upstream HTTP requests have a total timeout
The system SHALL enforce a global timeout for upstream HTTP requests.

#### Scenario: Long-running upstream request
- **WHEN** an upstream request exceeds the configured timeout
- **THEN** the request is canceled

### Requirement: Streaming requests are not prematurely terminated
The system SHALL avoid applying a short global timeout that interrupts streaming responses.

#### Scenario: Streaming response
- **WHEN** a request is handled as a streaming response
- **THEN** the server allows the stream to continue until the client disconnects or context is canceled

### Requirement: Redeem flow is atomic
The system SHALL apply redeem code usage and entitlements in a single transaction.

#### Scenario: Failure during redeem
- **WHEN** entitlement update fails after marking a code as used
- **THEN** all changes are rolled back

### Requirement: Dynamic HTML rendering is sanitized
The system SHALL render dynamic HTML content only after sanitization or by using safe templates.

#### Scenario: Unsafe input in HTML
- **WHEN** dynamic content contains HTML tags
- **THEN** the rendered output is escaped or sanitized
