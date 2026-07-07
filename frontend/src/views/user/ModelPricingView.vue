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
                ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300'
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
            <span class="font-semibold text-gray-900 dark:text-white">{{ t('modelPricing.rule.title') }}</span>
            <span>{{ t('modelPricing.rule.official') }}</span>
            <span class="hidden text-gray-300 dark:text-gray-600 sm:inline">·</span>
            <span>{{ t('modelPricing.rule.groupFormula') }}</span>
          </div>
          <div class="text-gray-500 dark:text-gray-400">
            {{ t('modelPricing.rule.example') }}
          </div>
        </div>
      </section>

      <section class="overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <header class="flex flex-col gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
          <div class="flex items-center gap-3">
            <span class="inline-flex h-9 w-9 items-center justify-center rounded-md bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200">
              <Icon name="badge" size="md" />
            </span>
            <div>
              <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('modelPricing.title') }}</h2>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('modelPricing.description') }}</p>
            </div>
          </div>

          <div class="relative w-full sm:w-72">
            <Icon
              name="search"
              size="sm"
              class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
            />
            <input
              v-model="searchQuery"
              type="text"
              :placeholder="t('modelPricing.searchPlaceholder')"
              class="input h-9 pl-9 text-sm"
            />
          </div>
        </header>

        <div v-if="loading" class="flex min-h-[360px] items-center justify-center">
          <Icon name="refresh" size="lg" class="animate-spin text-gray-400" />
        </div>

        <div v-else class="space-y-5 p-5">
          <div v-if="availableGroups.length > 0" class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4">
            <button
              v-for="group in availableGroups"
              :key="group.id"
              type="button"
              :class="[
                'rounded-lg border p-4 text-left transition-colors',
                selectedGroupId === group.id
                  ? 'border-primary-500 bg-primary-50/70 dark:bg-primary-900/15'
                  : 'border-gray-200 hover:border-gray-300 hover:bg-gray-50 dark:border-dark-700 dark:hover:border-dark-600 dark:hover:bg-dark-700/40',
              ]"
              @click="selectedGroupId = group.id"
            >
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <div class="truncate text-sm font-semibold text-gray-950 dark:text-white">{{ group.name }}</div>
                  <div class="mt-2 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('modelPricing.groupRateHint', { rate: formatMultiplier(effectiveGroupMultiplier(group)) }) }}
                  </div>
                </div>
                <span class="rounded-full px-2 py-0.5 text-[11px] font-semibold" :class="platformDiscountClass(selectedPlatform)">
                  {{ discountLabel(effectiveGroupMultiplier(group)) }}
                </span>
              </div>
            </button>
          </div>

          <div v-else class="rounded-lg border border-dashed border-gray-300 p-5 text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
            {{ t('modelPricing.noGroupsForPlatform') }}
          </div>

          <div v-if="filteredCards.length > 0" class="grid grid-cols-1 gap-4 xl:grid-cols-2">
            <article
              v-for="card in filteredCards"
              :key="card.key"
              class="rounded-lg border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800"
            >
              <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div class="min-w-0">
                  <div class="flex items-center gap-2">
                    <PlatformIcon :platform="card.platform as GroupPlatform" size="md" />
                    <h3 class="truncate text-sm font-semibold text-gray-950 dark:text-white">{{ card.model }}</h3>
                  </div>
                  <div class="mt-2 flex flex-wrap items-center gap-2 text-xs">
                    <span class="rounded px-1.5 py-0.5 font-semibold" :class="platformBadgeLightClass(card.platform)">
                      {{ t('modelPricing.billingModel') }}
                    </span>
                    <span class="text-gray-500 dark:text-gray-400">{{ billingModeLabel(card.billingMode) }}</span>
                  </div>
                </div>
                <span class="inline-flex w-fit rounded-full px-2.5 py-1 text-xs font-semibold" :class="savingBadgeClass(card.bestSaving)">
                  {{ savingLabel(card.bestSaving) }}
                </span>
              </div>

              <div class="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div
                  v-for="metric in card.metrics"
                  :key="metric.key"
                  class="rounded-md border border-gray-100 bg-gray-50 px-3 py-3 dark:border-dark-700 dark:bg-dark-700/35"
                >
                  <div class="text-xs text-gray-500 dark:text-gray-400">{{ metric.label }}</div>
                  <div class="mt-1 flex items-baseline gap-1">
                    <span class="text-base font-bold text-amber-700 dark:text-amber-300">
                      {{ formatCny(groupCny(metric.value, metric.scale, selectedMultiplier)) }}
                    </span>
                    <span class="text-[11px] text-gray-400 dark:text-gray-500">{{ metric.unit }}</span>
                  </div>
                  <div class="mt-1 text-[11px] text-gray-400 dark:text-gray-500">
                    {{ t('modelPricing.officialLabel') }}
                    <span class="line-through">{{ formatCny(officialCny(metric.value, metric.scale)) }}</span>
                  </div>
                </div>
              </div>
            </article>
          </div>

          <div v-else class="rounded-lg border border-dashed border-gray-300 p-10 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
            <Icon name="inbox" size="xl" class="mx-auto mb-3 text-gray-400" />
            {{ t('modelPricing.emptyModels') }}
          </div>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import userChannelsAPI, {
  type UserAvailableChannel,
  type UserAvailableGroup,
  type UserSupportedModelPricing,
} from '@/api/channels'
import type { BillingMode } from '@/constants/channel'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_TOKEN,
} from '@/constants/channel'
import type { GroupPlatform } from '@/types'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  platformBadgeLightClass,
  platformDiscountClass,
  platformLabel,
  platformTextClass,
} from '@/utils/platformColors'

interface PriceMetric {
  key: string
  label: string
  value: number | null
  scale: number
  unit: string
}

interface ModelPricingCard {
  key: string
  model: string
  platform: string
  billingMode: BillingMode
  metrics: PriceMetric[]
  bestSaving: number
}

const CNY_EXCHANGE_RATE = 7
const perMillionScale = 1_000_000

const { t } = useI18n()
const appStore = useAppStore()

const channels = ref<UserAvailableChannel[]>([])
const loading = ref(false)
const selectedPlatform = ref('')
const selectedGroupId = ref<number | null>(null)
const searchQuery = ref('')

const platformTabs = computed(() => {
  const platforms = new Set<string>()
  for (const channel of channels.value) {
    for (const section of channel.platforms) platforms.add(section.platform)
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

const availableGroups = computed(() => {
  const byID = new Map<number, UserAvailableGroup>()
  for (const channel of channels.value) {
    for (const section of channel.platforms) {
      if (section.platform !== selectedPlatform.value) continue
      for (const group of section.groups) byID.set(group.id, group)
    }
  }
  return Array.from(byID.values()).sort((a, b) => a.name.localeCompare(b.name))
})

const selectedGroup = computed(() => {
  return availableGroups.value.find((group) => group.id === selectedGroupId.value) || availableGroups.value[0] || null
})

const selectedMultiplier = computed(() => {
  return selectedGroup.value ? effectiveGroupMultiplier(selectedGroup.value) : 1
})

const modelCards = computed<ModelPricingCard[]>(() => {
  const cards: ModelPricingCard[] = []
  channels.value.forEach((channel, channelIndex) => {
    channel.platforms.forEach((section, sectionIndex) => {
      if (section.platform !== selectedPlatform.value) return
      section.supported_models.forEach((model, modelIndex) => {
        if (!model.pricing) return
        const metrics = buildMetrics(model.pricing)
        if (metrics.length === 0) return
        cards.push({
          key: `${channelIndex}-${sectionIndex}-${modelIndex}-${model.name}`,
          model: model.name,
          platform: model.platform,
          billingMode: model.pricing.billing_mode,
          metrics,
          bestSaving: Math.max(...metrics.map((metric) => savingRatio(metric.value, selectedMultiplier.value))),
        })
      })
    })
  })
  return cards
})

const filteredCards = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return modelCards.value
  return modelCards.value.filter((card) => card.model.toLowerCase().includes(q))
})

async function loadPricing() {
  loading.value = true
  try {
    const list = await userChannelsAPI.getPricing()
    channels.value = list
    ensureSelectedPlatform()
    ensureSelectedGroup()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('modelPricing.loadFailed')))
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

function buildMetrics(pricing: UserSupportedModelPricing): PriceMetric[] {
  if (pricing.billing_mode === BILLING_MODE_PER_REQUEST) {
    return [
      metric('perRequest', t('modelPricing.pricing.perRequestPrice'), pricing.per_request_price, 1, t('modelPricing.pricing.unitPerRequest')),
    ]
  }
  if (pricing.billing_mode === BILLING_MODE_IMAGE) {
    return [
      metric('imageOutput', t('modelPricing.pricing.imageOutputPrice'), pricing.image_output_price ?? pricing.per_request_price, 1, t('modelPricing.pricing.unitPerImage')),
    ]
  }
  return [
    metric('input', t('modelPricing.pricing.inputPrice'), pricing.input_price, perMillionScale, t('modelPricing.pricing.unitPerMillion')),
    metric('output', t('modelPricing.pricing.outputPrice'), pricing.output_price, perMillionScale, t('modelPricing.pricing.unitPerMillion')),
    metric('cacheWrite', t('modelPricing.pricing.cacheWritePrice'), pricing.cache_write_price, perMillionScale, t('modelPricing.pricing.unitPerMillion')),
    metric('cacheRead', t('modelPricing.pricing.cacheReadPrice'), pricing.cache_read_price, perMillionScale, t('modelPricing.pricing.unitPerMillion')),
  ].filter((item) => item.value != null)
}

function metric(key: string, label: string, value: number | null, scale: number, unit: string): PriceMetric {
  return { key, label, value, scale, unit }
}

function effectiveGroupMultiplier(group: UserAvailableGroup): number {
  return group.rate_multiplier ?? 1
}

function officialCny(value: number | null, scale: number): number | null {
  if (value == null) return null
  return value * scale * CNY_EXCHANGE_RATE
}

function groupCny(value: number | null, scale: number, multiplier: number): number | null {
  const official = officialCny(value, scale)
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
  if (value <= 0) return t('modelPricing.noSaving')
  return t('modelPricing.savePercent', { percent: Math.round(value * 100) })
}

function savingBadgeClass(value: number): string {
  if (value <= 0) return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
}

function discountLabel(multiplier: number): string {
  if (multiplier < 1) return t('modelPricing.discountLabel', { discount: trimNumber(multiplier * 10) })
  if (multiplier === 1) return t('modelPricing.standardRate')
  return t('modelPricing.multiplierLabel', { rate: trimNumber(multiplier) })
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
      return t('modelPricing.pricing.billingModePerRequest')
    case BILLING_MODE_IMAGE:
      return t('modelPricing.pricing.billingModeImage')
    case BILLING_MODE_TOKEN:
    default:
      return t('modelPricing.pricing.billingModeToken')
  }
}

function platformDisplayName(platform: string): string {
  const custom: Record<string, string> = {
    anthropic: 'Claude Code',
    openai: 'Codex',
  }
  return custom[platform] || platformLabel(platform)
}

onMounted(loadPricing)
</script>
