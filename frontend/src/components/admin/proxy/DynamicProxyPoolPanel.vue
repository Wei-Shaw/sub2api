<template>
  <div class="flex min-h-0 flex-1 flex-col gap-4">
    <div class="flex flex-wrap items-center gap-3">
      <div class="relative w-full sm:w-64">
        <Icon
          name="search"
          size="md"
          class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
        />
        <input
          v-model="search"
          type="text"
          class="input pl-10"
          :placeholder="t('admin.proxies.pools.searchPlaceholder')"
          @input="handleSearch"
        />
      </div>
      <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
        <button type="button" class="btn btn-secondary" :disabled="loading" @click="loadPools">
          <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
        </button>
        <button type="button" class="btn btn-primary" @click="openCreate">
          <Icon name="plus" size="md" class="mr-2" />
          {{ t('admin.proxies.pools.create') }}
        </button>
      </div>
    </div>

    <div class="min-h-0 flex-1 overflow-auto rounded-lg border border-gray-200 dark:border-dark-600">
      <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-600">
        <thead class="bg-gray-50 dark:bg-dark-800">
          <tr>
            <th class="px-4 py-3 text-left font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.proxies.pools.columns.name') }}
            </th>
            <th class="px-4 py-3 text-left font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.proxies.pools.columns.protocol') }}
            </th>
            <th class="px-4 py-3 text-left font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.proxies.pools.columns.alive') }}
            </th>
            <th class="px-4 py-3 text-left font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.proxies.pools.columns.interval') }}
            </th>
            <th class="px-4 py-3 text-left font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.proxies.pools.columns.duration') }}
            </th>
            <th class="px-4 py-3 text-left font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.proxies.pools.columns.lastExtract') }}
            </th>
            <th class="px-4 py-3 text-right font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.proxies.pools.columns.actions') }}
            </th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-900">
          <tr v-if="loading && pools.length === 0">
            <td colspan="7" class="px-4 py-10 text-center text-gray-400">
              {{ t('common.loading') }}
            </td>
          </tr>
          <tr v-else-if="pools.length === 0">
            <td colspan="7" class="px-4 py-10 text-center text-gray-400">
              {{ t('admin.proxies.pools.empty') }}
            </td>
          </tr>
          <tr v-for="row in pools" :key="row.id" class="hover:bg-gray-50 dark:hover:bg-dark-800/60">
            <td class="px-4 py-3">
              <div class="flex items-center gap-2">
                <span class="font-medium text-gray-900 dark:text-gray-100">{{ row.name }}</span>
                <span
                  v-if="!row.enabled"
                  class="rounded bg-amber-100 px-1.5 py-0.5 text-xs text-amber-700 dark:bg-amber-900/40 dark:text-amber-300"
                >
                  {{ t('admin.proxies.pools.disabled') }}
                </span>
              </div>
              <div class="mt-0.5 max-w-xs truncate text-xs text-gray-400" :title="row.extract_url">
                {{ row.extract_url }}
              </div>
            </td>
            <td class="px-4 py-3 uppercase text-gray-600 dark:text-gray-300">{{ row.protocol }}</td>
            <td class="px-4 py-3">
              <span :class="row.alive_count >= row.min_alive ? 'text-green-600' : 'text-orange-600'">
                {{ row.alive_count }} / {{ row.min_alive }}
              </span>
            </td>
            <td class="px-4 py-3 text-gray-600 dark:text-gray-300">{{ row.refresh_interval_sec }}s</td>
            <td class="px-4 py-3 text-gray-600 dark:text-gray-300">{{ row.ip_duration_sec }}s</td>
            <td class="px-4 py-3 text-xs text-gray-600 dark:text-gray-300">
              <div v-if="row.last_extract_at">{{ formatDateTime(row.last_extract_at) }}</div>
              <span v-if="row.last_extract_status === 'success'" class="text-green-600">
                {{ t('admin.proxies.pools.statusSuccess') }}
              </span>
              <span
                v-else-if="row.last_extract_status === 'error'"
                class="text-red-600"
                :title="row.last_extract_error"
              >
                {{ t('admin.proxies.pools.statusError') }}
              </span>
              <span v-else class="text-gray-400">{{ t('admin.proxies.pools.statusIdle') }}</span>
            </td>
            <td class="px-4 py-3">
              <div class="flex items-center justify-end gap-2">
                <button
                  type="button"
                  class="btn btn-sm btn-secondary"
                  :disabled="extractingId === row.id"
                  :title="t('admin.proxies.pools.extractNow')"
                  @click="triggerExtract(row)"
                >
                  <Icon name="refresh" size="sm" :class="extractingId === row.id ? 'animate-spin' : ''" />
                  <span class="ml-1 hidden sm:inline">
                    {{
                      extractingId === row.id
                        ? t('admin.proxies.pools.extracting')
                        : t('admin.proxies.pools.extractNow')
                    }}
                  </span>
                </button>
                <button
                  v-if="row.source_type === 'subscription'"
                  type="button"
                  class="btn btn-sm btn-secondary"
                  @click="openNodes(row)"
                >
                  <Icon name="grid" size="sm" />
                  <span class="ml-1 hidden sm:inline">{{ t('admin.proxies.pools.nodes') }}</span>
                </button>
                <button type="button" class="btn btn-sm btn-secondary" @click="managePool(row)">
                  <Icon name="cog" size="sm" />
                  <span class="ml-1 hidden sm:inline">{{ t('admin.proxies.pools.manage') }}</span>
                </button>
                <button type="button" class="btn btn-sm btn-secondary" @click="editPool(row)">
                  <Icon name="edit" size="sm" />
                </button>
                <button type="button" class="btn btn-sm btn-danger" @click="confirmDelete(row)">
                  <Icon name="trash" size="sm" />
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <Pagination
      v-if="total > pageSize"
      :total="total"
      :page="page"
      :page-size="pageSize"
      @change="handlePageChange"
    />

    <BaseDialog
      :show="showCreateModal"
      :title="t('admin.proxies.pools.create')"
      width="wide"
      @close="showCreateModal = false"
    >
      <PoolFormFields
        :model-value="createForm"
        :auth-mode-options="authModeOptions"
        :format-options="formatOptions"
        :protocol-options="protocolOptions"
        :source-type-options="sourceTypeOptions"
        :subscription-options="subscriptionOptions"
        @update:model-value="Object.assign(createForm, $event)"
      />
      <template #actions>
        <button type="button" class="btn btn-secondary" @click="showCreateModal = false">
          {{ t('common.cancel') }}
        </button>
        <button type="button" class="btn btn-primary" :disabled="saving" @click="createPool">
          {{ saving ? '...' : t('common.save') }}
        </button>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="showEditModal"
      :title="t('admin.proxies.pools.edit')"
      width="wide"
      @close="showEditModal = false"
    >
      <div class="mb-4 flex items-center gap-2">
        <input id="pool-enabled" v-model="editForm.enabled" type="checkbox" class="toggle" />
        <label for="pool-enabled" class="text-sm text-gray-700 dark:text-gray-200">
          {{ t('admin.proxies.pools.form.enabled') }}
        </label>
      </div>
      <PoolFormFields
        :model-value="editForm"
        :auth-mode-options="authModeOptions"
        :format-options="formatOptions"
        :protocol-options="protocolOptions"
        :source-type-options="sourceTypeOptions"
        :subscription-options="subscriptionOptions"
        @update:model-value="Object.assign(editForm, $event)"
      />
      <template #actions>
        <button type="button" class="btn btn-secondary" @click="showEditModal = false">
          {{ t('common.cancel') }}
        </button>
        <button type="button" class="btn btn-primary" :disabled="saving" @click="saveEdit">
          {{ saving ? '...' : t('common.save') }}
        </button>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="showDeleteConfirm"
      :title="t('admin.proxies.pools.delete')"
      :message="deleteMessage"
      @confirm="doDelete"
      @cancel="showDeleteConfirm = false"
    />

    <BaseDialog
      :show="showResultModal"
      :title="t('admin.proxies.pools.extractResult')"
      width="narrow"
      @close="showResultModal = false"
    >
      <div v-if="extractResult" class="space-y-2 text-sm">
        <div>
          {{ t('admin.proxies.pools.extractCreated') }}:
          <strong>{{ extractResult.created }}</strong>
        </div>
        <div>
          {{ t('admin.proxies.pools.extractFailed') }}:
          <strong>{{ extractResult.failed }}</strong>
        </div>
        <div>
          {{ t('admin.proxies.pools.extractAlive') }}:
          <strong>{{ extractResult.alive_count }}</strong>
        </div>
      </div>
      <template #actions>
        <button type="button" class="btn btn-primary" @click="showResultModal = false">
          {{ t('common.confirm') }}
        </button>
      </template>
    </BaseDialog>

    <!-- Nodes preview modal (subscription pools) -->
    <BaseDialog
      :show="showNodesModal"
      :title="t('admin.proxies.pools.nodesTitle')"
      width="wide"
      @close="showNodesModal = false"
    >
      <div class="space-y-3">
        <div v-if="nodesLoading" class="py-6 text-center text-sm text-gray-400">
          {{ t('admin.proxies.pools.nodesLoading') }}
        </div>
        <div v-else-if="nodes.length === 0" class="py-6 text-center text-sm text-gray-400">
          {{ t('admin.proxies.pools.noNodes') }}
        </div>
        <template v-else>
          <div class="mb-2 flex items-center justify-between">
            <span class="text-sm text-gray-500">{{ nodes.length }} {{ t('admin.proxies.pools.columns.name') }}</span>
            <button
              type="button"
              class="btn btn-primary btn-sm"
              :disabled="selectedNodeIds.length === 0"
              @click="addSelectedNodes"
            >
              {{ t('admin.proxies.pools.addNodes') }} ({{ selectedNodeIds.length }})
            </button>
          </div>
          <div class="max-h-72 overflow-auto rounded border border-gray-200 dark:border-dark-600">
            <table class="min-w-full divide-y divide-gray-100 text-xs dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800">
                <tr>
                  <th class="w-8 px-3 py-2"></th>
                  <th class="px-3 py-2 text-left font-medium text-gray-500">{{ t('admin.proxies.pools.columns.name') }}</th>
                  <th class="px-3 py-2 text-left font-medium text-gray-500">{{ t('admin.proxies.pools.columns.protocol') }}</th>
                  <th class="px-3 py-2 text-left font-medium text-gray-500">{{ t('admin.proxies.pools.address') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-50 dark:divide-dark-800">
                <tr v-for="node in nodes" :key="node.identity" class="hover:bg-gray-50 dark:hover:bg-dark-800/60">
                  <td class="px-3 py-1.5">
                    <input type="checkbox" :value="node.identity" v-model="selectedNodeIds" class="accent-primary-500" />
                  </td>
                  <td class="px-3 py-1.5 text-gray-700 dark:text-gray-300">{{ node.name }}</td>
                  <td class="px-3 py-1.5 uppercase text-gray-500">{{ node.type }}</td>
                  <td class="px-3 py-1.5 font-mono text-gray-500">{{ node.server }}:{{ node.port }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </template>
      </div>
    </BaseDialog>

    <!-- Manage pool proxies -->
    <BaseDialog
      :show="showManageModal"
      :title="t('admin.proxies.pools.manageTitle')"
      width="wide"
      @close="showManageModal = false"
    >
      <div class="space-y-4">
        <div class="flex items-center justify-between">
          <div class="text-sm text-gray-600 dark:text-gray-300">
            {{ t('admin.proxies.pools.manageHint') }}
          </div>
          <div class="flex gap-2">
            <button type="button" class="btn btn-sm btn-secondary" @click="loadAllProxies">
              {{ t('admin.proxies.pools.loadFromIpList') }}
            </button>
          </div>
        </div>

        <!-- Pool proxies -->
        <div v-if="poolProxies.length > 0">
          <div class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-200">
            {{ t('admin.proxies.pools.poolProxies') }} ({{ poolProxies.length }})
          </div>
          <div class="max-h-48 overflow-auto rounded border border-gray-200 dark:border-dark-600">
            <table class="min-w-full divide-y divide-gray-200 text-xs dark:divide-dark-600">
              <thead class="bg-gray-50 dark:bg-dark-800">
                <tr>
                  <th class="px-3 py-2 text-left font-medium text-gray-500 dark:text-gray-400">#</th>
                  <th class="px-3 py-2 text-left font-medium text-gray-500 dark:text-gray-400">{{ t('admin.proxies.pools.columns.name') }}</th>
                  <th class="px-3 py-2 text-left font-medium text-gray-500 dark:text-gray-400">{{ t('admin.proxies.pools.columns.protocol') }}</th>
                  <th class="px-3 py-2 text-left font-medium text-gray-500 dark:text-gray-400">{{ t('admin.proxies.pools.address') }}</th>
                  <th class="px-3 py-2 text-left font-medium text-gray-500 dark:text-gray-400">{{ t('admin.proxies.pools.columns.status') }}</th>
                  <th class="px-3 py-2 text-right font-medium text-gray-500 dark:text-gray-400">{{ t('admin.proxies.pools.actions') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr v-for="p in poolProxies" :key="p.id">
                  <td class="px-3 py-1.5 text-gray-400">{{ p.id }}</td>
                  <td class="px-3 py-1.5 text-gray-700 dark:text-gray-300">{{ p.name }}</td>
                  <td class="px-3 py-1.5 uppercase text-gray-500">{{ p.protocol }}</td>
                  <td class="px-3 py-1.5 text-gray-500">{{ p.host }}:{{ p.port }}</td>
                  <td class="px-3 py-1.5">
                    <span :class="p.status === 'active' ? 'text-green-600' : 'text-gray-400'">{{ p.status }}</span>
                  </td>
                  <td class="px-3 py-1.5 text-right">
                    <div class="flex items-center justify-end gap-1">
                      <button
                        class="btn btn-xs btn-secondary"
                        :disabled="testingId === p.id"
                        @click="testPoolProxy(p)"
                      >
                        <Icon name="play" size="xs" :class="testingId === p.id ? 'animate-spin' : ''" />
                        {{ t('admin.proxies.pools.testProxy') }}
                      </button>
                      <span v-if="p.latency !== undefined" class="text-xs text-gray-400">
                        {{ t('admin.proxies.pools.testLatency', { ms: p.latency }) }}
                      </span>
                      <button class="btn btn-xs btn-danger" @click="removePoolProxy(p)">{{ t('common.delete') }}</button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <!-- IP list proxies to add -->
        <div v-if="allProxies.length > 0">
          <div class="mb-2 flex items-center justify-between">
            <div class="text-sm font-medium text-gray-700 dark:text-gray-200">
              {{ t('admin.proxies.pools.ipListProxies') }} ({{ allProxies.length }})
            </div>
            <button type="button" class="btn btn-xs btn-primary" :disabled="selectedAddIds.length === 0" @click="addSelectedProxies">
              {{ t('admin.proxies.pools.addSelected') }} ({{ selectedAddIds.length }})
            </button>
          </div>
          <div class="max-h-48 overflow-auto rounded border border-gray-200 dark:border-dark-600">
            <table class="min-w-full divide-y divide-gray-200 text-xs dark:divide-dark-600">
              <thead class="bg-gray-50 dark:bg-dark-800">
                <tr>
                  <th class="w-8 px-3 py-2"></th>
                  <th class="px-3 py-2 text-left font-medium text-gray-500 dark:text-gray-400">#</th>
                  <th class="px-3 py-2 text-left font-medium text-gray-500 dark:text-gray-400">{{ t('admin.proxies.pools.columns.name') }}</th>
                  <th class="px-3 py-2 text-left font-medium text-gray-500 dark:text-gray-400">{{ t('admin.proxies.pools.columns.protocol') }}</th>
                  <th class="px-3 py-2 text-left font-medium text-gray-500 dark:text-gray-400">{{ t('admin.proxies.pools.address') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr v-for="p in allProxies" :key="p.id">
                  <td class="px-3 py-1.5">
                    <input type="checkbox" :value="p.id" v-model="selectedAddIds" class="accent-primary-500" />
                  </td>
                  <td class="px-3 py-1.5 text-gray-400">{{ p.id }}</td>
                  <td class="px-3 py-1.5 text-gray-700 dark:text-gray-300">{{ p.name }}</td>
                  <td class="px-3 py-1.5 uppercase text-gray-500">{{ p.protocol }}</td>
                  <td class="px-3 py-1.5 text-gray-500">{{ p.host }}:{{ p.port }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </BaseDialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { dynamicProxyPoolsAPI } from '@/api/admin/dynamicProxyPools'
import type { DynamicProxyPool } from '@/types'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import PoolFormFields from './DynamicProxyPoolFormFields.vue'

const { t } = useI18n()
const appStore = useAppStore()

const pools = ref<DynamicProxyPool[]>([])
const loading = ref(false)
const saving = ref(false)
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const search = ref('')
const searchTimer = ref<ReturnType<typeof setTimeout> | null>(null)
const extractingId = ref<number | null>(null)

const showCreateModal = ref(false)
const showEditModal = ref(false)
const showDeleteConfirm = ref(false)
const showResultModal = ref(false)
const showManageModal = ref(false)
const selectedPool = ref<DynamicProxyPool | null>(null)
const extractResult = ref<{ created: number; failed: number; alive_count: number } | null>(null)
const deleteMessage = ref('')
const poolProxies = ref<any[]>([])
const allProxies = ref<any[]>([])
const selectedAddIds = ref<number[]>([])
const managingPoolId = ref<number | null>(null)
const testingId = ref<number | null>(null)
const showNodesModal = ref(false)
const nodesLoading = ref(false)
const nodes = ref<Array<{ identity: string; name: string; type: string; server: string; port: string }>>([])
const selectedNodeIds = ref<string[]>([])
const nodesPoolId = ref<number | null>(null)

const protocolOptions = computed(() => [
  { value: 'http', label: 'HTTP' },
  { value: 'https', label: 'HTTPS' },
  { value: 'socks5', label: 'SOCKS5' },
  { value: 'socks5h', label: 'SOCKS5H' }
])

const authModeOptions = computed(() => [
  { value: 'none', label: t('admin.proxies.pools.form.authNone') },
  { value: 'fixed', label: t('admin.proxies.pools.form.authFixed') },
  { value: 'from_response', label: t('admin.proxies.pools.form.authFromResponse') }
])

const formatOptions = computed(() => [
  { value: 'txt', label: 'TXT' },
  { value: 'json', label: 'JSON' }
])

const sourceTypeOptions = [
  { value: 'extract_api', label: t('admin.proxies.pools.form.sourceExtractApi') },
  { value: 'subscription', label: t('admin.proxies.pools.form.sourceSubscription') }
]

const subscriptionOptions = ref<Array<{ value: number | null; label: string }>>([])
const subscriptionOptionsLoading = ref(false)

async function loadSubscriptionOptions() {
  subscriptionOptionsLoading.value = true
  try {
    const { proxySubscriptionsAPI } = await import('@/api/admin')
    const res = await proxySubscriptionsAPI.list({ page_size: 200 })
    const items = Array.isArray(res?.items) ? res.items : []
    subscriptionOptions.value = items.map((s: any) => ({
      value: s.id ?? s.ID,
      label: `${s.name ?? s.Name} (${s.name_prefix ?? s.NamePrefix})`
    }))
  } catch {
    subscriptionOptions.value = []
  } finally {
    subscriptionOptionsLoading.value = false
  }
}

type PoolForm = {
  name: string
  enabled?: boolean
  source_type: string
  subscription_id: number | null
  extract_url: string
  protocol: string
  auth_mode: string
  username: string
  password: string
  response_format: string
  line_separator: string
  ip_field_path: string
  port_field_path: string
  refresh_interval_sec: number
  ip_duration_sec: number
  extract_count: number
  min_alive: number
  health_check_interval_sec: number
}

function defaultForm(): PoolForm {
  return {
    name: '',
    enabled: true,
    source_type: 'extract_api',
    subscription_id: null,
    extract_url: '',
    protocol: 'http',
    auth_mode: 'from_response',
    username: '',
    password: '',
    response_format: 'json',
    line_separator: '\\r\\n',
    ip_field_path: 'ip',
    port_field_path: 'port',
    refresh_interval_sec: 300,
    ip_duration_sec: 300,
    extract_count: 1,
    min_alive: 1,
    health_check_interval_sec: 0
  }
}

const createForm = reactive<PoolForm>(defaultForm())
const editForm = reactive<PoolForm>(defaultForm())

function pick<T = any>(obj: Record<string, any>, ...keys: string[]): T | undefined {
  for (const k of keys) {
    if (obj[k] !== undefined && obj[k] !== null) return obj[k] as T
  }
  return undefined
}

function normalizePool(raw: Record<string, any>): DynamicProxyPool {
  return {
    id: Number(pick(raw, 'id', 'ID') ?? 0),
    name: String(pick(raw, 'name', 'Name') ?? ''),
    enabled: Boolean(pick(raw, 'enabled', 'Enabled') ?? true),
    source_type: String(pick(raw, 'source_type', 'SourceType') ?? 'extract_api'),
    subscription_id: (pick(raw, 'subscription_id', 'SubscriptionID') as number | null) ?? null,
    extract_url: String(pick(raw, 'extract_url', 'ExtractURL') ?? ''),
    protocol: String(pick(raw, 'protocol', 'Protocol') ?? 'http'),
    auth_mode: String(pick(raw, 'auth_mode', 'AuthMode') ?? 'none'),
    username: String(pick(raw, 'username', 'Username') ?? ''),
    password: String(pick(raw, 'password', 'Password') ?? ''),
    response_format: String(pick(raw, 'response_format', 'ResponseFormat') ?? 'txt'),
    line_separator: String(pick(raw, 'line_separator', 'LineSeparator') ?? '\\r\\n'),
    ip_field_path: String(pick(raw, 'ip_field_path', 'IPFieldPath') ?? ''),
    port_field_path: String(pick(raw, 'port_field_path', 'PortFieldPath') ?? ''),
    refresh_interval_sec: Number(pick(raw, 'refresh_interval_sec', 'RefreshIntervalSec') ?? 300),
    ip_duration_sec: Number(pick(raw, 'ip_duration_sec', 'IPDurationSec') ?? 300),
    extract_count: Number(pick(raw, 'extract_count', 'ExtractCount') ?? 1),
    min_alive: Number(pick(raw, 'min_alive', 'MinAlive') ?? 1),
    health_check_interval_sec: Number(pick(raw, 'health_check_interval_sec', 'HealthCheckIntervalSec') ?? 0),
    name_prefix: String(pick(raw, 'name_prefix', 'NamePrefix') ?? ''),
    last_extract_at: (pick(raw, 'last_extract_at', 'LastExtractAt') as string | null) ?? null,
    last_extract_status: String(pick(raw, 'last_extract_status', 'LastExtractStatus') ?? ''),
    last_extract_error: String(pick(raw, 'last_extract_error', 'LastExtractError') ?? ''),
    alive_count: Number(pick(raw, 'alive_count', 'AliveCount') ?? 0),
    created_at: String(pick(raw, 'created_at', 'CreatedAt') ?? ''),
    updated_at: String(pick(raw, 'updated_at', 'UpdatedAt') ?? '')
  }
}

function formatDateTime(ts: string) {
  const d = new Date(ts)
  if (Number.isNaN(d.getTime())) return ts
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function handleSearch() {
  if (searchTimer.value) clearTimeout(searchTimer.value)
  searchTimer.value = setTimeout(() => {
    page.value = 1
    loadPools()
  }, 300)
}

async function loadPools() {
  loading.value = true
  try {
    const res = await dynamicProxyPoolsAPI.list({
      page: page.value,
      page_size: pageSize.value,
      search: search.value || undefined
    })
    const items = Array.isArray(res?.items) ? res.items : []
    pools.value = items.map((it) => normalizePool(it as unknown as Record<string, any>))
    total.value = Number(res?.total ?? 0)
  } catch (e: any) {
    appStore.showError(e?.message || t('admin.proxies.pools.failedToLoad'))
  } finally {
    loading.value = false
  }
}

function handlePageChange(p: number) {
  page.value = p
  loadPools()
}

function openCreate() {
  Object.assign(createForm, defaultForm())
  showCreateModal.value = true
}

async function createPool() {
  if (!createForm.name.trim()) {
    appStore.showError(t('admin.proxies.pools.nameRequired'))
    return
  }
  if (createForm.source_type === 'subscription') {
    if (!createForm.subscription_id) {
      appStore.showError(t('admin.proxies.pools.form.subscription') + ' required')
      return
    }
  } else if (!createForm.extract_url.trim()) {
    appStore.showError(t('admin.proxies.pools.urlRequired'))
    return
  }
  saving.value = true
  try {
    await dynamicProxyPoolsAPI.create({ ...createForm })
    showCreateModal.value = false
    appStore.showSuccess(t('admin.proxies.pools.created'))
    await loadPools()
  } catch (e: any) {
    appStore.showError(e?.message || t('admin.proxies.pools.failedToSave'))
  } finally {
    saving.value = false
  }
}

function editPool(pool: DynamicProxyPool) {
  selectedPool.value = pool
  Object.assign(editForm, {
    name: pool.name,
    enabled: pool.enabled,
    source_type: pool.source_type,
    subscription_id: pool.subscription_id,
    extract_url: pool.extract_url,
    protocol: pool.protocol,
    auth_mode: pool.auth_mode,
    username: pool.username,
    password: '',
    response_format: pool.response_format,
    line_separator: pool.line_separator || '\\r\\n',
    ip_field_path: pool.ip_field_path,
    port_field_path: pool.port_field_path,
    refresh_interval_sec: pool.refresh_interval_sec,
    ip_duration_sec: pool.ip_duration_sec,
    extract_count: pool.extract_count,
    min_alive: pool.min_alive,
    health_check_interval_sec: pool.health_check_interval_sec ?? 0,
  })
  showEditModal.value = true
}

async function saveEdit() {
  if (!selectedPool.value) return
  if (!editForm.name.trim()) {
    appStore.showError(t('admin.proxies.pools.nameRequired'))
    return
  }
  if (editForm.source_type === 'subscription') {
    if (!editForm.subscription_id) {
      appStore.showError(t('admin.proxies.pools.form.subscription') + ' required')
      return
    }
  } else if (!editForm.extract_url.trim()) {
    appStore.showError(t('admin.proxies.pools.urlRequired'))
    return
  }
  saving.value = true
  try {
    const payload: Record<string, unknown> = { ...editForm }
    if (!editForm.password) delete payload.password
    await dynamicProxyPoolsAPI.update(selectedPool.value.id, payload)
    showEditModal.value = false
    appStore.showSuccess(t('admin.proxies.pools.updated'))
    await loadPools()
  } catch (e: any) {
    appStore.showError(e?.message || t('admin.proxies.pools.failedToSave'))
  } finally {
    saving.value = false
  }
}

function confirmDelete(pool: DynamicProxyPool) {
  selectedPool.value = pool
  deleteMessage.value = t('admin.proxies.pools.deleteConfirm', { name: pool.name })
  showDeleteConfirm.value = true
}

async function doDelete() {
  if (!selectedPool.value) return
  try {
    await dynamicProxyPoolsAPI.delete(selectedPool.value.id)
    showDeleteConfirm.value = false
    appStore.showSuccess(t('admin.proxies.pools.deleted'))
    await loadPools()
  } catch (e: any) {
    appStore.showError(e?.message || t('admin.proxies.pools.failedToDelete'))
  }
}

async function triggerExtract(pool: DynamicProxyPool) {
  extractingId.value = pool.id
  try {
    const result = await dynamicProxyPoolsAPI.extract(pool.id)
    const normalized = {
      created: Number((result as any)?.created ?? (result as any)?.Created ?? 0),
      failed: Number((result as any)?.failed ?? (result as any)?.Failed ?? 0),
      alive_count: Number((result as any)?.alive_count ?? (result as any)?.AliveCount ?? 0)
    }
    extractResult.value = normalized
    showResultModal.value = true
    appStore.showSuccess(
      t('admin.proxies.pools.extractSuccess', {
        created: normalized.created,
        failed: normalized.failed,
        alive: normalized.alive_count
      })
    )
    await loadPools()
  } catch (e: any) {
    appStore.showError(e?.message || t('admin.proxies.pools.failedToExtract'))
  } finally {
    extractingId.value = null
  }
}

async function openNodes(pool: DynamicProxyPool) {
  nodesPoolId.value = pool.id
  selectedNodeIds.value = []
  showNodesModal.value = true
  nodesLoading.value = true
  try {
    const res = await dynamicProxyPoolsAPI.previewNodes(pool.id)
    const items = Array.isArray(res?.nodes) ? res.nodes : []
    nodes.value = items.map((n: any) => ({
      identity: n.identity,
      name: n.name,
      type: n.type,
      server: n.server,
      port: n.port
    }))
  } catch (e: any) {
    appStore.showError(e?.message || 'Failed to load nodes')
    nodes.value = []
  } finally {
    nodesLoading.value = false
  }
}

async function addSelectedNodes() {
  if (!nodesPoolId.value || selectedNodeIds.value.length === 0) return
  try {
    const res = await dynamicProxyPoolsAPI.addNodes(nodesPoolId.value, selectedNodeIds.value)
    const count = (res as any)?.created ?? 0
    appStore.showSuccess(t('admin.proxies.pools.addNodesSuccess', { count }))
    selectedNodeIds.value = []
    showNodesModal.value = false
    await loadPools()
  } catch (e: any) {
    appStore.showError(e?.message || 'Failed to add nodes')
  }
}

async function testPoolProxy(proxy: any) {
  if (!managingPoolId.value) return
  testingId.value = proxy.id
  try {
    const res = await dynamicProxyPoolsAPI.testPoolProxy(managingPoolId.value, proxy.id)
    const data = res as any
    proxy.latency = data?.latency_ms ?? 0
    proxy.status = data?.success ? 'active' : 'error'
    appStore.showSuccess(data?.success
      ? t('admin.proxies.pools.testSuccess') + (data?.latency_ms ? ` (${data.latency_ms}ms)` : '')
      : t('admin.proxies.pools.testFail') + ': ' + (data?.message ?? ''))
  } catch (e: any) {
    appStore.showError(e?.message || t('admin.proxies.pools.testFail'))
  } finally {
    testingId.value = null
  }
}

async function managePool(pool: DynamicProxyPool) {
  managingPoolId.value = pool.id
  selectedPool.value = pool
  poolProxies.value = []
  allProxies.value = []
  selectedAddIds.value = []
  showManageModal.value = true
  try {
    const res = await dynamicProxyPoolsAPI.listProxies(pool.id)
    const items = Array.isArray(res?.items) ? res.items : []
    poolProxies.value = items.map((it: any) => ({
      id: it.id ?? it.ID,
      name: it.name ?? it.Name,
      protocol: it.protocol ?? it.Protocol,
      host: it.host ?? it.Host,
      port: it.port ?? it.Port,
      status: it.status ?? it.Status
    }))
  } catch (e: any) {
    appStore.showError(e?.message || 'Failed to load pool proxies')
  }
}

async function loadAllProxies() {
  try {
    const { proxiesAPI } = await import('@/api/admin')
    const res = await proxiesAPI.list(1, 500)
    const items = Array.isArray(res?.items) ? res.items : []
    const poolIds = new Set(poolProxies.value.map((p) => p.id))
    allProxies.value = items
      .filter((p: any) => !poolIds.has(p.id ?? p.ID))
      .map((p: any) => ({
        id: p.id ?? p.ID,
        name: p.name ?? p.Name,
        protocol: p.protocol ?? p.Protocol,
        host: p.host ?? p.Host,
        port: p.port ?? p.Port,
        status: p.status ?? p.Status
      }))
  } catch (e: any) {
    appStore.showError(e?.message || 'Failed to load proxies')
  }
}

async function addSelectedProxies() {
  if (!managingPoolId.value || selectedAddIds.value.length === 0) return
  try {
    await dynamicProxyPoolsAPI.associateProxies(managingPoolId.value, selectedAddIds.value)
    selectedAddIds.value = []
    allProxies.value = []
    // Reload pool proxies
    const res = await dynamicProxyPoolsAPI.listProxies(managingPoolId.value)
    const items = Array.isArray(res?.items) ? res.items : []
    poolProxies.value = items.map((it: any) => ({
      id: it.id ?? it.ID,
      name: it.name ?? it.Name,
      protocol: it.protocol ?? it.Protocol,
      host: it.host ?? it.Host,
      port: it.port ?? it.Port,
      status: it.status ?? it.Status
    }))
    appStore.showSuccess('Proxies added to pool')
    await loadPools()
  } catch (e: any) {
    appStore.showError(e?.message || 'Failed to add proxies')
  }
}

async function removePoolProxy(proxy: any) {
  if (!managingPoolId.value) return
  try {
    await dynamicProxyPoolsAPI.disassociateProxies(managingPoolId.value, [proxy.id])
    poolProxies.value = poolProxies.value.filter((p) => p.id !== proxy.id)
    appStore.showSuccess('Proxy removed from pool')
    await loadPools()
  } catch (e: any) {
    appStore.showError(e?.message || 'Failed to remove proxy')
  }
}

onMounted(() => {
  loadPools()
  loadSubscriptionOptions()
})
</script>
