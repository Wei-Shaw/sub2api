<template>
  <div class="flex items-start gap-2 rounded border p-2"
       :class="isEmpty ? 'card-highlight-danger' : 'card-highlight-neutral'">
    <!-- Token mode: context range + prices ($/MTok) -->
    <template v-if="mode === 'token'">
      <div class="w-20">
        <label class="text-xs text-gray-400">{{ t('admin.channels.form.intervalMin') }}</label>
        <input :value="interval.min_tokens" @input="emitField('min_tokens', toInt(($event.target as HTMLInputElement).value))"
          type="number" min="0" class="input mt-0.5 text-xs" />
      </div>
      <div class="w-20">
        <label class="text-xs text-gray-400">{{ t('admin.channels.form.intervalMax') }} <span class="text-gray-300">{{ t('admin.channels.form.intervalInclusive') }}</span></label>
        <input :value="interval.max_tokens ?? ''" @input="emitField('max_tokens', toIntOrNull(($event.target as HTMLInputElement).value))"
          type="number" min="0" class="input mt-0.5 text-xs" :placeholder="'∞'" />
      </div>
      <div class="flex-1">
        <label class="text-xs text-gray-400">{{ t('admin.channels.form.inputPrice', '输入') }} <span v-if="isEmpty" class="input-required">*</span> <span class="text-gray-300">$/M</span></label>
        <input :value="interval.input_price" @input="emitField('input_price', ($event.target as HTMLInputElement).value)"
          type="number" step="any" min="0" class="input mt-0.5 text-xs" />
      </div>
      <div class="flex-1">
        <label class="text-xs text-gray-400">{{ t('admin.channels.form.outputPrice', '输出') }} <span v-if="isEmpty" class="input-required">*</span> <span class="text-gray-300">$/M</span></label>
        <input :value="interval.output_price" @input="emitField('output_price', ($event.target as HTMLInputElement).value)"
          type="number" step="any" min="0" class="input mt-0.5 text-xs" />
      </div>
      <div class="flex-1">
        <label class="text-xs text-gray-400">{{ t('admin.channels.form.cacheWritePrice', '缓存W') }} <span class="text-gray-300">$/M</span></label>
        <input :value="interval.cache_write_price" @input="emitField('cache_write_price', ($event.target as HTMLInputElement).value)"
          type="number" step="any" min="0" class="input mt-0.5 text-xs" />
      </div>
      <div class="flex-1">
        <label class="text-xs text-gray-400">{{ t('admin.channels.form.cacheReadPrice', '缓存R') }} <span class="text-gray-300">$/M</span></label>
        <input :value="interval.cache_read_price" @input="emitField('cache_read_price', ($event.target as HTMLInputElement).value)"
          type="number" step="any" min="0" class="input mt-0.5 text-xs" />
      </div>
      <div class="flex-1">
        <label class="text-xs text-gray-400">{{ t('admin.channels.form.imageTokenPrice', '图片输出') }} <span class="text-gray-300">$/M</span></label>
        <input :value="interval.image_output_price" @input="emitField('image_output_price', ($event.target as HTMLInputElement).value)"
          type="number" step="any" min="0" class="input mt-0.5 text-xs" />
      </div>
    </template>

    <!-- Per-request / Image mode: tier label + context range + price -->
    <template v-else>
      <div class="w-24">
        <label class="text-xs text-gray-400">
          {{ mode === 'image' ? t('admin.channels.form.resolution', '分辨率') : t('admin.channels.form.tierLabel', '层级') }}
        </label>
        <input :value="interval.tier_label" @input="emitField('tier_label', ($event.target as HTMLInputElement).value)"
          type="text" class="input mt-0.5 text-xs" :placeholder="mode === 'image' ? '1K / 2K / 4K' : ''" />
      </div>
      <div class="w-20">
        <label class="text-xs text-gray-400">{{ t('admin.channels.form.intervalMin') }}</label>
        <input :value="interval.min_tokens" @input="emitField('min_tokens', toInt(($event.target as HTMLInputElement).value))"
          type="number" min="0" class="input mt-0.5 text-xs" />
      </div>
      <div class="w-20">
        <label class="text-xs text-gray-400">{{ t('admin.channels.form.intervalMax') }} <span class="text-gray-300">{{ t('admin.channels.form.intervalInclusive') }}</span></label>
        <input :value="interval.max_tokens ?? ''" @input="emitField('max_tokens', toIntOrNull(($event.target as HTMLInputElement).value))"
          type="number" min="0" class="input mt-0.5 text-xs" :placeholder="'∞'" />
      </div>
      <div class="flex-1">
        <label class="text-xs text-gray-400">{{ t('admin.channels.form.perRequestPrice', '单次价格') }} <span v-if="isEmpty" class="input-required">*</span> <span class="text-gray-300">$</span></label>
        <input :value="interval.per_request_price" @input="emitField('per_request_price', ($event.target as HTMLInputElement).value)"
          type="number" step="any" min="0" class="input mt-0.5 text-xs" />
      </div>
      <div v-if="mode === 'image'" class="flex-1">
        <label class="text-xs text-gray-400">{{ t('admin.channels.form.imageTokenPrice', '图片输出') }} <span class="text-gray-300">$/M</span></label>
        <input :value="interval.image_output_price" @input="emitField('image_output_price', ($event.target as HTMLInputElement).value)"
          type="number" step="any" min="0" class="input mt-0.5 text-xs" />
      </div>
    </template>

    <button type="button" @click="emit('remove')" class="btn-icon-danger mt-4 p-0.5">
      <Icon name="x" size="sm" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon } from '@sub2api/plugin-sdk'
import type { IntervalFormEntry } from './types'
import type { BillingMode } from '../api/channels'

const { t } = useI18n()

const props = defineProps<{
  interval: IntervalFormEntry
  mode: BillingMode
}>()

const emit = defineEmits<{
  update: [interval: IntervalFormEntry]
  remove: []
}>()

// 检测所有价格字段是否都为空（与后端 checkIntervalsHavePrices 保持一致：
// 任一价格字段非空即非空；image_output_price 与其它字段并列）
const isEmpty = computed(() => {
  const iv = props.interval
  return (iv.input_price == null || iv.input_price === '') &&
    (iv.output_price == null || iv.output_price === '') &&
    (iv.cache_write_price == null || iv.cache_write_price === '') &&
    (iv.cache_read_price == null || iv.cache_read_price === '') &&
    (iv.image_output_price == null || iv.image_output_price === '') &&
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
