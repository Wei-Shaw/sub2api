## ADDED Requirements

### Requirement: Group image generation capability
The system SHALL store a group-level `allow_image_generation` capability flag and SHALL expose it through admin group create, update, list, and detail APIs.

#### Scenario: New group defaults to image generation disabled
- **WHEN** an admin creates a group without providing `allow_image_generation`
- **THEN** the persisted group has `allow_image_generation=false`

#### Scenario: Existing image-capable platform groups are backfilled
- **WHEN** the migration is applied to existing groups
- **THEN** existing `openai`, `gemini`, and `antigravity` groups have `allow_image_generation=true`
- **AND** existing `anthropic` groups have `allow_image_generation=false`

#### Scenario: Admin enables image generation on an ordinary coding group
- **WHEN** an admin updates an `openai` group with `allow_image_generation=true`
- **THEN** the group can use image generation paths subject to the billing requirements

#### Scenario: Admin disables image generation on an ordinary coding group
- **WHEN** an admin updates an `openai` group with `allow_image_generation=false`
- **THEN** the group can still use non-image text model requests
- **AND** image generation intents are denied before upstream dispatch

### Requirement: Image generation intent detection
The system SHALL classify a request as an image generation intent before upstream account scheduling when the endpoint or request body can produce generated images.

#### Scenario: Images endpoint is an image generation intent
- **WHEN** a request targets `/v1/images/generations`, `/v1/images/edits`, `/images/generations`, or `/images/edits`
- **THEN** the request is classified as an image generation intent

#### Scenario: Responses request with image-only model is an image generation intent
- **WHEN** a `/v1/responses` request has a requested model whose normalized name starts with `gpt-image-`
- **THEN** the request is classified as an image generation intent before any model rewrite

#### Scenario: Responses request with image_generation tool is an image generation intent
- **WHEN** a `/v1/responses` request contains any `tools[]` entry with `type == "image_generation"`
- **THEN** the request is classified as an image generation intent

#### Scenario: Responses request with image_generation tool_choice is an image generation intent
- **WHEN** a `/v1/responses` request contains `tool_choice` that explicitly selects `image_generation`
- **THEN** the request is classified as an image generation intent even if `tools[]` is malformed or absent

#### Scenario: Generic tool_choice required is not sufficient by itself
- **WHEN** a `/v1/responses` request contains `tool_choice="required"`
- **AND** the request does not contain an `image_generation` tool
- **THEN** the request is not classified as an image generation intent because of `tool_choice` alone

#### Scenario: Text-only gpt-5.4 request is not an image generation intent
- **WHEN** a `/v1/responses` request uses `model="gpt-5.4"` or `model="gpt-5.5"` without `image_generation` tool and without image `tool_choice`
- **THEN** the request is not classified as an image generation intent

#### Scenario: Intent is checked before and after service-side mutation
- **WHEN** the service mutates a `/v1/responses` request by injecting `image_generation` or rewriting `gpt-image-*` to a Responses text model plus image tool
- **THEN** the final mutated request is checked against the same image generation intent rules before upstream dispatch

### Requirement: Disabled groups reject explicit image generation
The system SHALL reject explicit image generation intents for groups with `allow_image_generation=false` before selecting or calling an upstream account.

#### Scenario: Disabled group rejects Images API
- **WHEN** a group has `allow_image_generation=false`
- **AND** a user calls `/v1/images/generations`
- **THEN** the system returns HTTP 403 with error type `permission_error`
- **AND** no upstream account is selected
- **AND** no usage log is written

#### Scenario: Disabled group rejects Responses image tool
- **WHEN** a group has `allow_image_generation=false`
- **AND** a user calls `/v1/responses` with `tools:[{"type":"image_generation"}]`
- **THEN** the system returns HTTP 403 with error type `permission_error`
- **AND** no upstream account is selected
- **AND** no usage log is written

#### Scenario: Disabled group rejects Responses image-only model rewrite
- **WHEN** a group has `allow_image_generation=false`
- **AND** a user calls `/v1/responses` with `model` starting with `gpt-image-`
- **THEN** the system returns HTTP 403 with error type `permission_error`
- **AND** the request is not rewritten to a text Responses model

#### Scenario: Disabled group permits normal coding request
- **WHEN** a group has `allow_image_generation=false`
- **AND** a user calls `/v1/responses` with `model="gpt-5.4"` and no image generation intent
- **THEN** the request proceeds through the normal text forwarding path

### Requirement: Codex image tool injection respects group capability
The system SHALL only inject the OpenAI Responses `image_generation` tool and bridge instructions for Codex clients when the request group has `allow_image_generation=true`.

#### Scenario: Codex request in enabled group receives image tool
- **WHEN** a Codex CLI `/v1/responses` request belongs to a group with `allow_image_generation=true`
- **AND** the request has no `image_generation` tool
- **THEN** the system injects the existing `image_generation` tool payload
- **AND** the system appends the existing Codex image bridge instructions

#### Scenario: Codex request in disabled group does not receive image tool
- **WHEN** a Codex CLI `/v1/responses` request belongs to a group with `allow_image_generation=false`
- **AND** the request has no explicit image generation intent
- **THEN** the system does not inject `image_generation`
- **AND** the system does not append image bridge instructions
- **AND** the request proceeds as a text request

#### Scenario: Codex explicit image request in disabled group is denied
- **WHEN** a Codex CLI `/v1/responses` request belongs to a group with `allow_image_generation=false`
- **AND** the request explicitly contains `image_generation`
- **THEN** the system returns HTTP 403 with error type `permission_error`

### Requirement: Channel model restrictions remain enforced
The system SHALL keep existing channel model restriction behavior for image and non-image OpenAI requests, including when the advanced OpenAI account scheduler is enabled.

#### Scenario: Advanced scheduler blocks restricted requested model
- **WHEN** a channel has `restrict_models=true`
- **AND** the requested model is not allowed by channel pricing or mapping rules
- **AND** the OpenAI advanced scheduler path is used
- **THEN** the request is rejected before upstream account selection succeeds

#### Scenario: Image generation flag does not bypass channel restrictions
- **WHEN** a group has `allow_image_generation=true`
- **AND** the channel restriction rejects the requested or billing model
- **THEN** the image generation request is rejected
- **AND** no upstream image request is sent
