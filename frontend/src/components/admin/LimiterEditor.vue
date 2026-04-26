<template>
  <div class="space-y-3">
    <div v-if="modelValue.length === 0" class="rounded-lg border border-dashed border-gray-300 px-4 py-3 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
      {{ t('admin.serviceQuota.limiterEditor.empty') }}
    </div>
    <div
      v-for="(item, index) in modelValue"
      :key="item.uid ?? index"
      class="grid grid-cols-1 items-end gap-3 rounded-lg border border-gray-200 p-3 sm:grid-cols-[160px_1fr_140px_auto] dark:border-dark-700"
    >
      <label class="form-field">
        <span class="input-label">{{ t('admin.serviceQuota.columns.type') }}</span>
        <select
          :value="item.limiter_type"
          class="input"
          @change="updateField(index, 'limiter_type', ($event.target as HTMLSelectElement).value)"
        >
          <option v-for="opt in availableTypes(item.limiter_type)" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
        </select>
      </label>
      <label class="form-field">
        <span class="input-label">{{ t('admin.serviceQuota.columns.limit') }}</span>
        <input
          :value="item.limit_value"
          type="number"
          min="1"
          step="0.000001"
          class="input"
          required
          @input="updateField(index, 'limit_value', Number(($event.target as HTMLInputElement).value))"
        />
      </label>
      <label v-if="item.limiter_type !== 'concurrency'" class="form-field">
        <span class="input-label">{{ t('admin.serviceQuota.columns.window') }}</span>
        <select
          :value="item.window_mode"
          class="input"
          @change="updateField(index, 'window_mode', ($event.target as HTMLSelectElement).value)"
        >
          <option value="fixed">{{ t('admin.serviceQuota.windows.fixed') }}</option>
          <option value="rolling">{{ t('admin.serviceQuota.windows.rolling') }}</option>
        </select>
      </label>
      <div v-else class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.serviceQuota.windows.none') }}</div>
      <button
        type="button"
        class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20"
        :title="t('common.delete')"
        @click="remove(index)"
      >
        <Icon name="trash" size="sm" />
      </button>
    </div>
    <button
      type="button"
      class="btn btn-secondary"
      :disabled="!canAdd"
      @click="add"
    >
      <Icon name="plus" size="sm" class="mr-2" />
      {{ t('admin.serviceQuota.limiterEditor.add') }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { ServiceQuotaLimiterInput } from '@/api/admin/serviceQuota'

const { t } = useI18n()

const props = defineProps<{
  modelValue: ServiceQuotaLimiterInput[]
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: ServiceQuotaLimiterInput[]): void
}>()

const allTypes = computed(() => [
  { value: 'rpm', label: t('admin.serviceQuota.limiters.rpm') },
  { value: 'tpm', label: t('admin.serviceQuota.limiters.tpm') },
  { value: 'tpd', label: t('admin.serviceQuota.limiters.tpd') },
  { value: 'daily_usd', label: t('admin.serviceQuota.limiters.dailyUsd') },
  { value: 'concurrency', label: t('admin.serviceQuota.limiters.concurrency') },
])

const usedTypes = computed(() => new Set(props.modelValue.map((l) => l.limiter_type)))

const canAdd = computed(() => usedTypes.value.size < allTypes.value.length)

function availableTypes(currentType: string) {
  return allTypes.value.filter((opt) => opt.value === currentType || !usedTypes.value.has(opt.value))
}

function defaultLimitFor(type: string): number {
  switch (type) {
    case 'rpm': return 60
    case 'tpm': return 100000
    case 'tpd': return 1000000
    case 'daily_usd': return 50
    case 'concurrency': return 5
    default: return 1
  }
}

function pickFirstAvailableType(): string {
  const used = usedTypes.value
  const next = allTypes.value.find((opt) => !used.has(opt.value))
  return next?.value || 'rpm'
}

function blankLimiter(limiter_type: string): ServiceQuotaLimiterInput {
  return {
    uid: crypto.randomUUID(),
    limiter_type,
    window_mode: 'fixed',
    limit_value: defaultLimitFor(limiter_type),
  }
}

function add() {
  const limiter_type = pickFirstAvailableType()
  const newList: ServiceQuotaLimiterInput[] = [
    ...props.modelValue,
    blankLimiter(limiter_type),
  ]
  emit('update:modelValue', newList)
}

function remove(index: number) {
  const newList = props.modelValue.slice()
  newList.splice(index, 1)
  emit('update:modelValue', newList)
}

function updateField<K extends keyof ServiceQuotaLimiterInput>(index: number, key: K, value: ServiceQuotaLimiterInput[K]) {
  const newList = props.modelValue.slice()
  const next: ServiceQuotaLimiterInput = { ...newList[index], [key]: value }
  if (key === 'limiter_type' && value === 'concurrency') {
    next.window_mode = 'fixed'
  }
  newList[index] = next
  emit('update:modelValue', newList)
}
</script>

<style scoped>
.form-field {
  @apply space-y-1.5;
}
</style>