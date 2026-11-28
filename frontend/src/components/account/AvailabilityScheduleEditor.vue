<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import {
  AVAILABILITY_SCHEDULE_MAX_RULES,
  WEEKDAY_OPTIONS,
  createEmptyAvailabilityRule,
  previewAvailabilityForcedOff,
  type AvailabilityScheduleRuleForm
} from '@/utils/availabilitySchedule'

const enabled = defineModel<boolean>('enabled', { required: true })
const rules = defineModel<AvailabilityScheduleRuleForm[]>('rules', { required: true })

const { t } = useI18n()

const previewForcedOff = computed(() =>
  previewAvailabilityForcedOff(enabled.value, rules.value)
)

function addRule() {
  if (rules.value.length >= AVAILABILITY_SCHEDULE_MAX_RULES) return
  rules.value = [...rules.value, createEmptyAvailabilityRule('daily')]
}

function removeRule(index: number) {
  rules.value = rules.value.filter((_, i) => i !== index)
}

function moveRule(index: number, delta: number) {
  const target = index + delta
  if (target < 0 || target >= rules.value.length) return
  const next = [...rules.value]
  const [item] = next.splice(index, 1)
  next.splice(target, 0, item)
  rules.value = next
}

function toggleWeekday(rule: AvailabilityScheduleRuleForm, day: number) {
  const set = new Set(rule.weekdays)
  if (set.has(day)) set.delete(day)
  else set.add(day)
  rule.weekdays = Array.from(set).sort((a, b) => a - b)
}

function onKindChange(rule: AvailabilityScheduleRuleForm) {
  if (rule.kind === 'weekly' && rule.weekdays.length === 0) {
    rule.weekdays = [1, 2, 3, 4, 5]
  }
  if (rule.kind === 'daily') {
    rule.weekdays = []
  }
}
</script>

<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600 space-y-4">
    <div class="mb-1 flex items-center justify-between gap-3">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.availabilitySchedule.title') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.availabilitySchedule.hint') }}
        </p>
      </div>
      <button
        type="button"
        data-testid="availability-schedule-toggle"
        @click="enabled = !enabled"
        :class="[
          'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
          enabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
        ]"
      >
        <span
          :class="[
            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
            enabled ? 'translate-x-5' : 'translate-x-0'
          ]"
        />
      </button>
    </div>

    <div v-if="enabled" class="space-y-3">
      <div class="rounded-lg bg-blue-50 p-3 dark:bg-blue-900/20">
        <p class="text-xs text-blue-700 dark:text-blue-400">
          <Icon name="exclamationTriangle" size="sm" class="mr-1 inline" :stroke-width="2" />
          {{ t('admin.accounts.availabilitySchedule.notice') }}
        </p>
      </div>

      <p
        v-if="previewForcedOff !== null"
        class="text-xs"
        :class="previewForcedOff ? 'text-amber-600 dark:text-amber-400' : 'text-emerald-600 dark:text-emerald-400'"
        data-testid="availability-schedule-preview"
      >
        {{
          previewForcedOff
            ? t('admin.accounts.availabilitySchedule.previewOff')
            : t('admin.accounts.availabilitySchedule.previewOn')
        }}
      </p>
      <p v-else class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.availabilitySchedule.previewNone') }}
      </p>

      <div v-if="rules.length > 0" class="space-y-3">
        <div
          v-for="(rule, index) in rules"
          :key="rule.id"
          class="rounded-lg border border-gray-200 p-3 dark:border-dark-600"
          data-testid="availability-schedule-rule"
        >
          <div class="mb-2 flex items-center justify-between">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.availabilitySchedule.ruleIndex', { index: index + 1 }) }}
            </span>
            <div class="flex items-center gap-2">
              <button
                type="button"
                :disabled="index === 0"
                class="rounded p-1 text-gray-400 transition-colors hover:text-gray-600 disabled:cursor-not-allowed disabled:opacity-40 dark:hover:text-gray-200"
                @click="moveRule(index, -1)"
              >
                <Icon name="chevronUp" size="sm" :stroke-width="2" />
              </button>
              <button
                type="button"
                :disabled="index === rules.length - 1"
                class="rounded p-1 text-gray-400 transition-colors hover:text-gray-600 disabled:cursor-not-allowed disabled:opacity-40 dark:hover:text-gray-200"
                @click="moveRule(index, 1)"
              >
                <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
                </svg>
              </button>
              <button
                type="button"
                class="rounded p-1 text-red-500 transition-colors hover:text-red-600"
                @click="removeRule(index)"
              >
                <Icon name="x" size="sm" :stroke-width="2" />
              </button>
            </div>
          </div>

          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div>
              <label class="input-label">{{ t('admin.accounts.availabilitySchedule.kind') }}</label>
              <select v-model="rule.kind" class="input" @change="onKindChange(rule)">
                <option value="daily">{{ t('admin.accounts.availabilitySchedule.kindDaily') }}</option>
                <option value="weekly">{{ t('admin.accounts.availabilitySchedule.kindWeekly') }}</option>
              </select>
            </div>
            <div>
              <label class="input-label">{{ t('admin.accounts.availabilitySchedule.action') }}</label>
              <select v-model="rule.action" class="input">
                <option value="disable">{{ t('admin.accounts.availabilitySchedule.actionDisable') }}</option>
                <option value="enable">{{ t('admin.accounts.availabilitySchedule.actionEnable') }}</option>
              </select>
            </div>
            <div>
              <label class="input-label">{{ t('admin.accounts.availabilitySchedule.start') }}</label>
              <input v-model="rule.start" type="time" class="input" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.accounts.availabilitySchedule.end') }}</label>
              <input v-model="rule.end" type="time" class="input" />
              <p class="input-hint">{{ t('admin.accounts.availabilitySchedule.overnightHint') }}</p>
            </div>
          </div>

          <div v-if="rule.kind === 'weekly'" class="mt-3">
            <label class="input-label">{{ t('admin.accounts.availabilitySchedule.weekdays') }}</label>
            <div class="flex flex-wrap gap-2">
              <button
                v-for="day in WEEKDAY_OPTIONS"
                :key="day.value"
                type="button"
                class="rounded-lg px-2.5 py-1 text-xs font-medium transition-colors"
                :class="
                  rule.weekdays.includes(day.value)
                    ? 'bg-primary-600 text-white'
                    : 'bg-gray-100 text-gray-600 dark:bg-dark-600 dark:text-gray-300'
                "
                @click="toggleWeekday(rule, day.value)"
              >
                {{ t(`admin.accounts.availabilitySchedule.weekday.${day.key}`) }}
              </button>
            </div>
          </div>
        </div>
      </div>

      <button
        type="button"
        class="rounded-lg border border-dashed border-gray-300 px-3 py-2 text-sm text-gray-600 transition-colors hover:border-primary-400 hover:text-primary-600 dark:border-dark-500 dark:text-gray-300"
        :disabled="rules.length >= AVAILABILITY_SCHEDULE_MAX_RULES"
        data-testid="availability-schedule-add-rule"
        @click="addRule"
      >
        {{ t('admin.accounts.availabilitySchedule.addRule') }}
      </button>
    </div>
  </div>
</template>
