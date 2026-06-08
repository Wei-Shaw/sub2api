<template>
  <BaseDialog :show="ctx.settingsOpen.value" :title="t('admin.riskControl.settingsTitle')" width="extra-wide" @close="ctx.settingsOpen.value = false">
    <div class="space-y-6">
      <div class="flex gap-2 overflow-x-auto border-b border-gray-100 pb-3 dark:border-dark-700">
        <button
          v-for="tab in ctx.settingsTabs.value"
          :key="tab.id"
          type="button"
          class="inline-flex whitespace-nowrap rounded-lg px-3 py-2 text-sm font-medium transition-colors"
          :class="ctx.activeSettingsTab.value === tab.id ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'text-gray-500 hover:bg-gray-50 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white'"
          @click="ctx.activeSettingsTab.value = tab.id"
        >
          {{ tab.label }}
        </button>
      </div>

      <RiskSettingsBasicTab v-if="ctx.activeSettingsTab.value === 'basic'" />
      <RiskSettingsScopeTab v-else-if="ctx.activeSettingsTab.value === 'scope'" />
      <RiskSettingsRuntimeTab v-else-if="ctx.activeSettingsTab.value === 'runtime'" />
      <RiskSettingsResponseTab v-else-if="ctx.activeSettingsTab.value === 'response'" />
      <RiskSettingsThresholdsTab v-else-if="ctx.activeSettingsTab.value === 'riskThresholds'" />
      <RiskSettingsKeywordsTab v-else-if="ctx.activeSettingsTab.value === 'keywords'" />
      <RiskSettingsRetentionTab v-else />
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <button type="button" class="btn btn-secondary" @click="ctx.settingsOpen.value = false">{{ t('common.cancel') }}</button>
        <button type="button" class="btn btn-primary inline-flex items-center gap-2" :disabled="ctx.saving.value" @click="ctx.saveConfig">
          <Icon v-if="ctx.saving.value" name="refresh" size="sm" class="animate-spin" />
          <Icon v-else name="check" size="sm" />
          {{ ctx.saving.value ? t('common.saving') : t('admin.riskControl.saveConfig') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { BaseDialog, Icon } from '@sub2api/plugin-sdk'
import { RiskControlKey } from './riskControlContext'
import RiskSettingsBasicTab from './RiskSettingsBasicTab.vue'
import RiskSettingsScopeTab from './RiskSettingsScopeTab.vue'
import RiskSettingsRuntimeTab from './RiskSettingsRuntimeTab.vue'
import RiskSettingsResponseTab from './RiskSettingsResponseTab.vue'
import RiskSettingsThresholdsTab from './RiskSettingsThresholdsTab.vue'
import RiskSettingsKeywordsTab from './RiskSettingsKeywordsTab.vue'
import RiskSettingsRetentionTab from './RiskSettingsRetentionTab.vue'

const { t } = useI18n()
const ctx = inject(RiskControlKey)!
</script>
