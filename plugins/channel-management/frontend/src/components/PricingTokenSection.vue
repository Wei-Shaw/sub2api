<template>
  <!-- Default prices (fallback when no interval matches) -->
  <label class="mt-3 block text-xs font-medium text-gray-500 dark:text-gray-400">
    {{ t('admin.channels.form.defaultPrices', '默认价格（未命中区间时使用）') }}
    <span class="ml-1 font-normal text-gray-400">$/MTok</span>
  </label>
  <div class="mt-1 grid grid-cols-2 gap-2 sm:grid-cols-5">
    <div>
      <label class="text-xs text-gray-400">{{ t('admin.channels.form.inputPrice', '输入') }}</label>
      <input :value="entry.input_price" @input="emitField('input_price', ($event.target as HTMLInputElement).value)"
        type="number" step="any" min="0" class="input mt-0.5 text-sm" :placeholder="t('admin.channels.form.pricePlaceholder', '默认')" />
    </div>
    <div>
      <label class="text-xs text-gray-400">{{ t('admin.channels.form.outputPrice', '输出') }}</label>
      <input :value="entry.output_price" @input="emitField('output_price', ($event.target as HTMLInputElement).value)"
        type="number" step="any" min="0" class="input mt-0.5 text-sm" :placeholder="t('admin.channels.form.pricePlaceholder', '默认')" />
    </div>
    <div>
      <label class="text-xs text-gray-400">{{ t('admin.channels.form.cacheWritePrice', '缓存写入') }}</label>
      <input :value="entry.cache_write_price" @input="emitField('cache_write_price', ($event.target as HTMLInputElement).value)"
        type="number" step="any" min="0" class="input mt-0.5 text-sm" :placeholder="t('admin.channels.form.pricePlaceholder', '默认')" />
    </div>
    <div>
      <label class="text-xs text-gray-400">{{ t('admin.channels.form.cacheReadPrice', '缓存读取') }}</label>
      <input :value="entry.cache_read_price" @input="emitField('cache_read_price', ($event.target as HTMLInputElement).value)"
        type="number" step="any" min="0" class="input mt-0.5 text-sm" :placeholder="t('admin.channels.form.pricePlaceholder', '默认')" />
    </div>
    <div>
      <label class="text-xs text-gray-400">{{ t('admin.channels.form.imageTokenPrice', '图片输出') }}</label>
      <input :value="entry.image_output_price" @input="emitField('image_output_price', ($event.target as HTMLInputElement).value)"
        type="number" step="any" min="0" class="input mt-0.5 text-sm" :placeholder="t('admin.channels.form.pricePlaceholder', '默认')" />
    </div>
  </div>

  <!-- Token intervals -->
  <div class="mt-3">
    <div class="flex items-center justify-between">
      <label class="text-xs font-medium text-gray-500 dark:text-gray-400">
        {{ t('admin.channels.form.intervals', '上下文区间定价（可选）') }}
        <span class="ml-1 font-normal text-gray-400">(min, max]</span>
      </label>
      <button type="button" @click="emit('addInterval')" class="text-xs text-primary-600 hover:text-primary-700">
        + {{ t('admin.channels.form.addInterval', '添加区间') }}
      </button>
    </div>
    <div v-if="entry.intervals && entry.intervals.length > 0" class="mt-2 space-y-2">
      <IntervalRow
        v-for="(iv, idx) in entry.intervals"
        :key="idx"
        :interval="iv"
        :mode="entry.billing_mode"
        @update="emit('updateInterval', idx, $event)"
        @remove="emit('removeInterval', idx)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import IntervalRow from './IntervalRow.vue'
import type { PricingFormEntry, IntervalFormEntry } from './types'

const { t } = useI18n()

defineProps<{
  entry: PricingFormEntry
}>()

const emit = defineEmits<{
  field: [field: string, value: string]
  addInterval: []
  updateInterval: [idx: number, updated: IntervalFormEntry]
  removeInterval: [idx: number]
}>()

function emitField(field: string, value: string) {
  emit('field', field, value)
}
</script>
