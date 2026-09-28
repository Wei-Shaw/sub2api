<template>
  <div v-if="account.platform === 'anthropic' && account.type === 'oauth'" class="space-y-1">
    <div class="flex flex-wrap items-center gap-1.5">
      <slot name="pre-actions" />
      <button type="button" data-testid="claude-reset-count" :class="[actionClass, 'text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/30']" :disabled="loading || resetting" :title="status ? t('admin.accounts.claudeResetCredits.fetched', { time: formatExpiry(status.fetched_at, true) }) : t('admin.accounts.claudeResetCredits.query')" @click="refresh">
        <svg class="h-2.5 w-2.5" :class="{ 'animate-spin': loading }" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" /></svg>
        {{ t('admin.accounts.openaiQuotaReset.count') }}<span v-if="status"> {{ status.available_count }}</span>
      </button>
      <button type="button" data-testid="claude-reset-action" :class="[actionClass, 'text-orange-600 hover:bg-orange-50 dark:text-orange-400 dark:hover:bg-orange-900/30']" :disabled="!canReset" :title="t('admin.accounts.claudeResetCredits.confirm')" @click="openConfirm">
        <svg class="h-2.5 w-2.5" :class="{ 'animate-spin': resetting }" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 12a8 8 0 11-2.343-5.657L20 8m0 0V4m0 4h-4" /></svg>
        {{ t('admin.accounts.openaiQuotaReset.reset') }}
      </button>
    </div>
    <div v-if="expirations.length" class="space-y-1">
      <div class="flex flex-wrap items-center gap-1">
        <span data-testid="claude-reset-expiry" class="inline-flex max-w-full items-center rounded bg-gray-100 px-1.5 py-0.5 text-[10px] leading-4 text-gray-600 tabular-nums dark:bg-dark-800 dark:text-gray-300" :title="t('admin.accounts.openaiQuotaReset.expiresAtFull', { time: formatExpiry(expirations[0]!, true) })">{{ t('admin.accounts.openaiQuotaReset.expiresAt', { time: formatExpiry(expirations[0]!) }) }}</span>
        <button v-if="expirations.length > 1" type="button" data-testid="claude-reset-expiry-toggle" class="rounded-full bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-600 hover:bg-gray-200 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700" :aria-expanded="showDetails" :aria-label="t('admin.accounts.openaiQuotaReset.expirationDetails')" @click="showDetails = !showDetails">+{{ expirations.length - 1 }}</button>
      </div>
      <div v-if="showDetails && expirations.length > 1" data-testid="claude-reset-expiry-details" class="inline-grid max-w-full gap-0.5 rounded border border-gray-200 bg-white px-1.5 py-1 text-[10px] leading-4 text-gray-600 shadow-sm dark:border-dark-700 dark:bg-dark-900 dark:text-gray-300">
        <span v-for="(expiresAt, index) in expirations" :key="`${expiresAt}-${index}`" class="tabular-nums" :title="formatExpiry(expiresAt, true)">{{ formatExpiry(expiresAt) }}</span>
      </div>
    </div>
    <p v-if="error" role="alert" class="text-[10px] text-red-600 dark:text-red-400">{{ t('admin.accounts.claudeResetCredits.error') }}</p>
    <p v-else-if="status && !status.eligible" class="text-[10px] text-gray-500 dark:text-gray-400">{{ t('admin.accounts.claudeResetCredits.ineligible') }}</p>
    <p v-else-if="status?.cooldown_until" class="text-[10px] text-amber-600 dark:text-amber-400">{{ t('admin.accounts.claudeResetCredits.cooldown', { time: formatExpiry(status.cooldown_until) }) }}</p>
    <p v-if="outcome" role="status" class="text-[10px]" :class="outcome === 'reset' ? 'text-emerald-600 dark:text-emerald-400' : 'text-amber-600 dark:text-amber-400'">{{ t(`admin.accounts.claudeResetCredits.outcomes.${outcome}`) }}</p>
    <button v-if="operation && outcome === 'unknown'" type="button" :class="[actionClass, 'text-amber-600 dark:text-amber-400']" :disabled="resetting" @click="redeem">{{ t('admin.accounts.claudeResetCredits.retry') }}</button>
    <ConfirmDialog :show="!!selected" :title="t('admin.accounts.claudeResetCredits.redeem')" :message="t('admin.accounts.claudeResetCredits.confirm')" danger @confirm="confirm" @cancel="selected = null" />
    <TotpStepUpDialog v-if="selected || resetting" :controller="stepUp" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
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
const actionClass = 'inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50'
const showDetails = ref(false)
// Only the server-designated native next grant is redeemable. Expiry sorting
// affects the display only and must never select a different grant to consume.
const nextCredit = computed(() => status.value?.credits.find(credit => credit.redeemable))
const canReset = computed(() => !!status.value?.eligible && !!nextCredit.value && !loading.value && !resetting.value && !operation.value)
const expirations = computed(() => (status.value?.credits ?? [])
  .filter(credit => credit.resets_left > 0 && credit.expires_at && Number.isFinite(Date.parse(credit.expires_at)))
  .map(credit => credit.expires_at!)
  .sort((a, b) => Date.parse(a) - Date.parse(b)))
function formatExpiry(value: string, full = false): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return new Intl.DateTimeFormat(undefined, {
    ...(full ? { year: 'numeric' as const } : {}), month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit'
  }).format(date)
}
function openConfirm() {
  if (canReset.value && nextCredit.value) selected.value = nextCredit.value.selection_token
}
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
watch(() => props.account.id, () => { generation++; showDetails.value = false; selected.value = null; operation.value = operations.get(props.account.id); outcome.value = operation.value ? 'unknown' : null; resetting.value = false; status.value = null; loading.value = false; error.value = false })

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
