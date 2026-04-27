# Account Bulk Edit Scope And Compact Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add filter-result bulk edit to admin accounts, unify the table-level bulk-edit entry, and align OpenAI bulk-edit controls with the existing compact-related single-account settings.

**Architecture:** Extend the existing `/admin/accounts/bulk-update` flow to accept either explicit account IDs or a server-resolved filter target. Reuse the current account-list filter contract for scope resolution, then update the accounts view and bulk-edit modal so the UI can launch either selected-account edits or current-filter-result edits from one compact dropdown. Keep the existing bulk-edit form, but expand its target contract and OpenAI-specific field coverage.

**Tech Stack:** Vue 3, TypeScript, Vitest, Gin, Go service/repository layer, existing admin accounts API.

---

### Task 1: Add backend test coverage for filter-target bulk update

**Files:**
- Modify: `backend/internal/handler/admin/account_handler_mixed_channel_test.go`
- Modify: `backend/internal/service/admin_service_bulk_update_test.go`
- Test: `backend/internal/handler/admin/account_handler_mixed_channel_test.go`
- Test: `backend/internal/service/admin_service_bulk_update_test.go`

- [ ] **Step 1: Write the failing handler test for filter-target request acceptance**

```go
func TestBulkUpdateAcceptsFilterTargetRequest(t *testing.T) {
	// add a request body that omits account_ids and submits filters instead
	// assert the route does not reject the request as malformed once service stubs are wired
REDACTED
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./backend/internal/handler/admin -run TestBulkUpdateAcceptsFilterTargetRequest -count=1`
Expected: FAIL because `BulkUpdateAccountsRequest` does not yet support `filters`.

- [ ] **Step 3: Write the failing service test for resolving IDs from filters**

```go
func TestAdminServiceBulkUpdateAccounts_ResolvesIDsFromFilters(t *testing.T) {
	// construct BulkUpdateAccountsInput with Filters and no AccountIDs
	// stub repository list/search path to return matching IDs
	// assert BulkUpdate is called with all matching account IDs
REDACTED
```

- [ ] **Step 4: Run test to verify it fails**

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./backend/internal/service -run TestAdminServiceBulkUpdateAccounts_ResolvesIDsFromFilters -count=1`
Expected: FAIL because `BulkUpdateAccountsInput` and service logic only use explicit `AccountIDs`.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handler/admin/account_handler_mixed_channel_test.go backend/internal/service/admin_service_bulk_update_test.go
git commit -m "test: cover filter-target account bulk update"
```

### Task 2: Implement backend filter-target bulk update

**Files:**
- Modify: `backend/internal/handler/admin/account_handler.go`
- Modify: `backend/internal/service/admin_service.go`
- Modify: `backend/internal/repository/account_repo.go`
- Modify: `backend/internal/service/account_service.go`
- Test: `backend/internal/handler/admin/account_handler_mixed_channel_test.go`
- Test: `backend/internal/service/admin_service_bulk_update_test.go`

- [ ] **Step 1: Implement request structs and validation for filter targets**

```go
type BulkUpdateAccountFilters struct {
	Platform    string `json:"platform"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	Group       string `json:"group"`
	Search      string `json:"search"`
	PrivacyMode string `json:"privacy_mode"`
REDACTED

type BulkUpdateAccountsRequest struct {
	AccountIDs []int64                  `json:"account_ids"`
	Filters    *BulkUpdateAccountFilters `json:"filters"`
	// existing fields remain unchanged
REDACTED
```

- [ ] **Step 2: Resolve filter targets in the service layer with one canonical path**

```go
type BulkUpdateAccountsInput struct {
	AccountIDs []int64
	Filters    *BulkUpdateAccountFilters
	// existing fields remain unchanged
REDACTED

if len(input.AccountIDs) == 0 && input.Filters != nil {
	ids, err := s.resolveBulkUpdateTargetIDs(ctx, input.Filters)
	if err != nil {
		return nil, err
REDACTED
	input.AccountIDs = ids
REDACTED
```

- [ ] **Step 3: Reuse existing account-search/repository logic to resolve all matching IDs**

```go
func (s *AdminService) resolveBulkUpdateTargetIDs(ctx context.Context, filters *BulkUpdateAccountFilters) ([]int64, error) {
	// call the existing repository list/search path with the submitted filters
	// page through all matching rows or use a dedicated ID-only query helper
	// return unique IDs in stable order
REDACTED
```

- [ ] **Step 4: Run targeted backend tests**

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./backend/internal/handler/admin ./backend/internal/service -run 'TestBulkUpdateAcceptsFilterTargetRequest|TestAdminServiceBulkUpdateAccounts_ResolvesIDsFromFilters' -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handler/admin/account_handler.go backend/internal/service/admin_service.go backend/internal/repository/account_repo.go backend/internal/service/account_service.go backend/internal/handler/admin/account_handler_mixed_channel_test.go backend/internal/service/admin_service_bulk_update_test.go
git commit -m "feat: support filter-target account bulk update"
```

### Task 3: Add frontend API and modal tests for target scope

**Files:**
- Modify: `frontend/src/components/account/__tests__/BulkEditAccountModal.spec.ts`
- Create: `frontend/src/views/admin/__tests__/AccountsView.bulkEdit.spec.ts`
- Modify: `frontend/src/api/admin/accounts.ts`
- Test: `frontend/src/components/account/__tests__/BulkEditAccountModal.spec.ts`
- Test: `frontend/src/views/admin/__tests__/AccountsView.bulkEdit.spec.ts`

- [ ] **Step 1: Write the failing modal test for filter-target payload submission**

```ts
it('submits bulk edit using current filters when target mode is filtered-results', async () => {
  // mount BulkEditAccountModal with targetMode='filtered'
  // submit a minimal change
  // expect adminAPI.accounts.bulkUpdate to receive { filters: ... REDACTED rather than account_ids
REDACTED)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm -C frontend test:run src/components/account/__tests__/BulkEditAccountModal.spec.ts -t "filtered-results"`
Expected: FAIL because the modal only accepts `accountIds`.

- [ ] **Step 3: Write the failing accounts-view test for dropdown launch actions**

```ts
it('opens bulk edit for current filtered results from the table action dropdown', async () => {
  // mount AccountsView with filters set
  // click Bulk edit > current filtered results
  // assert modal props contain filter target metadata
REDACTED)
```

- [ ] **Step 4: Run test to verify it fails**

Run: `pnpm -C frontend test:run src/views/admin/__tests__/AccountsView.bulkEdit.spec.ts`
Expected: FAIL because the dropdown action and target scope state do not exist yet.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/account/__tests__/BulkEditAccountModal.spec.ts frontend/src/views/admin/__tests__/AccountsView.bulkEdit.spec.ts frontend/src/api/admin/accounts.ts
git commit -m "test: cover account bulk edit target scopes"
```

### Task 4: Implement unified frontend bulk-edit target scope flow

**Files:**
- Modify: `frontend/src/views/admin/AccountsView.vue`
- Modify: `frontend/src/components/admin/account/AccountBulkActionsBar.vue`
- Modify: `frontend/src/components/account/BulkEditAccountModal.vue`
- Modify: `frontend/src/api/admin/accounts.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`
- Test: `frontend/src/components/account/__tests__/BulkEditAccountModal.spec.ts`
- Test: `frontend/src/views/admin/__tests__/AccountsView.bulkEdit.spec.ts`

- [ ] **Step 1: Add a typed frontend target contract for bulk edit**

```ts
export type AccountBulkEditTarget =
  | { mode: 'selected'; accountIds: number[]; selectedPlatforms: AccountPlatform[]; selectedTypes: AccountType[] REDACTED
  | { mode: 'filtered'; filters: AccountListFilters; previewCount: number; selectedPlatforms: AccountPlatform[]; selectedTypes: AccountType[] REDACTED
```

- [ ] **Step 2: Replace the single selected-row edit button with one dropdown**

```vue
<BulkEditDropdown
  :has-selection="selectedIds.length > 0"
  @edit-selected="openBulkEditSelected"
  @edit-filtered="openBulkEditFiltered"
/>
```

- [ ] **Step 3: Snapshot current filters and preview count when launching filtered mode**

```ts
const openBulkEditFiltered = async () => {
  const filters = toBulkEditFilterSnapshot(params)
  const preview = await adminAPI.accounts.list(1, 1, filters)
  bulkEditTarget.value = {
    mode: 'filtered',
    filters,
    previewCount: preview.pagination.total,
    selectedPlatforms: collectPlatforms(preview.data),
    selectedTypes: collectTypes(preview.data)
  REDACTED
  showBulkEdit.value = true
REDACTED
```

- [ ] **Step 4: Update modal submission to call `bulkUpdate` with either `account_ids` or `filters`**

```ts
if (props.target.mode === 'selected') {
  await adminAPI.accounts.bulkUpdate({ account_ids: props.target.accountIds, ...updates REDACTED)
REDACTED else {
  await adminAPI.accounts.bulkUpdate({ filters: props.target.filters, ...updates REDACTED)
REDACTED
```

- [ ] **Step 5: Run targeted frontend tests**

Run: `pnpm -C frontend test:run src/components/account/__tests__/BulkEditAccountModal.spec.ts src/views/admin/__tests__/AccountsView.bulkEdit.spec.ts`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add frontend/src/views/admin/AccountsView.vue frontend/src/components/admin/account/AccountBulkActionsBar.vue frontend/src/components/account/BulkEditAccountModal.vue frontend/src/api/admin/accounts.ts frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts frontend/src/components/account/__tests__/BulkEditAccountModal.spec.ts frontend/src/views/admin/__tests__/AccountsView.bulkEdit.spec.ts
git commit -m "feat: add filtered-result account bulk edit"
```

### Task 5: Add failing tests for missing OpenAI bulk-edit fields

**Files:**
- Modify: `frontend/src/components/account/__tests__/BulkEditAccountModal.spec.ts`
- Test: `frontend/src/components/account/__tests__/BulkEditAccountModal.spec.ts`

- [ ] **Step 1: Write the failing OAuth test for `codex_cli_only`**

```ts
it('OpenAI OAuth bulk edit can submit codex_cli_only', async () => {
  // enable the toggle and submit
  // expect extra.codex_cli_only to be sent
REDACTED)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm -C frontend test:run src/components/account/__tests__/BulkEditAccountModal.spec.ts -t "codex_cli_only"`
Expected: FAIL because the modal has no such control or payload mapping.

- [ ] **Step 3: Write the failing API key test for API key WS mode**

```ts
it('OpenAI API key bulk edit submits API key WS mode fields', async () => {
  // enable the API key WS mode selector and submit
  // expect openai_apikey_responses_websockets_v2_mode and enabled flag
REDACTED)
```

- [ ] **Step 4: Run test to verify it fails**

Run: `pnpm -C frontend test:run src/components/account/__tests__/BulkEditAccountModal.spec.ts -t "API key WS mode"`
Expected: FAIL because the modal only submits OAuth WS mode.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/account/__tests__/BulkEditAccountModal.spec.ts
git commit -m "test: cover missing OpenAI bulk edit fields"
```

### Task 6: Implement missing OpenAI bulk-edit controls and payload wiring

**Files:**
- Modify: `frontend/src/components/account/BulkEditAccountModal.vue`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`
- Test: `frontend/src/components/account/__tests__/BulkEditAccountModal.spec.ts`

- [ ] **Step 1: Add UI controls for OAuth `codex_cli_only` and API key WS mode**

```vue
<div v-if="allOpenAIOAuth">
  <!-- existing OAuth WS mode -->
  <!-- add codex_cli_only toggle -->
</div>

<div v-if="allOpenAIAPIKey">
  <!-- add API key WS mode selector -->
</div>
```

- [ ] **Step 2: Mirror single-account payload semantics in the bulk-edit submit builder**

```ts
if (enableCodexCLIOnly.value) {
  const extra = ensureExtra()
  extra.codex_cli_only = codexCLIOnlyEnabled.value
REDACTED

if (enableOpenAIAPIKeyWSMode.value) {
  const extra = ensureExtra()
  extra.openai_apikey_responses_websockets_v2_mode = openaiAPIKeyResponsesWebSocketV2Mode.value
  extra.openai_apikey_responses_websockets_v2_enabled = isOpenAIWSModeEnabled(openaiAPIKeyResponsesWebSocketV2Mode.value)
REDACTED
```

- [ ] **Step 3: Run focused modal tests**

Run: `pnpm -C frontend test:run src/components/account/__tests__/BulkEditAccountModal.spec.ts`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/account/BulkEditAccountModal.vue frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts frontend/src/components/account/__tests__/BulkEditAccountModal.spec.ts
git commit -m "feat: align OpenAI bulk edit compact settings"
```

### Task 7: Final regression verification

**Files:**
- Modify: none expected
- Test: `frontend/src/components/account/__tests__/BulkEditAccountModal.spec.ts`
- Test: `frontend/src/views/admin/__tests__/AccountsView.bulkEdit.spec.ts`
- Test: `backend/internal/handler/admin/account_handler_mixed_channel_test.go`
- Test: `backend/internal/service/admin_service_bulk_update_test.go`

- [ ] **Step 1: Run frontend typecheck**

Run: `pnpm -C frontend typecheck`
Expected: PASS

- [ ] **Step 2: Run focused frontend test suite**

Run: `pnpm -C frontend test:run src/components/account/__tests__/BulkEditAccountModal.spec.ts src/views/admin/__tests__/AccountsView.bulkEdit.spec.ts`
Expected: PASS

- [ ] **Step 3: Run focused backend test suite**

Run: `GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod go test ./backend/internal/handler/admin ./backend/internal/service -run 'BulkUpdate|bulk update' -count=1`
Expected: PASS

- [ ] **Step 4: Commit final integration fixes if needed**

```bash
git add frontend/src/components/account/BulkEditAccountModal.vue frontend/src/views/admin/AccountsView.vue frontend/src/components/admin/account/AccountBulkActionsBar.vue frontend/src/api/admin/accounts.ts frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts backend/internal/handler/admin/account_handler.go backend/internal/service/admin_service.go backend/internal/repository/account_repo.go backend/internal/service/account_service.go frontend/src/components/account/__tests__/BulkEditAccountModal.spec.ts frontend/src/views/admin/__tests__/AccountsView.bulkEdit.spec.ts backend/internal/handler/admin/account_handler_mixed_channel_test.go backend/internal/service/admin_service_bulk_update_test.go
git commit -m "feat: finish account bulk edit scope and compact support"
```
