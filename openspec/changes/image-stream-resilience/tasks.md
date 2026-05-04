## 1. Config and defaults

- [x] 1.1 Add image-specific stream timeout fields to gateway config.
- [x] 1.2 Register image stream timeout defaults in the config loader.
- [x] 1.3 Add config validation for image stream timeout ranges.
- [x] 1.4 Expose image stream timeout defaults in `deploy/config.example.yaml`.

## 2. Image stream runtime behavior

- [x] 2.1 Detach image stream upstream contexts from client cancellation.
- [x] 2.2 Add image-specific data interval timeout handling to `/v1/images/*` streaming.
- [x] 2.3 Add image-specific data interval timeout handling to `Responses + image_generation` streaming.
- [x] 2.4 Preserve upstream draining after downstream write failures in both image stream paths.

## 3. Tests and verification

- [x] 3.1 Add config tests for image stream timeout defaults and validation.
- [x] 3.2 Add image streaming disconnect tests for the Images API path.
- [x] 3.3 Add image streaming disconnect tests for the Responses image tool path.
- [x] 3.4 Run focused Go tests for the touched config and image service paths.
