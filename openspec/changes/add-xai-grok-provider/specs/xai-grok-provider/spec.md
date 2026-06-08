## ADDED Requirements

### Requirement: xAI Platform Accounts
The system SHALL support `xai` as a first-class account and group platform for Grok text models.

#### Scenario: Admin creates xAI OAuth account
- **GIVEN** an administrator has completed xAI OAuth login
- **WHEN** the returned credential is saved
- **THEN** the account is stored with platform `xai`
- **AND** the credential contains the access token, refresh token, token endpoint, expiry, and xAI API base URL needed for inference.

#### Scenario: Existing platforms are unaffected
- **GIVEN** an OpenAI, Anthropic, Gemini, or Antigravity account exists
- **WHEN** xAI platform support is enabled
- **THEN** existing account routing, credentials, and model mappings remain unchanged.

### Requirement: xAI OAuth Login And Refresh
The system SHALL provide an xAI OAuth login flow and refresh expired xAI OAuth credentials before forwarding inference requests.

#### Scenario: xAI authorize URL is generated
- **WHEN** an administrator starts xAI OAuth login
- **THEN** the system discovers xAI OAuth endpoints
- **AND** returns an authorize URL containing PKCE challenge, state, nonce, the xAI client ID, and the registered loopback redirect URI.

#### Scenario: xAI callback is submitted manually
- **GIVEN** xAI displays an authorization code instead of redirecting the local callback automatically
- **WHEN** the administrator submits that code with the pending OAuth state
- **THEN** the system exchanges it for tokens
- **AND** stores the credential for future xAI requests.

#### Scenario: xAI token is refreshed
- **GIVEN** an xAI OAuth account has a refresh token
- **WHEN** its access token is expired or near expiry
- **THEN** the system refreshes the credential through xAI's token endpoint
- **AND** uses the refreshed access token for the upstream request.

### Requirement: Grok Text Responses Forwarding
The system SHALL forward OpenAI Responses-compatible text requests to xAI Grok using the xAI Responses endpoint.

#### Scenario: Non-streaming Responses request succeeds
- **GIVEN** an API key belongs to an `xai` group
- **AND** the group can select an active xAI account
- **WHEN** the client sends `POST /v1/responses` with a Grok text model
- **THEN** the system forwards the normalized request to `<xai_base_url>/responses`
- **AND** returns the upstream non-streaming response in OpenAI Responses-compatible form.

#### Scenario: Streaming Responses request succeeds
- **GIVEN** an API key belongs to an `xai` group
- **WHEN** the client sends `POST /v1/responses` with `stream=true`
- **THEN** the system forwards the normalized request to xAI with `Accept: text/event-stream`
- **AND** streams OpenAI Responses-compatible SSE events back to the client.

#### Scenario: Chat Completions request uses Grok
- **GIVEN** an API key belongs to an `xai` group
- **WHEN** the client sends `POST /v1/chat/completions`
- **THEN** the system converts the request to a Responses-compatible request before xAI forwarding
- **AND** returns a Chat Completions-compatible response to the client.

### Requirement: xAI Request Normalization
The system SHALL remove or transform request fields that are unsupported by xAI Grok before calling the xAI upstream.

#### Scenario: Unsupported Responses fields are removed
- **GIVEN** a client request contains `previous_response_id`, `prompt_cache_retention`, `safety_identifier`, or `stream_options`
- **WHEN** the request is prepared for xAI
- **THEN** those fields are omitted from the upstream body.

#### Scenario: Unsupported tools are normalized
- **GIVEN** a client request contains tools unsupported by xAI
- **WHEN** the request is prepared for xAI
- **THEN** unsupported tool entries are removed or converted to xAI-compatible function tools
- **AND** `tool_choice` and `parallel_tool_calls` are removed if no tools remain.

#### Scenario: Reasoning is model-aware
- **GIVEN** a Grok model that does not support reasoning effort
- **WHEN** the request contains a `reasoning` object
- **THEN** the reasoning object is omitted from the upstream body.

### Requirement: Grok Text Model Discovery
The system SHALL expose Grok text models through the model list and model restriction surfaces for `xai` platform groups.

#### Scenario: Admin refreshes xAI cloud models
- **GIVEN** an xAI account has valid OAuth or API key credentials
- **WHEN** an administrator refreshes the account's model list
- **THEN** the system fetches `<xai_base_url>/models` with that account's authorization
- **AND** stores the normalized model IDs as an account-level cloud model snapshot.

#### Scenario: Models list includes Grok text models
- **GIVEN** an API key belongs to an `xai` group
- **WHEN** the client calls `/v1/models`
- **THEN** the response includes manually configured model mappings, refreshed cloud model snapshots, or default Grok text models in that order
- **AND** model ownership identifies xAI/Grok rather than OpenAI.

#### Scenario: Cloud discovery does not block the gateway hot path
- **GIVEN** no administrator has refreshed cloud models for an xAI account
- **WHEN** the client calls `/v1/models`
- **THEN** the system falls back to local Grok defaults
- **AND** the gateway does not call xAI upstream during the model-list request.

### Requirement: Grok Integration Compatibility
The system SHALL verify Grok integration against explicit compatibility gates before the change is considered complete.

#### Scenario: Existing providers do not regress
- **GIVEN** existing OpenAI, Anthropic, Gemini, or Antigravity groups and accounts
- **WHEN** the xAI provider is added
- **THEN** their request routing, account scheduling, model restrictions, usage logging, and billing behavior remain unchanged.

#### Scenario: Responses streaming remains client-compatible
- **GIVEN** xAI returns streaming Responses events
- **WHEN** the final completed event lacks a complete `response.output`
- **THEN** the system reconstructs the completed response output from collected output item events before relaying completion to the client.

#### Scenario: Compatibility limits are explicit
- **GIVEN** a client depends on image generation, video generation, encrypted reasoning content, or unsupported tool types
- **WHEN** the request is routed to the phase-one xAI provider
- **THEN** those unsupported behaviors are rejected, removed, or transformed according to xAI normalization rules
- **AND** the system does not claim semantic parity with OpenAI for those behaviors.
