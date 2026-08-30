<template>
  <section
    :id="sectionId"
    data-testid="provider-card"
    class="scroll-mt-4 overflow-hidden rounded-lg border border-[#dce3f7] bg-white shadow-none dark:border-dark-700 dark:bg-dark-800/50"
    :style="accentStyle"
  >
    <header class="relative border-b border-[#e9edf7] bg-[#f7f9ff] px-5 py-3.5 dark:border-dark-700/60 dark:bg-dark-900/30 sm:px-6">
      <span class="absolute inset-y-0 left-0 w-[3px] bg-[var(--group-accent)]"></span>
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="flex min-w-0 items-center gap-3.5">
          <div
            class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-[#dce3f7] bg-white dark:border-dark-700 dark:bg-dark-800"
            :class="platformBadgeLightClass(provider)"
          >
            <ProviderLogo
              :provider="provider"
              :logo-key="logoKey"
              :logo-url="logoUrl"
              :alt="providerName"
              size="lg"
            />
          </div>
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <h2 class="text-lg font-bold text-[#111a34] dark:text-white">{{ providerName }}</h2>
              <span class="rounded-md bg-[#eef1f7] px-2 py-0.5 text-[11px] font-semibold text-[#677594] dark:bg-dark-700 dark:text-dark-300">
                {{ billingModeLabel }}
              </span>
            </div>
            <p class="mt-0.5 text-xs text-[#8b96ad] dark:text-dark-500">
              {{ t('modelPlaza.detail.currencyUnit', { currency: currencyLabel }) }}
            </p>
          </div>
        </div>
        <span class="rounded-md border border-[#dce3f7] bg-white px-3 py-2 text-sm font-semibold text-[#445373] dark:border-dark-700 dark:bg-dark-900/60 dark:text-dark-200">
          {{ t('modelPlaza.detail.modelCount', { count: models.length }) }}
        </span>
      </div>
    </header>

    <div
      v-if="providerNote"
      data-testid="provider-note"
      class="whitespace-pre-line border-b border-[#e9edf7] bg-[#fbfcff] px-5 py-3 text-sm leading-6 text-[#455372] dark:border-dark-700/60 dark:bg-dark-900/20 dark:text-dark-200 sm:px-6"
    >
      {{ providerNote }}
    </div>

    <div
      v-if="billingMode === BILLING_MODE_TOKEN || billingMode === BILLING_MODE_IMAGE"
      class="flex flex-wrap items-center gap-x-5 gap-y-2 border-b border-[#e9edf7] bg-white px-5 py-2.5 text-xs dark:border-dark-700/60 dark:bg-dark-800/30 sm:px-6"
      data-testid="price-legend"
    >
      <span class="inline-flex items-center gap-2 text-[#74809b] dark:text-dark-400">
        <span class="h-2.5 w-2.5 rounded-full bg-slate-400 dark:bg-dark-500"></span>
        {{ billingMode === BILLING_MODE_IMAGE ? t('modelPlaza.table.basePrice') : t('modelPlaza.table.officialPrice') }}
      </span>
      <span class="inline-flex items-center gap-2 font-semibold text-[#315bd6] dark:text-blue-300">
        <span class="h-2.5 w-2.5 rounded-full bg-[#315bd6] dark:bg-blue-400"></span>
        {{ t('modelPlaza.table.sitePrice') }}
      </span>
    </div>

    <PlazaModelPricingTable
      :models="models"
      :platform="provider"
      :billing-mode="billingMode"
      :currency="currency"
    />
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import ProviderLogo from './ProviderLogo.vue'
import PlazaModelPricingTable from './PlazaModelPricingTable.vue'
import type { DisplayPriceCurrency, DisplayPriceModel } from '@/api/modelPrices'
import type { BillingMode } from '@/constants/channel'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_TOKEN,
  BILLING_MODE_VIDEO
} from '@/constants/channel'
import {
  platformAccentColor,
  platformBadgeLightClass
} from '@/utils/platformColors'

const props = defineProps<{
  provider: string
  providerName: string
  providerNote?: string
  logoKey?: string
  logoUrl?: string
  currency: DisplayPriceCurrency
  billingMode: BillingMode
  models: DisplayPriceModel[]
  sectionId?: string
}>()

const { t } = useI18n()
const accentStyle = computed(() => ({ '--group-accent': platformAccentColor(props.provider) }))
const currencyLabel = computed(() => (props.currency === 'CNY' ? 'CNY · ¥' : 'USD · $'))
const billingModeLabel = computed(() => {
  if (props.billingMode === BILLING_MODE_TOKEN) return t('modelPlaza.filters.token')
  if (props.billingMode === BILLING_MODE_PER_REQUEST) return t('modelPlaza.filters.perRequest')
  if (props.billingMode === BILLING_MODE_IMAGE) return t('modelPlaza.filters.image')
  if (props.billingMode === BILLING_MODE_VIDEO) return t('modelPlaza.filters.video')
  return props.billingMode
})
</script>
