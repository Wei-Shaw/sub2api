## 1. Data Model And Migration

- [x] 1.1 Add `allow_image_generation`, `image_rate_independent`, and `image_rate_multiplier` to `backend/ent/schema/group.go`.
- [x] 1.2 Create a new idempotent SQL migration after `133_affiliate_rebate_freeze.sql` for the three group columns.
- [x] 1.3 Backfill existing `openai`, `gemini`, and `antigravity` groups to `allow_image_generation=true` and `anthropic` groups to `false`.
- [x] 1.4 Backfill all existing groups to `image_rate_independent=false` and `image_rate_multiplier=1` without changing existing `image_price_1k/2k/4k`.
- [x] 1.5 Regenerate or update Ent generated group fields, predicates, create/update setters, and query projections.
- [x] 1.6 Add the new fields to backend group domain/service structs, admin create/update inputs, admin responses, and group serialization.

## 2. Admin API And Frontend

- [x] 2.1 Add `allow_image_generation`, `image_rate_independent`, and `image_rate_multiplier` to `CreateGroupRequest` and `UpdateGroupRequest`.
- [x] 2.2 Validate `image_rate_multiplier >= 0` and keep negative image prices using the existing clear-price behavior only for `image_price_*`.
- [x] 2.3 Add the new fields to `frontend/src/types/index.ts` group, create, and update interfaces.
- [x] 2.4 Ensure omitted update fields do not overwrite existing image generation and multiplier mode settings.
- [x] 2.5 Update `frontend/src/views/admin/GroupsView.vue` create/edit forms with a 生图开关, 生图倍率是否独立开关, and conditional image multiplier input.
- [x] 2.6 Add a live final-price preview for `image_price_1k/2k/4k` under shared and independent multiplier modes.
- [x] 2.7 Update group form help text to state that default image billing shares the existing group effective multiplier and independent mode uses the image multiplier input.
- [x] 2.8 Update i18n strings for the new controls and image multiplier mode explanation.

## 3. Image Generation Access Control

- [x] 3.1 Implement a shared helper that detects image generation intent from endpoint, requested model, `tools[]`, and `tool_choice`.
- [x] 3.2 Gate `/v1/images/generations` and `/v1/images/edits` in `backend/internal/handler/openai_images.go` after request parsing and before billing eligibility/account scheduling.
- [x] 3.3 Gate `/v1/responses` explicit `image_generation` tool requests in `backend/internal/service/openai_gateway_service.go` before upstream account scheduling.
- [x] 3.4 Prevent `normalizeOpenAIResponsesImageOnlyModel` from rewriting `gpt-image-*` Responses requests when the group does not allow image generation.
- [x] 3.5 Skip Codex `image_generation` auto-injection and image bridge instructions when the group does not allow image generation.
- [x] 3.6 Re-run image intent detection after service-side request mutation and before upstream dispatch.
- [x] 3.7 Ensure OpenAI advanced scheduler paths apply the same channel `RestrictModels` checks as the load-aware path.

## 4. Responses Image Output Accounting

- [x] 4.1 Add shared parsers for final `image_generation_call.result` outputs in non-stream JSON and SSE payloads.
- [x] 4.2 Extend `openaiStreamingResult` with image count, image size tier, and image billing model fields.
- [x] 4.3 Update `handleStreamingResponse` to count final image outputs while preserving existing stream forwarding and usage parsing.
- [x] 4.4 Update `handleStreamingResponsePassthrough` with the same image output counting.
- [x] 4.5 Update `handleNonStreamingResponse` to count final image outputs from `output[]`.
- [x] 4.6 Update `handleNonStreamingResponsePassthrough` with the same non-stream image output counting.
- [x] 4.7 Populate `OpenAIForwardResult.ImageCount`, `ImageSize`, and image billing model for `gpt-5.4` / `gpt-5.5 + image_generation` requests.

## 5. Images API Accounting And Size Tiers

- [x] 5.1 Extend OpenAI Images API-key stream counting to handle `image_generation.completed`, `response.output_item.done`, and `response.completed`.
- [x] 5.2 Reuse the same final-image de-duplication rules across Images API and Responses API paths.
- [x] 5.3 Keep unknown explicit OpenAI image sizes pass-through and delegate invalid-size errors to upstream.
- [x] 5.4 Map documented OpenAI image sizes to `1K`/`2K`/`4K` billing tiers without rewriting request parameters.
- [x] 5.5 Classify custom OpenAI `WIDTHxHEIGHT` sizes by `2560x1440` total-pixel boundary, falling back to `2K` when unparseable.

## 6. Billing And Usage Logs

- [x] 6.1 Add an image multiplier resolver: shared mode uses the current effective group multiplier, independent mode uses `apiKey.Group.ImageRateMultiplier`.
- [x] 6.2 Update `CalculateImageCost` or its caller contract so image costs use the resolved image multiplier.
- [x] 6.3 Set image usage log `RateMultiplier` to the applied image multiplier; keep token logs unchanged.
- [x] 6.4 Change OpenAI channel image billing `RequestCount` from `1` to `result.ImageCount`.
- [x] 6.5 Change non-OpenAI gateway channel image billing `RequestCount` from `1` to `result.ImageCount`.
- [x] 6.6 Pass actual image count into account stats pricing for `billing_mode=image`.
- [x] 6.7 Ensure `ImageCount > 0` writes a usage log and bills even when upstream token usage is zero.
- [x] 6.8 Record accompanying token usage for Responses image tool requests while keeping default billing mode as `image`.

## 7. Tests And Documentation

- [x] 7.1 Add backend tests for disabled group rejecting `/v1/images/*`, `gpt-image-*` Responses, explicit `image_generation`, and image `tool_choice`.
- [x] 7.2 Add backend tests proving disabled Codex groups do not receive injected image tools while enabled Codex groups still do.
- [x] 7.3 Add backend tests proving omitted group update fields preserve existing image generation and multiplier mode settings.
- [x] 7.4 Add Responses stream and non-stream tests for `gpt-5.4` / `gpt-5.5 + image_generation` image counting and image billing.
- [x] 7.5 Add Images API stream tests for `image_generation.completed`, `response.output_item.done`, and `response.completed` counting.
- [x] 7.6 Add billing tests for shared mode `rate_multiplier=0.15`, `image_price_1k=0.2`, final `actual_cost=0.03`.
- [x] 7.7 Add billing tests for independent mode `rate_multiplier=0.15`, `image_rate_multiplier=1`, `image_price_1k=0.2`, final `actual_cost=0.2`.
- [x] 7.8 Add channel image billing tests proving multi-image requests use `RequestCount=ImageCount` in both shared and independent multiplier modes.
- [x] 7.9 Add size-tier tests for known OpenAI sizes and unknown explicit size pass-through.
- [x] 7.10 Add Responses image tool tests proving token usage is recorded but default billing remains image-mode only.
- [x] 7.11 Update `2ue/image-billing-risk-analysis.md` or add a linked follow-up note that points to this OpenSpec change as the normalized solution.
