<template>
  <div class="card" data-testid="recharge-bonus-tier-editor">
    <!-- Header：与服务商管理卡片同款 -->
    <div class="border-b border-gray-100 px-4 py-3 dark:border-dark-700">
      <div class="flex items-center justify-between gap-3">
        <div class="min-w-0">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">
            {{ t('admin.settings.payment.rechargeBonus.label') }}
          </h2>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.payment.rechargeBonus.hint') }}
          </p>
        </div>
        <button
          type="button"
          class="btn btn-primary btn-sm shrink-0"
          :disabled="tiers.length >= MAX_RECHARGE_BONUS_TIERS"
          data-testid="recharge-bonus-tier-add"
          @click="addTier"
        >
          <Icon name="plus" size="sm" class="mr-1" />
          {{ t('admin.settings.payment.rechargeBonus.addTier') }}
        </button>
      </div>
    </div>

    <div class="space-y-4 p-4">
      <!-- 模式：整条阶梯共用 -->
      <div class="flex flex-wrap items-center gap-x-3 gap-y-2">
        <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.settings.payment.rechargeBonus.modeLabel') }}
        </span>
        <div
          class="inline-flex rounded-lg border border-gray-200 bg-gray-50 p-0.5 dark:border-dark-600 dark:bg-dark-800"
          role="group"
        >
          <button
            v-for="option in modeOptions"
            :key="option.value"
            type="button"
            :class="[
              'rounded-md px-3 py-1 text-sm font-medium transition-colors',
              currentMode === option.value
                ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
                : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200',
            ]"
            :aria-pressed="currentMode === option.value"
            :data-testid="`recharge-bonus-mode-${option.value}`"
            @click="setMode(option.value)"
          >
            {{ option.label }}
          </button>
        </div>
        <span class="text-xs text-gray-400 dark:text-gray-500">
          {{ currentMode === 'discount'
            ? t('admin.settings.payment.rechargeBonus.modeDiscountHint')
            : t('admin.settings.payment.rechargeBonus.modeBonusHint') }}
        </span>
      </div>

      <!-- 档位列表：紧凑表格 -->
      <div
        v-if="tiers.length === 0"
        class="rounded-xl border border-dashed border-gray-300 px-4 py-3 text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400"
      >
        {{ t('admin.settings.payment.rechargeBonus.empty') }}
      </div>
      <div v-else class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-600">
        <div
          class="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_2rem] items-center gap-3 bg-gray-50 px-3 py-2 text-xs font-medium text-gray-500 dark:bg-dark-800 dark:text-gray-400"
        >
          <span>{{ t('admin.settings.payment.rechargeBonus.minAmountLabel') }}</span>
          <span>
            {{ currentMode === 'discount'
              ? t('admin.settings.payment.rechargeBonus.percentLabelDiscount')
              : t('admin.settings.payment.rechargeBonus.percentLabel') }}
          </span>
          <span></span>
        </div>
        <div class="divide-y divide-gray-100 dark:divide-dark-700">
          <div
            v-for="(tier, index) in tiers"
            :key="index"
            class="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_2rem] items-center gap-3 px-3 py-2"
            data-testid="recharge-bonus-tier-row"
          >
            <input
              :value="tier.min_amount ?? ''"
              type="number"
              step="0.01"
              min="0"
              class="input py-2"
              placeholder="100"
              data-testid="recharge-bonus-tier-min-input"
              @input="onMinAmountInput(index, $event)"
            />
            <div class="relative">
              <input
                :value="tier.bonus_percent ?? ''"
                type="number"
                step="0.01"
                min="0"
                :max="currentMode === 'discount' ? 99.99 : MAX_RECHARGE_BONUS_PERCENT"
                :class="['input py-2', currentMode === 'discount' ? 'pr-16' : 'pr-8']"
                placeholder="20"
                data-testid="recharge-bonus-tier-percent-input"
                @input="onPercentInput(index, $event)"
              />
              <span
                class="pointer-events-none absolute inset-y-0 right-0 flex items-center pr-3 text-xs font-medium"
                :class="currentMode === 'discount' ? 'text-red-600 dark:text-red-400' : 'text-gray-400'"
              >{{ currentMode === 'discount' ? '% OFF' : '%' }}</span>
            </div>
            <button
              type="button"
              class="flex h-8 w-8 items-center justify-center rounded-lg text-gray-400 hover:bg-red-50 hover:text-red-600 dark:hover:bg-dark-700 dark:hover:text-red-400"
              :title="t('admin.settings.payment.rechargeBonus.removeTier')"
              data-testid="recharge-bonus-tier-remove"
              @click="removeTier(index)"
            >
              <Icon name="x" size="sm" />
            </button>
            <p
              v-if="rowError(index)"
              class="col-span-3 -mt-1 text-xs text-red-600 dark:text-red-400"
              data-testid="recharge-bonus-tier-error"
            >
              {{ t(`admin.settings.payment.rechargeBonus.${rowError(index)}`) }}
            </p>
            <p
              v-else-if="rowIncomplete(index)"
              class="col-span-3 -mt-1 text-xs text-gray-400 dark:text-gray-500"
              data-testid="recharge-bonus-tier-incomplete"
            >
              {{ t('admin.settings.payment.rechargeBonus.incompleteRow') }}
            </p>
          </div>
        </div>
      </div>

      <!-- 区间预览：单行内联 -->
      <p
        v-if="intervals.length > 0"
        class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-500 dark:text-gray-400"
        data-testid="recharge-bonus-tier-preview"
      >
        <span class="font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.settings.payment.rechargeBonus.previewTitle') }}
        </span>
        <span
          v-for="(interval, index) in intervals"
          :key="index"
          class="rounded-md bg-gray-100 px-2 py-0.5 dark:bg-dark-700"
        >{{ intervalLabel(interval) }}</span>
      </p>

      <!-- 活动文案 -->
      <div class="border-t border-gray-100 pt-4 dark:border-dark-700">
        <label class="input-label">
          {{ t('admin.settings.payment.rechargeBonus.noticeLabel') }}
        </label>
        <textarea
          :value="notice ?? ''"
          rows="3"
          class="input"
          :placeholder="t('admin.settings.payment.rechargeBonus.noticePlaceholder')"
          data-testid="recharge-bonus-notice-input"
          @input="emit('update:notice', ($event.target as HTMLTextAreaElement).value)"
        ></textarea>
        <p class="mt-1 text-xs text-gray-400">
          {{ t('admin.settings.payment.rechargeBonus.noticeHint') }}
        </p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import Icon from '@/components/icons/Icon.vue'
import {
  MAX_RECHARGE_BONUS_PERCENT,
  MAX_RECHARGE_BONUS_TIERS,
  describeRechargeBonusIntervals,
  formatRechargeBonusNumber,
  isDuplicateRechargeBonusMinAmount,
  isRechargeBonusMinAmountValid,
  isRechargeBonusPercentValidForMode,
  normalizeRechargeBonusMode,
  sanitizeRechargeBonusTiersForSubmit,
  type RechargeBonusInterval,
  type RechargeBonusMode,
  type RechargeBonusTierDraft,
} from '@/utils/rechargeBonus'

const { t } = useI18n()

const props = defineProps<{
  modelValue?: RechargeBonusTierDraft[] | null
  mode?: RechargeBonusMode | string | null
  notice?: string | null
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: RechargeBonusTierDraft[]): void
  (e: 'update:mode', value: RechargeBonusMode): void
  (e: 'update:notice', value: string): void
}>()

const tiers = computed(() => props.modelValue ?? [])
const currentMode = computed<RechargeBonusMode>(() => normalizeRechargeBonusMode(props.mode))

const modeOptions = computed<{ value: RechargeBonusMode; label: string }[]>(() => [
  { value: 'bonus', label: t('admin.settings.payment.rechargeBonus.modeBonus') },
  { value: 'discount', label: t('admin.settings.payment.rechargeBonus.modeDiscount') },
])

const intervals = computed(() => describeRechargeBonusIntervals(sanitizeRechargeBonusTiersForSubmit(tiers.value)))

function setMode(mode: RechargeBonusMode) {
  if (mode === currentMode.value) return
  emit('update:mode', mode)
}

function cloneTiers(): RechargeBonusTierDraft[] {
  return tiers.value.map((tier) => ({ min_amount: tier.min_amount, bonus_percent: tier.bonus_percent }))
}

function addTier() {
  if (tiers.value.length >= MAX_RECHARGE_BONUS_TIERS) return
  emit('update:modelValue', [...cloneTiers(), { min_amount: null, bonus_percent: null }])
}

function removeTier(index: number) {
  const next = cloneTiers()
  next.splice(index, 1)
  emit('update:modelValue', next)
}

function parseNumberInput(event: Event): number | null {
  const raw = (event.target as HTMLInputElement).value
  if (raw === '') return null
  const parsed = Number(raw)
  return Number.isFinite(parsed) ? parsed : null
}

function onMinAmountInput(index: number, event: Event) {
  const next = cloneTiers()
  next[index]!.min_amount = parseNumberInput(event)
  emit('update:modelValue', next)
}

function onPercentInput(index: number, event: Event) {
  const next = cloneTiers()
  next[index]!.bonus_percent = parseNumberInput(event)
  emit('update:modelValue', next)
}

type RowError = '' | 'invalidMinAmount' | 'invalidPercent' | 'invalidDiscountPercent' | 'duplicateMinAmount'

function rowError(index: number): RowError {
  const tier = tiers.value[index]
  if (!tier) return ''
  if (tier.min_amount !== null && !isRechargeBonusMinAmountValid(tier.min_amount)) return 'invalidMinAmount'
  if (tier.bonus_percent !== null && !isRechargeBonusPercentValidForMode(tier.bonus_percent, currentMode.value)) {
    return currentMode.value === 'discount' && isRechargeBonusPercentValidForMode(tier.bonus_percent, 'bonus')
      ? 'invalidDiscountPercent'
      : 'invalidPercent'
  }
  if (isDuplicateRechargeBonusMinAmount(tiers.value, index)) return 'duplicateMinAmount'
  return ''
}

function rowIncomplete(index: number): boolean {
  const tier = tiers.value[index]
  return !!tier && (tier.min_amount === null || tier.bonus_percent === null)
}

function intervalLabel(interval: RechargeBonusInterval): string {
  const from = formatRechargeBonusNumber(interval.from)
  const percent = formatRechargeBonusNumber(interval.percent)
  const discount = currentMode.value === 'discount'
  if (interval.to === null) {
    if (interval.percent <= 0) return t('admin.settings.payment.rechargeBonus.previewOpenNone', { from })
    return discount
      ? t('admin.settings.payment.rechargeBonus.previewOpenDiscount', { from, percent })
      : t('admin.settings.payment.rechargeBonus.previewOpen', { from, percent })
  }
  const to = formatRechargeBonusNumber(interval.to)
  if (interval.percent <= 0) return t('admin.settings.payment.rechargeBonus.previewRangeNone', { from, to })
  return discount
    ? t('admin.settings.payment.rechargeBonus.previewRangeDiscount', { from, to, percent })
    : t('admin.settings.payment.rechargeBonus.previewRange', { from, to, percent })
}
</script>
