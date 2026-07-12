<template>
  <div class="mt-4 border-t border-gray-200 pt-3 dark:border-dark-600">
    <label class="flex items-center gap-2 text-xs font-medium text-gray-600 dark:text-gray-300">
      <input
        type="checkbox"
        :checked="config?.enabled === true"
        class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
        @change="toggleEnabled(($event.target as HTMLInputElement).checked)"
      />
      {{ t('admin.channels.form.timePricing') }}
    </label>

    <div v-if="config?.enabled" class="mt-3 space-y-3">
      <div class="max-w-sm">
        <label class="text-xs text-gray-400">{{ t('admin.channels.form.timezone') }}</label>
        <Select
          :model-value="config.timezone"
          :options="timezoneOptions"
          class="mt-0.5"
          searchable
          @update:model-value="updateTimezone"
        />
      </div>

      <div class="flex items-center justify-between">
        <div>
          <div class="text-xs font-medium text-gray-500 dark:text-gray-400">
            {{ t('admin.channels.form.timePeriods') }}
          </div>
          <div class="mt-0.5 text-xs text-gray-400">
            {{ t('admin.channels.form.timePricingHint') }}
          </div>
        </div>
        <button type="button" class="text-xs text-primary-600 hover:text-primary-700" @click="addPeriod">
          + {{ t('admin.channels.form.addTimePeriod') }}
        </button>
      </div>

      <div
        v-for="(period, index) in config.periods"
        :key="index"
        class="rounded border border-gray-200 bg-white p-3 dark:border-dark-600 dark:bg-dark-700"
      >
        <div class="grid gap-2 sm:grid-cols-[1fr_120px_120px_auto]">
          <input
            :value="period.name"
            type="text"
            class="input text-sm"
            :placeholder="t('admin.channels.form.timePeriodName')"
            @input="updatePeriod(index, { name: ($event.target as HTMLInputElement).value })"
          />
          <input
            :value="period.start_time"
            type="time"
            class="input text-sm"
            @input="updatePeriod(index, { start_time: ($event.target as HTMLInputElement).value })"
          />
          <input
            :value="period.end_time"
            type="time"
            class="input text-sm"
            @input="updatePeriod(index, { end_time: ($event.target as HTMLInputElement).value })"
          />
          <button
            type="button"
            class="rounded p-2 text-gray-400 hover:text-red-500"
            :title="t('common.delete')"
            @click="removePeriod(index)"
          >
            <Icon name="trash" size="sm" />
          </button>
        </div>

        <div class="mt-2 flex flex-wrap gap-x-3 gap-y-1">
          <label
            v-for="day in weekdayOptions"
            :key="day.value"
            class="flex items-center gap-1 text-xs text-gray-500 dark:text-gray-400"
          >
            <input
              type="checkbox"
              :checked="period.weekdays.includes(day.value)"
              class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              @change="toggleWeekday(index, day.value)"
            />
            {{ day.label }}
          </label>
          <span class="text-xs text-gray-400">{{ t('admin.channels.form.emptyWeekdaysAll') }}</span>
        </div>

        <div v-if="mode === 'token'" class="mt-2 grid grid-cols-2 gap-2 sm:grid-cols-5">
          <PriceInput :label="t('admin.channels.form.inputPrice')" :value="period.input_price" @update="updatePeriod(index, { input_price: $event })" />
          <PriceInput :label="t('admin.channels.form.outputPrice')" :value="period.output_price" @update="updatePeriod(index, { output_price: $event })" />
          <PriceInput :label="t('admin.channels.form.cacheWritePrice')" :value="period.cache_write_price" @update="updatePeriod(index, { cache_write_price: $event })" />
          <PriceInput :label="t('admin.channels.form.cacheReadPrice')" :value="period.cache_read_price" @update="updatePeriod(index, { cache_read_price: $event })" />
          <PriceInput :label="t('admin.channels.form.imageTokenPrice')" :value="period.image_output_price" @update="updatePeriod(index, { image_output_price: $event })" />
        </div>
        <div v-else class="mt-2 max-w-xs">
          <PriceInput :label="t('admin.channels.form.perRequestPrice')" :value="period.per_request_price" unit="$" @update="updatePeriod(index, { per_request_price: $event })" />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import PriceInput from './TimePricingPriceInput.vue'
import type { BillingMode } from '@/api/admin/channels'
import type { TimePricingFormConfig, TimePricingFormPeriod } from './types'

const { t } = useI18n()

const props = defineProps<{
  config: TimePricingFormConfig | null
  mode: BillingMode
}>()

const emit = defineEmits<{
  update: [config: TimePricingFormConfig | null]
}>()

const commonTimezones = [
  'UTC',
  'Asia/Shanghai',
  'Asia/Hong_Kong',
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

const timezoneOptions = computed<SelectOption[]>(() => {
  const currentTimezone = props.config?.timezone?.trim()
  const timezones = currentTimezone && !commonTimezones.includes(currentTimezone)
    ? [currentTimezone, ...commonTimezones]
    : commonTimezones

  return timezones.map(timezone => ({
    value: timezone,
    label: timezone,
  }))
})

const weekdayOptions = computed(() => [
  { value: 1, label: t('admin.channels.form.weekdayMon') },
  { value: 2, label: t('admin.channels.form.weekdayTue') },
  { value: 3, label: t('admin.channels.form.weekdayWed') },
  { value: 4, label: t('admin.channels.form.weekdayThu') },
  { value: 5, label: t('admin.channels.form.weekdayFri') },
  { value: 6, label: t('admin.channels.form.weekdaySat') },
  { value: 0, label: t('admin.channels.form.weekdaySun') },
])

function defaultConfig(): TimePricingFormConfig {
  return { enabled: true, timezone: 'Asia/Shanghai', periods: [] }
}

function toggleEnabled(enabled: boolean) {
  emit('update', { ...(props.config || defaultConfig()), enabled })
}

function updateConfig(patch: Partial<TimePricingFormConfig>) {
  emit('update', { ...(props.config || defaultConfig()), ...patch })
}

function updateTimezone(value: string | number | boolean | null) {
  if (typeof value === 'string') {
    updateConfig({ timezone: value })
  }
}

function addPeriod() {
  const config = props.config || defaultConfig()
  const period: TimePricingFormPeriod = {
    name: '',
    start_time: '09:00',
    end_time: '18:00',
    weekdays: [],
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_output_price: null,
    per_request_price: null,
  }
  emit('update', { ...config, periods: [...config.periods, period] })
}

function updatePeriod(index: number, patch: Partial<TimePricingFormPeriod>) {
  const config = props.config || defaultConfig()
  const periods = [...config.periods]
  periods[index] = { ...periods[index], ...patch }
  emit('update', { ...config, periods })
}

function removePeriod(index: number) {
  const config = props.config || defaultConfig()
  const periods = [...config.periods]
  periods.splice(index, 1)
  emit('update', { ...config, periods })
}

function toggleWeekday(index: number, weekday: number) {
  const config = props.config || defaultConfig()
  const period = config.periods[index]
  const weekdays = period.weekdays.includes(weekday)
    ? period.weekdays.filter(day => day !== weekday)
    : [...period.weekdays, weekday]
  updatePeriod(index, { weekdays })
}
</script>
