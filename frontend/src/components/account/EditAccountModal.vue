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
      <!-- Platform/type badge (read-only) -->
      <div class="flex items-center gap-2 border-t border-gray-200 pt-4 dark:border-dark-600">
        <PlatformIcon :platform="account.platform" :icon-svg="platformDecl?.icon_svg" size="sm" />
        <span class="text-sm font-medium">{{ platformDecl?.display_name || account.platform }}</span>
        <span class="rounded bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600 dark:bg-dark-600 dark:text-gray-400">
          {{ accountTypeDecl?.display_name || account.type }}
        </span>
      </div>

      <component
        v-if="resolvedFormComponent"
        :is="resolvedFormComponent"
        ref="platformFormRef"
        :context="platformFormContext"
        v-bind="platformFormExtraProps"
      />
      <div v-else-if="formLoading" class="flex items-center justify-center py-8">
        <span class="text-sm text-gray-500">{{ t('common.loading') }}</span>
      </div>

      <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <div>
          <label class="input-label">{{ t('common.status') }}</label>
          <Select v-model="form.status" :options="statusOptions" />
        </div>
      </div>
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
import { ref, reactive, shallowRef, computed, watch, onMounted, nextTick, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { adminAPI } from '@/api/admin'
import { useQuotaNotifyState } from '@/composables/useQuotaNotifyState'
import type { Account, Proxy, AdminGroup, CheckMixedChannelResponse } from '@/types'
import { BaseDialog, ConfirmDialog, Select, PlatformIcon } from '@sub2api/plugin-sdk'
import { usePlatforms } from '@/composables/usePlatforms'
import { resolveFormComponentAsync } from './forms/platformFormRegistry'
import type { PlatformFormContext, PlatformFormExposed } from './forms/types'
import { extractApiErrorMessage } from '@/utils/apiError'
import { applyQuotaToExtra } from '@sub2api/plugin-sdk'

interface Props { show: boolean; account: Account | null; proxies: Proxy[]; groups: AdminGroup[] }
const props = defineProps<Props>()
const emit = defineEmits<{ close: []; updated: [account: Account] }>()

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const { fetchPlatforms, getPlatformDecl, getAccountTypeDecl } = usePlatforms()
const { globalEnabled: quotaNotifyGlobalEnabled } = useQuotaNotifyState()

const submitting = ref(false)

const form = reactive({
  status: 'active' as 'active' | 'inactive' | 'error',
})

const platformFormRef = ref<PlatformFormExposed | null>(null)
const resolvedFormComponent = shallowRef<Component | null>(null)
const formLoading = ref(false)

watch(
  () => props.account?.platform,
  async (platform) => {
    if (!platform) {
      resolvedFormComponent.value = null
      return
    }
    formLoading.value = true
    try {
      resolvedFormComponent.value = await resolveFormComponentAsync(platform)
    } finally {
      formLoading.value = false
    }
  },
  { immediate: true },
)

// Re-init form after async component load completes (fixes race with syncFormFromAccount)
watch(resolvedFormComponent, (comp) => {
  if (comp && props.show && props.account) {
    nextTick(() => { platformFormRef.value?.initFromAccount?.(props.account!) })
  }
})

const platformDecl = computed(() => getPlatformDecl(props.account?.platform || ''))
const accountTypeDecl = computed(() => getAccountTypeDecl(props.account?.platform || '', props.account?.type || ''))

const platformFormContext = computed<PlatformFormContext>(() => ({
  accountCategory: resolveAccountCategory(),
  accountTypeId: props.account?.type || 'oauth',
  proxyId: props.account?.proxy_id ?? null,
  mode: 'edit',
  hostData: {
    proxies: props.proxies,
    groups: props.groups,
    isSimpleMode: authStore.isSimpleMode,
    quotaNotifyGlobalEnabled: quotaNotifyGlobalEnabled?.value ?? false,
    platform: props.account?.platform,
    compatiblePlatforms: getPlatformDecl(props.account?.platform || '')?.compatible_gateways ?? [],
  },
}))

const platformFormExtraProps = computed(() => ({
  platform: props.account?.platform || 'anthropic',
}))

function resolveAccountCategory(): string {
  const account = props.account
  if (!account) return 'oauth-based'
  if (account.type === 'apikey' || account.type === 'upstream') return 'apikey'
  if (account.type === 'bedrock') return 'bedrock'
  if (account.type === 'service_account') return 'service_account'
  return 'oauth-based'
}

const statusOptions = computed(() => {
  const opts = [{ value: 'active', label: t('common.active') }, { value: 'inactive', label: t('common.inactive') }]
  if (form.status === 'error') opts.push({ value: 'error', label: t('admin.accounts.status.error') })
  return opts
})

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
async function ensureAntigravityMixedChannelConfirmed(groupIds: number[], onConfirm: () => Promise<void>): Promise<boolean> {
  if (!needsMixedChannelCheck.value || antigravityMixedChannelConfirmed.value || !props.account) return true
  try {
    const result = await adminAPI.accounts.checkMixedChannelRisk({ platform: props.account.platform, group_ids: groupIds, account_id: props.account.id })
    if (!result.has_risk) return true
    openMixedChannelDialog({ response: result, onConfirm: async () => { antigravityMixedChannelConfirmed.value = true; await onConfirm() } })
    return false
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.accounts.failedToUpdate')))
    return false
  }
}

async function handleMixedChannelConfirm() { const a = mixedChannelWarningAction.value; if (!a) { clearMixedChannelDialog(); return }; clearMixedChannelDialog(); submitting.value = true; try { await a() } finally { submitting.value = false } }
function handleMixedChannelCancel() { clearMixedChannelDialog() }
function withConfirmFlag(payload: Record<string, unknown>): Record<string, unknown> {
  if (needsMixedChannelCheck.value && antigravityMixedChannelConfirmed.value) return { ...payload, confirm_mixed_channel_risk: true }
  const c = { ...payload }; delete c.confirm_mixed_channel_risk; return c
}

function syncFormFromAccount(account: Account) {
  antigravityMixedChannelConfirmed.value = false; clearMixedChannelDialog()
  form.status = (['active', 'inactive', 'error'].includes(account.status) ? account.status : 'active') as typeof form.status
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
    const ep = platformFormRef.value?.getEditPayload?.(props.account)
    const up: Record<string, unknown> = {
      name: ep?.common?.name?.trim() || props.account.name,
      notes: ep?.common?.notes?.trim() || null,
      status: form.status,
    }
    if (ep) {
      if (ep.credentials === undefined) { if (ep.error) appStore.showError(t(ep.error)); return }
      if (ep.credentials) up.credentials = ep.credentials
      if (ep.extra !== undefined) up.extra = ep.extra
    }
    // Merge quota fields from common into extra (backend expects them in extra)
    if (ep?.common) {
      const extra = { ...((up.extra as Record<string, unknown>) || {}) }
      applyQuotaToExtra(extra, {
        quotaLimit: ep.common.quota_enabled ? (ep.common.quota_limit ?? null) : null,
        quotaDailyLimit: ep.common.quota_enabled ? (ep.common.quota_daily_limit ?? null) : null,
        quotaWeeklyLimit: ep.common.quota_enabled ? (ep.common.quota_weekly_limit ?? null) : null,
        dailyResetMode: null, dailyResetHour: null,
        weeklyResetMode: null, weeklyResetDay: null, weeklyResetHour: null,
        resetTimezone: null,
      })
      up.extra = extra
    }
    // Merge common fields from plugin form
    if (ep?.common) {
      Object.assign(up, {
        proxy_id: ep.common.proxy_id,
        concurrency: ep.common.concurrency,
        load_factor: ep.common.load_factor,
        priority: ep.common.priority,
        rate_multiplier: ep.common.rate_multiplier,
        expires_at: ep.common.expires_at ?? 0,  // null → 0 tells backend to clear expiration
        auto_pause_on_expired: ep.common.auto_pause_on_expired,
        group_ids: ep.common.group_ids,
      })
    }
    const groupIds = (up.group_ids as number[] | undefined) ?? props.account.group_ids ?? []
    const ok = await ensureAntigravityMixedChannelConfirmed(groupIds, async () => { await submitUpdateAccount(props.account!.id, up) })
    if (!ok) return
    await submitUpdateAccount(props.account.id, up)
  } catch (err: unknown) { appStore.showError(extractApiErrorMessage(err, t('admin.accounts.failedToUpdate'))) }
}

function handleClose() { antigravityMixedChannelConfirmed.value = false; clearMixedChannelDialog(); emit('close') }
</script>
