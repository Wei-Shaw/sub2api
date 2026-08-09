<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <div class="relative w-full sm:w-64">
            <Icon
              name="search"
              size="md"
              class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
            />
            <input
              v-model="searchQuery"
              type="text"
              :placeholder="t('admin.proxyPools.searchPools')"
              class="input pl-10"
            />
          </div>

          <div class="w-full sm:w-40">
            <Select
              v-model="statusFilter"
              :options="statusOptions"
              :placeholder="t('admin.proxyPools.allStatus')"
              @change="loadPools"
            />
          </div>

          <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
            <button @click="loadPools" :disabled="loading" class="btn btn-secondary" :title="t('common.refresh')">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button @click="openCreate" class="btn btn-primary">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.proxyPools.createPool') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <div class="flex min-h-0 flex-1 flex-col overflow-hidden">
          <DataTable :columns="columns" :data="filteredPools" :loading="loading">
            <template #empty>
              <EmptyState
                :title="t('admin.proxyPools.noPools')"
                :description="t('admin.proxyPools.noPoolsHint')"
              />
            </template>

            <template #cell-name="{ row }">
              <div class="flex items-center gap-2">
                <span class="font-medium text-gray-900 dark:text-white">{{ row.name }}</span>
                <span v-if="row.description" class="max-w-[200px] truncate text-xs text-gray-400">
                  {{ row.description }}
                </span>
              </div>
            </template>

            <template #cell-status="{ value }">
              <span :class="['badge', value === 'active' ? 'badge-success' : 'badge-gray']">
                {{ value === 'active' ? t('admin.proxyPools.statusActive') : t('admin.proxyPools.statusDisabled') }}
              </span>
            </template>

            <template #cell-updated_at="{ value }">
              <span class="whitespace-nowrap text-sm text-gray-600 dark:text-gray-300">
                {{ formatDateTime(value) }}
              </span>
            </template>

            <template #cell-health="{ row }">
              <div class="flex items-center gap-2">
                <span v-if="row.proxy_count === 0" class="text-sm text-gray-400">-</span>
                <template v-else>
                  <span class="badge badge-success" :title="t('admin.proxyPools.healthyProxies')">
                    {{ row.healthy_count }}
                  </span>
                  <span class="badge badge-danger" :title="t('admin.proxyPools.unhealthyProxies')">
                    {{ row.unhealthy_count }}
                  </span>
                  <span class="badge badge-gray" :title="t('admin.proxyPools.unknownProxies')">
                    {{ row.unknown_count }}
                  </span>
                  <span class="text-xs text-gray-500">/ {{ row.proxy_count }}</span>
                </template>
              </div>
            </template>

            <template #cell-bound_account_sum="{ row }">
              <div class="flex items-center gap-2">
                <span>{{ row.bound_account_sum }}</span>
                <span
                  v-if="row.unassigned_account_count > 0"
                  class="badge badge-warning"
                  :title="t('admin.proxyPools.unassignedAccounts', { count: row.unassigned_account_count })"
                >
                  {{ t('admin.proxyPools.unassignedAccountBadge', { count: row.unassigned_account_count }) }}
                </span>
              </div>
            </template>

            <template #cell-auto_rebind="{ value }">
              <span :class="['badge', value ? 'badge-primary' : 'badge-gray']">
                {{ value ? t('admin.proxyPools.autoRebindOn') : t('admin.proxyPools.autoRebindOff') }}
              </span>
            </template>

            <template #cell-actions="{ row }">
              <div class="flex items-center gap-1.5">
                <button class="btn btn-secondary btn-sm" @click="openDetail(row)">
                  {{ t('admin.proxyPools.detail') }}
                </button>
                <button
                  class="btn btn-secondary btn-sm"
                  :title="t('admin.proxyPools.editPool')"
                  @click="openEdit(row)"
                >
                  <Icon name="edit" size="sm" />
                </button>
                <button
                  class="btn btn-danger btn-sm"
                  :title="t('admin.proxyPools.deletePool')"
                  @click="openDelete(row)"
                >
                  <Icon name="trash" size="sm" />
                </button>
              </div>
            </template>
          </DataTable>
        </div>
      </template>
    </TablePageLayout>

    <!-- Create / Edit Pool -->
    <BaseDialog
      :show="showForm"
      :title="editingPool ? t('admin.proxyPools.editPool') : t('admin.proxyPools.createPool')"
      width="normal"
      @close="showForm = false"
    >
      <div class="space-y-4">
        <div>
          <label class="input-label">{{ t('admin.proxyPools.poolName') }} <span class="text-red-500">*</span></label>
          <input v-model.trim="form.name" type="text" class="input" :placeholder="t('admin.proxyPools.poolNamePlaceholder')" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.proxyPools.descriptionLabel') }}</label>
          <textarea v-model.trim="form.description" rows="2" class="input" :placeholder="t('admin.proxyPools.descriptionPlaceholder')" />
        </div>
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="input-label">{{ t('admin.proxyPools.healthInterval') }}</label>
            <div class="flex items-center gap-2">
              <input v-model.number="form.health_interval_seconds" type="number" min="30" max="86400" class="input" />
              <span class="text-xs text-gray-500">{{ t('admin.proxyPools.seconds') }}</span>
            </div>
          </div>
          <div>
            <label class="input-label">{{ t('admin.proxyPools.failureThreshold') }}</label>
            <input v-model.number="form.failure_threshold" type="number" min="1" max="100" class="input" />
          </div>
        </div>
        <div class="flex items-center gap-2">
          <input v-model="form.auto_rebind" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          <span class="text-sm text-gray-700 dark:text-gray-200">{{ t('admin.proxyPools.autoRebindLabel') }}</span>
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <button class="btn btn-secondary" @click="showForm = false">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" :disabled="saving" @click="saveForm">
            {{ saving ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Delete Pool -->
    <ConfirmDialog
      :show="poolToDelete !== null"
      :title="t('admin.proxyPools.deletePool')"
      :message="t('admin.proxyPools.deletePoolConfirm', { name: poolToDelete?.name ?? '' })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      @confirm="confirmDelete"
      @cancel="poolToDelete = null"
    />

    <!-- Remove Proxy From Pool -->
    <ConfirmDialog
      :show="proxyToRemove !== null"
      :title="t('admin.proxyPools.removeFromPool')"
      :message="t('admin.proxyPools.removeProxyConfirm', { name: proxyToRemove?.name ?? '' })"
      :confirm-text="t('common.confirm')"
      :cancel-text="t('common.cancel')"
      @confirm="doRemoveProxy"
      @cancel="proxyToRemove = null"
    />

    <!-- Confirm moving proxies from another pool -->
    <ConfirmDialog
      :show="showReassignConfirm"
      :title="t('admin.proxyPools.reassignProxyTitle')"
      :message="t('admin.proxyPools.reassignProxyConfirm', { count: selectedReassignments.length, name: detailPool?.name ?? '' })"
      :confirm-text="t('admin.proxyPools.reassignProxyAction')"
      :cancel-text="t('common.cancel')"
      @confirm="doAssign"
      @cancel="showReassignConfirm = false"
    >
      <div data-testid="proxy-reassignment-sources" class="space-y-2">
        <div
          v-for="group in reassignSourceGroups"
          :key="group.poolId"
          class="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm dark:border-amber-800 dark:bg-amber-950/30"
        >
          <div class="font-medium text-amber-800 dark:text-amber-300">{{ group.poolName }}</div>
          <div class="mt-0.5 text-xs text-amber-700 dark:text-amber-400">{{ group.proxyNames.join(', ') }}</div>
        </div>
      </div>
    </ConfirmDialog>

    <!-- Pool Detail -->
    <BaseDialog
      :show="detailPool !== null"
      :title="detailPool ? t('admin.proxyPools.poolDetail', { name: detailPool.name }) : ''"
      width="extra-wide"
      @close="closeDetail"
    >
      <div v-if="detailPool" class="space-y-5">
        <!-- 概览统计 -->
        <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-700">
            <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.proxyPools.totalProxies') }}</div>
            <div class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ detailProxies.length }}</div>
          </div>
          <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-700">
            <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.proxyPools.healthyProxies') }}</div>
            <div class="mt-1 text-xl font-semibold text-emerald-600 dark:text-emerald-400">{{ healthyCount }}</div>
          </div>
          <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-700">
            <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.proxyPools.unhealthyProxies') }}</div>
            <div class="mt-1 text-xl font-semibold text-red-600 dark:text-red-400">{{ unhealthyCount }}</div>
          </div>
          <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-700">
            <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.proxyPools.boundAccounts') }}</div>
            <div class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ boundAccountSum }}</div>
            <div
              v-if="detailPool.unassigned_account_count > 0"
              class="mt-1 text-xs text-amber-600 dark:text-amber-400"
            >
              {{ t('admin.proxyPools.unassignedAccounts', { count: detailPool.unassigned_account_count }) }}
            </div>
          </div>
        </div>

        <!-- 池内代理 -->
        <div>
          <div class="mb-2 flex items-center justify-between">
            <h3 class="text-sm font-semibold text-gray-800 dark:text-gray-100">
              {{ t('admin.proxyPools.poolProxies') }}
            </h3>
            <div class="flex items-center gap-2">
              <button class="btn btn-secondary btn-sm" :disabled="rebinding" @click="handleRebind">
                <Icon name="refresh" size="sm" class="mr-1.5" :class="rebinding ? 'animate-spin' : ''" />
                {{ t('admin.proxyPools.rebindNow') }}
              </button>
              <button class="btn btn-primary btn-sm" @click="openAssign">
                <Icon name="plus" size="sm" class="mr-1.5" />
                {{ t('admin.proxyPools.assignProxy') }}
              </button>
            </div>
          </div>
          <div v-if="detailProxies.length === 0" class="rounded-lg border border-dashed border-gray-300 p-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
            {{ t('admin.proxyPools.noPoolProxies') }}
          </div>
          <div v-else class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-600">
            <table class="w-full text-sm">
              <thead class="bg-gray-50 text-left text-xs uppercase tracking-wide text-gray-500 dark:bg-dark-700 dark:text-gray-400">
                <tr>
                  <th class="px-3 py-2">{{ t('admin.proxyPools.proxyName') }}</th>
                  <th class="px-3 py-2">{{ t('admin.proxyPools.health') }}</th>
                  <th class="px-3 py-2">{{ t('admin.proxyPools.failures') }}</th>
                  <th class="px-3 py-2">{{ t('admin.proxyPools.lastChecked') }}</th>
                  <th class="px-3 py-2">{{ t('admin.proxyPools.latency') }}</th>
                  <th class="px-3 py-2">{{ t('admin.proxyPools.boundAccounts') }}</th>
                  <th class="px-3 py-2"></th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-600">
                <tr v-for="p in detailProxies" :key="p.id" class="hover:bg-gray-50 dark:hover:bg-dark-700">
                  <td class="px-3 py-2">
                    <div class="flex flex-col">
                      <span class="font-medium text-gray-900 dark:text-white">{{ p.name }}</span>
                      <code class="text-xs text-gray-500">{{ p.host }}:{{ p.port }}</code>
                    </div>
                  </td>
                  <td class="px-3 py-2">
                    <span :class="['badge', healthBadgeClass(p.pool_health)]">
                      {{ healthLabel(p.pool_health) }}
                    </span>
                  </td>
                  <td class="px-3 py-2 text-gray-700 dark:text-gray-200">{{ p.pool_failures ?? 0 }}</td>
                  <td class="px-3 py-2 text-gray-600 dark:text-gray-300">
                    {{ p.pool_checked_at ? formatDateTime(p.pool_checked_at) : '-' }}
                  </td>
                  <td class="px-3 py-2">
                    <span v-if="p.latency_ms != null" :class="['badge', p.latency_ms < 500 ? 'badge-success' : 'badge-warning']">
                      {{ p.latency_ms }}ms
                    </span>
                    <span v-else class="text-gray-400">-</span>
                  </td>
                  <td class="px-3 py-2 text-gray-700 dark:text-gray-200">{{ p.account_count ?? 0 }}</td>
                  <td class="px-3 py-2 text-right">
                    <button
                      class="text-xs text-red-500 hover:text-red-700 dark:hover:text-red-400"
                      :title="t('admin.proxyPools.removeFromPool')"
                      @click="confirmRemoveProxy(p)"
                    >
                      {{ t('admin.proxyPools.removeFromPool') }}
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <!-- 池绑定账号 -->
        <div data-testid="pool-assigned-accounts">
          <h3 class="mb-2 text-sm font-semibold text-gray-800 dark:text-gray-100">
            {{ t('admin.proxyPools.assignedAccounts') }}
            <span class="font-normal text-gray-500">({{ detailAccountsTotal }})</span>
          </h3>
          <div
            v-if="detailAccountsLoading"
            class="rounded-lg border border-gray-200 p-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400"
          >
            {{ t('common.loading') }}
          </div>
          <div
            v-else-if="detailAccounts.length === 0"
            class="rounded-lg border border-dashed border-gray-300 p-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400"
          >
            {{ t('admin.proxyPools.noAssignedAccounts') }}
          </div>
          <div v-else class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
            <div class="overflow-x-auto">
              <table class="w-full text-sm">
                <thead class="bg-gray-50 text-left text-xs uppercase tracking-wide text-gray-500 dark:bg-dark-700 dark:text-gray-400">
                  <tr>
                    <th class="px-3 py-2">{{ t('admin.proxyPools.accountName') }}</th>
                    <th class="px-3 py-2">{{ t('admin.proxyPools.platform') }}</th>
                    <th class="px-3 py-2">{{ t('admin.proxyPools.accountType') }}</th>
                    <th class="px-3 py-2">{{ t('admin.proxyPools.status') }}</th>
                    <th class="px-3 py-2">{{ t('admin.proxyPools.currentProxy') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-600">
                  <tr
                    v-for="account in detailAccounts"
                    :key="account.id"
                    class="hover:bg-gray-50 dark:hover:bg-dark-700"
                  >
                    <td class="px-3 py-2">
                      <span class="font-medium text-gray-900 dark:text-white">{{ account.name }}</span>
                      <span class="ml-1 text-xs text-gray-400">#{{ account.id }}</span>
                    </td>
                    <td class="px-3 py-2 text-gray-700 dark:text-gray-200">{{ account.platform }}</td>
                    <td class="px-3 py-2 text-gray-700 dark:text-gray-200">{{ account.type }}</td>
                    <td class="px-3 py-2">
                      <span :class="['badge', account.status === 'active' ? 'badge-success' : 'badge-gray']">
                        {{ account.status }}
                      </span>
                    </td>
                    <td class="px-3 py-2 text-gray-700 dark:text-gray-200">
                      {{ account.proxy_name || (account.proxy_id ? `#${account.proxy_id}` : t('admin.proxyPools.directConnection')) }}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <Pagination
              v-if="detailAccountsTotal > detailAccountsPageSize"
              :page="detailAccountsPage"
              :total="detailAccountsTotal"
              :page-size="detailAccountsPageSize"
              :show-page-size-selector="false"
              @update:page="handleDetailAccountsPage"
            />
          </div>
        </div>

        <!-- 重绑日志 -->
        <div>
          <h3 class="mb-2 text-sm font-semibold text-gray-800 dark:text-gray-100">
            {{ t('admin.proxyPools.rebindLogs') }}
          </h3>
          <div v-if="rebindLogs.length === 0" class="rounded-lg border border-dashed border-gray-300 p-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
            {{ t('admin.proxyPools.noRebindLogs') }}
          </div>
          <div v-else class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-600">
            <table class="w-full text-sm">
              <thead class="bg-gray-50 text-left text-xs uppercase tracking-wide text-gray-500 dark:bg-dark-700 dark:text-gray-400">
                <tr>
                  <th class="px-3 py-2">{{ t('admin.proxyPools.time') }}</th>
                  <th class="px-3 py-2">{{ t('admin.proxyPools.fromProxy') }}</th>
                  <th class="px-3 py-2">{{ t('admin.proxyPools.toProxy') }}</th>
                  <th class="px-3 py-2">{{ t('admin.proxyPools.accounts') }}</th>
                  <th class="px-3 py-2">{{ t('admin.proxyPools.reason') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-600">
                <tr v-for="log in rebindLogs" :key="log.id" class="hover:bg-gray-50 dark:hover:bg-dark-700">
                  <td class="whitespace-nowrap px-3 py-2 text-gray-600 dark:text-gray-300">
                    {{ formatDateTime(log.created_at) }}
                  </td>
                  <td class="px-3 py-2 text-gray-700 dark:text-gray-200">{{ log.from_proxy_name || log.from_proxy_id || '-' }}</td>
                  <td class="px-3 py-2 text-gray-700 dark:text-gray-200">{{ log.to_proxy_name || log.to_proxy_id || '-' }}</td>
                  <td class="px-3 py-2">
                    <span class="badge badge-primary">{{ log.account_count }}</span>
                  </td>
                  <td class="px-3 py-2 text-gray-600 dark:text-gray-300">{{ reasonLabel(log.reason) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </BaseDialog>

    <!-- Assign Proxies -->
    <BaseDialog
      :show="showAssign"
      :title="t('admin.proxyPools.assignProxy')"
      width="wide"
      @close="closeAssign"
    >
      <div class="mb-3 flex items-center justify-between">
        <div class="relative w-full sm:w-64">
          <Icon
            name="search"
            size="sm"
            class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
          />
          <input v-model.trim="assignSearch" type="text" class="input pl-9" :placeholder="t('admin.proxyPools.searchProxies')" />
        </div>
        <span class="text-xs text-gray-500">
          {{ t('admin.proxyPools.selectedCount', { count: assignSelected.size }) }}
        </span>
      </div>
      <div v-if="assignableProxies.length === 0" class="py-8 text-center text-sm text-gray-500">
        {{ t('admin.proxyPools.noAssignableProxies') }}
      </div>
      <div v-else class="max-h-80 space-y-1 overflow-y-auto">
        <label
          v-for="p in filteredAssignableProxies"
          :key="p.id"
          class="flex cursor-pointer items-center gap-3 rounded-lg px-3 py-2 hover:bg-gray-50 dark:hover:bg-dark-700"
        >
          <input
            type="checkbox"
            class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            :checked="assignSelected.has(p.id)"
            @change="toggleAssign(p.id)"
          />
          <div class="flex min-w-0 flex-1 items-center justify-between gap-3">
            <div class="flex min-w-0 flex-col">
              <span class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ p.name }}</span>
              <code class="truncate text-xs text-gray-500">{{ p.host }}:{{ p.port }}</code>
            </div>
            <div class="flex shrink-0 items-center gap-2">
              <span
                v-if="p.pool_id && p.pool_id !== detailPool?.id"
                class="badge badge-warning"
              >
                {{ t('admin.proxyPools.boundToPool', { name: poolDisplayName(p.pool_id) }) }}
              </span>
              <span v-if="p.status !== 'active'" class="badge badge-gray">{{ p.status }}</span>
            </div>
          </div>
        </label>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <button class="btn btn-secondary" @click="closeAssign">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" :disabled="assignSaving || assignSelected.size === 0" @click="confirmAssign">
            {{ assignSaving ? t('common.saving') : t('admin.proxyPools.assignConfirm') }}
          </button>
        </div>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { refDebounced } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { Proxy, ProxyPoolWithStats, ProxyPoolRebindLog, ProxyPoolAccountSummary } from '@/types'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const pools = ref<ProxyPoolWithStats[]>([])
const searchQuery = ref('')
const debouncedSearchQuery = refDebounced(searchQuery, 250)
const statusFilter = ref('')

const statusOptions = [
  { label: t('admin.proxyPools.allStatus'), value: '' },
  { label: t('admin.proxyPools.statusActive'), value: 'active' },
  { label: t('admin.proxyPools.statusDisabled'), value: 'disabled' }
]

const columns: Column[] = [
  { key: 'name', label: t('admin.proxyPools.poolName') },
  { key: 'status', label: t('admin.proxyPools.status') },
  { key: 'health', label: t('admin.proxyPools.health') },
  { key: 'bound_account_sum', label: t('admin.proxyPools.boundAccounts') },
  { key: 'health_interval_seconds', label: t('admin.proxyPools.healthInterval') },
  { key: 'failure_threshold', label: t('admin.proxyPools.failureThreshold') },
  { key: 'auto_rebind', label: t('admin.proxyPools.autoRebind') },
  { key: 'updated_at', label: t('admin.proxyPools.updatedAt') },
  { key: 'actions', label: '' }
]

const filteredPools = computed(() => {
  const q = debouncedSearchQuery.value.trim().toLowerCase()
  return pools.value.filter((p) => {
    if (statusFilter.value && p.status !== statusFilter.value) return false
    if (q && !p.name.toLowerCase().includes(q) && !(p.description ?? '').toLowerCase().includes(q)) return false
    return true
  })
})

async function loadPools() {
  loading.value = true
  try {
    const refreshedPools = await adminAPI.proxyPools.list()
    pools.value = refreshedPools
    if (detailPool.value) {
      detailPool.value = refreshedPools.find((pool) => pool.id === detailPool.value?.id) ?? null
    }
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.proxyPools.failedToLoad')))
  } finally {
    loading.value = false
  }
}


// ── 创建 / 编辑 ──────────────────────────────────────────
const showForm = ref(false)
const editingPool = ref<ProxyPoolWithStats | null>(null)
const saving = ref(false)
const form = reactive({
  name: '',
  description: '',
  health_interval_seconds: 300,
  failure_threshold: 2,
  auto_rebind: true
})

function openCreate() {
  editingPool.value = null
  form.name = ''
  form.description = ''
  form.health_interval_seconds = 300
  form.failure_threshold = 2
  form.auto_rebind = true
  showForm.value = true
}

function openEdit(pool: ProxyPoolWithStats) {
  editingPool.value = pool
  form.name = pool.name
  form.description = pool.description ?? ''
  form.health_interval_seconds = pool.health_interval_seconds
  form.failure_threshold = pool.failure_threshold
  form.auto_rebind = pool.auto_rebind
  showForm.value = true
}

async function saveForm() {
  if (!form.name.trim()) {
    appStore.showError(t('admin.proxyPools.nameRequired'))
    return
  }
  saving.value = true
  try {
    const payload = {
      name: form.name.trim(),
      description: form.description.trim() || null,
      health_interval_seconds: form.health_interval_seconds,
      failure_threshold: form.failure_threshold,
      auto_rebind: form.auto_rebind
    }
    if (editingPool.value) {
      await adminAPI.proxyPools.update(editingPool.value.id, payload)
      appStore.showSuccess(t('admin.proxyPools.updateSuccess'))
    } else {
      await adminAPI.proxyPools.create(payload)
      appStore.showSuccess(t('admin.proxyPools.createSuccess'))
    }
    showForm.value = false
    await loadPools()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.proxyPools.saveFailed')))
  } finally {
    saving.value = false
  }
}

// ── 删除 ────────────────────────────────────────────────
const poolToDelete = ref<ProxyPoolWithStats | null>(null)

function openDelete(pool: ProxyPoolWithStats) {
  poolToDelete.value = pool
}

async function confirmDelete() {
  if (!poolToDelete.value) return
  try {
    await adminAPI.proxyPools.remove(poolToDelete.value.id)
    appStore.showSuccess(t('admin.proxyPools.deleteSuccess'))
    if (detailPool.value?.id === poolToDelete.value.id) closeDetail()
    poolToDelete.value = null
    await loadPools()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.proxyPools.deleteFailed')))
  }
}

// ── 详情 ────────────────────────────────────────────────
const detailPool = ref<ProxyPoolWithStats | null>(null)
const detailProxies = ref<Proxy[]>([])
const rebindLogs = ref<ProxyPoolRebindLog[]>([])
const detailAccounts = ref<ProxyPoolAccountSummary[]>([])
const detailAccountsPage = ref(1)
const detailAccountsPageSize = 10
const detailAccountsTotal = ref(0)
const detailAccountsLoading = ref(false)
const rebinding = ref(false)

const healthyCount = computed(() => detailProxies.value.filter((p) => p.pool_health === 'healthy').length)
const unhealthyCount = computed(() => detailProxies.value.filter((p) => p.pool_health === 'unhealthy').length)
const boundAccountSum = computed(() => detailPool.value?.bound_account_sum ?? detailAccountsTotal.value)

async function openDetail(pool: ProxyPoolWithStats) {
  detailPool.value = pool
  detailProxies.value = []
  rebindLogs.value = []
  detailAccounts.value = []
  detailAccountsPage.value = 1
  detailAccountsTotal.value = 0
  await Promise.all([
    loadDetailProxies(pool.id),
    loadDetailAccounts(pool.id, 1),
    loadRebindLogs(pool.id)
  ])
}

function closeDetail() {
  detailPool.value = null
  detailProxies.value = []
  rebindLogs.value = []
  detailAccounts.value = []
  detailAccountsPage.value = 1
  detailAccountsTotal.value = 0
}

async function loadDetailProxies(poolId: number) {
  try {
    detailProxies.value = await adminAPI.proxyPools.listProxies(poolId)
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.proxyPools.failedToLoadProxies')))
  }
}

async function loadDetailAccounts(poolId: number, page = detailAccountsPage.value) {
  detailAccountsLoading.value = true
  try {
    const result = await adminAPI.proxyPools.listAccounts(poolId, page, detailAccountsPageSize)
    if (detailPool.value?.id !== poolId) return
    detailAccounts.value = result.items
    detailAccountsTotal.value = result.total
    detailAccountsPage.value = result.page
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.proxyPools.failedToLoadAccounts')))
  } finally {
    if (detailPool.value?.id === poolId) detailAccountsLoading.value = false
  }
}

async function handleDetailAccountsPage(page: number) {
  if (!detailPool.value || page === detailAccountsPage.value) return
  detailAccountsPage.value = page
  await loadDetailAccounts(detailPool.value.id, page)
}

async function loadRebindLogs(poolId: number) {
  try {
    rebindLogs.value = await adminAPI.proxyPools.rebindLogs(poolId, 50)
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.proxyPools.failedToLoadLogs')))
  }
}

async function handleRebind() {
  if (!detailPool.value || rebinding.value) return
  rebinding.value = true
  try {
    const result = await adminAPI.proxyPools.rebind(detailPool.value.id)
    if (result.partialFailure) {
      appStore.showError(
        t('admin.proxyPools.rebindPartial', {
          count: String(result.reboundAccounts),
          failed: String(result.failedProxies)
        })
      )
    } else {
      appStore.showSuccess(
        t('admin.proxyPools.rebindDone', { count: String(result.reboundAccounts) })
      )
    }
    await Promise.all([
      loadDetailProxies(detailPool.value.id),
      loadDetailAccounts(detailPool.value.id, detailAccountsPage.value),
      loadRebindLogs(detailPool.value.id),
      loadPools()
    ])
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.proxyPools.rebindFailed')))
  } finally {
    rebinding.value = false
  }
}

// ── 分配代理 ────────────────────────────────────────────
const showAssign = ref(false)
const assignSaving = ref(false)
const assignSearch = ref('')
const allProxies = ref<Proxy[]>([])
const assignSelected = ref(new Set<number>())
const showReassignConfirm = ref(false)

const assignableProxies = computed(() => {
  const inPool = new Set(detailProxies.value.map((p) => p.id))
  return allProxies.value.filter((p) => !inPool.has(p.id))
})

const selectedReassignments = computed(() => {
  const currentPoolId = detailPool.value?.id
  if (!currentPoolId) return []
  return assignableProxies.value.filter(
    (proxy) => assignSelected.value.has(proxy.id) && proxy.pool_id && proxy.pool_id !== currentPoolId
  )
})

const reassignSourceGroups = computed(() => {
  const groups = new Map<number, { poolId: number; poolName: string; proxyNames: string[] }>()
  for (const proxy of selectedReassignments.value) {
    const poolId = proxy.pool_id as number
    const group = groups.get(poolId) ?? { poolId, poolName: poolDisplayName(poolId), proxyNames: [] }
    group.proxyNames.push(proxy.name)
    groups.set(poolId, group)
  }
  return [...groups.values()]
})

function poolDisplayName(poolId: number): string {
  return pools.value.find((pool) => pool.id === poolId)?.name ?? `#${poolId}`
}

const filteredAssignableProxies = computed(() => {
  const q = assignSearch.value.trim().toLowerCase()
  if (!q) return assignableProxies.value
  return assignableProxies.value.filter(
    (p) => p.name.toLowerCase().includes(q) || p.host.toLowerCase().includes(q)
  )
})

async function openAssign() {
  assignSelected.value = new Set()
  assignSearch.value = ''
  showAssign.value = true
  showReassignConfirm.value = false
  try {
    allProxies.value = await adminAPI.proxies.getAll()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.proxyPools.failedToLoadProxies')))
  }
}

function closeAssign() {
  showAssign.value = false
  showReassignConfirm.value = false
}

function toggleAssign(id: number) {
  const next = new Set(assignSelected.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  assignSelected.value = next
}

function confirmAssign() {
  if (!detailPool.value || assignSelected.value.size === 0) return
  if (selectedReassignments.value.length > 0) {
    showReassignConfirm.value = true
    return
  }
  void doAssign()
}

async function doAssign() {
  if (!detailPool.value || assignSelected.value.size === 0 || assignSaving.value) return
  showReassignConfirm.value = false
  assignSaving.value = true
  try {
    const assigned = await adminAPI.proxyPools.assignProxies(detailPool.value.id, [...assignSelected.value])
    appStore.showSuccess(t('admin.proxyPools.assignSuccess', { count: String(assigned) }))
    closeAssign()
    await Promise.all([loadDetailProxies(detailPool.value.id), loadPools()])
    await loadDetailAccounts(detailPool.value.id, detailAccountsPage.value)
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.proxyPools.assignFailed')))
  } finally {
    assignSaving.value = false
  }
}

// ── 移出代理 ────────────────────────────────────────────
const proxyToRemove = ref<Proxy | null>(null)

function confirmRemoveProxy(p: Proxy) {
  proxyToRemove.value = p
}

async function doRemoveProxy() {
  if (!detailPool.value || !proxyToRemove.value) return
  try {
    await adminAPI.proxyPools.removeProxies(detailPool.value.id, [proxyToRemove.value.id])
    appStore.showSuccess(t('admin.proxyPools.removeSuccess'))
    proxyToRemove.value = null
    await Promise.all([loadDetailProxies(detailPool.value.id), loadPools()])
    await loadDetailAccounts(detailPool.value.id, detailAccountsPage.value)
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.proxyPools.removeFailed')))
  }
}

// ── 展示辅助 ────────────────────────────────────────────
function healthBadgeClass(health?: string): string {
  if (health === 'healthy') return 'badge-success'
  if (health === 'unhealthy') return 'badge-danger'
  return 'badge-gray'
}

function healthLabel(health?: string): string {
  if (health === 'healthy') return t('admin.proxyPools.healthy')
  if (health === 'unhealthy') return t('admin.proxyPools.unhealthy')
  return t('admin.proxyPools.unknown')
}

function reasonLabel(reason: string): string {
  if (reason === 'unhealthy') return t('admin.proxyPools.reasonUnhealthy')
  if (reason === 'manual') return t('admin.proxyPools.reasonManual')
  return reason
}

onMounted(() => {
  loadPools()
})

</script>
