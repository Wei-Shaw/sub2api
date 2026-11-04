<template>
  <section class="mt-6 space-y-4 border-t border-gray-200 pt-4 dark:border-dark-600" data-testid="byteplus-prepaid">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">{{ tt('title') }}</h3>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="busy" @click="refresh">{{ tt('refresh') }}</button>
    </div>
    <p class="input-hint">{{ tt('savedCredentials') }}</p>
    <p class="rounded-lg bg-amber-50 p-3 text-xs text-amber-900 dark:bg-amber-900/20 dark:text-amber-200">{{ tt('warning') }}</p>
    <p v-if="loading" role="status" class="input-hint">{{ tt('loading') }}</p>
    <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400" data-testid="prepaid-error">{{ error }}</p>

    <template v-if="status">
      <p class="text-sm font-medium" data-testid="prepaid-mode">{{ mode }}</p>
      <p v-if="!status.policies.length" class="input-hint">{{ tt('regionUnsupported') }}</p>
      <template v-if="status.pool">
        <p class="text-sm">{{ status.pool.name }}</p>
        <p class="input-hint">{{ tt('shared', { accounts: status.bound_accounts, keys: status.credentials }) }}</p>
        <p v-if="status.pool.reason" class="input-hint">{{ status.pool.reason }}</p>
        <p v-if="!status.credential_matches" role="alert" class="text-sm text-amber-700 dark:text-amber-300">{{ tt('mismatch') }}</p>
        <p v-if="status.pool.reconcile_after" class="text-sm text-amber-700 dark:text-amber-300">{{ tt('recheck') }}</p>
      </template>

      <label class="flex items-start gap-2 text-sm">
        <input v-model="confirmed" type="checkbox" data-testid="prepaid-confirmed" class="mt-1 rounded" :disabled="busy" />
        <span>{{ tt('acknowledgement') }}</span>
      </label>

      <form v-if="!status.pool?.enforced || !status.credential_matches" class="space-y-3" @submit.prevent="enroll">
        <label class="block">
          <span class="input-label">{{ tt('pool') }}</span>
          <select v-model="selectedPool" class="input" data-testid="prepaid-pool" :disabled="busy">
            <option value="">{{ tt('newPool') }}</option>
            <option v-for="pool in pools" :key="pool.id" :value="pool.id" :disabled="pool.enforced && !pool.frozen">
              {{ pool.name }}{{ pool.enforced && !pool.frozen ? ` (${tt('pauseFirst')})` : '' }}
            </option>
          </select>
        </label>
        <label v-if="!selectedPool" class="block">
          <span class="input-label">{{ tt('poolName') }}</span>
          <input v-model="poolName" class="input" maxlength="100" data-testid="prepaid-name" :disabled="busy" />
        </label>
        <button type="submit" class="btn btn-primary" data-testid="prepaid-enable" :disabled="busy || !confirmed || selectedPoolIsActive || !status.policies.length">
          {{ status.pool && !status.credential_matches ? tt('link') : tt('enable') }}
        </button>
      </form>

      <template v-if="status.pool?.enforced">
        <div class="flex flex-wrap gap-2">
          <button v-if="!status.pool.frozen" type="button" class="btn btn-secondary" data-testid="prepaid-freeze" :disabled="busy" @click="act('freeze')">{{ tt('pause') }}</button>
          <button v-else type="button" class="btn btn-primary" data-testid="prepaid-resume" :disabled="busy || !confirmed || status.pool.safety_lock || !status.credential_matches || !hasEligibleFunding || needsVerification" @click="act('resume', { confirmed: true })">{{ tt('resume') }}</button>
        </div>

        <form v-if="status.pool.frozen && status.credential_matches" class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600" data-testid="prepaid-grant-form" @submit.prevent="addGrant">
          <h4 class="text-sm font-semibold">{{ tt('addPack') }}</h4>
          <label class="block"><span class="input-label">{{ tt('packId') }}</span><input v-model="packId" class="input" required maxlength="160" data-testid="prepaid-pack-id" :disabled="busy" /></label>
          <p class="input-hint">{{ tt('packIdHint') }}</p>
          <label class="block"><span class="input-label">{{ tt('model') }}</span><select v-model="model" class="input" required data-testid="prepaid-model" :disabled="busy"><option value="" disabled>{{ tt('model') }}</option><option v-for="policy in status.policies" :key="policy.model" :value="policy.model">{{ policy.name }}</option></select></label>
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <label class="block"><span class="input-label">{{ tt('remaining') }}</span><input v-model="balance" class="input" inputmode="decimal" required data-testid="prepaid-balance" :disabled="busy" /></label>
            <label class="block"><span class="input-label">{{ tt('units') }}</span><select v-model="scale" class="input" data-testid="prepaid-unit" :disabled="busy"><option :value="0">{{ tt('tokens') }}</option><option :value="3">{{ tt('ktokens') }}</option><option :value="6">{{ tt('mtokens') }}</option></select></label>
          </div>
          <label class="block"><span class="input-label">{{ tt('expiry') }}</span><input v-model="expiry" class="input" type="datetime-local" step="1" required data-testid="prepaid-expiry" :disabled="busy" /></label>
          <p v-if="model" class="input-hint">{{ tt('window', { minutes: selectedValidityMinutes }) }}</p>
          <button type="submit" class="btn btn-primary" data-testid="prepaid-add" :disabled="busy || !confirmed || !model">{{ tt('addPack') }}</button>
        </form>
      </template>

      <div v-if="status.pool" class="space-y-2">
        <h4 class="text-sm font-semibold">{{ tt('packs') }}</h4>
        <p v-if="!status.grants.length" class="input-hint">{{ tt('noPacks') }}</p>
        <div v-else class="overflow-x-auto">
          <table class="w-full text-left text-xs" data-testid="prepaid-grants">
            <thead><tr><th class="p-2">{{ tt('packId') }}</th><th class="p-2">{{ tt('model') }}</th><th class="p-2">{{ tt('available') }}</th><th class="p-2">{{ tt('held') }}</th><th class="p-2">{{ tt('spent') }}</th><th class="p-2">{{ tt('expiry') }}</th><th class="p-2">{{ tt('expiryState') }}</th><th /></tr></thead>
            <tbody><tr v-for="grant in status.grants" :key="grant.id" class="border-t border-gray-100 dark:border-dark-600">
              <td class="p-2 font-mono">{{ grant.id }}</td><td class="p-2">{{ policyName(grant.model) }}</td><td class="p-2">{{ grant.available_tokens }}</td><td class="p-2">{{ grant.reserved_tokens }}</td><td class="p-2">{{ grant.spent_tokens }}</td><td class="p-2">{{ localTime(grant.expires_at) }}</td><td class="p-2">{{ grant.needs_verification ? tt('recheck') : grant.eligible ? tt('eligible') : tt('ineligible') }}</td>
              <td class="p-2"><button v-if="status.pool.frozen && status.open_jobs === 0 && status.credential_matches" type="button" class="btn btn-secondary btn-sm" :disabled="busy" :data-testid="`prepaid-verify-${grant.id}`" @click="startVerification(grant)">{{ tt('verify') }}</button></td>
            </tr></tbody>
          </table>
        </div>
        <form v-if="verifyPack" class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600" data-testid="prepaid-verify-form" @submit.prevent="verifyGrant">
          <h4 class="text-sm font-semibold">{{ tt('verify') }}: {{ verifyPack }}</h4>
          <p class="input-hint">{{ tt('verifyHint') }}</p>
          <label class="block"><span class="input-label">{{ tt('remaining') }} ({{ tt('tokens') }})</span><input v-model="verifyBalance" inputmode="decimal" class="input" required :disabled="busy" data-testid="prepaid-verify-balance" /></label>
          <label class="block"><span class="input-label">{{ tt('expiry') }}</span><input v-model="verifyExpiry" type="datetime-local" step="1" class="input" required :disabled="busy" data-testid="prepaid-verify-expiry" /></label>
          <div class="flex gap-2"><button type="submit" class="btn btn-primary" :disabled="busy || !confirmed">{{ tt('saveVerification') }}</button><button type="button" class="btn btn-secondary" :disabled="busy" @click="verifyPack = ''">{{ tt('cancel') }}</button></div>
        </form>

        <h4 class="pt-2 text-sm font-semibold">{{ tt('jobs', { count: status.open_jobs }) }}</h4>
        <p class="input-hint">{{ tt('deadlineHint') }}</p>
        <p v-if="!status.jobs.length" class="input-hint">{{ tt('noJobs') }}</p>
        <div v-else class="overflow-x-auto"><table class="w-full text-left text-xs" data-testid="prepaid-jobs">
          <thead><tr><th class="p-2">{{ tt('job') }}</th><th class="p-2">{{ tt('state') }}</th><th class="p-2">{{ tt('held') }}</th><th class="p-2">{{ tt('spent') }}</th><th class="p-2">{{ tt('deadline') }}</th><th /></tr></thead>
          <tbody><tr v-for="job in status.jobs" :key="job.id" class="border-t border-gray-100 dark:border-dark-600">
            <td class="p-2 font-mono">{{ job.task_id || job.id }}</td><td class="p-2">{{ jobState(job.state) }}</td><td class="p-2" :title="`${tt('ceiling')}: ${job.ceiling_tokens}`">{{ job.reserved_tokens }}</td><td class="p-2">{{ job.actual_tokens ?? '—' }}</td><td class="p-2">{{ localTime(job.valid_until) }}</td>
            <td class="p-2"><button v-if="job.task_id && ['reserved', 'pending', 'uncertain'].includes(job.state)" type="button" class="btn btn-secondary btn-sm" :disabled="busy || !status.credential_matches" :data-testid="`prepaid-reconcile-${job.id}`" @click="reconcileJob(job.id)">{{ tt('reconcile') }}</button><span v-else-if="!job.task_id && job.state !== 'succeeded' && job.state !== 'failed'" class="input-hint">{{ tt('missingTask') }}</span></td>
          </tr></tbody>
        </table><p class="input-hint">{{ tt('allRecent') }}</p></div>

        <details v-if="status.pool.enforced" class="rounded-lg border border-red-200 p-3 dark:border-red-900/50">
          <summary class="cursor-pointer text-sm font-medium text-red-700 dark:text-red-300">{{ tt('disableTitle') }}</summary>
          <p class="my-2 text-xs">{{ tt('disableWarning') }}</p>
          <label class="my-3 flex items-start gap-2 text-sm"><input v-model="allowPayg" type="checkbox" class="mt-1 rounded" data-testid="prepaid-allow-payg" :disabled="busy" /><span>{{ tt('allowPayg') }}</span></label>
          <button type="button" class="btn btn-secondary text-red-700 dark:text-red-300" data-testid="prepaid-disable" :disabled="busy || !allowPayg || status.open_jobs > 0 || status.pool.safety_lock" @click="act('disable', { allow_pay_as_you_go: true })">{{ tt('disable') }}</button>
        </details>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import * as funding from '@/api/admin/byteplusPrepaid'
import type { PrepaidAction, PrepaidGrant, PrepaidInput, PrepaidPool, PrepaidStatus } from '@/api/admin/byteplusPrepaid'
import { packTokens } from '@/utils/byteplusPrepaid'
import { formatDateTime } from '@/utils/format'

const props = defineProps<{ accountId: number; accountName?: string }>()
const { t } = useI18n()
const localTime = (value: string) => formatDateTime(value, { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false, timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone, timeZoneName: 'short' })
const tt = (key: string, values: Record<string, string | number> = {}) => t(`admin.byteplusPrepaid.${key}`, values)
const status = ref<PrepaidStatus | null>(null)
const pools = ref<PrepaidPool[]>([])
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const confirmed = ref(false)
const allowPayg = ref(false)
const poolName = ref('')
const selectedPool = ref('')
const packId = ref('')
const model = ref('')
const balance = ref('')
const scale = ref<0 | 3 | 6>(0)
const expiry = ref('')
const verifyPack = ref('')
const verifyBalance = ref('')
const verifyExpiry = ref('')
let generation = 0
const mode = computed(() => {
  const pool = status.value?.pool
  if (!pool) return tt('payg')
  if (!pool.enforced) return tt('disabled')
  return pool.safety_lock ? tt('locked') : pool.frozen ? tt('paused') : tt('active')
})
const hasEligibleFunding = computed(() => status.value?.grants.some(g => g.eligible) ?? false)
const needsVerification = computed(() => status.value?.grants.some(g => g.needs_verification) ?? false)
const selectedPoolIsActive = computed(() => pools.value.some(p => p.id === selectedPool.value && p.enforced && !p.frozen))
const selectedValidityMinutes = computed(() => (status.value?.policies.find(p => p.model === model.value)?.minimum_validity_seconds ?? 0) / 60)
const policyName = (id: string) => status.value?.policies.find(p => p.model === id)?.name ?? id
const stateKeys: Record<string, string> = { reserved: 'stateReserved', pending: 'statePending', uncertain: 'stateUncertain', succeeded: 'stateSucceeded', failed: 'stateFailed' }
const jobState = (state: string) => tt(stateKeys[state] ?? 'stateUncertain')
const message = (e: unknown) => e && typeof e === 'object' && 'message' in e && typeof e.message === 'string' ? e.message : tt('failed')

async function refresh() {
  const id = props.accountId
  const current = ++generation
  loading.value = true
  error.value = ''
  try {
    const [data, availablePools] = await Promise.all([funding.getStatus(id), funding.listPools(id)])
    if (current !== generation || id !== props.accountId) return
    status.value = data
    pools.value = availablePools
    selectedPool.value = data.pool?.id ?? ''
  } catch (e) { if (current === generation) error.value = message(e) }
  finally { if (current === generation) loading.value = false }
}

async function perform(task: (id: number) => Promise<PrepaidStatus>) {
  if (busy.value || loading.value) return false
  const id = props.accountId
  const current = ++generation
  busy.value = true
  error.value = ''
  try {
    const data = await task(id)
    if (current !== generation || id !== props.accountId) return false
    status.value = data
    confirmed.value = false
    allowPayg.value = false
    selectedPool.value = data.pool?.id ?? ''
    // A failed follow-up read must not make a committed credit look failed.
    if (data.pool) pools.value = [data.pool, ...pools.value.filter(p => p.id !== data.pool?.id)]
    return true
  } catch (e) { if (current === generation) error.value = message(e); return false }
  finally { if (current === generation) busy.value = false }
}
async function act(action: PrepaidAction, input: PrepaidInput = {}) {
  if (action === 'resume' && !confirmed.value) return false
  if (action === 'disable' && !allowPayg.value) return false
  return perform(id => funding.update(id, action, input))
}
async function enroll() {
  if (!confirmed.value) return
  await act(status.value?.pool && !status.value.credential_matches ? 'link' : 'enable', { confirmed: true, pool_id: selectedPool.value, name: poolName.value })
}
async function addGrant() {
  if (!confirmed.value || !model.value) return
  try {
    const ok = await act('grant', { confirmed: true, pack_id: packId.value, model: model.value, remaining_tokens: packTokens(balance.value, scale.value), expires_at: new Date(expiry.value).toISOString() })
    if (ok) { packId.value = ''; balance.value = ''; expiry.value = '' }
  } catch { error.value = tt('invalidAmount') }
}
function localInput(iso: string) {
  const d = new Date(iso)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}
function startVerification(grant: PrepaidGrant) {
  verifyPack.value = grant.id
  verifyBalance.value = grant.available_tokens
  verifyExpiry.value = localInput(grant.expires_at)
  confirmed.value = false
}
async function verifyGrant() {
  if (!confirmed.value) return
  try {
    const ok = await act('verify-grant', { confirmed: true, pack_id: verifyPack.value, remaining_tokens: packTokens(verifyBalance.value, 0), expires_at: new Date(verifyExpiry.value).toISOString() })
    if (ok) verifyPack.value = ''
  } catch { error.value = tt('invalidAmount') }
}
async function reconcileJob(id: string) { await perform(account => funding.reconcile(account, id)) }

watch(() => props.accountId, () => {
  status.value = null
  pools.value = []
  confirmed.value = false
  allowPayg.value = false
  busy.value = false
  poolName.value = (props.accountName ?? '').slice(0, 100)
  selectedPool.value = ''
  model.value = ''
  packId.value = ''
  balance.value = ''
  expiry.value = ''
  verifyPack.value = ''
  void refresh()
}, { immediate: true })
onBeforeUnmount(() => { generation++ })
</script>
