<template>
  <AppLayout>
    <div class="space-y-4">
      <section class="rounded-lg border border-gray-200 bg-white p-2 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-wrap items-center gap-2">
          <button
            v-for="tab in platformTabs"
            :key="tab.platform"
            type="button"
            :class="[
              'inline-flex h-10 items-center gap-2 rounded-md border px-4 text-sm font-medium transition-colors',
              selectedPlatform === tab.platform
                ? platformActiveTabClass(tab.platform)
                : 'border-transparent text-gray-600 hover:bg-gray-50 hover:text-gray-900 dark:text-gray-300 dark:hover:bg-dark-700 dark:hover:text-white',
            ]"
            @click="selectPlatform(tab.platform)"
          >
            <PlatformIcon :platform="tab.platform as GroupPlatform" size="md" />
            {{ tab.label }}
          </button>

          <button
            type="button"
            class="ml-auto inline-flex h-10 w-10 items-center justify-center rounded-md border border-gray-200 text-gray-500 transition-colors hover:bg-gray-50 hover:text-gray-900 disabled:opacity-60 dark:border-dark-700 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white"
            :disabled="loading"
            :title="t('common.refresh')"
            @click="loadPricing"
          >
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
        </div>
      </section>

      <section class="rounded-lg border border-gray-200 bg-white px-4 py-3 text-xs shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-2 lg:flex-row lg:items-center lg:justify-between">
          <div class="flex flex-wrap items-center gap-2 text-gray-600 dark:text-gray-300">
            <Icon name="calculator" size="sm" :class="platformTextClass(selectedPlatform)" />
            <span class="font-semibold text-gray-900 dark:text-white">{{ t('admin.pricing.rule.title') }}</span>
            <span>{{ t('admin.pricing.rule.official') }}</span>
            <span class="hidden text-gray-300 dark:text-gray-600 sm:inline">·</span>
            <span>{{ t('admin.pricing.rule.groupFormula') }}</span>
          </div>
          <div class="text-gray-500 dark:text-gray-400">
            {{ t('admin.pricing.rule.example') }}
          </div>
        </div>
      </section>

      <section class="overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <header class="flex flex-col gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
          <div class="flex items-center gap-3">
            <span
              class="inline-flex h-9 w-9 items-center justify-center rounded-md"
              :class="platformSoftClass(selectedPlatform)"
            >
              <Icon name="badge" size="md" />
            </span>
            <div>
              <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.pricing.priceList') }}</h2>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.pricing.priceListHint') }}</p>
            </div>
          </div>

          <div class="flex flex-col gap-2 sm:flex-row sm:items-center">
            <div class="relative w-full sm:w-72">
              <Icon
                name="search"
                size="sm"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
              />
              <input
                v-model="searchQuery"
                type="text"
                :placeholder="t('admin.pricing.searchPlaceholder')"
                class="input h-9 pl-9 text-sm"
              />
            </div>
            <div class="inline-flex h-9 rounded-full bg-gray-100 p-1 text-xs font-semibold dark:bg-dark-700">
              <span class="rounded-full bg-white px-3 py-1.5 text-gray-900 shadow-sm dark:bg-dark-800 dark:text-white">
                {{ t('admin.pricing.groupPrice') }}
              </span>
              <span class="px-3 py-1.5 text-gray-500 dark:text-gray-400">
                {{ t('admin.pricing.officialPrice') }}
              </span>
            </div>
          </div>
        </header>

        <div v-if="loading" class="flex min-h-[360px] items-center justify-center">
          <Icon name="refresh" size="lg" class="animate-spin text-gray-400" />
        </div>

        <div v-else class="p-5">
          <div v-if="availableGroups.length > 0" class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4">
            <button
              v-for="group in availableGroups"
              :key="group.id"
              type="button"
              :class="[
                'rounded-lg border p-4 text-left transition-colors',
                selectedGroupId === group.id
                  ? platformSelectedGroupClass(selectedPlatform)
                  : 'border-gray-200 hover:border-gray-300 hover:bg-gray-50 dark:border-dark-700 dark:hover:border-dark-600 dark:hover:bg-dark-700/40',
              ]"
              @click="selectedGroupId = group.id"
            >
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <div class="truncate text-sm font-semibold text-gray-950 dark:text-white">{{ group.name }}</div>
                  <div class="mt-2 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('admin.pricing.groupRateHint', { rate: formatMultiplier(group.rate_multiplier) }) }}
                  </div>
                </div>
                <span class="rounded-full px-2 py-0.5 text-[11px] font-semibold" :class="platformDiscountClass(selectedPlatform)">
                  {{ discountLabel(group.rate_multiplier) }}
                </span>
              </div>
            </button>
          </div>

          <div v-else class="rounded-lg border border-dashed border-gray-300 p-5 text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
            {{ t('admin.pricing.noGroupsForPlatform') }}
          </div>

          <div
            v-if="selectedGroup"
            class="mt-4 rounded-lg border px-4 py-3 text-sm"
            :class="platformInfoClass(selectedPlatform)"
          >
            <span class="font-semibold">{{ t('admin.pricing.groupIntro') }}</span>
            <span class="ml-2">{{ selectedGroup.description || t('admin.pricing.groupIntroFallback') }}</span>
          </div>

          <div class="mt-4 overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-700">
            <table class="w-full min-w-[880px] table-fixed border-collapse text-sm">
              <thead class="bg-gray-50 text-xs font-semibold text-gray-500 dark:bg-dark-700/60 dark:text-gray-400">
                <tr>
                  <th class="w-[240px] px-4 py-3 text-left">{{ t('admin.pricing.table.model') }}</th>
                  <th class="px-4 py-3 text-left">{{ t('admin.pricing.table.input') }}</th>
                  <th class="px-4 py-3 text-left">{{ t('admin.pricing.table.output') }}</th>
                  <th class="px-4 py-3 text-left">{{ t('admin.pricing.table.cacheWrite') }}</th>
                  <th class="px-4 py-3 text-left">{{ t('admin.pricing.table.cacheRead') }}</th>
                  <th class="w-[120px] px-4 py-3 text-left">{{ t('admin.pricing.table.saving') }}</th>
                </tr>
              </thead>
              <tbody v-if="filteredRows.length === 0">
                <tr>
                  <td colspan="6" class="px-4 py-14 text-center text-sm text-gray-500 dark:text-gray-400">
                    <Icon name="inbox" size="xl" class="mx-auto mb-3 text-gray-400" />
                    {{ t('admin.pricing.emptyModels') }}
                  </td>
                </tr>
              </tbody>
              <tbody v-else class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr
                  v-for="row in filteredRows"
                  :key="row.key"
                  class="transition-colors hover:bg-gray-50/70 dark:hover:bg-dark-700/35"
                >
                  <td class="px-4 py-4">
                    <div class="flex items-center gap-2">
                      <span class="font-semibold text-gray-900 dark:text-white">{{ row.model }}</span>
                      <span
                        class="rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase"
                        :class="platformBadgeLightClass(row.platform)"
                      >
                        {{ billingModeLabel(row.billingMode) }}
                      </span>
                    </div>
                  </td>
                  <td class="px-4 py-4"><PriceCellContent :official="row.inputPrice" /></td>
                  <td class="px-4 py-4"><PriceCellContent :official="row.outputPrice" /></td>
                  <td class="px-4 py-4"><PriceCellContent :official="row.cacheWritePrice" /></td>
                  <td class="px-4 py-4"><PriceCellContent :official="row.cacheReadPrice" /></td>
                  <td class="px-4 py-4">
                    <span class="inline-flex rounded-full px-2.5 py-1 text-xs font-semibold" :class="savingBadgeClass(row.bestSaving)">
                      {{ savingLabel(row.bestSaving) }}
                    </span>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, type PropType, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import adminAPI from '@/api/admin'
import type { Channel } from '@/api/admin/channels'
import type { AdminGroup, GroupPlatform } from '@/types'
import type { BillingMode } from '@/constants/channel'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_TOKEN,
} from '@/constants/channel'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  platformBadgeLightClass,
  platformDiscountClass,
  platformLabel,
  platformTextClass,
} from '@/utils/platformColors'

interface PricingRow {
  key: string
  model: string
  platform: string
  billingMode: BillingMode
  inputPrice: number | null
  outputPrice: number | null
  cacheWritePrice: number | null
  cacheReadPrice: number | null
  bestSaving: number
}

const CNY_EXCHANGE_RATE = 7
const perMillionScale = 1_000_000

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const groups = ref<AdminGroup[]>([])
const channels = ref<Channel[]>([])
const selectedPlatform = ref('')
const selectedGroupId = ref<number | null>(null)
const searchQuery = ref('')

const platformTabs = computed(() => {
  const platforms = new Set<string>()
  for (const group of groups.value) platforms.add(group.platform)
  for (const channel of channels.value) {
    for (const entry of channel.model_pricing || []) platforms.add(entry.platform)
  }

  const order = ['anthropic', 'openai', 'gemini', 'antigravity', 'grok']
  return Array.from(platforms)
    .sort((a, b) => {
      const ia = order.indexOf(a)
      const ib = order.indexOf(b)
      if (ia !== -1 || ib !== -1) return (ia === -1 ? 999 : ia) - (ib === -1 ? 999 : ib)
      return a.localeCompare(b)
    })
    .map((platform) => ({ platform, label: platformDisplayName(platform) }))
})

const availableGroups = computed(() =>
  groups.value
    .filter((group) => group.platform === selectedPlatform.value && group.status === 'active')
    .sort((a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0)),
)

const selectedGroup = computed(() => {
  return availableGroups.value.find((group) => group.id === selectedGroupId.value) || availableGroups.value[0] || null
})

const selectedMultiplier = computed(() => selectedGroup.value?.rate_multiplier ?? 1)

const modelRows = computed<PricingRow[]>(() => {
  return channels.value.flatMap((channel) =>
    (channel.model_pricing || [])
      .filter((entry) => entry.platform === selectedPlatform.value)
      .flatMap((entry, entryIndex) =>
        entry.models.map((model) => {
          const prices = normalizePriceFields(entry)
          const bestSaving = Math.max(...prices.map((price) => savingRatio(price, selectedMultiplier.value)))
          return {
            key: `${channel.id}-${entryIndex}-${model}`,
            model,
            platform: entry.platform,
            billingMode: entry.billing_mode,
            inputPrice: prices[0],
            outputPrice: prices[1],
            cacheWritePrice: prices[2],
            cacheReadPrice: prices[3],
            bestSaving,
          }
        }),
      ),
  )
})

const filteredRows = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return modelRows.value
  return modelRows.value.filter((row) => row.model.toLowerCase().includes(q))
})

async function loadAllChannels(): Promise<Channel[]> {
  const pageSize = 100
  const first = await adminAPI.channels.list(1, pageSize)
  const items = [...first.items]
  const total = first.total || items.length
  for (let page = 2; items.length < total; page += 1) {
    const next = await adminAPI.channels.list(page, pageSize)
    if (next.items.length === 0) break
    items.push(...next.items)
  }
  return items
}

async function loadPricing() {
  loading.value = true
  try {
    const [groupList, channelList] = await Promise.all([
      adminAPI.groups.getAllIncludingInactive(),
      loadAllChannels(),
    ])
    groups.value = groupList
    channels.value = channelList
    ensureSelectedPlatform()
    ensureSelectedGroup()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.pricing.loadFailed')))
  } finally {
    loading.value = false
  }
}

function ensureSelectedPlatform() {
  if (!platformTabs.value.some((tab) => tab.platform === selectedPlatform.value)) {
    selectedPlatform.value = platformTabs.value[0]?.platform || ''
  }
}

function ensureSelectedGroup() {
  if (!availableGroups.value.some((group) => group.id === selectedGroupId.value)) {
    selectedGroupId.value = availableGroups.value[0]?.id ?? null
  }
}

function selectPlatform(platform: string) {
  selectedPlatform.value = platform
}

watch(selectedPlatform, () => {
  searchQuery.value = ''
  ensureSelectedGroup()
})

function normalizePriceFields(entry: Channel['model_pricing'][number]): [number | null, number | null, number | null, number | null] {
  if (entry.billing_mode === BILLING_MODE_PER_REQUEST) {
    return [entry.per_request_price, null, null, null]
  }
  if (entry.billing_mode === BILLING_MODE_IMAGE) {
    return [entry.image_output_price ?? entry.per_request_price, null, null, null]
  }
  return [entry.input_price, entry.output_price, entry.cache_write_price, entry.cache_read_price]
}

function officialCny(value: number | null): number | null {
  if (value == null) return null
  return value * perMillionScale * CNY_EXCHANGE_RATE
}

function groupCny(value: number | null, multiplier: number): number | null {
  const official = officialCny(value)
  return official == null ? null : official * multiplier
}

function formatCny(value: number | null): string {
  if (value == null) return '-'
  if (value >= 1000) return `¥${value.toFixed(0)}`
  if (value >= 100) return `¥${value.toFixed(1)}`
  return `¥${value.toFixed(2).replace(/\.00$/, '')}`
}

function savingRatio(value: number | null, multiplier: number): number {
  if (value == null) return 0
  return Math.max(0, 1 - multiplier)
}

function savingLabel(value: number): string {
  if (value <= 0) return t('admin.pricing.noSaving')
  return t('admin.pricing.savePercent', { percent: Math.round(value * 100) })
}

function savingBadgeClass(value: number): string {
  if (value <= 0) return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
}

function discountLabel(multiplier: number): string {
  if (multiplier < 1) return t('admin.pricing.discountLabel', { discount: trimNumber(multiplier * 10) })
  if (multiplier === 1) return t('admin.pricing.standardRate')
  return t('admin.pricing.multiplierLabel', { rate: trimNumber(multiplier) })
}

function formatMultiplier(value: number): string {
  return `${trimNumber(value)}x`
}

function trimNumber(value: number): string {
  return Number.isInteger(value) ? String(value) : value.toFixed(2).replace(/0+$/, '').replace(/\.$/, '')
}

function billingModeLabel(mode: BillingMode): string {
  switch (mode) {
    case BILLING_MODE_PER_REQUEST:
      return t('admin.pricing.pricing.billingModePerRequest')
    case BILLING_MODE_IMAGE:
      return t('admin.pricing.pricing.billingModeImage')
    case BILLING_MODE_TOKEN:
    default:
      return t('admin.pricing.pricing.billingModeToken')
  }
}

function platformDisplayName(platform: string): string {
  const custom: Record<string, string> = {
    anthropic: 'Claude Code',
    openai: 'Codex',
  }
  return custom[platform] || platformLabel(platform)
}

function platformActiveTabClass(platform: string): string {
  switch (platform) {
    case 'anthropic':
      return 'border-orange-500 bg-orange-50 text-orange-700 dark:bg-orange-900/20 dark:text-orange-300'
    case 'openai':
      return 'border-emerald-500 bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'
    case 'gemini':
      return 'border-blue-500 bg-blue-50 text-blue-700 dark:bg-blue-900/20 dark:text-blue-300'
    case 'antigravity':
      return 'border-purple-500 bg-purple-50 text-purple-700 dark:bg-purple-900/20 dark:text-purple-300'
    case 'grok':
      return 'border-zinc-700 bg-zinc-100 text-zinc-800 dark:border-zinc-500 dark:bg-zinc-800 dark:text-zinc-100'
    default:
      return 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300'
  }
}

function platformSelectedGroupClass(platform: string): string {
  switch (platform) {
    case 'anthropic':
      return 'border-orange-500 bg-orange-50/70 dark:bg-orange-900/15'
    case 'openai':
      return 'border-emerald-500 bg-emerald-50/70 dark:bg-emerald-900/15'
    case 'gemini':
      return 'border-blue-500 bg-blue-50/70 dark:bg-blue-900/15'
    case 'antigravity':
      return 'border-purple-500 bg-purple-50/70 dark:bg-purple-900/15'
    case 'grok':
      return 'border-zinc-700 bg-zinc-50 dark:border-zinc-500 dark:bg-zinc-800/40'
    default:
      return 'border-primary-500 bg-primary-50/70 dark:bg-primary-900/15'
  }
}

function platformSoftClass(platform: string): string {
  switch (platform) {
    case 'anthropic':
      return 'bg-orange-50 text-orange-600 dark:bg-orange-900/25 dark:text-orange-300'
    case 'openai':
      return 'bg-emerald-50 text-emerald-600 dark:bg-emerald-900/25 dark:text-emerald-300'
    case 'gemini':
      return 'bg-blue-50 text-blue-600 dark:bg-blue-900/25 dark:text-blue-300'
    case 'antigravity':
      return 'bg-purple-50 text-purple-600 dark:bg-purple-900/25 dark:text-purple-300'
    case 'grok':
      return 'bg-zinc-100 text-zinc-800 dark:bg-zinc-800 dark:text-zinc-100'
    default:
      return 'bg-primary-50 text-primary-600 dark:bg-primary-900/25 dark:text-primary-300'
  }
}

function platformInfoClass(platform: string): string {
  switch (platform) {
    case 'anthropic':
      return 'border-orange-200 bg-orange-50 text-orange-800 dark:border-orange-900/50 dark:bg-orange-900/15 dark:text-orange-200'
    case 'openai':
      return 'border-emerald-200 bg-emerald-50 text-emerald-800 dark:border-emerald-900/50 dark:bg-emerald-900/15 dark:text-emerald-200'
    case 'gemini':
      return 'border-blue-200 bg-blue-50 text-blue-800 dark:border-blue-900/50 dark:bg-blue-900/15 dark:text-blue-200'
    case 'antigravity':
      return 'border-purple-200 bg-purple-50 text-purple-800 dark:border-purple-900/50 dark:bg-purple-900/15 dark:text-purple-200'
    case 'grok':
      return 'border-zinc-200 bg-zinc-50 text-zinc-800 dark:border-zinc-700 dark:bg-zinc-800/40 dark:text-zinc-200'
    default:
      return 'border-primary-200 bg-primary-50 text-primary-800 dark:border-primary-900/50 dark:bg-primary-900/15 dark:text-primary-200'
  }
}

const PriceCell = defineComponent({
  name: 'PriceCell',
  props: {
    official: { type: Number as PropType<number | null>, default: null },
    multiplier: { type: Number, required: true },
  },
  setup(props) {
    return () => {
      const group = groupCny(props.official, props.multiplier)
      const official = officialCny(props.official)
      return h('div', { class: 'space-y-0.5' }, [
        h('div', { class: 'text-sm font-bold text-amber-700 dark:text-amber-300' }, formatCny(group)),
        h('div', { class: 'text-[11px] text-gray-400 dark:text-gray-500' }, [
          t('admin.pricing.officialLabel'),
          ' ',
          h('span', { class: 'line-through' }, formatCny(official)),
        ]),
      ])
    }
  },
})

const PriceCellContent = defineComponent({
  name: 'PriceCellContent',
  props: {
    official: { type: Number as PropType<number | null>, default: null },
  },
  setup(props) {
    return () => h(PriceCell, { official: props.official, multiplier: selectedMultiplier.value })
  },
})

onMounted(loadPricing)
</script>
