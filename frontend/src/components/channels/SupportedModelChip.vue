<template>
  <div class="relative inline-block">
    <span
      ref="triggerEl"
      :class="[
        'inline-flex cursor-help items-center gap-1 rounded-md border px-2 py-0.5 text-xs font-medium transition-colors',
        effectivePlatform
          ? platformBadgeClass(effectivePlatform)
          : 'border-gray-200 bg-gray-50 text-gray-700 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300',
      ]"
      @mouseenter="onEnter"
      @mouseleave="onLeave"
      @focusin="onEnter"
      @focusout="onLeave"
      tabindex="0"
    >
      <PlatformIcon
        v-if="effectivePlatform"
        :platform="effectivePlatform as GroupPlatform"
        size="xs"
      />
      <span
        v-if="showPlatform && model.platform"
        class="rounded bg-gray-200/60 px-1 text-[10px] uppercase text-gray-600 dark:bg-dark-700 dark:text-gray-400"
      >
        {{ model.platform }}
      </span>
      {{ model.name }}
    </span>

    <!-- Teleport to body so the popover is not clipped by card/overflow-hidden
         ancestors. Fixed-position coords are computed from the trigger's
         bounding rect; re-measured on enter / scroll / resize. -->
    <Teleport to="body">
      <div
        v-show="show"
        ref="popoverEl"
        role="tooltip"
        class="pointer-events-none fixed z-[99999] w-80 max-w-[min(22rem,calc(100vw-1rem))] rounded-lg border bg-white text-xs shadow-xl dark:bg-dark-800"
        :class="[popoverBorderClass]"
        :style="popoverStyle"
      >
        <!-- Header：平台主题色背景，含模型名 + 平台徽章 -->
        <div
          class="flex items-center justify-between gap-2 rounded-t-lg border-b px-3 py-2"
          :class="[popoverHeaderClass, popoverBorderClass]"
        >
          <span class="truncate font-semibold">{{ model.name }}</span>
          <span
            v-if="model.platform"
            class="flex-shrink-0 rounded bg-white/70 px-1.5 py-0.5 text-[10px] uppercase tracking-wide dark:bg-dark-900/60"
          >
            {{ model.platform }}
          </span>
        </div>

        <div class="p-3">
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
              <PricingRow
                v-if="model.pricing.image_output_price != null && model.pricing.image_output_price > 0"
                :label="t(prefixKey('imageOutputPrice'))"
                :value="model.pricing.image_output_price"
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
              class="mt-2 border-t pt-2"
              :class="[popoverBorderClass]"
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
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
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
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { GroupPlatform } from '@/types'
import { platformBadgeClass, platformBorderClass, platformBadgeLightClass } from '@/utils/platformColors'

const props = withDefaults(
  defineProps<{
    model: UserSupportedModel
    /** i18n 前缀：管理端传 `admin.availableChannels.pricing`，用户端传 `availableChannels.pricing`。 */
    pricingKeyPrefix?: string
    noPricingLabel?: string
    showPlatform?: boolean
    /**
     * 当 model.platform 缺失（如 admin 聚合场景）时，用父行的平台作为兜底着色。
     * 仅用于视觉，不影响业务逻辑。
     */
    platformHint?: string
  }>(),
  {
    pricingKeyPrefix: 'availableChannels.pricing',
    noPricingLabel: '',
    showPlatform: true,
    platformHint: ''
  }
)

const effectivePlatform = computed<string>(() => props.model.platform || props.platformHint || '')

const { t } = useI18n()

/** 按 token 定价展示时的换算单位：每百万 token。 */
const perMillionScale = 1_000_000

// Popover border + header classes echo the platform theme so each card reads
// at a glance which model family it belongs to.
const popoverBorderClass = computed(() =>
  effectivePlatform.value
    ? platformBorderClass(effectivePlatform.value)
    : 'border-gray-200 dark:border-dark-600',
)
const popoverHeaderClass = computed(() =>
  effectivePlatform.value
    ? platformBadgeLightClass(effectivePlatform.value)
    : 'bg-gray-50 text-gray-700 dark:bg-dark-700/60 dark:text-gray-300',
)

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

// ── Popover positioning ─────────────────────────────────────────────
// Teleport-to-body + fixed positioning avoids being clipped by
// overflow-hidden ancestors (the parent table card). We re-measure on
// hover enter, scroll, and resize. Pinning to the trigger's top-center
// with a flip when the viewport edge is near keeps it aligned without a
// full-blown positioning lib.
const show = ref(false)
const triggerEl = ref<HTMLElement | null>(null)
const popoverEl = ref<HTMLElement | null>(null)
const popoverStyle = ref<Record<string, string>>({ top: '0px', left: '0px' })

function updatePosition() {
  const trigger = triggerEl.value
  if (!trigger) return
  const rect = trigger.getBoundingClientRect()
  const margin = 8
  const popover = popoverEl.value
  const popWidth = popover?.offsetWidth ?? 320
  const popHeight = popover?.offsetHeight ?? 240
  const vw = window.innerWidth
  const vh = window.innerHeight

  let top = rect.bottom + margin
  // Flip upward if it would overflow below.
  if (top + popHeight > vh - margin) {
    top = Math.max(margin, rect.top - popHeight - margin)
  }

  let left = rect.left + rect.width / 2 - popWidth / 2
  if (left < margin) left = margin
  if (left + popWidth > vw - margin) left = vw - margin - popWidth

  popoverStyle.value = {
    top: `${Math.round(top)}px`,
    left: `${Math.round(left)}px`,
  }
}

function onEnter() {
  show.value = true
  nextTick(() => {
    updatePosition()
    window.addEventListener('scroll', updatePosition, true)
    window.addEventListener('resize', updatePosition)
  })
}

function onLeave() {
  show.value = false
  window.removeEventListener('scroll', updatePosition, true)
  window.removeEventListener('resize', updatePosition)
}

onBeforeUnmount(() => {
  window.removeEventListener('scroll', updatePosition, true)
  window.removeEventListener('resize', updatePosition)
})
</script>
