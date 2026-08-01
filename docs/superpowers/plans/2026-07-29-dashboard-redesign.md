# Dashboard Redesign + Orange Rebrand Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure the user dashboard (`/dashboard`) to match jiji.cc's skeleton (greeting, 4 metric cards, Quick Access card, quick-entry grid) while keeping Sub2API's deeper analytics, fix the Tailwind `@apply` build blocker, and rebrand the whole app from teal to orange.

**Architecture:** Vue 3 + Vite + Tailwind CSS, componentized under `frontend/src/components/user/dashboard/`. Changes are frontend-only; all data comes from existing APIs. Three sequential phases: (0) unblock `pnpm build`, (1) global primary teal→orange, (2) dashboard restructure.

**Tech Stack:** Vue 3 (`<script setup>`), TypeScript, Tailwind CSS 3.4, vue-i18n, Pinia, Vitest + Vue Test Utils, Axios.

## Global Constraints

- **No backend changes** — all data via existing endpoints (`usageAPI.getDashboardStats`, `keysAPI.list`, `public_settings.api_base_url`, `authStore.user`).
- **Do not rename the `dark` color** in `tailwind.config.js` — templates use `dark-*` classes via JIT (they work); only `@apply` usages are broken and get converted to `slate-*`.
- **Primary palette** must keep shades 50–950 and the key `primary` (used app-wide).
- **i18n:** every new visible string gets both `zh` and `en` keys under `dashboard.*`.
- **Theme is dark-mode aware** — the app supports `darkMode: 'class'`; new/changed components must look right in both light and dark.
- **Verify each phase** with `pnpm build` (must be green) and visual check in `pnpm dev` against the local backend at `http://localhost:8080`.
- **Commits:** one commit per task; the operator has a "commit only when asked" rule — commit at each task's explicit commit step (that is the ask).

**Reference spec:** `docs/superpowers/specs/2026-07-29-dashboard-redesign-design.md`

---

## File Structure

**Create:**
- `frontend/src/components/user/dashboard/UserDashboardQuickAccess.vue` — new Quick Access card (base URL + masked key + copy + tutorial + status).

**Modify:**
- `frontend/tailwind.config.js` — `primary` palette teal→orange.
- `frontend/src/style.css` + ~9 `.vue` files — Phase 0 `@apply` fixes (dark→slate; custom-color @apply → raw CSS).
- `frontend/src/views/user/DashboardView.vue` — new single-column layout order + greeting + QuickAccess.
- `frontend/src/components/user/dashboard/UserDashboardStats.vue` — slim to 4 top cards; extract Row-2 metrics; keep platform breakdown as a separate block.
- `frontend/src/components/user/dashboard/UserDashboardCharts.vue` — add 4-metric auxiliary strip.
- `frontend/src/components/user/dashboard/UserDashboardQuickActions.vue` — vertical list → 4-cell grid.
- `frontend/src/components/user/dashboard/UserDashboardRecentUsage.vue` — add 「查看完整账单 →」 link.
- `frontend/src/i18n/locales/zh/dashboard.ts` + `frontend/src/i18n/locales/en/dashboard.ts` — new keys.

---

## Phase 0 — Fix the `@apply` build blocker

### Task 0.1: Convert colliding `dark-` `@apply` usages to `slate-`

**Files:**
- Modify: `frontend/src/style.css`, `frontend/src/components/layout/TablePageLayout.vue`, `frontend/src/components/common/{DateRangePicker,ProxySelector,AnnouncementBell,Select}.vue`, `frontend/src/views/admin/SettingsView.vue`, `frontend/src/views/user/CustomPageView.vue`, `frontend/src/views/public/LegalDocumentView.vue`, `frontend/src/components/admin/AdminComplianceDialog.vue`

**Why:** `@apply dark:<util>-dark-<shade>` fails (Tailwind can't resolve a color named `dark` after the `dark:` variant). `dark-*` and `slate-*` are hex-identical in this config, so the swap is appearance-preserving.

- [ ] **Step 1: Apply the swap on `@apply` lines only**

Run (zsh array; only touches lines containing `@apply`):
```bash
files=(frontend/src/style.css \
frontend/src/components/layout/TablePageLayout.vue \
frontend/src/components/common/DateRangePicker.vue \
frontend/src/components/common/ProxySelector.vue \
frontend/src/components/common/AnnouncementBell.vue \
frontend/src/components/common/Select.vue \
frontend/src/views/admin/SettingsView.vue \
frontend/src/views/user/CustomPageView.vue \
frontend/src/views/public/LegalDocumentView.vue \
frontend/src/components/admin/AdminComplianceDialog.vue)
for f in $files; do sed -i '' -E '/@apply/s/dark:([a-z]+)-dark-/dark:\1-slate-/g' "$f"; done
```

- [ ] **Step 2: Verify zero remaining `@apply …dark-…` collisions**

Run: `grep -rn "@apply.*dark:[a-z]*-dark-" frontend/src/ | wc -l`
Expected: `0`

- [ ] **Step 3: Commit**

```bash
git add frontend/src
git commit -m "fix(frontend): replace dark-color @apply collisions with slate (hex-identical)"
```

### Task 0.2: Resolve any remaining custom-color `@apply` errors; get `pnpm build` green

**Files:**
- Modify: whichever files the build flags (known example: `frontend/src/style.css` `@apply bg-primary-500/20`).

**Why:** After 0.1, run the build. If `@apply` with `primary-*`/`accent-*` (especially opacity modifiers like `/20`) still errors, convert those specific declarations to raw CSS.

- [ ] **Step 1: Run the production build and capture errors**

Run: `cd frontend && pnpm build 2>&1 | grep -E "class does not exist|plugin:vite:css" | head -40`
Expected: either success, or a list of remaining failing `@apply` classes with file:line.

- [ ] **Step 2: For each remaining failing `@apply`, convert to raw CSS**

Pattern — replace, e.g.:
```css
::selection { @apply bg-primary-500/20 text-primary-900; }
```
with:
```css
::selection { background-color: rgb(255 107 53 / 0.2); color: var(--color-primary-900, #7F2B16); }
```
Use the orange RGB from the new palette (`rgb(255 107 53 …)`) once Task 1.1 lands; until then keep current teal hex so the build is unblocked first. Repeat for every class the build flags.

- [ ] **Step 3: Re-run build until green**

Run: `cd frontend && pnpm build`
Expected: build completes, emits `backend/internal/web/dist`.

- [ ] **Step 4: Smoke-test the dev server**

Run: `cd frontend && pnpm dev` (bypass pnpm deps check if it prompts: `node_modules/.bin/vite frontend --host 0.0.0.0 --port 3000`)
Open `http://localhost:3000/login` → confirm NO Vite error overlay; login page renders.
Expected: page renders, 0 PostCSS errors in console.

- [ ] **Step 5: Commit**

```bash
git add frontend/src
git commit -m "fix(frontend): unblock pnpm build — convert remaining custom-color @apply to raw CSS"
```

---

## Phase 1 — Global primary color: teal → orange

### Task 1.1: Replace the `primary` palette with orange

**Files:**
- Modify: `frontend/tailwind.config.js` (the `primary: { … }` block under `theme.extend.colors`).
- Test: visual (no unit test for palette).

- [ ] **Step 1: Replace the `primary` block**

In `frontend/tailwind.config.js`, replace the existing `primary: { 50..950 }` object with:
```js
primary: {
  50: '#FFF4ED',
  100: '#FFE6D5',
  200: '#FFC9A8',
  300: '#FFA570',
  400: '#FF7E3D',
  500: '#FF6B35',
  600: '#F04E1A',
  700: '#C73C13',
  800: '#9E3116',
  900: '#7F2B16',
  950: '#45140A',
},
```

- [ ] **Step 2: Audit hardcoded teal**

Run: `grep -rnE "#14b8a6|#0d9488|#0f766e|#115e59|#134e4a|#042f2e|\bteal-[0-9]" frontend/src | wc -l`
For each hit, replace with the equivalent `primary-*` class or the matching orange hex above. (If count is 0, skip.)

- [ ] **Step 3: Build + visual check**

Run: `cd frontend && pnpm build`
Then `pnpm dev`, open `http://localhost:8080/login` (local backend already running) — buttons/links/focus rings now orange.
Expected: build green; app renders orange.

- [ ] **Step 4: Commit**

```bash
git add frontend/tailwind.config.js frontend/src
git commit -m "feat(theme): rebrand primary color teal → orange (#FF6B35)"
```

---

## Phase 2 — Dashboard restructure

### Task 2.1: Add i18n keys

**Files:**
- Modify: `frontend/src/i18n/locales/zh/dashboard.ts`, `frontend/src/i18n/locales/en/dashboard.ts`.

**Interfaces:**
- Produces keys used by 2.2–2.6: `dashboard.overview`, `dashboard.greeting` (with `{name}`), `dashboard.quickAccess.title`, `dashboard.quickAccess.subtitle`, `dashboard.quickAccess.baseUrl`, `dashboard.quickAccess.authorization`, `dashboard.quickAccess.copyUrl`, `dashboard.quickAccess.tutorial`, `dashboard.quickAccess.serviceOk`, `dashboard.quickAccess.noKey`, `dashboard.quickAccess.createKey`, `dashboard.viewFullBill`.

- [ ] **Step 1: Add keys to the Chinese locale**

Append (inside the existing `dashboard` object) to `zh/dashboard.ts`:
```ts
overview: '概览',
greeting: '{name}，这里是你的账户与 API 使用概况',
quickAccess: {
  title: '快速接入',
  subtitle: 'OpenAI 兼容 API 地址',
  baseUrl: 'BASE URL',
  authorization: 'AUTHORIZATION',
  copyUrl: '复制地址',
  tutorial: '配置教程',
  serviceOk: '服务正常',
  noKey: '暂无可用密钥',
  createKey: '去创建',
},
viewFullBill: '查看完整账单',
```

- [ ] **Step 2: Add matching keys to the English locale**

Append to `en/dashboard.ts`:
```ts
overview: 'Overview',
greeting: '{name}, here is your account and API usage overview.',
quickAccess: {
  title: 'Quick Access',
  subtitle: 'OpenAI-compatible API endpoint',
  baseUrl: 'BASE URL',
  authorization: 'AUTHORIZATION',
  copyUrl: 'Copy URL',
  tutorial: 'Tutorial',
  serviceOk: 'Service OK',
  noKey: 'No active key',
  createKey: 'Create one',
},
viewFullBill: 'View full bill',
```

- [ ] **Step 3: Verify typecheck**

Run: `cd frontend && pnpm typecheck`
Expected: PASS (no missing-key type errors).

- [ ] **Step 4: Commit**

```bash
git add frontend/src/i18n
git commit -m "feat(i18n): add dashboard redesign keys (zh/en)"
```

### Task 2.2: Create the Quick Access card component

**Files:**
- Create: `frontend/src/components/user/dashboard/UserDashboardQuickAccess.vue`.
- Test: `frontend/src/components/user/dashboard/__tests__/UserDashboardQuickAccess.spec.ts`.

**Interfaces:**
- Consumes: `appStore.cachedPublicSettings.api_base_url` (string), `keysAPI.list({ page:1, page_size:1, status:'active' })` → `ApiKey[]` (fields used: `key_preview` or `prefix`+`last_four`; verify exact field in `frontend/src/types/index.ts` `ApiKey`), `useRouter` for the create-key CTA.
- Produces: default-exported Vue component `<UserDashboardQuickAccess />` (no props).

- [ ] **Step 1: Confirm the `ApiKey` masked-preview field name**

Run: `grep -nE "key_preview|last_four|prefix|masked|preview" frontend/src/types/index.ts | head`
Note the field used for the masked display (use it in Step 3).

- [ ] **Step 2: Write the failing test**

`frontend/src/components/user/dashboard/__tests__/UserDashboardQuickAccess.spec.ts`:
```ts
import { mount, flushPromises } from '@vue/test-utils'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import UserDashboardQuickAccess from '../UserDashboardQuickAccess.vue'

vi.mock('@/api/keys', () => ({
  keysAPI: {
    list: vi.fn().mockResolvedValue({
      items: [{ id: 1, key_preview: 'sk-abcd1234', status: 'active' }],
      total: 1,
    }),
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    cachedPublicSettings: { api_base_url: 'https://demo.test/v1' },
  }),
  useAuthStore: () => ({ user: { username: 'tester' } }),
}))

describe('UserDashboardQuickAccess', () => {
  beforeEach(() => vi.clearAllMocks())
  it('renders the api_base_url', async () => {
    const w = mount(UserDashboardQuickAccess)
    await flushPromises()
    expect(w.text()).toContain('https://demo.test/v1')
  })
  it('renders the masked key preview when an active key exists', async () => {
    const w = mount(UserDashboardQuickAccess)
    await flushPromises()
    expect(w.text()).toContain('sk-abcd1234')
  })
})
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd frontend && pnpm vitest run components/user/dashboard/__tests__/UserDashboardQuickAccess.spec.ts`
Expected: FAIL (component file does not exist).

- [ ] **Step 4: Implement the component**

`frontend/src/components/user/dashboard/UserDashboardQuickAccess.vue`:
```vue
<template>
  <div class="card p-5">
    <div class="mb-3 flex items-center justify-between">
      <div>
        <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('dashboard.quickAccess.title') }}</h2>
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('dashboard.quickAccess.subtitle') }}</p>
      </div>
      <span class="inline-flex items-center gap-1.5 rounded-full bg-green-50 px-2 py-0.5 text-xs font-medium text-green-600 dark:bg-green-900/30 dark:text-green-400">
        <span class="h-1.5 w-1.5 rounded-full bg-green-500" />
        {{ t('dashboard.quickAccess.serviceOk') }}
      </span>
    </div>

    <div class="space-y-2">
      <div class="flex items-center justify-between rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-800/50">
        <div class="min-w-0">
          <p class="text-[10px] uppercase tracking-wide text-gray-400">{{ t('dashboard.quickAccess.baseUrl') }}</p>
          <p class="truncate font-mono text-sm text-gray-900 dark:text-white">{{ baseUrl }}</p>
        </div>
      </div>
      <div class="flex items-center justify-between rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-800/50">
        <div class="min-w-0">
          <p class="text-[10px] uppercase tracking-wide text-gray-400">{{ t('dashboard.quickAccess.authorization') }}</p>
          <p v-if="maskedKey" class="truncate font-mono text-sm text-gray-900 dark:text-white">Bearer {{ maskedKey }}</p>
          <button v-else @click="router.push('/keys')" class="text-sm font-medium text-primary-600 hover:underline dark:text-primary-400">
            {{ t('dashboard.quickAccess.noKey') }} · {{ t('dashboard.quickAccess.createKey') }}
          </button>
        </div>
      </div>
    </div>

    <div class="mt-3 flex gap-2">
      <button @click="copyUrl" class="btn btn-primary flex-1">{{ t('dashboard.quickAccess.copyUrl') }}</button>
      <button @click="router.push('/keys')" class="btn btn-secondary flex-1">{{ t('dashboard.quickAccess.tutorial') }}</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { keysAPI } from '@/api/keys'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()

const firstKey = ref<{ key_preview?: string; prefix?: string; last_four?: string } | null>(null)

const baseUrl = computed(() => appStore.cachedPublicSettings?.api_base_url || `${window.location.origin}/v1`)
const maskedKey = computed(() => {
  const k = firstKey.value
  if (!k) return ''
  return k.key_preview || (k.prefix && k.last_four ? `${k.prefix}...${k.last_four}` : '')
})

async function copyUrl() {
  try { await navigator.clipboard.writeText(baseUrl.value) } catch { /* ignore */ }
}

onMounted(async () => {
  try {
    const res = await keysAPI.list({ page: 1, page_size: 1, status: 'active' })
    firstKey.value = (res.items?.[0] as any) || null
  } catch { firstKey.value = null }
})
</script>
```
Adjust the field names (`key_preview` vs `prefix`/`last_four`) to match what Step 1 found.

- [ ] **Step 5: Run tests to verify pass**

Run: `cd frontend && pnpm vitest run components/user/dashboard/__tests__/UserDashboardQuickAccess.spec.ts`
Expected: PASS (2 tests).

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/user/dashboard
git commit -m "feat(dashboard): add UserDashboardQuickAccess card (base URL + masked key)"
```

### Task 2.3: Slim stats to 4 top cards; move Row-2 metrics into the trend card

**Files:**
- Modify: `frontend/src/components/user/dashboard/UserDashboardStats.vue` (keep Balance / API Keys / Today Requests / Today Cost; delete the Row-2 block: Today Tokens, Total Tokens, Performance, Avg Response).
- Modify: `frontend/src/components/user/dashboard/UserDashboardCharts.vue` (add an auxiliary strip showing the 4 moved metrics).

**Interfaces:**
- `UserDashboardStats` props unchanged (`stats`, `balance`, `isSimple`, `platformQuotas`) — it just renders fewer cards + the platform block.
- `UserDashboardCharts` needs `stats` (in addition to its current trend/model props) to show the 4 metrics — add a `stats` prop.

- [ ] **Step 1: Remove Row-2 from `UserDashboardStats.vue`**

Delete the entire `<!-- Row 2: Token Stats -->` block (the `grid … lg:grid-cols-4` containing Today Tokens, Total Tokens, Performance, Avg Response). Keep Row 1 (4 cards) and the platform-breakdown block.

- [ ] **Step 2: Add a `stats` prop and aux strip to `UserDashboardCharts.vue`**

In `defineProps`, add `stats: { type: Object as PropType<UserDashboardStats>, required: false }` (import the type from `@/api/usage`). At the top of the chart card body, add:
```vue
<div class="mb-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
  <span>{{ t('dashboard.todayTokens') }}: <b class="text-gray-700 dark:text-gray-200">{{ fmt(stats?.today_tokens) }}</b></span>
  <span>{{ t('dashboard.totalTokens') }}: <b class="text-gray-700 dark:text-gray-200">{{ fmt(stats?.total_tokens) }}</b></span>
  <span>RPM: <b class="text-gray-700 dark:text-gray-200">{{ fmt(stats?.rpm) }}</b></span>
  <span>TPM: <b class="text-gray-700 dark:text-gray-200">{{ fmt(stats?.tpm) }}</b></span>
  <span>{{ t('dashboard.avgResponse') }}: <b class="text-gray-700 dark:text-gray-200">{{ dur(stats?.average_duration_ms) }}</b></span>
</div>
```
Add small helpers `fmt` (K/M) and `dur` (ms/s) mirroring `UserDashboardStats.vue` formatters.

- [ ] **Step 3: Pass `stats` from the parent**

In `DashboardView.vue`, update the `<UserDashboardCharts … />` usage to also bind `:stats="stats"`.

- [ ] **Step 4: Build + typecheck**

Run: `cd frontend && pnpm typecheck && pnpm build`
Expected: PASS + green build.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/user/dashboard
git commit -m "refactor(dashboard): slim stats to 4 cards; move token/rpm/latency into chart card"
```

### Task 2.4: QuickActions vertical list → 4-cell grid

**Files:**
- Modify: `frontend/src/components/user/dashboard/UserDashboardQuickActions.vue`.

- [ ] **Step 1: Replace the `space-y-3 p-4` button stack with a 2×2 / 4-col grid**

Change the container `<div class="space-y-3 p-4">` to `<div class="grid grid-cols-2 gap-3 p-4 lg:grid-cols-4">`. Keep the existing four buttons (Create Key, View Usage, Batch Image, Redeem) but swap each button's layout from horizontal (`flex items-center gap-4`) to vertical stacked (icon on top, label below), e.g.:
```vue
<button @click="router.push('/keys')" class="group flex flex-col items-center gap-2 rounded-xl bg-gray-50 p-4 text-center transition-all hover:bg-gray-100 dark:bg-dark-800/50 dark:hover:bg-dark-800">
  <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-primary-100 transition-transform group-hover:scale-105 dark:bg-primary-900/30">
    <Icon name="key" size="md" class="text-primary-600 dark:text-primary-400" />
  </div>
  <span class="text-xs font-medium text-gray-900 dark:text-white">{{ t('dashboard.createApiKey') }}</span>
</button>
```
Apply the same vertical pattern to the other three buttons (keep their existing icon colors/routes).

- [ ] **Step 2: Build + visual check**

Run: `cd frontend && pnpm build`, then `pnpm dev` → open `/dashboard` (log in at `:8080` first).
Expected: quick-entry shows as a 4-cell grid; build green.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/user/dashboard/UserDashboardQuickActions.vue
git commit -m "style(dashboard): quick actions vertical list → 4-cell grid"
```

### Task 2.5: Add 「查看完整账单 →」 link to recent usage

**Files:**
- Modify: `frontend/src/components/user/dashboard/UserDashboardRecentUsage.vue`.

- [ ] **Step 1: Add a router link in the card header**

In the header row (next to the title), add:
```vue
<RouterLink to="/usage" class="text-xs font-medium text-primary-600 hover:underline dark:text-primary-400">
  {{ t('dashboard.viewFullBill') }} →
</RouterLink>
```
Place it in the existing `flex items-center justify-between` header so it sits right-aligned.

- [ ] **Step 2: Build + typecheck**

Run: `cd frontend && pnpm typecheck && pnpm build`
Expected: PASS + green build.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/user/dashboard/UserDashboardRecentUsage.vue
git commit -m "feat(dashboard): add 'view full bill' link to recent usage"
```

### Task 2.6: Assemble the new dashboard layout (greeting + QuickAccess + order)

**Files:**
- Modify: `frontend/src/views/user/DashboardView.vue`.

- [ ] **Step 1: Replace the template body with the new single-column order**

Replace the `<template>…</template>` inner content (inside `<AppLayout>`) with:
```vue
<div class="space-y-6">
  <div v-if="loading" class="flex items-center justify-center py-12"><LoadingSpinner /></div>
  <template v-else-if="stats">
    <!-- 1. Greeting -->
    <div>
      <h1 class="text-xl font-bold text-gray-900 dark:text-white">{{ t('dashboard.overview') }}</h1>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('dashboard.greeting', { name: user?.username || user?.email || '' }) }}</p>
    </div>

    <!-- 2. Four metric cards -->
    <UserDashboardStats :stats="stats" :balance="user?.balance || 0" :is-simple="authStore.isSimpleMode" :platform-quotas="platformQuotas" />

    <!-- 3. Quick Access -->
    <UserDashboardQuickAccess />

    <!-- 4. Trend chart (now also shows the moved metrics) -->
    <UserDashboardCharts
      v-model:startDate="startDate" v-model:endDate="endDate" v-model:granularity="granularity"
      :loading="loadingCharts" :trend="trendData" :models="modelStats" :stats="stats"
      @dateRangeChange="loadCharts" @granularityChange="loadCharts" @refresh="refreshAll" />

    <!-- 5. Quick entry grid -->
    <UserDashboardQuickActions />

    <!-- 6. Recent calls (with view-full-bill link) -->
    <UserDashboardRecentUsage :data="recentUsage" :loading="loadingUsage" />

    <!-- 7. Model usage distribution (inside Charts, or keep separate as before) -->
    <!-- 8. Per-platform breakdown is rendered inside UserDashboardStats -->
  </template>
</div>
```
Note: `UserDashboardCharts` already contains the model-distribution chart in this codebase; if model distribution is a separate component, keep its existing placement. The platform breakdown remains inside `UserDashboardStats` (Task 2.3 kept it).

- [ ] **Step 2: Import `UserDashboardQuickAccess` in the `<script setup>`**

Add to the imports: `import UserDashboardQuickAccess from '@/components/user/dashboard/UserDashboardQuickAccess.vue'`.

- [ ] **Step 3: Build + full visual QA**

Run: `cd frontend && pnpm build`, then `pnpm dev` and open `http://localhost:8080/dashboard` (log in with a local admin account; keep credentials out of tracked files).
Walk the page: greeting shows username → 4 metric cards → Quick Access shows base URL + masked key + copy works → trend chart shows the 4 aux metrics → quick-entry 4-grid → recent calls with 「查看完整账单→」 → model distribution → platform breakdown. Check both light and dark mode.
Expected: all sections present and orange-themed; build green.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/views/user/DashboardView.vue
git commit -m "feat(dashboard): assemble jiji.cc-style layout (greeting + quick access + reordered sections)"
```

---

## Final verification

- [ ] **Full build + tests:** `cd frontend && pnpm build && pnpm test:run`
- [ ] **Visual sign-off** at `http://localhost:8080/dashboard` (light + dark) against the running local backend.
- [ ] Confirm no `@apply` errors, app is orange end-to-end, dashboard matches the spec's 8-point structure.
