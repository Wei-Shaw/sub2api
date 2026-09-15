## ADDED Requirements

### Requirement: Preserve client text protocol for dual-endpoint API-key upstreams

The gateway SHALL allow an OpenAI API-key account to declare that its upstream supports both Chat Completions and Responses. When this mode is active, the gateway SHALL route each supported client text endpoint to the matching upstream endpoint without cross-protocol conversion.

#### Scenario: Chat Completions remains Chat Completions

- **GIVEN** an OpenAI API-key account configured with `openai_responses_mode=preserve_inbound`
- **AND** the account supports the `chat_completions` endpoint capability
- **WHEN** a client sends a request to `/v1/chat/completions`
- **THEN** the gateway sends the request to the upstream `/v1/chat/completions` endpoint
- **AND** the gateway does not convert the request to Responses format

#### Scenario: Responses remains Responses

- **GIVEN** an OpenAI API-key account configured with `openai_responses_mode=preserve_inbound`
- **WHEN** a client sends a request to `/v1/responses`
- **THEN** the gateway sends the request to the upstream `/v1/responses` endpoint
- **AND** the gateway does not convert the request to Chat Completions format

#### Scenario: Conflicting endpoint capability is rejected

- **GIVEN** an OpenAI API-key account update selects only the `embeddings` endpoint capability
- **WHEN** the same update selects `openai_responses_mode=preserve_inbound`
- **THEN** the gateway rejects the update before persistence

#### Scenario: Existing modes retain their behavior

- **WHEN** an account uses `auto`, `force_responses`, or `force_chat_completions`
- **THEN** routing behavior remains unchanged
