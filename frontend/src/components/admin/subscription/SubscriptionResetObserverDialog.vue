<template>
  <BaseDialog :show="show" :title="t(`${prefix}.title`, { group: groupName })" width="wide" :close-on-escape="!saving" :show-close-button="!saving" @close="emit('close')">
    <div class="space-y-5">
      <p class="rounded-xl bg-blue-50 p-4 text-sm leading-6 text-blue-800 dark:bg-blue-900/20 dark:text-blue-200">
        {{ t(`${prefix}.observationOnly`) }}
      </p>
      <div v-if="loading" class="flex justify-center py-10" role="status">
        <LoadingSpinner /><span class="sr-only">{{ t('common.loading') }}</span>
      </div>
      <div v-else-if="loadFailed" role="alert" class="rounded-xl bg-red-50 p-4 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">
        <p>{{ t(`${prefix}.loadFailed`) }}</p>
        <button type="button" class="mt-2 underline" data-test="retry" @click="load">{{ t(`${prefix}.retry`) }}</button>
      </div>
      <template v-else-if="status">
        <div class="flex gap-2 border-b border-gray-200 pb-3 dark:border-dark-700" role="tablist" :aria-label="t(`${prefix}.title`, { group: groupName })">
          <button v-for="item in tabs" :key="item" type="button" role="tab" :aria-selected="tab === item" :data-test="`tab-${item}`" class="rounded-lg px-4 py-2 text-sm font-medium" :class="tab === item ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'text-gray-500 dark:text-gray-400'" @click="tab = item">{{ t(`${prefix}.${item}`) }}</button>
        </div>

        <form v-if="tab === 'settings'" id="subscription-reset-policy" class="space-y-5" @submit.prevent="save">
          <fieldset :disabled="saving" class="space-y-5">
            <div class="grid gap-4 sm:grid-cols-2">
              <label class="block text-sm font-medium text-gray-700 dark:text-gray-200">
                {{ t(`${prefix}.mode`) }}
                <select v-model="form.mode" class="input mt-2 w-full" data-test="mode">
                  <option value="off">{{ t(`${prefix}.off`) }}</option>
                  <option value="observe">{{ t(`${prefix}.observe`) }}</option>
                </select>
              </label>
              <div class="text-sm text-gray-700 dark:text-gray-200">
                <p class="font-medium">{{ t(`${prefix}.source`) }}</p>
                <p class="mt-2 rounded-lg bg-gray-50 px-3 py-2.5 dark:bg-dark-700">{{ t(`${prefix}.source7d`) }}</p>
              </div>
            </div>
            <section class="space-y-3">
              <div class="flex flex-wrap items-center justify-between gap-2">
                <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t(`${prefix}.references`) }}</h4>
                <span class="text-xs text-gray-500 dark:text-gray-400">{{ t(`${prefix}.selected`, { count: form.account_ids.length }) }}</span>
              </div>
              <p class="text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t(`${prefix}.referenceHint`) }}</p>
              <input v-model="search" type="search" class="input w-full" :placeholder="t(`${prefix}.searchPlaceholder`)" :aria-label="t(`${prefix}.searchPlaceholder`)" data-test="account-search" />
              <div class="max-h-48 overflow-y-auto rounded-xl border border-gray-200 dark:border-dark-600">
                <label v-for="account in filteredAccounts" :key="account.id" class="flex cursor-pointer items-center gap-3 border-b border-gray-100 px-3 py-2.5 last:border-0 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700">
                  <input v-model="form.account_ids" type="checkbox" :value="account.id" :data-test="`account-${account.id}`" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
                  <span class="min-w-0 flex-1 truncate text-sm text-gray-800 dark:text-gray-100">{{ account.name }} <span class="text-xs text-gray-400">#{{ account.id }}</span></span>
                  <span class="text-xs text-gray-500 dark:text-gray-400">{{ planName(account) || '—' }}</span>
                  <span v-if="account.status !== 'active'" class="text-xs text-amber-700 dark:text-amber-400">{{ t(`${prefix}.unavailable`) }}</span>
                </label>
                <p v-if="!filteredAccounts.length" class="p-4 text-sm text-gray-500 dark:text-gray-400">{{ t(`${prefix}.noAccounts`) }}</p>
              </div>
              <div v-if="missingSelectedIDs.length" class="rounded-lg bg-amber-50 p-3 text-xs text-amber-800 dark:bg-amber-900/20 dark:text-amber-200">
                <p>{{ t(`${prefix}.missingReferences`) }}</p>
                <label v-for="id in missingSelectedIDs" :key="id" class="mt-2 flex items-center gap-2">
                  <input v-model="form.account_ids" type="checkbox" :value="id" :data-test="`missing-account-${id}`" />{{ accountName(id) }}
                </label>
              </div>
            </section>
            <div class="grid gap-4 sm:grid-cols-2">
              <label class="block text-sm font-medium text-gray-700 dark:text-gray-200">{{ t(`${prefix}.quorum`) }}
                <input v-model.number="form.quorum_percent" type="number" min="1" max="100" step="1" class="input mt-2 w-full" data-test="quorum" />
              </label>
              <label class="block text-sm font-medium text-gray-700 dark:text-gray-200">{{ t(`${prefix}.aggregation`) }}
                <input v-model.number="form.aggregation_minutes" type="number" min="1" max="60" step="1" class="input mt-2 w-full" data-test="aggregation" />
              </label>
            </div>
            <p class="text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t(`${prefix}.quorumHint`) }}</p>
            <section class="rounded-xl border border-gray-200 p-4 dark:border-dark-600">
              <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t(`${prefix}.dimensions`) }}</h4>
              <div class="mt-3 flex flex-wrap gap-5">
                <label v-for="dimension in dimensions" :key="dimension" class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
                  <input v-model="form.reset_dimensions" type="checkbox" :value="dimension" :data-test="`dimension-${dimension}`" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />{{ t(`admin.subscriptions.${dimension}`) }}
                </label>
              </div>
              <p class="mt-3 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t(`${prefix}.dimensionHint`) }}</p>
              <p v-if="form.reset_dimensions.includes('monthly')" class="mt-2 text-xs leading-5 text-amber-700 dark:text-amber-300" data-test="monthly-preview">{{ t(`${prefix}.monthlyPreview`) }}</p>
            </section>
          </fieldset>
          <p v-if="validationError" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ validationError }}</p>
          <p v-if="saveFailed" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ t(`${prefix}.${saveConflict ? 'saveConflict' : 'saveFailed'}`) }}</p>
          <button v-if="saveConflict" type="button" class="text-sm underline" @click="load">{{ t(`${prefix}.reloadPolicy`) }}</button>
          <p v-if="saved" role="status" class="text-sm text-green-700 dark:text-green-400">{{ t(`${prefix}.saved`) }}</p>
        </form>

        <div v-else class="space-y-5" data-test="observations">
          <div class="flex items-center justify-between gap-3">
            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t(`${prefix}.lastObserved`, { time: formatTime(status.last_observed_at) }) }}</p>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="refreshing" @click="refreshStatus">{{ t('common.refresh') }}</button>
          </div>
          <p v-if="statusFailed" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ t(`${prefix}.statusFailed`) }}</p>
          <div class="grid grid-cols-2 gap-3 sm:grid-cols-3">
            <div v-for="metric in metrics" :key="metric.label" class="rounded-xl bg-gray-50 p-4 dark:bg-dark-700">
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ metric.label }}</p>
              <p class="mt-2 text-xl font-semibold text-gray-900 dark:text-white">{{ metric.value }}</p>
            </div>
          </div>
          <p class="text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t(`${prefix}.evidenceHint`) }}</p>
          <p v-if="!status.ready" class="rounded-lg bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-900/20 dark:text-amber-200">{{ t(`${prefix}.${status.policy.mode === 'off' ? 'observationOff' : 'notReady'}`) }}</p>
          <section class="space-y-3">
            <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t(`${prefix}.accountStatus`) }}</h4>
            <div v-for="account in status.accounts" :key="account.account_id" class="rounded-xl border border-gray-200 p-3 text-sm dark:border-dark-600">
              <div class="flex flex-wrap justify-between gap-2">
                <span class="font-medium text-gray-900 dark:text-white">{{ accountName(account.account_id) }}</span>
                <span class="text-gray-500 dark:text-gray-400">{{ t(`${prefix}.states.${account.state}`) }}</span>
              </div>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t(`${prefix}.accountEvidence`, { percent: account.used_percent == null ? '—' : `${account.used_percent.toFixed(1)}%`, time: formatTime(account.last_observed_at) }) }}</p>
              <p v-if="account.duplicate_of != null" class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t(`${prefix}.duplicate`, { account: accountName(account.duplicate_of) }) }}</p>
              <p v-if="account.reason || account.last_error" class="mt-1 text-xs text-amber-700 dark:text-amber-300">{{ reasonText(account.last_error || account.reason, !!account.last_error) }}</p>
            </div>
            <p v-if="!status.accounts.length" class="text-sm text-gray-500">{{ t(`${prefix}.noReferences`) }}</p>
          </section>
          <section class="space-y-3">
            <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t(`${prefix}.batches`) }}</h4>
            <p v-if="!events.length" class="rounded-xl border border-dashed border-gray-200 p-5 text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">{{ t(`${prefix}.noEvents`) }}</p>
            <details v-for="event in events" :key="event.id" :open="event.id === status.active_event?.id" class="rounded-xl border border-gray-200 p-4 dark:border-dark-600" data-test="event">
              <summary class="cursor-pointer text-sm text-gray-900 dark:text-white">
                <span class="font-semibold">{{ t(`${prefix}.eventStates.${event.status}`) }}</span>
                <span class="ml-3">{{ t(`${prefix}.confirmation`, { count: event.confirmed_count, total: event.denominator }) }}</span>
                <span class="ml-2 text-xs text-gray-500">{{ formatTime(event.opened_at) }}</span>
              </summary>
              <p class="mt-3 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t(`${prefix}.batchDetail`, { version: event.policy_version, required: event.required_count, deadline: formatTime(event.deadline_at) }) }}</p>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t(`${prefix}.batchSource`, { kind: t(`${prefix}.kinds.${event.kind}`), dimensions: event.reset_dimensions.map(d => t(`admin.subscriptions.${d}`)).join(' / ') }) }}</p>
              <p v-if="event.reason" class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ reasonText(event.reason) }}</p>
              <ul class="mt-3 space-y-2">
                <li v-for="(member, index) in event.members" :key="index" class="rounded-lg bg-gray-50 p-2 text-xs dark:bg-dark-700">
                  <div class="flex flex-wrap justify-between gap-2"><span class="text-gray-700 dark:text-gray-200">{{ member.account_ids.map(accountName).join(', ') }}</span><span class="text-gray-500 dark:text-gray-400">{{ t(`${prefix}.${member.confirmed ? 'memberConfirmed' : 'memberUnconfirmed'}`) }}</span></div>
                  <p v-if="member.reason" class="mt-1 text-gray-500 dark:text-gray-400">{{ reasonText(member.reason) }}</p>
                </li>
              </ul>
            </details>
          </section>
        </div>
      </template>
    </div>
    <template #footer>
      <div class="flex items-center justify-end gap-3">
        <button type="button" class="btn btn-secondary" :disabled="saving" @click="emit('close')">{{ t('common.close') }}</button>
        <button v-if="tab === 'settings' && status && !loadFailed" type="submit" form="subscription-reset-policy" class="btn btn-primary" :disabled="loading || saving" data-test="save">{{ t(saving ? 'common.saving' : 'common.save') }}</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { formatDateTimeToMinute } from '@/utils/format'
import type { AccountListItem } from '@/types'
import type { SubscriptionResetDimension, SubscriptionResetPolicy, SubscriptionResetPolicyInput, SubscriptionResetStatus } from '@/types/subscriptionResetObserver'

const props = defineProps<{ show: boolean; groupId: number; groupName: string }>()
const emit = defineEmits<{ (e: 'close'): void }>()
const { t, te } = useI18n()
const prefix = 'admin.subscriptions.resetObserver'
const tabs = ['settings', 'observations'] as const
const tab = ref<typeof tabs[number]>('settings')
const dimensions: SubscriptionResetDimension[] = ['daily', 'weekly', 'monthly']
const form = reactive<SubscriptionResetPolicyInput>({ mode: 'off', source: '7d', account_ids: [], quorum_percent: 80, aggregation_minutes: 10, reset_dimensions: [...dimensions], allow_single_subject: false, version: 0 })
const accounts = ref<AccountListItem[]>([])
const status = ref<SubscriptionResetStatus | null>(null)
const loading = ref(false)
const loadFailed = ref(false)
const saving = ref(false)
const saved = ref(false)
const saveFailed = ref(false)
const saveConflict = ref(false)
const refreshing = ref(false)
const statusFailed = ref(false)
const validationError = ref('')
const search = ref('')
let generation = 0
let controller: AbortController | undefined

function eligible(account: AccountListItem): boolean {
  const dimension = account.quota_dimension?.trim().toLowerCase() ?? ''
  return account.platform === 'openai' && account.type === 'oauth' && account.parent_account_id == null
    && (dimension === '' || dimension === 'global')
    && ![account.credentials?.auth_mode, account.credentials?.openai_auth_mode].some(value =>
      typeof value === 'string' && ['agent_identity', 'personalaccesstoken', 'personal_access_token'].includes(value.trim().toLowerCase()))
}

function planName(account: AccountListItem): string {
  const plan = status.value?.accounts.find(a => a.account_id === account.id)?.plan_type
    || account.credentials?.plan_type || account.extra?.plan_type
  return typeof plan === 'string' ? plan : ''
}

const filteredAccounts = computed(() => {
  const term = search.value.trim().toLowerCase()
  return accounts.value.filter(a => `${a.name} ${planName(a)} ${a.id}`.toLowerCase().includes(term))
})
const missingSelectedIDs = computed(() => form.account_ids.filter(id => !accounts.value.some(a => a.id === id)))
const events = computed(() => {
  const items = status.value?.events ?? []
  const active = status.value?.active_event
  return active ? [active, ...items.filter(e => e.id !== active.id)] : items
})
const metrics = computed(() => {
  const current = status.value
  const event = current?.active_event
  return [
    { label: t(`${prefix}.confirmedCoverage`), value: event ? `${event.confirmed_count} / ${event.denominator}` : '—' },
    { label: t(`${prefix}.unconfirmedSubjects`), value: event ? event.denominator - event.confirmed_count : '—' },
    { label: t(`${prefix}.verifiedSubjects`), value: `${current?.verified_subject_count ?? 0} / ${current?.subject_count ?? 0}` },
    { label: t(`${prefix}.unknownSubjects`), value: current?.accounts.filter(a => !a.subject_key && a.duplicate_of == null).length ?? 0 },
    { label: t(`${prefix}.staleAccounts`), value: current?.accounts.filter(a => a.reason === 'stale_sample').length ?? 0 },
    { label: t(`${prefix}.unavailableAccounts`), value: current?.accounts.filter(a => a.last_error).length ?? 0 }
  ]
})

function accountName(id: number): string { return accounts.value.find(a => a.id === id)?.name ?? t(`${prefix}.accountFallback`, { id }) }
function formatTime(value: string | null): string { return value ? formatDateTimeToMinute(value) : t(`${prefix}.never`) }
function reasonText(reason: string, failed = false): string {
  const key = `${prefix}.reasons.${reason}`
  return te(key) ? t(key) : t(`${prefix}.${failed ? 'probeFailed' : 'evidencePending'}`)
}
function applyPolicy(policy: SubscriptionResetPolicy) {
  Object.assign(form, { mode: policy.mode, source: '7d', account_ids: [...(policy.account_ids ?? [])], quorum_percent: policy.quorum_percent, aggregation_minutes: policy.aggregation_minutes, reset_dimensions: [...(policy.reset_dimensions ?? [])], allow_single_subject: policy.allow_single_subject, version: policy.version })
}

async function loadAccounts(groupId: number, signal: AbortSignal): Promise<AccountListItem[]> {
  const items: AccountListItem[] = []
  let page = 1
  let pages = 1
  do {
    const result = await adminAPI.accounts.list(page, 100, { group: String(groupId), platform: 'openai', type: 'oauth', lite: '1' }, { signal })
    items.push(...result.items.filter(eligible))
    pages = result.pages
    page++
  } while (page <= pages && !signal.aborted)
  return items
}

async function load() {
  const request = ++generation
  controller?.abort()
  controller = new AbortController()
  const { signal } = controller
  loading.value = true
  loadFailed.value = false
  status.value = null
  saved.value = false
  saveFailed.value = false
  saveConflict.value = false
  validationError.value = ''
  try {
    const [nextStatus, nextAccounts] = await Promise.all([
      adminAPI.groups.getSubscriptionResetStatus(props.groupId, signal),
      loadAccounts(props.groupId, signal)
    ])
    if (request !== generation) return
    status.value = nextStatus
    accounts.value = nextAccounts
    applyPolicy(nextStatus.policy)
  } catch {
    if (request === generation) loadFailed.value = true
  } finally {
    if (request === generation) loading.value = false
  }
}

async function refreshStatus() {
  if (refreshing.value) return
  const request = generation
  refreshing.value = true
  statusFailed.value = false
  try {
    const next = await adminAPI.groups.getSubscriptionResetStatus(props.groupId, controller?.signal)
    if (request === generation) status.value = next
  } catch {
    if (request === generation) statusFailed.value = true
  } finally {
    if (request === generation) refreshing.value = false
  }
}

async function save() {
  if (saving.value) return
  validationError.value = ''
  saved.value = false
  saveFailed.value = false
  saveConflict.value = false
  if (form.mode === 'observe' && (form.account_ids.length < (form.allow_single_subject ? 1 : 2) || missingSelectedIDs.value.length)) {
    validationError.value = t(`${prefix}.invalidReferences`)
    return
  }
  if (form.account_ids.length > 200) {
    validationError.value = t(`${prefix}.tooManyReferences`)
    return
  }
  if (!Number.isInteger(form.quorum_percent) || form.quorum_percent < 1 || form.quorum_percent > 100
    || !Number.isInteger(form.aggregation_minutes) || form.aggregation_minutes < 1 || form.aggregation_minutes > 60
    || form.reset_dimensions.length === 0) {
    validationError.value = t(`${prefix}.invalidSettings`)
    return
  }
  const request = generation
  const groupId = props.groupId
  saving.value = true
  try {
    const policy = await adminAPI.groups.updateSubscriptionResetPolicy(groupId, { ...form, account_ids: [...form.account_ids], reset_dimensions: [...form.reset_dimensions] })
    if (request !== generation) return
    applyPolicy(policy)
    saved.value = true
    await refreshStatus()
  } catch (error) {
    if (request === generation) {
      saveFailed.value = true
      const apiError = error as { status?: number; response?: { status?: number } }
      saveConflict.value = (apiError?.status ?? apiError?.response?.status) === 409
    }
  } finally {
    if (request === generation) saving.value = false
  }
}

watch(() => [props.show, props.groupId] as const, ([show]) => {
  generation++
  controller?.abort()
  saving.value = false
  refreshing.value = false
  statusFailed.value = false
  search.value = ''
  tab.value = 'settings'
  if (show) void load()
}, { immediate: true })
onUnmounted(() => { generation++; controller?.abort() })
</script>
