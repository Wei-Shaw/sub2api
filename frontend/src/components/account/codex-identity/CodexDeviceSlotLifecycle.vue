<template>
  <section class="space-y-3 border-t border-gray-200 pt-5 dark:border-dark-600" :aria-labelledby="`${idPrefix}-title`">
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0">
        <h3 :id="`${idPrefix}-title`" class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ copy('admin.accounts.codexIdentity.lifecycleTitle', 'Device-slot lifecycle') }}
        </h3>
        <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-dark-400">
          {{ copy('admin.accounts.codexIdentity.lifecycleDesc', 'Saving identity changes rotates affected slots. Previous slots drain until their bindings become idle.') }}
        </p>
      </div>
      <button
        ref="refreshButton"
        type="button"
        class="btn btn-secondary btn-sm shrink-0"
        :disabled="loading || finalizing"
        :aria-label="copy('admin.accounts.codexIdentity.refreshSlots', 'Refresh device slots')"
        @click="loadSlots"
      >
        <Icon name="refresh" size="sm" :class="loading && 'animate-spin'" />
      </button>
    </div>

    <div v-if="loading && slots.length === 0" class="py-4 text-sm text-gray-500 dark:text-dark-400" role="status" aria-live="polite">
      {{ copy('admin.accounts.codexIdentity.loadingSlots', 'Loading device slots...') }}
    </div>
    <div
      v-else-if="errorMessage"
      class="flex items-start justify-between gap-3 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-900 dark:bg-red-900/15 dark:text-red-300"
      role="alert"
    >
      <span>{{ errorMessage }}</span>
      <button type="button" class="font-medium underline" @click="loadSlots">
        {{ copy('admin.accounts.codexIdentity.retry', 'Retry') }}
      </button>
    </div>
    <p v-else-if="slots.length === 0" class="py-3 text-sm text-gray-500 dark:text-dark-400" role="status">
      {{ copy('admin.accounts.codexIdentity.noSlots', 'No device slots have been materialized yet.') }}
    </p>

    <ul v-else class="divide-y divide-gray-100 border-y border-gray-200 dark:divide-dark-700 dark:border-dark-600" :aria-busy="loading">
      <li
        v-for="slot in slots"
        :key="`${slot.os_class}:${slot.epoch}:${slot.slot_index}`"
        class="grid grid-cols-1 gap-2 py-3 sm:grid-cols-[minmax(0,1.4fr)_repeat(3,minmax(5rem,0.7fr))] sm:items-center"
      >
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-sm font-medium text-gray-900 dark:text-white">
              {{ profileLabel(slot) }}
            </span>
            <span
              class="badge text-[11px]"
              :class="slot.state === 'draining' ? 'badge-warning' : 'badge-success'"
            >
              {{ slot.state === 'draining'
                ? copy('admin.accounts.codexIdentity.drainingSlot', 'Draining')
                : copy('admin.accounts.codexIdentity.activeSlot', 'Active') }}
            </span>
          </div>
          <p class="mt-0.5 truncate text-xs text-gray-500 dark:text-dark-400">
            {{ copy('admin.accounts.codexIdentity.catalogVersion', 'Catalog') }} v{{ slot.catalog_version }} · {{ proxyLabel(slot.proxy_id) }}
          </p>
        </div>
        <div class="text-xs text-gray-500 dark:text-dark-400">
          <span class="block">{{ copy('admin.accounts.codexIdentity.slotNumber', 'Slot') }}</span>
          <strong class="text-sm font-medium text-gray-800 dark:text-dark-200">#{{ slot.slot_index + 1 }}</strong>
        </div>
        <div class="text-xs text-gray-500 dark:text-dark-400">
          <span class="block">{{ copy('admin.accounts.codexIdentity.epoch', 'Epoch') }}</span>
          <strong class="text-sm font-medium text-gray-800 dark:text-dark-200">{{ slot.epoch }}</strong>
        </div>
        <div class="text-xs text-gray-500 dark:text-dark-400">
          <span class="block">{{ copy('admin.accounts.codexIdentity.bindings', 'Bindings') }}</span>
          <strong class="text-sm font-medium text-gray-800 dark:text-dark-200">{{ slot.binding_count }}</strong>
        </div>
      </li>
    </ul>

    <div v-if="drainingSlots.length" class="space-y-2">
      <div v-if="confirmingFinalize" class="flex flex-col gap-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-3 text-xs text-amber-800 sm:flex-row sm:items-center sm:justify-between dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-300" role="alertdialog" :aria-labelledby="`${idPrefix}-finalize-title`">
        <span :id="`${idPrefix}-finalize-title`">
          {{ copy('admin.accounts.codexIdentity.finalizeConfirm', 'Finalize only slots the server confirms are fully drained?') }}
        </span>
        <div class="flex shrink-0 justify-end gap-2">
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            data-testid="cancel-finalize-draining"
            :disabled="finalizing"
            @click="closeFinalizeConfirmation"
          >
            {{ copy('admin.accounts.codexIdentity.cancel', 'Cancel') }}
          </button>
          <button
            ref="finalizeConfirmButton"
            type="button"
            class="btn btn-primary btn-sm"
            data-testid="confirm-finalize-draining"
            :disabled="finalizing"
            @click="finalizeDraining"
          >
            <Icon v-if="!finalizing" name="checkCircle" size="sm" />
            {{ finalizing
              ? copy('admin.accounts.codexIdentity.finalizing', 'Finalizing...')
              : copy('admin.accounts.codexIdentity.confirmFinalize', 'Confirm finalize') }}
          </button>
        </div>
      </div>
      <button
        v-else
        ref="finalizeTriggerButton"
        type="button"
        class="btn btn-secondary btn-sm"
        data-testid="finalize-draining-slots"
        :disabled="finalizing"
        @click="openFinalizeConfirmation"
      >
        <Icon name="checkCircle" size="sm" />
        {{ copy('admin.accounts.codexIdentity.finalizeDraining', 'Finalize drained slots') }}
      </button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { CodexDeviceSlotSummary } from '@/api/admin/accounts'
import type { CodexIdentityProxyOption } from '@/types/codexIdentity'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { useCodexIdentityCopy } from './copy'

const props = withDefaults(defineProps<{
  accountId: number
  proxies?: readonly CodexIdentityProxyOption[]
  idPrefix?: string
}>(), {
  proxies: () => [],
  idPrefix: 'codex-device-slots',
})

const emit = defineEmits<{
  finalized: [deleted: number]
}>()

const { t } = useI18n()
const copy = useCodexIdentityCopy()
const appStore = useAppStore()
const slots = ref<CodexDeviceSlotSummary[]>([])
const loading = ref(false)
const finalizing = ref(false)
const confirmingFinalize = ref(false)
const errorMessage = ref('')
const refreshButton = ref<HTMLButtonElement | null>(null)
const finalizeTriggerButton = ref<HTMLButtonElement | null>(null)
const finalizeConfirmButton = ref<HTMLButtonElement | null>(null)
const drainingSlots = computed(() => slots.value.filter((slot) => slot.state === 'draining'))

const focusFinalizeReturnTarget = async () => {
  await nextTick()
  const target = finalizeTriggerButton.value ?? refreshButton.value
  target?.focus()
}

const openFinalizeConfirmation = async () => {
  confirmingFinalize.value = true
  await nextTick()
  finalizeConfirmButton.value?.focus()
}

const closeFinalizeConfirmation = async () => {
  confirmingFinalize.value = false
  await focusFinalizeReturnTarget()
}

const loadSlots = async () => {
  loading.value = true
  errorMessage.value = ''
  try {
    slots.value = await adminAPI.accounts.listCodexDeviceSlots(props.accountId, true)
  } catch (error: any) {
    errorMessage.value = error?.message || copy('admin.accounts.codexIdentity.loadSlotsFailed', 'Failed to load device slots.')
  } finally {
    loading.value = false
  }
}

const finalizeDraining = async () => {
  if (finalizing.value) return
  finalizing.value = true
  let confirmationClosed = false
  try {
    const result = await adminAPI.accounts.finalizeCodexDrainingSlots(props.accountId)
    confirmingFinalize.value = false
    confirmationClosed = true
    if (result.deleted > 0) {
      appStore.showSuccess(t('admin.accounts.codexIdentity.finalizeSuccess', { count: result.deleted }))
    } else {
      appStore.showInfo(t('admin.accounts.codexIdentity.finalizeNone'))
    }
    emit('finalized', result.deleted)
    await loadSlots()
  } catch (error: any) {
    appStore.showError(error?.message || copy('admin.accounts.codexIdentity.finalizeFailed', 'Failed to finalize drained slots.'))
  } finally {
    finalizing.value = false
    if (confirmationClosed) {
      await focusFinalizeReturnTarget()
    }
  }
}

const profileLabel = (slot: CodexDeviceSlotSummary): string => {
  const os = {
    windows: copy('admin.accounts.codexIdentity.windows', 'Windows'),
    macos: copy('admin.accounts.codexIdentity.macos', 'macOS'),
    linux: copy('admin.accounts.codexIdentity.linux', 'Linux'),
    generic: copy('admin.accounts.codexIdentity.genericOS', 'Generic'),
  }[slot.os_class]
  const surface = {
    desktop: copy('admin.accounts.codexIdentity.desktop', 'Desktop'),
    cli: copy('admin.accounts.codexIdentity.cli', 'CLI'),
    sdk: copy('admin.accounts.codexIdentity.sdk', 'SDK'),
    third_party: copy('admin.accounts.codexIdentity.thirdParty', 'Third-party'),
  }[slot.canonical_surface]
  return [os, surface, slot.architecture].filter(Boolean).join(' / ')
}

const proxyLabel = (proxyID?: number | null): string => {
  if (!proxyID) return copy('admin.accounts.codexIdentity.directConnection', 'Direct connection')
  const proxy = props.proxies.find((item) => item.id === proxyID)
  return `${copy('admin.accounts.codexIdentity.proxy', 'Proxy')}: ${proxy?.name ?? `#${proxyID}`}`
}

watch(() => props.accountId, loadSlots, { immediate: true })
</script>
