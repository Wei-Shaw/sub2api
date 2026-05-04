## ADDED Requirements

### Requirement: Image multiplier mode
The system SHALL calculate image generation cost with group image prices and a selectable image multiplier mode. By default image billing SHALL share the existing effective group multiplier; when `image_rate_independent=true`, image billing SHALL use `image_rate_multiplier`.

#### Scenario: Default image billing shares current effective group multiplier
- **WHEN** a group has `rate_multiplier=0.15`
- **AND** `image_rate_independent=false`
- **AND** `image_price_1k=0.2`
- **AND** a successful image request produces one `1K` image
- **THEN** `actual_cost` is `0.03`
- **AND** the calculation matches current default behavior

#### Scenario: User-specific token multiplier still applies in shared mode
- **WHEN** a user has a user-group token multiplier override of `0.2`
- **AND** the group has `image_rate_independent=false`
- **AND** `image_price_1k=0.5`
- **AND** a successful image request produces one `1K` image
- **THEN** `actual_cost` is `0.1`
- **AND** the applied image multiplier is the same effective multiplier used by token billing

#### Scenario: Independent image multiplier allows direct final price
- **WHEN** a group has `rate_multiplier=0.15`
- **AND** `image_rate_independent=true`
- **AND** `image_rate_multiplier=1`
- **AND** `image_price_1k=0.2`
- **AND** a successful image request produces one `1K` image
- **THEN** `actual_cost` is `0.2`
- **AND** ordinary `rate_multiplier=0.15` is not applied to the image cost

#### Scenario: Independent image multiplier supports image discounts
- **WHEN** a group has `image_rate_independent=true`
- **AND** `image_rate_multiplier=0.5`
- **AND** `image_price_1k=0.2`
- **AND** a successful image request produces two `1K` images
- **THEN** `total_cost` is `0.4`
- **AND** `actual_cost` is `0.2`

#### Scenario: Migration preserves existing image price behavior
- **WHEN** an existing group has `rate_multiplier=0.15` and `image_price_1k=1.3333333333`
- **AND** the migration is applied
- **THEN** the stored `image_price_1k` remains `1.3333333333`
- **AND** the stored `image_rate_independent` is `false`
- **AND** the stored `image_rate_multiplier` is `1`
- **AND** default-mode image billing still produces the historical final price within decimal precision

#### Scenario: Omitted update fields preserve existing multiplier mode
- **WHEN** an admin updates a group without sending `image_rate_independent`
- **AND** without sending `image_rate_multiplier`
- **THEN** the stored image multiplier mode and image multiplier value remain unchanged

#### Scenario: Image multiplier can be zero only by explicit independent mode configuration
- **WHEN** a group has `image_rate_independent=true`
- **AND** `image_rate_multiplier=0`
- **AND** a successful image request produces one image
- **THEN** the image request is free
- **AND** this free-image behavior does not occur unless the group explicitly enables independent image multiplier mode with zero multiplier

### Requirement: Responses image output accounting
The system SHALL count generated image outputs from OpenAI Responses stream, non-stream, and passthrough paths and SHALL return the count in `OpenAIForwardResult.ImageCount`.

#### Scenario: Non-stream Responses image tool output is counted
- **WHEN** a non-stream `/v1/responses` upstream response contains `output[]` item with `type == "image_generation_call"` and non-empty `result`
- **THEN** `OpenAIForwardResult.ImageCount` equals the number of unique final image outputs
- **AND** `OpenAIForwardResult.ImageSize` is the normalized image size tier

#### Scenario: Stream Responses output item is counted
- **WHEN** a stream `/v1/responses` upstream SSE event has `type == "response.output_item.done"`
- **AND** the event item has `type == "image_generation_call"` and non-empty `result`
- **THEN** the streaming result increments the unique final image output count

#### Scenario: Stream Responses completed output is counted
- **WHEN** a stream `/v1/responses` upstream SSE event has `type == "response.completed"`
- **AND** `response.output[]` contains final image generation outputs
- **THEN** the streaming result counts those images without double-counting images already seen in `response.output_item.done`

#### Scenario: Partial image events are not billed as completed images
- **WHEN** a stream response contains `partial_image` events
- **THEN** those partial events do not increment `ImageCount`
- **AND** only final image generation outputs increment `ImageCount`

#### Scenario: gpt-5.4 image tool request is billed as image
- **WHEN** a `/v1/responses` request uses `model="gpt-5.4"` or `model="gpt-5.5"`
- **AND** the request includes an `image_generation` tool
- **AND** the upstream response contains one final image output
- **THEN** the usage log has `image_count=1`
- **AND** the usage log has `billing_mode="image"`
- **AND** image pricing, not token pricing, determines `actual_cost`

#### Scenario: Image output with zero usage is still billed
- **WHEN** an upstream Responses result contains final image output
- **AND** the upstream result has zero or missing token usage
- **THEN** the system writes a usage log
- **AND** the system bills using image pricing

#### Scenario: Responses image request records accompanying token usage
- **WHEN** a `/v1/responses` image tool request returns final images and token usage
- **THEN** the usage log records input tokens, output tokens, image output tokens, and image count
- **AND** the applied billing mode remains `image`

#### Scenario: Responses image request does not introduce hybrid billing by default
- **WHEN** a `/v1/responses` image tool request returns final images and text tokens
- **THEN** the request is billed by image pricing under this change
- **AND** non-image token charges are not added unless a future explicit hybrid billing mode is implemented

### Requirement: OpenAI Images API output accounting
The system SHALL count generated images from dedicated OpenAI Images API stream and non-stream paths and SHALL set `ImageCount` for successful image responses.

#### Scenario: Images non-stream data array is counted
- **WHEN** `/v1/images/generations` returns a non-stream JSON response with top-level `data[]`
- **THEN** `ImageCount` equals the length of `data[]`

#### Scenario: Images stream data array is counted
- **WHEN** `/v1/images/generations` stream response emits SSE data containing top-level `data[]`
- **THEN** `ImageCount` equals the maximum final data array count observed for the request

#### Scenario: Images stream completed event is counted
- **WHEN** `/v1/images/generations` stream response emits `image_generation.completed` with a final image payload
- **THEN** the stream result counts one final image output

#### Scenario: Images stream Responses-form event is counted
- **WHEN** an Images API upstream path emits Responses-form `response.output_item.done` or `response.completed` events with final image outputs
- **THEN** the stream result counts final image outputs using the same de-duplication rules as Responses

### Requirement: Channel image billing uses actual image count
The system SHALL use actual generated image count for channel `billing_mode=image` pricing and SHALL NOT bill multi-image requests as a single request.

#### Scenario: OpenAI channel image billing counts multiple images
- **WHEN** a channel image pricing entry resolves to unit price `0.25`
- **AND** an OpenAI image request produces three images
- **THEN** `total_cost` is `0.75` before the selected image multiplier is applied
- **AND** `RequestCount` passed into unified pricing is `3`

#### Scenario: Gateway channel image billing counts multiple images
- **WHEN** a non-OpenAI gateway image path produces two images
- **AND** channel image pricing resolves for the billing model
- **THEN** `RequestCount` passed into unified pricing is `2`

#### Scenario: Channel image pricing uses shared multiplier by default
- **WHEN** a channel image pricing entry resolves to unit price `0.25`
- **AND** the group has ordinary effective multiplier `0.15`
- **AND** the group has `image_rate_independent=false`
- **AND** the image request produces one image
- **THEN** `actual_cost` is `0.0375`

#### Scenario: Channel image pricing uses independent image multiplier when enabled
- **WHEN** a channel image pricing entry resolves to unit price `0.25`
- **AND** the group has ordinary effective multiplier `0.15`
- **AND** the group has `image_rate_independent=true`
- **AND** the group has `image_rate_multiplier=1`
- **AND** the image request produces one image
- **THEN** `actual_cost` is `0.25`
- **AND** ordinary effective multiplier `0.15` is not applied

#### Scenario: Account stats image pricing receives image count
- **WHEN** account stats pricing uses `billing_mode=image`
- **AND** the request produces multiple images
- **THEN** account stats cost is calculated with the actual image count

### Requirement: Image size tier normalization
The system SHALL normalize OpenAI image sizes to explicit billing tiers for billing only. The system SHALL NOT reject requests locally because of an unknown or provider-invalid `size`; it SHALL forward the original size parameter upstream and let the official upstream API decide whether the request is valid.

#### Scenario: OpenAI 1024 square maps to 1K
- **WHEN** an OpenAI image request specifies `size="1024x1024"`
- **THEN** `ImageSize` is `1K`

#### Scenario: OpenAI landscape and portrait large sizes map to 2K
- **WHEN** an OpenAI image request specifies `1536x1024`, `1024x1536`, `1792x1024`, `1024x1792`, `2048x2048`, `2048x1152`, or `1152x2048`
- **THEN** `ImageSize` is `2K`

#### Scenario: OpenAI gpt-image-2 4K presets map to 4K
- **WHEN** an OpenAI `gpt-image-2` image request specifies `3840x2160` or `2160x3840`
- **THEN** `ImageSize` is `4K`

#### Scenario: OpenAI auto size maps to 2K
- **WHEN** an OpenAI image request omits size or specifies `size="auto"`
- **THEN** `ImageSize` is `2K`

#### Scenario: Custom OpenAI size is forwarded without local validation
- **WHEN** an OpenAI image request specifies a custom explicit `WIDTHxHEIGHT` size
- **THEN** the system forwards the request upstream
- **AND** `ImageSize` is normalized to `2K` or `4K` for billing

#### Scenario: Responses image tool without model uses default image billing model
- **WHEN** a `/v1/responses` request uses an `image_generation` tool without `tool.model`
- **THEN** image size validation and image billing use `gpt-image-2` as the image billing model

#### Scenario: Invalid OpenAI size constraints are delegated upstream
- **WHEN** an OpenAI image request specifies an explicit size that fails OpenAI size constraints
- **THEN** the system forwards the request upstream
- **AND** any invalid-size error comes from the upstream provider response

#### Scenario: Custom OpenAI size tier mapping
- **WHEN** a custom size cannot be parsed as positive `WIDTHxHEIGHT`
- **THEN** `ImageSize` is `2K`
- **WHEN** a custom size parses as positive `WIDTHxHEIGHT`
- **AND** `WIDTH * HEIGHT` is no more than `2560x1440`
- **THEN** `ImageSize` is `2K`
- **WHEN** a custom size parses as positive `WIDTHxHEIGHT`
- **AND** `WIDTH * HEIGHT` exceeds `2560x1440`
- **THEN** `ImageSize` is `4K`

### Requirement: Image usage log semantics
The system SHALL write usage logs for successful image generation with image billing metadata that matches the applied image pricing path.

#### Scenario: Image usage log records image billing mode
- **WHEN** a successful request has `ImageCount > 0`
- **THEN** the usage log has `billing_mode="image"`
- **AND** the usage log records `image_count`
- **AND** the usage log records `image_size` when a normalized size tier is available

#### Scenario: Shared mode image usage log records shared multiplier
- **WHEN** a successful image request is billed with `image_rate_independent=false`
- **AND** the effective ordinary multiplier is `0.15`
- **THEN** `usage_logs.rate_multiplier` is `0.15`

#### Scenario: Independent mode image usage log records image multiplier
- **WHEN** a successful image request is billed with `image_rate_independent=true`
- **AND** `image_rate_multiplier=0.5`
- **THEN** `usage_logs.rate_multiplier` is `0.5`

#### Scenario: Token request usage log is unchanged
- **WHEN** a successful non-image token request is billed
- **THEN** `usage_logs.rate_multiplier` continues to record the ordinary token multiplier
- **AND** `image_count` is `0`
