<template>
  <div class="space-y-3 rounded-lg border border-gray-200 p-4 dark:border-dark-600">
    <p class="text-sm font-medium">{{ t('admin.accounts.muse.status') }}</p>
    <p class="input-hint">{{ status?.state === 'session_expired' ? t('admin.accounts.muse.expired') : status?.verified ? t('admin.accounts.muse.ready') : t('admin.accounts.muse.setupNote') }}</p>
    <p v-if="authentication?.authenticated" class="input-hint">{{ t('admin.accounts.muse.authenticated') }}</p>
    <p v-if="status?.usage?.plan" class="text-sm">{{ status.usage.plan }}</p>
    <p v-if="status?.verified && !status?.usage" class="input-hint">{{ t('admin.accounts.muse.usageUnknown') }}</p>
    <ul v-if="status?.usage?.windows?.length" class="space-y-1 text-sm">
      <li v-for="window in status.usage.windows" :key="window.name">
        {{ window.name }}: {{ window.used ?? '—' }} / {{ window.limit ?? '—' }}
        <span v-if="window.resets_at"> · {{ new Date(window.resets_at).toLocaleString() }}</span>
      </li>
    </ul>
    <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
    <div class="flex flex-wrap gap-2">
      <button type="button" class="btn btn-secondary btn-sm" :disabled="busy" @click="refresh">{{ t('common.refresh') }}</button>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="busy || !status?.cookie_auth_supported || !!status?.pending_turns?.length" @click="authenticate">{{ t('admin.accounts.muse.authenticate') }}</button>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="busy || !status?.qualified_transport" @click="verify">{{ t('admin.accounts.muse.verify') }}</button>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="busy || (!status?.qualified_transport && !status?.cookie_auth_supported) || !!status?.pending_turns?.length" @click="renew">{{ t('admin.accounts.muse.renew') }}</button>
    </div>
    <div v-for="turn in status?.pending_turns ?? []" :key="turn.id" class="space-y-2 border-t border-gray-200 pt-3 dark:border-dark-600" data-testid="muse-pending-turn">
      <p class="text-sm font-medium">{{ t('admin.accounts.muse.pendingTurn') }} · {{ turn.state }}</p>
      <p class="break-all font-mono text-xs text-gray-500">{{ turn.id }}</p>
      <p class="input-hint">{{ t('admin.accounts.muse.pendingNote') }}</p>
      <button v-if="['completed', 'failed', 'cancelled', 'rejected'].includes(turn.state)" type="button" class="btn btn-secondary btn-sm" :disabled="busy" @click="retrySettlement(turn.id)">{{ t('admin.accounts.muse.retrySettlement') }}</button>
      <div v-if="turn.state === 'owner_review'" class="space-y-2">
        <label :for="`muse-outcome-${turn.id}`" class="block text-sm">{{ t('admin.accounts.muse.remoteOutcome') }}</label>
        <select :id="`muse-outcome-${turn.id}`" v-model="outcomes[turn.id]" class="input" :disabled="busy">
          <option value="cancelled">{{ t('admin.accounts.muse.cancelled') }}</option>
          <option value="failed">{{ t('admin.accounts.muse.failed') }}</option>
          <option value="completed">{{ t('admin.accounts.muse.completed') }}</option>
        </select>
        <label class="flex items-start gap-2 text-sm">
          <input v-model="confirmed[turn.id]" type="checkbox" class="mt-1" :disabled="busy" />
          {{ t('admin.accounts.muse.confirmTerminal') }}
        </label>
        <p v-if="outcomes[turn.id] === 'completed'" class="input-hint">{{ t('admin.accounts.muse.completedCharge') }}</p>
        <button type="button" class="btn btn-secondary btn-sm" :disabled="busy || !confirmed[turn.id] || !outcomes[turn.id]" @click="resolve(turn.id)">{{ t('admin.accounts.muse.resolve') }}</button>
      </div>
    </div>
  </div>
</template>
<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { getMuseStatus, verifyMuse, renewMuse, resolveMuseTurn, retryMuseSettlement, authenticateMuse, type MuseSessionCheck, type MuseStatus } from '@/api/admin/muse'
const props = defineProps<{ accountId: number }>()
const { t } = useI18n()
const busy = ref(false)
const error = ref('')
const status = ref<MuseStatus>()
const authentication = ref<MuseSessionCheck>()
const outcomes = ref<Record<string, 'completed' | 'failed' | 'cancelled'>>({})
const confirmed = ref<Record<string, boolean>>({})
// A request belongs to the account and generation at dispatch, even when the
// dialog is reused while it is in flight.
let generation = 0
type Operation = { accountId: number; generation: number }
const current = (op: Operation) => op.generation === generation && op.accountId === props.accountId
async function load(op: Operation) {
  const next = await getMuseStatus(op.accountId)
  if (!current(op)) return
  status.value = next
  for (const turn of next.pending_turns ?? []) outcomes.value[turn.id] ??= 'cancelled'
}
async function run(action: (op: Operation) => Promise<void>) {
  if (busy.value) return
  const op = { accountId: props.accountId, generation }
  busy.value = true
  error.value = ''
  try { await action(op) } catch {
    if (current(op)) error.value = t('admin.accounts.muse.verificationUnavailable')
  } finally { if (current(op)) busy.value = false }
}
const refresh = () => run(load)
const authenticate = () => run(async op => {
  const next = await authenticateMuse(op.accountId)
  if (!current(op)) return
  authentication.value = next
  await load(op)
})
const verify = () => run(async op => { await verifyMuse(op.accountId); if (current(op)) await load(op) })
const renew = () => run(async op => { await renewMuse(op.accountId); if (current(op)) await load(op) })
const ownsTurn = (id: string) => status.value?.pending_turns?.some(turn => turn.id === id)
const retrySettlement = (id: string) => {
  if (!ownsTurn(id)) return
  return run(async op => { await retryMuseSettlement(id); if (current(op)) await load(op) })
}
const resolve = (id: string) => {
  if (!ownsTurn(id) || !confirmed.value[id] || !outcomes.value[id]) return
  const outcome = outcomes.value[id]
  return run(async op => {
    await resolveMuseTurn(id, outcome)
    if (!current(op)) return
    confirmed.value[id] = false
    await load(op)
  })
}
watch(() => props.accountId, () => {
  generation++
  busy.value = false
  error.value = ''
  status.value = undefined
  authentication.value = undefined
  outcomes.value = {}
  confirmed.value = {}
  void refresh()
}, { immediate: true })
onBeforeUnmount(() => { generation++ })
</script>
