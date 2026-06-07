<template>
  <PlazaLayout>
    <div class="flex flex-col gap-6 lg:flex-row">
      <!-- Left-hand persistent group sidebar -->
      <aside
        class="w-full shrink-0 overflow-hidden rounded-2xl border border-gray-200/70 bg-white/80 p-3 shadow-sm backdrop-blur-sm dark:border-dark-700/70 dark:bg-dark-900/60 lg:w-60 lg:self-start"
      >
        <div class="mb-2 px-2 pt-1 text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">
          {{ t('plaza.models.groupSidebarTitle') }}
        </div>
        <ul class="flex max-h-[60vh] flex-col gap-1 overflow-y-auto pr-1">
          <li>
            <button
              type="button"
              :class="groupBtnClass(undefined)"
              @click="selectGroup(undefined)"
            >
              <span class="truncate">{{ t('plaza.models.allGroups') }}</span>
              <span class="ml-2 shrink-0 text-[11px] text-gray-400 dark:text-dark-400">
                {{ allRows.length }}
              </span>
            </button>
          </li>
          <li v-for="g in allGroups" :key="g.id">
            <button
              type="button"
              :class="groupBtnClass(g.id)"
              @click="selectGroup(g.id)"
            >
              <span class="truncate">{{ g.name }}</span>
              <span class="ml-2 shrink-0 text-[11px] text-gray-400 dark:text-dark-400">
                {{ g.count }}
              </span>
            </button>
          </li>
        </ul>
      </aside>

      <!-- Main table -->
      <div class="min-w-0 flex-1">
        <ModelPlazaTable
          :rows="rows"
          :loading="loading"
          :currency-display="currencyToggle.display.value"
          :format-usd="formatUsd"
          :format-base="formatBaseUsd"
          :recharge-multiplier="meta?.balance_recharge_multiplier ?? null"
          :selected-group-id="selectedGroupId"
          :platform-options="allPlatforms"
          @update:filter="onFilterChange"
          @currency-change="currencyToggle.set"
          @use-group="onUseGroup"
        />
      </div>
    </div>
  </PlazaLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { plazaAPI, type PlazaModelRow, type PlazaCurrencyMeta, type PlazaModelsFilter } from '@/api/plaza'
import { useCurrencyToggle } from '@/composables/useCurrencyToggle'
import { useAuthRedirect } from '@/composables/useAuthRedirect'
import PlazaLayout from './PlazaLayout.vue'
import ModelPlazaTable from '@/components/plaza/ModelPlazaTable.vue'

const { t } = useI18n()
const { gotoOrLogin } = useAuthRedirect()

// `allRows` is the unfiltered snapshot used to populate the persistent sidebar
// and the platform dropdown. It is captured exactly once on first load so the
// catalog of groups / platforms never shrinks as the user narrows the table.
const allRows = ref<PlazaModelRow[]>([])
const rows = ref<PlazaModelRow[]>([])
const meta = ref<PlazaCurrencyMeta | null>(null)
const loading = ref(false)

// Group selection lives at the page level — it is driven by the sidebar and
// passed down to the table as a controlled prop.
const selectedGroupId = ref<number | undefined>(undefined)

// Wire toggle to a getter so it always reads the latest meta — the meta is fetched
// asynchronously after the composable is constructed, hence the getter pattern.
const currencyToggle = useCurrencyToggle(() => meta.value?.balance_recharge_multiplier)

function formatUsd(amount: number, digits?: number): string {
  return currencyToggle.format(amount, 'USD', digits)
}

/**
 * Always render USD regardless of the CNY ⇄ USD toggle. The "原价" column reflects
 * the upstream LiteLLM list price, which is intrinsically denominated in USD —
 * converting it to CNY would imply a recharge-rate-dependent number that doesn't
 * exist upstream. Only the "站点价" column should track the toggle.
 */
function formatBaseUsd(amount: number, digits?: number): string {
  if (!Number.isFinite(amount)) return '$0'
  const fractionDigits = digits ?? 4
  const formatted = amount.toLocaleString(undefined, {
    minimumFractionDigits: 0,
    maximumFractionDigits: fractionDigits,
  })
  return `$${formatted}`
}

const allGroups = computed(() => {
  const counts = new Map<number, { id: number; name: string; count: number }>()
  for (const r of allRows.value) {
    const existing = counts.get(r.group_id)
    if (existing) existing.count += 1
    else counts.set(r.group_id, { id: r.group_id, name: r.group_name, count: 1 })
  }
  return Array.from(counts.values()).sort((a, b) => a.id - b.id)
})

const allPlatforms = computed(() => {
  const set = new Set<string>()
  for (const r of allRows.value) if (r.platform) set.add(r.platform)
  return Array.from(set).sort()
})

function groupBtnClass(id: number | undefined): string {
  const isSelected = selectedGroupId.value === id
  return [
    'flex w-full items-center justify-between rounded-lg px-3 py-2 text-left text-sm transition-colors',
    isSelected
      ? 'bg-primary-50 font-semibold text-primary-700 dark:bg-primary-900/30 dark:text-primary-200'
      : 'text-gray-700 hover:bg-gray-100 dark:text-dark-200 dark:hover:bg-dark-800',
  ].join(' ')
}

function selectGroup(id: number | undefined) {
  selectedGroupId.value = id
}

/**
 * "Use this group" CTA — funnel visitors into the create-key modal pre-selected
 * to that group. Anonymous visitors are routed through `/login?redirect=…` and
 * `LoginView` then forwards to the encoded `/keys?openCreate=1&group_id=…` URL.
 *
 * The query-string contract (`openCreate=1&group_id=<id>`) is consumed by
 * `KeysView.onMounted` which opens the modal and replaces the URL to clear
 * the params, so reload doesn't re-trigger the dialog.
 */
function onUseGroup(groupId: number) {
  void gotoOrLogin({
    path: '/keys',
    query: { openCreate: '1', group_id: String(groupId) },
  })
}

let lastFilterKey = ''
async function load(filter: PlazaModelsFilter = {}, isInitial = false) {
  loading.value = true
  try {
    const resp = await plazaAPI.listModels(filter)
    rows.value = resp.rows ?? []
    meta.value = resp.currency_meta
    if (isInitial) {
      // Snapshot the unfiltered catalog exactly once so the sidebar stays stable.
      allRows.value = resp.rows ?? []
    }
  } catch {
    // Public endpoint: keep prior rows on transient error rather than blanking the page.
  } finally {
    loading.value = false
  }
}

function onFilterChange(filter: { groupId?: number; platform?: string; q?: string }) {
  // The table also filters client-side; we still refresh from the backend so server-side caps
  // (e.g. pagination later) stay accurate. Skip duplicate fetches caused by debouncing.
  const apiFilter: PlazaModelsFilter = {
    group_id: filter.groupId,
    platform: filter.platform,
    q: filter.q,
  }
  const key = JSON.stringify(apiFilter)
  if (key === lastFilterKey) return
  lastFilterKey = key
  load(apiFilter)
}

onMounted(() => {
  lastFilterKey = JSON.stringify({})
  load({}, /* isInitial */ true)
})
</script>
