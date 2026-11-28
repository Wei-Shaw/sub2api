<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition-opacity duration-200 ease-out"
      enter-from-class="opacity-0"
      enter-to-class="opacity-100"
      leave-active-class="transition-opacity duration-150 ease-in"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0"
    >
      <div
        v-if="open && model && group"
        class="fixed inset-0 z-[70] bg-gray-950/35 backdrop-blur-[1px]"
        @click.self="emit('close')"
      >
        <Transition
          appear
          enter-active-class="transition-transform duration-200 ease-out"
          enter-from-class="translate-x-full"
          enter-to-class="translate-x-0"
          leave-active-class="transition-transform duration-150 ease-in"
          leave-from-class="translate-x-0"
          leave-to-class="translate-x-full"
        >
          <aside
            ref="drawerRef"
            class="absolute inset-y-0 right-0 flex w-full max-w-[540px] flex-col bg-white shadow-2xl dark:bg-dark-900"
            role="dialog"
            aria-modal="true"
            :aria-labelledby="titleId"
          >
            <header
              class="flex shrink-0 items-start gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700"
            >
              <div
                class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800"
              >
                <ModelIcon
                  :model="model.name"
                  size="26px"
                  theme-aware
                  class="text-gray-900 dark:text-gray-100"
                />
              </div>
              <div class="min-w-0 flex-1">
                <h2
                  :id="titleId"
                  class="break-words text-base font-semibold text-gray-900 dark:text-white"
                >
                  {{ model.name }}
                </h2>
                <div class="mt-1 flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-dark-400">
                  <span class="inline-flex items-center gap-1.5">
                    <PlatformIcon :platform="model.platform as GroupPlatform" size="xs" />
                    {{ model.platform || group.platform }}
                  </span>
                  <span class="text-gray-300 dark:text-dark-600">/</span>
                  <span>{{ group.name }}</span>
                </div>
              </div>
              <button
                ref="closeButtonRef"
                type="button"
                class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40 dark:text-dark-500 dark:hover:bg-dark-800 dark:hover:text-gray-200"
                :aria-label="t('modelPlaza.detail.close')"
                @click="emit('close')"
              >
                <Icon name="x" size="sm" />
              </button>
            </header>

            <div class="min-h-0 flex-1 overflow-y-auto">
              <section class="border-b border-gray-100 px-5 py-5 dark:border-dark-700">
                <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
                  <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
                    {{ t('modelPlaza.detail.paidPricing') }}
                  </h3>
                  <span
                    class="rounded-md bg-gray-100 px-2 py-1 text-xs font-medium text-gray-600 dark:bg-dark-800 dark:text-dark-300"
                  >
                    {{ billingModeLabel }}
                  </span>
                </div>

                <template v-if="model.pricing">
                  <div
                    v-if="billingMode === BILLING_MODE_TOKEN && !tokenIntervals.length"
                    class="grid grid-cols-2 gap-3"
                  >
                    <PriceMetric
                      :label="t('modelPlaza.table.input')"
                      :value="paidPerMillion(model.pricing.input_price)"
                      :unit="t('modelPlaza.table.unitPerMillion')"
                    />
                    <PriceMetric
                      :label="t('modelPlaza.table.output')"
                      :value="paidPerMillion(model.pricing.output_price)"
                      :unit="t('modelPlaza.table.unitPerMillion')"
                    />
                  </div>

                  <div v-else-if="billingMode !== BILLING_MODE_TOKEN && !requestIntervals.length">
                    <PriceMetric
                      :label="t('modelPlaza.detail.unitPrice')"
                      :value="paidPerRequest(model.pricing.per_request_price)"
                      :unit="unitSuffix"
                    />
                  </div>

                  <div
                    v-if="pricingIntervals.length"
                    class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700"
                  >
                    <div
                      class="grid grid-cols-[minmax(90px,1fr)_1fr_1fr] bg-gray-50 px-3 py-2 text-[11px] font-medium text-gray-400 dark:bg-dark-800/70 dark:text-dark-500"
                    >
                      <span>{{ t('modelPlaza.detail.tier') }}</span>
                      <span>{{ intervalPriceLabel(0) }}</span>
                      <span>{{ intervalPriceLabel(1) }}</span>
                    </div>
                    <div
                      v-for="(interval, index) in pricingIntervals"
                      :key="index"
                      class="grid grid-cols-[minmax(90px,1fr)_1fr_1fr] border-t border-gray-100 px-3 py-2.5 text-xs dark:border-dark-700"
                    >
                      <span class="text-gray-500 dark:text-dark-400">{{ tierLabel(interval) }}</span>
                      <span class="font-mono font-semibold text-gray-900 dark:text-white">
                        {{ intervalPrice(interval, 0) }}
                      </span>
                      <span class="font-mono font-semibold text-gray-900 dark:text-white">
                        {{ intervalPrice(interval, 1) }}
                      </span>
                    </div>
                  </div>

                  <dl
                    v-if="hasPaidCache"
                    class="mt-4 grid grid-cols-2 gap-x-4 gap-y-2 border-t border-gray-100 pt-4 text-xs dark:border-dark-700"
                  >
                    <div>
                      <dt class="text-gray-400 dark:text-dark-500">
                        {{ t('modelPlaza.detail.cacheWrite') }}
                      </dt>
                      <dd class="mt-1 font-mono font-medium text-gray-800 dark:text-gray-200">
                        {{ paidPerMillion(model.pricing.cache_write_price) }}
                      </dd>
                    </div>
                    <div>
                      <dt class="text-gray-400 dark:text-dark-500">
                        {{ t('modelPlaza.detail.cacheRead') }}
                      </dt>
                      <dd class="mt-1 font-mono font-medium text-gray-800 dark:text-gray-200">
                        {{ paidPerMillion(model.pricing.cache_read_price) }}
                      </dd>
                    </div>
                  </dl>
                </template>
                <p v-else class="text-sm text-gray-400 dark:text-dark-500">
                  {{ t('modelPlaza.detail.noPricing') }}
                </p>
              </section>

              <section class="border-b border-gray-100 px-5 py-5 dark:border-dark-700">
                <h3 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
                  {{ t('modelPlaza.detail.officialReference') }}
                </h3>
                <template v-if="model.official_pricing">
                  <div class="grid grid-cols-2 gap-3">
                    <PriceMetric
                      :label="t('modelPlaza.table.input')"
                      :value="official(model.official_pricing.input_price)"
                      :unit="t('modelPlaza.table.unitPerMillion')"
                      muted
                    />
                    <PriceMetric
                      :label="t('modelPlaza.table.output')"
                      :value="official(model.official_pricing.output_price)"
                      :unit="t('modelPlaza.table.unitPerMillion')"
                      muted
                    />
                  </div>
                  <dl
                    v-if="hasOfficialCache"
                    class="mt-4 grid grid-cols-2 gap-x-4 gap-y-2 text-xs"
                  >
                    <div>
                      <dt class="text-gray-400 dark:text-dark-500">
                        {{ t('modelPlaza.detail.cacheWrite') }}
                      </dt>
                      <dd class="mt-1 font-mono text-gray-700 dark:text-dark-300">
                        {{ official(model.official_pricing.cache_write_price) }}
                        <template v-if="model.official_pricing.cache_write_1h_price != null">
                          <span class="font-sans text-gray-400 dark:text-dark-500"> · 1h </span>
                          {{ official(model.official_pricing.cache_write_1h_price) }}
                        </template>
                      </dd>
                    </div>
                    <div>
                      <dt class="text-gray-400 dark:text-dark-500">
                        {{ t('modelPlaza.detail.cacheRead') }}
                      </dt>
                      <dd class="mt-1 font-mono text-gray-700 dark:text-dark-300">
                        {{ official(model.official_pricing.cache_read_price) }}
                      </dd>
                    </div>
                  </dl>
                </template>
                <p v-else class="text-sm text-gray-400 dark:text-dark-500">
                  {{ t('modelPlaza.detail.noOfficialPricing') }}
                </p>
              </section>

              <section class="px-5 py-5">
                <h3 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
                  {{ t('modelPlaza.detail.groupInfo') }}
                </h3>
                <dl class="grid grid-cols-2 gap-x-5 gap-y-4 text-sm">
                  <div>
                    <dt class="text-xs text-gray-400 dark:text-dark-500">
                      {{ t('modelPlaza.detail.effectiveRate') }}
                    </dt>
                    <dd class="mt-1 font-mono font-semibold text-gray-900 dark:text-white">
                      <span
                        v-if="hasCustomRate"
                        class="mr-1.5 font-normal text-gray-400 line-through dark:text-dark-500"
                      >
                        {{ group.rate_multiplier }}x
                      </span>
                      {{ effectiveRate }}x
                    </dd>
                  </div>
                  <div>
                    <dt class="text-xs text-gray-400 dark:text-dark-500">
                      {{ t('modelPlaza.detail.groupType') }}
                    </dt>
                    <dd class="mt-1 text-gray-800 dark:text-gray-200">
                      {{
                        group.subscription_type === 'subscription'
                          ? t('modelPlaza.badges.subscription')
                          : t('modelPlaza.detail.standard')
                      }}
                    </dd>
                  </div>
                  <div>
                    <dt class="text-xs text-gray-400 dark:text-dark-500">
                      {{ t('modelPlaza.filters.platformLabel') }}
                    </dt>
                    <dd class="mt-1 text-gray-800 dark:text-gray-200">{{ group.platform }}</dd>
                  </div>
                  <div>
                    <dt class="text-xs text-gray-400 dark:text-dark-500">
                      {{ t('modelPlaza.detail.access') }}
                    </dt>
                    <dd class="mt-1 text-gray-800 dark:text-gray-200">
                      {{
                        group.is_exclusive
                          ? t('modelPlaza.badges.exclusive')
                          : t('modelPlaza.detail.publicGroup')
                      }}
                    </dd>
                  </div>
                </dl>
                <p
                  v-if="group.description"
                  class="mt-4 border-t border-gray-100 pt-4 text-sm leading-6 text-gray-500 dark:border-dark-700 dark:text-dark-400"
                >
                  {{ group.description }}
                </p>
                <p
                  v-if="peakNote"
                  class="mt-3 flex items-start gap-2 rounded-md bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300"
                >
                  <Icon name="clock" size="xs" class="mt-0.5 shrink-0" />
                  {{ peakNote }}
                </p>
              </section>
            </div>
          </aside>
        </Transition>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import { formatScaled } from '@/utils/pricing'
import { hasPeakRate, formatPeakRateWindow, serverTimezoneLabel } from '@/utils/peak-rate'
import { useAppStore } from '@/stores/app'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_TOKEN,
  type BillingMode
} from '@/constants/channel'
import type { ModelPlazaGroup, PlazaModel } from '@/api/modelPlaza'
import type { UserPricingInterval } from '@/api/channels'
import type { GroupPlatform } from '@/types'

let drawerId = 0

const props = defineProps<{
  open: boolean
  model: PlazaModel | null
  group: ModelPlazaGroup | null
}>()

const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const appStore = useAppStore()
const titleId = `model-plaza-detail-${++drawerId}`
const drawerRef = ref<HTMLElement | null>(null)
const closeButtonRef = ref<HTMLButtonElement | null>(null)
let previousActiveElement: HTMLElement | null = null

const PER_MILLION = 1_000_000
const MIN_DECIMALS = 2

const PriceMetric = defineComponent({
  props: {
    label: { type: String, required: true },
    value: { type: String, required: true },
    unit: { type: String, required: true },
    muted: { type: Boolean, default: false }
  },
  setup(metricProps) {
    return () =>
      h('div', { class: 'min-w-0 rounded-md bg-gray-50 px-3 py-3 dark:bg-dark-800/60' }, [
        h('div', { class: 'text-[11px] font-medium text-gray-400 dark:text-dark-500' }, metricProps.label),
        h(
          'div',
          {
            class: [
              'mt-1 break-words font-mono text-xl font-semibold',
              metricProps.muted
                ? 'text-gray-700 dark:text-gray-200'
                : 'text-gray-950 dark:text-white'
            ]
          },
          metricProps.value
        ),
        h('div', { class: 'mt-1 text-[10px] text-gray-400 dark:text-dark-500' }, metricProps.unit)
      ])
  }
})

const billingMode = computed<BillingMode>(
  () => props.model?.pricing?.billing_mode ?? BILLING_MODE_TOKEN
)
const billingModeLabel = computed(() => {
  if (billingMode.value === BILLING_MODE_IMAGE) return t('modelPlaza.table.perImage')
  if (billingMode.value === BILLING_MODE_PER_REQUEST) return t('modelPlaza.table.perRequest')
  return t('modelPlaza.filters.tokenBilling')
})
const effectiveRate = computed(
  () => props.group?.user_rate_multiplier ?? props.group?.rate_multiplier ?? 1
)
const hasCustomRate = computed(
  () =>
    props.group?.user_rate_multiplier != null &&
    props.group.user_rate_multiplier !== props.group.rate_multiplier
)
const tokenIntervals = computed(() =>
  billingMode.value === BILLING_MODE_TOKEN ? props.model?.pricing?.intervals ?? [] : []
)
const requestIntervals = computed(() =>
  billingMode.value !== BILLING_MODE_TOKEN
    ? (props.model?.pricing?.intervals ?? []).filter(
        (interval) => interval.per_request_price != null
      )
    : []
)
const pricingIntervals = computed(() =>
  billingMode.value === BILLING_MODE_TOKEN ? tokenIntervals.value : requestIntervals.value
)
const unitSuffix = computed(() =>
  billingMode.value === BILLING_MODE_IMAGE
    ? t('modelPlaza.table.perUnitImage')
    : t('modelPlaza.table.perUnitRequest')
)
const hasPaidCache = computed(
  () =>
    props.model?.pricing?.cache_write_price != null ||
    props.model?.pricing?.cache_read_price != null
)
const hasOfficialCache = computed(
  () =>
    props.model?.official_pricing?.cache_write_price != null ||
    props.model?.official_pricing?.cache_write_1h_price != null ||
    props.model?.official_pricing?.cache_read_price != null
)
const peakNote = computed(() => {
  if (!props.group || !hasPeakRate(props.group)) return ''
  const window = formatPeakRateWindow(
    props.group,
    serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset)
  )
  return t('modelPlaza.detail.peakNote', {
    window,
    multiplier: props.group.peak_rate_multiplier
  })
})

function paidPerMillion(value: number | null | undefined): string {
  if (value == null) return '-'
  return formatScaled(value * effectiveRate.value, PER_MILLION, MIN_DECIMALS)
}

function paidPerRequest(value: number | null | undefined): string {
  if (value == null) return '-'
  return formatScaled(value * effectiveRate.value, 1, MIN_DECIMALS)
}

function official(value: number | null | undefined): string {
  if (value == null) return '-'
  return formatScaled(value, PER_MILLION, MIN_DECIMALS)
}

function intervalPriceLabel(column: number): string {
  if (billingMode.value === BILLING_MODE_TOKEN) {
    return column === 0 ? t('modelPlaza.table.input') : t('modelPlaza.table.output')
  }
  return column === 0 ? t('modelPlaza.detail.unitPrice') : unitSuffix.value
}

function intervalPrice(interval: UserPricingInterval, column: number): string {
  if (billingMode.value === BILLING_MODE_TOKEN) {
    return paidPerMillion(column === 0 ? interval.input_price : interval.output_price)
  }
  return column === 0 ? paidPerRequest(interval.per_request_price) : unitSuffix.value
}

function tierLabel(interval: UserPricingInterval): string {
  if (interval.tier_label) return interval.tier_label
  const min = interval.min_tokens
  const max = interval.max_tokens
  if (max == null) return `>${formatTokenCount(min)}`
  if (min === 0) return `≤${formatTokenCount(max)}`
  return `${formatTokenCount(min)}–${formatTokenCount(max)}`
}

function formatTokenCount(value: number): string {
  if (value >= 1_000_000) return `${trimZero(value / 1_000_000)}M`
  if (value >= 1_000) return `${trimZero(value / 1_000)}K`
  return String(value)
}

function trimZero(value: number): string {
  return String(Math.round(value * 100) / 100)
}

function handleKeydown(event: KeyboardEvent): void {
  if (!props.open) return
  if (event.key === 'Escape') {
    emit('close')
    return
  }
  if (event.key !== 'Tab' || !drawerRef.value) return
  const focusable = [...drawerRef.value.querySelectorAll<HTMLElement>(
    'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
  )].filter((element) => !element.hasAttribute('disabled'))
  if (!focusable.length) return
  const first = focusable[0]
  const last = focusable[focusable.length - 1]
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first.focus()
  }
}

watch(
  () => props.open,
  async (isOpen) => {
    if (isOpen) {
      previousActiveElement = document.activeElement as HTMLElement
      document.body.classList.add('modal-open')
      await nextTick()
      closeButtonRef.value?.focus()
    } else {
      document.body.classList.remove('modal-open')
      previousActiveElement?.focus()
      previousActiveElement = null
    }
  },
  { immediate: true }
)

onMounted(() => document.addEventListener('keydown', handleKeydown))
onUnmounted(() => {
  document.removeEventListener('keydown', handleKeydown)
  document.body.classList.remove('modal-open')
})
</script>
