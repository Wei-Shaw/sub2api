<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import QuotaNotifyToggle from './QuotaNotifyToggle.vue'

const { t } = useI18n()

const props = defineProps<{
  dim: 'daily' | 'weekly' | 'total'
  label: string
  limit: number | null
  quotaNotifyGlobalEnabled: boolean
  notifyEnabled: boolean | null
  notifyThreshold: number | null
  notifyThresholdType: string | null
  // Reset mode (only for daily/weekly, null for total)
  resetMode: 'rolling' | 'fixed' | null
  resetHour: number | null
  resetDay: number | null  // weekly only
  resetTimezone: string | null
  hintRolling: string
  hintFixed: string
  // Shared options passed from parent
  hourOptions: number[]
  dayOptions: { value: number; key: string }[]
}>()

const emit = defineEmits<{
  'update:limit': [value: number | null]
  'update:notifyEnabled': [value: boolean | null]
  'update:notifyThreshold': [value: number | null]
  'update:notifyThresholdType': [value: string | null]
  'update:resetMode': [value: 'rolling' | 'fixed' | null]
  'update:resetHour': [value: number | null]
  'update:resetDay': [value: number | null]
  'update:resetTimezone': [value: string | null]
}>()

const hasResetMode = props.dim !== 'total'

const onLimitInput = (e: Event) => {
  const raw = (e.target as HTMLInputElement).valueAsNumber
  emit('update:limit', Number.isNaN(raw) ? null : raw)
}

const onModeChange = (e: Event) => {
  const val = (e.target as HTMLSelectElement).value as 'rolling' | 'fixed'
  emit('update:resetMode', val)
  if (val === 'fixed') {
    if (props.resetHour == null) emit('update:resetHour', 0)
    if (props.dim === 'weekly' && props.resetDay == null) emit('update:resetDay', 1)
    if (!props.resetTimezone) emit('update:resetTimezone', 'UTC')
  }
}
</script>

<template>
  <div>
    <!-- Title row (only when global notify is enabled) -->
    <div v-if="quotaNotifyGlobalEnabled" class="flex items-center gap-2 mb-1">
      <span class="text-xs font-medium text-gray-700 dark:text-gray-300 flex-1 min-w-0">{{ label }}</span>
      <span v-if="limit && limit > 0" class="text-xs font-medium text-gray-700 dark:text-gray-300 flex-1 min-w-0">{{ t('admin.accounts.quotaNotify.alert') }}</span>
    </div>
    <label v-else class="text-xs font-medium text-gray-700 dark:text-gray-300 mb-1 block">{{ label }}</label>

    <!-- Input row -->
    <div class="flex items-center gap-2">
      <div :class="['relative', quotaNotifyGlobalEnabled ? 'flex-1 min-w-0' : 'flex-1']">
        <span class="absolute left-2.5 top-1/2 -translate-y-1/2 text-gray-500 dark:text-gray-400 text-sm">$</span>
        <input :value="limit" @input="onLimitInput" type="number" min="0" step="0.01" class="input pl-6 py-1.5 text-sm" :placeholder="t('admin.accounts.quotaLimitPlaceholder')" />
      </div>
      <QuotaNotifyToggle
        v-if="quotaNotifyGlobalEnabled && limit && limit > 0"
        class="flex-1 min-w-0"
        :enabled="notifyEnabled" :threshold="notifyThreshold" :threshold-type="notifyThresholdType"
        @update:enabled="emit('update:notifyEnabled', $event)" @update:threshold="emit('update:notifyThreshold', $event)" @update:threshold-type="emit('update:notifyThresholdType', $event)"
      />
    </div>

    <!-- Reset mode row (daily/weekly only) -->
    <div v-if="hasResetMode" class="mt-1 flex items-center gap-2 flex-wrap">
      <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{{ t('admin.accounts.quotaResetMode') }}</label>
      <select :value="resetMode || 'rolling'" @change="onModeChange" class="input py-1 text-xs w-auto">
        <option value="rolling">{{ t('admin.accounts.quotaResetModeRolling') }}</option>
        <option value="fixed">{{ t('admin.accounts.quotaResetModeFixed') }}</option>
      </select>
      <template v-if="resetMode === 'fixed'">
        <!-- Weekly: day of week selector -->
        <template v-if="dim === 'weekly'">
          <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{{ t('admin.accounts.quotaWeeklyResetDay') }}</label>
          <select :value="resetDay ?? 1" @change="emit('update:resetDay', Number(($event.target as HTMLSelectElement).value))" class="input py-1 text-xs w-28">
            <option v-for="d in dayOptions" :key="d.value" :value="d.value">{{ t('admin.accounts.dayOfWeek.' + d.key) }}</option>
          </select>
        </template>
        <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{{ t('admin.accounts.quotaResetHour') }}</label>
        <select :value="resetHour ?? 0" @change="emit('update:resetHour', Number(($event.target as HTMLSelectElement).value))" class="input py-1 text-xs w-24">
          <option v-for="h in hourOptions" :key="h" :value="h">{{ String(h).padStart(2, '0') }}:00</option>
        </select>
      </template>
      <span class="text-[11px] text-gray-500 dark:text-gray-400">
        <template v-if="resetMode === 'fixed'">{{ hintFixed }}</template>
        <template v-else>{{ hintRolling }}</template>
      </span>
    </div>

    <!-- Total dimension hint (no reset mode) -->
    <p v-if="!hasResetMode" class="input-hint mb-0 text-[11px]">{{ hintRolling }}</p>
  </div>
</template>
