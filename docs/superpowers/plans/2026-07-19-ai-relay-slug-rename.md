# AI Relay Slug Rename Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let administrators edit a route Slug by deleting the old route and creating the new route from the standalone plugin page.

**Architecture:** Keep the backend API unchanged. The Vue save flow compares the normalized form Slug with the original route Slug; unchanged values use `updateRoute`, while changed values call `deleteRoutes` with the old key and then `createRoute` with the new payload.

**Tech Stack:** Vue 3, TypeScript, Vitest, Vue Test Utils.

---

### Task 1: Enable and migrate edited Slugs

**Files:**
- Modify: `plugin-service/plugins/ai-relay/frontend/src/App.vue`
- Modify: `plugin-service/plugins/ai-relay/frontend/src/App.spec.ts`

- [ ] **Step 1: Write failing tests**

Add a test that opens an existing route, verifies the Slug input is enabled, changes it from `old-slug` to `new-slug`, submits, and asserts these ordered calls:

```ts
expect(deleteRoutes).toHaveBeenCalledWith([{ platform: 'openai', slug: 'old-slug' }])
expect(createRoute).toHaveBeenCalledWith(expect.objectContaining({ platform: 'openai', slug: 'new-slug' }))
expect(updateRoute).not.toHaveBeenCalled()
expect(deleteRoutes.mock.invocationCallOrder[0]).toBeLessThan(createRoute.mock.invocationCallOrder[0])
```

Retain or add a test proving an unchanged Slug still calls `updateRoute` and does not delete/create.

- [ ] **Step 2: Run the focused test and verify it fails**

Run: `npm test -- --run src/App.spec.ts`
Expected: FAIL because the Slug input is disabled and save always calls `updateRoute` for edits.

- [ ] **Step 3: Implement the minimum Vue change**

Remove `:disabled="!!editing"` from the Slug input. In `save`, normalize the submitted Slug, compare it with `editing.value.slug`, and execute delete-then-create only when it differs. Leave Platform disabled during edit and preserve existing form validation and error handling.

- [ ] **Step 4: Run frontend verification**

Run: `npm test -- --run`; `npm run typecheck`; `npm run build`
Expected: all tests pass, typecheck succeeds, and the production plugin assets build.

- [ ] **Step 5: Inspect and commit**

Run: `git diff --check`; `git status --short`; `git add plugin-service/plugins/ai-relay/frontend && git commit -m "feat(ai-relay): allow editing route slugs"`
Expected: only standalone AI Relay plugin frontend/test files are committed.
