<template>
  <div
    v-if="status"
    class="flex items-center gap-1.5 text-[9px]"
    :class="status.textClass"
    :title="status.title"
  >
    <span class="rounded px-1.5 py-0.5 font-medium" :class="status.badgeClass">
      {{ status.label }}
    </span>
    <span v-if="status.detail" class="text-gray-500 dark:text-gray-400">
      {{ status.detail }}
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { CodexQuotaOverdraftProbeState } from '@/types'

const props = defineProps<{
  state?: CodexQuotaOverdraftProbeState | null
}>()

const { t } = useI18n()

const status = computed(() => {
  const probe = props.state
  if (!probe) return null

  const attempts = Math.max(0, probe.attempts || 0)
  const limit = Math.max(1, probe.limit || 1)
  const windowLabel = probe.quota_window === 'multiple' ? '5h / 7d' : probe.quota_window
  const testedAt = probe.tested_at ? new Date(probe.tested_at).toLocaleString() : ''
  const recoverAt = probe.recover_at ? new Date(probe.recover_at).toLocaleString() : ''
  const titleParts = [windowLabel]
  if (probe.model) titleParts.push(probe.model)
  if (probe.reason_code) titleParts.push(probe.reason_code)
  if (testedAt) titleParts.push(`${t('usage.overdraftTestedAt')}: ${testedAt}`)
  if (recoverAt) titleParts.push(`${t('usage.overdraftRecoverAt')}: ${recoverAt}`)

  switch (probe.status) {
    case 'pending':
      return {
        label: t('usage.overdraftProbePending'),
        detail: `${attempts}/${limit} · ${windowLabel}`,
        title: titleParts.join(' · '),
        textClass: 'text-amber-600 dark:text-amber-400',
        badgeClass: 'bg-amber-50 dark:bg-amber-950/40'
      }
    case 'passed':
      return {
        label: t('usage.overdraftActive'),
        detail: `${attempts}/${limit} · ${windowLabel}`,
        title: titleParts.join(' · '),
        textClass: 'text-red-600 dark:text-red-400',
        badgeClass: 'bg-red-50 dark:bg-red-950/40'
      }
    case 'failed':
      return {
        label: t('usage.overdraftProbeFailed'),
        detail: `${attempts}/${limit} · ${windowLabel}`,
        title: titleParts.join(' · '),
        textClass: 'text-red-600 dark:text-red-400',
        badgeClass: 'bg-red-50 dark:bg-red-950/40'
      }
    case 'inconclusive':
      return {
        label: t('usage.overdraftProbeInconclusive'),
        detail: `${attempts}/${limit} · ${windowLabel}`,
        title: titleParts.join(' · '),
        textClass: 'text-amber-600 dark:text-amber-400',
        badgeClass: 'bg-amber-50 dark:bg-amber-950/40'
      }
    case 'recovered':
      return {
        label: t('usage.overdraftRecovered'),
        detail: windowLabel,
        title: titleParts.join(' · '),
        textClass: 'text-emerald-600 dark:text-emerald-400',
        badgeClass: 'bg-emerald-50 dark:bg-emerald-950/40'
      }
    default:
      return null
  }
})
</script>
