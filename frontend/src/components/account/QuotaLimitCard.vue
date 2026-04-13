<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import QuotaNotifyToggle from './QuotaNotifyToggle.vue'

const { t } = useI18n()

const props = withDefaults(defineProps<{
  totalLimit: number | null
  dailyLimit: number | null
  weeklyLimit: number | null
  dailyResetMode: 'rolling' | 'fixed' | null
  dailyResetHour: number | null
  weeklyResetMode: 'rolling' | 'fixed' | null
  weeklyResetDay: number | null
  weeklyResetHour: number | null
  resetTimezone: string | null
  quotaNotifyGlobalEnabled?: boolean
  quotaNotifyDailyEnabled?: boolean | null
  quotaNotifyDailyThreshold?: number | null
  quotaNotifyDailyThresholdType?: string | null
  quotaNotifyWeeklyEnabled?: boolean | null
  quotaNotifyWeeklyThreshold?: number | null
  quotaNotifyWeeklyThresholdType?: string | null
  quotaNotifyTotalEnabled?: boolean | null
  quotaNotifyTotalThreshold?: number | null
  quotaNotifyTotalThresholdType?: string | null
}>(), {
  quotaNotifyGlobalEnabled: false,
  quotaNotifyDailyEnabled: null,
  quotaNotifyDailyThreshold: null,
  quotaNotifyDailyThresholdType: null,
  quotaNotifyWeeklyEnabled: null,
  quotaNotifyWeeklyThreshold: null,
  quotaNotifyWeeklyThresholdType: null,
  quotaNotifyTotalEnabled: null,
  quotaNotifyTotalThreshold: null,
  quotaNotifyTotalThresholdType: null,
})

const emit = defineEmits<{
  'update:totalLimit': [value: number | null]
  'update:dailyLimit': [value: number | null]
  'update:weeklyLimit': [value: number | null]
  'update:dailyResetMode': [value: 'rolling' | 'fixed' | null]
  'update:dailyResetHour': [value: number | null]
  'update:weeklyResetMode': [value: 'rolling' | 'fixed' | null]
  'update:weeklyResetDay': [value: number | null]
  'update:weeklyResetHour': [value: number | null]
  'update:resetTimezone': [value: string | null]
  'update:quotaNotifyDailyEnabled': [value: boolean | null]
  'update:quotaNotifyDailyThreshold': [value: number | null]
  'update:quotaNotifyDailyThresholdType': [value: string | null]
  'update:quotaNotifyWeeklyEnabled': [value: boolean | null]
  'update:quotaNotifyWeeklyThreshold': [value: number | null]
  'update:quotaNotifyWeeklyThresholdType': [value: string | null]
  'update:quotaNotifyTotalEnabled': [value: boolean | null]
  'update:quotaNotifyTotalThreshold': [value: number | null]
  'update:quotaNotifyTotalThresholdType': [value: string | null]
}>()

const enabled = computed(() =>
  (props.totalLimit != null && props.totalLimit > 0) ||
  (props.dailyLimit != null && props.dailyLimit > 0) ||
  (props.weeklyLimit != null && props.weeklyLimit > 0)
)

const localEnabled = ref(enabled.value)
const collapsed = ref(false)

// Sync when props change externally
watch(enabled, (val) => {
  localEnabled.value = val
})

// When toggle is turned off, clear all values and expand
watch(localEnabled, (val) => {
  if (!val) {
    collapsed.value = false
    emit('update:totalLimit', null)
    emit('update:dailyLimit', null)
    emit('update:weeklyLimit', null)
    emit('update:dailyResetMode', null)
    emit('update:dailyResetHour', null)
    emit('update:weeklyResetMode', null)
    emit('update:weeklyResetDay', null)
    emit('update:weeklyResetHour', null)
    emit('update:resetTimezone', null)
  }
})

// Whether any fixed mode is active (to show timezone selector)
const hasFixedMode = computed(() =>
  props.dailyResetMode === 'fixed' || props.weeklyResetMode === 'fixed'
)

// Common timezone options
const timezoneOptions = [
  'UTC',
  'Asia/Shanghai',
  'Asia/Tokyo',
  'Asia/Seoul',
  'Asia/Singapore',
  'Asia/Kolkata',
  'Asia/Dubai',
  'Europe/London',
  'Europe/Paris',
  'Europe/Berlin',
  'Europe/Moscow',
  'America/New_York',
  'America/Chicago',
  'America/Denver',
  'America/Los_Angeles',
  'America/Sao_Paulo',
  'Australia/Sydney',
  'Pacific/Auckland',
]

// Hours for dropdown (0-23)
const hourOptions = Array.from({ length: 24 }, (_, i) => i)

// Day of week options
const dayOptions = [
  { value: 1, key: 'monday' },
  { value: 2, key: 'tuesday' },
  { value: 3, key: 'wednesday' },
  { value: 4, key: 'thursday' },
  { value: 5, key: 'friday' },
  { value: 6, key: 'saturday' },
  { value: 0, key: 'sunday' },
]

const onTotalInput = (e: Event) => {
  const raw = (e.target as HTMLInputElement).valueAsNumber
  emit('update:totalLimit', Number.isNaN(raw) ? null : raw)
}
const onDailyInput = (e: Event) => {
  const raw = (e.target as HTMLInputElement).valueAsNumber
  emit('update:dailyLimit', Number.isNaN(raw) ? null : raw)
}
const onWeeklyInput = (e: Event) => {
  const raw = (e.target as HTMLInputElement).valueAsNumber
  emit('update:weeklyLimit', Number.isNaN(raw) ? null : raw)
}

const onDailyModeChange = (e: Event) => {
  const val = (e.target as HTMLSelectElement).value as 'rolling' | 'fixed'
  emit('update:dailyResetMode', val)
  if (val === 'fixed') {
    if (props.dailyResetHour == null) emit('update:dailyResetHour', 0)
    if (!props.resetTimezone) emit('update:resetTimezone', 'UTC')
  }
}

const onWeeklyModeChange = (e: Event) => {
  const val = (e.target as HTMLSelectElement).value as 'rolling' | 'fixed'
  emit('update:weeklyResetMode', val)
  if (val === 'fixed') {
    if (props.weeklyResetDay == null) emit('update:weeklyResetDay', 1)
    if (props.weeklyResetHour == null) emit('update:weeklyResetHour', 0)
    if (!props.resetTimezone) emit('update:resetTimezone', 'UTC')
  }
}
</script>

<template>
  <div class="rounded-lg border border-gray-200 dark:border-dark-600">
      <!-- Header: toggle + collapse -->
      <div class="flex items-center justify-between p-4" :class="{ 'pb-0': localEnabled && !collapsed }">
        <div class="flex items-center gap-2 flex-1 cursor-pointer" @click="localEnabled && (collapsed = !collapsed)">
          <svg v-if="localEnabled" class="h-4 w-4 text-gray-400 transition-transform" :class="{ '-rotate-90': collapsed }" viewBox="0 0 20 20" fill="currentColor">
            <path fill-rule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clip-rule="evenodd" />
          </svg>
          <div>
            <label class="input-label mb-0 cursor-pointer">{{ t('admin.accounts.quotaLimitToggle') }}</label>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.quotaLimitToggleHint') }}
            </p>
          </div>
        </div>
        <button
          type="button"
          @click="localEnabled = !localEnabled"
          :class="[
            'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
            localEnabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
          ]"
        >
          <span
            :class="[
              'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
              localEnabled ? 'translate-x-5' : 'translate-x-0'
            ]"
          />
        </button>
      </div>

      <!-- Collapsible content -->
      <div v-if="localEnabled && !collapsed" class="space-y-2 p-4 pt-3">
        <!-- 日配额 -->
        <div>
          <!-- 标题行（仅全局通知开启时显示） -->
          <div v-if="quotaNotifyGlobalEnabled" class="flex items-center gap-2 mb-1">
            <span class="text-xs font-medium text-gray-700 dark:text-gray-300 w-28 flex-shrink-0">{{ t('admin.accounts.quotaDailyLimit') }}</span>
            <span v-if="dailyLimit && dailyLimit > 0" class="text-xs font-medium text-gray-700 dark:text-gray-300">{{ t('admin.accounts.quotaNotify.alert') }}</span>
          </div>
          <label v-else class="text-xs font-medium text-gray-700 dark:text-gray-300 mb-1 block">{{ t('admin.accounts.quotaDailyLimit') }}</label>
          <!-- 输入行 -->
          <div class="flex items-center gap-2">
            <div :class="['relative', quotaNotifyGlobalEnabled ? 'w-28 flex-shrink-0' : 'flex-1']">
              <span class="absolute left-2.5 top-1/2 -translate-y-1/2 text-gray-500 dark:text-gray-400 text-sm">$</span>
              <input :value="dailyLimit" @input="onDailyInput" type="number" min="0" step="0.01" class="input pl-6 py-1.5 text-sm" :placeholder="t('admin.accounts.quotaLimitPlaceholder')" />
            </div>
            <QuotaNotifyToggle
              v-if="quotaNotifyGlobalEnabled && dailyLimit && dailyLimit > 0"
              class="flex-1 min-w-0"
              :enabled="props.quotaNotifyDailyEnabled" :threshold="props.quotaNotifyDailyThreshold" :threshold-type="props.quotaNotifyDailyThresholdType"
              @update:enabled="emit('update:quotaNotifyDailyEnabled', $event)" @update:threshold="emit('update:quotaNotifyDailyThreshold', $event)" @update:threshold-type="emit('update:quotaNotifyDailyThresholdType', $event)"
            />
          </div>
          <div class="mt-1 flex items-center gap-2 flex-wrap">
            <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{{ t('admin.accounts.quotaResetMode') }}</label>
            <select :value="dailyResetMode || 'rolling'" @change="onDailyModeChange" class="input py-1 text-xs w-auto">
              <option value="rolling">{{ t('admin.accounts.quotaResetModeRolling') }}</option>
              <option value="fixed">{{ t('admin.accounts.quotaResetModeFixed') }}</option>
            </select>
            <template v-if="dailyResetMode === 'fixed'">
              <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{{ t('admin.accounts.quotaResetHour') }}</label>
              <select :value="dailyResetHour ?? 0" @change="emit('update:dailyResetHour', Number(($event.target as HTMLSelectElement).value))" class="input py-1 text-xs w-24">
                <option v-for="h in hourOptions" :key="h" :value="h">{{ String(h).padStart(2, '0') }}:00</option>
              </select>
            </template>
          </div>
          <p class="input-hint mb-0 text-[11px]">
            <template v-if="dailyResetMode === 'fixed'">{{ t('admin.accounts.quotaDailyLimitHintFixed', { hour: String(dailyResetHour ?? 0).padStart(2, '0'), timezone: resetTimezone || 'UTC' }) }}</template>
            <template v-else>{{ t('admin.accounts.quotaDailyLimitHint') }}</template>
          </p>
        </div>

        <!-- 周配额 -->
        <div>
          <div v-if="quotaNotifyGlobalEnabled" class="flex items-center gap-2 mb-1">
            <span class="text-xs font-medium text-gray-700 dark:text-gray-300 w-28 flex-shrink-0">{{ t('admin.accounts.quotaWeeklyLimit') }}</span>
            <span v-if="weeklyLimit && weeklyLimit > 0" class="text-xs font-medium text-gray-700 dark:text-gray-300">{{ t('admin.accounts.quotaNotify.alert') }}</span>
          </div>
          <label v-else class="text-xs font-medium text-gray-700 dark:text-gray-300 mb-1 block">{{ t('admin.accounts.quotaWeeklyLimit') }}</label>
          <div class="flex items-center gap-2">
            <div :class="['relative', quotaNotifyGlobalEnabled ? 'w-28 flex-shrink-0' : 'flex-1']">
              <span class="absolute left-2.5 top-1/2 -translate-y-1/2 text-gray-500 dark:text-gray-400 text-sm">$</span>
              <input :value="weeklyLimit" @input="onWeeklyInput" type="number" min="0" step="0.01" class="input pl-6 py-1.5 text-sm" :placeholder="t('admin.accounts.quotaLimitPlaceholder')" />
            </div>
            <QuotaNotifyToggle
              v-if="quotaNotifyGlobalEnabled && weeklyLimit && weeklyLimit > 0"
              class="flex-1 min-w-0"
              :enabled="props.quotaNotifyWeeklyEnabled" :threshold="props.quotaNotifyWeeklyThreshold" :threshold-type="props.quotaNotifyWeeklyThresholdType"
              @update:enabled="emit('update:quotaNotifyWeeklyEnabled', $event)" @update:threshold="emit('update:quotaNotifyWeeklyThreshold', $event)" @update:threshold-type="emit('update:quotaNotifyWeeklyThresholdType', $event)"
            />
          </div>
          <div class="mt-1 flex items-center gap-2 flex-wrap">
            <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{{ t('admin.accounts.quotaResetMode') }}</label>
            <select :value="weeklyResetMode || 'rolling'" @change="onWeeklyModeChange" class="input py-1 text-xs w-auto">
              <option value="rolling">{{ t('admin.accounts.quotaResetModeRolling') }}</option>
              <option value="fixed">{{ t('admin.accounts.quotaResetModeFixed') }}</option>
            </select>
            <template v-if="weeklyResetMode === 'fixed'">
              <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{{ t('admin.accounts.quotaWeeklyResetDay') }}</label>
              <select :value="weeklyResetDay ?? 1" @change="emit('update:weeklyResetDay', Number(($event.target as HTMLSelectElement).value))" class="input py-1 text-xs w-28">
                <option v-for="d in dayOptions" :key="d.value" :value="d.value">{{ t('admin.accounts.dayOfWeek.' + d.key) }}</option>
              </select>
              <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{{ t('admin.accounts.quotaResetHour') }}</label>
              <select :value="weeklyResetHour ?? 0" @change="emit('update:weeklyResetHour', Number(($event.target as HTMLSelectElement).value))" class="input py-1 text-xs w-24">
                <option v-for="h in hourOptions" :key="h" :value="h">{{ String(h).padStart(2, '0') }}:00</option>
              </select>
            </template>
          </div>
          <p class="input-hint mb-0 text-[11px]">
            <template v-if="weeklyResetMode === 'fixed'">{{ t('admin.accounts.quotaWeeklyLimitHintFixed', { day: t('admin.accounts.dayOfWeek.' + (dayOptions.find(d => d.value === (weeklyResetDay ?? 1))?.key || 'monday')), hour: String(weeklyResetHour ?? 0).padStart(2, '0'), timezone: resetTimezone || 'UTC' }) }}</template>
            <template v-else>{{ t('admin.accounts.quotaWeeklyLimitHint') }}</template>
          </p>
        </div>

        <!-- 时区选择 -->
        <div v-if="hasFixedMode">
          <label class="input-label">{{ t('admin.accounts.quotaResetTimezone') }}</label>
          <select :value="resetTimezone || 'UTC'" @change="emit('update:resetTimezone', ($event.target as HTMLSelectElement).value)" class="input text-sm">
            <option v-for="tz in timezoneOptions" :key="tz" :value="tz">{{ tz }}</option>
          </select>
        </div>

        <!-- 总配额 -->
        <div>
          <div v-if="quotaNotifyGlobalEnabled" class="flex items-center gap-2 mb-1">
            <span class="text-xs font-medium text-gray-700 dark:text-gray-300 w-28 flex-shrink-0">{{ t('admin.accounts.quotaTotalLimit') }}</span>
            <span v-if="totalLimit && totalLimit > 0" class="text-xs font-medium text-gray-700 dark:text-gray-300">{{ t('admin.accounts.quotaNotify.alert') }}</span>
          </div>
          <label v-else class="text-xs font-medium text-gray-700 dark:text-gray-300 mb-1 block">{{ t('admin.accounts.quotaTotalLimit') }}</label>
          <div class="flex items-center gap-2">
            <div :class="['relative', quotaNotifyGlobalEnabled ? 'w-28 flex-shrink-0' : 'flex-1']">
              <span class="absolute left-2.5 top-1/2 -translate-y-1/2 text-gray-500 dark:text-gray-400 text-sm">$</span>
              <input :value="totalLimit" @input="onTotalInput" type="number" min="0" step="0.01" class="input pl-6 py-1.5 text-sm" :placeholder="t('admin.accounts.quotaLimitPlaceholder')" />
            </div>
            <QuotaNotifyToggle
              v-if="quotaNotifyGlobalEnabled && totalLimit && totalLimit > 0"
              class="flex-1 min-w-0"
              :enabled="props.quotaNotifyTotalEnabled" :threshold="props.quotaNotifyTotalThreshold" :threshold-type="props.quotaNotifyTotalThresholdType"
              @update:enabled="emit('update:quotaNotifyTotalEnabled', $event)" @update:threshold="emit('update:quotaNotifyTotalThreshold', $event)" @update:threshold-type="emit('update:quotaNotifyTotalThresholdType', $event)"
            />
          </div>
          <p class="input-hint mb-0 text-[11px]">{{ t('admin.accounts.quotaTotalLimitHint') }}</p>
        </div>
      </div>
  </div>
</template>
