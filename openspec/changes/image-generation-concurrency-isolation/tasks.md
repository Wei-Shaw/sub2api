## 1. Spec and documentation

- [x] 1.1 Create OpenSpec proposal, design, tasks, and capability spec for image concurrency isolation.
- [x] 1.2 Add a local `2ue` note for the external image gateway deployment pattern and current non-goals.

## 2. Config

- [x] 2.1 Add `gateway.image_concurrency.enabled` and `gateway.image_concurrency.max_concurrent_requests` config fields.
- [x] 2.2 Register defaults that keep existing behavior unchanged.
- [x] 2.3 Validate max concurrent requests as non-negative.
- [x] 2.4 Update `deploy/config.example.yaml` with safe usage notes.
- [x] 2.5 Add image concurrency overflow mode, wait timeout, and max waiting request config.

## 3. Runtime limiter

- [x] 3.1 Implement a process-level image concurrency limiter with resize-on-config-read behavior.
- [x] 3.2 Acquire/release the limiter around `/v1/images/generations` and `/v1/images/edits` before account scheduling.
- [x] 3.3 Acquire/release the limiter around explicit `/v1/responses` image generation intent before account scheduling.
- [x] 3.4 Ensure limiter rejections return `429 rate_limit_error` and do not trigger account failover.
- [x] 3.5 Support `reject` and `wait` overflow modes with bounded wait timeout and waiting queue size.

## 4. Tests and verification

- [x] 4.1 Add config default and validation tests.
- [x] 4.2 Add handler tests for image endpoint limiter rejection.
- [x] 4.3 Add handler tests proving text-only Responses requests are not rejected by the image limiter.
- [x] 4.4 Run focused Go tests for config and OpenAI handler/service paths.
- [x] 4.5 Add limiter tests for wait success, wait timeout, and waiting queue overflow.
