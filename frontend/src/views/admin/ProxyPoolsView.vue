<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center justify-end gap-2">
          <button class="btn btn-secondary" :disabled="loading" :title="t('common.refresh')" @click="loadPools">
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
          <button class="btn btn-primary" @click="openCreate">
            <Icon name="plus" size="md" class="mr-2" />
            {{ t('admin.proxyPools.create') }}
          </button>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="pools" :loading="loading" row-key="id">
          <template #cell-name="{ row }">
            <button class="text-left font-medium text-gray-900 hover:text-primary-600 dark:text-white dark:hover:text-primary-400" @click="openDetail(row)">
              {{ row.name }}
            </button>
            <div v-if="row.description" class="mt-0.5 max-w-xs truncate text-xs text-gray-500 dark:text-gray-400">{{ row.description }}</div>
          </template>
          <template #cell-status="{ value }">
            <span :class="['badge', value === 'active' ? 'badge-success' : 'badge-gray']">
              {{ value === 'active' ? t('admin.proxyPools.active') : t('admin.proxyPools.disabled') }}
            </span>
          </template>
          <template #cell-health="{ row }">
            <div class="flex items-center gap-2 text-sm">
              <span class="font-medium text-emerald-600 dark:text-emerald-400">{{ row.healthy_proxy_count }}</span>
              <span class="text-gray-300 dark:text-dark-500">/</span>
              <span :class="row.unhealthy_proxy_count ? 'font-medium text-red-600 dark:text-red-400' : 'text-gray-400'">{{ row.unhealthy_proxy_count }}</span>
              <span class="text-xs text-gray-400">{{ t('admin.proxyPools.healthyRatio') }}</span>
            </div>
          </template>
          <template #cell-auto_rebind="{ value }">
            <span :class="['badge', value ? 'badge-primary' : 'badge-gray']">{{ value ? t('common.enabled') : t('common.disabled') }}</span>
          </template>
          <template #cell-actions="{ row }">
            <div class="flex items-center justify-end gap-1">
              <button class="rounded p-2 text-gray-500 hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700" :title="t('admin.proxyPools.details')" @click="openDetail(row)">
                <Icon name="server" size="sm" />
              </button>
              <button class="rounded p-2 text-gray-500 hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700" :title="t('common.edit')" @click="openEdit(row)">
                <Icon name="edit" size="sm" />
              </button>
              <button class="rounded p-2 text-gray-500 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20" :title="t('common.delete')" @click="deletingPool = row">
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </template>
          <template #empty>
            <div class="py-12 text-center text-gray-500 dark:text-gray-400">
              <Icon name="server" size="xl" class="mx-auto mb-3" />
              <p>{{ t('admin.proxyPools.noPools') }}</p>
            </div>
          </template>
        </DataTable>
      </template>
    </TablePageLayout>

    <BaseDialog :show="showForm" :title="editingPool ? t('admin.proxyPools.edit') : t('admin.proxyPools.create')" width="normal" @close="showForm = false">
      <form id="proxy-pool-form" class="space-y-5" @submit.prevent="savePool">
        <div>
          <label class="input-label">{{ t('admin.proxyPools.name') }}</label>
          <input v-model.trim="form.name" class="input" maxlength="100" required />
        </div>
        <div>
          <label class="input-label">{{ t('admin.proxyPools.descriptionLabel') }}</label>
          <textarea v-model.trim="form.description" class="input" rows="3"></textarea>
        </div>
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div>
            <label class="input-label">{{ t('admin.proxyPools.healthInterval') }}</label>
            <input v-model.number="form.health_interval_seconds" class="input" type="number" min="30" max="86400" required />
          </div>
          <div>
            <label class="input-label">{{ t('admin.proxyPools.failureThreshold') }}</label>
            <input v-model.number="form.failure_threshold" class="input" type="number" min="1" max="10" required />
          </div>
          <div>
            <label class="input-label">{{ t('admin.proxyPools.status') }}</label>
            <Select v-model="form.status" :options="statusOptions" />
          </div>
        </div>
        <label class="flex items-center gap-3 border-t border-gray-200 pt-4 dark:border-dark-600">
          <input v-model="form.auto_rebind" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" type="checkbox" />
          <span class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('admin.proxyPools.autoRebind') }}</span>
        </label>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="showForm = false">{{ t('common.cancel') }}</button>
          <button type="submit" form="proxy-pool-form" class="btn btn-primary" :disabled="saving">{{ saving ? t('common.saving') : t('common.save') }}</button>
        </div>
      </template>
    </BaseDialog>

    <BaseDialog :show="!!detailPool" :title="detailPool?.name || ''" width="extra-wide" @close="closeDetail">
      <div v-if="detailPool" class="space-y-6">
        <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <div class="rounded bg-gray-50 px-3 py-2 dark:bg-dark-700">
            <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.proxyPools.proxies') }}</div>
            <div class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ detailProxies.length }}</div>
          </div>
          <div class="rounded bg-emerald-50 px-3 py-2 dark:bg-emerald-900/20">
            <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.proxyPools.healthy') }}</div>
            <div class="mt-1 text-xl font-semibold text-emerald-600 dark:text-emerald-400">{{ healthyCount }}</div>
          </div>
          <div class="rounded bg-red-50 px-3 py-2 dark:bg-red-900/20">
            <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.proxyPools.unhealthy') }}</div>
            <div class="mt-1 text-xl font-semibold text-red-600 dark:text-red-400">{{ unhealthyCount }}</div>
          </div>
          <div class="rounded bg-primary-50 px-3 py-2 dark:bg-primary-900/20">
            <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.proxyPools.boundAccounts') }}</div>
            <div class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ boundAccounts }}</div>
          </div>
        </div>

        <div class="flex flex-wrap items-center justify-between gap-3 border-y border-gray-200 py-3 dark:border-dark-600">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.proxyPools.members') }}</h3>
          <div class="flex gap-2">
            <button class="btn btn-secondary btn-sm" :disabled="rebinding" @click="runRebind">
              <Icon name="refresh" size="sm" class="mr-2" :class="rebinding ? 'animate-spin' : ''" />
              {{ t('admin.proxyPools.checkNow') }}
            </button>
            <button class="btn btn-primary btn-sm" @click="openAssign">
              <Icon name="plus" size="sm" class="mr-2" />{{ t('admin.proxyPools.addProxies') }}
            </button>
          </div>
        </div>

        <div v-if="detailProxies.length" class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead class="border-b border-gray-200 text-left text-xs uppercase text-gray-500 dark:border-dark-600 dark:text-gray-400">
              <tr><th class="px-3 py-2">{{ t('admin.proxyPools.proxy') }}</th><th class="px-3 py-2">{{ t('admin.proxyPools.health') }}</th><th class="px-3 py-2">{{ t('admin.proxyPools.exit') }}</th><th class="px-3 py-2">{{ t('admin.proxyPools.latency') }}</th><th class="px-3 py-2">{{ t('admin.proxyPools.checkedAt') }}</th><th class="px-3 py-2">{{ t('admin.proxyPools.boundAccounts') }}</th><th class="px-3 py-2"></th></tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-600">
              <tr v-for="proxy in detailProxies" :key="proxy.id">
                <td class="px-3 py-3"><div class="font-medium text-gray-900 dark:text-white">{{ proxy.name }}</div><code class="text-xs text-gray-500">{{ proxy.host }}:{{ proxy.port }}</code></td>
                <td class="px-3 py-3"><span :class="healthClass(proxy.pool_health)">{{ healthLabel(proxy.pool_health) }}</span><div v-if="proxy.pool_failures" class="mt-1 text-xs text-red-500">{{ t('admin.proxyPools.failures', { count: proxy.pool_failures }) }}</div></td>
                <td class="px-3 py-3"><div class="text-gray-700 dark:text-gray-200">{{ proxy.ip_address || '-' }}</div><div v-if="proxy.country" class="mt-0.5 text-xs text-gray-500">{{ proxy.country }}<span v-if="proxy.country_code"> ({{ proxy.country_code }})</span></div></td>
                <td class="px-3 py-3 text-gray-600 dark:text-gray-300">{{ typeof proxy.latency_ms === 'number' ? `${proxy.latency_ms}ms` : '-' }}</td>
                <td class="px-3 py-3 text-gray-600 dark:text-gray-300">{{ proxy.pool_checked_at ? formatDateTime(proxy.pool_checked_at) : '-' }}</td>
                <td class="px-3 py-3 text-gray-700 dark:text-gray-200">{{ proxy.account_count }}</td>
                <td class="px-3 py-3 text-right"><button class="rounded p-2 text-gray-400 hover:bg-red-50 hover:text-red-600" :title="t('admin.proxyPools.removeProxy')" @click="removeProxy(proxy.id)"><Icon name="x" size="sm" /></button></td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="border-y border-dashed border-gray-300 py-10 text-center text-sm text-gray-500 dark:border-dark-600">{{ t('admin.proxyPools.noMembers') }}</div>

        <div>
          <h3 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.proxyPools.rebindLogs') }}</h3>
          <div v-if="logs.length" class="space-y-2">
            <div v-for="entry in logs" :key="entry.id" class="grid grid-cols-[auto_1fr_auto] items-center gap-3 border-b border-gray-100 py-2 text-sm dark:border-dark-700">
              <span class="text-xs text-gray-400">{{ formatDateTime(entry.created_at) }}</span>
              <span class="flex min-w-0 items-center gap-2 text-gray-700 dark:text-gray-200"><span class="truncate">{{ entry.from_proxy_name || entry.from_proxy_id || '-' }}</span><Icon name="arrowRight" size="xs" class="flex-none text-gray-400" /><span class="truncate">{{ entry.to_proxy_name || entry.to_proxy_id || '-' }}</span></span>
              <span class="badge badge-primary">{{ entry.account_count }}</span>
            </div>
          </div>
          <div v-else class="py-4 text-sm text-gray-400">{{ t('admin.proxyPools.noLogs') }}</div>
        </div>
      </div>
    </BaseDialog>

    <BaseDialog :show="showAssign" :title="t('admin.proxyPools.addProxies')" width="wide" @close="showAssign = false">
      <div class="space-y-3">
        <input v-model.trim="proxySearch" class="input" :placeholder="t('admin.proxies.searchProxies')" />
        <div class="max-h-96 overflow-y-auto border-y border-gray-200 py-2 dark:border-dark-600">
          <label v-for="proxy in assignableProxies" :key="proxy.id" class="flex cursor-pointer items-center gap-3 px-2 py-2 hover:bg-gray-50 dark:hover:bg-dark-700">
            <input type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600" :checked="selectedProxyIds.has(proxy.id)" @change="toggleProxy(proxy.id)" />
            <span class="min-w-0 flex-1 truncate text-sm font-medium text-gray-900 dark:text-white">{{ proxy.name }}</span>
            <code class="text-xs text-gray-500">{{ proxy.host }}:{{ proxy.port }}</code>
          </label>
          <div v-if="!assignableProxies.length" class="py-8 text-center text-sm text-gray-500">{{ t('admin.proxyPools.noAssignableProxies') }}</div>
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3"><button class="btn btn-secondary" @click="showAssign = false">{{ t('common.cancel') }}</button><button class="btn btn-primary" :disabled="!selectedProxyIds.size || assigning" @click="assignProxies">{{ assigning ? t('common.saving') : t('admin.proxyPools.addSelected') }}</button></div>
      </template>
    </BaseDialog>

    <ConfirmDialog :show="!!deletingPool" :title="t('admin.proxyPools.delete')" :message="t('admin.proxyPools.deleteConfirm', { name: deletingPool?.name })" :danger="true" @confirm="deletePool" @cancel="deletingPool = null" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTime } from '@/utils/format'
import type { Column } from '@/components/common/types'
import type { Proxy, ProxyPoolWithStats, ProxyPoolProxy, ProxyPoolRebindLog } from '@/types'

const { t } = useI18n()
const appStore = useAppStore()
const pools = ref<ProxyPoolWithStats[]>([])
const loading = ref(false)
const saving = ref(false)
const showForm = ref(false)
const editingPool = ref<ProxyPoolWithStats | null>(null)
const deletingPool = ref<ProxyPoolWithStats | null>(null)
const detailPool = ref<ProxyPoolWithStats | null>(null)
const detailProxies = ref<ProxyPoolProxy[]>([])
const logs = ref<ProxyPoolRebindLog[]>([])
const rebinding = ref(false)
const showAssign = ref(false)
const assigning = ref(false)
const allProxies = ref<Proxy[]>([])
const selectedProxyIds = ref(new Set<number>())
const proxySearch = ref('')

const columns: Column[] = [
  { key: 'name', label: t('admin.proxyPools.name') },
  { key: 'status', label: t('admin.proxyPools.status') },
  { key: 'health', label: t('admin.proxyPools.health') },
  { key: 'bound_account_count', label: t('admin.proxyPools.boundAccounts') },
  { key: 'health_interval_seconds', label: t('admin.proxyPools.healthInterval') },
  { key: 'failure_threshold', label: t('admin.proxyPools.failureThreshold') },
  { key: 'auto_rebind', label: t('admin.proxyPools.autoRebind') },
  { key: 'actions', label: '' }
]
const statusOptions = computed(() => [
  { value: 'active', label: t('admin.proxyPools.active') },
  { value: 'disabled', label: t('admin.proxyPools.disabled') }
])
const form = reactive({ name: '', description: '', status: 'active', health_interval_seconds: 300, failure_threshold: 2, auto_rebind: true })
const healthyCount = computed(() => detailProxies.value.filter((proxy) => proxy.pool_health === 'healthy').length)
const unhealthyCount = computed(() => detailProxies.value.filter((proxy) => proxy.pool_health === 'unhealthy').length)
const boundAccounts = computed(() => detailProxies.value.reduce((sum, proxy) => sum + proxy.account_count, 0))
const assignableProxies = computed(() => {
  const memberIds = new Set(detailProxies.value.map((proxy) => proxy.id))
  const query = proxySearch.value.toLowerCase()
  return allProxies.value.filter((proxy) => !memberIds.has(proxy.id) && (!query || proxy.name.toLowerCase().includes(query) || proxy.host.toLowerCase().includes(query)))
})

async function loadPools() { loading.value = true; try { pools.value = await adminAPI.proxyPools.list() } catch { appStore.showError(t('admin.proxyPools.loadFailed')) } finally { loading.value = false } }
function resetForm() { Object.assign(form, { name: '', description: '', status: 'active', health_interval_seconds: 300, failure_threshold: 2, auto_rebind: true }) }
function openCreate() { editingPool.value = null; resetForm(); showForm.value = true }
function openEdit(pool: ProxyPoolWithStats) { editingPool.value = pool; Object.assign(form, { name: pool.name, description: pool.description || '', status: pool.status, health_interval_seconds: pool.health_interval_seconds, failure_threshold: pool.failure_threshold, auto_rebind: pool.auto_rebind }); showForm.value = true }
async function savePool() { saving.value = true; try { const payload = { ...form, status: form.status as 'active' | 'disabled' }; if (editingPool.value) await adminAPI.proxyPools.update(editingPool.value.id, payload); else await adminAPI.proxyPools.create(payload); showForm.value = false; appStore.showSuccess(t('admin.proxyPools.saved')); await loadPools() } catch { appStore.showError(t('admin.proxyPools.saveFailed')) } finally { saving.value = false } }
async function deletePool() { if (!deletingPool.value) return; try { await adminAPI.proxyPools.remove(deletingPool.value.id); deletingPool.value = null; appStore.showSuccess(t('admin.proxyPools.deleted')); await loadPools() } catch { appStore.showError(t('admin.proxyPools.deleteFailed')) } }
async function openDetail(pool: ProxyPoolWithStats) {
  detailPool.value = pool
  try {
    await refreshDetail()
  } catch {
    appStore.showError(t('admin.proxyPools.loadFailed'))
  }
}
function closeDetail() { detailPool.value = null; detailProxies.value = []; logs.value = [] }
async function refreshDetail() { if (!detailPool.value) return; [detailProxies.value, logs.value] = await Promise.all([adminAPI.proxyPools.listProxies(detailPool.value.id), adminAPI.proxyPools.rebindLogs(detailPool.value.id)]) }
async function runRebind() { if (!detailPool.value) return; rebinding.value = true; try { const count = await adminAPI.proxyPools.rebind(detailPool.value.id); appStore.showSuccess(t('admin.proxyPools.rebindDone', { count })); await Promise.all([refreshDetail(), loadPools()]) } catch { appStore.showError(t('admin.proxyPools.rebindFailed')) } finally { rebinding.value = false } }
async function openAssign() {
  selectedProxyIds.value = new Set()
  proxySearch.value = ''
  try {
    allProxies.value = await adminAPI.proxies.getAll()
    showAssign.value = true
  } catch {
    appStore.showError(t('admin.proxies.failedToLoad'))
  }
}
function toggleProxy(id: number) { const next = new Set(selectedProxyIds.value); next.has(id) ? next.delete(id) : next.add(id); selectedProxyIds.value = next }
async function assignProxies() { if (!detailPool.value) return; assigning.value = true; try { await adminAPI.proxyPools.assignProxies(detailPool.value.id, [...selectedProxyIds.value]); showAssign.value = false; await Promise.all([refreshDetail(), loadPools()]) } catch { appStore.showError(t('admin.proxyPools.assignFailed')) } finally { assigning.value = false } }
async function removeProxy(id: number) { if (!detailPool.value) return; try { await adminAPI.proxyPools.removeProxies(detailPool.value.id, [id]); await Promise.all([refreshDetail(), loadPools()]) } catch { appStore.showError(t('admin.proxyPools.removeFailed')) } }
function healthClass(health: string) { return ['badge', health === 'healthy' ? 'badge-success' : health === 'unhealthy' ? 'badge-danger' : 'badge-gray'] }
function healthLabel(health: string) { return health === 'healthy' ? t('admin.proxyPools.healthy') : health === 'unhealthy' ? t('admin.proxyPools.unhealthy') : t('admin.proxyPools.unknown') }

onMounted(loadPools)
</script>
