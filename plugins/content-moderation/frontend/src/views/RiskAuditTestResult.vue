<template>
  <div v-if="ctx.moderationTestResult.value" class="mt-4 rounded-lg border border-gray-100 bg-white p-3 dark:border-dark-700 dark:bg-dark-800">
    <div class="flex items-start justify-between gap-3">
      <div>
        <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.auditTestResult') }}</p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.riskControl.auditTestHighest', { category: ctx.moderationTestResult.value.highest_category || '-', score: ctx.percent(ctx.moderationTestResult.value.highest_score) }) }}
        </p>
      </div>
      <span class="inline-flex rounded-full px-2 py-1 text-xs font-medium" :class="ctx.moderationTestResult.value.flagged ? 'bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300' : 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'">
        {{ ctx.moderationTestResult.value.flagged ? t('admin.riskControl.auditTestFlagged') : t('admin.riskControl.auditTestPassed') }}
      </span>
    </div>
    <div class="mt-3">
      <div class="mb-2 flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
        <span>{{ t('admin.riskControl.auditTestComposite') }}</span>
        <span class="font-semibold text-gray-900 dark:text-white">{{ ctx.percent(ctx.moderationTestResult.value.composite_score) }}</span>
      </div>
      <div class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
        <div class="h-full rounded-full" :class="ctx.moderationTestResult.value.flagged ? 'bg-red-500' : 'bg-emerald-500'" :style="{ width: ctx.percentWidth(ctx.moderationTestResult.value.composite_score) }"></div>
      </div>
    </div>
    <div class="mt-3 max-h-52 space-y-2 overflow-y-auto pr-1">
      <div v-for="score in ctx.moderationScoreRows.value" :key="score.category">
        <div class="mb-1 flex items-center justify-between gap-3 text-xs">
          <span class="truncate text-gray-600 dark:text-gray-300">{{ score.category }}</span>
          <span class="font-mono text-gray-500 dark:text-gray-400">{{ ctx.percent(score.score) }} / {{ ctx.percent(score.threshold) }}</span>
        </div>
        <div class="h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
          <div class="h-full rounded-full" :class="score.hit ? 'bg-red-500' : 'bg-primary-500'" :style="{ width: ctx.percentWidth(score.score) }"></div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { RiskControlKey } from './riskControlContext'

const { t } = useI18n()
const ctx = inject(RiskControlKey)!
</script>
