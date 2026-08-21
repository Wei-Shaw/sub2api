<template>
  <div class="flex items-center gap-1">
    <button
      type="button"
      class="btn btn-secondary btn-xs"
      :title="t('admin.channels.form.pricingClipboard.copy')"
      @click="copyPricing"
    >
      <Icon name="copy" size="xs" />
      <span class="hidden sm:inline">{{ t('admin.channels.form.pricingClipboard.copy') }}</span>
    </button>
    <button
      type="button"
      class="btn btn-secondary btn-xs"
      :title="t('admin.channels.form.pricingClipboard.paste')"
      @click="openImport"
    >
      <Icon name="clipboard" size="xs" />
      <span class="hidden sm:inline">{{ t('admin.channels.form.pricingClipboard.paste') }}</span>
    </button>
  </div>

  <BaseDialog
    :show="showImport"
    :title="t('admin.channels.form.pricingClipboard.importTitle')"
    width="wide"
    @close="showImport = false"
  >
    <p class="mb-2 text-xs text-gray-500 dark:text-gray-400">
      {{ t('admin.channels.form.pricingClipboard.importHint') }}
    </p>
    <textarea
      v-model="importText"
      rows="14"
      class="input w-full resize-y font-mono text-xs"
      :placeholder="t('admin.channels.form.pricingClipboard.placeholder')"
      spellcheck="false"
    />
    <template #footer>
      <div class="flex justify-end gap-2">
        <button type="button" class="btn btn-secondary" @click="showImport = false">
          {{ t('common.cancel', 'Cancel') }}
        </button>
        <button type="button" class="btn btn-primary" @click="applyImport">
          <Icon name="check" size="sm" class="mr-1" />
          {{ t('admin.channels.form.pricingClipboard.apply') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { useClipboard } from '@/composables/useClipboard'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { createDefaultTimePricingForm, type PricingFormEntry } from './types'

const props = defineProps<{ modelValue: PricingFormEntry[] }>()
const emit = defineEmits<{ 'update:modelValue': [value: PricingFormEntry[]] }>()
const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()
const showImport = ref(false)
const importText = ref('')

async function copyPricing() {
  await copyToClipboard(
    JSON.stringify(props.modelValue, null, 2),
    t('admin.channels.form.pricingClipboard.copied'),
  )
}

async function openImport() {
  importText.value = ''
  try {
    if (navigator.clipboard?.readText) importText.value = await navigator.clipboard.readText()
  } catch {
    // Clipboard read permission is optional; the dialog still supports manual Ctrl+V.
  }
  showImport.value = true
  await nextTick()
}

function parsePricing(text: string): PricingFormEntry[] | null {
  let parsed: unknown
  try {
    parsed = JSON.parse(text)
  } catch {
    return null
  }
  if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
    const wrapper = parsed as { model_pricing?: unknown; pricing?: unknown }
    parsed = wrapper.model_pricing ?? wrapper.pricing
  }
  if (!Array.isArray(parsed)) return null
  const validModes = new Set(['token', 'per_request', 'image', 'video'])
  if (!parsed.every((entry) => {
    if (!entry || typeof entry !== 'object') return false
    const value = entry as Partial<PricingFormEntry>
    return Array.isArray(value.models) && value.models.every((model) => typeof model === 'string') &&
      typeof value.billing_mode === 'string' && validModes.has(value.billing_mode) &&
      Array.isArray(value.intervals)
  })) return null
  return (structuredClone(parsed) as Array<Partial<PricingFormEntry>>).map((entry) => ({
    models: entry.models || [],
    billing_mode: entry.billing_mode || 'token',
    input_price: entry.input_price ?? null,
    output_price: entry.output_price ?? null,
    cache_write_price: entry.cache_write_price ?? null,
    cache_read_price: entry.cache_read_price ?? null,
    image_input_price: entry.image_input_price ?? null,
    image_output_price: entry.image_output_price ?? null,
    per_request_price: entry.per_request_price ?? null,
    intervals: entry.intervals || [],
    time_pricing: entry.time_pricing || createDefaultTimePricingForm(),
  })) as PricingFormEntry[]
}

function applyImport() {
  const pricing = parsePricing(importText.value.trim())
  if (!pricing) {
    appStore.showError(t('admin.channels.form.pricingClipboard.invalid'))
    return
  }
  emit('update:modelValue', pricing)
  showImport.value = false
  appStore.showSuccess(t('admin.channels.form.pricingClipboard.imported', { count: pricing.length }))
}
</script>
