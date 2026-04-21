<template>
  <div class="group relative inline-block">
    <span
      class="inline-flex cursor-help items-center rounded-md border border-gray-200 bg-gray-50 px-2 py-0.5 text-xs font-medium text-gray-700 transition-colors hover:border-brand-400 hover:bg-brand-50 hover:text-brand-700 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-brand-500 dark:hover:bg-brand-900/30 dark:hover:text-brand-300"
    >
      <span
        v-if="showPlatform && model.platform"
        class="mr-1 rounded bg-gray-200 px-1 text-[10px] uppercase text-gray-600 dark:bg-dark-700 dark:text-gray-400"
      >
        {{ model.platform }}
      </span>
      {{ model.name }}
    </span>

    <div
      class="pointer-events-none invisible absolute left-1/2 z-50 mt-2 w-80 -translate-x-1/2 opacity-0 transition-opacity group-hover:visible group-hover:opacity-100"
    >
      <div
        class="rounded-lg border border-gray-200 bg-white p-3 text-xs shadow-lg dark:border-dark-600 dark:bg-dark-800"
      >
        <div
          class="mb-2 flex items-center justify-between gap-2 border-b border-gray-200 pb-2 dark:border-dark-600"
        >
          <span class="font-semibold text-gray-900 dark:text-gray-100">{{ model.name }}</span>
          <span
            v-if="model.platform"
            class="rounded bg-gray-100 px-1.5 py-0.5 text-[10px] uppercase text-gray-600 dark:bg-dark-700 dark:text-gray-400"
          >
            {{ model.platform }}
          </span>
        </div>

        <div v-if="!model.pricing" class="text-gray-500 dark:text-gray-400">
          {{ noPricingLabel }}
        </div>

        <div v-else class="space-y-2 text-gray-700 dark:text-gray-300">
          <div class="flex justify-between">
            <span class="text-gray-500 dark:text-gray-400">{{ t(prefixKey('billingMode')) }}</span>
            <span>{{ billingModeLabel }}</span>
          </div>

          <template v-if="model.pricing.billing_mode === BILLING_MODE_TOKEN">
            <PricingRow
              :label="t(prefixKey('inputPrice'))"
              :value="model.pricing.input_price"
              :unit="t(prefixKey('unitPerMillion'))"
              :scale="perMillionScale"
            />
            <PricingRow
              :label="t(prefixKey('outputPrice'))"
              :value="model.pricing.output_price"
              :unit="t(prefixKey('unitPerMillion'))"
              :scale="perMillionScale"
            />
            <PricingRow
              :label="t(prefixKey('cacheWritePrice'))"
              :value="model.pricing.cache_write_price"
              :unit="t(prefixKey('unitPerMillion'))"
              :scale="perMillionScale"
            />
            <PricingRow
              :label="t(prefixKey('cacheReadPrice'))"
              :value="model.pricing.cache_read_price"
              :unit="t(prefixKey('unitPerMillion'))"
              :scale="perMillionScale"
            />
          </template>

          <PricingRow
            v-if="
              model.pricing.billing_mode === BILLING_MODE_PER_REQUEST &&
              model.pricing.per_request_price != null
            "
            :label="t(prefixKey('perRequestPrice'))"
            :value="model.pricing.per_request_price"
            :unit="t(prefixKey('unitPerRequest'))"
            :scale="1"
          />

          <PricingRow
            v-if="
              model.pricing.billing_mode === BILLING_MODE_IMAGE &&
              model.pricing.image_output_price != null
            "
            :label="t(prefixKey('imageOutputPrice'))"
            :value="model.pricing.image_output_price"
            :unit="t(prefixKey('unitPerRequest'))"
            :scale="1"
          />

          <div
            v-if="model.pricing.intervals && model.pricing.intervals.length > 0"
            class="mt-2 border-t border-gray-200 pt-2 dark:border-dark-600"
          >
            <div class="mb-1 font-medium text-gray-600 dark:text-gray-400">
              {{ t(prefixKey('intervals')) }}
            </div>
            <div class="space-y-1">
              <div
                v-for="(iv, idx) in model.pricing.intervals"
                :key="idx"
                class="flex justify-between text-[11px]"
              >
                <span class="text-gray-500 dark:text-gray-400">
                  <template v-if="iv.tier_label">{{ iv.tier_label }}</template>
                  <template v-else>{{ formatRange(iv.min_tokens, iv.max_tokens) }}</template>
                </span>
                <span>{{ formatInterval(iv, model.pricing.billing_mode) }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import PricingRow from './PricingRow.vue'
import { formatScaled } from '@/utils/pricing'
import {
  BILLING_MODE_TOKEN,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_IMAGE,
  type BillingMode
} from '@/constants/channel'
// 复用 api/channels.ts 的用户侧最小形态 DTO。
// admin 侧 ChannelModelPricing 字段更多，但结构上是用户 DTO 的超集，admin 视图传入可直接通过结构化子类型检查。
import type { UserPricingInterval, UserSupportedModel } from '@/api/channels'

const props = withDefaults(
  defineProps<{
    model: UserSupportedModel
    /** i18n 前缀：管理端传 `admin.availableChannels.pricing`，用户端传 `availableChannels.pricing`。 */
    pricingKeyPrefix?: string
    noPricingLabel?: string
    showPlatform?: boolean
  }>(),
  {
    pricingKeyPrefix: 'availableChannels.pricing',
    noPricingLabel: '',
    showPlatform: true
  }
)

const { t } = useI18n()

/** 按 token 定价展示时的换算单位：每百万 token。 */
const perMillionScale = 1_000_000

function prefixKey(k: string): string {
  return `${props.pricingKeyPrefix}.${k}`
}

const billingModeLabel = computed(() => {
  const mode = props.model.pricing?.billing_mode
  switch (mode) {
    case BILLING_MODE_TOKEN:
      return t(prefixKey('billingModeToken'))
    case BILLING_MODE_PER_REQUEST:
      return t(prefixKey('billingModePerRequest'))
    case BILLING_MODE_IMAGE:
      return t(prefixKey('billingModeImage'))
    default:
      return '-'
  }
})

function formatRange(min: number, max: number | null): string {
  const maxLabel = max == null ? '∞' : String(max)
  return `(${min}, ${maxLabel}]`
}

function formatInterval(iv: UserPricingInterval, mode: BillingMode): string {
  if (mode === BILLING_MODE_PER_REQUEST || mode === BILLING_MODE_IMAGE) {
    return formatScaled(iv.per_request_price, 1)
  }
  const input = formatScaled(iv.input_price, perMillionScale)
  const output = formatScaled(iv.output_price, perMillionScale)
  return `${input} / ${output}`
}
</script>
