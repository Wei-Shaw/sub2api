<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div class="flex min-w-0 items-start gap-3">
          <div class="flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-2xl bg-primary-50 text-primary-600 dark:bg-primary-900/30 dark:text-primary-300">
            <Icon name="globe" size="lg" />
          </div>
          <div class="min-w-0">
            <p class="text-xs font-semibold uppercase tracking-wide text-primary-600 dark:text-primary-300">
              {{ t('customDomains.endpointIsolation') }}
            </p>
            <h1 class="mt-1 text-2xl font-semibold text-gray-900 dark:text-white">{{ t('customDomains.title') }}</h1>
            <p class="mt-1 max-w-2xl text-sm text-gray-500 dark:text-gray-400">{{ t('customDomains.description') }}</p>
          </div>
        </div>

        <div class="flex flex-wrap items-center gap-2">
          <div v-if="domains.length > 0" class="inline-flex items-center gap-2 rounded-xl border border-gray-200 bg-white px-3 py-2 text-xs font-medium text-gray-600 shadow-sm dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300">
            <span class="inline-flex items-center gap-1.5">
              <span class="h-2 w-2 rounded-full bg-emerald-500"></span>
              {{ t('customDomains.activeCount', { count: activeCount }) }}
            </span>
            <span class="h-4 w-px bg-gray-200 dark:bg-dark-600"></span>
            <span class="inline-flex items-center gap-1.5">
              <span class="h-2 w-2 rounded-full bg-amber-500"></span>
              {{ t('customDomains.pendingCount', { count: pendingCount }) }}
            </span>
          </div>
          <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="loading" @click="loadDomains">
            <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
            {{ t('common.refresh') }}
          </button>
        </div>
      </div>

      <div v-if="!enabled && !loading" class="rounded-2xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800 dark:border-amber-800/60 dark:bg-amber-900/20 dark:text-amber-200">
        <div class="flex items-start gap-3">
          <Icon name="infoCircle" size="md" class="mt-0.5 flex-shrink-0" />
          <div>
            <p class="font-medium">{{ t('customDomains.disabled') }}</p>
            <p class="mt-1 text-amber-700 dark:text-amber-200/80">{{ t('customDomains.disabledHint') }}</p>
          </div>
        </div>
      </div>

      <div class="grid grid-cols-1 gap-6 xl:grid-cols-[minmax(0,420px)_minmax(0,1fr)]">
        <section class="card overflow-hidden">
          <div class="card-header">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('customDomains.addDomain') }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('customDomains.addDomainDescription') }}</p>
          </div>

          <form class="space-y-4 p-6" @submit.prevent="createDomain">
            <div>
              <label class="input-label" for="custom-domain-input">{{ t('customDomains.domainLabel') }}</label>
              <input
                id="custom-domain-input"
                ref="domainInput"
                v-model="newDomain"
                type="text"
                class="input font-mono"
                autocomplete="off"
                autocapitalize="off"
                spellcheck="false"
                :placeholder="t('customDomains.domainPlaceholder')"
                :disabled="!enabled || creating"
              />
              <p class="input-hint">{{ t('customDomains.domainHint') }}</p>
            </div>

            <button type="submit" class="btn btn-primary w-full" :disabled="!enabled || creating || !newDomain.trim()">
              <Icon name="plus" size="sm" />
              {{ creating ? t('common.loading') : t('customDomains.addDomain') }}
            </button>
          </form>

          <div class="border-t border-gray-100 px-6 py-4 text-sm dark:border-dark-700">
            <div class="flex items-start gap-3">
              <Icon name="server" size="md" class="mt-0.5 flex-shrink-0 text-gray-400" />
              <div class="min-w-0 flex-1">
                <p class="font-medium text-gray-900 dark:text-white">{{ t('customDomains.gatewayTarget') }}</p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('customDomains.gatewayTargetHint') }}</p>
                <CopyField
                  class="mt-3"
                  :label="t('customDomains.cnameTarget')"
                  :value="cnameTarget || '-'"
                  :disabled="!cnameTarget"
                  @copy="copy"
                />
              </div>
            </div>
          </div>
        </section>

        <section class="card overflow-hidden">
          <div class="card-header">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('customDomains.setupGuideTitle') }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('customDomains.setupGuideDescription') }}</p>
          </div>

          <ol class="divide-y divide-gray-100 dark:divide-dark-700">
            <li v-for="step in setupSteps" :key="step.key" class="flex gap-4 px-6 py-4">
              <div
                class="mt-0.5 flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full border text-sm font-semibold"
                :class="setupStepClass(step.state)"
              >
                <Icon v-if="step.state === 'complete'" name="check" size="sm" />
                <span v-else>{{ step.number }}</span>
              </div>
              <div class="min-w-0">
                <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ step.title }}</p>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ step.description }}</p>
              </div>
            </li>
          </ol>
        </section>
      </div>

      <section class="space-y-4">
        <div class="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('customDomains.yourDomains') }}</h2>
            <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('customDomains.yourDomainsDescription') }}</p>
          </div>
        </div>

        <div v-if="loading" class="flex items-center justify-center rounded-2xl border border-gray-100 bg-white py-16 shadow-card dark:border-dark-700/50 dark:bg-dark-800/50">
          <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
        </div>

        <EmptyState
          v-else-if="domains.length === 0"
          :title="t('customDomains.empty')"
          :description="t('customDomains.emptyDescription')"
          :action-text="enabled ? t('customDomains.addDomain') : ''"
          @action="focusDomainInput"
        />

        <div v-else class="space-y-4">
          <article v-for="domain in domains" :key="domain.id" class="card overflow-hidden">
            <div class="flex flex-col gap-4 border-b border-gray-100 px-6 py-5 dark:border-dark-700 sm:flex-row sm:items-start sm:justify-between">
              <div class="min-w-0">
                <div class="flex min-w-0 items-center gap-3">
                  <div class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl" :class="statusIconWrapClass(domain.status)">
                    <Icon v-if="domain.status === 'active'" name="checkCircle" size="md" />
                    <Icon v-else-if="domain.status === 'error'" name="exclamationCircle" size="md" />
                    <Icon v-else-if="domain.status === 'disabled'" name="ban" size="md" />
                    <Icon v-else name="clock" size="md" />
                  </div>
                  <div class="min-w-0">
                    <h3 class="truncate font-mono text-lg font-semibold text-gray-900 dark:text-white">{{ domain.domain }}</h3>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                      {{ t('customDomains.createdAt') }} {{ formatDateTime(domain.created_at) }}
                    </p>
                  </div>
                </div>
              </div>

              <span class="inline-flex w-fit items-center rounded-full px-2.5 py-1 text-xs font-medium" :class="statusClass(domain.status)">
                {{ statusLabel(domain.status) }}
              </span>
            </div>

            <div class="grid grid-cols-1 lg:grid-cols-[minmax(0,1fr)_360px]">
              <div class="space-y-4 border-b border-gray-100 p-6 dark:border-dark-700 lg:border-b-0 lg:border-r">
                <div class="rounded-xl border p-4" :class="statusPanelClass(domain.status)" role="status" aria-live="polite">
                  <div class="flex gap-3">
                    <Icon v-if="domain.status === 'active'" name="checkCircle" size="md" class="mt-0.5 flex-shrink-0" />
                    <Icon v-else-if="domain.status === 'error'" name="exclamationCircle" size="md" class="mt-0.5 flex-shrink-0" />
                    <Icon v-else-if="domain.status === 'disabled'" name="ban" size="md" class="mt-0.5 flex-shrink-0" />
                    <Icon v-else name="clock" size="md" class="mt-0.5 flex-shrink-0" />
                    <div>
                      <p class="font-semibold">{{ statusHeadline(domain) }}</p>
                      <p class="mt-1 text-sm">{{ statusDescription(domain) }}</p>
                      <p v-if="verificationMessage(domain.id)" class="mt-3 text-sm font-medium">
                        {{ verificationMessage(domain.id) }}
                      </p>
                    </div>
                  </div>
                </div>

                <div v-if="domain.status === 'active'" class="rounded-xl border border-emerald-100 bg-emerald-50/70 p-4 dark:border-emerald-900/50 dark:bg-emerald-900/20">
                  <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                    <div class="min-w-0">
                      <p class="text-sm font-semibold text-emerald-900 dark:text-emerald-100">{{ t('customDomains.apiBaseUrl') }}</p>
                      <p class="mt-1 text-sm text-emerald-800 dark:text-emerald-200/80">{{ t('customDomains.apiBaseUrlHint') }}</p>
                    </div>
                    <button type="button" class="btn btn-secondary flex-shrink-0" @click="copy(baseUrl(domain))">
                      <Icon name="copy" size="sm" />
                      {{ t('customDomains.copyBaseUrl') }}
                    </button>
                  </div>
                  <CopyField class="mt-4" :label="t('customDomains.apiBaseUrl')" :value="baseUrl(domain)" @copy="copy" />
                </div>

                <div v-else class="space-y-3">
                  <div class="flex flex-col gap-1 sm:flex-row sm:items-end sm:justify-between">
                    <div>
                      <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('customDomains.dnsRecords') }}</p>
                      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('customDomains.dnsRecordsDescription') }}</p>
                    </div>
                    <span class="text-xs font-medium text-gray-400 dark:text-dark-400">{{ t('customDomains.copyExactly') }}</span>
                  </div>

                  <div class="grid grid-cols-1 gap-3 xl:grid-cols-2">
                    <div class="rounded-xl border border-gray-100 bg-gray-50/60 p-4 dark:border-dark-700 dark:bg-dark-900/40">
                      <div class="flex items-start justify-between gap-3">
                        <div>
                          <span class="inline-flex rounded-md bg-primary-100 px-2 py-1 text-xs font-semibold text-primary-700 dark:bg-primary-900/40 dark:text-primary-200">TXT</span>
                          <h4 class="mt-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('customDomains.ownershipRecordTitle') }}</h4>
                          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('customDomains.ownershipRecordHint') }}</p>
                        </div>
                      </div>
                      <div class="mt-4 space-y-3">
                        <CopyField :label="t('customDomains.recordName')" :value="domain.verification_txt_name" @copy="copy" />
                        <CopyField :label="t('customDomains.recordValue')" :value="domain.verification_txt_value" @copy="copy" />
                      </div>
                    </div>

                    <div class="rounded-xl border border-gray-100 bg-gray-50/60 p-4 dark:border-dark-700 dark:bg-dark-900/40">
                      <div class="flex items-start justify-between gap-3">
                        <div>
                          <span class="inline-flex rounded-md bg-sky-100 px-2 py-1 text-xs font-semibold text-sky-700 dark:bg-sky-900/40 dark:text-sky-200">CNAME</span>
                          <h4 class="mt-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('customDomains.routingRecordTitle') }}</h4>
                          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('customDomains.routingRecordHint') }}</p>
                        </div>
                      </div>
                      <div class="mt-4 space-y-3">
                        <CopyField :label="t('customDomains.recordName')" :value="domain.domain" @copy="copy" />
                        <CopyField
                          :label="t('customDomains.recordValue')"
                          :value="cnameValue(domain) || '-'"
                          :disabled="!cnameValue(domain)"
                          @copy="copy"
                        />
                      </div>
                    </div>
                  </div>
                </div>

                <div v-if="domain.last_error" class="rounded-xl border border-red-100 bg-red-50 p-4 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-900/20 dark:text-red-300">
                  <div class="flex gap-3">
                    <Icon name="exclamationTriangle" size="md" class="mt-0.5 flex-shrink-0" />
                    <div>
                      <p class="font-semibold">{{ t('customDomains.lastError') }}</p>
                      <p class="mt-1">{{ domain.last_error }}</p>
                    </div>
                  </div>
                </div>
              </div>

              <aside class="space-y-4 bg-gray-50/60 p-6 dark:bg-dark-900/30">
                <div>
                  <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('customDomains.statusUpdateTitle') }}</p>
                  <dl class="mt-3 space-y-3 text-sm">
                    <div class="flex items-center justify-between gap-4">
                      <dt class="text-gray-500 dark:text-gray-400">{{ t('customDomains.lastChecked') }}</dt>
                      <dd class="text-right font-medium text-gray-900 dark:text-white">
                        {{ domain.last_checked_at ? formatDateTime(domain.last_checked_at) : t('customDomains.neverChecked') }}
                      </dd>
                    </div>
                    <div v-if="domain.verified_at" class="flex items-center justify-between gap-4">
                      <dt class="text-gray-500 dark:text-gray-400">{{ t('customDomains.verifiedAt') }}</dt>
                      <dd class="text-right font-medium text-gray-900 dark:text-white">{{ formatDateTime(domain.verified_at) }}</dd>
                    </div>
                  </dl>
                </div>

                <div class="rounded-xl border border-gray-100 bg-white p-4 dark:border-dark-700 dark:bg-dark-800/70">
                  <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('customDomains.nextActionTitle') }}</p>
                  <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ nextAction(domain) }}</p>
                </div>

                <div v-if="domain.can_manage" class="flex flex-col gap-2">
                  <button
                    type="button"
                    class="btn w-full"
                    :class="domain.status === 'active' ? 'btn-secondary' : 'btn-primary'"
                    :disabled="verifyingId === domain.id || domain.status === 'disabled'"
                    @click="verifyDomain(domain.id)"
                  >
                    <Icon name="refresh" size="sm" :class="verifyingId === domain.id ? 'animate-spin' : ''" />
                    {{ verifyingId === domain.id ? t('customDomains.verifying') : verifyButtonLabel(domain) }}
                  </button>
                  <button type="button" class="btn btn-danger w-full" :disabled="deletingId === domain.id" @click="confirmDelete(domain)">
                    <Icon name="trash" size="sm" />
                    {{ t('common.delete') }}
                  </button>
                </div>
              </aside>
            </div>
          </article>
        </div>
      </section>
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
import { computed, defineComponent, h, onMounted, ref, type PropType } from 'vue'
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

type SetupStepState = 'complete' | 'current' | 'pending'

const CopyField = defineComponent({
  name: 'CopyField',
  props: {
    label: { type: String, required: true },
    value: { type: String, required: true },
    disabled: { type: Boolean, default: false },
    class: { type: String as PropType<string>, default: '' },
  },
  emits: ['copy'],
  setup(props, { emit }) {
    return () => h('div', {
      class: [
        'min-w-0 rounded-lg border border-gray-100 bg-white px-3 py-2 dark:border-dark-700 dark:bg-dark-800/70',
        props.class,
      ],
    }, [
      h('p', { class: 'text-[11px] font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400' }, props.label),
      h('div', { class: 'mt-1 flex min-w-0 items-center gap-2' }, [
        h('code', { class: 'min-w-0 flex-1 truncate text-xs text-gray-700 dark:text-gray-200' }, props.value),
        h('button', {
          type: 'button',
          class: 'rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 disabled:cursor-not-allowed disabled:opacity-40 dark:hover:bg-dark-700 dark:hover:text-gray-200',
          disabled: props.disabled,
          title: props.label,
          onClick: () => {
            if (!props.disabled) emit('copy', props.value)
          },
        }, [h(Icon, { name: 'copy', size: 'sm' })]),
      ]),
    ])
  },
})

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
const domainInput = ref<HTMLInputElement | null>(null)
const deleteDialogOpen = ref(false)
const selectedDomain = ref<CustomDomain | null>(null)
const verificationResults = ref<Record<number, { status: CustomDomainStatus; message: string }>>({})

const activeCount = computed(() => domains.value.filter((domain) => domain.status === 'active').length)
const pendingCount = computed(() => domains.value.filter((domain) => domain.status === 'pending_dns' || domain.status === 'error').length)

const primaryDomain = computed(() =>
  domains.value.find((domain) => domain.status === 'pending_dns' || domain.status === 'error') ||
  domains.value.find((domain) => domain.status === 'active') ||
  domains.value[0] ||
  null
)

const setupSteps = computed(() => {
  const hasDomain = domains.value.length > 0
  const active = primaryDomain.value?.status === 'active'
  const waitingForDns = hasDomain && !active

  return [
    {
      key: 'add',
      number: 1,
      state: hasDomain ? 'complete' : 'current',
      title: t('customDomains.setupSteps.addDomain.title'),
      description: t('customDomains.setupSteps.addDomain.description'),
    },
    {
      key: 'txt',
      number: 2,
      state: active ? 'complete' : waitingForDns ? 'current' : 'pending',
      title: t('customDomains.setupSteps.addTxt.title'),
      description: t('customDomains.setupSteps.addTxt.description'),
    },
    {
      key: 'cname',
      number: 3,
      state: active ? 'complete' : waitingForDns ? 'current' : 'pending',
      title: t('customDomains.setupSteps.addCname.title'),
      description: t('customDomains.setupSteps.addCname.description'),
    },
    {
      key: 'verify',
      number: 4,
      state: active ? 'complete' : waitingForDns ? 'current' : 'pending',
      title: t('customDomains.setupSteps.verify.title'),
      description: t('customDomains.setupSteps.verify.description'),
    },
  ] satisfies Array<{
    key: string
    number: number
    state: SetupStepState
    title: string
    description: string
  }>
})

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
    verificationResults.value = {
      ...verificationResults.value,
      [domain.id]: {
        status: domain.status,
        message: t('customDomains.createdNeedsDns'),
      },
    }
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
    verificationResults.value = {
      ...verificationResults.value,
      [id]: {
        status: domain.status,
        message: domain.status === 'active'
          ? t('customDomains.verifiedInline')
          : t('customDomains.verifyPendingInline'),
      },
    }
    appStore.showSuccess(domain.status === 'active' ? t('customDomains.verified') : t('customDomains.verifyPending'))
  } catch (err) {
    const message = extractApiErrorMessage(err, t('customDomains.verifyPending'))
    verificationResults.value = {
      ...verificationResults.value,
      [id]: {
        status: 'error',
        message,
      },
    }
    appStore.showError(message)
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
    const nextResults = { ...verificationResults.value }
    delete nextResults[id]
    verificationResults.value = nextResults
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

function cnameValue(domain: CustomDomain) {
  return domain.cname_target || cnameTarget.value
}

function focusDomainInput() {
  domainInput.value?.focus()
}

function statusLabel(status: CustomDomainStatus) {
  return t(`customDomains.statuses.${status}`)
}

function statusHeadline(domain: CustomDomain) {
  return t(`customDomains.statusMessages.${domain.status}.title`)
}

function statusDescription(domain: CustomDomain) {
  return t(`customDomains.statusMessages.${domain.status}.description`)
}

function nextAction(domain: CustomDomain) {
  return t(`customDomains.nextActions.${domain.status}`)
}

function verifyButtonLabel(domain: CustomDomain) {
  return domain.status === 'active' ? t('customDomains.recheck') : t('customDomains.verify')
}

function verificationMessage(id: number) {
  return verificationResults.value[id]?.message || ''
}

function setupStepClass(state: SetupStepState) {
  const classes: Record<SetupStepState, string> = {
    complete: 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/60 dark:bg-emerald-900/30 dark:text-emerald-200',
    current: 'border-primary-200 bg-primary-50 text-primary-700 dark:border-primary-900/60 dark:bg-primary-900/30 dark:text-primary-200',
    pending: 'border-gray-200 bg-gray-50 text-gray-400 dark:border-dark-700 dark:bg-dark-900/50 dark:text-dark-400',
  }
  return classes[state]
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

function statusIconWrapClass(status: CustomDomainStatus) {
  const classes: Record<CustomDomainStatus, string> = {
    pending_dns: 'bg-amber-50 text-amber-600 dark:bg-amber-900/20 dark:text-amber-300',
    active: 'bg-emerald-50 text-emerald-600 dark:bg-emerald-900/20 dark:text-emerald-300',
    disabled: 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-300',
    error: 'bg-red-50 text-red-600 dark:bg-red-900/20 dark:text-red-300',
  }
  return classes[status] || classes.pending_dns
}

function statusPanelClass(status: CustomDomainStatus) {
  const classes: Record<CustomDomainStatus, string> = {
    pending_dns: 'border-amber-100 bg-amber-50 text-amber-800 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200',
    active: 'border-emerald-100 bg-emerald-50 text-emerald-800 dark:border-emerald-900/50 dark:bg-emerald-900/20 dark:text-emerald-200',
    disabled: 'border-gray-200 bg-gray-50 text-gray-700 dark:border-dark-700 dark:bg-dark-900/40 dark:text-gray-300',
    error: 'border-red-100 bg-red-50 text-red-800 dark:border-red-900/50 dark:bg-red-900/20 dark:text-red-200',
  }
  return classes[status] || classes.pending_dns
}

onMounted(loadDomains)
</script>
