<template>
  <div class="overflow-hidden rounded-2xl border border-gray-200/70 bg-white/80 shadow-sm backdrop-blur-sm dark:border-dark-700/70 dark:bg-dark-900/60">
    <!-- Recharge ratio banner -->
    <div
      v-if="rechargeMultiplier && rechargeMultiplier > 0"
      class="flex flex-col gap-1 border-b border-amber-200/70 bg-gradient-to-r from-amber-50 via-amber-50/70 to-orange-50 px-4 py-3 text-sm dark:border-amber-500/30 dark:from-amber-900/20 dark:via-amber-900/15 dark:to-orange-900/20 sm:flex-row sm:items-center sm:gap-3"
    >
      <span
        class="inline-flex items-center gap-1.5 self-start rounded-full bg-amber-500/15 px-2.5 py-0.5 text-xs font-semibold uppercase tracking-wider text-amber-700 dark:bg-amber-400/15 dark:text-amber-300"
      >
        <svg class="h-3.5 w-3.5" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
          <path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clip-rule="evenodd" />
        </svg>
        {{ t('plaza.models.rechargeRatio') }}
      </span>
      <span class="font-medium text-gray-800 dark:text-dark-100">
        {{ t('plaza.models.rechargeNote', { usd: rechargeMultiplier.toFixed(2) }) }}
      </span>
      <span class="text-xs text-gray-500 dark:text-dark-400 sm:ml-auto">
        {{ t('plaza.models.rechargeHint') }}
      </span>
    </div>

    <!-- Toolbar -->
    <div class="flex flex-col gap-3 border-b border-gray-200/70 px-4 py-3 dark:border-dark-700/70 lg:flex-row lg:items-center lg:justify-between">
      <div class="flex flex-1 flex-wrap items-center gap-2">
        <div class="relative w-full sm:w-72">
          <input
            v-model="searchQuery"
            type="text"
            :maxlength="MAX_Q_LEN"
            :placeholder="t('plaza.models.searchPlaceholder')"
            class="input pl-3"
          />
        </div>
        <div class="w-44">
          <Select
            :modelValue="platform"
            @update:modelValue="platform = (($event as string | null) ?? '')"
            :options="platformSelectOptions"
            :placeholder="t('plaza.models.allPlatforms')"
          />
        </div>
        <button class="btn btn-secondary text-xs" @click="resetFilters">
          {{ t('plaza.common.reset') }}
        </button>
      </div>
      <div class="flex items-center gap-3">
        <span class="text-xs text-gray-500 dark:text-dark-400">{{ t('plaza.common.currency') }}</span>
        <CurrencyToggle :model-value="currencyDisplay" @update:model-value="emit('currency-change', $event)" />
      </div>
    </div>

    <!-- Table -->
    <div class="overflow-x-auto">
      <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
        <thead class="bg-gray-50 text-xs uppercase tracking-wider text-gray-500 dark:bg-dark-800 dark:text-dark-400">
          <tr>
            <th class="px-4 py-3 text-left font-medium">{{ t('plaza.models.col.group') }}</th>
            <th class="px-4 py-3 text-left font-medium">{{ t('plaza.models.col.model') }}</th>
            <th class="px-4 py-3 text-left font-medium">{{ t('plaza.models.col.platform') }}</th>
            <th class="px-4 py-3 text-left font-medium">{{ t('plaza.models.col.type') }}</th>
            <th class="px-4 py-3 text-left font-medium">
              <div class="flex items-center gap-1.5">
                <span>{{ t('plaza.models.col.basePrice') }}</span>
                <span class="rounded bg-gray-200 px-1 py-0.5 text-[10px] font-semibold text-gray-600 dark:bg-dark-700 dark:text-dark-300">USD</span>
              </div>
            </th>
            <th class="px-4 py-3 text-left font-medium">{{ t('plaza.models.col.sitePrice') }}</th>
            <th class="px-4 py-3 text-left font-medium">{{ t('plaza.models.col.multiplier') }}</th>
            <!--
              Per-row "Use this group" CTA column. Placed at the end of the
              row so the price columns above are the visual anchor. The
              header label is intentionally short ("操作" / "Action") so it
              doesn't compete with pricing data for attention.
            -->
            <th class="px-4 py-3 text-right font-medium">{{ t('plaza.models.col.action') }}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-900">
          <tr v-if="loading">
            <td colspan="8" class="px-4 py-12 text-center text-sm text-gray-500 dark:text-dark-400">
              {{ t('common.loading') }}
            </td>
          </tr>
          <tr v-else-if="filteredRows.length === 0">
            <td colspan="8" class="px-4 py-12 text-center text-sm text-gray-500 dark:text-dark-400">
              {{ t('plaza.models.empty') }}
            </td>
          </tr>
          <tr
            v-else
            v-for="row in filteredRows"
            :key="`${row.group_id}-${row.platform}-${row.model}-${row.type}`"
            class="hover:bg-gray-50 dark:hover:bg-dark-800/50"
          >
            <td class="whitespace-nowrap px-4 py-3 font-medium text-gray-900 dark:text-white">
              {{ row.group_name }}
            </td>
            <td class="px-4 py-3 font-mono text-xs text-gray-700 dark:text-dark-200">{{ row.model }}</td>
            <td class="px-4 py-3 text-gray-600 dark:text-dark-300">{{ row.platform }}</td>
            <td class="px-4 py-3">
              <span
                :class="[
                  'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
                  row.type === 'token'
                    ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
                    : 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300',
                ]"
              >
                {{ row.type === 'token' ? t('plaza.models.type.token') : t('plaza.models.type.image') }}
              </span>
            </td>

            <!-- Base price -->
            <td class="whitespace-nowrap px-4 py-3 text-gray-700 dark:text-dark-200">
              <template v-if="row.type === 'token'">
                <div class="space-y-0.5 text-xs">
                  <div>
                    <span class="text-gray-400">{{ t('plaza.models.in') }}</span>
                    {{ formatTokenBase(row.input_price_per_mtok) }}
                  </div>
                  <div>
                    <span class="text-gray-400">{{ t('plaza.models.out') }}</span>
                    {{ formatTokenBase(row.output_price_per_mtok) }}
                  </div>
                  <div>
                    <span class="text-gray-400">{{ t('plaza.models.cache_write') }}</span>
                    {{ formatTokenBase(row.cache_write_price_per_mtok) }}
                  </div>
                  <div>
                    <span class="text-gray-400">{{ t('plaza.models.cache_read') }}</span>
                    {{ formatTokenBase(row.cache_read_price_per_mtok) }}
                  </div>
                </div>
              </template>
              <template v-else>
                <div class="grid grid-cols-3 gap-2 text-xs">
                  <div>
                    <div class="text-[10px] text-gray-400">1K</div>
                    {{ formatImageBase(row.base_image_prices?.tier_1k) }}
                  </div>
                  <div>
                    <div class="text-[10px] text-gray-400">2K</div>
                    {{ formatImageBase(row.base_image_prices?.tier_2k) }}
                  </div>
                  <div>
                    <div class="text-[10px] text-gray-400">4K</div>
                    {{ formatImageBase(row.base_image_prices?.tier_4k) }}
                  </div>
                </div>
              </template>
            </td>

            <!-- Site price -->
            <td class="whitespace-nowrap px-4 py-3 text-gray-700 dark:text-dark-200">
              <template v-if="row.type === 'token'">
                <div class="space-y-0.5 text-xs">
                  <div>
                    <span class="text-gray-400">{{ t('plaza.models.in') }}</span>
                    {{ formatTokenPrice(row.site_input_price_per_mtok) }}
                  </div>
                  <div>
                    <span class="text-gray-400">{{ t('plaza.models.out') }}</span>
                    {{ formatTokenPrice(row.site_output_price_per_mtok) }}
                  </div>
                  <div>
                    <span class="text-gray-400">{{ t('plaza.models.cache_write') }}</span>
                    {{ formatTokenPrice(row.site_cache_write_price_per_mtok) }}
                  </div>
                  <div>
                    <span class="text-gray-400">{{ t('plaza.models.cache_read') }}</span>
                    {{ formatTokenPrice(row.site_cache_read_price_per_mtok) }}
                  </div>
                </div>
              </template>
              <template v-else>
                <div class="grid grid-cols-3 gap-2 text-xs">
                  <div>
                    <div class="text-[10px] text-gray-400">1K</div>
                    {{ formatImagePrice(row.site_image_prices?.tier_1k) }}
                  </div>
                  <div>
                    <div class="text-[10px] text-gray-400">2K</div>
                    {{ formatImagePrice(row.site_image_prices?.tier_2k) }}
                  </div>
                  <div>
                    <div class="text-[10px] text-gray-400">4K</div>
                    {{ formatImagePrice(row.site_image_prices?.tier_4k) }}
                  </div>
                </div>
              </template>
            </td>

            <td class="whitespace-nowrap px-4 py-3 text-gray-700 dark:text-dark-200">
              ×{{ row.multiplier.toFixed(2) }}
            </td>

            <!--
              Per-row "Use this group" CTA. Emits the group id upward so the
              parent (`PlazaModelsView`) can run the auth-aware redirect into
              the create-key modal. Each model row points at exactly one
              group, so we send `row.group_id` directly.
            -->
            <td class="whitespace-nowrap px-4 py-3 text-right">
              <button
                type="button"
                class="rounded-md border border-primary-500/30 bg-primary-500/10 px-2.5 py-1.5 text-xs font-semibold text-primary-700 transition-colors hover:bg-primary-500/20 dark:border-primary-400/30 dark:text-primary-200"
                :title="t('plaza.use_group')"
                @click="emit('use-group', row.group_id)"
              >
                {{ t('plaza.use_group') }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PlazaModelRow } from '@/api/plaza'
import type { PlazaCurrency } from '@/composables/useCurrencyToggle'
import CurrencyToggle from './CurrencyToggle.vue'
import Select from '@/components/common/Select.vue'

const props = withDefaults(
  defineProps<{
    rows: PlazaModelRow[]
    loading: boolean
    currencyDisplay: PlazaCurrency
    /**
     * Money formatter that converts a USD-native amount to the currently displayed currency.
     * Caller wires this to `useCurrencyToggle().format(amount, 'USD', digits)`.
     * Used for the **site price** column, which reflects what the user pays at the site
     * and therefore tracks the CNY ⇄ USD toggle.
     */
    formatUsd: (amount: number, digits?: number) => string
    /**
     * Base-price formatter that ALWAYS renders USD regardless of the toggle.
     * The "原价" column is the upstream LiteLLM list price, which is fixed in USD;
     * exposing it in CNY would imply a recharge-rate-dependent number that doesn't
     * actually exist upstream. Optional for backwards compatibility — when omitted
     * we fall back to `formatUsd` (legacy behavior).
     */
    formatBase?: (amount: number, digits?: number) => string
    /**
     * Recharge multiplier (1 CNY → multiplier USD). When > 0, a prominent banner
     * is rendered at the top of the table explaining the conversion ratio.
     */
    rechargeMultiplier?: number | null
    /**
     * Group filter is owned by the parent (driven by the left-hand sidebar).
     * `undefined` means "all groups".
     */
    selectedGroupId?: number | undefined
    /**
     * Optional stable list of platform values to render in the platform dropdown.
     * When omitted, the list is derived from the currently visible rows, which is
     * fine for static datasets but will shrink whenever `selectedGroupId` narrows
     * the rows; pass an explicit list from the parent to keep choices stable.
     */
    platformOptions?: string[]
  }>(),
  {
    selectedGroupId: undefined,
    platformOptions: () => [],
    formatBase: undefined,
    rechargeMultiplier: undefined,
  }
)

const emit = defineEmits<{
  (e: 'update:filter', f: { groupId?: number; platform?: string; q?: string }): void
  (e: 'currency-change', c: PlazaCurrency): void
  /** "Use this group" per-row CTA. Parent decides routing/auth. */
  (e: 'use-group', groupId: number): void
}>()

const { t } = useI18n()

const MAX_Q_LEN = 64

const searchQuery = ref('')
const platform = ref('')

// Use the parent-provided list when available; otherwise derive from rows so the
// component still renders a sensible dropdown when used standalone.
const derivedPlatforms = computed(() => {
  const set = new Set<string>()
  for (const r of props.rows) if (r.platform) set.add(r.platform)
  return Array.from(set).sort()
})

const platformSelectOptions = computed(() => {
  const list = props.platformOptions.length > 0 ? props.platformOptions : derivedPlatforms.value
  // Use empty string as the "all" sentinel so the trigger renders the placeholder.
  return [
    { value: '', label: t('plaza.models.allPlatforms') },
    ...list.map((p) => ({ value: p, label: p })),
  ]
})

/**
 * Client-side filtering layered on top of whatever the server already returned.
 * The server-side filter is wired to `update:filter` for cases where rows are paginated
 * or refetched — for the v1 plaza we render in-memory, so this purely reflects current UI.
 */
const filteredRows = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  const gid = props.selectedGroupId
  return props.rows.filter((r) => {
    if (gid !== undefined && r.group_id !== gid) return false
    if (platform.value && r.platform.toLowerCase() !== platform.value.toLowerCase()) return false
    if (q && !r.model.toLowerCase().includes(q)) return false
    return true
  })
})

let debounceTimer: ReturnType<typeof setTimeout> | null = null
watch([searchQuery, () => props.selectedGroupId, platform], () => {
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(() => {
    emit('update:filter', {
      groupId: props.selectedGroupId,
      platform: platform.value || undefined,
      q: searchQuery.value.trim() || undefined,
    })
  }, 250)
})

function resetFilters() {
  // Reset only the in-component filters (search + platform). The group
  // selection is owned by the parent's left-hand sidebar; users clear it
  // by clicking the "All groups" entry there.
  searchQuery.value = ''
  platform.value = ''
}

/**
 * Verbose, industry-standard suffix for per-million-token prices. Centralised so
 * a future tweak (e.g. swapping to "/M tok" or localising) is a one-line change
 * and base/site formatters never drift apart.
 */
const PRICE_UNIT_SUFFIX = ' / M Tokens'

function formatTokenPrice(amount: number | undefined): string {
  if (amount === undefined || amount === null || !Number.isFinite(amount)) return '—'
  // Token prices are USD per Mtok; render with 4 decimals by default.
  return props.formatUsd(amount, 4) + PRICE_UNIT_SUFFIX
}

function formatImagePrice(amount: number | undefined): string {
  if (amount === undefined || amount === null || !Number.isFinite(amount)) return '—'
  return props.formatUsd(amount, 4)
}

// Base-price formatters: anchored to USD, immune to the currency toggle.
// Falls back to the toggle-aware formatter when no `formatBase` is provided so
// existing call sites keep working.
function formatTokenBase(amount: number | undefined): string {
  if (amount === undefined || amount === null || !Number.isFinite(amount)) return '—'
  const fmt = props.formatBase ?? props.formatUsd
  return fmt(amount, 4) + PRICE_UNIT_SUFFIX
}

function formatImageBase(amount: number | undefined): string {
  if (amount === undefined || amount === null || !Number.isFinite(amount)) return '—'
  const fmt = props.formatBase ?? props.formatUsd
  return fmt(amount, 4)
}
</script>
