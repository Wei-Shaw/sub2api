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
import { ref, watch } from 'vue'
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
async function load() {
  status.value = await getMuseStatus(props.accountId)
  for (const turn of status.value.pending_turns ?? []) outcomes.value[turn.id] ??= 'cancelled'
}
async function run(action: () => Promise<void>) {
  busy.value = true
  error.value = ''
  try { await action() } catch { error.value = t('admin.accounts.muse.verificationUnavailable') }
  finally { busy.value = false }
}
const refresh = () => run(load)
const authenticate = () => run(async () => { authentication.value = await authenticateMuse(props.accountId); await load() })
const verify = () => run(async () => { await verifyMuse(props.accountId); await load() })
const renew = () => run(async () => { await renewMuse(props.accountId); await load() })
const retrySettlement = (id: string) => run(async () => { await retryMuseSettlement(id); await load() })
const resolve = (id: string) => {
  if (!confirmed.value[id] || !outcomes.value[id]) return
  return run(async () => { await resolveMuseTurn(id, outcomes.value[id]); confirmed.value[id] = false; await load() })
}
watch(() => props.accountId, () => {
  authentication.value = undefined
  outcomes.value = {}
  confirmed.value = {}
  void refresh()
}, { immediate: true })
</script>
