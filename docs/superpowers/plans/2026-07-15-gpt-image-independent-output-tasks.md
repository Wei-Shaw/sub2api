# GPT Multi-Image Independent Tasks Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make each GPT multi-image or multi-angle output an independent plugin task that appears as soon as it succeeds and survives sibling failures or cancellation.

**Architecture:** The frontend expands one user submission into one `GenerateRequest` per desired GPT image, each with `output_count: 1`. A keyed active-task registry replaces the single-job state so submission, polling, terminal rendering, and cancellation operate per task while preserving the existing plugin API and independent backend history records.

**Tech Stack:** Vue 3 composables, TypeScript, Vitest, Go plugin service

---

### Task 1: Specify independent submission behavior

**Files:**
- Test: `plugin-service/plugins/image-generation/frontend/src/composables/useImageGeneration.spec.ts`

- [ ] Add a test where two angle variants produce two `generate` calls with `output_count: 1`, no `variants`, angle-specific prompts, and two assistant result messages.
- [ ] Add a deferred-promise test proving the first completed request updates its own pending message before the second completes.
- [ ] Add a partial-failure test proving one failed request leaves its successful sibling visible and marks only its own assistant message failed.
- [ ] Run `npm run test -- --run src/composables/useImageGeneration.spec.ts` from the frontend directory and verify the new assertions fail because submission is still aggregated.

### Task 2: Implement per-image task submission and completion

**Files:**
- Modify: `plugin-service/plugins/image-generation/frontend/src/composables/useImageGeneration.ts`
- Modify: `plugin-service/plugins/image-generation/frontend/src/types/index.ts` only if task metadata must be represented in a public message type

- [ ] Replace the single job/timer bookkeeping with an internal task registry keyed by a stable pending-message ID and containing job ID, conversation ID, prompt, and timer.
- [ ] Build independent request descriptors: one per selected angle, otherwise one per GPT output count; keep the existing single-request path for one image and non-GPT/batch providers.
- [ ] Append one user message followed by one pending assistant message per descriptor, labeled by angle or image sequence.
- [ ] Submit all descriptors independently with `output_count: 1`; on a synchronous result replace only that descriptor's pending message, and on a pending result start its own poll loop.
- [ ] Derive global `generationStatus` from whether any descriptors are submitting, polling, or cancelling; expose a representative `activeJobId` for compatibility.
- [ ] Handle each rejection locally and set the conversation-level error only when every descriptor fails.
- [ ] Run the focused Vitest file and verify all independent submission, early completion, and partial failure tests pass.

### Task 3: Cancel outstanding tasks without discarding successes

**Files:**
- Test: `plugin-service/plugins/image-generation/frontend/src/composables/useImageGeneration.spec.ts`
- Modify: `plugin-service/plugins/image-generation/frontend/src/composables/useImageGeneration.ts`

- [ ] Add a failing test with one succeeded task and two pending jobs; call `cancelGeneration`, assert both pending job IDs are cancelled, and assert the succeeded image remains.
- [ ] Update cancellation to clear every active task timer, cancel every task with a job ID, mark only unfinished messages canceled, and leave terminal messages unchanged.
- [ ] Update disposal and history-resume logic to clear or reconstruct all applicable task timers without leaking polling callbacks.
- [ ] Run the focused Vitest file and verify cancellation and existing single-job tests pass.

### Task 4: Verify plugin behavior and generated assets

**Files:**
- Modify generated files only if the repository build produces changes under `plugin-service/plugins/image-generation/web/assets/`

- [ ] Run `npm run test -- --run` in `plugin-service/plugins/image-generation/frontend` and confirm zero failures.
- [ ] Run `npm run build` in the frontend directory to refresh production assets and confirm the build succeeds.
- [ ] Run `go test ./plugins/image-generation/...` in `plugin-service` and confirm zero failures.
- [ ] Run `git diff --check` and inspect `git diff --stat` to confirm no main-site `backend` files changed.

