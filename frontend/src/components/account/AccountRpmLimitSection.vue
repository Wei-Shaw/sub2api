<template>
  <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600" data-testid="account-rpm-limit-section">
    <div class="mb-3 flex items-center justify-between">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.rpmLimit.label') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.quotaControl.rpmLimit.hint') }}
        </p>
      </div>
      <button
        type="button"
        data-testid="account-rpm-limit-toggle"
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

    <div v-if="enabled" class="space-y-4">
      <div>
        <label class="input-label">{{ t('admin.accounts.quotaControl.rpmLimit.baseRpm') }}</label>
        <input
          v-model.number="baseRpm"
          data-testid="account-rpm-limit-base-rpm"
          type="number"
          min="1"
          max="10000"
          step="1"
          class="input"
          :placeholder="t('admin.accounts.quotaControl.rpmLimit.baseRpmPlaceholder')"
        />
        <p class="input-hint">{{ t('admin.accounts.quotaControl.rpmLimit.baseRpmHint') }}</p>
      </div>

      <div>
        <label class="input-label">{{ t('admin.accounts.quotaControl.rpmLimit.strategy') }}</label>
        <div class="flex gap-2">
          <button
            v-for="option in strategyOptions"
            :key="option.value"
            type="button"
            @click="strategy = option.value"
            :class="[
              'flex-1 rounded-lg px-3 py-2 text-sm font-medium transition-all',
              strategy === option.value
                ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
            ]"
          >
            <div class="text-center">
              <div>{{ option.label }}</div>
              <div class="mt-0.5 text-[10px] opacity-70">{{ option.hint }}</div>
            </div>
          </button>
        </div>
      </div>

      <div v-if="strategy === 'tiered'">
        <label class="input-label">{{ t('admin.accounts.quotaControl.rpmLimit.stickyBuffer') }}</label>
        <input
          v-model.number="stickyBuffer"
          type="number"
          min="1"
          step="1"
          class="input"
          :placeholder="t('admin.accounts.quotaControl.rpmLimit.stickyBufferPlaceholder')"
        />
        <p class="input-hint">{{ t('admin.accounts.quotaControl.rpmLimit.stickyBufferHint') }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
// Account-level RPM limit (extra.base_rpm / rpm_strategy / rpm_sticky_buffer).
// Shared by the create and edit dialogs for every account type outside the
// Anthropic OAuth/SetupToken quota-control panel, which keeps its own copy
// next to the user message queue setting.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

export type AccountRpmStrategy = 'tiered' | 'sticky_exempt'

const enabled = defineModel<boolean>('enabled', { required: true })
const baseRpm = defineModel<number | null>('baseRpm', { required: true })
const strategy = defineModel<AccountRpmStrategy>('strategy', { required: true })
const stickyBuffer = defineModel<number | null>('stickyBuffer', { required: true })

const { t } = useI18n()

const strategyOptions = computed(() => [
  {
    value: 'tiered' as const,
    label: t('admin.accounts.quotaControl.rpmLimit.strategyTiered'),
    hint: t('admin.accounts.quotaControl.rpmLimit.strategyTieredHint')
  },
  {
    value: 'sticky_exempt' as const,
    label: t('admin.accounts.quotaControl.rpmLimit.strategyStickyExempt'),
    hint: t('admin.accounts.quotaControl.rpmLimit.strategyStickyExemptHint')
  }
])
</script>
