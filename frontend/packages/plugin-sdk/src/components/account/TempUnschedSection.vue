<template>
  <div class="space-y-4 border-t border-gray-200 pt-4 dark:border-dark-600">
    <div class="mb-3 flex items-center justify-between">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.tempUnschedulable.title') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.tempUnschedulable.hint') }}
        </p>
      </div>
      <button
        type="button"
        @click="emit('update:enabled', !enabled)"
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
          {{ t('admin.accounts.tempUnschedulable.notice') }}
        </p>
      </div>

      <!-- Preset buttons -->
      <div class="flex flex-wrap gap-2">
        <button
          v-for="preset in presets"
          :key="preset.label"
          type="button"
          @click="addRule(preset.rule)"
          class="rounded-lg bg-gray-100 px-3 py-1.5 text-xs font-medium text-gray-600 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
        >
          + {{ preset.label }}
        </button>
      </div>

      <!-- Rule cards -->
      <div v-if="rules.length > 0" class="space-y-3">
        <div
          v-for="(rule, index) in rules"
          :key="getRuleKey(rule)"
          class="rounded-lg border border-gray-200 p-3 dark:border-dark-600"
        >
          <div class="mb-2 flex items-center justify-between">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.tempUnschedulable.ruleIndex', { index: index + 1 }) }}
            </span>
            <div class="flex items-center gap-2">
              <button
                type="button"
                :disabled="index === 0"
                @click="moveRule(index, -1)"
                class="rounded p-1 text-gray-400 transition-colors hover:text-gray-600 disabled:cursor-not-allowed disabled:opacity-40 dark:hover:text-gray-200"
              >
                <Icon name="chevronUp" size="sm" :stroke-width="2" />
              </button>
              <button
                type="button"
                :disabled="index === rules.length - 1"
                @click="moveRule(index, 1)"
                class="rounded p-1 text-gray-400 transition-colors hover:text-gray-600 disabled:cursor-not-allowed disabled:opacity-40 dark:hover:text-gray-200"
              >
                <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
                </svg>
              </button>
              <button
                type="button"
                @click="removeRule(index)"
                class="rounded p-1 text-red-500 transition-colors hover:text-red-600"
              >
                <Icon name="x" size="sm" :stroke-width="2" />
              </button>
            </div>
          </div>

          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div>
              <label class="input-label">{{ t('admin.accounts.tempUnschedulable.errorCode') }}</label>
              <input
                v-model.number="rule.error_code"
                type="number"
                min="100"
                max="599"
                class="input"
                :placeholder="t('admin.accounts.tempUnschedulable.errorCodePlaceholder')"
              />
            </div>
            <div>
              <label class="input-label">{{ t('admin.accounts.tempUnschedulable.durationMinutes') }}</label>
              <input
                v-model.number="rule.duration_minutes"
                type="number"
                min="1"
                class="input"
                :placeholder="t('admin.accounts.tempUnschedulable.durationPlaceholder')"
              />
            </div>
            <div class="sm:col-span-2">
              <label class="input-label">{{ t('admin.accounts.tempUnschedulable.keywords') }}</label>
              <input
                v-model="rule.keywords"
                type="text"
                class="input"
                :placeholder="t('admin.accounts.tempUnschedulable.keywordsPlaceholder')"
              />
              <p class="input-hint">{{ t('admin.accounts.tempUnschedulable.keywordsHint') }}</p>
            </div>
            <div class="sm:col-span-2">
              <label class="input-label">{{ t('admin.accounts.tempUnschedulable.description') }}</label>
              <input
                v-model="rule.description"
                type="text"
                class="input"
                :placeholder="t('admin.accounts.tempUnschedulable.descriptionPlaceholder')"
              />
            </div>
          </div>
        </div>
      </div>

      <!-- Add rule button -->
      <button
        type="button"
        @click="addRule()"
        class="w-full rounded-lg border-2 border-dashed border-gray-300 px-4 py-2 text-sm text-gray-600 transition-colors hover:border-gray-400 hover:text-gray-700 dark:border-dark-500 dark:text-gray-400 dark:hover:border-dark-400 dark:hover:text-gray-300"
      >
        <svg class="mr-1 inline h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
        </svg>
        {{ t('admin.accounts.tempUnschedulable.addRule') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '../Icon.vue'
import { createStableObjectKeyResolver } from '../../utils/stableObjectKey'

import type { TempUnschedRuleForm } from '../../account-form-helpers'

const props = defineProps<{
  enabled: boolean
  rules: TempUnschedRuleForm[]
}>()

const emit = defineEmits<{
  'update:enabled': [value: boolean]
  'update:rules': [value: TempUnschedRuleForm[]]
}>()

const { t } = useI18n()

const getRuleKey = createStableObjectKeyResolver<TempUnschedRuleForm>('temp-unsched-rule')

const presets = computed(() => [
  {
    label: t('admin.accounts.tempUnschedulable.presets.overloadLabel'),
    rule: {
      error_code: 529,
      keywords: 'overloaded, too many',
      duration_minutes: 60,
      description: t('admin.accounts.tempUnschedulable.presets.overloadDesc')
    } as TempUnschedRuleForm
  },
  {
    label: t('admin.accounts.tempUnschedulable.presets.rateLimitLabel'),
    rule: {
      error_code: 429,
      keywords: 'rate limit, too many requests',
      duration_minutes: 10,
      description: t('admin.accounts.tempUnschedulable.presets.rateLimitDesc')
    } as TempUnschedRuleForm
  },
  {
    label: t('admin.accounts.tempUnschedulable.presets.unavailableLabel'),
    rule: {
      error_code: 503,
      keywords: 'unavailable, maintenance',
      duration_minutes: 30,
      description: t('admin.accounts.tempUnschedulable.presets.unavailableDesc')
    } as TempUnschedRuleForm
  }
])

const addRule = (preset?: TempUnschedRuleForm) => {
  const next = [...props.rules]
  if (preset) {
    next.push({ ...preset })
  } else {
    next.push({ error_code: null, keywords: '', duration_minutes: 30, description: '' })
  }
  emit('update:rules', next)
}

const removeRule = (index: number) => {
  const next = [...props.rules]
  next.splice(index, 1)
  emit('update:rules', next)
}

const moveRule = (index: number, direction: number) => {
  const target = index + direction
  if (target < 0 || target >= props.rules.length) return
  const next = [...props.rules]
  const current = next[index]
  next[index] = next[target]
  next[target] = current
  emit('update:rules', next)
}
</script>
