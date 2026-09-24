<template>
  <div v-if="account.platform === 'anthropic' && account.type === 'oauth'" class="space-y-1 text-xs">
    <button type="button" class="text-blue-600 disabled:opacity-50" :disabled="loading" @click="refresh">
      {{ t('admin.accounts.claudeResetCredits.query') }}
    </button>
    <p v-if="error" role="alert">{{ t('admin.accounts.claudeResetCredits.error') }}</p>
    <template v-if="status">
      <p>{{ t('admin.accounts.claudeResetCredits.count', { count: status.available_count }) }}</p>
      <p v-if="!status.eligible">{{ t('admin.accounts.claudeResetCredits.ineligible') }}</p>
      <p v-if="status.cooldown_until">{{ t('admin.accounts.claudeResetCredits.cooldown', { time: status.cooldown_until }) }}</p>
      <p v-if="status.weekly_resets_at">{{ t('admin.accounts.claudeResetCredits.weekly', { time: status.weekly_resets_at }) }}</p>
      <div v-for="credit in status.credits" :key="credit.selection_token" class="rounded border p-1">
        <p>{{ credit.label }} · {{ credit.resets_left }}</p>
        <p v-if="credit.expires_at">{{ t('admin.accounts.claudeResetCredits.expires', { time: credit.expires_at }) }}</p>
        <p>{{ t('admin.accounts.claudeResetCredits.clears', { windows: credit.clears.join(', ') }) }}</p>
        <p v-for="(percent, window) in credit.percent_used" :key="window">{{ window }}: {{ percent }}%</p>
        <p v-if="credit.blocking.length">{{ credit.blocking.join(', ') }}</p>
      </div>
      <p class="text-gray-500">{{ t('admin.accounts.claudeResetCredits.fetched', { time: status.fetched_at }) }}</p>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account } from '@/types'
import { getClaudeResetCredits, type ClaudeResetCredits } from '@/api/admin/claudeResetCredits'

const props = defineProps<{ account: Account }>()
const { t } = useI18n()
const status = ref<ClaudeResetCredits | null>(null)
const loading = ref(false)
const error = ref(false)
let generation = 0
watch(() => props.account.id, () => { generation++; status.value = null; loading.value = false; error.value = false })

async function refresh() {
  if (loading.value) return
  const current = ++generation
  loading.value = true
  error.value = false
  try {
    const result = await getClaudeResetCredits(props.account.id)
    if (current === generation) status.value = result
  } catch {
    if (current === generation) { error.value = true; status.value = null }
  } finally {
    if (current === generation) loading.value = false
  }
}
</script>
