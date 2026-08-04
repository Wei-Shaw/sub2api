<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { opsAPI, type OpsErrorListView, type OpsErrorLog } from '@/api/admin/ops'
import Icon from '@/components/icons/Icon.vue'
import Select from '@/components/common/Select.vue'
import { useAppStore } from '@/stores'
import OpsErrorLogTable from './OpsErrorLogTable.vue'

const props = withDefaults(defineProps<{
  platformFilter?: string
  groupIdFilter?: number | null
  refreshToken?: number
}>(), {
  platformFilter: '',
  groupIdFilter: null,
  refreshToken: 0
})

const emit = defineEmits<{
  (e: 'openErrorDetail', id: number, kind: 'request' | 'upstream'): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const rows = ref<OpsErrorLog[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const sortBy = ref('created_at')
const sortOrder = ref<'asc' | 'desc'>('desc')

const filters = reactive({
  timeRange: '24h',
  query: '',
  statusCode: '' as string,
  phase: '',
  owner: '',
  view: 'errors' as OpsErrorListView,
  resolved: ''
})

const timeRangeOptions = computed(() => [
  { value: '1h', label: t('admin.ops.timeRange.1h') },
  { value: '6h', label: t('admin.ops.timeRange.6h') },
  { value: '24h', label: t('admin.ops.timeRange.24h') },
  { value: '7d', label: t('admin.ops.timeRange.7d') },
  { value: '30d', label: t('admin.ops.timeRange.30d') }
])

const statusOptions = computed(() => [
  { value: '', label: t('common.all') },
  ...[400, 401, 403, 404, 409, 422, 429, 500, 502, 503, 504, 529].map((code) => ({
    value: String(code),
    label: String(code)
  }))
])

const phaseOptions = computed(() => [
  { value: '', label: t('common.all') },
  ...['request', 'auth', 'account_auth', 'routing', 'upstream', 'network', 'internal'].map((phase) => ({
    value: phase,
    label: t(`admin.ops.errorDetails.phase.${phase}`)
  }))
])

const ownerOptions = computed(() => [
  { value: '', label: t('common.all') },
  { value: 'provider', label: t('admin.ops.errorDetails.owner.provider') },
  { value: 'client', label: t('admin.ops.errorDetails.owner.client') },
  { value: 'platform', label: t('admin.ops.errorDetails.owner.platform') }
])

const viewOptions = computed(() => [
  { value: 'errors', label: t('admin.ops.errorDetails.viewErrors') },
  { value: 'excluded', label: t('admin.ops.errorDetails.viewExcluded') },
  { value: 'all', label: t('common.all') }
])

const resolutionOptions = computed(() => [
  { value: '', label: t('common.all') },
  { value: 'false', label: t('admin.ops.errorExplorer.unresolved') },
  { value: 'true', label: t('admin.ops.errorExplorer.resolved') }
])

function buildQuery() {
  const query: Record<string, string | number> = {
    page: page.value,
    page_size: pageSize.value,
    time_range: filters.timeRange,
    view: filters.view,
    sort_by: sortBy.value,
    sort_order: sortOrder.value
  }

  if (props.platformFilter.trim()) query.platform = props.platformFilter.trim()
  if (typeof props.groupIdFilter === 'number' && props.groupIdFilter > 0) query.group_id = props.groupIdFilter
  if (filters.query.trim()) query.q = filters.query.trim()
  if (filters.statusCode) query.status_codes = filters.statusCode
  if (filters.phase) query.phase = filters.phase
  if (filters.owner) query.error_owner = filters.owner
  if (filters.resolved) query.resolved = filters.resolved
  return query
}

async function fetchErrorLogs() {
  loading.value = true
  try {
    const result = await opsAPI.listErrorLogs(buildQuery())
    rows.value = result.items || []
    total.value = result.total || 0
  } catch (err: any) {
    console.error('[OpsErrorLogExplorer] Failed to fetch error logs', err)
    rows.value = []
    total.value = 0
    appStore.showError(err?.response?.data?.detail || t('admin.ops.errorExplorer.loadFailed'))
  } finally {
    loading.value = false
  }
}

function applyFilters() {
  page.value = 1
  void fetchErrorLogs()
}

function resetFilters() {
  filters.timeRange = '24h'
  filters.query = ''
  filters.statusCode = ''
  filters.phase = ''
  filters.owner = ''
  filters.view = 'errors'
  filters.resolved = ''
  sortBy.value = 'created_at'
  sortOrder.value = 'desc'
  applyFilters()
}

function onSort(nextSortBy: string, nextSortOrder: 'asc' | 'desc') {
  sortBy.value = nextSortBy
  sortOrder.value = nextSortOrder
  applyFilters()
}

function openErrorDetail(id: number) {
  const row = rows.value.find((item) => item.id === id)
  const kind = String(row?.phase || '').toLowerCase() === 'upstream' ? 'upstream' : 'request'
  emit('openErrorDetail', id, kind)
}

watch(() => [page.value, pageSize.value] as const, () => {
  void fetchErrorLogs()
})

watch(() => [props.platformFilter, props.groupIdFilter, props.refreshToken] as const, () => {
  page.value = 1
  void fetchErrorLogs()
})

onMounted(() => {
  void fetchErrorLogs()
})
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="flex items-baseline gap-3">
        <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.errorExplorer.title') }}</h2>
        <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.errorExplorer.total', { total }) }}</span>
      </div>
      <button
        type="button"
        class="inline-flex h-8 w-8 items-center justify-center rounded-md text-gray-500 transition hover:bg-gray-100 hover:text-gray-900 disabled:opacity-50 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white"
        :disabled="loading"
        :title="t('common.refresh')"
        @click="fetchErrorLogs"
      >
        <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
      </button>
    </div>

    <div class="grid grid-cols-2 gap-2 md:grid-cols-4 xl:grid-cols-8">
      <Select v-model="filters.timeRange" :options="timeRangeOptions" />
      <div class="col-span-2">
        <input
          v-model="filters.query"
          type="search"
          class="input h-9 text-sm"
          :placeholder="t('admin.ops.errorExplorer.searchPlaceholder')"
          @keyup.enter="applyFilters"
        />
      </div>
      <Select v-model="filters.statusCode" :options="statusOptions" />
      <Select v-model="filters.phase" :options="phaseOptions" />
      <Select v-model="filters.owner" :options="ownerOptions" />
      <Select v-model="filters.view" :options="viewOptions" />
      <Select v-model="filters.resolved" :options="resolutionOptions" />
    </div>

    <div class="flex justify-end gap-2">
      <button type="button" class="btn btn-secondary btn-sm" @click="resetFilters">{{ t('common.reset') }}</button>
      <button type="button" class="btn btn-primary btn-sm" @click="applyFilters">{{ t('common.search') }}</button>
    </div>

    <OpsErrorLogTable
      :rows="rows"
      :total="total"
      :loading="loading"
      :page="page"
      :page-size="pageSize"
      @openErrorDetail="openErrorDetail"
      @sort="onSort"
      @update:page="page = $event"
      @update:pageSize="pageSize = $event"
    />
  </section>
</template>
