# Multi-Image Output Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Generate multiple images from one prompt with model-specific output limits.

**Architecture:** Extend the existing image model capability registry with output limits and make the backend resolve and validate `output_count`. Expose the same registry to the Vue frontend so its quantity select, request payload, and history labels share the backend's rules.

**Tech Stack:** Go, Vue 3, TypeScript, Vitest, Playwright

---

### Task 1: Backend capability and provider mapping

**Files:**
- Modify: `plugin-service/plugins/image-generation/backend/model_capabilities.go`
- Modify: `plugin-service/plugins/image-generation/backend/model_capabilities_test.go`
- Modify: `plugin-service/plugins/image-generation/backend/generation_service.go`
- Test: `plugin-service/plugins/image-generation/backend/generation_service_test.go`
- Test: `plugin-service/plugins/image-generation/backend/handlers_test.go`

- [ ] Add failing tests asserting `MaxOutputImages` values 10, 10, 4, and unknown-model default 1.
- [ ] Add failing tests asserting `output_count: 3` maps to OpenAI generation/edit `n: 3` and Gemini batch `output_count: 3`.
- [ ] Add failing tests asserting missing count resolves to 1 and out-of-range counts return `ErrInvalidOutputCount` before provider invocation.
- [ ] Run `go test ./plugins/image-generation/backend` and confirm failures are caused by missing output-count support.
- [ ] Add `OutputCount int` to `GenerateRequest`, `MaxOutputImages int` to `ImageModelCapability`, a resolver/validator, and replace provider constants with the resolved count.
- [ ] Run `go test ./plugins/image-generation/backend` and confirm all backend tests pass.

### Task 2: Frontend quantity state and control

**Files:**
- Modify: `plugin-service/plugins/image-generation/frontend/src/types/index.ts`
- Modify: `plugin-service/plugins/image-generation/frontend/src/composables/useImageGeneration.ts`
- Test: `plugin-service/plugins/image-generation/frontend/src/composables/useImageGeneration.spec.ts`
- Modify: `plugin-service/plugins/image-generation/frontend/src/components/PromptComposer.vue`
- Test: `plugin-service/plugins/image-generation/frontend/src/components/components.spec.ts`
- Modify: `plugin-service/plugins/image-generation/frontend/src/App.vue`

- [ ] Add failing tests asserting model capabilities expose output limits, selected count starts at 1, switching models clamps it, and submit sends `output_count`.
- [ ] Add a failing component test asserting the quantity select offers exactly 1 through `maxOutputImages` and emits numeric updates.
- [ ] Run the focused Vitest files and confirm expected failures.
- [ ] Add typed capability/count state, computed maximum, clamp watcher, request serialization, props/emits, and a quantity select beside size.
- [ ] Run focused Vitest files and confirm they pass.

### Task 3: History, responsive QA, and generated assets

**Files:**
- Modify: `plugin-service/plugins/image-generation/frontend/src/composables/conversationMessages.ts`
- Test: `plugin-service/plugins/image-generation/frontend/src/components/components.spec.ts`
- Test: `plugin-service/plugins/image-generation/frontend/src/composables/useImageGeneration.spec.ts`
- Modify: `plugin-service/plugins/image-generation/frontend/scripts/browser-smoke.mjs`
- Modify: `plugin-service/plugins/image-generation/web/assets/app.js`
- Modify: `plugin-service/plugins/image-generation/web/assets/app.css`

- [ ] Add failing assertions that optimistic and restored messages show the actual requested quantity.
- [ ] Implement history quantity parsing with a default of 1 for older records.
- [ ] Extend browser smoke testing to select multiple outputs and verify the control remains visible at desktop and 390px mobile widths.
- [ ] Run `npm run test`, `npm run typecheck`, `npm run build`, `npm run verify:generated`, and `npm run test:browser`.
- [ ] Run `go test ./plugins/image-generation/...` and `git diff --check`.
- [ ] Commit implementation as `feat(image-generation): support model-aware output counts`.

