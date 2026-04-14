<script setup lang="ts">
import { useI18n REDACTED from 'vue-i18n'
import QuotaNotifyToggle from './QuotaNotifyToggle.vue'
import type { QuotaThresholdType, QuotaResetMode REDACTED from '@/constants/account'

const { t REDACTED = useI18n()

const props = defineProps<{
  dim: 'daily' | 'weekly' | 'total'
  label: string
  limit: number | null
  quotaNotifyGlobalEnabled: boolean
  notifyEnabled: boolean | null
  notifyThreshold: number | null
  notifyThresholdType: QuotaThresholdType | null
  // Reset mode (only for daily/weekly, null for total)
  resetMode: QuotaResetMode | null
  resetHour: number | null
  resetDay: number | null  // weekly only
  resetTimezone: string | null
  hintRolling: string
  hintFixed: string
  // Shared options passed from parent
  hourOptions: number[]
  dayOptions: { value: number; key: string REDACTED[]
  timezoneOptions?: string[]
REDACTED>()

const emit = defineEmits<{
  'update:limit': [value: number | null]
  'update:notifyEnabled': [value: boolean | null]
  'update:notifyThreshold': [value: number | null]
  'update:notifyThresholdType': [value: QuotaThresholdType | null]
  'update:resetMode': [value: QuotaResetMode | null]
  'update:resetHour': [value: number | null]
  'update:resetDay': [value: number | null]
  'update:resetTimezone': [value: string | null]
REDACTED>()

const hasResetMode = props.dim !== 'total'

const onLimitInput = (e: Event) => {
  const raw = (e.target as HTMLInputElement).valueAsNumber
  emit('update:limit', Number.isNaN(raw) ? null : raw)
REDACTED

const onModeChange = (e: Event) => {
  const val = (e.target as HTMLSelectElement).value as QuotaResetMode
  emit('update:resetMode', val)
  if (val === 'fixed') {
    if (props.resetHour == null) emit('update:resetHour', 0)
    if (props.dim === 'weekly' && props.resetDay == null) emit('update:resetDay', 1)
    if (!props.resetTimezone) emit('update:resetTimezone', 'UTC')
  REDACTED
REDACTED

function getTimezoneOffsetLabel(tz: string): string {
  try {
    const dtf = new Intl.DateTimeFormat('en-US', { timeZone: tz, timeZoneName: 'shortOffset' REDACTED)
    const parts = dtf.formatToParts(new Date())
    const tzPart = parts.find(p => p.type === 'timeZoneName')
    return tzPart ? (tzPart.value === 'GMT' ? 'GMT+0' : tzPart.value) : ''
  REDACTED catch {
    return ''
  REDACTED
REDACTED
</script>

<template>
  <div>
    <!-- Title row (only when global notify is enabled) -->
    <div v-if="quotaNotifyGlobalEnabled" class="flex items-center gap-2 mb-1">
      <span class="text-xs font-medium text-gray-700 dark:text-gray-300 flex-1 min-w-0">{{ label REDACTEDREDACTED</span>
      <span v-if="limit && limit > 0" class="text-xs font-medium text-gray-700 dark:text-gray-300 flex-1 min-w-0">{{ t('admin.accounts.quotaNotify.alert') REDACTEDREDACTED</span>
    </div>
    <label v-else class="text-xs font-medium text-gray-700 dark:text-gray-300 mb-1 block">{{ label REDACTEDREDACTED</label>

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
      <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{{ t('admin.accounts.quotaResetMode') REDACTEDREDACTED</label>
      <select :value="resetMode || 'rolling'" @change="onModeChange" class="input py-1 text-xs w-auto">
        <option value="rolling">{{ t('admin.accounts.quotaResetModeRolling') REDACTEDREDACTED</option>
        <option value="fixed">{{ t('admin.accounts.quotaResetModeFixed') REDACTEDREDACTED</option>
      </select>
      <template v-if="resetMode === 'fixed'">
        <!-- Weekly: day of week selector -->
        <template v-if="dim === 'weekly'">
          <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{{ t('admin.accounts.quotaWeeklyResetDay') REDACTEDREDACTED</label>
          <select :value="resetDay ?? 1" @change="emit('update:resetDay', Number(($event.target as HTMLSelectElement).value))" class="input py-1 text-xs w-28">
            <option v-for="d in dayOptions" :key="d.value" :value="d.value">{{ t('admin.accounts.dayOfWeek.' + d.key) REDACTEDREDACTED</option>
          </select>
        </template>
        <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{{ t('admin.accounts.quotaResetHour') REDACTEDREDACTED</label>
        <select :value="resetHour ?? 0" @change="emit('update:resetHour', Number(($event.target as HTMLSelectElement).value))" class="input py-1 text-xs w-24">
          <option v-for="h in hourOptions" :key="h" :value="h">{{ String(h).padStart(2, '0') REDACTEDREDACTED:00</option>
        </select>
        <template v-if="timezoneOptions && timezoneOptions.length > 0">
          <select :value="resetTimezone || 'UTC'" @change="emit('update:resetTimezone', ($event.target as HTMLSelectElement).value)" class="input py-1 text-xs w-auto">
            <option v-for="tz in timezoneOptions" :key="tz" :value="tz">{{ tz REDACTEDREDACTED ({{ getTimezoneOffsetLabel(tz) REDACTEDREDACTED)</option>
          </select>
        </template>
      </template>
      <span class="text-[11px] text-gray-500 dark:text-gray-400">
        <template v-if="resetMode === 'fixed'">{{ hintFixed REDACTEDREDACTED</template>
        <template v-else>{{ hintRolling REDACTEDREDACTED</template>
      </span>
    </div>

    <!-- Total dimension hint (no reset mode) -->
    <p v-if="!hasResetMode" class="input-hint mb-0 text-[11px]">{{ hintRolling REDACTEDREDACTED</p>
  </div>
</template>
