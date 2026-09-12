<template>
  <div class="relative" ref="containerRef">
    <button
      type="button"
      @click="toggle"
      :class="['date-picker-trigger', isOpen && 'date-picker-trigger-open']"
    >
      <span class="date-picker-icon">
        <Icon name="calendar" size="sm" />
      </span>
      <span class="date-picker-value">
        {{ displayValue }}
      </span>
      <span class="date-picker-chevron">
        <Icon
          name="chevronDown"
          size="sm"
          :class="['transition-transform duration-200', isOpen && 'rotate-180']"
        />
      </span>
    </button>

    <Transition name="date-picker-dropdown">
      <div v-if="isOpen" class="date-picker-dropdown" :style="dropdownStyle">
        <!-- Quick presets -->
        <div class="date-picker-presets">
          <button
            v-for="preset in presets"
            :key="preset.value"
            @click="selectPreset(preset)"
            :class="['date-picker-preset', isPresetActive(preset) && 'date-picker-preset-active']"
          >
            {{ t(preset.labelKey) }}
          </button>
        </div>

        <div class="date-picker-divider"></div>

        <!-- Custom date range inputs -->
        <div class="date-picker-custom">
          <div class="date-picker-field">
            <label :for="`${inputId}-start`" class="date-picker-label">{{ t(enableTime ? 'dates.startTime' : 'dates.startDate') }}</label>
            <input
              :id="`${inputId}-start`"
              :type="enableTime ? 'datetime-local' : 'date'"
              :step="enableTime ? 60 : undefined"
              v-model="startInput"
              :max="endInput || undefined"
              :aria-invalid="!validRange"
              class="date-picker-input"
            />
          </div>
          <div class="date-picker-separator">
            <Icon name="arrowRight" size="sm" class="text-gray-400" />
          </div>
          <div class="date-picker-field">
            <label :for="`${inputId}-end`" class="date-picker-label">{{ t(enableTime ? 'dates.endTime' : 'dates.endDate') }}</label>
            <input
              :id="`${inputId}-end`"
              :type="enableTime ? 'datetime-local' : 'date'"
              :step="enableTime ? 60 : undefined"
              v-model="endInput"
              :min="startInput || undefined"
              :aria-invalid="!validRange"
              class="date-picker-input"
            />
          </div>
        </div>

        <p v-if="!validRange" role="alert" class="px-3 pb-2 text-xs text-red-600">{{ t('dates.invalidRange') }}</p>

        <!-- Apply button -->
        <div class="date-picker-actions">
          <button @click="apply" :disabled="!validRange" class="date-picker-apply">
            {{ t('dates.apply') }}
          </button>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, useId } from 'vue'
import type { CSSProperties } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { formatLocalDate, formatLocalMinute, getDatePresetRange, parseDateBoundary, parseLocalMinute } from '@/utils/dateRange'

interface Props {
  startDate: string
  endDate: string
  enableTime?: boolean
  preset?: string | null
}

interface Emits {
  (e: 'update:startDate', value: string): void
  (e: 'update:endDate', value: string): void
  (e: 'change', range: { startDate: string; endDate: string; preset: string | null }): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()
const { t, locale } = useI18n()
const inputId = useId()
const isOpen = ref(false)
const containerRef = ref<HTMLElement | null>(null)
const localStartDate = ref(props.startDate)
const localEndDate = ref(props.endDate)
const activePreset = ref<string | null>(null)
const dropdownStyle = ref<CSSProperties>({})

const presets = [
  { value: 'today', labelKey: 'dates.today' },
  { value: 'yesterday', labelKey: 'dates.yesterday' },
  { value: 'last24Hours', labelKey: 'dates.last24Hours' },
  { value: '7days', labelKey: 'dates.last7Days' },
  { value: '14days', labelKey: 'dates.last14Days' },
  { value: '30days', labelKey: 'dates.last30Days' },
  { value: 'thisMonth', labelKey: 'dates.thisMonth' },
  { value: 'lastMonth', labelKey: 'dates.lastMonth' }
]

const presetRange = (preset: string) => {
  const range = getDatePresetRange(preset)!
  // Date-only consumers retain their original API contract.
  if (!props.enableTime && preset === 'last24Hours') {
    const now = new Date()
    return { start: formatLocalDate(new Date(now.getTime() - 86400000)), end: formatLocalDate(now) }
  }
  return range
}

const detectPreset = () => {
  if (props.preset !== undefined) return props.preset
  // Explicit timestamps are fixed ranges unless the parent identifies a preset.
  if (props.startDate.includes('T') || props.endDate.includes('T')) return null
  return presets.find((p) => {
    const range = presetRange(p.value)
    return range.start === props.startDate && range.end === props.endDate
  })?.value ?? null
}

const syncDraft = () => {
  localStartDate.value = props.startDate
  localEndDate.value = props.endDate
  activePreset.value = detectPreset()
}

const inputValue = (value: string, end: boolean): string => {
  if (!value) return ''
  if (!props.enableTime) return value.includes('T') ? formatLocalDate(new Date(value)) : value
  // An unfinished/invalid local input must remain invalid rather than normalize.
  if (value.includes('T') && !/(?:[zZ]|[+-]\d{2}:\d{2})$/.test(value)) return value
  const date = parseDateBoundary(value, end)
  return Number.isFinite(date.getTime()) ? formatLocalMinute(date) : ''
}

const editBoundary = (value: string, end: boolean) => {
  const other = end ? localStartDate : localEndDate
  if (props.enableTime && other.value && !other.value.includes('T')) {
    other.value = parseDateBoundary(other.value, !end).toISOString()
  }
  const date = props.enableTime ? parseLocalMinute(value) : null
  ;(end ? localEndDate : localStartDate).value = date ? date.toISOString() : value
  activePreset.value = null
}
const startInput = computed({ get: () => inputValue(localStartDate.value, false), set: (v: string) => editBoundary(v, false) })
const endInput = computed({ get: () => inputValue(localEndDate.value, true), set: (v: string) => editBoundary(v, true) })

const validRange = computed(() => {
  if (!localStartDate.value || !localEndDate.value) return false
  if (props.enableTime && (!parseLocalMinute(startInput.value) || !parseLocalMinute(endInput.value))) return false
  const start = parseDateBoundary(localStartDate.value).getTime()
  const end = parseDateBoundary(localEndDate.value, true).getTime()
  return Number.isFinite(start) && Number.isFinite(end) && start < end
})

const formatDate = (value: string) => {
  const date = parseDateBoundary(value)
  return date.toLocaleString(locale.value === 'zh' ? 'zh-CN' : 'en-US', {
    year: 'numeric', month: 'short', day: 'numeric',
    ...(props.enableTime && value.includes('T') ? { hour: '2-digit', minute: '2-digit', hour12: false } : {})
  })
}
const displayValue = computed(() => {
  const preset = presets.find((p) => p.value === detectPreset())
  if (preset) return t(preset.labelKey)
  if (!props.startDate || !props.endDate) return t('dates.selectDateRange')
  return props.startDate === props.endDate ? formatDate(props.startDate) : `${formatDate(props.startDate)} - ${formatDate(props.endDate)}`
})

const isPresetActive = (preset: { value: string }) => activePreset.value === preset.value
const selectPreset = (preset: { value: string }) => {
  const range = presetRange(preset.value)
  localStartDate.value = range.start
  localEndDate.value = range.end
  activePreset.value = preset.value
}
const positionDropdown = () => {
  const rect = containerRef.value?.getBoundingClientRect()
  if (!rect) return
  const width = Math.min(props.enableTime ? 520 : 360, window.innerWidth - 24)
  const below = window.innerHeight - rect.bottom - 16
  const above = rect.top - 16
  const showAbove = below < 320 && above > below
  dropdownStyle.value = {
    width: `${width}px`,
    left: `${Math.max(12, Math.min(rect.left, window.innerWidth - width - 12))}px`,
    ...(showAbove ? { bottom: `${window.innerHeight - rect.top + 8}px` } : { top: `${rect.bottom + 8}px` }),
    maxHeight: `${Math.max(120, showAbove ? above : below)}px`
  }
}
const toggle = () => {
  if (!isOpen.value) { syncDraft(); positionDropdown() }
  isOpen.value = !isOpen.value
}
const apply = () => {
  if (!validRange.value) return
  if (activePreset.value) selectPreset({ value: activePreset.value })
  emit('update:startDate', localStartDate.value)
  emit('update:endDate', localEndDate.value)
  emit('change', { startDate: localStartDate.value, endDate: localEndDate.value, preset: activePreset.value })
  isOpen.value = false
}
const handleClickOutside = (event: MouseEvent) => {
  if (containerRef.value && !containerRef.value.contains(event.target as Node)) isOpen.value = false
}
const handleEscape = (event: KeyboardEvent) => { if (event.key === 'Escape') isOpen.value = false }
watch(() => [props.startDate, props.endDate, props.preset], () => { if (!isOpen.value) syncDraft() })
onMounted(() => {
  syncDraft()
  document.addEventListener('click', handleClickOutside)
  document.addEventListener('keydown', handleEscape)
  window.addEventListener('resize', positionDropdown)
  window.addEventListener('scroll', positionDropdown, true)
})
onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  document.removeEventListener('keydown', handleEscape)
  window.removeEventListener('resize', positionDropdown)
  window.removeEventListener('scroll', positionDropdown, true)
})
</script>

<style scoped>
.date-picker-trigger {
  max-width: 100%;
  @apply flex items-center gap-2;
  @apply rounded-lg px-3 py-2 text-sm;
  @apply bg-white dark:bg-dark-800;
  @apply border border-gray-200 dark:border-dark-600;
  @apply text-gray-700 dark:text-gray-300;
  @apply transition-all duration-200;
  @apply focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/30;
  @apply hover:border-gray-300 dark:hover:border-dark-500;
  @apply cursor-pointer;
}

.date-picker-trigger-open {
  @apply border-primary-500 ring-2 ring-primary-500/30;
}

.date-picker-icon {
  @apply text-gray-400 dark:text-dark-400;
}

.date-picker-value {
  @apply min-w-0 break-words font-medium;
}

.date-picker-chevron {
  @apply text-gray-400 dark:text-dark-400;
}

.date-picker-dropdown {
  @apply fixed z-[100];
  @apply bg-white dark:bg-dark-800;
  @apply rounded-xl;
  @apply border border-gray-200 dark:border-dark-700;
  @apply shadow-lg shadow-black/10 dark:shadow-black/30;
  @apply overflow-hidden;
  @apply overflow-y-auto;
}

.date-picker-presets {
  @apply grid grid-cols-2 gap-1 p-2;
}

.date-picker-preset {
  @apply rounded-md px-3 py-1.5 text-xs font-medium;
  @apply text-gray-600 dark:text-gray-400;
  @apply hover:bg-gray-100 dark:hover:bg-dark-700;
  @apply transition-colors duration-150;
}

.date-picker-preset-active {
  @apply bg-primary-100 dark:bg-primary-900/30;
  @apply text-primary-700 dark:text-primary-300;
}

.date-picker-divider {
  @apply border-t border-gray-100 dark:border-dark-700;
}

.date-picker-custom {
  @apply flex flex-col items-stretch gap-2 p-3 sm:flex-row sm:items-end;
}

.date-picker-field {
  @apply min-w-0 flex-1;
}

.date-picker-label {
  @apply mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400;
}

.date-picker-input {
  @apply w-full rounded-md px-2 py-1.5 text-sm;
  @apply bg-gray-50 dark:bg-dark-700;
  @apply border border-gray-200 dark:border-dark-600;
  @apply text-gray-900 dark:text-gray-100;
  @apply focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/30;
}

.date-picker-input::-webkit-calendar-picker-indicator {
  @apply cursor-pointer opacity-60 hover:opacity-100;
  filter: invert(0.5);
}

.dark .date-picker-input::-webkit-calendar-picker-indicator {
  filter: none;
}

.date-picker-separator {
  @apply hidden items-center justify-center pb-1 sm:flex;
}

.date-picker-actions {
  @apply flex justify-end p-2 pt-0;
}

.date-picker-apply:disabled { opacity: 0.5; cursor: not-allowed; }

.date-picker-apply {
  @apply rounded-lg px-4 py-1.5 text-sm font-medium;
  @apply bg-primary-600 text-white;
  @apply hover:bg-primary-700;
  @apply transition-colors duration-150;
}

/* Dropdown animation */
.date-picker-dropdown-enter-active,
.date-picker-dropdown-leave-active {
  transition: all 0.2s ease;
}

.date-picker-dropdown-enter-from,
.date-picker-dropdown-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}
</style>
