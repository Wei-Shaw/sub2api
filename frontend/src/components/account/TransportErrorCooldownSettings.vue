<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import { MAX_TRANSPORT_ERROR_COOLDOWN_MINUTES } from './transportErrorCooldown'

const enabled = defineModel<boolean>('enabled', { required: true })
const minutes = defineModel<number | string>('minutes', { required: true })
const { t } = useI18n()
</script>

<template>
  <div class="space-y-3 border-t border-gray-200 pt-4 dark:border-dark-600">
    <div class="flex items-center justify-between gap-3">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.transportErrorCooldown.title') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.transportErrorCooldown.hint') }}
        </p>
      </div>
      <Toggle
        v-model="enabled"
        data-testid="transport-error-cooldown-toggle"
        :aria-label="t('admin.accounts.transportErrorCooldown.title')"
      />
    </div>
    <label v-if="enabled" class="block">
      <span class="input-label">{{ t('admin.accounts.transportErrorCooldown.minutes') }}</span>
      <input
        v-model.number="minutes"
        data-testid="transport-error-cooldown-minutes"
        type="number"
        min="1"
        :max="MAX_TRANSPORT_ERROR_COOLDOWN_MINUTES"
        step="1"
        required
        class="input"
      />
    </label>
    <p v-else class="text-xs text-amber-700 dark:text-amber-400">
      {{ t('admin.accounts.transportErrorCooldown.disabledWarning') }}
    </p>
  </div>
</template>
