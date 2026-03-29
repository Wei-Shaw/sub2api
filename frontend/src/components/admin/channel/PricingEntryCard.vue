<template>
  <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-800">
    <!-- Header: Models + Billing Mode + Remove -->
    <div class="mb-2 flex items-start gap-2">
      <div class="flex-1">
        <label class="text-xs font-medium text-gray-500 dark:text-gray-400">
          {{ t('admin.channels.form.models', 'Models (comma separated, supports *)') }}
        </label>
        <textarea
          :value="entry.modelsInput"
          @input="emit('update', { ...entry, modelsInput: ($event.target as HTMLTextAreaElement).value })"
          rows="2"
          class="input mt-1 text-sm"
          :placeholder="t('admin.channels.form.modelsPlaceholder', 'claude-sonnet-4-20250514, claude-opus-4-20250514, *')"
        ></textarea>
      </div>
      <div class="w-40">
        <label class="text-xs font-medium text-gray-500 dark:text-gray-400">
          {{ t('admin.channels.form.billingMode', 'Billing Mode') }}
        </label>
        <Select
          :modelValue="entry.billing_mode"
          @update:modelValue="emit('update', { ...entry, billing_mode: $event as BillingMode, intervals: [] })"
          :options="billingModeOptions"
          class="mt-1"
        />
      </div>
      <button
        type="button"
        @click="emit('remove')"
        class="mt-5 rounded p-1 text-gray-400 hover:text-red-500"
      >
        <Icon name="trash" size="sm" />
      </button>
    </div>

    <!-- Token mode: flat prices + intervals -->
    <div v-if="entry.billing_mode === 'token'">
      <!-- Flat prices (used when no intervals) -->
      <div class="grid grid-cols-2 gap-2 sm:grid-cols-4">
        <div>
          <label class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.channels.form.inputPrice', 'Input Price') }}
          </label>
          <input
            :value="entry.input_price"
            @input="emitField('input_price', ($event.target as HTMLInputElement).value)"
            type="number"
            step="any" min="0"
            class="input mt-1 text-sm"
            :placeholder="t('admin.channels.form.pricePlaceholder', 'Default')"
          />
        </div>
        <div>
          <label class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.channels.form.outputPrice', 'Output Price') }}
          </label>
          <input
            :value="entry.output_price"
            @input="emitField('output_price', ($event.target as HTMLInputElement).value)"
            type="number"
            step="any" min="0"
            class="input mt-1 text-sm"
            :placeholder="t('admin.channels.form.pricePlaceholder', 'Default')"
          />
        </div>
        <div>
          <label class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.channels.form.cacheWritePrice', 'Cache Write') }}
          </label>
          <input
            :value="entry.cache_write_price"
            @input="emitField('cache_write_price', ($event.target as HTMLInputElement).value)"
            type="number"
            step="any" min="0"
            class="input mt-1 text-sm"
            :placeholder="t('admin.channels.form.pricePlaceholder', 'Default')"
          />
        </div>
        <div>
          <label class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.channels.form.cacheReadPrice', 'Cache Read') }}
          </label>
          <input
            :value="entry.cache_read_price"
            @input="emitField('cache_read_price', ($event.target as HTMLInputElement).value)"
            type="number"
            step="any" min="0"
            class="input mt-1 text-sm"
            :placeholder="t('admin.channels.form.pricePlaceholder', 'Default')"
          />
        </div>
      </div>

      <!-- Token intervals -->
      <div class="mt-3">
        <div class="flex items-center justify-between">
          <label class="text-xs font-medium text-gray-500 dark:text-gray-400">
            {{ t('admin.channels.form.intervals', 'Context Intervals (optional)') }}
          </label>
          <button type="button" @click="addInterval" class="text-xs text-primary-600 hover:text-primary-700">
            + {{ t('admin.channels.form.addInterval', 'Add Interval') }}
          </button>
        </div>
        <div v-if="entry.intervals && entry.intervals.length > 0" class="mt-2 space-y-2">
          <IntervalRow
            v-for="(iv, idx) in entry.intervals"
            :key="idx"
            :interval="iv"
            :mode="entry.billing_mode"
            @update="updateInterval(idx, $event)"
            @remove="removeInterval(idx)"
          />
        </div>
      </div>
    </div>

    <!-- Per-request mode: tiers -->
    <div v-else-if="entry.billing_mode === 'per_request'">
      <div class="flex items-center justify-between">
        <label class="text-xs font-medium text-gray-500 dark:text-gray-400">
          {{ t('admin.channels.form.requestTiers', 'Request Tiers') }}
        </label>
        <button type="button" @click="addInterval" class="text-xs text-primary-600 hover:text-primary-700">
          + {{ t('admin.channels.form.addTier', 'Add Tier') }}
        </button>
      </div>
      <div v-if="entry.intervals && entry.intervals.length > 0" class="mt-2 space-y-2">
        <IntervalRow
          v-for="(iv, idx) in entry.intervals"
          :key="idx"
          :interval="iv"
          :mode="entry.billing_mode"
          @update="updateInterval(idx, $event)"
          @remove="removeInterval(idx)"
        />
      </div>
      <div v-else class="mt-2 rounded border border-dashed border-gray-300 p-3 text-center text-xs text-gray-400 dark:border-dark-500">
        {{ t('admin.channels.form.noTiersYet', 'No tiers. Add one to configure per-request pricing.') }}
      </div>
    </div>

    <!-- Image mode: tiers -->
    <div v-else-if="entry.billing_mode === 'image'">
      <div class="flex items-center justify-between">
        <label class="text-xs font-medium text-gray-500 dark:text-gray-400">
          {{ t('admin.channels.form.imageTiers', 'Image Tiers') }}
        </label>
        <button type="button" @click="addImageTier" class="text-xs text-primary-600 hover:text-primary-700">
          + {{ t('admin.channels.form.addTier', 'Add Tier') }}
        </button>
      </div>
      <div v-if="entry.intervals && entry.intervals.length > 0" class="mt-2 space-y-2">
        <IntervalRow
          v-for="(iv, idx) in entry.intervals"
          :key="idx"
          :interval="iv"
          :mode="entry.billing_mode"
          @update="updateInterval(idx, $event)"
          @remove="removeInterval(idx)"
        />
      </div>
      <div v-else>
        <!-- Legacy image_output_price fallback -->
        <div class="mt-2 grid grid-cols-2 gap-2 sm:grid-cols-4">
          <div>
            <label class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.channels.form.imageOutputPrice', 'Image Output Price') }}
            </label>
            <input
              :value="entry.image_output_price"
              @input="emitField('image_output_price', ($event.target as HTMLInputElement).value)"
              type="number"
              step="any" min="0"
              class="input mt-1 text-sm"
              :placeholder="t('admin.channels.form.pricePlaceholder', 'Default')"
            />
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import IntervalRow from './IntervalRow.vue'
import type { PricingFormEntry, IntervalFormEntry } from './types'
import type { BillingMode } from '@/api/admin/channels'

const { t } = useI18n()

const props = defineProps<{
  entry: PricingFormEntry
}>()

const emit = defineEmits<{
  update: [entry: PricingFormEntry]
  remove: []
}>()

const billingModeOptions = computed(() => [
  { value: 'token', label: t('admin.channels.billingMode.token', 'Token') },
  { value: 'per_request', label: t('admin.channels.billingMode.perRequest', 'Per Request') },
  { value: 'image', label: t('admin.channels.billingMode.image', 'Image') }
])

function emitField(field: keyof PricingFormEntry, value: string) {
  emit('update', { ...props.entry, [field]: value === '' ? null : value })
}

function addInterval() {
  const intervals = [...(props.entry.intervals || [])]
  intervals.push({
    min_tokens: 0,
    max_tokens: null,
    tier_label: '',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    per_request_price: null,
    sort_order: intervals.length
  })
  emit('update', { ...props.entry, intervals })
}

function addImageTier() {
  const intervals = [...(props.entry.intervals || [])]
  const labels = ['1K', '2K', '4K', 'HD']
  const nextLabel = labels[intervals.length] || ''
  intervals.push({
    min_tokens: 0,
    max_tokens: null,
    tier_label: nextLabel,
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    per_request_price: null,
    sort_order: intervals.length
  })
  emit('update', { ...props.entry, intervals })
}

function updateInterval(idx: number, updated: IntervalFormEntry) {
  const intervals = [...(props.entry.intervals || [])]
  intervals[idx] = updated
  emit('update', { ...props.entry, intervals })
}

function removeInterval(idx: number) {
  const intervals = [...(props.entry.intervals || [])]
  intervals.splice(idx, 1)
  emit('update', { ...props.entry, intervals })
}
</script>
