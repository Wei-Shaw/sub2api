<template>
  <div class="min-w-[220px] max-w-xl space-y-2 text-xs" data-testid="byteplus-usage">
    <div class="flex flex-wrap items-center gap-2">
      <span class="font-medium">{{ tt('localTitle') }}</span>
      <button type="button" class="text-primary-600 hover:underline disabled:opacity-50" :disabled="loading" @click="refresh">{{ tt('localRefresh') }}</button>
      <button type="button" class="text-primary-600 hover:underline disabled:opacity-50" :disabled="querying || loading" @click="query">{{ tt('providerQuery') }}</button>
      <button type="button" class="text-primary-600 hover:underline" :aria-expanded="expanded" @click="expanded = !expanded">{{ tt('details') }}</button>
    </div>
    <p v-if="loading" role="status">{{ tt('loading') }}</p>
    <p v-if="error" role="alert" class="text-amber-700 dark:text-amber-300">{{ tt('stale') }} {{ error }}</p>
    <template v-if="status">
      <p v-if="status.pool" class="text-gray-500">{{ status.pool.name }} · {{ tt('shared', { accounts: status.bound_accounts, keys: status.credentials }) }}</p>
      <p v-else>{{ tt('payg') }}</p>
      <p v-if="status.pool && !status.credential_matches" class="text-amber-700">{{ tt('mismatch') }}</p>
      <p v-if="status.pool?.reason" class="text-amber-700 dark:text-amber-300">{{ status.pool.reason }}</p>
      <div v-for="model in summaries" :key="model.model" class="space-y-0.5" data-testid="byteplus-model">
        <p class="font-medium">{{ policyName(model.model) }}</p>
        <p>{{ tt('spendable') }}: {{ model.spendable }} · {{ tt('held') }}: {{ model.held }}</p>
        <p class="text-gray-500">{{ tt('nextExpiry') }}: {{ date(model.expiry) }}</p>
      </div>
      <p v-if="status.observed_at" class="text-gray-500">{{ tt('localObserved') }}: {{ date(status.observed_at) }}</p>
    </template>
    <p v-if="billing && !billing.configured" class="text-amber-700 dark:text-amber-300" data-testid="byteplus-setup">{{ tt('setupTitle') }}</p>
    <p v-if="queryError" role="alert" class="text-amber-700 dark:text-amber-300">{{ tt('providerFailed') }}</p>
    <div v-if="expanded" class="space-y-3 rounded border border-gray-200 p-3 dark:border-dark-600">
      <p class="text-gray-500">{{ tt('separation') }}</p>
      <div v-if="billing && !billing.configured" class="space-y-2">
        <p>{{ tt(billing.setup === 'invalid_configuration' ? 'invalidConfig' : billing.setup === 'pool_required' ? 'poolSetup' : 'setupHelp') }}</p>
        <p>{{ tt('setupFile') }}</p>
        <a class="text-primary-600 hover:underline" href="https://github.com/byteplus-sdk/byteplus-go-sdk-v2/blob/main/docs/1-Credentials.md" target="_blank" rel="noopener noreferrer">{{ tt('credentialDocs') }}</a>
      </div>
      <div v-if="status?.grants.length" class="overflow-x-auto">
        <table class="w-full text-left text-xs">
          <caption class="pb-2 text-left font-medium">{{ tt('packs') }}</caption>
          <thead><tr><th class="p-1">{{ tt('packId') }}</th><th class="p-1">{{ tt('available') }}</th><th class="p-1">{{ tt('held') }}</th><th class="p-1">{{ tt('spent') }}</th><th class="p-1">{{ tt('expiry') }}</th><th class="p-1">{{ tt('recordedVerification') }}</th><th class="p-1">{{ tt('state') }}</th></tr></thead>
          <tbody><tr v-for="grant in status.grants" :key="grant.id"><td class="p-1">{{ grant.id }}<br>{{ policyName(grant.model) }}</td><td class="p-1">{{ grant.available_tokens }}</td><td class="p-1">{{ grant.reserved_tokens }}</td><td class="p-1">{{ grant.spent_tokens }}</td><td class="p-1">{{ date(grant.expires_at) }}</td><td class="p-1">{{ date(grant.verified_at) }}</td><td class="p-1">{{ tt(`reasons.${reason(grant)}`) }}</td></tr></tbody>
        </table>
      </div>
      <p v-if="status">{{ tt('jobs', { count: status.open_jobs }) }}</p>
      <template v-if="billing?.configured">
        <section v-for="name in sourceNames" :key="name" class="space-y-1" :data-testid="`byteplus-provider-${name}`">
          <h4 class="font-medium">{{ tt(`sources.${name}`) }}</h4>
          <p :class="billing[name].state !== 'ok' ? 'text-amber-700 dark:text-amber-300' : ''">{{ tt(`queryStates.${billing[name].state}`) }}</p>
          <p class="text-gray-500">{{ tt('providerObserved') }}: {{ date(billing[name].observed_at) }} · {{ tt('attempted') }}: {{ date(billing[name].attempted_at) }}</p>
          <template v-if="name === 'balance' && billing.balance.data">
            <p>{{ tt('providerAvailable') }}: {{ value(billing.balance.data, 'AvailableBalance') }} {{ value(billing.balance.data, 'Currency') }}</p>
            <p>{{ tt('cash') }}: {{ value(billing.balance.data, 'CashBalance') }} · {{ tt('creditLimit') }}: {{ value(billing.balance.data, 'CreditLimit') }}</p>
          </template>
          <template v-else>
            <div v-for="row in rows(billing[name].data)" :key="String(row.InstanceNo ?? row.CouponID)" class="border-t border-gray-100 py-1 dark:border-dark-600">
              <p>{{ row.InstanceNo ?? row.CouponID }} · {{ row.ConfigurationName ?? row.CouponName }}</p>
              <p>{{ tt('providerRemaining') }}: {{ row.AvailableAmount ?? row.RemainingAmount }} {{ row.Unit ?? '' }} · {{ tt('providerExpiry') }}: {{ date(String(row.ExpiryTime ?? row.ExpiredTime ?? '')) }}</p>
              <template v-if="name === 'packages' && billing.packages.state === 'ok'">
                <p v-if="comparison(row).kind === 'unmatched'" class="text-gray-500">{{ tt('comparisonUnmatched') }}</p>
                <template v-else>
                  <p>{{ tt('comparisonLocal', { model: comparison(row).model, remaining: comparison(row).remaining, held: comparison(row).held }) }}</p>
                  <p :class="comparison(row).kind === 'different' ? 'text-amber-700 dark:text-amber-300' : 'text-gray-500'">{{ tt(`comparisonStates.${comparison(row).kind}`) }}</p>
                </template>
              </template>
              <p v-if="row.Status != null">{{ tt('state') }}: {{ row.Status }}</p>
              <p v-if="name === 'coupons'">{{ tt('couponRestrictions') }}: {{ restrictions(row) }}</p>
            </div>
            <p v-if="billing[name].state === 'ok' && rows(billing[name].data).length === 0">{{ tt('providerEmpty') }}</p>
          </template>
        </section>
        <p class="text-gray-500">{{ tt('providerCaveat') }}</p>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import * as funding from '@/api/admin/byteplusPrepaid'
import type { BillingStatus, PrepaidGrant, PrepaidStatus } from '@/api/admin/byteplusPrepaid'
const props = withDefaults(defineProps<{ accountId: number; refreshToken?: number }>(), { refreshToken: 0 })
const { t } = useI18n()
const tt = (key: string, values: Record<string, string | number> = {}) => t(`admin.byteplusPrepaid.${key}`, values)
const status = ref<PrepaidStatus | null>(null)
const billing = ref<BillingStatus | null>(null)
const expanded = ref(false)
const loading = ref(false)
const querying = ref(false)
const error = ref('')
const queryError = ref(false)
const elapsed = ref(0)
let generation = 0
let queryGeneration = 0
let timer: ReturnType<typeof setTimeout> | undefined
let snapshotReceivedAt = 0
const sourceNames = ['balance', 'packages', 'coupons'] as const
const date = (iso?: string | null) => iso ? new Date(iso).toLocaleString(undefined, { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', timeZoneName: 'short' }) : '—'
const policyName = (model: string) => status.value?.policies.find(p => p.model === model)?.name ?? model
const units = (s: string) => { const [a, b = ''] = s.split('.'); return BigInt(a) * 1000000n + BigInt(b.padEnd(6, '0')) }
const tokens = (n: bigint) => { const whole = n / 1000000n; const frac = (n % 1000000n).toString().padStart(6, '0').replace(/0+$/, ''); return frac ? `${whole}.${frac}` : String(whole) }
function reason(g: PrepaidGrant) {
  const original = g.eligibility_reason ?? (g.eligible ? 'eligible' : 'unsupported')
  if (original !== 'eligible') return original
  const now = new Date(status.value?.observed_at ?? '').getTime() + elapsed.value
  if (new Date(g.expires_at).getTime() <= now) return 'expired'
  const margin = (status.value?.policies.find(p => p.model === g.model)?.minimum_validity_seconds ?? 0) * 1000
  return new Date(g.expires_at).getTime() < now + margin ? 'too_close_to_expiry' : original
}
const summaries = computed(() => (status.value?.policies ?? []).map(p => {
  const grants = status.value?.grants.filter(g => g.model === p.model) ?? []
  const eligible = grants.filter(g => reason(g) === 'eligible')
  return { model: p.model, spendable: tokens(eligible.reduce((n, g) => n + units(g.available_tokens), 0n)), held: tokens(grants.reduce((n, g) => n + units(g.reserved_tokens), 0n)), expiry: eligible.map(g => g.expires_at).sort((a, b) => Date.parse(a) - Date.parse(b))[0] }
}))
function scheduleBoundary() {
  clearTimeout(timer)
  const observed = Date.parse(status.value?.observed_at ?? '')
  if (!Number.isFinite(observed)) return
  const boundaries = status.value?.grants.flatMap(g => {
    const margin = (status.value?.policies.find(p => p.model === g.model)?.minimum_validity_seconds ?? 0) * 1000
    return [Date.parse(g.expires_at) - margin, Date.parse(g.expires_at)]
  }).filter(n => n >= observed + elapsed.value) ?? []
  if (!boundaries.length) return
  const delay = Math.min(...boundaries) - observed - elapsed.value + 1
  timer = setTimeout(() => { elapsed.value = Date.now() - snapshotReceivedAt; scheduleBoundary() }, Math.min(delay, 2147483647))
}
async function refresh() {
  const id = props.accountId; const current = ++generation; const providerGeneration = queryGeneration; const requestStarted = Date.now()
  loading.value = true; error.value = ''; queryError.value = false
  const results = await Promise.allSettled([funding.getStatus(id), funding.providerStatus(id)])
  if (current !== generation || id !== props.accountId) return
  if (results[0].status === 'fulfilled') { status.value = results[0].value; elapsed.value = Date.now() - requestStarted; snapshotReceivedAt = requestStarted; scheduleBoundary() }
  else error.value = tt('localFailed')
  if (providerGeneration === queryGeneration && !querying.value) {
    if (results[1].status === 'fulfilled') billing.value = results[1].value
    else queryError.value = true
  }
  loading.value = false
}
async function query() {
  if (querying.value) return
  const id = props.accountId; const current = ++queryGeneration
  querying.value = true; queryError.value = false; expanded.value = true
  try { const result = await funding.queryProvider(id); if (current === queryGeneration && id === props.accountId) billing.value = result }
  catch { if (current === queryGeneration) queryError.value = true }
  finally { if (current === queryGeneration) querying.value = false }
}
// Compare an exact pack identity in token units; never infer prices or credit.
function comparison(row: Record<string, unknown>) {
  const id = String(row.InstanceNo ?? '').replace(/^ModelArk_resource_packages(?=\d+$)/, '')
  const grant = status.value?.grants.find(g => g.id.replace(/^ModelArk_resource_packages(?=\d+$)/, '') === id)
  if (!grant) return { kind: 'unmatched', model: '', remaining: '', held: '' }
  const remaining = tokens(units(grant.available_tokens) + units(grant.reserved_tokens))
  const base = { model: policyName(grant.model), remaining, held: grant.reserved_tokens }
  if (!['token', 'tokens'].includes(String(row.Unit ?? '').trim().toLowerCase())) return { ...base, kind: 'unverifiedUnit' }
  const normalize = (value: string) => {
    if (!/^\d+(?:\.\d+)?$/.test(value)) return null
    const [whole, fraction = ''] = value.split('.')
    const a = whole.replace(/^0+(?=\d)/, '')
    const b = fraction.replace(/0+$/, '')
    return b ? `${a}.${b}` : a
  }
  const amount = typeof row.AvailableAmount === 'string' ? normalize(row.AvailableAmount) : null
  const expiry = typeof row.ExpiryTime === 'string' ? Date.parse(row.ExpiryTime) : NaN
  return { ...base, kind: amount === normalize(remaining) && Number.isFinite(expiry) && expiry === Date.parse(grant.expires_at) ? 'matches' : 'different' }
}
const rows = (data: unknown): Record<string, unknown>[] => Array.isArray(data) ? data : []
const value = (data: unknown, key: string) => data && !Array.isArray(data) && typeof data === 'object' ? (data as Record<string, unknown>)[key] ?? '—' : '—'
const restrictions = (row: Record<string, unknown>) => [row.OrderTypeLimit, row.PayTypeLimit, row.AmountLimit != null ? `${tt('amountLimit')}: ${row.AmountLimit}` : '', row.UsageLimit != null ? `${tt('usageLimit')}: ${row.UsageLimit}` : '', ...rows(row.ProductLimitList).map(p => `${p.ProductName ?? p.ProductCode ?? ''} ${p.BillingMethod ?? ''} ${p.ConfigurationName ?? p.ConfigurationCode ?? ''} ${p.ChargeItemCode ?? ''} ${p.TimeLimitLower ?? ''}…${p.TimeLimitUpper ?? ''}`)].filter(Boolean).join(' · ') || tt('restrictionUnknown')
watch(() => props.accountId, () => { generation++; queryGeneration++; querying.value = false; status.value = null; billing.value = null; queryError.value = false; clearTimeout(timer); void refresh() }, { immediate: true })
watch(() => props.refreshToken, () => { if (!document.hidden) void refresh() })
onBeforeUnmount(() => { generation++; queryGeneration++; clearTimeout(timer) })
</script>
