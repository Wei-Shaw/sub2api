<template>
  <section class="mt-5 border-t border-gray-200 pt-5 dark:border-dark-600" aria-labelledby="backup-archive-title">
    <div class="mb-3 flex items-center justify-between gap-3">
      <h4 id="backup-archive-title" class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.backup.archive.title') }}</h4>
      <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
        <input data-testid="archive-enabled" type="checkbox" :checked="modelValue.enabled" @change="update({ enabled: ($event.target as HTMLInputElement).checked })" />
        {{ t('admin.backup.archive.enabled') }}
      </label>
    </div>
    <div v-if="modelValue.enabled">
      <div class="grid grid-cols-1 items-start gap-3 md:grid-cols-2">
        <div>
          <label for="backup-archive-dates" class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.backup.archive.dates') }}</label>
          <div ref="datePicker" class="relative" @keydown.esc.stop.prevent="closeDates(true)" @focusout="handleFocusOut">
            <button
              id="backup-archive-dates" ref="dateTrigger" type="button" class="input flex w-full items-center justify-between gap-2 text-left"
              :aria-expanded="datesOpen" aria-controls="backup-archive-options" aria-describedby="backup-archive-dates-hint"
              @click="datesOpen = !datesOpen" @keydown.down.prevent="openDatesWithKeyboard"
            >
              <span class="flex min-w-0 flex-wrap items-center gap-1">
                <span v-if="!selectedLabels.length" class="text-gray-400">{{ t('admin.backup.archive.selectDates') }}</span>
                <span v-for="label in selectedLabels.slice(0, 3)" :key="label" class="rounded bg-primary-50 px-1.5 py-0.5 text-xs text-primary-700 dark:bg-primary-900/30 dark:text-primary-300">{{ label }}</span>
                <span v-if="selectedLabels.length > 3" class="text-xs">+{{ selectedLabels.length - 3 }}</span>
              </span>
              <Icon name="chevronDown" size="sm" class="shrink-0" />
            </button>
            <div v-if="datesOpen" id="backup-archive-options" class="absolute left-0 top-full z-30 mt-1 w-full rounded-xl border border-gray-200 bg-white p-2 shadow-lg dark:border-dark-600 dark:bg-dark-800">
              <div class="mb-1 flex items-center justify-between gap-2 px-1 text-xs text-gray-500 dark:text-gray-400">
                <span>{{ t('admin.backup.archive.selectedDates', { count: selectedLabels.length }) }}</span>
                <button type="button" class="min-h-9 px-2 text-primary-600 dark:text-primary-400" @click="closeDates(true)">{{ t('admin.backup.archive.done') }}</button>
              </div>
              <div class="grid grid-cols-4 gap-1" role="group" :aria-label="t('admin.backup.archive.dates')">
                <label v-for="day in 31" :key="day" class="flex min-h-10 cursor-pointer items-center gap-1.5 rounded px-1 text-xs hover:bg-gray-100 dark:hover:bg-dark-700">
                  <input type="checkbox" :value="day" :checked="modelValue.days.includes(day)" :aria-label="t('admin.backup.archive.day', { day })" @change="toggleDay(day)" />
                  {{ day }}
                </label>
                <label class="flex min-h-10 cursor-pointer items-center gap-1.5 rounded px-1 text-xs hover:bg-gray-100 dark:hover:bg-dark-700">
                  <input type="checkbox" value="last" :checked="modelValue.include_month_end" :aria-label="t('admin.backup.archive.monthEnd')" @change="update({ include_month_end: ($event.target as HTMLInputElement).checked })" />
                  {{ t('admin.backup.archive.monthEnd') }}
                </label>
              </div>
            </div>
          </div>
          <p id="backup-archive-dates-hint" class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.backup.archive.datesHint') }}</p>
        </div>
        <div>
          <span class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.backup.archive.retention') }}</span>
          <div class="flex min-h-10 flex-wrap items-center gap-3">
            <label v-if="!forever" class="flex min-w-0 flex-1 items-center gap-2">
              <input data-testid="archive-count" :value="modelValue.retain_count" type="number" min="1" step="1" class="input w-full min-w-0" :aria-label="t('admin.backup.archive.count')" aria-describedby="backup-archive-retention-hint" @input="setCount" />
              <span class="shrink-0 text-sm">{{ t('admin.backup.archive.copies') }}</span>
            </label>
            <label class="inline-flex min-h-10 shrink-0 items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
              <input data-testid="archive-forever" type="checkbox" :checked="forever" @change="toggleForever" />
              {{ t('admin.backup.archive.forever') }}
            </label>
          </div>
          <p id="backup-archive-retention-hint" class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t(forever ? 'admin.backup.archive.foreverHint' : 'admin.backup.archive.countHint') }}</p>
        </div>
      </div>
      <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.backup.archive.fallbackHint') }}</p>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.backup.archive.independentHint') }}</p>
    </div>
    <p v-else class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.backup.archive.disabledHint') }}</p>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { BackupMonthlyArchiveConfig } from '@/api/admin/backup'

const props = defineProps<{ modelValue: BackupMonthlyArchiveConfig }>()
const emit = defineEmits<{ 'update:modelValue': [value: BackupMonthlyArchiveConfig] }>()
const { t } = useI18n()
const datesOpen = ref(false)
const datePicker = ref<HTMLElement | null>(null)
const dateTrigger = ref<HTMLButtonElement | null>(null)
const lastFiniteCount = ref(12)
const forever = computed(() => props.modelValue.retain_count === 0)
const selectedLabels = computed(() => [
  ...[...props.modelValue.days].sort((a, b) => a - b).map(day => t('admin.backup.archive.day', { day })),
  ...(props.modelValue.include_month_end ? [t('admin.backup.archive.monthEnd')] : []),
])

watch(() => props.modelValue.retain_count, value => {
  if (Number.isInteger(value) && value > 0) lastFiniteCount.value = value
}, { immediate: true })
watch(() => props.modelValue.enabled, enabled => { if (!enabled) datesOpen.value = false })

function update(patch: Partial<BackupMonthlyArchiveConfig>) {
  emit('update:modelValue', { ...props.modelValue, ...patch })
}
function toggleDay(day: number) {
  const days = props.modelValue.days.includes(day) ? props.modelValue.days.filter(value => value !== day) : [...props.modelValue.days, day]
  update({ days: days.sort((a, b) => a - b) })
}
function setCount(event: Event) {
  const value = (event.target as HTMLInputElement).valueAsNumber
  // Zero is the wire representation of permanent retention, selected only by
  // the checkbox. Typing zero must remain an invalid finite count.
  update({ retain_count: value === 0 ? Number.NaN : value })
}
function toggleForever(event: Event) {
  update({ retain_count: (event.target as HTMLInputElement).checked ? 0 : lastFiniteCount.value })
}
function closeDates(focus = false) {
  datesOpen.value = false
  if (focus) dateTrigger.value?.focus()
}
async function openDatesWithKeyboard() {
  datesOpen.value = true
  await nextTick()
  datePicker.value?.querySelector<HTMLInputElement>('input')?.focus()
}
function handleFocusOut(event: FocusEvent) {
  if (event.relatedTarget instanceof Node && !datePicker.value?.contains(event.relatedTarget)) closeDates()
}
function handleOutsideClick(event: MouseEvent) {
  if (event.target instanceof Node && !datePicker.value?.contains(event.target)) closeDates()
}
onMounted(() => document.addEventListener('click', handleOutsideClick))
onBeforeUnmount(() => document.removeEventListener('click', handleOutsideClick))
</script>
