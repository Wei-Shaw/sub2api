<template>
  <div class="flex items-start gap-2 rounded border p-2"
       :class="isEmpty ? 'border-red-400 bg-red-50 dark:border-red-500 dark:bg-red-950/20' : 'border-gray-200 bg-white dark:border-dark-500 dark:bg-dark-700'">
    <!-- Token mode: context range + prices ($/MTok) -->
    <template v-if="mode === 'token'">
      <div class="grid min-w-0 flex-1 grid-cols-2 gap-2 sm:grid-cols-4 xl:grid-cols-7">
        <div>
          <label class="text-xs text-gray-400">{{ t('admin.channels.form.minTokens') }}</label>
          <input :value="interval.min_tokens" @input="emitField('min_tokens', toInt(($event.target as HTMLInputElement).value))"
            type="number" min="0" class="input mt-0.5 text-xs" />
        </div>
        <div>
          <label class="text-xs text-gray-400">{{ t('admin.channels.form.maxTokens') }} <span class="text-gray-300">{{ t('admin.channels.form.inclusive') }}</span></label>
          <input :value="interval.max_tokens ?? ''" @input="emitField('max_tokens', toIntOrNull(($event.target as HTMLInputElement).value))"
            type="number" min="0" class="input mt-0.5 text-xs" :placeholder="'∞'" />
        </div>
        <div>
          <label class="text-xs text-gray-400">{{ t('admin.channels.form.inputPrice') }} <span v-if="isEmpty" class="text-red-500">*</span> <span class="text-gray-300">$/M</span></label>
          <input :value="interval.input_price" @input="emitField('input_price', ($event.target as HTMLInputElement).value)"
            type="number" step="any" min="0" class="input mt-0.5 text-xs" />
        </div>
        <div>
          <label class="text-xs text-gray-400">{{ t('admin.channels.form.outputPrice') }} <span v-if="isEmpty" class="text-red-500">*</span> <span class="text-gray-300">$/M</span></label>
          <input :value="interval.output_price" @input="emitField('output_price', ($event.target as HTMLInputElement).value)"
            type="number" step="any" min="0" class="input mt-0.5 text-xs" />
        </div>
        <div>
          <label class="text-xs text-gray-400">{{ t('admin.channels.form.cacheWrite5mPriceShort') }} <span class="text-gray-300">$/M</span></label>
          <input :value="interval.cache_write_price" @input="emitField('cache_write_price', ($event.target as HTMLInputElement).value)"
            type="number" step="any" min="0" class="input mt-0.5 text-xs" />
        </div>
        <div>
          <label class="text-xs text-gray-400">{{ t('admin.channels.form.cacheWrite1hPriceShort') }} <span class="text-gray-300">$/M</span></label>
          <input :value="interval.cache_write_1h_price" @input="emitField('cache_write_1h_price', ($event.target as HTMLInputElement).value)"
            type="number" step="any" min="0" class="input mt-0.5 text-xs" />
        </div>
        <div>
          <label class="text-xs text-gray-400">{{ t('admin.channels.form.cacheReadPriceShort') }} <span class="text-gray-300">$/M</span></label>
          <input :value="interval.cache_read_price" @input="emitField('cache_read_price', ($event.target as HTMLInputElement).value)"
            type="number" step="any" min="0" class="input mt-0.5 text-xs" />
        </div>
        <template v-if="enableMultipliers">
          <div>
            <label class="text-xs text-gray-400">{{ t('admin.channels.form.inputMultiplier') }}</label>
            <input :value="interval.input_multiplier" @input="emitField('input_multiplier', ($event.target as HTMLInputElement).value)"
              type="number" step="any" min="0.000001" class="input mt-0.5 text-xs" />
          </div>
          <div>
            <label class="text-xs text-gray-400">{{ t('admin.channels.form.outputMultiplier') }}</label>
            <input :value="interval.output_multiplier" @input="emitField('output_multiplier', ($event.target as HTMLInputElement).value)"
              type="number" step="any" min="0.000001" class="input mt-0.5 text-xs" />
          </div>
          <div>
            <label class="text-xs text-gray-400">{{ t('admin.channels.form.cacheWriteMultiplier') }}</label>
            <input :value="interval.cache_write_multiplier" @input="emitField('cache_write_multiplier', ($event.target as HTMLInputElement).value)"
              type="number" step="any" min="0.000001" class="input mt-0.5 text-xs" />
          </div>
          <div>
            <label class="text-xs text-gray-400">{{ t('admin.channels.form.cacheReadMultiplier') }}</label>
            <input :value="interval.cache_read_multiplier" @input="emitField('cache_read_multiplier', ($event.target as HTMLInputElement).value)"
              type="number" step="any" min="0.000001" class="input mt-0.5 text-xs" />
          </div>
        </template>
      </div>
    </template>

    <!-- Per-request / Image mode: tier label + context range + price -->
    <template v-else>
      <div class="w-24">
        <label class="text-xs text-gray-400">
          {{ mode === 'image' ? t('admin.channels.form.resolution') : t('admin.channels.form.tierLabel') }}
        </label>
        <input :value="interval.tier_label" @input="emitField('tier_label', ($event.target as HTMLInputElement).value)"
          type="text" class="input mt-0.5 text-xs" :placeholder="mode === 'image' ? '1K / 2K / 4K' : ''" />
      </div>
      <div class="w-20">
        <label class="text-xs text-gray-400">{{ t('admin.channels.form.minTokens') }}</label>
        <input :value="interval.min_tokens" @input="emitField('min_tokens', toInt(($event.target as HTMLInputElement).value))"
          type="number" min="0" class="input mt-0.5 text-xs" />
      </div>
      <div class="w-20">
        <label class="text-xs text-gray-400">{{ t('admin.channels.form.maxTokens') }} <span class="text-gray-300">{{ t('admin.channels.form.inclusive') }}</span></label>
        <input :value="interval.max_tokens ?? ''" @input="emitField('max_tokens', toIntOrNull(($event.target as HTMLInputElement).value))"
          type="number" min="0" class="input mt-0.5 text-xs" :placeholder="'∞'" />
      </div>
      <div class="flex-1">
        <label class="text-xs text-gray-400">{{ t('admin.channels.form.perRequestPrice') }} <span v-if="isEmpty" class="text-red-500">*</span> <span class="text-gray-300">$</span></label>
        <input :value="interval.per_request_price" @input="emitField('per_request_price', ($event.target as HTMLInputElement).value)"
          type="number" step="any" min="0" class="input mt-0.5 text-xs" />
      </div>
    </template>

    <button type="button" @click="emit('remove')" class="mt-4 rounded p-0.5 text-gray-400 hover:text-red-500">
      <Icon name="x" size="sm" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { IntervalFormEntry } from './types'
import type { BillingMode } from '@/api/admin/channels'

const { t } = useI18n()

const props = defineProps<{
  interval: IntervalFormEntry
  mode: BillingMode
  enableMultipliers?: boolean
}>()

const emit = defineEmits<{
  update: [interval: IntervalFormEntry]
  remove: []
}>()

// 检测所有价格字段是否都为空
const isEmpty = computed(() => {
  const iv = props.interval
  return (iv.input_price == null || iv.input_price === '') &&
    (iv.output_price == null || iv.output_price === '') &&
    (iv.cache_write_price == null || iv.cache_write_price === '') &&
    (iv.cache_write_1h_price == null || iv.cache_write_1h_price === '') &&
    (iv.cache_read_price == null || iv.cache_read_price === '') &&
    (iv.input_multiplier == null || iv.input_multiplier === '') &&
    (iv.output_multiplier == null || iv.output_multiplier === '') &&
    (iv.cache_write_multiplier == null || iv.cache_write_multiplier === '') &&
    (iv.cache_read_multiplier == null || iv.cache_read_multiplier === '') &&
    (iv.per_request_price == null || iv.per_request_price === '')
})

function emitField(field: keyof IntervalFormEntry, value: string | number | null) {
  emit('update', { ...props.interval, [field]: value === '' ? null : value })
}

function toInt(val: string): number {
  const n = parseInt(val, 10)
  return isNaN(n) ? 0 : n
}

function toIntOrNull(val: string): number | null {
  if (val === '') return null
  const n = parseInt(val, 10)
  return isNaN(n) ? null : n
}
</script>
