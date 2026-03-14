<script setup lang="ts">
import { ref, watch, computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'

const { t REDACTED = useI18n()

const props = defineProps<{
  totalLimit: number | null
  dailyLimit: number | null
  weeklyLimit: number | null
  dailyResetMode: 'rolling' | 'fixed' | null
  dailyResetHour: number | null
  weeklyResetMode: 'rolling' | 'fixed' | null
  weeklyResetDay: number | null
  weeklyResetHour: number | null
  resetTimezone: string | null
REDACTED>()

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
REDACTED>()

const enabled = computed(() =>
  (props.totalLimit != null && props.totalLimit > 0) ||
  (props.dailyLimit != null && props.dailyLimit > 0) ||
  (props.weeklyLimit != null && props.weeklyLimit > 0)
)

const localEnabled = ref(enabled.value)

// Sync when props change externally
watch(enabled, (val) => {
  localEnabled.value = val
REDACTED)

// When toggle is turned off, clear all values
watch(localEnabled, (val) => {
  if (!val) {
    emit('update:totalLimit', null)
    emit('update:dailyLimit', null)
    emit('update:weeklyLimit', null)
    emit('update:dailyResetMode', null)
    emit('update:dailyResetHour', null)
    emit('update:weeklyResetMode', null)
    emit('update:weeklyResetDay', null)
    emit('update:weeklyResetHour', null)
    emit('update:resetTimezone', null)
  REDACTED
REDACTED)

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
const hourOptions = Array.from({ length: 24 REDACTED, (_, i) => i)

// Day of week options
const dayOptions = [
  { value: 1, key: 'monday' REDACTED,
  { value: 2, key: 'tuesday' REDACTED,
  { value: 3, key: 'wednesday' REDACTED,
  { value: 4, key: 'thursday' REDACTED,
  { value: 5, key: 'friday' REDACTED,
  { value: 6, key: 'saturday' REDACTED,
  { value: 0, key: 'sunday' REDACTED,
]

const onTotalInput = (e: Event) => {
  const raw = (e.target as HTMLInputElement).valueAsNumber
  emit('update:totalLimit', Number.isNaN(raw) ? null : raw)
REDACTED
const onDailyInput = (e: Event) => {
  const raw = (e.target as HTMLInputElement).valueAsNumber
  emit('update:dailyLimit', Number.isNaN(raw) ? null : raw)
REDACTED
const onWeeklyInput = (e: Event) => {
  const raw = (e.target as HTMLInputElement).valueAsNumber
  emit('update:weeklyLimit', Number.isNaN(raw) ? null : raw)
REDACTED

const onDailyModeChange = (e: Event) => {
  const val = (e.target as HTMLSelectElement).value as 'rolling' | 'fixed'
  emit('update:dailyResetMode', val)
  if (val === 'fixed') {
    if (props.dailyResetHour == null) emit('update:dailyResetHour', 0)
    if (!props.resetTimezone) emit('update:resetTimezone', 'UTC')
  REDACTED
REDACTED

const onWeeklyModeChange = (e: Event) => {
  const val = (e.target as HTMLSelectElement).value as 'rolling' | 'fixed'
  emit('update:weeklyResetMode', val)
  if (val === 'fixed') {
    if (props.weeklyResetDay == null) emit('update:weeklyResetDay', 1)
    if (props.weeklyResetHour == null) emit('update:weeklyResetHour', 0)
    if (!props.resetTimezone) emit('update:resetTimezone', 'UTC')
  REDACTED
REDACTED
</script>

<template>
  <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
      <div class="mb-3 flex items-center justify-between">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.quotaLimitToggle') REDACTEDREDACTED</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.quotaLimitToggleHint') REDACTEDREDACTED
          </p>
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

      <div v-if="localEnabled" class="space-y-3">
        <!-- 日配额 -->
        <div>
          <label class="input-label">{{ t('admin.accounts.quotaDailyLimit') REDACTEDREDACTED</label>
          <div class="relative">
            <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500 dark:text-gray-400">$</span>
            <input
              :value="dailyLimit"
              @input="onDailyInput"
              type="number"
              min="0"
              step="0.01"
              class="input pl-7"
              :placeholder="t('admin.accounts.quotaLimitPlaceholder')"
            />
          </div>
          <!-- 日配额重置模式 -->
          <div class="mt-2 flex items-center gap-2">
            <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{{ t('admin.accounts.quotaResetMode') REDACTEDREDACTED</label>
            <select
              :value="dailyResetMode || 'rolling'"
              @change="onDailyModeChange"
              class="input py-1 text-xs"
            >
              <option value="rolling">{{ t('admin.accounts.quotaResetModeRolling') REDACTEDREDACTED</option>
              <option value="fixed">{{ t('admin.accounts.quotaResetModeFixed') REDACTEDREDACTED</option>
            </select>
          </div>
          <!-- 固定模式：小时选择 -->
          <div v-if="dailyResetMode === 'fixed'" class="mt-2 flex items-center gap-2">
            <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{{ t('admin.accounts.quotaResetHour') REDACTEDREDACTED</label>
            <select
              :value="dailyResetHour ?? 0"
              @change="emit('update:dailyResetHour', Number(($event.target as HTMLSelectElement).value))"
              class="input py-1 text-xs w-24"
            >
              <option v-for="h in hourOptions" :key="h" :value="h">{{ String(h).padStart(2, '0') REDACTEDREDACTED:00</option>
            </select>
          </div>
          <p class="input-hint">
            <template v-if="dailyResetMode === 'fixed'">
              {{ t('admin.accounts.quotaDailyLimitHintFixed', { hour: String(dailyResetHour ?? 0).padStart(2, '0'), timezone: resetTimezone || 'UTC' REDACTED) REDACTEDREDACTED
            </template>
            <template v-else>
              {{ t('admin.accounts.quotaDailyLimitHint') REDACTEDREDACTED
            </template>
          </p>
        </div>

        <!-- 周配额 -->
        <div>
          <label class="input-label">{{ t('admin.accounts.quotaWeeklyLimit') REDACTEDREDACTED</label>
          <div class="relative">
            <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500 dark:text-gray-400">$</span>
            <input
              :value="weeklyLimit"
              @input="onWeeklyInput"
              type="number"
              min="0"
              step="0.01"
              class="input pl-7"
              :placeholder="t('admin.accounts.quotaLimitPlaceholder')"
            />
          </div>
          <!-- 周配额重置模式 -->
          <div class="mt-2 flex items-center gap-2">
            <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{{ t('admin.accounts.quotaResetMode') REDACTEDREDACTED</label>
            <select
              :value="weeklyResetMode || 'rolling'"
              @change="onWeeklyModeChange"
              class="input py-1 text-xs"
            >
              <option value="rolling">{{ t('admin.accounts.quotaResetModeRolling') REDACTEDREDACTED</option>
              <option value="fixed">{{ t('admin.accounts.quotaResetModeFixed') REDACTEDREDACTED</option>
            </select>
          </div>
          <!-- 固定模式：星期几 + 小时 -->
          <div v-if="weeklyResetMode === 'fixed'" class="mt-2 flex items-center gap-2 flex-wrap">
            <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{{ t('admin.accounts.quotaWeeklyResetDay') REDACTEDREDACTED</label>
            <select
              :value="weeklyResetDay ?? 1"
              @change="emit('update:weeklyResetDay', Number(($event.target as HTMLSelectElement).value))"
              class="input py-1 text-xs w-28"
            >
              <option v-for="d in dayOptions" :key="d.value" :value="d.value">{{ t('admin.accounts.dayOfWeek.' + d.key) REDACTEDREDACTED</option>
            </select>
            <label class="text-xs text-gray-500 dark:text-gray-400 whitespace-nowrap">{{ t('admin.accounts.quotaResetHour') REDACTEDREDACTED</label>
            <select
              :value="weeklyResetHour ?? 0"
              @change="emit('update:weeklyResetHour', Number(($event.target as HTMLSelectElement).value))"
              class="input py-1 text-xs w-24"
            >
              <option v-for="h in hourOptions" :key="h" :value="h">{{ String(h).padStart(2, '0') REDACTEDREDACTED:00</option>
            </select>
          </div>
          <p class="input-hint">
            <template v-if="weeklyResetMode === 'fixed'">
              {{ t('admin.accounts.quotaWeeklyLimitHintFixed', { day: t('admin.accounts.dayOfWeek.' + (dayOptions.find(d => d.value === (weeklyResetDay ?? 1))?.key || 'monday')), hour: String(weeklyResetHour ?? 0).padStart(2, '0'), timezone: resetTimezone || 'UTC' REDACTED) REDACTEDREDACTED
            </template>
            <template v-else>
              {{ t('admin.accounts.quotaWeeklyLimitHint') REDACTEDREDACTED
            </template>
          </p>
        </div>

        <!-- 时区选择（当任一维度使用固定模式时显示） -->
        <div v-if="hasFixedMode">
          <label class="input-label">{{ t('admin.accounts.quotaResetTimezone') REDACTEDREDACTED</label>
          <select
            :value="resetTimezone || 'UTC'"
            @change="emit('update:resetTimezone', ($event.target as HTMLSelectElement).value)"
            class="input text-sm"
          >
            <option v-for="tz in timezoneOptions" :key="tz" :value="tz">{{ tz REDACTEDREDACTED</option>
          </select>
        </div>

        <!-- 总配额 -->
        <div>
          <label class="input-label">{{ t('admin.accounts.quotaTotalLimit') REDACTEDREDACTED</label>
          <div class="relative">
            <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500 dark:text-gray-400">$</span>
            <input
              :value="totalLimit"
              @input="onTotalInput"
              type="number"
              min="0"
              step="0.01"
              class="input pl-7"
              :placeholder="t('admin.accounts.quotaLimitPlaceholder')"
            />
          </div>
          <p class="input-hint">{{ t('admin.accounts.quotaTotalLimitHint') REDACTEDREDACTED</p>
        </div>
      </div>
  </div>
</template>
