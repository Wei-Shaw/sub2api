/** Composable: provider CRUD, conflict detection, toggle, reorder. */
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/host'
import { adminPaymentAPI } from '../api/admin/payment'
import { extractI18nErrorMessage } from '@sub2api/plugin-sdk'
import type { ProviderInstance } from '../types/payment'

type ProviderEnablementCandidate = Pick<
  ProviderInstance,
  'id' | 'provider_key' | 'supported_types' | 'enabled' | 'name'
>

function normalizeVisibleMethod(type: string): string {
  if (type === 'alipay_direct') return 'alipay'
  if (type === 'wxpay_direct') return 'wxpay'
  return type
}

function getProviderVisibleMethods(
  provider: ProviderEnablementCandidate,
): Array<'alipay' | 'wxpay'> {
  if (!provider.enabled) return []
  const supportedTypes = Array.isArray(provider.supported_types) ? provider.supported_types : []
  const methods = new Set<'alipay' | 'wxpay'>()
  const addMethod = (type: string) => {
    const m = normalizeVisibleMethod(type)
    if (m === 'alipay' || m === 'wxpay') methods.add(m)
  }
  if (provider.provider_key === 'alipay') {
    if (supportedTypes.length === 0) methods.add('alipay')
    else supportedTypes.forEach(typ => { if (normalizeVisibleMethod(typ) === 'alipay') methods.add('alipay') })
  } else if (provider.provider_key === 'wxpay') {
    if (supportedTypes.length === 0) methods.add('wxpay')
    else supportedTypes.forEach(typ => { if (normalizeVisibleMethod(typ) === 'wxpay') methods.add('wxpay') })
  } else if (provider.provider_key === 'easypay') {
    supportedTypes.forEach(addMethod)
  }
  return Array.from(methods)
}

export function useProviderManagement() {
  const { t } = useI18n()
  const appStore = useAppStore()

  const providers = ref<ProviderInstance[]>([])
  const providersLoading = ref(false)
  const providerSaving = ref(false)
  const editingProvider = ref<ProviderInstance | null>(null)
  const showProviderDialog = ref(false)
  const showDeleteProviderDialog = ref(false)
  const deletingProviderId = ref<number | null>(null)

  async function loadProviders() {
    providersLoading.value = true
    try {
      const res = await adminPaymentAPI.getProviders()
      providers.value = res.data || []
    } catch (err: unknown) {
      appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
    } finally {
      providersLoading.value = false
    }
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

  function showConflictError(conflict: { method: 'alipay' | 'wxpay'; conflicting: ProviderInstance }) {
    const key = 'payment.adminSettings.enableConflict'
    const translated = t(key, {
      method: t('payment.methods.' + conflict.method),
      provider: conflict.conflicting.name,
    })
    const fallback = conflict.conflicting.name + ' already handles ' + conflict.method
    appStore.showError(translated === key ? fallback : translated)
  }

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
      if (conflict) { showConflictError(conflict); return }
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
        id: provider.id, provider_key: provider.provider_key,
        supported_types: provider.supported_types, enabled: true, name: provider.name,
      })
      if (conflict) { showConflictError(conflict); return }
    }
    const payload: Record<string, boolean> = { [field]: nextValue }
    if (field === 'refund_enabled' && !nextValue) payload.allow_user_refund = false
    try {
      await adminPaymentAPI.updateProvider(provider.id, payload)
      await loadProviders()
    } catch (err: unknown) {
      appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
    }
  }

  async function handleToggleType(provider: ProviderInstance, type: string) {
    const updated = provider.supported_types.includes(type)
      ? provider.supported_types.filter(typ => typ !== type)
      : [...provider.supported_types, type]
    const conflict = findProviderEnablementConflict({
      id: provider.id, provider_key: provider.provider_key,
      supported_types: updated, enabled: provider.enabled, name: provider.name,
    })
    if (conflict) { showConflictError(conflict); return }
    try {
      await adminPaymentAPI.updateProvider(provider.id, { supported_types: updated } as Partial<ProviderInstance>)
      await loadProviders()
    } catch (err: unknown) {
      appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
    }
  }

  async function handleReorderProviders(updates: { id: number; sort_order: number }[]) {
    try {
      await Promise.all(
        updates.map(u => adminPaymentAPI.updateProvider(u.id, { sort_order: u.sort_order } as Partial<ProviderInstance>)),
      )
      await loadProviders()
    } catch (err: unknown) {
      appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
      void loadProviders()
    }
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

  return {
    providers,
    providersLoading,
    providerSaving,
    editingProvider,
    showProviderDialog,
    showDeleteProviderDialog,
    deletingProviderId,
    loadProviders,
    handleSaveProvider,
    handleToggleField,
    handleToggleType,
    handleReorderProviders,
    handleDeleteProvider,
  }
}