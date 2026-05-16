<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.editAccount')"
    width="wide"
    @close="handleClose"
  >
    <form
      v-if="account"
      id="edit-account-form"
      @submit.prevent="handleSubmit"
      class="space-y-5"
    >
      <div>
        <label class="input-label">{{ t('common.name') }}</label>
        <input v-model="form.name" type="text" required class="input" data-tour="edit-account-form-name" />
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.notes') }}</label>
        <textarea v-model="form.notes" rows="3" class="input" :placeholder="t('admin.accounts.notesPlaceholder')"></textarea>
        <p class="input-hint">{{ t('admin.accounts.notesHint') }}</p>
      </div>

      <component :is="platformFormComponent" ref="platformFormRef" :context="platformFormContext" v-bind="platformFormExtraProps" />

      <div v-if="showQuotaControl" class="border-t border-gray-200 pt-4 dark:border-dark-600 space-y-4">
        <div class="mb-3">
          <h3 class="input-label mb-0 text-base font-semibold">{{ t('admin.accounts.quotaControl.title') }}</h3>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ isAnthropicQuotaControl ? t('admin.accounts.quotaControl.hint') : t('admin.accounts.quotaLimitHint') }}</p>
        </div>
        <QuotaLimitCard
          :totalLimit="editQuotaLimit" :dailyLimit="editQuotaDailyLimit" :weeklyLimit="editQuotaWeeklyLimit"
          :dailyResetMode="editDailyResetMode" :dailyResetHour="editDailyResetHour"
          :weeklyResetMode="editWeeklyResetMode" :weeklyResetDay="editWeeklyResetDay" :weeklyResetHour="editWeeklyResetHour"
          :resetTimezone="editResetTimezone"
          :quotaNotifyGlobalEnabled="quotaNotifyGlobalEnabled"
          :quotaNotifyDailyEnabled="quotaNotifyState.daily.enabled" :quotaNotifyDailyThreshold="quotaNotifyState.daily.threshold" :quotaNotifyDailyThresholdType="quotaNotifyState.daily.thresholdType"
          :quotaNotifyWeeklyEnabled="quotaNotifyState.weekly.enabled" :quotaNotifyWeeklyThreshold="quotaNotifyState.weekly.threshold" :quotaNotifyWeeklyThresholdType="quotaNotifyState.weekly.thresholdType"
          :quotaNotifyTotalEnabled="quotaNotifyState.total.enabled" :quotaNotifyTotalThreshold="quotaNotifyState.total.threshold" :quotaNotifyTotalThresholdType="quotaNotifyState.total.thresholdType"
          @update:totalLimit="editQuotaLimit = $event" @update:dailyLimit="editQuotaDailyLimit = $event" @update:weeklyLimit="editQuotaWeeklyLimit = $event"
          @update:dailyResetMode="editDailyResetMode = $event" @update:dailyResetHour="editDailyResetHour = $event"
          @update:weeklyResetMode="editWeeklyResetMode = $event" @update:weeklyResetDay="editWeeklyResetDay = $event" @update:weeklyResetHour="editWeeklyResetHour = $event"
          @update:resetTimezone="editResetTimezone = $event"
          @update:quotaNotifyDailyEnabled="quotaNotifyState.daily.enabled = $event" @update:quotaNotifyDailyThreshold="quotaNotifyState.daily.threshold = $event" @update:quotaNotifyDailyThresholdType="quotaNotifyState.daily.thresholdType = $event"
          @update:quotaNotifyWeeklyEnabled="quotaNotifyState.weekly.enabled = $event" @update:quotaNotifyWeeklyThreshold="quotaNotifyState.weekly.threshold = $event" @update:quotaNotifyWeeklyThresholdType="quotaNotifyState.weekly.thresholdType = $event"
          @update:quotaNotifyTotalEnabled="quotaNotifyState.total.enabled = $event" @update:quotaNotifyTotalThreshold="quotaNotifyState.total.threshold = $event" @update:quotaNotifyTotalThresholdType="quotaNotifyState.total.thresholdType = $event"
        />
      </div>

      <div>
        <label class="input-label">{{ t('admin.accounts.proxy') }}</label>
        <ProxySelector v-model="form.proxy_id" :proxies="proxies" />
      </div>

      <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <div>
          <label class="input-label">{{ t('admin.accounts.concurrency') }}</label>
          <input v-model.number="form.concurrency" type="number" min="1" class="input" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.loadFactor') }}</label>
          <input v-model.number="form.load_factor" type="number" min="1" class="input" :placeholder="String(form.concurrency || 1)" />
          <p class="input-hint">{{ t('admin.accounts.loadFactorHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.priority') }}</label>
          <input v-model.number="form.priority" type="number" min="1" class="input" data-tour="account-form-priority" />
          <p class="input-hint">{{ t('admin.accounts.priorityHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.billingRateMultiplier') }}</label>
          <input v-model.number="form.rate_multiplier" type="number" min="0" step="0.001" class="input" />
          <p class="input-hint">{{ t('admin.accounts.billingRateMultiplierHint') }}</p>
        </div>
      </div>

      <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <label class="input-label">{{ t('admin.accounts.expiresAt') }}</label>
        <input v-model="expiresAtInput" type="datetime-local" class="input" />
        <p class="input-hint">{{ t('admin.accounts.expiresAtHint') }}</p>
      </div>

      <div>
        <div class="flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.autoPauseOnExpired') }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.autoPauseOnExpiredDesc') }}</p>
          </div>
          <button type="button" @click="autoPauseOnExpired = !autoPauseOnExpired"
            :class="['relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2', autoPauseOnExpired ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600']">
            <span :class="['pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out', autoPauseOnExpired ? 'translate-x-5' : 'translate-x-0']" />
          </button>
        </div>
      </div>

      <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div>
          <label class="input-label">{{ t('common.status') }}</label>
          <Select v-model="form.status" :options="statusOptions" />
        </div>
      </div>

      <GroupSelector
        v-if="!authStore.isSimpleMode"
        v-model="form.group_ids"
        :groups="groups"
        :platform="account ? account.platform : undefined"
        data-tour="account-form-groups"
      />
    </form>

    <template #footer>
      <div v-if="account" class="flex justify-end gap-3">
        <button @click="handleClose" type="button" class="btn btn-secondary">{{ t('common.cancel') }}</button>
        <button type="submit" form="edit-account-form" :disabled="submitting" class="btn btn-primary" data-tour="account-form-submit">
          <svg v-if="submitting" class="-ml-1 mr-2 h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
          </svg>
          {{ submitting ? t('admin.accounts.updating') : t('common.update') }}
        </button>
      </div>
    </template>
  </BaseDialog>

  <ConfirmDialog
    :show="showMixedChannelWarning"
    :title="t('admin.accounts.mixedChannelWarningTitle')"
    :message="mixedChannelWarningMessageText"
    :confirm-text="t('common.confirm')"
    :cancel-text="t('common.cancel')"
    :danger="true"
    @confirm="handleMixedChannelConfirm"
    @cancel="handleMixedChannelCancel"
  />
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted, nextTick, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { adminAPI } from '@/api/admin'
import { useQuotaNotifyState } from '@/composables/useQuotaNotifyState'
import type { Account, Proxy, AdminGroup, CheckMixedChannelResponse } from '@/types'
import { BaseDialog, ConfirmDialog, Select } from '@sub2api/plugin-sdk'
import ProxySelector from '@/components/common/ProxySelector.vue'
import GroupSelector from '@/components/common/GroupSelector.vue'
import QuotaLimitCard from '@/components/account/QuotaLimitCard.vue'
import { usePlatforms } from '@/composables/usePlatforms'
import { resolvePlatformForm } from './forms/platformFormRegistry'
import type { PlatformFormContext, PlatformFormExposed } from './forms/types'
import { formatDateTimeLocalInput, parseDateTimeLocalInput } from '@/utils/format'
import { loadQuotaFromExtra, applyQuotaToExtra } from './forms/editHelpers'
import { extractApiErrorMessage } from '@/utils/apiError'

interface Props { show: boolean; account: Account | null; proxies: Proxy[]; groups: AdminGroup[] }
const props = defineProps<Props>()
const emit = defineEmits<{ close: []; updated: [account: Account] }>()

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const { fetchPlatforms } = usePlatforms()
const BUILTIN_PLATFORMS = new Set(['anthropic', 'openai', 'gemini', 'antigravity'])

const submitting = ref(false)
const autoPauseOnExpired = ref(false)

const form = reactive({
  name: '', notes: '', proxy_id: null as number | null,
  concurrency: 1, load_factor: null as number | null,
  priority: 1, rate_multiplier: 1,
  status: 'active' as 'active' | 'inactive' | 'error',
  group_ids: [] as number[], expires_at: null as number | null,
})

const platformFormRef = ref<PlatformFormExposed | null>(null)
const platformFormComponent = computed<Component>(() => resolvePlatformForm(props.account?.platform || 'anthropic'))

const platformFormContext = computed<PlatformFormContext>(() => ({
  accountCategory: resolveAccountCategory(),
  accountTypeId: props.account?.type || 'oauth',
  proxyId: form.proxy_id,
  mode: 'edit',
}))

const platformFormExtraProps = computed(() => {
  if (props.account && !BUILTIN_PLATFORMS.has(props.account.platform)) return { platform: props.account.platform }
  return {}
})

function resolveAccountCategory(): string {
  const account = props.account
  if (!account) return 'oauth-based'
  if (account.type === 'apikey' || account.type === 'upstream') return 'apikey'
  if (account.type === 'bedrock') return 'bedrock'
  if (account.type === 'service_account') return 'service_account'
  return 'oauth-based'
}

// Quota
const editQuotaLimit = ref<number | null>(null)
const editQuotaDailyLimit = ref<number | null>(null)
const editQuotaWeeklyLimit = ref<number | null>(null)
const editDailyResetMode = ref<'rolling' | 'fixed' | null>(null)
const editDailyResetHour = ref<number | null>(null)
const editWeeklyResetMode = ref<'rolling' | 'fixed' | null>(null)
const editWeeklyResetDay = ref<number | null>(null)
const editWeeklyResetHour = ref<number | null>(null)
const editResetTimezone = ref<string | null>(null)
const { globalEnabled: quotaNotifyGlobalEnabled, state: quotaNotifyState, loadGlobalState: loadQuotaNotifyGlobal, loadFromExtra: loadQuotaNotifyFromExtra, writeToExtra: writeQuotaNotifyToExtra, reset: resetQuotaNotify } = useQuotaNotifyState()
loadQuotaNotifyGlobal()

const showQuotaControl = computed(() => { const tp = props.account?.type; return tp === 'apikey' || tp === 'bedrock' })
const isAnthropicQuotaControl = computed(() => props.account?.platform === 'anthropic' && (props.account?.type === 'apikey' || props.account?.type === 'bedrock'))

const statusOptions = computed(() => {
  const opts = [{ value: 'active', label: t('common.active') }, { value: 'inactive', label: t('common.inactive') }]
  if (form.status === 'error') opts.push({ value: 'error', label: t('admin.accounts.status.error') })
  return opts
})
const expiresAtInput = computed({ get: () => formatDateTimeLocalInput(form.expires_at), set: (v: string) => { form.expires_at = parseDateTimeLocalInput(v) } })

// Mixed channel warning
const showMixedChannelWarning = ref(false)
const mixedChannelWarningDetails = ref<{ groupName: string; currentPlatform: string; otherPlatform: string } | null>(null)
const mixedChannelWarningRawMessage = ref('')
const mixedChannelWarningAction = ref<(() => Promise<void>) | null>(null)
const antigravityMixedChannelConfirmed = ref(false)
const mixedChannelWarningMessageText = computed(() => mixedChannelWarningDetails.value ? t('admin.accounts.mixedChannelWarning', mixedChannelWarningDetails.value) : mixedChannelWarningRawMessage.value)
const needsMixedChannelCheck = computed(() => {
  const cfg = platformFormRef.value?.oauthConfig
  return cfg?.needsMixedChannelCheck ?? false
})
function clearMixedChannelDialog() { showMixedChannelWarning.value = false; mixedChannelWarningDetails.value = null; mixedChannelWarningRawMessage.value = ''; mixedChannelWarningAction.value = null }
function openMixedChannelDialog(opts: { response?: CheckMixedChannelResponse; message?: string; onConfirm: () => Promise<void> }) {
  const d = opts.response?.details
  mixedChannelWarningDetails.value = d ? { groupName: d.group_name || 'Unknown', currentPlatform: d.current_platform || 'Unknown', otherPlatform: d.other_platform || 'Unknown' } : null
  mixedChannelWarningRawMessage.value = opts.message || opts.response?.message || t('admin.accounts.failedToUpdate')
  mixedChannelWarningAction.value = opts.onConfirm; showMixedChannelWarning.value = true
}
async function ensureAntigravityMixedChannelConfirmed(onConfirm: () => Promise<void>): Promise<boolean> {
  if (!needsMixedChannelCheck.value || antigravityMixedChannelConfirmed.value || !props.account) return true
  try {
    const result = await adminAPI.accounts.checkMixedChannelRisk({ platform: props.account.platform, group_ids: form.group_ids, account_id: props.account.id })
    if (!result.has_risk) return true
    openMixedChannelDialog({ response: result, onConfirm: async () => { antigravityMixedChannelConfirmed.value = true; await onConfirm() } })
    return false
  } catch (err: unknown) { appStore.showError(extractApiErrorMessage(err, t('admin.accounts.failedToUpdate'))); return false }
}
async function handleMixedChannelConfirm() { const a = mixedChannelWarningAction.value; if (!a) { clearMixedChannelDialog(); return }; clearMixedChannelDialog(); submitting.value = true; try { await a() } finally { submitting.value = false } }
function handleMixedChannelCancel() { clearMixedChannelDialog() }
function withConfirmFlag(payload: Record<string, unknown>): Record<string, unknown> {
  if (needsMixedChannelCheck.value && antigravityMixedChannelConfirmed.value) return { ...payload, confirm_mixed_channel_risk: true }
  const c = { ...payload }; delete c.confirm_mixed_channel_risk; return c
}

function syncFormFromAccount(account: Account) {
  antigravityMixedChannelConfirmed.value = false; clearMixedChannelDialog()
  form.name = account.name; form.notes = account.notes || ''; form.proxy_id = account.proxy_id
  form.concurrency = account.concurrency; form.load_factor = account.load_factor ?? null
  form.priority = account.priority; form.rate_multiplier = account.rate_multiplier ?? 1
  form.status = (['active', 'inactive', 'error'].includes(account.status) ? account.status : 'active') as typeof form.status
  form.group_ids = account.group_ids || []; form.expires_at = account.expires_at ?? null
  autoPauseOnExpired.value = account.auto_pause_on_expired === true
  const extra = account.extra as Record<string, unknown> | undefined
  if (account.type === 'apikey' || account.type === 'bedrock') {
    loadQuotaFromExtra(extra, { quotaLimit: editQuotaLimit, quotaDailyLimit: editQuotaDailyLimit, quotaWeeklyLimit: editQuotaWeeklyLimit, dailyResetMode: editDailyResetMode, dailyResetHour: editDailyResetHour, weeklyResetMode: editWeeklyResetMode, weeklyResetDay: editWeeklyResetDay, weeklyResetHour: editWeeklyResetHour, resetTimezone: editResetTimezone })
    loadQuotaNotifyFromExtra(extra)
  } else { editQuotaLimit.value = null; editQuotaDailyLimit.value = null; editQuotaWeeklyLimit.value = null; editDailyResetMode.value = null; editDailyResetHour.value = null; editWeeklyResetMode.value = null; editWeeklyResetDay.value = null; editWeeklyResetHour.value = null; editResetTimezone.value = null; resetQuotaNotify() }
  nextTick(() => { platformFormRef.value?.initFromAccount?.(account) })
}

watch([() => props.show, () => props.account], ([show, a], [wasShow, prev]) => { if (!show || !a) return; if (!wasShow || a !== prev) syncFormFromAccount(a) }, { immediate: true })
onMounted(() => { fetchPlatforms() })

async function submitUpdateAccount(accountID: number, updatePayload: Record<string, unknown>) {
  submitting.value = true
  try {
    const updated = await adminAPI.accounts.update(accountID, withConfirmFlag(updatePayload))
    appStore.showSuccess(t('admin.accounts.accountUpdated')); emit('updated', updated); handleClose()
  } catch (err: unknown) {
    const e = err as { status?: number; error?: string; message?: string }
    if (e.status === 409 && e.error === 'mixed_channel_warning' && needsMixedChannelCheck.value) {
      openMixedChannelDialog({ message: e.message, onConfirm: async () => { antigravityMixedChannelConfirmed.value = true; await submitUpdateAccount(accountID, updatePayload) } }); return
    }
    appStore.showError(extractApiErrorMessage(err, t('admin.accounts.failedToUpdate')))
  } finally { submitting.value = false }
}

async function handleSubmit() {
  if (!props.account) return
  const validation = platformFormRef.value?.validate()
  if (validation && !validation.valid) { appStore.showError(validation.error || t('common.error')); return }
  if (!['active', 'inactive', 'error'].includes(form.status)) { appStore.showError(t('admin.accounts.pleaseSelectStatus')); return }
  try {
    const up: Record<string, unknown> = { ...form }
    if (up.proxy_id === null) up.proxy_id = 0
    if (form.expires_at === null) up.expires_at = 0
    const lf = form.load_factor; if (lf == null || Number.isNaN(lf) || lf <= 0) up.load_factor = 0
    up.auto_pause_on_expired = autoPauseOnExpired.value
    const ep = platformFormRef.value?.getEditPayload?.(props.account)
    if (ep) {
      if (ep.credentials === undefined) return
      if (ep.credentials) up.credentials = ep.credentials
      if (ep.extra !== undefined) up.extra = ep.extra
    }
    if (props.account.type === 'apikey' || props.account.type === 'bedrock') {
      const cur = (up.extra as Record<string, unknown>) || (props.account.extra as Record<string, unknown>) || {}
      const ne: Record<string, unknown> = { ...cur }
      applyQuotaToExtra(ne, { quotaLimit: editQuotaLimit.value, quotaDailyLimit: editQuotaDailyLimit.value, quotaWeeklyLimit: editQuotaWeeklyLimit.value, dailyResetMode: editDailyResetMode.value, dailyResetHour: editDailyResetHour.value, weeklyResetMode: editWeeklyResetMode.value, weeklyResetDay: editWeeklyResetDay.value, weeklyResetHour: editWeeklyResetHour.value, resetTimezone: editResetTimezone.value })
      writeQuotaNotifyToExtra(ne, 'update'); up.extra = ne
    }
    const ok = await ensureAntigravityMixedChannelConfirmed(async () => { await submitUpdateAccount(props.account!.id, up) })
    if (!ok) return
    await submitUpdateAccount(props.account.id, up)
  } catch (err: unknown) { appStore.showError(extractApiErrorMessage(err, t('admin.accounts.failedToUpdate'))) }
}

function handleClose() { antigravityMixedChannelConfirmed.value = false; clearMixedChannelDialog(); emit('close') }
</script>
