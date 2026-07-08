<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('customDomains.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('customDomains.description') }}</p>
        </div>
        <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="loading" @click="loadDomains">
          <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
          {{ t('common.refresh') }}
        </button>
      </div>

      <div v-if="!enabled && !loading" class="rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800 dark:border-amber-800/60 dark:bg-amber-900/20 dark:text-amber-200">
        <div class="flex items-start gap-3">
          <Icon name="infoCircle" size="md" class="mt-0.5 flex-shrink-0" />
          <span>{{ t('customDomains.disabled') }}</span>
        </div>
      </div>

      <div class="grid grid-cols-1 gap-6 xl:grid-cols-[minmax(0,420px)_minmax(0,1fr)]">
        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('customDomains.addDomain') }}</h2>
          </div>
          <form class="space-y-4 p-6" @submit.prevent="createDomain">
            <div>
              <label class="input-label">{{ t('customDomains.title') }}</label>
              <input
                v-model="newDomain"
                type="text"
                class="input font-mono"
                :placeholder="t('customDomains.domainPlaceholder')"
                :disabled="!enabled || creating"
              />
            </div>
            <button type="submit" class="btn btn-primary w-full" :disabled="!enabled || creating || !newDomain.trim()">
              <Icon name="plus" size="sm" class="mr-2" />
              {{ t('customDomains.addDomain') }}
            </button>
          </form>

          <div class="border-t border-gray-100 px-6 py-4 text-sm dark:border-dark-700">
            <p class="font-medium text-gray-900 dark:text-white">{{ t('customDomains.gatewayTarget') }}</p>
            <div class="mt-2 flex min-w-0 items-center gap-2 rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-700/60">
              <code class="min-w-0 flex-1 truncate text-xs text-gray-700 dark:text-gray-200">{{ cnameTarget || '-' }}</code>
              <button
                type="button"
                class="rounded p-1 text-gray-400 transition-colors hover:bg-white hover:text-gray-700 dark:hover:bg-dark-600 dark:hover:text-gray-200"
                :disabled="!cnameTarget"
                :title="t('common.copy')"
                @click="copy(cnameTarget)"
              >
                <Icon name="copy" size="sm" />
              </button>
            </div>
          </div>
        </div>

        <div class="space-y-4">
          <div v-if="loading" class="flex items-center justify-center py-16">
            <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
          </div>

          <EmptyState
            v-else-if="domains.length === 0"
            :title="t('customDomains.empty')"
            :description="t('customDomains.description')"
            :action-text="enabled ? t('customDomains.addDomain') : ''"
            @action="focusDomainInput"
          />

          <div v-else class="space-y-4">
            <div v-for="domain in domains" :key="domain.id" class="card">
              <div class="flex flex-col gap-3 border-b border-gray-100 px-6 py-4 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
                <div class="min-w-0">
                  <div class="flex min-w-0 items-center gap-2">
                    <Icon name="globe" size="md" class="flex-shrink-0 text-gray-400" />
                    <h2 class="truncate font-mono text-lg font-semibold text-gray-900 dark:text-white">{{ domain.domain }}</h2>
                  </div>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('customDomains.lastChecked') }}: {{ domain.last_checked_at ? formatDateTime(domain.last_checked_at) : t('customDomains.neverChecked') }}
                  </p>
                </div>
                <span class="inline-flex w-fit items-center rounded-full px-2.5 py-1 text-xs font-medium" :class="statusClass(domain.status)">
                  {{ statusLabel(domain.status) }}
                </span>
              </div>

              <div class="space-y-5 p-6">
                <div v-if="domain.status === 'active'" class="rounded-lg bg-emerald-50 p-4 dark:bg-emerald-900/20">
                  <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                    <div class="min-w-0">
                      <p class="text-sm font-medium text-emerald-900 dark:text-emerald-200">{{ t('customDomains.apiBaseUrl') }}</p>
                      <code class="mt-1 block truncate text-sm text-emerald-800 dark:text-emerald-100">{{ baseUrl(domain) }}</code>
                    </div>
                    <button type="button" class="btn btn-secondary flex-shrink-0" @click="copy(baseUrl(domain))">
                      <Icon name="copy" size="sm" class="mr-2" />
                      {{ t('customDomains.copyBaseUrl') }}
                    </button>
                  </div>
                </div>

                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('customDomains.dnsRecords') }}</p>
                  <div class="mt-3 grid grid-cols-1 gap-3 lg:grid-cols-2">
                    <DnsRecord
                      :title="t('customDomains.txtRecord')"
                      type="TXT"
                      :name="domain.verification_txt_name"
                      :value="domain.verification_txt_value"
                      :name-label="t('customDomains.recordName')"
                      :value-label="t('customDomains.recordValue')"
                      @copy="copy"
                    />
                    <DnsRecord
                      v-if="domain.cname_target"
                      :title="t('customDomains.cnameRecord')"
                      type="CNAME"
                      :name="domain.domain"
                      :value="domain.cname_target"
                      :name-label="t('customDomains.recordName')"
                      :value-label="t('customDomains.recordValue')"
                      @copy="copy"
                    />
                  </div>
                </div>

                <div v-if="domain.last_error" class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">
                  <span class="font-medium">{{ t('customDomains.lastError') }}:</span>
                  {{ domain.last_error }}
                </div>

                <div v-if="domain.can_manage" class="flex flex-wrap items-center justify-end gap-2">
                  <button type="button" class="btn btn-secondary" :disabled="verifyingId === domain.id" @click="verifyDomain(domain.id)">
                    <Icon name="refresh" size="sm" :class="verifyingId === domain.id ? 'mr-2 animate-spin' : 'mr-2'" />
                    {{ verifyingId === domain.id ? t('customDomains.verifying') : t('customDomains.verify') }}
                  </button>
                  <button type="button" class="btn btn-danger" :disabled="deletingId === domain.id" @click="confirmDelete(domain)">
                    <Icon name="trash" size="sm" class="mr-2" />
                    {{ t('common.delete') }}
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <ConfirmDialog
      :show="deleteDialogOpen"
      :title="t('customDomains.deleteConfirmTitle')"
      :message="t('customDomains.deleteConfirmMessage', { domain: selectedDomain?.domain || '' })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="deleteDomain"
      @cancel="deleteDialogOpen = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { defineComponent, h, onMounted, ref, type PropType } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { useClipboard } from '@/composables/useClipboard'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'
import {
  customDomainsAPI,
  type CustomDomain,
  type CustomDomainStatus,
} from '@/api/customDomains'

const DnsRecord = defineComponent({
  name: 'DnsRecord',
  props: {
    title: { type: String, required: true },
    type: { type: String, required: true },
    name: { type: String, required: true },
    value: { type: String as PropType<string | null | undefined>, default: '' },
    nameLabel: { type: String, required: true },
    valueLabel: { type: String, required: true },
  },
  emits: ['copy'],
  setup(props, { emit }) {
    return () => h('div', { class: 'rounded-lg border border-gray-100 p-4 dark:border-dark-700' }, [
      h('div', { class: 'flex items-center justify-between gap-3' }, [
        h('div', { class: 'min-w-0' }, [
          h('p', { class: 'text-sm font-medium text-gray-900 dark:text-white' }, props.title),
          h('p', { class: 'mt-1 text-xs text-gray-500 dark:text-gray-400' }, props.type),
        ]),
      ]),
      h('div', { class: 'mt-3 space-y-2' }, [
        renderCopyRow(props.nameLabel, props.name, emit),
        renderCopyRow(props.valueLabel, props.value || '-', emit),
      ]),
    ])
  },
})

function renderCopyRow(label: string, value: string, emit: (event: 'copy', value: string) => void) {
  return h('div', { class: 'min-w-0 rounded bg-gray-50 px-3 py-2 dark:bg-dark-700/60' }, [
    h('p', { class: 'text-[11px] font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400' }, label),
    h('div', { class: 'mt-1 flex min-w-0 items-center gap-2' }, [
      h('code', { class: 'min-w-0 flex-1 truncate text-xs text-gray-700 dark:text-gray-200' }, value),
      h('button', {
        type: 'button',
        class: 'rounded p-1 text-gray-400 transition-colors hover:bg-white hover:text-gray-700 dark:hover:bg-dark-600 dark:hover:text-gray-200',
        onClick: () => emit('copy', value),
      }, [h(Icon, { name: 'copy', size: 'sm' })]),
    ]),
  ])
}

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

const loading = ref(false)
const creating = ref(false)
const verifyingId = ref<number | null>(null)
const deletingId = ref<number | null>(null)
const enabled = ref(false)
const cnameTarget = ref('')
const domains = ref<CustomDomain[]>([])
const newDomain = ref('')
const deleteDialogOpen = ref(false)
const selectedDomain = ref<CustomDomain | null>(null)

async function loadDomains() {
  loading.value = true
  try {
    const result = await customDomainsAPI.listUserCustomDomains()
    enabled.value = result.enabled
    cnameTarget.value = result.cname_target
    domains.value = result.domains
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('customDomains.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function createDomain() {
  const value = newDomain.value.trim()
  if (!value) return
  creating.value = true
  try {
    const domain = await customDomainsAPI.createCustomDomain(value)
    domains.value = [domain, ...domains.value]
    newDomain.value = ''
    appStore.showSuccess(t('customDomains.created'))
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('customDomains.saveFailed')))
  } finally {
    creating.value = false
  }
}

async function verifyDomain(id: number) {
  verifyingId.value = id
  try {
    const domain = await customDomainsAPI.verifyCustomDomain(id)
    replaceDomain(domain)
    appStore.showSuccess(domain.status === 'active' ? t('customDomains.verified') : t('customDomains.verifyPending'))
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('customDomains.verifyPending')))
    await loadDomains()
  } finally {
    verifyingId.value = null
  }
}

function confirmDelete(domain: CustomDomain) {
  selectedDomain.value = domain
  deleteDialogOpen.value = true
}

async function deleteDomain() {
  if (!selectedDomain.value) return
  const id = selectedDomain.value.id
  deletingId.value = id
  try {
    await customDomainsAPI.deleteCustomDomain(id)
    domains.value = domains.value.filter((domain) => domain.id !== id)
    deleteDialogOpen.value = false
    selectedDomain.value = null
    appStore.showSuccess(t('customDomains.deleted'))
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    deletingId.value = null
  }
}

function replaceDomain(updated: CustomDomain) {
  domains.value = domains.value.map((domain) => (domain.id === updated.id ? updated : domain))
}

function copy(value: string) {
  copyToClipboard(value)
}

function baseUrl(domain: CustomDomain) {
  return `https://${domain.domain}`
}

function focusDomainInput() {
  const input = document.querySelector<HTMLInputElement>('input[placeholder="api.example.com"]')
  input?.focus()
}

function statusLabel(status: CustomDomainStatus) {
  return t(`customDomains.statuses.${status}`)
}

function statusClass(status: CustomDomainStatus) {
  const classes: Record<CustomDomainStatus, string> = {
    pending_dns: 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300',
    active: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-300',
    disabled: 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300',
    error: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300',
  }
  return classes[status] || classes.pending_dns
}

onMounted(loadDomains)
</script>
