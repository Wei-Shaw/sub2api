<script setup lang="ts">
import { ref, watch, computed REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'

const { t REDACTED = useI18n()

const props = defineProps<{
  totalLimit: number | null
  dailyLimit: number | null
  weeklyLimit: number | null
REDACTED>()

const emit = defineEmits<{
  'update:totalLimit': [value: number | null]
  'update:dailyLimit': [value: number | null]
  'update:weeklyLimit': [value: number | null]
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
  REDACTED
REDACTED)

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
          <p class="input-hint">{{ t('admin.accounts.quotaDailyLimitHint') REDACTEDREDACTED</p>
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
          <p class="input-hint">{{ t('admin.accounts.quotaWeeklyLimitHint') REDACTEDREDACTED</p>
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
