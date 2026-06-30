<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import {
  IMAGE_PRICING_TIER_KEYS,
  IMAGE_PRICING_QUALITY_KEYS,
  cloneDefaultImagePricingMatrix,
  createEmptyImagePricingMatrix,
  validateEditableMatrix,
  type EditableImagePricingMatrix,
} from '@/constants/imagePricingMatrix'

interface Props {
  modelValue: EditableImagePricingMatrix
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:modelValue', value: EditableImagePricingMatrix): void
}>()

const { t } = useI18n()

const tiers = IMAGE_PRICING_TIER_KEYS
const qualities = IMAGE_PRICING_QUALITY_KEYS

const invalidCells = computed(() => new Set(validateEditableMatrix(props.modelValue)))

function cellId(tier: string, quality: string): string {
  return `${tier}/${quality}`
}

function isInvalid(tier: string, quality: string): boolean {
  return invalidCells.value.has(cellId(tier, quality))
}

function updateCell(tier: string, quality: string, raw: string) {
  const next: EditableImagePricingMatrix = {}
  for (const t2 of tiers) {
    next[t2] = { ...(props.modelValue[t2] || { low: null, medium: null, high: null }) }
  }
  if (!next[tier]) {
    next[tier] = { low: null, medium: null, high: null }
  }
  if (raw === '' || raw === null) {
    next[tier][quality] = null
  } else {
    const n = Number(raw)
    next[tier][quality] = Number.isFinite(n) ? n : (NaN as unknown as number)
  }
  emit('update:modelValue', next)
}

function fillDefaults() {
  const def = cloneDefaultImagePricingMatrix()
  const next: EditableImagePricingMatrix = {}
  for (const tier of tiers) {
    next[tier] = {
      low: def[tier]?.low ?? null,
      medium: def[tier]?.medium ?? null,
      high: def[tier]?.high ?? null,
    }
  }
  emit('update:modelValue', next)
}

function clearAll() {
  emit('update:modelValue', createEmptyImagePricingMatrix())
}

function clearRow(tier: string) {
  const next: EditableImagePricingMatrix = {}
  for (const t2 of tiers) {
    next[t2] = { ...(props.modelValue[t2] || { low: null, medium: null, high: null }) }
  }
  next[tier] = { low: null, medium: null, high: null }
  emit('update:modelValue', next)
}
</script>

<template>
  <div class="space-y-3">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <p class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.groups.imagePricing.matrixHint') }}
      </p>
      <div class="flex items-center gap-2">
        <button
          type="button"
          class="rounded-md border border-blue-500 bg-blue-50 px-3 py-1 text-xs font-medium text-blue-700 hover:bg-blue-100 dark:border-blue-400 dark:bg-blue-950 dark:text-blue-300 dark:hover:bg-blue-900"
          @click="fillDefaults"
        >
          {{ t('admin.groups.imagePricing.fillDefaults') }}
        </button>
        <button
          type="button"
          class="rounded-md border border-gray-300 bg-white px-3 py-1 text-xs font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700"
          @click="clearAll"
        >
          {{ t('admin.groups.imagePricing.clearAll') }}
        </button>
      </div>
    </div>

    <div class="overflow-x-auto rounded-md border border-gray-200 dark:border-gray-700">
      <table class="w-full min-w-[480px] text-sm">
        <thead class="bg-gray-50 dark:bg-gray-800">
          <tr>
            <th class="px-3 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
              {{ t('admin.groups.imagePricing.matrixSizeHeader') }}
            </th>
            <th
              v-for="quality in qualities"
              :key="quality"
              class="px-3 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400"
            >
              {{ t(`admin.groups.imagePricing.quality.${quality}`) }}
              <span class="ml-1 normal-case text-[10px] text-gray-400">($)</span>
            </th>
            <th class="w-12 px-2 py-2"></th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 dark:divide-gray-700">
          <tr
            v-for="tier in tiers"
            :key="tier"
            class="bg-white dark:bg-gray-900"
          >
            <td class="whitespace-nowrap px-3 py-2 font-mono text-xs text-gray-700 dark:text-gray-200">
              {{ tier }}
            </td>
            <td
              v-for="quality in qualities"
              :key="quality"
              class="px-2 py-1"
            >
              <input
                type="number"
                step="0.001"
                min="0"
                :value="modelValue[tier]?.[quality] ?? ''"
                :class="[
                  'input w-full text-sm',
                  isInvalid(tier, quality)
                    ? 'border-red-500 focus:border-red-500 focus:ring-red-500'
                    : '',
                ]"
                placeholder="—"
                @input="(e) => updateCell(tier, quality, (e.target as HTMLInputElement).value)"
              />
            </td>
            <td class="px-2 py-1 text-right">
              <button
                type="button"
                class="text-xs text-gray-400 hover:text-red-500"
                :title="t('admin.groups.imagePricing.clearRow')"
                @click="clearRow(tier)"
              >
                ✕
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <p
      v-if="invalidCells.size > 0"
      class="text-xs text-red-500"
    >
      {{ t('admin.groups.imagePricing.invalidCells', { count: invalidCells.size }) }}
    </p>
  </div>
</template>
