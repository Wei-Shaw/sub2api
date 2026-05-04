<template>
  <AppLayout>
    <PaymentProviderList
      :providers="providers"
      :loading="loading"
      :can-create="hasAnyPaymentTypeEnabled"
      :enabled-payment-types="enabledPaymentTypes"
      :all-payment-types="allPaymentTypes"
      :redirect-label="t('payment.adminSettings.easypayRedirect')"
      @refresh="loadAll"
      @create="openCreateProvider"
      @edit="openEditProvider"
      @delete="confirmDeleteProvider"
      @toggle-field="handleToggleField"
      @toggle-type="handleToggleType"
      @reorder="handleReorderProviders"
    />

    <!-- Create / edit dialog -->
    <PaymentProviderDialog
      ref="providerDialogRef"
      :show="showProviderDialog"
      :saving="providerSaving"
      :editing="editingProvider"
      :all-key-options="providerKeyOptions"
      :enabled-key-options="enabledProviderKeyOptions"
      :all-payment-types="allPaymentTypes"
      :redirect-label="t('payment.adminSettings.easypayRedirect')"
      @close="onDialogClose"
      @save="handleSaveProvider"
    />

    <!-- Delete confirmation -->
    <ConfirmDialog
      :show="showDeleteProviderDialog"
      :title="t('payment.adminSettings.deleteProvider')"
      :message="t('payment.adminSettings.deleteProviderConfirm')"
      :confirm-text="t('common.delete')"
      danger
      @confirm="handleDeleteProvider"
      @cancel="showDeleteProviderDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
/**
 * Admin Payment Providers — wrapper view.
 *
 * PaymentProviderList is a presentational child component that requires
 * `providers` / `loading` / `enabledPaymentTypes` / etc. as props. This
 * wrapper owns the state, fetches data, and routes the list's events to
 * PaymentProviderDialog and the admin payment API.
 *
 * Originally lived inside the host's monolithic SettingsView; ported here
 * during the V3 plugin migration so the route can register a self-contained
 * component (otherwise the list errors with "providers is undefined").
 */
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ConfirmDialog } from '@sub2api/plugin-sdk'
import { useAppStore } from '../../stores/host'
import { adminPaymentAPI } from '../../api/admin/payment'
import { extractApiErrorMessage, extractI18nErrorMessage } from '../../utils/apiError'
import type { ProviderInstance } from '../../types/payment'
import AppLayout from '../../components/common/AppLayout.vue'
import PaymentProviderList from '../../components/payment/PaymentProviderList.vue'
import PaymentProviderDialog from '../../components/payment/PaymentProviderDialog.vue'

const { t } = useI18n()
const appStore = useAppStore()

// ==================== State ====================

const loading = ref(false)
const providers = ref<ProviderInstance[]>([])
const enabledPaymentTypes = ref<string[]>([])

const showProviderDialog = ref(false)
const showDeleteProviderDialog = ref(false)
const providerSaving = ref(false)
const editingProvider = ref<ProviderInstance | null>(null)
const deletingProviderId = ref<number | null>(null)
const providerDialogRef = ref<InstanceType<typeof PaymentProviderDialog> | null>(null)

// ==================== Computed ====================

const allPaymentTypes = computed(() => [
  { value: 'easypay', label: t('payment.methods.easypay') },
  { value: 'alipay', label: t('payment.methods.alipay') },
  { value: 'wxpay', label: t('payment.methods.wxpay') },
  { value: 'stripe', label: t('payment.methods.stripe') },
])

const hasAnyPaymentTypeEnabled = computed(() => enabledPaymentTypes.value.length > 0)

const providerKeyOptions = computed(() => [
  { value: 'easypay', label: t('payment.adminSettings.providerEasypay') },
  { value: 'alipay', label: t('payment.adminSettings.providerAlipay') },
  { value: 'wxpay', label: t('payment.adminSettings.providerWxpay') },
  { value: 'stripe', label: t('payment.adminSettings.providerStripe') },
])

const enabledProviderKeyOptions = computed(() =>
  providerKeyOptions.value.filter(opt => enabledPaymentTypes.value.includes(opt.value)),
)

// ==================== Provider conflict detection ====================
//
// Only one provider may surface a given visible method (alipay / wxpay) at a
// time, otherwise the user-facing payment selector becomes ambiguous. The
// conflict check mirrors the host SettingsView logic (4549bce60^).

type ProviderEnablementCandidate = Pick<
  ProviderInstance,
  'id' | 'provider_key' | 'supported_types' | 'enabled' | 'name'
>

function normalizeVisibleMethod(type: string): string {
  if (type === 'alipay_direct') return 'alipay'
  if (type === 'wxpay_direct') return 'wxpay'
  return type
}

function getProviderVisibleMethods(provider: ProviderEnablementCandidate): Array<'alipay' | 'wxpay'> {
  if (!provider.enabled) return []
  const supportedTypes = Array.isArray(provider.supported_types) ? provider.supported_types : []
  const methods = new Set<'alipay' | 'wxpay'>()
  const addMethod = (type: string) => {
    const m = normalizeVisibleMethod(type)
    if (m === 'alipay' || m === 'wxpay') methods.add(m)
  }

  if (provider.provider_key === 'alipay') {
    if (supportedTypes.length === 0) methods.add('alipay')
    else supportedTypes.forEach(t => { if (normalizeVisibleMethod(t) === 'alipay') methods.add('alipay') })
  } else if (provider.provider_key === 'wxpay') {
    if (supportedTypes.length === 0) methods.add('wxpay')
    else supportedTypes.forEach(t => { if (normalizeVisibleMethod(t) === 'wxpay') methods.add('wxpay') })
  } else if (provider.provider_key === 'easypay') {
    supportedTypes.forEach(addMethod)
  }
  return Array.from(methods)
}

function findProviderEnablementConflict(
  candidate: ProviderEnablementCandidate,
): { method: 'alipay' | 'wxpay'; conflicting: ProviderInstance } | null {
  const claimed = getProviderVisibleMethods(candidate)
  if (claimed.length === 0) return null
  for (const other of providers.value) {
    if (other.id === candidate.id || !other.enabled) continue
    const otherMethods = getProviderVisibleMethods(other)
    const matched = claimed.find(m => otherMethods.includes(m))
    if (matched) return { method: matched, conflicting: other }
  }
  return null
}

function showProviderEnablementConflict(conflict: {
  method: 'alipay' | 'wxpay'
  conflicting: ProviderInstance
}) {
  // enableConflict key is plugin-local; falls back to a generic message.
  const fallback = `${conflict.conflicting.name} already handles ${conflict.method}`
  const key = 'payment.adminSettings.enableConflict'
  const translated = t(key, {
    method: t(`payment.methods.${conflict.method}`),
    provider: conflict.conflicting.name,
  })
  appStore.showError(translated === key ? fallback : translated)
}

// ==================== Data loading ====================

async function loadProviders() {
  try {
    const res = await adminPaymentAPI.getProviders()
    providers.value = res.data || []
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  }
}

async function loadEnabledTypes() {
  try {
    const res = await adminPaymentAPI.getConfig()
    enabledPaymentTypes.value = res.data?.enabled_payment_types || []
  } catch (err: unknown) {
    // Non-fatal: provider list still renders, "Create" button stays disabled
    // until the config endpoint succeeds. Surface the error so admins notice.
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  }
}

async function loadAll() {
  loading.value = true
  try {
    await Promise.all([loadProviders(), loadEnabledTypes()])
  } finally {
    loading.value = false
  }
}

// ==================== Dialog open / close ====================

function openCreateProvider() {
  editingProvider.value = null
  providerDialogRef.value?.reset(enabledProviderKeyOptions.value[0]?.value || 'easypay')
  showProviderDialog.value = true
}

function openEditProvider(provider: ProviderInstance) {
  editingProvider.value = provider
  providerDialogRef.value?.loadProvider(provider)
  showProviderDialog.value = true
}

function onDialogClose() {
  showProviderDialog.value = false
  // Refresh on close so admins see latest server state even if save was
  // cancelled (legacy parity with SettingsView behavior).
  void loadProviders()
}

// ==================== CRUD handlers ====================

async function handleSaveProvider(payload: Partial<ProviderInstance>) {
  providerSaving.value = true
  try {
    const candidate: ProviderEnablementCandidate = {
      id: editingProvider.value?.id ?? 0,
      provider_key: payload.provider_key ?? editingProvider.value?.provider_key ?? '',
      supported_types: payload.supported_types ?? editingProvider.value?.supported_types ?? [],
      enabled: payload.enabled ?? editingProvider.value?.enabled ?? false,
      name: payload.name ?? editingProvider.value?.name ?? '',
    }
    const conflict = findProviderEnablementConflict(candidate)
    if (conflict) {
      showProviderEnablementConflict(conflict)
      return
    }

    if (editingProvider.value) {
      await adminPaymentAPI.updateProvider(editingProvider.value.id, payload)
    } else {
      await adminPaymentAPI.createProvider(payload)
    }
    showProviderDialog.value = false
    await loadProviders()
    appStore.showSuccess(t('common.saved'))
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    providerSaving.value = false
  }
}

async function handleToggleField(
  provider: ProviderInstance,
  field: 'enabled' | 'refund_enabled' | 'allow_user_refund',
) {
  let nextValue: boolean
  if (field === 'enabled') nextValue = !provider.enabled
  else if (field === 'refund_enabled') nextValue = !provider.refund_enabled
  else nextValue = !provider.allow_user_refund

  if (field === 'enabled' && nextValue) {
    const conflict = findProviderEnablementConflict({
      id: provider.id,
      provider_key: provider.provider_key,
      supported_types: provider.supported_types,
      enabled: true,
      name: provider.name,
    })
    if (conflict) {
      showProviderEnablementConflict(conflict)
      return
    }
  }

  const payload: Record<string, boolean> = { [field]: nextValue }
  // Cascade: turning off refund_enabled also turns off allow_user_refund.
  if (field === 'refund_enabled' && !nextValue) {
    payload.allow_user_refund = false
  }
  try {
    await adminPaymentAPI.updateProvider(provider.id, payload)
    await loadProviders()
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  }
}

async function handleToggleType(provider: ProviderInstance, type: string) {
  const updated = provider.supported_types.includes(type)
    ? provider.supported_types.filter(t => t !== type)
    : [...provider.supported_types, type]
  const conflict = findProviderEnablementConflict({
    id: provider.id,
    provider_key: provider.provider_key,
    supported_types: updated,
    enabled: provider.enabled,
    name: provider.name,
  })
  if (conflict) {
    showProviderEnablementConflict(conflict)
    return
  }
  try {
    await adminPaymentAPI.updateProvider(provider.id, {
      supported_types: updated,
    } as Partial<ProviderInstance>)
    await loadProviders()
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  }
}

async function handleReorderProviders(updates: { id: number; sort_order: number }[]) {
  try {
    await Promise.all(
      updates.map(u =>
        adminPaymentAPI.updateProvider(u.id, { sort_order: u.sort_order } as Partial<ProviderInstance>),
      ),
    )
    await loadProviders()
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
    void loadProviders()
  }
}

function confirmDeleteProvider(provider: ProviderInstance) {
  deletingProviderId.value = provider.id
  showDeleteProviderDialog.value = true
}

async function handleDeleteProvider() {
  if (!deletingProviderId.value) return
  try {
    await adminPaymentAPI.deleteProvider(deletingProviderId.value)
    appStore.showSuccess(t('common.deleted'))
    showDeleteProviderDialog.value = false
    await loadProviders()
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  }
}

// ==================== Lifecycle ====================

onMounted(() => {
  void loadAll()
})
</script>
