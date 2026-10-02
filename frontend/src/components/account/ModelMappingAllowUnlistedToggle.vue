<template>
  <div class="mb-3 flex items-center justify-between gap-4 rounded-lg border border-gray-200 p-3 dark:border-dark-600">
    <div class="min-w-0">
      <label class="input-label mb-0">{{ t('admin.accounts.modelMappingAllowUnlisted.title') }}</label>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.modelMappingAllowUnlisted.hint') }}
      </p>
    </div>
    <Toggle
      v-model="enabled"
      data-testid="model-mapping-allow-unlisted-toggle"
      :aria-label="t('admin.accounts.modelMappingAllowUnlisted.title')"
    />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'

const enabled = defineModel<boolean>({ required: true })
const { t } = useI18n()
</script>
