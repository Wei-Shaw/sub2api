<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
          <div class="grid min-w-0 flex-1 grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:flex">
            <SearchInput
              v-model="search"
              class="min-w-0 xl:max-w-xs"
              :placeholder="t('admin.upstreams.search')"
            />
            <Select v-model="platformFilter" class="min-w-0 xl:w-36" :options="platformFilterOptions" />
            <Select v-model="brandFilter" class="min-w-0 xl:w-36" :options="brandFilterOptions" />
            <Select v-model="modelFilter" class="min-w-0 xl:w-48" :options="modelFilterOptions" searchable />
            <Select v-model="siteTypeFilter" class="min-w-0 xl:w-36" :options="siteTypeFilterOptions" />
            <Select v-model="healthFilter" class="min-w-0 xl:w-36" :options="healthFilterOptions" />
          </div>
          <div class="grid grid-cols-2 gap-2 sm:flex sm:items-center">
            <span class="mr-2 hidden text-xs text-gray-500 xl:inline dark:text-dark-400">
              {{ t('admin.upstreams.summary', { stations: stations.length, routes: totalRoutes }) }}
            </span>
            <button type="button" class="btn btn-secondary whitespace-nowrap" :disabled="syncingAll" @click="syncAll">
              <Icon name="sync" size="sm" :class="syncingAll ? 'animate-spin' : ''" />
              {{ t('admin.upstreams.syncAll') }}
            </button>
            <button type="button" class="btn btn-primary whitespace-nowrap" @click="openCreate">
              <Icon name="plus" size="sm" />
              {{ t('admin.upstreams.create') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="filteredStations"
          :loading="loading"
          row-key="id"
          sort-storage-key="admin-upstreams-sort"
          default-sort-key="name"
        >
          <template #cell-name="{ row }">
            <div class="min-w-44">
              <div class="flex items-center gap-2">
                <span class="font-medium text-gray-900 dark:text-white">{{ row.name }}</span>
                <span class="rounded bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-500 dark:bg-dark-700 dark:text-dark-300">
                  {{ siteTypeLabel(row.site_type) }}
                </span>
                <span v-if="row.credential_mode === 'api_key'" class="rounded bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-500 dark:bg-dark-700 dark:text-dark-300">
                  {{ t('admin.upstreams.fixed') }}
                </span>
              </div>
              <div class="mt-0.5 max-w-64 truncate font-mono text-[11px] text-gray-400" :title="row.base_url">{{ row.base_url }}</div>
            </div>
          </template>

          <template #cell-protocols="{ row }">
            <div class="flex min-w-28 flex-wrap gap-1">
              <span v-for="platform in protocolsFor(row.id)" :key="platform" :class="platformBadge(platform)">
                {{ platformLabel(platform) }}
              </span>
              <span v-if="protocolsFor(row.id).length === 0" class="text-gray-400">—</span>
            </div>
          </template>

          <template #cell-balance="{ row }">
            <span class="font-mono text-sm">{{ formatBalance(row.balance) }}</span>
          </template>

          <template #cell-recharge_multiplier="{ row }">
            <div class="font-mono text-sm">{{ formatRate(row.recharge_multiplier) }}</div>
            <div class="text-[10px] text-gray-400">{{ rechargeSourceLabel(row.recharge_source) }}</div>
          </template>

          <template #cell-effective_rate="{ row }">
            <span v-if="lowestRate(row.id) != null" class="font-mono font-semibold text-emerald-700 dark:text-emerald-400">
              {{ formatRate(lowestRate(row.id) || 0) }}
            </span>
            <span v-else class="text-gray-400">—</span>
          </template>

          <template #cell-route_count="{ row }">
            <button type="button" class="text-sm font-medium text-primary-600 hover:underline dark:text-primary-400" @click="openRoutes(row)">
              {{ routesFor(row.id).length }}
            </button>
          </template>

          <template #cell-health_status="{ row }">
            <div class="flex items-center gap-2" :title="row.last_error || ''">
              <span :class="healthDot(row.health_status)"></span>
              <span class="text-xs">{{ healthLabel(row.health_status) }}</span>
            </div>
          </template>

          <template #cell-last_sync_at="{ row }">
            <span class="whitespace-nowrap text-xs text-gray-500 dark:text-dark-300">{{ formatDate(row.last_sync_at) }}</span>
          </template>

          <template #cell-enabled="{ row }">
            <Toggle :model-value="row.enabled" @update:model-value="toggleStation(row, $event)" />
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center justify-end gap-1">
              <button class="icon-button" type="button" :title="t('admin.upstreams.actions.sync')" :disabled="busyId === row.id" @click="syncStation(row)">
                <Icon name="sync" size="sm" :class="busyId === row.id ? 'animate-spin' : ''" />
              </button>
              <button class="icon-button" type="button" :title="t('admin.upstreams.actions.test')" :disabled="busyId === row.id" @click="testStation(row)">
                <Icon name="play" size="sm" />
              </button>
              <button class="icon-button" type="button" :title="t('admin.upstreams.actions.routes')" @click="openRoutes(row)">
                <Icon name="database" size="sm" />
              </button>
              <button class="icon-button" type="button" :title="t('admin.upstreams.actions.logs')" @click="openLogs(row)">
                <Icon name="document" size="sm" />
              </button>
              <button class="icon-button" type="button" :title="t('common.edit')" @click="openEdit(row)">
                <Icon name="edit" size="sm" />
              </button>
              <button class="icon-button-danger" type="button" :title="t('common.delete')" @click="askDelete(row)">
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('admin.upstreams.empty.title')"
              :description="t('admin.upstreams.empty.description')"
              :action-text="t('admin.upstreams.create')"
              @action="openCreate"
            />
          </template>
        </DataTable>
      </template>
    </TablePageLayout>

    <UpstreamStationFormDialog :show="showForm" :station="editing" @close="closeForm" @saved="reload" />
    <UpstreamRoutesDialog :show="showRoutes" :station="selected" @close="closeRoutes" @changed="reload" />
    <UpstreamLogsDialog :show="showLogs" :station="selected" @close="closeLogs" />
    <ConfirmDialog
      :show="showDelete"
      :title="t('common.delete')"
      :message="t('admin.upstreams.deleteConfirm', { name: deleting?.name || '' })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="showDelete = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'
import type { UpstreamHealth, UpstreamPlatform, UpstreamRoute, UpstreamSiteType, UpstreamStation } from '@/api/admin/upstreamStations'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import UpstreamStationFormDialog from '@/components/admin/upstream/UpstreamStationFormDialog.vue'
import UpstreamRoutesDialog from '@/components/admin/upstream/UpstreamRoutesDialog.vue'
import UpstreamLogsDialog from '@/components/admin/upstream/UpstreamLogsDialog.vue'

const { t, locale } = useI18n()
const appStore = useAppStore()
const stations = ref<UpstreamStation[]>([])
const routeMap = ref<Record<number, UpstreamRoute[]>>({})
const loading = ref(false)
const busyId = ref<number | null>(null)
const syncingAll = ref(false)
const search = ref('')
const siteTypeFilter = ref('')
const platformFilter = ref('')
const brandFilter = ref('')
const modelFilter = ref('')
const healthFilter = ref('')
const showForm = ref(false)
const editing = ref<UpstreamStation | null>(null)
const selected = ref<UpstreamStation | null>(null)
const showRoutes = ref(false)
const showLogs = ref(false)
const showDelete = ref(false)
const deleting = ref<UpstreamStation | null>(null)

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.upstreams.columns.name'), sortable: true },
  { key: 'protocols', label: t('admin.upstreams.columns.protocols'), sortable: false },
  { key: 'balance', label: t('admin.upstreams.columns.balance'), sortable: true },
  { key: 'recharge_multiplier', label: t('admin.upstreams.columns.rechargeMultiplier'), sortable: true },
  { key: 'effective_rate', label: t('admin.upstreams.columns.lowestRate'), sortable: false },
  { key: 'route_count', label: t('admin.upstreams.columns.routes'), sortable: false },
  { key: 'health_status', label: t('admin.upstreams.columns.health'), sortable: true },
  { key: 'last_sync_at', label: t('admin.upstreams.columns.lastSync'), sortable: true },
  { key: 'enabled', label: t('admin.upstreams.columns.enabled'), sortable: false },
  { key: 'actions', label: t('common.actions'), sortable: false },
])

const siteTypeFilterOptions = computed(() => [
  { value: '', label: t('admin.upstreams.filters.allTypes') },
  { value: 'auto', label: t('admin.upstreams.siteTypes.auto') },
  { value: 'newapi', label: 'NewAPI' },
  { value: 'sub2api', label: 'Sub2API' },
])
const platformFilterOptions = computed(() => [
  { value: '', label: t('admin.upstreams.filters.allProtocols') },
  ...(['openai', 'anthropic', 'gemini', 'grok'] as UpstreamPlatform[]).map(value => ({ value, label: platformLabel(value) })),
])
const brandFilterOptions = computed(() => [
  { value: '', label: t('admin.upstreams.filters.allBrands') },
  ...(['openai', 'claude', 'gemini', 'grok', 'other'] as ModelBrand[]).map(value => ({ value, label: t(`admin.upstreams.modelBrands.${value}`) })),
])
const modelFilterOptions = computed(() => {
  const models = [...new Set(Object.values(routeMap.value).flatMap(routes => routes.flatMap(route => route.models || [])))].sort()
  return [{ value: '', label: t('admin.upstreams.filters.allModels') }, ...models.map(value => ({ value, label: value }))]
})
const healthFilterOptions = computed(() => [
  { value: '', label: t('admin.upstreams.filters.allHealth') },
  { value: 'healthy', label: t('admin.upstreams.health.healthy') },
  { value: 'error', label: t('admin.upstreams.health.error') },
  { value: 'unknown', label: t('admin.upstreams.health.unknown') },
])
const filteredStations = computed(() => {
  const query = search.value.trim().toLowerCase()
  return stations.value.filter(station => {
    if (siteTypeFilter.value && station.site_type !== siteTypeFilter.value) return false
    if (healthFilter.value && station.health_status !== healthFilter.value) return false
    if (matchingRoutes(station.id).length === 0 && (platformFilter.value || brandFilter.value || modelFilter.value)) return false
    if (!query) return true
    return station.name.toLowerCase().includes(query)
      || station.base_url.toLowerCase().includes(query)
      || routesFor(station.id).some(route => (route.models || []).some(model => model.toLowerCase().includes(query)))
  })
})
const totalRoutes = computed(() => Object.values(routeMap.value).reduce((sum, routes) => sum + routes.length, 0))

onMounted(reload)

async function reload() {
  loading.value = true
  try {
    const items = await adminAPI.upstreamStations.list()
    stations.value = items || []
    const routeEntries = await Promise.all(items.map(async station => {
      try {
        return [station.id, await adminAPI.upstreamStations.listRoutes(station.id)] as const
      } catch {
        return [station.id, []] as const
      }
    }))
    routeMap.value = Object.fromEntries(routeEntries)
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.upstreams.messages.loadFailed')))
  } finally {
    loading.value = false
  }
}

function routesFor(id: number): UpstreamRoute[] {
  return routeMap.value[id] || []
}
type ModelBrand = 'openai' | 'claude' | 'gemini' | 'grok' | 'other'
function modelBrand(model: string): ModelBrand {
  const value = model.toLowerCase()
  if (value.includes('claude')) return 'claude'
  if (value.includes('gemini')) return 'gemini'
  if (value.includes('grok')) return 'grok'
  if (/^(gpt-|chatgpt-|o[134]-|codex)/.test(value)) return 'openai'
  return 'other'
}
function matchingRoutes(id: number): UpstreamRoute[] {
  return routesFor(id).filter(route => {
    if (platformFilter.value && route.platform !== platformFilter.value) return false
    if (modelFilter.value && !(route.models || []).includes(modelFilter.value)) return false
    if (brandFilter.value && !(route.models || []).some(model => modelBrand(model) === brandFilter.value)) return false
    return true
  })
}
function protocolsFor(id: number): UpstreamPlatform[] {
  return [...new Set(routesFor(id).map(route => route.platform))]
}
function lowestRate(id: number): number | null {
  const rates = matchingRoutes(id).filter(route => route.schedulable && route.health_status !== 'error').map(route => route.effective_rate)
  return rates.length ? Math.min(...rates) : null
}
function openCreate() { editing.value = null; showForm.value = true }
function openEdit(station: UpstreamStation) { editing.value = station; showForm.value = true }
function closeForm() { showForm.value = false; editing.value = null }
function openRoutes(station: UpstreamStation) { selected.value = station; showRoutes.value = true }
function closeRoutes() { showRoutes.value = false; selected.value = null }
function openLogs(station: UpstreamStation) { selected.value = station; showLogs.value = true }
function closeLogs() { showLogs.value = false; selected.value = null }
function askDelete(station: UpstreamStation) { deleting.value = station; showDelete.value = true }

async function syncStation(station: UpstreamStation) {
  if (busyId.value != null) return
  busyId.value = station.id
  try {
    const result = await adminAPI.upstreamStations.sync(station.id)
    await reload()
    appStore.showSuccess(t('admin.upstreams.messages.syncSuccess', { routes: result.synced_routes }))
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.upstreams.messages.syncFailed')))
  } finally {
    busyId.value = null
  }
}

async function syncAll() {
  if (syncingAll.value) return
  syncingAll.value = true
  try {
    await adminAPI.upstreamStations.syncAll()
    await reload()
    appStore.showSuccess(t('admin.upstreams.messages.syncAllSuccess'))
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.upstreams.messages.syncFailed')))
  } finally {
    syncingAll.value = false
  }
}

async function testStation(station: UpstreamStation) {
  if (busyId.value != null) return
  busyId.value = station.id
  try {
    await adminAPI.upstreamStations.test(station.id)
    await reload()
    appStore.showSuccess(t('admin.upstreams.messages.testSuccess'))
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.upstreams.messages.testFailed')))
  } finally {
    busyId.value = null
  }
}

async function toggleStation(station: UpstreamStation, value: boolean) {
  const previous = station.enabled
  station.enabled = value
  try {
    await adminAPI.upstreamStations.update(station.id, { enabled: value })
  } catch (error: unknown) {
    station.enabled = previous
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  }
}

async function confirmDelete() {
  if (!deleting.value) return
  try {
    await adminAPI.upstreamStations.del(deleting.value.id)
    showDelete.value = false
    deleting.value = null
    await reload()
    appStore.showSuccess(t('admin.upstreams.messages.deleted'))
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  }
}

function formatRate(value: number): string {
  return Number(value || 0).toFixed(8).replace(/0+$/, '').replace(/\.$/, '') || '0'
}
function formatBalance(value?: number | null): string {
  return value == null ? '—' : Number(value).toFixed(4).replace(/0+$/, '').replace(/\.$/, '')
}
function formatDate(value?: string | null): string {
  if (!value) return '—'
  return new Intl.DateTimeFormat(locale.value, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(value))
}
function siteTypeLabel(type: UpstreamSiteType): string {
  return type === 'auto' ? t('admin.upstreams.siteTypes.auto') : type === 'newapi' ? 'NewAPI' : 'Sub2API'
}
function platformLabel(platform: UpstreamPlatform): string {
  return platform === 'anthropic' ? 'Claude' : platform === 'openai' ? 'OpenAI' : platform === 'gemini' ? 'Gemini' : 'Grok'
}
function platformBadge(platform: UpstreamPlatform): string {
  const base = 'inline-flex rounded-md px-2 py-1 text-xs font-medium'
  if (platform === 'openai') return `${base} bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300`
  if (platform === 'anthropic') return `${base} bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300`
  if (platform === 'gemini') return `${base} bg-blue-50 text-blue-700 dark:bg-blue-500/10 dark:text-blue-300`
  return `${base} bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200`
}
function rechargeSourceLabel(source: 'manual' | 'auto'): string {
  return t(`admin.upstreams.rechargeSources.${source}`)
}
function healthLabel(health: UpstreamHealth): string {
  return t(`admin.upstreams.health.${health}`)
}
function healthDot(health: UpstreamHealth): string {
  return health === 'healthy' ? 'h-2 w-2 rounded-full bg-emerald-500' : health === 'error' ? 'h-2 w-2 rounded-full bg-red-500' : 'h-2 w-2 rounded-full bg-gray-400'
}
</script>

<style scoped>
.icon-button,
.icon-button-danger {
  @apply inline-flex h-8 w-8 items-center justify-center rounded-md text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 disabled:cursor-not-allowed disabled:opacity-40 dark:text-dark-300 dark:hover:bg-dark-700 dark:hover:text-white;
}

.icon-button-danger {
  @apply hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-500/10 dark:hover:text-red-400;
}
</style>
