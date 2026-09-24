<template>
  <div v-if="account.platform === 'anthropic' && account.type === 'oauth'" class="space-y-1 text-xs">
    <button type="button" class="text-blue-600 disabled:opacity-50" :disabled="loading || resetting" @click="refresh">
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
        <button type="button" :disabled="loading || resetting || !credit.redeemable || !status.eligible || !!operation" class="text-orange-600 disabled:opacity-50" @click="selected = credit.selection_token">
          {{ t('admin.accounts.claudeResetCredits.redeem') }}
        </button>
        <p v-if="credit.blocking.length">{{ credit.blocking.join(', ') }}</p>
      </div>
      <p class="text-gray-500">{{ t('admin.accounts.claudeResetCredits.fetched', { time: status.fetched_at }) }}</p>
    </template>
    <p v-if="outcome" role="status">{{ t(`admin.accounts.claudeResetCredits.outcomes.${outcome}`) }}</p>
    <button v-if="operation && outcome === 'unknown'" type="button" :disabled="resetting" @click="redeem">
      {{ t('admin.accounts.claudeResetCredits.retry') }}
    </button>
    <ConfirmDialog :show="!!selected" :title="t('admin.accounts.claudeResetCredits.redeem')" :message="t('admin.accounts.claudeResetCredits.confirm')" danger @confirm="confirm" @cancel="selected = null" />
    <TotpStepUpDialog :controller="stepUp" />
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account } from '@/types'
import { getClaudeResetCredits, redeemClaudeResetCredit, type ClaudeResetCredits, type ClaudeResetResult } from '@/api/admin/claudeResetCredits'

import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import { useStepUp, isStepUpCancelled, isStepUpBlocked } from '@/composables/useStepUp'

const props = defineProps<{ account: Account }>()
const { t } = useI18n()
const status = ref<ClaudeResetCredits | null>(null)
const loading = ref(false)
const error = ref(false)
const stepUp = useStepUp()
const selected = ref<string | null>(null)
const resetting = ref(false)
const outcome = ref<ClaudeResetResult['outcome'] | null>(null)
// Keep one operation per account across component remounts. An uncertain request
// must never silently turn into a fresh redemption of the next credit.
const operations = pendingOperations
const operation = ref(operations.get(props.account.id))
if (operation.value) outcome.value = 'unknown'
let generation = 0
watch(() => props.account.id, () => { generation++; selected.value = null; operation.value = operations.get(props.account.id); outcome.value = operation.value ? 'unknown' : null; resetting.value = false; status.value = null; loading.value = false; error.value = false })

function confirm() {
  if (!selected.value || resetting.value || operation.value) return
  operation.value = { token: selected.value, key: crypto.randomUUID() }
  operations.set(props.account.id, operation.value)
  selected.value = null
  void redeem()
}
async function redeem() {
  const op = operation.value
  if (!op || resetting.value) return
  const id = props.account.id
  const current = generation
  resetting.value = true
  try {
    const result = await stepUp.run(() => redeemClaudeResetCredit(id, op.token, op.key))
    if (result.outcome !== 'unknown') operations.delete(id)
    if (current !== generation) return
    outcome.value = result.outcome
    if (result.credits) status.value = result.credits
    else status.value = null
    operation.value = operations.get(id)
  } catch (err) {
    if (isStepUpCancelled(err) || isStepUpBlocked(err)) operations.delete(id)
    if (current !== generation) return
    operation.value = operations.get(id)
    outcome.value = operation.value ? 'unknown' : null
  } finally {
    if (current === generation) resetting.value = false
  }
}

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

<script lang="ts">
const pendingOperations = new Map<number, { token: string; key: string }>()
</script>
