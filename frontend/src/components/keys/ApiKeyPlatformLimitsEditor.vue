<template>
  <div class="space-y-3">
    <p class="input-hint">{{ t('keys.platformLimitsHint') }}</p>

    <p v-if="rows.length === 0" class="text-sm text-gray-500 dark:text-gray-400" data-test="platform-limits-empty">
      {{ t('keys.platformLimitsEmpty') }}
    </p>

    <div
      v-for="(row, index) in rows"
      :key="row.platform"
      class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600"
      data-test="platform-limit-row"
    >
      <div class="flex items-center gap-2">
        <div class="flex-1">
          <label class="input-label">{{ t('keys.platformLimitsPlatform') }}</label>
          <Select
            :model-value="row.platform"
            :options="optionsFor(row.platform)"
            :aria-label="t('keys.platformLimitsPlatform')"
            :data-test="`platform-limit-select-${index}`"
            @update:model-value="(value) => changePlatform(row.platform, String(value))"
          />
        </div>
        <button
          type="button"
          class="mt-6 rounded-lg px-2 py-1.5 text-sm text-red-500 transition-colors hover:bg-red-50 dark:hover:bg-red-900/20"
          :title="t('keys.platformLimitsRemove')"
          :aria-label="t('keys.platformLimitsRemove')"
          :data-test="`platform-limit-remove-${index}`"
          @click="removePlatform(row.platform)"
        >
          {{ t('common.delete') }}
        </button>
      </div>

      <div class="grid grid-cols-2 gap-3">
        <div v-for="field in LIMIT_FIELDS" :key="field.key">
          <label class="input-label">{{ t(field.label) }}</label>
          <div class="relative">
            <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500">$</span>
            <input
              :value="row.limit[field.key] ?? null"
              type="number"
              step="0.01"
              min="0"
              class="input pl-7"
              placeholder="0"
              :data-test="`platform-limit-${field.key}-${index}`"
              @input="updateField(row.platform, field.key, ($event.target as HTMLInputElement).value)"
            />
          </div>
        </div>
      </div>

      <!-- Usage (edit mode only, one line per configured window) -->
      <div v-if="usageFor(row)" class="space-y-1.5" data-test="platform-limit-usage">
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('keys.platformLimitsUsage') }}</p>
        <div v-for="bar in usageBars(row)" :key="bar.key" class="space-y-1">
          <div class="flex items-center justify-between text-xs">
            <span class="text-gray-500 dark:text-gray-400">{{ t(bar.label) }}</span>
            <span :class="usageTextClass(bar.used, bar.limit)">
              ${{ bar.used.toFixed(4) }} / ${{ bar.limit.toFixed(2) }}
            </span>
          </div>
          <div class="h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
            <div
              :class="['h-full rounded-full transition-all', usageBarClass(bar.used, bar.limit)]"
              :style="{ width: Math.min((bar.used / bar.limit) * 100, 100) + '%' }"
            />
          </div>
        </div>
      </div>
    </div>

    <div class="flex items-center gap-3">
      <button
        type="button"
        class="btn btn-secondary text-sm"
        :disabled="availablePlatforms.length === 0"
        data-test="platform-limit-add"
        @click="addPlatform"
      >
        {{ t('keys.platformLimitsAdd') }}
      </button>
      <span v-if="availablePlatforms.length === 0" class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('keys.platformLimitsAllUsed') }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import { CONCRETE_PLATFORM_OPTIONS } from '@/constants/platforms'
import type { ApiKeyPlatformLimit, ApiKeyPlatformLimits, ApiKeyPlatformUsage } from '@/types'

type LimitField = keyof ApiKeyPlatformLimit

const LIMIT_FIELDS: Array<{ key: LimitField; label: string }> = [
  { key: 'quota', label: 'keys.platformLimitsQuota' },
  { key: 'rate_limit_5h', label: 'keys.rateLimit5h' },
  { key: 'rate_limit_1d', label: 'keys.rateLimit1d' },
  { key: 'rate_limit_7d', label: 'keys.rateLimit7d' }
]

const props = defineProps<{
  modelValue: ApiKeyPlatformLimits
  /** Per-source usage from the key detail endpoint; absent in create mode. */
  usages?: ApiKeyPlatformUsage[]
}>()
const emit = defineEmits<{ 'update:modelValue': [ApiKeyPlatformLimits] }>()

const { t } = useI18n()

// Keep a stable order so editing one row does not reshuffle the list.
const rows = computed(() =>
  CONCRETE_PLATFORM_OPTIONS.filter((option) => props.modelValue[option.value] !== undefined).map((option) => ({
    platform: option.value as string,
    limit: props.modelValue[option.value] ?? {}
  }))
)

const availablePlatforms = computed(() =>
  CONCRETE_PLATFORM_OPTIONS.filter((option) => props.modelValue[option.value] === undefined)
)

/** The row's own platform stays selectable so the select shows its current value. */
function optionsFor(platform: string) {
  return CONCRETE_PLATFORM_OPTIONS.filter(
    (option) => option.value === platform || props.modelValue[option.value] === undefined
  ).map((option) => ({ value: option.value as string, label: option.label }))
}

function commit(next: ApiKeyPlatformLimits) {
  emit('update:modelValue', next)
}

function addPlatform() {
  const next = availablePlatforms.value[0]
  if (!next) return
  commit({ ...props.modelValue, [next.value]: {} })
}

function removePlatform(platform: string) {
  const next = { ...props.modelValue }
  delete next[platform]
  commit(next)
}

function changePlatform(from: string, to: string) {
  if (!to || to === from || props.modelValue[to] !== undefined) return
  const next: ApiKeyPlatformLimits = {}
  // Rebuild in place so the row keeps its position.
  for (const [platform, limit] of Object.entries(props.modelValue)) {
    next[platform === from ? to : platform] = limit
  }
  commit(next)
}

function updateField(platform: string, field: LimitField, raw: string) {
  const limit = { ...(props.modelValue[platform] ?? {}) }
  const value = Number(raw)
  if (raw.trim() === '' || !Number.isFinite(value) || value <= 0) {
    delete limit[field]
  } else {
    limit[field] = value
  }
  commit({ ...props.modelValue, [platform]: limit })
}

function usageFor(row: { platform: string }) {
  return props.usages?.find((usage) => usage.platform === row.platform)
}

/** Only windows with a configured limit get a bar — a bar without a limit has no denominator. */
function usageBars(row: { platform: string; limit: ApiKeyPlatformLimit }) {
  const usage = usageFor(row)
  if (!usage) return []
  const pairs: Array<{ key: LimitField; label: string; used: number }> = [
    { key: 'quota', label: 'keys.platformLimitsQuota', used: usage.quota_used },
    { key: 'rate_limit_5h', label: 'keys.rateLimit5h', used: usage.usage_5h },
    { key: 'rate_limit_1d', label: 'keys.rateLimit1d', used: usage.usage_1d },
    { key: 'rate_limit_7d', label: 'keys.rateLimit7d', used: usage.usage_7d }
  ]
  return pairs
    .map((pair) => ({ ...pair, limit: row.limit[pair.key] ?? 0 }))
    .filter((pair) => pair.limit > 0)
}

function usageTextClass(used: number, limit: number) {
  if (used >= limit) return 'font-medium text-red-500'
  if (used >= limit * 0.8) return 'font-medium text-yellow-500'
  return 'font-medium text-gray-900 dark:text-white'
}

function usageBarClass(used: number, limit: number) {
  if (used >= limit) return 'bg-red-500'
  if (used >= limit * 0.8) return 'bg-yellow-500'
  return 'bg-green-500'
}
</script>
