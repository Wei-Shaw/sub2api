<template>
  <div class="space-y-6">
    <div v-if="ctx.loading.value" class="flex items-center justify-center py-16">
      <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
    </div>

    <template v-else>
      <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.description') }}</p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="ctx.statusLoading.value" @click="ctx.loadStatus(false)">
            <Icon name="refresh" size="sm" :class="ctx.statusLoading.value ? 'animate-spin' : ''" />
            {{ t('admin.riskControl.refreshStatus') }}
          </button>
          <button type="button" class="btn btn-primary inline-flex items-center gap-2" @click="ctx.openSettings">
            <Icon name="cog" size="sm" />
            {{ t('admin.riskControl.openSettings') }}
          </button>
        </div>
      </div>

      <RiskOverviewCards />
      <RiskPreBlockCards />
      <RiskWorkerCard />
      <RiskAuditLogTable />
    </template>

    <RiskSettingsDialog />
    <RiskInputDetailDialog />
  </div>
</template>

<script setup lang="ts">
import { provide } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon } from '@sub2api/plugin-sdk'
import { useRiskControl } from './useRiskControl'
import { RiskControlKey } from './riskControlContext'
import RiskOverviewCards from './RiskOverviewCards.vue'
import RiskPreBlockCards from './RiskPreBlockCards.vue'
import RiskWorkerCard from './RiskWorkerCard.vue'
import RiskAuditLogTable from './RiskAuditLogTable.vue'
import RiskSettingsDialog from './RiskSettingsDialog.vue'
import RiskInputDetailDialog from './RiskInputDetailDialog.vue'

const { t } = useI18n()
const ctx = useRiskControl()
provide(RiskControlKey, ctx)
</script>
