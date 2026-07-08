<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('admin.customDomains.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.customDomains.description') }}</p>
        </div>
        <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="loading" @click="loadAll">
          <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
          {{ t('common.refresh') }}
        </button>
      </div>

      <div class="grid grid-cols-1 gap-6 xl:grid-cols-[minmax(0,420px)_minmax(0,1fr)]">
        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.customDomains.globalFeature') }}</h2>
          </div>
          <div class="space-y-4 p-6">
            <div class="flex items-center justify-between gap-4 rounded-lg bg-gray-50 p-4 dark:bg-dark-700/60">
              <div>
                <p class="text-sm font-medium text-gray-900 dark:text-white">
                  {{ config.enabled ? t('admin.customDomains.enabled') : t('admin.customDomains.disabled') }}
                </p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('customDomains.gatewayTarget') }}</p>
              </div>
              <button
                type="button"
                :class="[
                  'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
                  config.enabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
                ]"
                :disabled="savingConfig"
                @click="toggleConfig"
              >
                <span
                  :class="[
                    'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                    config.enabled ? 'translate-x-5' : 'translate-x-0'
                  ]"
                />
              </button>
            </div>
            <div class="rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-700/60">
              <code class="block truncate text-xs text-gray-700 dark:text-gray-200">{{ config.cname_target || '-' }}</code>
            </div>
            <form class="space-y-3 border-t border-gray-100 pt-4 dark:border-dark-700" @submit.prevent="createDomain">
              <div>
                <label class="input-label">{{ t('admin.customDomains.owner') }}</label>
                <select v-model="newUserId" class="input" :disabled="creating || usersLoading || !config.enabled">
                  <option value="">
                    {{ usersLoading ? t('admin.customDomains.loadingUsers') : t('admin.customDomains.selectOwner') }}
                  </option>
                  <option v-for="user in userOptions" :key="user.id" :value="user.id">
                    {{ userLabel(user) }}
                  </option>
                </select>
              </div>
              <div>
                <label class="input-label">{{ t('admin.customDomains.filters.domain') }}</label>
                <input
                  v-model="newDomain"
                  type="text"
                  class="input font-mono"
                  :placeholder="t('customDomains.domainPlaceholder')"
                  :disabled="creating || !config.enabled"
                />
              </div>
              <div class="space-y-2 rounded-lg bg-gray-50 p-3 dark:bg-dark-700/60">
                <label class="flex items-center gap-2 text-sm font-medium text-gray-700 dark:text-gray-200">
                  <input v-model="newAllUsers" type="checkbox" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
                  {{ t('admin.customDomains.accessAllUsers') }}
                </label>
                <div v-if="!newAllUsers">
                  <label class="input-label">{{ t('admin.customDomains.accessUsers') }}</label>
                  <select v-model="newUserIds" multiple class="input min-h-[110px]" :disabled="creating || usersLoading || !config.enabled">
                    <option v-for="user in userOptions" :key="user.id" :value="user.id">
                      {{ userLabel(user) }}
                    </option>
                  </select>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.customDomains.ownerAccessHint') }}</p>
                </div>
              </div>
              <button type="submit" class="btn btn-primary w-full" :disabled="creating || !config.enabled || !newDomain.trim() || !newUserId">
                <Icon name="plus" size="sm" class="mr-2" />
                {{ t('customDomains.addDomain') }}
              </button>
            </form>
          </div>
        </div>

        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <div class="grid grid-cols-1 gap-3 md:grid-cols-[minmax(0,1fr)_180px_240px_auto] md:items-end">
              <div>
                <label class="input-label">{{ t('admin.customDomains.filters.domain') }}</label>
                <input v-model="filters.domain" type="text" class="input font-mono" :placeholder="t('customDomains.domainPlaceholder')" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.customDomains.filters.status') }}</label>
                <select v-model="filters.status" class="input">
                  <option value="">{{ t('admin.customDomains.allStatuses') }}</option>
                  <option v-for="status in statuses" :key="status" :value="status">
                    {{ statusLabel(status) }}
                  </option>
                </select>
              </div>
              <div>
                <label class="input-label">{{ t('admin.customDomains.filters.access') }}</label>
                <select v-model="filters.access" class="input" :disabled="usersLoading">
                  <option value="">{{ t('admin.customDomains.anyAccess') }}</option>
                  <option value="all_users">{{ t('admin.customDomains.allUsers') }}</option>
                  <option v-for="user in userOptions" :key="user.id" :value="`user:${user.id}`">
                    {{ userLabel(user) }}
                  </option>
                </select>
              </div>
              <button type="button" class="btn btn-primary" :disabled="loading" @click="loadDomains">
                <Icon name="search" size="sm" class="mr-2" />
                {{ t('common.search') }}
              </button>
            </div>
          </div>

          <div v-if="loading" class="flex items-center justify-center py-16">
            <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
          </div>

          <div v-else class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-100 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800/70">
                <tr>
                  <th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('customDomains.title') }}</th>
                  <th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.customDomains.owner') }}</th>
                  <th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.customDomains.access') }}</th>
                  <th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('common.status') }}</th>
                  <th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('customDomains.lastChecked') }}</th>
                  <th class="px-6 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400">{{ t('admin.customDomains.actions') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-800">
                <tr v-if="domains.length === 0">
                  <td colspan="6" class="px-6 py-12 text-center text-sm text-gray-500 dark:text-gray-400">
                    {{ t('admin.customDomains.listEmpty') }}
                  </td>
                </tr>
                <tr v-for="domain in domains" :key="domain.id" class="hover:bg-gray-50 dark:hover:bg-dark-700/50">
                  <td class="px-6 py-4">
                    <div class="min-w-[220px]">
                      <p class="font-mono text-sm font-medium text-gray-900 dark:text-white">{{ domain.domain }}</p>
                      <p v-if="domain.last_error" class="mt-1 max-w-sm truncate text-xs text-red-600 dark:text-red-300">{{ domain.last_error }}</p>
                    </div>
                  </td>
                  <td class="px-6 py-4 text-sm text-gray-600 dark:text-gray-300">
                    {{ ownerLabel(domain) }}
                  </td>
                  <td class="px-6 py-4 text-sm text-gray-600 dark:text-gray-300">
                    {{ accessLabel(domain) }}
                  </td>
                  <td class="px-6 py-4">
                    <span class="inline-flex items-center rounded-full px-2.5 py-1 text-xs font-medium" :class="statusClass(domain.status)">
                      {{ statusLabel(domain.status) }}
                    </span>
                  </td>
                  <td class="px-6 py-4 text-sm text-gray-600 dark:text-gray-300">
                    {{ domain.last_checked_at ? formatDateTime(domain.last_checked_at) : t('customDomains.neverChecked') }}
                  </td>
                  <td class="px-6 py-4">
                    <div class="flex justify-end gap-2">
                      <button type="button" class="btn btn-secondary btn-sm" :disabled="isActionBusy(domain.id)" @click="verifyDomain(domain)">
                        {{ t('customDomains.verify') }}
                      </button>
                      <button type="button" class="btn btn-secondary btn-sm" :disabled="isActionBusy(domain.id)" @click="openAccessDialog(domain)">
                        {{ t('admin.customDomains.editAccess') }}
                      </button>
                      <button
                        v-if="domain.status === 'disabled'"
                        type="button"
                        class="btn btn-secondary btn-sm"
                        :disabled="isActionBusy(domain.id)"
                        @click="enableDomain(domain)"
                      >
                        {{ t('admin.customDomains.enable') }}
                      </button>
                      <button
                        v-else
                        type="button"
                        class="btn btn-secondary btn-sm"
                        :disabled="isActionBusy(domain.id)"
                        @click="disableDomain(domain)"
                      >
                        {{ t('admin.customDomains.disable') }}
                      </button>
                      <button type="button" class="btn btn-danger btn-sm" :disabled="isActionBusy(domain.id)" @click="confirmDelete(domain)">
                        {{ t('common.delete') }}
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
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

    <div v-if="accessDialogOpen" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 px-4">
      <div class="w-full max-w-lg rounded-lg bg-white shadow-xl dark:bg-dark-800">
        <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.customDomains.editAccess') }}</h2>
          <p class="mt-1 truncate font-mono text-sm text-gray-500 dark:text-gray-400">{{ accessDomain?.domain }}</p>
        </div>
        <form class="space-y-4 p-6" @submit.prevent="saveAccess">
          <label class="flex items-center gap-2 text-sm font-medium text-gray-700 dark:text-gray-200">
            <input v-model="accessAllUsers" type="checkbox" class="rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
            {{ t('admin.customDomains.accessAllUsers') }}
          </label>
          <div v-if="!accessAllUsers">
            <label class="input-label">{{ t('admin.customDomains.accessUsers') }}</label>
            <select v-model="accessUserIds" multiple class="input min-h-[160px]" :disabled="usersLoading">
              <option v-for="user in userOptions" :key="user.id" :value="user.id">
                {{ userLabel(user) }}
              </option>
            </select>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.customDomains.ownerAccessHint') }}</p>
          </div>
          <div class="flex justify-end gap-2">
            <button type="button" class="btn btn-secondary" @click="accessDialogOpen = false">{{ t('common.cancel') }}</button>
            <button type="submit" class="btn btn-primary" :disabled="savingAccess || !accessDomain">
              {{ t('admin.customDomains.saveAccess') }}
            </button>
          </div>
        </form>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import adminAPI from '@/api/admin'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'
import type { AdminUser } from '@/types'
import {
  customDomainsAPI,
  type AdminCustomDomainFilters,
  type CustomDomain,
  type CustomDomainConfig,
  type CustomDomainStatus,
} from '@/api/customDomains'

const { t } = useI18n()
const appStore = useAppStore()

const statuses: CustomDomainStatus[] = ['pending_dns', 'active', 'disabled', 'error']
const loading = ref(false)
const usersLoading = ref(false)
const savingConfig = ref(false)
const savingAccess = ref(false)
const creating = ref(false)
const busyDomainId = ref<number | null>(null)
const domains = ref<CustomDomain[]>([])
const users = ref<AdminUser[]>([])
const config = reactive<CustomDomainConfig>({
  enabled: false,
  cname_target: '',
})
const filters = reactive({
  domain: '',
  status: '',
  access: '',
})
const newDomain = ref('')
const newUserId = ref<number | ''>('')
const newAllUsers = ref(false)
const newUserIds = ref<number[]>([])
const deleteDialogOpen = ref(false)
const selectedDomain = ref<CustomDomain | null>(null)
const accessDialogOpen = ref(false)
const accessDomain = ref<CustomDomain | null>(null)
const accessAllUsers = ref(false)
const accessUserIds = ref<number[]>([])

interface UserOption {
  id: number
  email?: string
  username?: string
  role?: string
  status?: string
}

const userOptions = computed<UserOption[]>(() => {
  const byId = new Map<number, UserOption>()
  for (const user of users.value) {
    byId.set(user.id, user)
  }
  for (const domain of domains.value) {
    if (domain.user) {
      byId.set(domain.user.id, { ...byId.get(domain.user.id), ...domain.user })
    }
  }
  return Array.from(byId.values()).sort((a, b) => userLabel(a).localeCompare(userLabel(b)))
})

async function loadAll() {
  const usersPromise = loadUsers()
  loading.value = true
  try {
    const [nextConfig, nextDomains] = await Promise.all([
      customDomainsAPI.getCustomDomainConfig(),
      customDomainsAPI.listAdminCustomDomains(adminFilters()),
    ])
    config.enabled = nextConfig.enabled
    config.cname_target = nextConfig.cname_target
    domains.value = nextDomains
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('customDomains.loadFailed')))
  } finally {
    loading.value = false
  }
  await usersPromise
}

async function loadUsers() {
  usersLoading.value = true
  try {
    const response = await adminAPI.users.list(1, 1000, { sort_by: 'email', sort_order: 'asc' })
    users.value = response.items
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.customDomains.usersLoadFailed')))
  } finally {
    usersLoading.value = false
  }
}

async function loadDomains() {
  loading.value = true
  try {
    domains.value = await customDomainsAPI.listAdminCustomDomains(adminFilters())
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('customDomains.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function toggleConfig() {
  savingConfig.value = true
  const nextEnabled = !config.enabled
  try {
    const nextConfig = await customDomainsAPI.updateCustomDomainConfig(nextEnabled)
    config.enabled = nextConfig.enabled
    config.cname_target = nextConfig.cname_target
    appStore.showSuccess(t('admin.customDomains.configSaved'))
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    savingConfig.value = false
  }
}

async function createDomain() {
  if (!newUserId.value || !newDomain.value.trim()) return
  const userId = Number(newUserId.value)
  if (!userId) return
  creating.value = true
  try {
    const domain = await customDomainsAPI.createAdminCustomDomain(userId, newDomain.value.trim(), {
      all_users: newAllUsers.value,
      user_ids: normalizeUserIds(newUserIds.value),
    })
    domains.value = [domain, ...domains.value]
    newDomain.value = ''
    newUserId.value = ''
    newAllUsers.value = false
    newUserIds.value = []
    appStore.showSuccess(t('customDomains.created'))
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('customDomains.saveFailed')))
  } finally {
    creating.value = false
  }
}

async function verifyDomain(domain: CustomDomain) {
  busyDomainId.value = domain.id
  try {
    replaceDomain(await customDomainsAPI.verifyAdminCustomDomain(domain.id))
    appStore.showSuccess(t('admin.customDomains.verified'))
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('customDomains.verifyPending')))
    await loadDomains()
  } finally {
    busyDomainId.value = null
  }
}

async function disableDomain(domain: CustomDomain) {
  busyDomainId.value = domain.id
  try {
    replaceDomain(await customDomainsAPI.disableAdminCustomDomain(domain.id))
    appStore.showSuccess(t('admin.customDomains.disabledDomain'))
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    busyDomainId.value = null
  }
}

async function enableDomain(domain: CustomDomain) {
  busyDomainId.value = domain.id
  try {
    replaceDomain(await customDomainsAPI.enableAdminCustomDomain(domain.id))
    appStore.showSuccess(t('admin.customDomains.enabledDomain'))
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    busyDomainId.value = null
  }
}

function confirmDelete(domain: CustomDomain) {
  selectedDomain.value = domain
  deleteDialogOpen.value = true
}

async function deleteDomain() {
  if (!selectedDomain.value) return
  const id = selectedDomain.value.id
  busyDomainId.value = id
  try {
    await customDomainsAPI.deleteAdminCustomDomain(id)
    domains.value = domains.value.filter((domain) => domain.id !== id)
    selectedDomain.value = null
    deleteDialogOpen.value = false
    appStore.showSuccess(t('admin.customDomains.deletedDomain'))
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    busyDomainId.value = null
  }
}

function replaceDomain(updated: CustomDomain) {
  domains.value = domains.value.map((domain) => (domain.id === updated.id ? updated : domain))
}

function openAccessDialog(domain: CustomDomain) {
  accessDomain.value = domain
  accessAllUsers.value = Boolean(domain.all_users)
  accessUserIds.value = normalizeUserIds(domain.user_ids || [])
  accessDialogOpen.value = true
}

async function saveAccess() {
  if (!accessDomain.value) return
  savingAccess.value = true
  try {
    const updated = await customDomainsAPI.updateAdminCustomDomainAccess(accessDomain.value.id, {
      all_users: accessAllUsers.value,
      user_ids: normalizeUserIds(accessUserIds.value),
    })
    replaceDomain(updated)
    accessDialogOpen.value = false
    accessDomain.value = null
    appStore.showSuccess(t('admin.customDomains.accessUpdated'))
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    savingAccess.value = false
  }
}

function isActionBusy(id: number) {
  return busyDomainId.value === id
}

function userLabel(user: UserOption) {
  const primary = user.email || user.username || t('admin.customDomains.ownerId', { id: user.id })
  const username = user.username && user.email && user.username !== user.email ? ` (${user.username})` : ''
  return `${primary}${username} (#${user.id})`
}

function ownerLabel(domain: CustomDomain) {
  return domain.user ? userLabel(domain.user) : t('admin.customDomains.ownerId', { id: domain.user_id })
}

function accessLabel(domain: CustomDomain) {
  if (domain.all_users) {
    return t('admin.customDomains.allUsers')
  }
  const users = (domain.users || []).map(userLabel)
  return users.length > 0 ? users.join(', ') : ownerLabel(domain)
}

function adminFilters() {
  const out: AdminCustomDomainFilters = {
    domain: filters.domain,
    status: filters.status,
  }
  if (filters.access === 'all_users') {
    out.all_users = true
  } else if (filters.access.startsWith('user:')) {
    out.user_id = filters.access.slice(5)
  }
  return out
}

function normalizeUserIds(values: Array<number | string>) {
  const seen = new Set<number>()
  const ids: number[] = []
  for (const value of values) {
    const id = Number(value)
    if (!id || seen.has(id)) continue
    seen.add(id)
    ids.push(id)
  }
  return ids
}

function statusLabel(status: CustomDomainStatus | string) {
  return t(`customDomains.statuses.${status}`)
}

function statusClass(status: CustomDomainStatus | string) {
  const classes: Record<CustomDomainStatus, string> = {
    pending_dns: 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300',
    active: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-300',
    disabled: 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300',
    error: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300',
  }
  return classes[status as CustomDomainStatus] || classes.pending_dns
}

onMounted(loadAll)
</script>
